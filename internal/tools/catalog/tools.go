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

// Package catalog provides the four runtime MCP catalog tools for discovering,
// inspecting, and invoking any WSO2 Identity Server REST API operation.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	iscatalog "github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mark3labs/mcp-go/mcp"
)

const maxSchemaDepth = 4

var pathPlaceholderPattern = regexp.MustCompile(`\{([^}]+)\}`)

type searchResponse struct {
	Total      int                  `json:"total"`
	Operations []searchOperationDTO `json:"operations"`
}

type searchOperationDTO struct {
	ID           string                 `json:"id"`
	Summary      string                 `json:"summary"`
	Method       string                 `json:"method"`
	Path         string                 `json:"path"`
	Kind         iscatalog.Kind         `json:"kind"`
	Organization bool                   `json:"organization"`
	Availability iscatalog.Availability `json:"availability"`
}

type describeResponse struct {
	ID                 string                 `json:"id"`
	Summary            string                 `json:"summary"`
	Description        string                 `json:"description"`
	Method             string                 `json:"method"`
	Path               string                 `json:"path"`
	Kind               iscatalog.Kind         `json:"kind"`
	Organization       bool                   `json:"organization"`
	Scopes             []string               `json:"scopes"`
	Availability       iscatalog.Availability `json:"availability"`
	AvailabilityReason string                 `json:"availability_reason,omitempty"`
	Deprecated         bool                   `json:"deprecated"`
	Parameters         []describeParamDTO     `json:"parameters"`
	Body               *describeBodyDTO       `json:"body,omitempty"`
	CallWith           string                 `json:"call_with"`
	Example            map[string]any         `json:"example"`
}

type describeParamDTO struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Schema   any    `json:"schema,omitempty"`
}

type describeBodyDTO struct {
	ContentType string `json:"content_type"`
	Required    bool   `json:"required"`
	Schema      any    `json:"schema,omitempty"`
}

