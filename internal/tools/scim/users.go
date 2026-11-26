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
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func userTools(ops *opSet, cl *client.Client, cfg *config.Config) []toolkit.ToolDef {
	compactAttrs := compactUserAttributes(cfg)
	compactAttrCSV := strings.Join(compactAttrs, ",")
	profileSpecs := parseProfileAttributes(cfg)

	profileOpts := make([]mcp.ToolOption, 0, len(profileSpecs)+2)
	profileOpts = append(profileOpts,
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user to update.")),
	)
	for _, spec := range profileSpecs {
		profileOpts = append(profileOpts, mcp.WithString(
			spec.shortName,
			mcp.Description(fmt.Sprintf("Profile attribute %s (%s).", spec.shortName, spec.raw)),
		))
	}

	return []toolkit.ToolDef{
		{
			Name:        "list_users",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.listUsers.root, ops.listUsers.org),
			OrgCapable:  true,
			Operation:   ops.listUsers.root,
			Description: "List users in WSO2 Identity Server with compact profile attributes.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithInteger("count", mcp.Description("Maximum number of users to return (default 100).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				count, err := optionalCount(args, defaultListCount)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.listUsers.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(callCtx, cl, op, nil, map[string]any{
					"attributes": compactAttrCSV,
					"startIndex": 1,
					"count":      count,
				}, nil)
				if err != nil {
					return nil, err
				}
				return listOutput(resp, "users"), nil
			},
		},
		{
			Name:        "search_users",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.searchUsers.root, ops.searchUsers.org),
			OrgCapable:  true,
			Operation:   ops.searchUsers.root,
			Description: "Search users in WSO2 Identity Server using a SCIM filter expression.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("filter", mcp.Required(), mcp.Description("SCIM filter expression (e.g., userName eq kim or emails co example.com).")),
				mcp.WithInteger("count", mcp.Description("Maximum number of users to return (default 100).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				filter, err := requiredString(args, "filter")
				if err != nil {
					return nil, err
				}
				count, err := optionalCount(args, defaultListCount)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.searchUsers.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":    []string{searchReqSchema},
					"attributes": compactAttrs,
					"filter":     filter,
					"startIndex": 1,
					"count":      count,
				}
				resp, err := doOpBodylessValidation(callCtx, cl, op, nil, nil, body)
				if err != nil {
					return nil, err
				}
				return listOutput(resp, "users"), nil
			},
		},
		{
			Name:        "get_user",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      unionScopes(ops.getUser.root, ops.getUser.org),
			OrgCapable:  true,
			Operation:   ops.getUser.root,
			Description: "Get full details of a user by ID in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.getUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOp(callCtx, cl, op, map[string]string{"id": userID}, nil, nil)
				if err != nil {
					return nil, err
				}
				return map[string]any{"user": asMap(resp)}, nil
			},
		},
		{
			Name:        "create_user",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.createUser.root, ops.createUser.org),
			OrgCapable:  true,
			Operation:   ops.createUser.root,
			Description: "Create a new user in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("userName", mcp.Description("Username for the user.")),
				mcp.WithString("username", mcp.Description("Legacy alias for userName.")),
				mcp.WithString("password", mcp.Required(), mcp.Description("Initial password for the user.")),
				mcp.WithString("givenName", mcp.Description("Given (first) name of the user.")),
				mcp.WithString("first_name", mcp.Description("Legacy alias for givenName.")),
				mcp.WithString("familyName", mcp.Description("Family (last) name of the user.")),
				mcp.WithString("last_name", mcp.Description("Legacy alias for familyName.")),
				mcp.WithString("email", mcp.Required(), mcp.Description("Primary email address of the user.")),
				mcp.WithString("userstore_domain", mcp.Description("Userstore domain (default PRIMARY).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body, err := buildCreateUserBody(args)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.createUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				resp, err := doOpBodylessValidation(callCtx, cl, op, nil, nil, body)
				if err != nil {
					return nil, err
				}
				m := asMap(resp)
				return map[string]any{
					"id":       m["id"],
					"userName": m["userName"],
				}, nil
			},
		},
		{
			Name:        "update_user_attribute",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchUser.root, ops.patchUser.org),
			OrgCapable:  true,
			Operation:   ops.patchUser.root,
			Description: "Replace a single SCIM attribute on a user in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user.")),
				mcp.WithString("attribute", mcp.Required(), mcp.Description("SCIM attribute path to replace (e.g., name.givenName or active).")),
				mcp.WithAny("value", mcp.Required(), mcp.Description("New value for the attribute.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}
				attr, err := requiredString(args, "attribute")
				if err != nil {
					return nil, err
				}
				val, hasVal := args["value"]
				if !hasVal || val == nil {
					return nil, errors.New("invalid input: value: is required")
				}
				op, callCtx, err := ops.patchUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas": []string{patchOpSchema},
					"Operations": []map[string]any{
						{
							"op":    "replace",
							"path":  attr,
							"value": val,
						},
					},
				}
				resp, err := doOpBodylessValidation(callCtx, cl, op, map[string]string{"id": userID}, nil, body)
				if err != nil {
					return nil, err
				}
				m := asMap(resp)
				return map[string]any{
					"id":       m["id"],
					"userName": m["userName"],
				}, nil
			},
		},
		{
			Name:        "update_user_profile",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchUser.root, ops.patchUser.org),
			OrgCapable:  true,
			Operation:   ops.patchUser.root,
			Description: "Update configured profile attributes on a user in WSO2 Identity Server.",
			Options:     profileOpts,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}
				patchOps, err := buildProfilePatchOps(profileSpecs, args)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.patchUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":    []string{patchOpSchema},
					"Operations": patchOps,
				}
				resp, err := doOpBodylessValidation(callCtx, cl, op, map[string]string{"id": userID}, nil, body)
				if err != nil {
					return nil, err
				}
				m := asMap(resp)
				out := map[string]any{
					"id":       m["id"],
					"userName": m["userName"],
				}
				if addrs, ok := m["addresses"]; ok && addrs != nil {
					out["addresses"] = addrs
				}
				if ent, ok := m[enterpriseUserSchema]; ok && ent != nil {
					out["enterprise"] = ent
				}
				if cust, ok := m[customUserSchema]; ok && cust != nil {
					out["custom"] = cust
				}
				return out, nil
			},
		},
		{
			Name:        "delete_user",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindDestructive,
			Scopes:      unionScopes(ops.deleteUser.root, ops.deleteUser.org),
			OrgCapable:  true,
			Operation:   ops.deleteUser.root,
			Description: "Delete a user by ID from WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user to delete.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.deleteUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				if _, err := doOp(callCtx, cl, op, map[string]string{"id": userID}, nil, nil); err != nil {
					return nil, err
				}
				return map[string]any{"status": "deleted"}, nil
			},
		},
		{
			Name:        "patch_user",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.patchUser.root, ops.patchUser.org),
			OrgCapable:  true,
			Operation:   ops.patchUser.root,
			Description: "Apply raw SCIM PATCH Operations to a user in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithString("userId", mcp.Required(), mcp.Description("Unique ID of the user to patch.")),
				mcp.WithArray("operations", mcp.Required(), mcp.Description("SCIM PatchOp Operations array ({op, path?, value?}).")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				userID, err := requiredString(args, "userId")
				if err != nil {
					return nil, err
				}
				patchOps, err := parseRawPatchOperations(args)
				if err != nil {
					return nil, err
				}
				op, callCtx, err := ops.patchUser.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":    []string{patchOpSchema},
					"Operations": patchOps,
				}
				resp, err := doOpBodylessValidation(callCtx, cl, op, map[string]string{"id": userID}, nil, body)
				if err != nil {
					return nil, err
				}
				return map[string]any{"user": asMap(resp)}, nil
			},
		},
	}
}

