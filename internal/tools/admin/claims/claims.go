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

// Package claims implements list_claims (migrated from the Asgardeo SDK tool) and
// create_local_claim (contracts/admin-curated-tools.md).
package claims

import (
	"context"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tools returns list_claims and create_local_claim.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	list, err := toolkit.LookupPair(cat, "claims.getLocalClaims", "claims.org.getLocalClaims")
	if err != nil {
		return nil, err
	}
	add, err := toolkit.LookupPair(cat, "claims.addLocalClaim", "claims.org.addLocalClaim")
	if err != nil {
		return nil, err
	}

	return []toolkit.ToolDef{
		{
			Name: "list_claims", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "List all local claims (user attributes) in WSO2 Identity Server, hidden claims excluded.",
			Handler: func(ctx context.Context, _ map[string]any) (any, error) {
				// The organization variant has no exclude-hidden-claims parameter.
				return toolkit.CallDeclared(ctx, cl, list, nil, map[string]any{"exclude-hidden-claims": true}, nil)
			},
		},
		{
			Name: "create_local_claim", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: add.Scopes(), OrgCapable: true, Operation: add.Root,
			Description: "Create a local claim (user attribute) in WSO2 Identity Server, mapped to a user store attribute.",
			Options: []mcp.ToolOption{
				mcp.WithString("claim_uri", mcp.Required(), mcp.Description("Claim URI, for example http://wso2.org/claims/tier.")),
				mcp.WithString("display_name", mcp.Required(), mcp.Description("Display name of the claim.")),
				mcp.WithString("description", mcp.Required(), mcp.Description("Description of the claim.")),
				mcp.WithArray("attribute_mapping", mcp.Required(), mcp.Description("User store mappings as {mappedAttribute, userstore} objects, for example {mappedAttribute: tier, userstore: PRIMARY}.")),
				mcp.WithBoolean("supported_by_default", mcp.Description("Show the attribute in user profiles by default.")),
				mcp.WithBoolean("read_only", mcp.Description("Users cannot change the value.")),
				mcp.WithBoolean("required", mcp.Description("The value is required.")),
				mcp.WithString("regex", mcp.Description("Regular expression the value must match.")),
				mcp.WithString("data_type", mcp.Description("Data type, for example string, integer, boolean, date_time.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body := map[string]any{}
				for _, f := range [][2]string{{"claim_uri", "claimURI"}, {"display_name", "displayName"}, {"description", "description"}} {
					v, err := toolkit.RequireString(args, f[0])
					if err != nil {
						return nil, err
					}
					body[f[1]] = v
				}
				mapping, err := toolkit.OptArray(args, "attribute_mapping")
				if err != nil {
					return nil, err
				}
				body["attributeMapping"] = mapping
				toolkit.PutIf(body, args, map[string]string{
					"supported_by_default": "supportedByDefault", "read_only": "readOnly",
					"required": "required", "regex": "regEx", "data_type": "dataType",
				})
				return toolkit.Call(ctx, cl, add, nil, nil, body)
			},
		},
	}, nil
}
