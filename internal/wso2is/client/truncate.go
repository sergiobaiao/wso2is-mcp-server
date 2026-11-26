/*
 * Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package client

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
)

// Truncate limits oversized JSON responses to maxBytes by cutting at the last complete
// element of a top-level array or SCIM Resources array and attaching truncation metadata.
func Truncate(val any, maxBytes int, op *catalog.Operation) any {
	if val == nil || maxBytes <= 0 {
		return val
	}

	raw, err := json.Marshal(val)
	if err != nil || len(raw) <= maxBytes {
		return val
	}

	normalized := val
	switch normalized.(type) {
	case []any, map[string]any:
	default:
		var generic any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return val
		}
		normalized = generic
	}

	hint := pagingHint(op)

	if arr, ok := normalized.([]any); ok {
		return truncateArray(arr, maxBytes, hint)
	}

	if obj, ok := normalized.(map[string]any); ok {
		if resources, ok := obj["Resources"].([]any); ok {
			return truncateSCIMResources(obj, resources, maxBytes, hint)
		}
	}

	return map[string]any{
		"truncated": true,
		"returned":  0,
		"hint":      hint,
	}
}

func truncateArray(arr []any, maxBytes int, hint string) map[string]any {
	total := len(arr)
	build := func(k int) map[string]any {
		return map[string]any{
			"items":     arr[:k],
			"truncated": true,
			"returned":  k,
			"total":     total,
			"hint":      hint,
		}
	}

	best := largestFittingPrefix(total, maxBytes, func(k int) any {
		return build(k)
	})
	return build(best)
}

func truncateSCIMResources(orig map[string]any, resources []any, maxBytes int, hint string) map[string]any {
	total := len(resources)
	if tr, ok := numericTotal(orig["totalResults"]); ok {
		total = tr
	}

	build := func(k int) map[string]any {
		out := make(map[string]any, len(orig)+3)
		maps.Copy(out, orig)
		out["Resources"] = resources[:k]
		out["truncated"] = true
		out["returned"] = k
		out["total"] = total
		out["hint"] = hint
		return out
	}

	best := largestFittingPrefix(len(resources), maxBytes, func(k int) any {
		return build(k)
	})
	return build(best)
}

func largestFittingPrefix(n, maxBytes int, makeVal func(k int) any) int {
	lo, hi := 0, n
	best := 0
	for lo <= hi {
		mid := lo + (hi-lo)/2
		b, err := json.Marshal(makeVal(mid))
		if err == nil && len(b) <= maxBytes {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best
}

func numericTotal(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func pagingHint(op *catalog.Operation) string {
	if op == nil {
		return "Response exceeded max bytes; use pagination or filtering parameters to narrow the result."
	}

	present := make(map[string]bool)
	for _, p := range op.Params {
		if p.In == "query" {
			present[p.Name] = true
		}
	}

	var names []string
	for _, candidate := range []string{"limit", "offset", "count", "startIndex", "before", "after"} {
		if present[candidate] {
			names = append(names, candidate)
		}
	}

	if len(names) == 0 {
		return "Response exceeded max bytes; use pagination or filtering parameters to narrow the result."
	}
	return fmt.Sprintf("Response exceeded max bytes; paginate using query parameters: %s.", strings.Join(names, ", "))
}
