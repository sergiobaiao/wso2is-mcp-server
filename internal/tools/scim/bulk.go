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

const maxBulkUsers = 100

type bulkUserInput struct {
	userName   string
	password   string
	givenName  string
	familyName string
	email      string
}

func bulkTools(ops *opSet, cl *client.Client) []toolkit.ToolDef {
	return []toolkit.ToolDef{
		{
			Name:        "bulk_create_users",
			Layer:       toolkit.LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      unionScopes(ops.bulkUsers.root, ops.bulkUsers.org),
			OrgCapable:  true,
			Operation:   ops.bulkUsers.root,
			Description: "Create 1 to 100 users in a single SCIM Bulk request in WSO2 Identity Server.",
			Options: []mcp.ToolOption{
				mcp.WithSchemaAdditionalProperties(false),
				mcp.WithArray("users",
					mcp.Description("List of 1 to 100 user objects ({userName, password, givenName, familyName, email?})."),
					mcp.Items(map[string]any{
						"type":     "object",
						"required": []string{"userName", "password", "givenName", "familyName"},
						"properties": map[string]any{
							"userName":   map[string]any{"type": "string"},
							"password":   map[string]any{"type": "string"},
							"givenName":  map[string]any{"type": "string"},
							"familyName": map[string]any{"type": "string"},
							"email":      map[string]any{"type": "string"},
						},
					}),
				),
				mcp.WithString("user1UserName", mcp.Description("Legacy alias for user 1 userName.")),
				mcp.WithString("user1Username", mcp.Description("Legacy alias for user 1 username.")),
				mcp.WithString("user1Password", mcp.Description("Legacy alias for user 1 password.")),
				mcp.WithString("user1GivenName", mcp.Description("Legacy alias for user 1 givenName.")),
				mcp.WithString("user1FamilyName", mcp.Description("Legacy alias for user 1 familyName.")),
				mcp.WithString("user1Email", mcp.Description("Legacy alias for user 1 email.")),
				mcp.WithString("user2UserName", mcp.Description("Legacy alias for user 2 userName.")),
				mcp.WithString("user2Username", mcp.Description("Legacy alias for user 2 username.")),
				mcp.WithString("user2Password", mcp.Description("Legacy alias for user 2 password.")),
				mcp.WithString("user2GivenName", mcp.Description("Legacy alias for user 2 givenName.")),
				mcp.WithString("user2FamilyName", mcp.Description("Legacy alias for user 2 familyName.")),
				mcp.WithString("user2Email", mcp.Description("Legacy alias for user 2 email.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				users, err := parseBulkUsers(args)
				if err != nil {
					return nil, err
				}

				bulkOps := make([]map[string]any, 0, len(users))
				for i, u := range users {
					data := map[string]any{
						"schemas":  []string{coreUserSchema},
						"userName": u.userName,
						"password": u.password,
						"name": map[string]any{
							"givenName":  u.givenName,
							"familyName": u.familyName,
						},
					}
					if u.email != "" {
						data["emails"] = []map[string]any{
							{"value": u.email, "primary": true},
						}
					}
					bulkOps = append(bulkOps, map[string]any{
						"method": "POST",
						"path":   "/Users",
						"bulkId": fmt.Sprintf("user-%d", i+1),
						"data":   data,
					})
				}

				op, callCtx, err := ops.bulkUsers.selectOp(ctx, args)
				if err != nil {
					return nil, err
				}
				body := map[string]any{
					"schemas":      []string{bulkReqSchema},
					"failOnErrors": 0,
					"Operations":   bulkOps,
				}
				resp, err := doOp(callCtx, cl, op, nil, nil, body)
				if err != nil {
					return nil, err
				}
				return formatBulkResults(resp), nil
			},
		},
	}
}

func parseBulkUsers(args map[string]any) ([]bulkUserInput, error) {
	rawUsers, hasUsers := args["users"]
	hasLegacy := hasAnyLegacyBulkField(args)
	if hasUsers && rawUsers != nil && hasLegacy {
		return nil, errors.New("invalid input: provide either users array or legacy user1*/user2* fields, not both")
	}

	if hasUsers && rawUsers != nil {
		items, ok := rawUsers.([]any)
		if !ok {
			return nil, errors.New("invalid input: users: must be an array")
		}
		if len(items) < 1 || len(items) > maxBulkUsers {
			return nil, fmt.Errorf("invalid input: users: must contain between 1 and %d entries", maxBulkUsers)
		}
		out := make([]bulkUserInput, 0, len(items))
		for i, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid input: users[%d]: must be an object", i)
			}
			u, err := parseBulkUserEntry(fmt.Sprintf("users[%d]", i), m)
			if err != nil {
				return nil, err
			}
			out = append(out, u)
		}
		return out, nil
	}

	if hasLegacy {
		var out []bulkUserInput
		for _, prefix := range []string{"user1", "user2"} {
			if !hasLegacyPrefix(args, prefix) {
				continue
			}
			entry := map[string]any{
				"userName":   args[prefix+"UserName"],
				"username":   args[prefix+"Username"],
				"password":   args[prefix+"Password"],
				"givenName":  args[prefix+"GivenName"],
				"familyName": args[prefix+"FamilyName"],
				"email":      args[prefix+"Email"],
			}
			u, err := parseBulkUserEntry(prefix, entry)
			if err != nil {
				return nil, err
			}
			out = append(out, u)
		}
		if len(out) == 0 {
			return nil, errors.New("invalid input: users: at least 1 user is required")
		}
		return out, nil
	}

	return nil, errors.New("invalid input: users: is required")
}

