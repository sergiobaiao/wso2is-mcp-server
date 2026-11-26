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

package toolkit

import (
	"fmt"
	"strings"
)

// Argument readers for curated tool handlers. Errors use the contract format
// "invalid input: <field>: <reason>" (contracts/tool-conventions.md).

// RequireString returns the trimmed, non-empty string argument name.
func RequireString(args map[string]any, name string) (string, error) {
	s, err := OptString(args, name)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", fmt.Errorf("invalid input: %s: is required", name)
	}
	return s, nil
}

// OptString returns the trimmed string argument name, or "" when it is absent.
func OptString(args map[string]any, name string) (string, error) {
	raw, ok := args[name]
	if !ok || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("invalid input: %s: must be a string", name)
	}
	return strings.TrimSpace(s), nil
}

// OptInt returns the integer argument name; ok is false when it is absent.
func OptInt(args map[string]any, name string) (n int, ok bool, err error) {
	raw, present := args[name]
	if !present || raw == nil {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case int:
		return v, true, nil
	case int64:
		return int(v), true, nil
	case float64:
		if v != float64(int(v)) {
			return 0, false, fmt.Errorf("invalid input: %s: must be an integer", name)
		}
		return int(v), true, nil
	default:
		return 0, false, fmt.Errorf("invalid input: %s: must be an integer", name)
	}
}

// OptBool returns the boolean argument name; ok is false when it is absent.
func OptBool(args map[string]any, name string) (b bool, ok bool, err error) {
	raw, present := args[name]
	if !present || raw == nil {
		return false, false, nil
	}
	v, isBool := raw.(bool)
	if !isBool {
		return false, false, fmt.Errorf("invalid input: %s: must be a boolean", name)
	}
	return v, true, nil
}

// OptStrings returns the string-array argument name (nil when absent).
func OptStrings(args map[string]any, name string) ([]string, error) {
	raw, present := args[name]
	if !present || raw == nil {
		return nil, nil
	}
	var items []any
	switch v := raw.(type) {
	case []any:
		items = v
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("invalid input: %s: must be an array of strings", name)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, fmt.Errorf("invalid input: %s: must be an array of strings", name)
		}
		out = append(out, s)
	}
	return out, nil
}

// OptArray returns the array argument name (nil when absent).
func OptArray(args map[string]any, name string) ([]any, error) {
	raw, present := args[name]
	if !present || raw == nil {
		return nil, nil
	}
	v, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: %s: must be an array", name)
	}
	return v, nil
}

// OptObject returns the object argument name (nil when absent).
func OptObject(args map[string]any, name string) (map[string]any, error) {
	raw, present := args[name]
	if !present || raw == nil {
		return nil, nil
	}
	v, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: %s: must be an object", name)
	}
	return v, nil
}

// PutIf copies optional arguments into body under new keys when they are present:
// mapping is argument name to body key.
func PutIf(body, args map[string]any, mapping map[string]string) {
	for arg, key := range mapping {
		if v, ok := args[arg]; ok && v != nil {
			body[key] = v
		}
	}
}

// Paging copies the optional "limit" and "offset" arguments into a query map.
func Paging(args map[string]any) (map[string]any, error) {
	q := map[string]any{}
	for _, name := range []string{"limit", "offset"} {
		n, ok, err := OptInt(args, name)
		if err != nil {
			return nil, err
		}
		if ok {
			if n < 0 {
				return nil, fmt.Errorf("invalid input: %s: must not be negative", name)
			}
			q[name] = n
		}
	}
	return q, nil
}