func resolveAliasedField(args map[string]any, primary, alias string, required bool) (string, error) {
	v1, err := optionalString(args, primary)
	if err != nil {
		return "", err
	}
	v2, err := optionalString(args, alias)
	if err != nil {
		return "", err
	}
	if v1 != "" && v2 != "" && v1 != v2 {
		return "", fmt.Errorf("invalid input: %s: conflicting values between %s and %s", primary, primary, alias)
	}
	val := v1
	if val == "" {
		val = v2
	}
	if required && val == "" {
		return "", fmt.Errorf("invalid input: %s: is required", primary)
	}
	return val, nil
}

func buildCreateUserBody(args map[string]any) (map[string]any, error) {
	rawUserName, err := resolveAliasedField(args, "userName", "username", true)
	if err != nil {
		return nil, err
	}
	password, err := requiredString(args, "password")
	if err != nil {
		return nil, err
	}
	givenName, err := resolveAliasedField(args, "givenName", "first_name", true)
	if err != nil {
		return nil, err
	}
	familyName, err := resolveAliasedField(args, "familyName", "last_name", true)
	if err != nil {
		return nil, err
	}
	email, err := requiredString(args, "email")
	if err != nil {
		return nil, err
	}
	domain, err := optionalString(args, "userstore_domain")
	if err != nil {
		return nil, err
	}

	fullUserName := rawUserName
	if strings.Contains(rawUserName, "/") {
		prefix, _, _ := strings.Cut(rawUserName, "/")
		if domain != "" && !strings.EqualFold(prefix, domain) {
			return nil, fmt.Errorf("invalid input: userstore_domain: conflicts with domain prefix in %q", rawUserName)
		}
	} else {
		if domain == "" {
			domain = "PRIMARY"
		}
		fullUserName = domain + "/" + rawUserName
	}

	return map[string]any{
		"schemas":  []string{coreUserSchema},
		"userName": fullUserName,
		"password": password,
		"name": map[string]any{
			"givenName":  givenName,
			"familyName": familyName,
		},
		"emails": []map[string]any{
			{
				"value":   email,
				"primary": true,
			},
		},
	}, nil
}