// Tools constructs the four catalog ToolDefs backed by cat, cl, and cfg.
func Tools(cat *iscatalog.Catalog, cl *client.Client, cfg *config.Config) []toolkit.ToolDef {
	if cfg == nil {
		cfg = &config.Config{}
	}

	return []toolkit.ToolDef{
		{
			Name:        "search_operations",
			Layer:       toolkit.LayerCatalog,
			Kind:        iscatalog.KindRead,
			Description: "Search WSO2 Identity Server REST API operations by keyword, area, kind, or organization scope.",
			Options: []mcp.ToolOption{
				mcp.WithString("query", mcp.Required(), mcp.Description("Search keywords matched against operation id, summary, path, area, and tags.")),
				mcp.WithString("area", mcp.Description("Optional API area filter (for example applications, users, roles, identity_providers).")),
				mcp.WithString("kind", mcp.Enum("read", "write", "destructive"), mcp.Description("Optional operation safety kind filter: read, write, or destructive.")),
				mcp.WithBoolean("organization", mcp.Description("Optional filter: true for sub-organization operations only, false for root tenant operations only.")),
				mcp.WithInteger("limit", mcp.Description("Maximum number of results to return (1-50, default 20).")),
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				return handleSearch(cat, cfg, args)
			},
		},
		{
			Name:        "describe_operation",
			Layer:       toolkit.LayerCatalog,
			Kind:        iscatalog.KindRead,
			Description: "Describe a WSO2 Identity Server REST API operation's parameters, body schema, scopes, availability, and example input.",
			Options: []mcp.ToolOption{
				mcp.WithString("operation_id", mcp.Required(), mcp.Description("Canonical operation id from search_operations.")),
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				return handleDescribe(cat, cfg, args)
			},
		},
		{
			Name:        "call_read_operation",
			Layer:       toolkit.LayerCatalog,
			Kind:        iscatalog.KindRead,
			OrgCapable:  true,
			Description: "Call a read-only WSO2 Identity Server REST API operation by catalog operation_id.",
			Options: []mcp.ToolOption{
				mcp.WithString("operation_id", mcp.Required(), mcp.Description("Canonical operation id from search_operations or describe_operation.")),
				mcp.WithObject("path_params", mcp.Description("Path parameter values keyed by parameter name."), mcp.AdditionalProperties(map[string]any{"type": "string"})),
				mcp.WithObject("query", mcp.Description("Optional query parameter values keyed by parameter name.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				op, pathParams, query, orgID, err := prepareCall(cat, cfg, args, true)
				if err != nil {
					return nil, err
				}
				if cl == nil {
					return nil, errors.New("IS client is not configured")
				}
				if orgID != "" {
					ctx = client.WithOrganizationID(ctx, orgID)
				}
				return cl.Do(ctx, op, pathParams, query, nil)
			},
		},
		{
			Name:        "call_write_operation",
			Layer:       toolkit.LayerCatalog,
			Kind:        iscatalog.KindDestructive,
			OrgCapable:  true,
			Description: "Call a write or destructive WSO2 Identity Server REST API operation by catalog operation_id.",
			SummaryFunc: func(args map[string]any) string {
				return summarizeWriteCall(cat, args)
			},
			Options: []mcp.ToolOption{
				mcp.WithString("operation_id", mcp.Required(), mcp.Description("Canonical operation id from search_operations or describe_operation.")),
				mcp.WithObject("path_params", mcp.Description("Path parameter values keyed by parameter name."), mcp.AdditionalProperties(map[string]any{"type": "string"})),
				mcp.WithObject("query", mcp.Description("Optional query parameter values keyed by parameter name.")),
				mcp.WithAny("body", mcp.Description("Request body payload (JSON object, array, or string) matching the operation schema.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				op, pathParams, query, orgID, err := prepareCall(cat, cfg, args, false)
				if err != nil {
					return nil, err
				}
				if cl == nil {
					return nil, errors.New("IS client is not configured")
				}
				if orgID != "" {
					ctx = client.WithOrganizationID(ctx, orgID)
				}
				return cl.Do(ctx, op, pathParams, query, args["body"])
			},
		},
	}
}

// Register registers the catalog tools into reg.
func Register(reg *toolkit.Registry, cat *iscatalog.Catalog, cl *client.Client, cfg *config.Config) error {
	if reg == nil {
		return errors.New("tool registry is required")
	}
	if cat == nil {
		return errors.New("catalog is required")
	}
	for _, def := range Tools(cat, cl, cfg) {
		if err := reg.Register(def); err != nil {
			return err
		}
	}
	return nil
}

func handleSearch(cat *iscatalog.Catalog, cfg *config.Config, args map[string]any) (any, error) {
	if cat == nil {
		return nil, errors.New("catalog is not configured")
	}

	rawQuery, exists := args["query"]
	if !exists || rawQuery == nil {
		return nil, errors.New("invalid input: query: is required")
	}
	query, ok := rawQuery.(string)
	if !ok {
		return nil, errors.New("invalid input: query: expected string")
	}

	area, err := parseOptionalString(args, "area")
	if err != nil {
		return nil, err
	}

	kindStr, err := parseOptionalString(args, "kind")
	if err != nil {
		return nil, err
	}
	var kind iscatalog.Kind
	if kindStr != "" {
		switch iscatalog.Kind(kindStr) {
		case iscatalog.KindRead, iscatalog.KindWrite, iscatalog.KindDestructive:
			kind = iscatalog.Kind(kindStr)
		default:
			return nil, errors.New("invalid input: kind: must be read, write, or destructive")
		}
	}

	orgFilter, err := parseOptionalBool(args, "organization")
	if err != nil {
		return nil, err
	}

	limit, err := parseOptionalInt(args, "limit")
	if err != nil {
		return nil, err
	}

	matches, total := cat.Search(query, iscatalog.SearchFilter{
		Area:         area,
		Kind:         kind,
		Organization: orgFilter,
		Limit:        limit,
	})

	items := make([]searchOperationDTO, 0, len(matches))
	for _, op := range matches {
		items = append(items, searchOperationDTO{
			ID:           op.ID,
			Summary:      op.Summary,
			Method:       op.Method,
			Path:         op.Path,
			Kind:         op.Kind,
			Organization: op.OrgScoped(),
			Availability: cat.Availability(op, cfg),
		})
	}

	return searchResponse{
		Total:      total,
		Operations: items,
	}, nil
}

func handleDescribe(cat *iscatalog.Catalog, cfg *config.Config, args map[string]any) (any, error) {
	if cat == nil {
		return nil, errors.New("catalog is not configured")
	}
	opID, err := parseRequiredString(args, "operation_id")
	if err != nil {
		return nil, err
	}
	op, ok := cat.Get(opID)
	if !ok {
		return nil, fmt.Errorf("operation %q not found", opID)
	}
	op = normalizeOpForCall(op)

	avail := cat.Availability(op, cfg)
	var availReason string
	if avail != iscatalog.AvailabilityAvailable {
		availReason = iscatalog.RefusalReason(op, avail)
	}

	callWith := "call_write_operation"
	if op.Kind == iscatalog.KindRead {
		callWith = "call_read_operation"
	}

	scopes := make([]string, len(op.Scopes))
	copy(scopes, op.Scopes)

	params := make([]describeParamDTO, 0, len(op.Params))
	for _, p := range op.Params {
		params = append(params, describeParamDTO{
			Name:     p.Name,
			In:       p.In,
			Required: p.Required,
			Schema:   summarizeSchema(p.Schema, 1, map[string]bool{}),
		})
	}

	var bodyDTO *describeBodyDTO
	if op.Body != nil {
		bodyDTO = &describeBodyDTO{
			ContentType: op.Body.ContentType,
			Required:    op.Body.Required,
			Schema:      summarizeSchema(op.Body.Schema, 1, map[string]bool{}),
		}
	}

	return describeResponse{
		ID:                 op.ID,
		Summary:            op.Summary,
		Description:        op.Description,
		Method:             op.Method,
		Path:               op.Path,
		Kind:               op.Kind,
		Organization:       op.OrgScoped(),
		Scopes:             scopes,
		Availability:       avail,
		AvailabilityReason: availReason,
		Deprecated:         op.Deprecated,
		Parameters:         params,
		Body:               bodyDTO,
		CallWith:           callWith,
		Example:            buildExampleInput(op),
	}, nil
}

func prepareCall(cat *iscatalog.Catalog, cfg *config.Config, args map[string]any, expectRead bool) (*iscatalog.Operation, map[string]string, map[string]any, string, error) {
	if cat == nil {
		return nil, nil, nil, "", errors.New("catalog is not configured")
	}

	opID, err := parseRequiredString(args, "operation_id")
	if err != nil {
		return nil, nil, nil, "", err
	}
	op, ok := cat.Get(opID)
	if !ok {
		return nil, nil, nil, "", fmt.Errorf("operation %q not found", opID)
	}
	op = normalizeOpForCall(op)

	avail := cat.Availability(op, cfg)
	if avail != iscatalog.AvailabilityAvailable {
		return nil, nil, nil, "", errors.New(iscatalog.RefusalReason(op, avail))
	}

	if expectRead && op.Kind != iscatalog.KindRead {
		return nil, nil, nil, "", fmt.Errorf("operation %s changes data; use call_write_operation", op.ID)
	}
	if !expectRead && op.Kind == iscatalog.KindRead {
		return nil, nil, nil, "", fmt.Errorf("operation %s is read-only; use call_read_operation", op.ID)
	}

	orgID, err := parseOptionalString(args, "organization_id")
	if err != nil {
		return nil, nil, nil, "", err
	}
	if op.OrgScoped() && orgID == "" {
		return nil, nil, nil, "", fmt.Errorf("operation %s requires organization_id", op.ID)
	}
	if !op.OrgScoped() && orgID != "" {
		return nil, nil, nil, "", fmt.Errorf("operation %s is not available for organizations", op.ID)
	}

	pathParams, err := parseStringMap(args, "path_params")
	if err != nil {
		return nil, nil, nil, "", err
	}
	query, err := parseObjectArg(args, "query")
	if err != nil {
		return nil, nil, nil, "", err
	}

	return op, pathParams, query, orgID, nil
}

func summarizeWriteCall(cat *iscatalog.Catalog, args map[string]any) string {
	var parts []string
	if opID, ok := args["operation_id"].(string); ok {
		opID = strings.TrimSpace(opID)
		if opID != "" {
			if cat != nil {
				if op, exists := cat.Get(opID); exists {
					parts = append(parts, fmt.Sprintf("operation=%s (%s %s)", op.ID, op.Method, op.Path))
				} else {
					parts = append(parts, "operation="+opID)
				}
			} else {
				parts = append(parts, "operation="+opID)
			}
		}
	}

	if orgID, ok := args["organization_id"].(string); ok && strings.TrimSpace(orgID) != "" {
		parts = append(parts, "organization_id="+strings.TrimSpace(orgID))
	}

	if pp, ok := args["path_params"].(map[string]any); ok && len(pp) > 0 {
		keys := sortedMapKeys(pp)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, pp[k]))
		}
	}

	if body, exists := args["body"]; exists && body != nil {
		switch b := body.(type) {
		case map[string]any:
			if len(b) > 0 {
				keys := sortedMapKeys(b)
				parts = append(parts, "body_keys=["+strings.Join(keys, ",")+"]")
			} else {
				parts = append(parts, "body={}")
			}
		case []any:
			parts = append(parts, fmt.Sprintf("body=array(%d)", len(b)))
		default:
			parts = append(parts, fmt.Sprintf("body=%T", body))
		}
	}

	if len(parts) == 0 {
		return "execute catalog write operation"
	}
	return strings.Join(parts, " ")
}

func summarizeSchema(sr *openapi3.SchemaRef, depth int, visited map[string]bool) any {
	if sr == nil {
		return nil
	}

	out := make(map[string]any)
	if sr.Ref != "" {
		out["$ref"] = sr.Ref
	}

	s := sr.Value
	if s == nil {
		if len(out) == 0 {
			return nil
		}
		return out
	}

	if s.Type != nil {
		types := s.Type.Slice()
		if len(types) == 1 {
			out["type"] = types[0]
		} else if len(types) > 1 {
			out["type"] = types
		}
	}
	if s.Format != "" {
		out["format"] = s.Format
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if len(s.Enum) > 0 {
		out["enum"] = s.Enum
	}
	if s.Default != nil {
		out["default"] = s.Default
	}
	if len(s.Required) > 0 {
		out["required"] = s.Required
	}
	if s.Nullable {
		out["nullable"] = true
	}

	hasChildren := len(s.Properties) > 0 || s.Items != nil || len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0 || s.AdditionalProperties.Schema != nil
	if depth >= maxSchemaDepth || (sr.Ref != "" && visited[sr.Ref]) {
		if hasChildren {
			out["truncated"] = true
		}
		return out
	}

	if sr.Ref != "" {
		visited[sr.Ref] = true
		defer delete(visited, sr.Ref)
	}

	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for k, prop := range s.Properties {
			props[k] = summarizeSchema(prop, depth+1, visited)
		}
		out["properties"] = props
	}
	if s.AdditionalProperties.Has != nil {
		out["additionalProperties"] = *s.AdditionalProperties.Has
	} else if s.AdditionalProperties.Schema != nil {
		out["additionalProperties"] = summarizeSchema(s.AdditionalProperties.Schema, depth+1, visited)
	}
	if s.Items != nil {
		out["items"] = summarizeSchema(s.Items, depth+1, visited)
	}
	if len(s.AllOf) > 0 {
		out["allOf"] = summarizeSchemaList(s.AllOf, depth+1, visited)
	}
	if len(s.OneOf) > 0 {
		out["oneOf"] = summarizeSchemaList(s.OneOf, depth+1, visited)
	}
	if len(s.AnyOf) > 0 {
		out["anyOf"] = summarizeSchemaList(s.AnyOf, depth+1, visited)
	}

	return out
}

func summarizeSchemaList(refs openapi3.SchemaRefs, depth int, visited map[string]bool) []any {
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, summarizeSchema(ref, depth, visited))
	}
	return out
}

