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

// Package apiauth implements the api_authorization tools: the four API resource tools migrated
// from the Asgardeo SDK (names and inputs frozen, FR-009) and five new ones
// (contracts/admin-curated-tools.md).
package apiauth

import (
	"context"
	"errors"
	"fmt"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tools returns the nine api_authorization tools.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"api_authorization.getAPIResources", "api_authorization.org.getAPIResources"},
		{"api_authorization.addAPIResource", ""},
		{"api_authorization.patchApiResourcesByApiresourceid", ""},
		{"api_authorization.getApiResourcesByApiresourceidScopes", "api_authorization.org.getApiResourcesByApiresourceidScopes"},
		{"api_authorization.putApiResourcesByApiresourceidScopes", ""},
		{"api_authorization.deleteApiResourcesByApiresourceid", ""},
		{"api_authorization.getApiServerV1OidcScopes", ""},
		{"api_authorization.addScope", ""},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	list, add, patch, getScopes, putScopes, del, oidcList, oidcAdd := ops[0], ops[1], ops[2], ops[3], ops[4], ops[5], ops[6], ops[7]

	search := func(ctx context.Context, q map[string]any) ([]any, error) {
		resp, err := toolkit.Call(ctx, cl, list, nil, q, nil)
		if err != nil {
			return nil, err
		}
		m, _ := resp.(map[string]any)
		items, _ := m["APIResources"].([]any)
		out := make([]any, 0, len(items))
		for _, it := range items {
			out = append(out, summarize(it))
		}
		return out, nil
	}
	resourceID := mcp.WithString("id", mcp.Required(), mcp.Description("API resource id."))

	return []toolkit.ToolDef{
		{
			Name: "list_api_resources", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "List API resources registered in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithString("filter", mcp.Description("Filter expression, e.g. name eq Payments API or identifier eq payments_api. Supports sw, co, ew and eq.")),
				mcp.WithString("before", mcp.Description("Base64 encoded cursor value for backward pagination.")),
				mcp.WithString("after", mcp.Description("Base64 encoded cursor value for forward pagination.")),
				mcp.WithNumber("limit", mcp.Description("Maximum number of results to return; 100 or less is recommended.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				q := map[string]any{}
				for _, name := range []string{"filter", "before", "after"} {
					v, err := toolkit.OptString(args, name)
					if err != nil {
						return nil, err
					}
					if v != "" {
						q[name] = v
					}
				}
				if n, ok, err := toolkit.OptInt(args, "limit"); err != nil {
					return nil, err
				} else if ok {
					q["limit"] = n
				}
				return search(ctx, q)
			},
		},
		{
			Name: "search_api_resources_by_name", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "Search API resources by name in WSO2 Identity Server.",
			Options:     []mcp.ToolOption{mcp.WithString("name", mcp.Required(), mcp.Description("Name of the API resource."))},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				name, err := toolkit.RequireString(args, "name")
				if err != nil {
					return nil, err
				}
				return search(ctx, map[string]any{"filter": "name eq " + name})
			},
		},
		{
			Name: "get_api_resource_by_identifier", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "Get an API resource of WSO2 Identity Server by its identifier.",
			Options:     []mcp.ToolOption{mcp.WithString("identifier", mcp.Required(), mcp.Description("Identifier of the API resource."))},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				identifier, err := toolkit.RequireString(args, "identifier")
				if err != nil {
					return nil, err
				}
				found, err := search(ctx, map[string]any{"filter": "identifier eq " + identifier})
				if err != nil {
					return nil, err
				}
				if len(found) == 0 {
					return nil, fmt.Errorf("API resource with identifier %q not found", identifier)
				}
				return found[0], nil
			},
		},
		{
			Name: "create_api_resource", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: add.Scopes(), Operation: add.Root,
			Description: "Create an API resource with its scopes in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithString("identifier", mcp.Required(), mcp.Description("Identifier (audience) of the API resource.")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Name of the API resource.")),
				mcp.WithBoolean("requiresAuthorization", mcp.Required(), mcp.DefaultBool(true), mcp.Description("Whether the API resource requires authorization.")),
				mcp.WithArray("scopes", mcp.Required(), mcp.Description(`Scopes as names or {"name", "displayName", "description"} objects, e.g. ["orders:read", {"name": "orders:write", "displayName": "Write orders"}].`)),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body := map[string]any{}
				for _, f := range []string{"identifier", "name"} {
					v, err := toolkit.RequireString(args, f)
					if err != nil {
						return nil, err
					}
					body[f] = v
				}
				requires, ok, err := toolkit.OptBool(args, "requiresAuthorization")
				if err != nil {
					return nil, err
				}
				if !ok {
					requires = true
				}
				body["requiresAuthorization"] = requires
				scopes, err := scopeList(args, "scopes")
				if err != nil {
					return nil, err
				}
				body["scopes"] = scopes
				return toolkit.Call(ctx, cl, add, nil, nil, body)
			},
		},
		{
			Name: "update_api_resource", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: patch.Scopes(), Operation: patch.Root,
			Description: "Update the name or description of an API resource in WSO2 Identity Server, and add or remove scopes.",
			Options: []mcp.ToolOption{resourceID,
				mcp.WithString("name", mcp.Description("New name.")),
				mcp.WithString("description", mcp.Description("New description.")),
				mcp.WithArray("added_scopes", mcp.Description("Scopes to add, as names or {name, displayName, description} objects.")),
				mcp.WithArray("removed_scopes", mcp.WithStringItems(), mcp.Description("Names of scopes to remove.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "id")
				if err != nil {
					return nil, err
				}
				body := map[string]any{}
				toolkit.PutIf(body, args, map[string]string{"name": "name", "description": "description"})
				if _, ok := args["added_scopes"]; ok {
					if body["addedScopes"], err = scopeList(args, "added_scopes"); err != nil {
						return nil, err
					}
				}
				removed, err := toolkit.OptStrings(args, "removed_scopes")
				if err != nil {
					return nil, err
				}
				if len(removed) > 0 {
					body["removedScopes"] = removed
				}
				if len(body) == 0 {
					return nil, errors.New("invalid input: nothing to update: pass name, description, added_scopes or removed_scopes")
				}
				return toolkit.Call(ctx, cl, patch, map[string]string{"apiResourceId": id}, nil, body)
			},
		},
		{
			Name: "add_api_resource_scopes", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: toolkit.UnionScopes(getScopes.Root, putScopes.Root), Operation: putScopes.Root,
			Description: "Add scopes to an API resource in WSO2 Identity Server, keeping its existing scopes. A scope with an existing name replaces it.",
			Options: []mcp.ToolOption{resourceID,
				mcp.WithArray("scopes", mcp.Required(), mcp.Description("Scopes to add, as names or {name, displayName, description} objects.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "id")
				if err != nil {
					return nil, err
				}
				added, err := scopeList(args, "scopes")
				if err != nil {
					return nil, err
				}
				path := map[string]string{"apiResourceId": id}
				resp, err := toolkit.Call(ctx, cl, getScopes, path, nil, nil)
				if err != nil {
					return nil, err
				}
				existing, _ := resp.([]any)
				return toolkit.Call(ctx, cl, putScopes, path, nil, mergeScopes(existing, added))
			},
		},
		{
			Name: "delete_api_resource", Layer: toolkit.LayerCurated, Kind: catalog.KindDestructive,
			Scopes: del.Scopes(), Operation: del.Root,
			Description: "Delete an API resource and its scopes from WSO2 Identity Server. Applications lose their authorization for it.",
			Options:     []mcp.ToolOption{resourceID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, del, map[string]string{"apiResourceId": id}, nil, nil)
			},
		},
		{
			Name: "list_oidc_scopes", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: oidcList.Scopes(), Operation: oidcList.Root,
			Description: "List the OpenID Connect scopes of WSO2 Identity Server and the claims each one returns.",
			Handler: func(ctx context.Context, _ map[string]any) (any, error) {
				return toolkit.Call(ctx, cl, oidcList, nil, nil, nil)
			},
		},
		{
			Name: "create_oidc_scope", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: oidcAdd.Scopes(), Operation: oidcAdd.Root,
			Description: "Create an OpenID Connect scope in WSO2 Identity Server that returns the given claims.",
			Options: []mcp.ToolOption{
				mcp.WithString("name", mcp.Required(), mcp.Description("Scope name.")),
				mcp.WithString("display_name", mcp.Required(), mcp.Description("Display name.")),
				mcp.WithString("description", mcp.Description("Description.")),
				mcp.WithArray("claims", mcp.Required(), mcp.WithStringItems(), mcp.Description("OIDC claim names the scope returns, e.g. email, given_name.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				name, err := toolkit.RequireString(args, "name")
				if err != nil {
					return nil, err
				}
				display, err := toolkit.RequireString(args, "display_name")
				if err != nil {
					return nil, err
				}
				claims, err := toolkit.OptStrings(args, "claims")
				if err != nil {
					return nil, err
				}
				body := map[string]any{"name": name, "displayName": display, "claims": claims}
				toolkit.PutIf(body, args, map[string]string{"description": "description"})
				return toolkit.Call(ctx, cl, oidcAdd, nil, nil, body)
			},
		},
	}, nil
}

// summarize keeps the API resource fields the legacy tools returned.
func summarize(v any) map[string]any {
	m, _ := v.(map[string]any)
	out := map[string]any{"id": m["id"], "name": m["name"]}
	for _, k := range []string{"identifier", "type", "requiresAuthorization"} {
		if val, ok := m[k]; ok {
			out[k] = val
		}
	}
	return out
}

// scopeList reads scopes given as names or {name, displayName, description} objects.
func scopeList(args map[string]any, field string) ([]any, error) {
	items, err := toolkit.OptArray(args, field)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(items))
	for i, it := range items {
		switch v := it.(type) {
		case string:
			out = append(out, map[string]any{"name": v, "displayName": v})
		case map[string]any:
			name, _ := v["name"].(string)
			if name == "" {
				return nil, fmt.Errorf("invalid input: %s[%d].name: is required", field, i)
			}
			s := map[string]any{"name": name, "displayName": name}
			for _, k := range []string{"displayName", "description"} {
				if str, ok := v[k].(string); ok && str != "" {
					s[k] = str
				}
			}
			out = append(out, s)
		default:
			return nil, fmt.Errorf("invalid input: %s[%d]: must be a string or an object", field, i)
		}
	}
	return out, nil
}

// mergeScopes keeps existing scopes in order and appends or replaces by name.
func mergeScopes(existing, added []any) []any {
	out := make([]any, 0, len(existing)+len(added))
	index := map[string]int{}
	for _, e := range append(append([]any{}, existing...), added...) {
		m, _ := e.(map[string]any)
		name, _ := m["name"].(string)
		s := map[string]any{"name": name}
		for _, k := range []string{"displayName", "description"} {
			if v, ok := m[k]; ok {
				s[k] = v
			}
		}
		if i, ok := index[name]; ok {
			out[i] = s
			continue
		}
		index[name] = len(out)
		out = append(out, s)
	}
	return out
}
