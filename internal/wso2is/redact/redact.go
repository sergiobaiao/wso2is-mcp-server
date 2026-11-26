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

// Package redact removes sensitive values from WSO2 IS API responses and error payloads.
package redact

import (
	"encoding/json"
	"strings"
)

// RedactedPlaceholder replaces removed sensitive values so callers know the field exists.
const RedactedPlaceholder = "[REDACTED]"

var exactSensitiveKeys = map[string]bool{
	"clientsecret":      true,
	"client_secret":     true,
	"password":          true,
	"privatekey":        true,
	"secret":            true,
	"token":             true,
	"access_token":      true,
	"refresh_token":     true,
	"id_token":          true,
	"apikey":            true,
	"key":               true,
	"qrcodeurl":         true,
	"backupcodes":       true,
	"devicetoken":       true,
	"challenge":         true,
	"publickey":         true,
	"publickeycose":     true,
	"attestationobject": true,
	"clientdatajson":    true,
	"signature":         true,
}

var sensitiveSubstrings = []string{
	"secret",
	"password",
	"passwd",
	"privatekey",
	"apikey",
}

// Value recursively redacts secrets from decoded JSON/Go values.
// Non-map/slice primitives are returned unchanged.
func Value(v any) any {
	return redactValue(normalizeForRedact(v), false)
}

// JSONBytes parses raw JSON and returns redacted JSON bytes.
// If raw is not valid JSON, it returns raw unchanged unless it looks empty.
func JSONBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	redacted := Value(v)
	out, err := json.Marshal(redacted)
	if err != nil {
		return raw
	}
	return out
}

func normalizeForRedact(v any) any {
	switch val := v.(type) {
	case nil, string, bool, float64, int, int32, int64:
		return v
	case map[string]any, []any:
		return v
	default:
		// Convert structs/typed maps/slices via JSON roundtrip so struct tags and fields are covered uniformly.
		b, err := json.Marshal(val)
		if err != nil {
			return v
		}
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			return v
		}
		return out
	}
}

func redactValue(v any, inPrivate bool) any {
	switch val := v.(type) {
	case map[string]any:
		return redactMap(val, inPrivate)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = redactValue(item, inPrivate)
		}
		return out
	default:
		return v
	}
}

func redactMap(m map[string]any, inPrivate bool) map[string]any {
	out := make(map[string]any, len(m))

	// Check IS property array element shape: {"key": "...", "value": "..."} or {"name": "...", "value": "..."}.
	propName := propertyItemName(m)

	for k, item := range m {
		lower := strings.ToLower(k)

		if propName != "" && lower == "value" && isSensitiveKeyName(propName, inPrivate) {
			out[k] = RedactedPlaceholder
			continue
		}

		// Preserve property entry metadata keys ("key" / "name") so property arrays stay identifiable.
		if propName != "" && (lower == "key" || lower == "name") {
			out[k] = item
			continue
		}

		nextPrivate := inPrivate || strings.Contains(lower, "privatekey") || strings.Contains(lower, "private_key")

		if strings.Contains(lower, "credential") {
			if _, isStr := item.(string); isStr {
				out[k] = RedactedPlaceholder
				continue
			}
			out[k] = redactValue(item, nextPrivate)
			continue
		}

		if isSensitiveKeyName(k, inPrivate) {
			switch item.(type) {
			case map[string]any:
				// If a sensitive container key like privateKey holds a map, redact all sensitive children inside it.
				out[k] = redactValue(item, nextPrivate)
			default:
				out[k] = RedactedPlaceholder
			}
			continue
		}

		out[k] = redactValue(item, nextPrivate)
	}
	return out
}

func propertyItemName(m map[string]any) string {
	if _, hasVal := m["value"]; !hasVal {
		return ""
	}
	if s, ok := m["key"].(string); ok && s != "" {
		return s
	}
	if s, ok := m["name"].(string); ok && s != "" {
		return s
	}
	return ""
}

func isSensitiveKeyName(key string, inPrivate bool) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	if inPrivate && lower == "certificate" {
		return true
	}
	if exactSensitiveKeys[lower] {
		return true
	}
	if strings.HasSuffix(lower, "token") {
		return true
	}
	for _, sub := range sensitiveSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