func buildExampleInput(op *iscatalog.Operation) map[string]any {
	ex := map[string]any{
		"operation_id": op.ID,
	}
	if op.OrgScoped() {
		ex["organization_id"] = "<organization_id>"
	}

	pathParams := make(map[string]string)
	query := make(map[string]any)
	for _, p := range op.Params {
		switch p.In {
		case "path":
			pathParams[p.Name] = "<" + p.Name + ">"
		case "query":
			if p.Required {
				query[p.Name] = exampleValue(p.Schema, 0, map[string]bool{})
			}
		}
	}
	for _, match := range pathPlaceholderPattern.FindAllStringSubmatch(op.Path, -1) {
		name := match[1]
		if _, ok := pathParams[name]; !ok {
			pathParams[name] = "<" + name + ">"
		}
	}
	if len(pathParams) > 0 {
		ex["path_params"] = pathParams
	}
	if len(query) > 0 {
		ex["query"] = query
	}
	if op.Body != nil {
		ex["body"] = exampleValue(op.Body.Schema, 0, map[string]bool{})
	}
	return ex
}

func exampleValue(sr *openapi3.SchemaRef, depth int, visited map[string]bool) any {
	if sr == nil || sr.Value == nil || depth >= maxSchemaDepth {
		return map[string]any{}
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
			if m, ok := exampleValue(sub, depth+1, visited).(map[string]any); ok {
				for k, v := range m {
					merged[k] = v
				}
			}
		}
		if len(merged) > 0 {
			return merged
		}
	}
	if len(s.OneOf) > 0 {
		return exampleValue(s.OneOf[0], depth+1, visited)
	}
	if len(s.AnyOf) > 0 {
		return exampleValue(s.AnyOf[0], depth+1, visited)
	}

	var typ string
	if s.Type != nil {
		types := s.Type.Slice()
		if len(types) > 0 {
			typ = types[0]
		}
	}
	if typ == "" && (len(s.Properties) > 0 || len(s.Required) > 0) {
		typ = openapi3.TypeObject
	}

	switch typ {
	case openapi3.TypeBoolean:
		return false
	case openapi3.TypeInteger:
		return 0
	case openapi3.TypeNumber:
		return 0.0
	case openapi3.TypeArray:
		if s.Items != nil {
			return []any{exampleValue(s.Items, depth+1, visited)}
		}
		return []any{}
	case openapi3.TypeObject:
		obj := map[string]any{}
		for _, reqName := range s.Required {
			if prop, ok := s.Properties[reqName]; ok {
				obj[reqName] = exampleValue(prop, depth+1, visited)
			} else {
				obj[reqName] = "<" + reqName + ">"
			}
		}
		return obj
	default:
		return "<string>"
	}
}

