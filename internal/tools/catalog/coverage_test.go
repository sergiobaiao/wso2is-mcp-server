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

package catalog

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
	iscatalog "github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/getkin/kin-openapi/openapi3"
)

var pathPlaceholderRe = regexp.MustCompile(`\{([^}]+)\}`)

func TestCoverage_All730OperationsReachable(t *testing.T) {
	cat := load730Catalog(t)

	t.Run("default config executes all available ops and refuses flow/deprecated ops", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{
			AllowFlowAPIs:     false,
			IncludeDeprecated: false,
			ReadOnly:          false,
		}, nil)

		var mu sync.Mutex
		hits := make(map[string]int)
		h.srv.SetFallback(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[r.Method+" "+r.URL.Path]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		})

		for _, op := range cat.Operations {
			args := buildMinimalCallArgs(op)
			toolName := "call_write_operation"
			if op.Kind == iscatalog.KindRead {
				toolName = "call_read_operation"
			}

			expectedPath := resolveExpectedPath(op.Path, args)
			routeKey := strings.ToUpper(op.Method) + " " + expectedPath

			mu.Lock()
			before := hits[routeKey]
			mu.Unlock()

			res, text := callTool(t, h.mcpClient, toolName, args)

			switch {
			case op.Flow:
				if !res.IsError || !strings.Contains(text, "ALLOW_FLOW_APIS=true") {
					t.Errorf("%s (flow): got (isError=%v, text=%q), want ALLOW_FLOW_APIS=true refusal", op.ID, res.IsError, text)
				}
			case op.Deprecated:
				if !res.IsError || !strings.Contains(text, "INCLUDE_DEPRECATED=true") {
					t.Errorf("%s (deprecated): got (isError=%v, text=%q), want INCLUDE_DEPRECATED=true refusal", op.ID, res.IsError, text)
				}
			default:
				if res.IsError {
					t.Errorf("%s (%s %s): unexpected error: %s", op.ID, op.Method, op.Path, text)
					continue
				}
				mu.Lock()
				after := hits[routeKey]
				mu.Unlock()
				if after <= before {
					t.Errorf("%s: expected fake IS hit on %s, got none", op.ID, routeKey)
				}
			}
		}
	})

	t.Run("all flags enabled reaches every single operation in 7.3.0", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{
			AllowFlowAPIs:     true,
			IncludeDeprecated: true,
			ReadOnly:          false,
		}, nil)

		var mu sync.Mutex
		hits := make(map[string]int)
		h.srv.SetFallback(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[r.Method+" "+r.URL.Path]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		})

		for _, op := range cat.Operations {
			args := buildMinimalCallArgs(op)
			toolName := "call_write_operation"
			if op.Kind == iscatalog.KindRead {
				toolName = "call_read_operation"
			}
			expectedPath := resolveExpectedPath(op.Path, args)
			routeKey := strings.ToUpper(op.Method) + " " + expectedPath

			mu.Lock()
			before := hits[routeKey]
			mu.Unlock()

			res, text := callTool(t, h.mcpClient, toolName, args)
			if res.IsError {
				t.Errorf("%s (%s %s): unexpected error: %s", op.ID, op.Method, op.Path, text)
				continue
			}

			mu.Lock()
			after := hits[routeKey]
			mu.Unlock()
			if after <= before {
				t.Errorf("%s: expected fake IS hit on %s, got none", op.ID, routeKey)
			}
		}
	})
}

func resolveExpectedPath(pathTemplate string, args map[string]any) string {
	pp, _ := args["path_params"].(map[string]any)
	return pathPlaceholderRe.ReplaceAllStringFunc(pathTemplate, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "{"), "}")
		if pp != nil {
			if v, ok := pp[name].(string); ok {
				return url.PathEscape(v)
			}
		}
		return match
	})
}

func buildMinimalCallArgs(op *iscatalog.Operation) map[string]any {
	norm := normalizeOpForCall(op)
	args := map[string]any{
		"operation_id": norm.ID,
	}
	if norm.OrgScoped() {
		args["organization_id"] = "test-org-id"
	}

	pathParams := make(map[string]any)
	query := make(map[string]any)

	for _, p := range norm.Params {
		switch p.In {
		case "path":
			pathParams[p.Name] = samplePathParam(p.Name, p.Schema)
		case "query":
			if p.Required {
				query[p.Name] = sampleValueForSchema(p.Schema, 0, map[string]bool{})
			}
		}
	}
	for _, match := range pathPlaceholderRe.FindAllStringSubmatch(norm.Path, -1) {
		name := match[1]
		if _, ok := pathParams[name]; !ok {
			pathParams[name] = "test-" + name
		}
	}

	if len(pathParams) > 0 {
		args["path_params"] = pathParams
	}
	if len(query) > 0 {
		args["query"] = query
	}
	if norm.Body != nil && norm.Body.Required {
		if norm.Body.ContentType == "multipart/form-data" {
			args["body"] = sampleMultipartBody(norm.Body.Schema)
		} else if norm.Body.ContentType == "application/x-www-form-urlencoded" {
			args["body"] = sampleFormBody(norm.Body.Schema)
		} else if norm.Body.Schema != nil {
			args["body"] = sampleValueForSchema(norm.Body.Schema, 0, map[string]bool{})
		} else {
			args["body"] = map[string]any{}
		}
	}
	return args
}

