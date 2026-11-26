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

package scim

import (
	"context"

	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const agentSchemaURN = "urn:scim:wso2:agent:schema"

func agentTools(ops *opSet, cl *client.Client) []toolkit.ToolDef {
	return []toolkit.ToolDef{
		{
			Name:        "list_agents",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.listAgents),
			OrgCapable:  false,
			Operation:   ops.listAgents,
			Description: "List AI agents in WSO2 Identity Server (root organization only).",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithInteger("count", mcp.Description("Maximum number of agents to return (default 100).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if err := rejectOrgID(ctx, ops.listAgents, args); err != nil {
					return nil, err
				}
				count, err := optionalCount(args, defaultListCount)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(ctx, cl, ops.listAgents, nil, map[string]any{
					"startIndex": 1,
					"count":      count,
				}, nil)
				if err != nil {
					return nil, err
				}
				return listOutput(resp, "agents"), nil
			},
		},
		{
			Name:        "create_agent",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.createAgent),
			OrgCapable:  false,
			Operation:   ops.createAgent,
			Description: "Create an AI agent identity in WSO2 Identity Server (root organization only).",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("displayName", mcp.Required(), mcp.Description("Display name of the AI agent.")),
				mcp.WithString("description", mcp.Description("Description of the AI agent.")),
				mcp.WithString("owner", mcp.Description("Optional owner identifier for the AI agent.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if err := rejectOrgID(ctx, ops.createAgent, args); err != nil {
					return nil, err
				}
				displayName, err := requiredString(args, "displayName")
				if err != nil {
					return nil, err
				}
				description, err := optionalString(args, "description")
				if err != nil {
					return nil, err
				}
				owner, err := optionalString(args, "owner")
				if err != nil {
					return nil, err
				}

				agentSchema := map[string]any{
					"DisplayName": displayName,
				}
				if description != "" {
					agentSchema["Description"] = description
				}
				if owner != "" {
					agentSchema["Owner"] = owner
				}

				body := map[string]any{
					agentSchemaURN: agentSchema,
				}
				resp, err := doOpBodylessValidation(ctx, cl, ops.createAgent, nil, nil, body)
				if err != nil {
					return nil, err
				}
				m := asMap(resp)
				out := map[string]any{
					"id":       m["id"],
					"userName": m["userName"],
				}
				if pwd, ok := m["password"]; ok && pwd != nil {
					out["password"] = pwd
				}
				return out, nil
			},
		},
		{
			Name:        "delete_agent",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.deleteAgent),
			OrgCapable:  false,
			Operation:   ops.deleteAgent,
			Description: "Delete an AI agent by ID from WSO2 Identity Server (root organization only).",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("agentId", mcp.Required(), mcp.Description("Unique ID of the AI agent to delete.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if err := rejectOrgID(ctx, ops.deleteAgent, args); err != nil {
					return nil, err
				}
				agentID, err := requiredString(args, "agentId")
				if err != nil {
					return nil, err
				}
				if _, err := doOp(ctx, cl, ops.deleteAgent, map[string]string{"agentId": agentID}, nil, nil); err != nil {
					return nil, err
				}
				return map[string]any{"status": "deleted"}, nil
			},
		},
		{
			Name:        "get_agent",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.getAgent),
			OrgCapable:  false,
			Operation:   ops.getAgent,
			Description: "Get an AI agent by ID from WSO2 Identity Server (root organization only).",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("agentId", mcp.Required(), mcp.Description("Unique ID of the AI agent.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if err := rejectOrgID(ctx, ops.getAgent, args); err != nil {
					return nil, err
				}
				agentID, err := requiredString(args, "agentId")
				if err != nil {
					return nil, err
				}
				resp, err := doOp(ctx, cl, ops.getAgent, map[string]string{"agentId": agentID}, nil, nil)
				if err != nil {
					return nil, err
				}
				return map[string]any{"agent": asMap(resp)}, nil
			},
		},
	}
}