func parseRequiredString(args map[string]any, field string) (string, error) {
	raw, exists := args[field]
	if !exists || raw == nil {
		return "", fmt.Errorf("invalid input: %s: is required", field)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("invalid input: %s: expected string", field)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("invalid input: %s: must not be empty", field)
	}
	return s, nil
}

func parseOptionalString(args map[string]any, field string) (string, error) {
	raw, exists := args[field]
	if !exists || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("invalid input: %s: expected string", field)
	}
	return strings.TrimSpace(s), nil
}

func parseOptionalBool(args map[string]any, field string) (*bool, error) {
	raw, exists := args[field]
	if !exists || raw == nil {
		return nil, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil, fmt.Errorf("invalid input: %s: expected boolean", field)
	}
	return &b, nil
}

func parseOptionalInt(args map[string]any, field string) (int, error) {
	raw, exists := args[field]
	if !exists || raw == nil {
		return 0, nil
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("invalid input: %s: expected integer", field)
		}
		return int(v), nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid input: %s: expected integer", field)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("invalid input: %s: expected integer", field)
	}
}

func parseStringMap(args map[string]any, field string) (map[string]string, error) {
	obj, err := parseObjectArg(args, field)
	if err != nil || obj == nil {
		return nil, err
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid input: %s.%s: expected string", field, k)
		}
		out[k] = s
	}
	return out, nil
}