func parseBulkUserEntry(ctxLabel string, m map[string]any) (bulkUserInput, error) {
	userName, err := resolveAliasedField(m, "userName", "username", true)
	if err != nil {
		return bulkUserInput{}, fmt.Errorf("invalid input: %s: %w", ctxLabel, err)
	}
	password, err := requiredString(m, "password")
	if err != nil {
		return bulkUserInput{}, fmt.Errorf("invalid input: %s: %w", ctxLabel, err)
	}
	givenName, err := requiredString(m, "givenName")
	if err != nil {
		return bulkUserInput{}, fmt.Errorf("invalid input: %s: %w", ctxLabel, err)
	}
	familyName, err := requiredString(m, "familyName")
	if err != nil {
		return bulkUserInput{}, fmt.Errorf("invalid input: %s: %w", ctxLabel, err)
	}
	email, err := optionalString(m, "email")
	if err != nil {
		return bulkUserInput{}, fmt.Errorf("invalid input: %s: %w", ctxLabel, err)
	}
	return bulkUserInput{
		userName:   userName,
		password:   password,
		givenName:  givenName,
		familyName: familyName,
		email:      email,
	}, nil
}

func hasAnyLegacyBulkField(args map[string]any) bool {
	return hasLegacyPrefix(args, "user1") || hasLegacyPrefix(args, "user2")
}

func hasLegacyPrefix(args map[string]any, prefix string) bool {
	for _, suffix := range []string{"UserName", "Username", "Password", "GivenName", "FamilyName", "Email"} {
		if v, ok := args[prefix+suffix]; ok && v != nil {
			if s, isStr := v.(string); !isStr || s != "" {
				return true
			}
		}
	}
	return false
}

func formatBulkResults(resp any) map[string]any {
	m := asMap(resp)
	var rawOps []any
	switch ops := m["Operations"].(type) {
	case []any:
		rawOps = ops
	case map[string]any:
		rawOps = []any{ops}
	}

	results := make([]map[string]any, 0, len(rawOps))
	for _, rawOp := range rawOps {
		item := asMap(rawOp)
		entry := map[string]any{
			"location": item["location"],
			"status":   item["status"],
		}
		if bulkID, ok := item["bulkId"]; ok && bulkID != nil {
			entry["bulkId"] = bulkID
		}
		if respDetail, ok := item["response"]; ok && respDetail != nil {
			entry["response"] = respDetail
		}
		results = append(results, entry)
	}
	return map[string]any{"results": results}
}
