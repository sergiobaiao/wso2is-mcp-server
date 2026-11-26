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
	"errors"
	"fmt"

	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func roleTools(ops *opSet, cl *client.Client) []toolkit.ToolDef {
	return []toolkit.ToolDef{
		{
			Name:        "list_roles",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.listRoles.root, ops.listRoles.org),
			OrgCapable:  true,
			Operation:   ops.listRoles.root,
			Description: "List Roles v2 in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithInteger("count", mcp.Description("Maximum number of roles to return (default 100).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				count, err := optionalCount(args, defaultListCount)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.listRoles.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(callCtx, cl, op, nil, map[string]any{
					"startIndex": 1,
					"count":      count,
				}, nil)
				if err != nil {
					return nil, err
				}
				return listOutput(resp, "roles"), nil
			},
		},
		{
			Name:        "create_role",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.createRole.root, ops.createRole.org),
			OrgCapable:  true,
			Operation:   ops.createRole.root,
			Description: "Create a new organization-audience role in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("displayName", mcp.Required(), mcp.Description("Display name of the role.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				displayName, err := requiredString(args, "displayName")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.createRole.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":     []string{roleV2Schema},
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
			Name:        "assign_role_to_user",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchRole.root, ops.patchRole.org),
			OrgCapable:  true,
			Operation:   ops.patchRole.root,
			Description: "Assign a role to a user via SCIM Roles v2 PATCH in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role.")),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				return patchRoleMember(ctx, ops, cl, args, "add", "users", "userId")
			},
		},
		{
			Name:        "remove_role_from_user",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.patchRole.root, ops.patchRole.org),
			OrgCapable:  true,
			Operation:   ops.patchRole.root,
			Description: "Remove a role from a user via SCIM Roles v2 PATCH in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role.")),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				return patchRoleMember(ctx, ops, cl, args, "remove", "users", "userId")
			},
		},
		{
			Name:        "delete_role",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.deleteRole.root, ops.deleteRole.org),
			OrgCapable:  true,
			Operation:   ops.deleteRole.root,
			Description: "Delete a role by ID from WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role to delete.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				roleID, err := requiredString(args, "roleId")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.deleteRole.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				if _, err := doOp(callCtx, cl, op, map[string]string{"id": roleID}, nil, nil); err != nil {
					return nil, err
				}
				return map[string]any{"status": "deleted"}, nil
			},
		},
		{
			Name:        "get_role",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.getRole.root, ops.getRole.org),
			OrgCapable:  true,
			Operation:   ops.getRole.root,
			Description: "Get a role by ID including its users, groups, permissions, and audience in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				roleID, err := requiredString(args, "roleId")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.getRole.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(callCtx, cl, op, map[string]string{"id": roleID}, nil, nil)
				if err != nil {
					return nil, err
				}
				return map[string]any{"role": asMap(resp)}, nil
			},
		},
		{
			Name:        "assign_role_to_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchRole.root, ops.patchRole.org),
			OrgCapable:  true,
			Operation:   ops.patchRole.root,
			Description: "Assign a role to a group via SCIM Roles v2 PATCH in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role.")),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				return patchRoleMember(ctx, ops, cl, args, "add", "groups", "groupId")
			},
		},
		{
			Name:        "remove_role_from_group",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.patchRole.root, ops.patchRole.org),
			OrgCapable:  true,
			Operation:   ops.patchRole.root,
			Description: "Remove a role from a group via SCIM Roles v2 PATCH in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role.")),
				mcp.WithString("groupId", mcp.Required(), mcp.Description("Unique ID of the group.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				return patchRoleMember(ctx, ops, cl, args, "remove", "groups", "groupId")
			},
		},
		{
			Name:        "update_role_permissions",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchRole.root, ops.patchRole.org),
			OrgCapable:  true,
			Operation:   ops.patchRole.root,
			Description: "Add or remove API scopes (permissions) on a role in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("roleId", mcp.Required(), mcp.Description("Unique ID of the role.")),
				mcp.WithArray("addPermissions", mcp.WithStringItems(), mcp.Description("API scope names to add to the role.")),
				mcp.WithArray("removePermissions", mcp.WithStringItems(), mcp.Description("API scope names to remove from the role.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				roleID, err := requiredString(args, "roleId")
				if err != nil {
					return nil, err
				}
				addPerms, err := optionalStringSlice(args, "addPermissions")
				if err != nil {
					return nil, err
				}
				remPerms, err := optionalStringSlice(args, "removePermissions")
				if err != nil {
					return nil, err
				}
				if len(addPerms) == 0 && len(remPerms) == 0 {
					return nil, errors.New("invalid input: addPermissions or removePermissions must contain at least one scope")
				}

				var patchOps []map[string]any
				if len(addPerms) > 0 {
					vals := make([]map[string]any, 0, len(addPerms))
					for _, p := range addPerms {
						vals = append(vals, map[string]any{"value": p})
					}
					patchOps = append(patchOps, map[string]any{
						"op":    "add",
						"path":  "permissions",
						"value": vals,
					})
				}
				for _, p := range remPerms {
					if err := validateSCIMFilterValue("removePermissions", p); err != nil {
						return nil, err
					}
					patchOps = append(patchOps, map[string]any{
						"op":   "remove",
						"path": fmt.Sprintf(`permissions[value eq "%s"]`, p),
					})
				}

				op, callCtx, err := ops.patchRole.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":    []string{patchOpSchema},
					"Operations": patchOps,
				}
				resp, err := doOp(callCtx, cl, op, map[string]string{"id": roleID}, nil, body)
				if err != nil {
					return nil, err
				}
				return map[string]any{"role": asMap(resp)}, nil
			},
		},
	}
}

func patchRoleMember(ctx context.Context, ops *opSet, cl *client.Client, args map[string]any, action, pathAttr, idField string) (any, error) {
	roleID, err := requiredString(args, "roleId")
	if err != nil {
		return nil, err
	}
	targetID, err := requiredString(args, idField)
	if err != nil {
		return nil, err
	}
	if action == "remove" {
		if err := validateSCIMFilterValue(idField, targetID); err != nil {
			return nil, err
		}
	}

	op, callCtx, err := ops.patchRole.selectOp(ctx, args)
	if err != nil {
		return nil, err
	}

	var patchOp map[string]any
	if action == "add" {
		patchOp = map[string]any{
			"op":    "add",
			"path":  pathAttr,
			"value": []map[string]any{{"value": targetID}},
		}
	} else {
		patchOp = map[string]any{
			"op":   "remove",
			"path": fmt.Sprintf(`%s[value eq "%s"]`, pathAttr, targetID),
		}
	}

	body := map[string]any{
		"schemas":    []string{patchOpSchema},
		"Operations": []map[string]any{patchOp},
	}
	resp, err := doOp(callCtx, cl, op, map[string]string{"id": roleID}, nil, body)
	if err != nil {
		return nil, err
	}
	return map[string]any{"role": asMap(resp)}, nil
}
