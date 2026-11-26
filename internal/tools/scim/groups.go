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
	"fmt"

	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func groupTools(ops *opSet, cl *client.Client) []toolkit.ToolDef {
	return []toolkit.ToolDef{
		{
			Name:        "list_groups",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.listGroups.root, ops.listGroups.org),
			OrgCapable:  true,
			Operation:   ops.listGroups.root,
			Description: "List groups in WSO2 Identity Server with display names and members.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithInteger("count", mcp.Description("Maximum number of groups to return (default 100).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				count, err := optionalCount(args, defaultListCount)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.listGroups.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(callCtx, cl, op, nil, map[string]any{
					"attributes": "displayName,members",
					"startIndex": 1,
					"count":      count,
				}, nil)
				if err != nil {
					return nil, err
				}
				return listOutput(resp, "groups"), nil
			},
		},
		{
			Name:        "create_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.createGroup.root, ops.createGroup.org),
			OrgCapable:  true,
			Operation:   ops.createGroup.root,
			Description: "Create a new group in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("displayName", mcp.Required(), mcp.Description("Display name of the group.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				displayName, err := requiredString(args, "displayName")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.createGroup.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":     []string{coreGroupSchema},
					"displayName": displayName,
				}
				resp, err := doOp(callCtx, cl, op, nil, nil, body)
				if err != nil {
					return nil, err
				}
				m := asMap(resp)
				return map[string]any{
					"id":          m["id"],
					"displayName": m["displayName"],
				}, nil
			},
		},
		{
			Name:        "add_user_to_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.getUser.root, ops.getUser.org, ops.patchGroup.root, ops.patchGroup.org),
			OrgCapable:  true,
			Operation:   ops.patchGroup.root,
			Description: "Add an existing user to a group in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group.")),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user to add.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				groupID, err := requiredString(args, "groupId")
				if err != nil {
					return nil, err
				}
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}

				getUserOp, callCtx, err := ops.getUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				userResp, err := doOp(callCtx, cl, getUserOp, map[string]string{"id": userID}, map[string]any{
					"attributes": "userName",
				}, nil)
				if err != nil {
					return nil, err
				}
				userName, _ := asMap(userResp)["userName"].(string)

				patchGroupOp, _, err := ops.patchGroup.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				member := map[string]any{"value": userID}
				if userName != "" {
					member["display"] = userName
				}
				body := map[string]any{
					"schemas": []string{patchOpSchema},
					"Operations": []map[string]any{
						{
							"op": "add",
							"value": map[string]any{
								"members": []map[string]any{member},
							},
						},
					},
				}
				groupResp, err := doOp(callCtx, cl, patchGroupOp, map[string]string{"id": groupID}, nil, body)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"group": asMap(groupResp),
					"addedUser": map[string]any{
						"id":       userID,
						"userName": userName,
					},
				}, nil
			},
		},
		{
			Name:        "remove_user_from_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.patchGroup.root, ops.patchGroup.org),
			OrgCapable:  true,
			Operation:   ops.patchGroup.root,
			Description: "Remove a user from a group in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group.")),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user to remove.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				groupID, err := requiredString(args, "groupId")
				if err != nil {
					return nil, err
				}
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}
				if err := validateSCIMFilterValue("userId", userID); err != nil {
					return nil, err
				}
				op, callCtx, err := ops.patchGroup.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas": []string{patchOpSchema},
					"Operations": []map[string]any{
						{
							"op":   "remove",
							"path": fmt.Sprintf(`members[value eq "%s"]`, userID),
						},
					},
				}
				resp, err := doOp(callCtx, cl, op, map[string]string{"id": groupID}, nil, body)
				if err != nil {
					return nil, err
				}
				return map[string]any{"group": asMap(resp)}, nil
			},
		},
		{
			Name:        "delete_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.deleteGroup.root, ops.deleteGroup.org),
			OrgCapable:  true,
			Operation:   ops.deleteGroup.root,
			Description: "Delete a group by ID from WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group to delete.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				groupID, err := requiredString(args, "groupId")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.deleteGroup.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				if _, err := doOp(callCtx, cl, op, map[string]string{"id": groupID}, nil, nil); err != nil {
					return nil, err
				}
				return map[string]any{"status": "deleted"}, nil
			},
		},
		{
			Name:        "get_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.getGroup.root, ops.getGroup.org),
			OrgCapable:  true,
			Operation:   ops.getGroup.root,
			Description: "Get a group by ID with its members in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				groupID, err := requiredString(args, "groupId")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.getGroup.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(callCtx, cl, op, map[string]string{"id": groupID}, nil, nil)
				if err != nil {
					return nil, err
				}
				return map[string]any{"group": asMap(resp)}, nil
			},
		},
		{
			Name:        "update_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchGroup.root, ops.patchGroup.org),
			OrgCapable:  true,
			Operation:   ops.patchGroup.root,
			Description: "Rename a group in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group to rename.")),
				mcp.WithString("displayName", mcp.Required(), mcp.Description("New display name for the group.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				groupID, err := requiredString(args, "groupId")
				if err != nil {
					return nil, err
				}
				displayName, err := requiredString(args, "displayName")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.patchGroup.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas": []string{patchOpSchema},
					"Operations": []map[string]any{
						{
							"op": "replace",
							"value": map[string]any{
								"displayName": displayName,
							},
						},
					},
				}
				resp, err := doOp(callCtx, cl, op, map[string]string{"id": groupID}, nil, body)
				if err != nil {
					return nil, err
				}
				return map[string]any{"group": asMap(resp)}, nil
			},
		},
	}
}
