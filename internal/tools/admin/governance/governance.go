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

// Package governance implements the curated governance_security tools
// (contracts/admin-curated-tools.md).
package governance

import (
	"context"
	"errors"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tools returns the governance connector and validation rule tools.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"governance_security.getCategories", "governance_security.org.getCategories"},
		{"governance_security.getConnectorsOfCategory", "governance_security.org.getConnectorsOfCategory"},
		{"governance_security.getConnector", "governance_security.org.getConnector"},
		{"governance_security.patchConnector", "governance_security.org.patchConnector"},
		{"governance_security.updateValidationRules", "governance_security.org.updateValidationRules"},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	categories, ofCategory, get, patch, rules := ops[0], ops[1], ops[2], ops[3], ops[4]

	connectorPath := func(args map[string]any) (map[string]string, error) {
		category, err := toolkit.RequireString(args, "category_id")
		if err != nil {
			return nil, err
		}
		connector, err := toolkit.RequireString(args, "connector_id")
		if err != nil {
			return nil, err
		}
		return map[string]string{"category-id": category, "connector-id": connector}, nil
	}
	categoryID := mcp.WithString("category_id", mcp.Required(), mcp.Description("Connector category id, as returned by list_governance_connectors."))
	connectorID := mcp.WithString("connector_id", mcp.Required(), mcp.Description("Connector id, as returned by list_governance_connectors."))

	return []toolkit.ToolDef{
		{
			Name: "list_governance_connectors", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes:     toolkit.UnionScopes(categories.Root, categories.Org, ofCategory.Root, ofCategory.Org),
			OrgCapable: true, Operation: categories.Root,
			Description: "List the identity governance connector categories of WSO2 Identity Server (password " +
				"policies, account locking, self registration, account recovery and others) with their " +
				"connectors. Pass category_id to list one category.",
			Options: []mcp.ToolOption{
				mcp.WithString("category_id", mcp.Description("Only list the connectors of this category.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.OptString(args, "category_id")
				if err != nil {
					return nil, err
				}
				if id == "" {
					return toolkit.Call(ctx, cl, categories, nil, nil, nil)
				}
				return toolkit.Call(ctx, cl, ofCategory, map[string]string{"category-id": id}, nil, nil)
			},
		},
		{
			Name: "get_governance_connector", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: get.Scopes(), OrgCapable: true, Operation: get.Root,
			Description: "Get one identity governance connector of WSO2 Identity Server with its properties.",
			Options:     []mcp.ToolOption{categoryID, connectorID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				path, err := connectorPath(args)
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, get, path, nil, nil)
			},
		},
		{
			Name: "update_governance_connector", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: patch.Scopes(), OrgCapable: true, Operation: patch.Root,
			Description: "Update properties of an identity governance connector of WSO2 Identity Server, for " +
				"example enable account locking or change the password policy.",
			Options: []mcp.ToolOption{categoryID, connectorID,
				mcp.WithArray("properties", mcp.Required(), mcp.Description("Properties to set, as {name, value} objects with string values.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				path, err := connectorPath(args)
				if err != nil {
					return nil, err
				}
				props, err := toolkit.OptArray(args, "properties")
				if err != nil {
					return nil, err
				}
				if len(props) == 0 {
					return nil, errors.New("invalid input: properties: is required")
				}
				return toolkit.Call(ctx, cl, patch, path, nil, map[string]any{"operation": "UPDATE", "properties": props})
			},
		},
		{
			Name: "update_validation_rules", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: rules.Scopes(), OrgCapable: true, Operation: rules.Root,
			Description: "Replace the input validation rules of WSO2 Identity Server for fields such as password and username.",
			Options: []mcp.ToolOption{
				mcp.WithArray("rules", mcp.Required(), mcp.Description("Validation configurations: {field, rules: [{validator, properties: [{key, value}]}]} or {field, regEx: [...]}.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				list, err := toolkit.OptArray(args, "rules")
				if err != nil {
					return nil, err
				}
				if len(list) == 0 {
					return nil, errors.New("invalid input: rules: is required")
				}
				return toolkit.Call(ctx, cl, rules, nil, nil, list)
			},
		},
	}, nil
}
