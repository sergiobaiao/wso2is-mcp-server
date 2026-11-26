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

// Package sessions implements the curated user_lifecycle session tools
// (contracts/admin-curated-tools.md).
package sessions

import (
	"context"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tools returns list_user_sessions and terminate_user_sessions.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	list, err := toolkit.LookupPair(cat, "user_lifecycle.getSessionsByUserId", "")
	if err != nil {
		return nil, err
	}
	terminate, err := toolkit.LookupPair(cat, "user_lifecycle.terminateSessionsByUserId", "")
	if err != nil {
		return nil, err
	}
	userID := mcp.WithString("user_id", mcp.Required(), mcp.Description("ID of the user."))

	return []toolkit.ToolDef{
		{
			Name: "list_user_sessions", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), Operation: list.Root,
			Description: "List the active login sessions of a user in WSO2 Identity Server.",
			Options: []mcp.ToolOption{userID,
				mcp.WithNumber("limit", mcp.Description("Maximum number of sessions to return.")),
				mcp.WithNumber("offset", mcp.Description("Number of sessions to skip.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "user_id")
				if err != nil {
					return nil, err
				}
				q, err := toolkit.Paging(args)
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, list, map[string]string{"user-id": id}, q, nil)
			},
		},
		{
			Name: "terminate_user_sessions", Layer: toolkit.LayerCurated, Kind: catalog.KindDestructive,
			Scopes: terminate.Scopes(), Operation: terminate.Root,
			Description: "Terminate all active login sessions of a user in WSO2 Identity Server, signing the user out everywhere.",
			Options:     []mcp.ToolOption{userID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "user_id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, terminate, map[string]string{"user-id": id}, nil, nil)
			},
		},
	}, nil
}