func parseObjectArg(args map[string]any, field string) (map[string]any, error) {
	raw, exists := args[field]
	if !exists || raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: %s: expected object", field)
	}
	return m, nil
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// normalizeOpForCall returns a shallow copy of op with residual Swagger 2.0
// in:body parameters promoted to Body and urlencoded object schemas typed.
func normalizeOpForCall(op *iscatalog.Operation) *iscatalog.Operation {
	if op == nil {
		return nil
	}

	var hasBodyParam bool
	for _, p := range op.Params {
		if p.In == "body" {
			hasBodyParam = true
			break
		}
	}
	needsFormType := op.Body != nil &&
		op.Body.ContentType == "application/x-www-form-urlencoded" &&
		op.Body.Schema != nil &&
		op.Body.Schema.Value != nil &&
		op.Body.Schema.Value.Type == nil &&
		len(op.Body.Schema.Value.Properties) > 0

	if !hasBodyParam && !needsFormType {
		return op
	}

	copied := *op
	if hasBodyParam {
		params := make([]iscatalog.Param, 0, len(op.Params))
		for _, p := range op.Params {
			if p.In == "body" {
				if copied.Body == nil {
					copied.Body = &iscatalog.BodySpec{
						ContentType: "application/json",
						Schema:      p.Schema,
						Required:    p.Required,
					}
				}
				continue
			}
			params = append(params, p)
		}
		copied.Params = params
	}
	if needsFormType {
		bodyCopy := *copied.Body
		schemaValCopy := *bodyCopy.Schema.Value
		objTypes := openapi3.Types{openapi3.TypeObject}
		schemaValCopy.Type = &objTypes
		bodyCopy.Schema = &openapi3.SchemaRef{
			Ref:   bodyCopy.Schema.Ref,
			Value: &schemaValCopy,
		}
		copied.Body = &bodyCopy
	}
	return &copied
}
