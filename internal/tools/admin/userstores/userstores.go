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

// Package userstores implements the curated user_stores tools (contracts/admin-curated-tools.md).
package userstores

import (
	"context"
	"fmt"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tools returns list_user_stores, get_user_store, create_user_store and
// test_user_store_connection.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"user_stores.getSecondaryUserStores", "user_stores.org.getSecondaryUserStores"},
		{"user_stores.getUserStoreByDomainId", "user_stores.org.getUserStoreByDomainId"},
		{"user_stores.addUserStore", "user_stores.org.addUserStore"},
		{"user_stores.getAvailableUserStoreTypes", "user_stores.org.getAvailableUserStoreTypes"},
		{"user_stores.testRDBMSConnection", "user_stores.org.testRDBMSConnection"},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	list, get, create, types, test := ops[0], ops[1], ops[2], ops[3], ops[4]

	return []toolkit.ToolDef{
		{
			Name: "list_user_stores", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "List the secondary user stores configured in WSO2 Identity Server.",
			Handler: func(ctx context.Context, _ map[string]any) (any, error) {
				return toolkit.Call(ctx, cl, list, nil, nil, nil)
			},
		},
		{
			Name: "get_user_store", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: get.Scopes(), OrgCapable: true, Operation: get.Root,
			Description: "Get one user store of WSO2 Identity Server by its domain id, with its properties (secrets are redacted).",
			Options: []mcp.ToolOption{
				mcp.WithString("domain_id", mcp.Required(), mcp.Description("User store domain id, as returned by list_user_stores.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "domain_id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, get, map[string]string{"userstore-domain-id": id}, nil, nil)
			},
		},
		{
			Name: "create_user_store", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes:     toolkit.UnionScopes(create.Root, create.Org, types.Root, types.Org),
			OrgCapable: true, Operation: create.Root,
			Description: "Create a secondary user store in WSO2 Identity Server. type_name is a user store " +
				"manager type such as UniqueIDJDBCUserStoreManager or UniqueIDReadWriteLDAPUserStoreManager.",
			Options: []mcp.ToolOption{
				mcp.WithString("name", mcp.Required(), mcp.Description("Domain name of the user store, for example SEC.")),
				mcp.WithString("type_name", mcp.Required(), mcp.Description("User store manager type name.")),
				mcp.WithString("description", mcp.Description("Description of the user store.")),
				mcp.WithArray("properties", mcp.Required(), mcp.Description("User store properties as {name, value} objects (connection URL, user names, passwords).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				name, err := toolkit.RequireString(args, "name")
				if err != nil {
					return nil, err
				}
				typeName, err := toolkit.RequireString(args, "type_name")
				if err != nil {
					return nil, err
				}
				props, err := toolkit.OptArray(args, "properties")
				if err != nil {
					return nil, err
				}
				typeID, err := resolveTypeID(ctx, cl, types, typeName)
				if err != nil {
					return nil, err
				}
				body := map[string]any{"typeId": typeID, "name": name, "properties": props}
				toolkit.PutIf(body, args, map[string]string{"description": "description"})
				return toolkit.Call(ctx, cl, create, nil, nil, body)
			},
		},
		{
			// POST in the spec, but it saves nothing, so the contract overrides it to read.
			Name: "test_user_store_connection", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: test.Scopes(), OrgCapable: true, Operation: test.Root,
			Description: "Test a JDBC connection for a user store in WSO2 Identity Server without saving anything.",
			Options: []mcp.ToolOption{
				mcp.WithString("driver_name", mcp.Required(), mcp.Description("JDBC driver class, for example com.mysql.cj.jdbc.Driver.")),
				mcp.WithString("connection_url", mcp.Required(), mcp.Description("JDBC connection URL.")),
				mcp.WithString("username", mcp.Required(), mcp.Description("Database user name.")),
				mcp.WithString("connection_password", mcp.Required(), mcp.Description("Database password. It is sent to IS and never returned.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body := map[string]any{}
				for _, f := range [][2]string{
					{"driver_name", "driverName"}, {"connection_url", "connectionURL"},
					{"username", "username"}, {"connection_password", "connectionPassword"},
				} {
					v, err := toolkit.RequireString(args, f[0])
					if err != nil {
						return nil, err
					}
					body[f[1]] = v
				}
				return toolkit.Call(ctx, cl, test, nil, nil, body)
			},
		},
	}, nil
}

// resolveTypeID maps a user store manager type name to its typeId through
// getAvailableUserStoreTypes.
func resolveTypeID(ctx context.Context, cl *client.Client, types toolkit.OpPair, typeName string) (string, error) {
	resp, err := toolkit.Call(ctx, cl, types, nil, nil, nil)
	if err != nil {
		return "", err
	}
	list, _ := resp.([]any)
	names := make([]string, 0, len(list))
	for _, it := range list {
		m, _ := it.(map[string]any)
		name, _ := m["typeName"].(string)
		if id, _ := m["typeId"].(string); name == typeName && id != "" {
			return id, nil
		}
		names = append(names, name)
	}
	return "", fmt.Errorf("invalid input: type_name: unknown user store type %q; available: %v", typeName, names)
}
