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

// Package organizations implements the curated organizations tools (B2B sub-organizations and
// user sharing, contracts/admin-curated-tools.md).
package organizations

import (
	"context"
	"errors"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tools returns the five organization tools.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"organizations.organizationsGet", "organizations.org.organizationsGet"},
		{"organizations.getOrganizationsByOrganizationId", "organizations.org.getOrganizationsByOrganizationId"},
		{"organizations.organizationPost", "organizations.org.organizationPost"},
		{"organizations.deleteOrganizationsByOrganizationId", "organizations.org.deleteOrganizationsByOrganizationId"},
		{"organizations.shareUsersWithSelected", "organizations.org.shareUsersWithSelected"},
		{"organizations.shareUsersWithAll", "organizations.org.shareUsersWithAll"},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	list, get, create, del, shareSelected, shareAll := ops[0], ops[1], ops[2], ops[3], ops[4], ops[5]
	orgID := mcp.WithString("id", mcp.Required(), mcp.Description("ID of the organization to act on (not the organization_id context input)."))

	return []toolkit.ToolDef{
		{
			Name: "list_organizations", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "List the child organizations (B2B sub-organizations) in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithString("filter", mcp.Description("Filter expression, e.g. name co acme.")),
				mcp.WithNumber("limit", mcp.Description("Maximum number of results.")),
				mcp.WithBoolean("recursive", mcp.Description("Include all descendants, not only direct children.")),
				mcp.WithString("after", mcp.Description("Cursor for the next page.")),
				mcp.WithString("before", mcp.Description("Cursor for the previous page.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				q := map[string]any{}
				for _, k := range []string{"filter", "after", "before"} {
					v, err := toolkit.OptString(args, k)
					if err != nil {
						return nil, err
					}
					if v != "" {
						q[k] = v
					}
				}
				if n, ok, err := toolkit.OptInt(args, "limit"); err != nil {
					return nil, err
				} else if ok {
					q["limit"] = n
				}
				if b, ok, err := toolkit.OptBool(args, "recursive"); err != nil {
					return nil, err
				} else if ok {
					q["recursive"] = b
				}
				return toolkit.Call(ctx, cl, list, nil, q, nil)
			},
		},
		{
			Name: "get_organization", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: get.Scopes(), OrgCapable: true, Operation: get.Root,
			Description: "Get one organization of WSO2 Identity Server by id, with its attributes and parent.",
			Options:     []mcp.ToolOption{orgID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, get, map[string]string{"organization-id": id}, nil, nil)
			},
		},
		{
			Name: "create_organization", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: create.Scopes(), OrgCapable: true, Operation: create.Root,
			Description: "Create a child organization (B2B sub-organization) in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithString("name", mcp.Required(), mcp.Description("Name of the organization.")),
				mcp.WithString("description", mcp.Description("Description of the organization.")),
				mcp.WithString("parent_id", mcp.Description("Parent organization id; defaults to the organization the call runs in.")),
				mcp.WithString("org_handle", mcp.Description("Unique handle of the organization, used in URLs.")),
				mcp.WithArray("attributes", mcp.Description("Attributes as {key, value} objects.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				name, err := toolkit.RequireString(args, "name")
				if err != nil {
					return nil, err
				}
				body := map[string]any{"name": name}
				toolkit.PutIf(body, args, map[string]string{
					"description": "description", "parent_id": "parentId", "org_handle": "orgHandle", "attributes": "attributes",
				})
				return toolkit.Call(ctx, cl, create, nil, nil, body)
			},
		},
		{
			Name: "delete_organization", Layer: toolkit.LayerCurated, Kind: catalog.KindDestructive,
			Scopes: del.Scopes(), OrgCapable: true, Operation: del.Root,
			Description: "Delete an organization from WSO2 Identity Server. IS refuses while it still has child organizations.",
			Options:     []mcp.ToolOption{orgID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, del, map[string]string{"organization-id": id}, nil, nil)
			},
		},
		{
			Name: "share_users", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: toolkit.UnionScopes(shareSelected.Root, shareSelected.Org, shareAll.Root, shareAll.Org), OrgCapable: true,
			Operation: shareSelected.Root,
			Description: "Share users with child organizations in WSO2 Identity Server (user sharing v2). Pass organizations " +
				"to share with selected ones, or policy to share with all organizations.",
			Options: []mcp.ToolOption{
				mcp.WithArray("user_ids", mcp.Required(), mcp.WithStringItems(), mcp.Description("IDs of the users to share.")),
				mcp.WithArray("organizations", mcp.Description("Selected organizations as {orgId, policy, roleAssignment?} objects; policy is SELECTED_ORG_ONLY or SELECTED_ORG_WITH_ALL_EXISTING_AND_FUTURE_CHILDREN (see IS docs for the full list).")),
				mcp.WithString("policy", mcp.Description("Share with all organizations using this policy, e.g. ALL_EXISTING_ORGS_ONLY or ALL_EXISTING_AND_FUTURE_ORGS.")),
				mcp.WithObject("role_assignment", mcp.Description("Roles to give in the shared organizations, as {mode, roles}; used with policy.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				users, err := toolkit.OptStrings(args, "user_ids")
				if err != nil {
					return nil, err
				}
				if len(users) == 0 {
					return nil, errors.New("invalid input: user_ids: is required")
				}
				orgs, err := toolkit.OptArray(args, "organizations")
				if err != nil {
					return nil, err
				}
				policy, err := toolkit.OptString(args, "policy")
				if err != nil {
					return nil, err
				}
				criteria := map[string]any{"userIds": users}
				switch {
				case len(orgs) > 0 && policy != "":
					return nil, errors.New("invalid input: pass organizations or policy, not both")
				case len(orgs) > 0:
					return toolkit.Call(ctx, cl, shareSelected, nil, nil, map[string]any{"userCriteria": criteria, "organizations": orgs})
				case policy != "":
					body := map[string]any{"userCriteria": criteria, "policy": policy}
					toolkit.PutIf(body, args, map[string]string{"role_assignment": "roleAssignment"})
					return toolkit.Call(ctx, cl, shareAll, nil, nil, body)
				default:
					return nil, errors.New("invalid input: organizations or policy: one is required")
				}
			},
		},
	}, nil
}