func buildProfilePatchOps(specs []profileAttrSpec, args map[string]any) ([]map[string]any, error) {
	byShort := make(map[string]profileAttrSpec, len(specs))
	for _, s := range specs {
		byShort[s.shortName] = s
	}

	extGrouped := make(map[string]map[string]any)
	var addresses []map[string]any
	coreAttrs := make(map[string]any)
	matched := 0

	keys := make([]string, 0, len(args))
	for k := range args {
		if k == "userId" || k == "organization_id" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		val := args[k]
		if val == nil {
			continue
		}
		spec, ok := byShort[k]
		if !ok {
			return nil, fmt.Errorf("invalid input: %s: attribute is not configured in SCIM_PROFILE_ATTRIBUTES", k)
		}
		matched++
		switch {
		case spec.addressType != "":
			if m, isMap := val.(map[string]any); isMap {
				entry := make(map[string]any, len(m)+1)
				for mk, mv := range m {
					entry[mk] = mv
				}
				entry["type"] = spec.addressType
				addresses = append(addresses, entry)
			} else {
				addresses = append(addresses, map[string]any{
					"type":      spec.addressType,
					"formatted": val,
				})
			}
		case spec.schemaURN != "":
			if extGrouped[spec.schemaURN] == nil {
				extGrouped[spec.schemaURN] = make(map[string]any)
			}
			extGrouped[spec.schemaURN][spec.attrName] = val
		default:
			coreAttrs[spec.attrName] = val
		}
	}

	if matched == 0 {
		return nil, errors.New("invalid input: at least one configured profile attribute must be provided")
	}

	var ops []map[string]any
	if len(coreAttrs) > 0 {
		ops = append(ops, map[string]any{
			"op":    "replace",
			"value": coreAttrs,
		})
	}
	if len(addresses) > 0 {
		ops = append(ops, map[string]any{
			"op": "replace",
			"value": map[string]any{
				"addresses": addresses,
			},
		})
	}

	urns := make([]string, 0, len(extGrouped))
	for urn := range extGrouped {
		urns = append(urns, urn)
	}
	sort.Strings(urns)
	for _, urn := range urns {
		ops = append(ops, map[string]any{
			"op": "replace",
			"value": map[string]any{
				urn: extGrouped[urn],
			},
		})
	}

	return ops, nil
}

func parseRawPatchOperations(args map[string]any) ([]map[string]any, error) {
	raw, exists := args["operations"]
	if !exists || raw == nil {
		raw, exists = args["Operations"]
	}
	if !exists || raw == nil {
		return nil, errors.New("invalid input: operations: is required")
	}

	rawSlice, ok := raw.([]any)
	if !ok || len(rawSlice) == 0 {
		return nil, errors.New("invalid input: operations: must be a non-empty array")
	}

	out := make([]map[string]any, 0, len(rawSlice))
	for i, item := range rawSlice {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid input: operations[%d]: must be an object", i)
		}
		opRaw, _ := m["op"].(string)
		opName := strings.ToLower(strings.TrimSpace(opRaw))
		switch opName {
		case "add", "remove", "replace":
		default:
			return nil, fmt.Errorf("invalid input: operations[%d].op: must be add, remove, or replace", i)
		}
		pathStr, _ := m["path"].(string)
		pathStr = strings.TrimSpace(pathStr)
		val, hasVal := m["value"]

		if opName == "remove" && pathStr == "" {
			return nil, fmt.Errorf("invalid input: operations[%d].path: is required for remove", i)
		}
		if (opName == "add" || opName == "replace") && (!hasVal || val == nil) {
			return nil, fmt.Errorf("invalid input: operations[%d].value: is required for %s", i, opName)
		}

		entry := map[string]any{"op": opName}
		if pathStr != "" {
			entry["path"] = pathStr
		}
		if hasVal && val != nil {
			entry["value"] = val
		}
		out = append(out, entry)
	}
	return out, nil
}