func samplePathParam(name string, sr *openapi3.SchemaRef) string {
	if sr != nil && sr.Value != nil {
		if len(sr.Value.Enum) > 0 {
			if s, ok := sr.Value.Enum[0].(string); ok && s != "" {
				return s
			}
		}
		if sr.Value.Type != nil {
			if sr.Value.Type.Is(openapi3.TypeInteger) || sr.Value.Type.Is(openapi3.TypeNumber) {
				return "1"
			}
			if sr.Value.Type.Is(openapi3.TypeBoolean) {
				return "true"
			}
		}
	}
	return "test-" + name
}

func sampleMultipartBody(sr *openapi3.SchemaRef) map[string]any {
	out := map[string]any{}
	if sr == nil || sr.Value == nil {
		return out
	}
	for name, prop := range sr.Value.Properties {
		if prop != nil && prop.Value != nil && strings.EqualFold(prop.Value.Format, "binary") {
			out[name] = base64.StdEncoding.EncodeToString([]byte("test-data"))
		} else {
			out[name] = sampleValueForSchema(prop, 0, map[string]bool{})
		}
	}
	return out
}

func sampleFormBody(sr *openapi3.SchemaRef) map[string]any {
	out := map[string]any{}
	if sr == nil || sr.Value == nil {
		return out
	}
	for _, reqName := range sr.Value.Required {
		if prop, ok := sr.Value.Properties[reqName]; ok {
			out[reqName] = sampleValueForSchema(prop, 0, map[string]bool{})
		} else {
			out[reqName] = "sample"
		}
	}
	return out
}

func sampleValueForSchema(sr *openapi3.SchemaRef, depth int, visited map[string]bool) any {
	if sr == nil || sr.Value == nil || depth > 5 {
		return "sample"
	}
	if sr.Ref != "" {
		if visited[sr.Ref] {
			return map[string]any{}
		}
		visited[sr.Ref] = true
		defer delete(visited, sr.Ref)
	}

	s := sr.Value
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}
	if s.Default != nil {
		return s.Default
	}
	if len(s.AllOf) > 0 {
		merged := map[string]any{}
		for _, sub := range s.AllOf {
			if m, ok := sampleValueForSchema(sub, depth+1, visited).(map[string]any); ok {
				for k, v := range m {
					merged[k] = v
				}
			}
		}
		if len(merged) > 0 {
			return merged
		}
		return sampleValueForSchema(s.AllOf[0], depth+1, visited)
	}
	if len(s.OneOf) > 0 {
		val := sampleValueForSchema(s.OneOf[0], depth+1, visited)
		if obj, ok := val.(map[string]any); ok && len(s.OneOf) > 1 {
			for _, other := range s.OneOf[1:] {
				if other == nil || other.Value == nil {
					continue
				}
				for propName, propRef := range other.Value.Properties {
					if s.OneOf[0].Value != nil && s.OneOf[0].Value.Properties != nil {
						if _, inFirst := s.OneOf[0].Value.Properties[propName]; inFirst {
							continue
						}
					}
					if propRef != nil && propRef.Value != nil && propRef.Value.Type != nil &&
						(propRef.Value.Type.Is(openapi3.TypeInteger) || propRef.Value.Type.Is(openapi3.TypeNumber) || propRef.Value.Type.Is(openapi3.TypeBoolean)) {
						obj[propName] = "exclude-branch"
						break
					}
				}
			}
		}
		return val
	}
	if len(s.AnyOf) > 0 {
		return sampleValueForSchema(s.AnyOf[0], depth+1, visited)
	}

	var typ string
	if s.Type != nil {
		slice := s.Type.Slice()
		if len(slice) > 0 {
			typ = slice[0]
		}
	}
	if typ == "" && (len(s.Properties) > 0 || len(s.Required) > 0) {
		typ = openapi3.TypeObject
	}

	switch typ {
	case openapi3.TypeBoolean:
		return true
	case openapi3.TypeInteger:
		v := 1
		if s.Min != nil && float64(v) < *s.Min {
			v = int(*s.Min)
		}
		return v
	case openapi3.TypeNumber:
		v := 1.0
		if s.Min != nil && v < *s.Min {
			v = *s.Min
		}
		return v
	case openapi3.TypeArray:
		count := 1
		if s.MinItems > 1 {
			count = int(s.MinItems)
		}
		if s.Items == nil {
			return []any{"sample"}
		}
		arr := make([]any, count)
		for i := 0; i < count; i++ {
			arr[i] = sampleValueForSchema(s.Items, depth+1, visited)
		}
		return arr
	case openapi3.TypeObject:
		obj := map[string]any{}
		for _, reqName := range s.Required {
			if prop, ok := s.Properties[reqName]; ok {
				obj[reqName] = sampleValueForSchema(prop, depth+1, visited)
			} else {
				obj[reqName] = "sample"
			}
		}
		return obj
	default:
		switch s.Format {
		case "uuid":
			return "11111111-1111-4111-8111-111111111111"
		case "email":
			return "admin@example.com"
		case "uri", "url":
			return "https://example.com"
		case "date":
			return "2026-01-01"
		case "date-time":
			return "2026-01-01T00:00:00Z"
		case "byte":
			return base64.StdEncoding.EncodeToString([]byte("test"))
		}
		if strings.Contains(s.Pattern, "https?") {
			return "https://example.com"
		}
		val := "sample"
		if s.MinLength > uint64(len(val)) {
			val = strings.Repeat("a", int(s.MinLength))
		}
		return val
	}
}
