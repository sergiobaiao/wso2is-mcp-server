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

// Package extensions implements the curated webhook and action tools
// (contracts/admin-curated-tools.md).
package extensions

import (
	"context"
	"errors"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// actionTypes are the action type path values of the IS 7.3 actions API.
var actionTypes = []string{"preIssueAccessToken", "preUpdatePassword", "preUpdateProfile", "preRegistration"}

// Tools returns the six webhook and action tools.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"extensions.getWebhooks", ""},
		{"extensions.createWebhook", ""},
		{"extensions.deleteWebhook", ""},
		{"extensions.getActionTypes", "extensions.org.getActionTypes"},
		{"extensions.getActionsByActionType", "extensions.org.getActionsByActionType"},
		{"extensions.createAction", "extensions.org.createAction"},
		{"extensions.deleteAction", "extensions.org.deleteAction"},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	hooks, addHook, delHook, types, actions, addAction, delAction := ops[0], ops[1], ops[2], ops[3], ops[4], ops[5], ops[6]
	actionType := mcp.WithString("action_type", mcp.Required(), mcp.Enum(actionTypes...), mcp.Description("Action type."))

	return []toolkit.ToolDef{
		{
			Name: "list_webhooks", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: hooks.Scopes(), Operation: hooks.Root,
			Description: "List the webhooks of WSO2 Identity Server that send events to external endpoints.",
			Handler: func(ctx context.Context, _ map[string]any) (any, error) {
				return toolkit.Call(ctx, cl, hooks, nil, nil, nil)
			},
		},
		{
			Name: "create_webhook", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: addHook.Scopes(), Operation: addHook.Root,
			Description: "Create a webhook in WSO2 Identity Server that sends the subscribed events to an endpoint. Event channel URIs come from the webhook metadata (search_operations webhook metadata).",
			Options: []mcp.ToolOption{
				mcp.WithString("name", mcp.Required(), mcp.Description("Webhook name.")),
				mcp.WithString("endpoint", mcp.Required(), mcp.Description("HTTPS endpoint that receives the events.")),
				mcp.WithArray("channels_subscribed", mcp.Required(), mcp.WithStringItems(), mcp.Description("Event channel URIs, e.g. https://schemas.identity.wso2.org/events/login.")),
				mcp.WithString("secret", mcp.Description("Secret IS uses to sign events. Sent to IS and never returned.")),
				mcp.WithObject("event_profile", mcp.Description("Event profile {name, uri}; default WSO2 https://schemas.identity.wso2.org/events.")),
				mcp.WithString("status", mcp.Enum("ACTIVE", "INACTIVE"), mcp.Description("Initial status; default ACTIVE.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body := map[string]any{"eventProfile": map[string]any{"name": "WSO2", "uri": "https://schemas.identity.wso2.org/events"}}
				for _, f := range []string{"name", "endpoint"} {
					v, err := toolkit.RequireString(args, f)
					if err != nil {
						return nil, err
					}
					body[f] = v
				}
				channels, err := toolkit.OptStrings(args, "channels_subscribed")
				if err != nil {
					return nil, err
				}
				if len(channels) == 0 {
					return nil, errors.New("invalid input: channels_subscribed: is required")
				}
				body["channelsSubscribed"] = channels
				toolkit.PutIf(body, args, map[string]string{"secret": "secret", "event_profile": "eventProfile", "status": "status"})
				return toolkit.Call(ctx, cl, addHook, nil, nil, body)
			},
		},
		{
			Name: "delete_webhook", Layer: toolkit.LayerCurated, Kind: catalog.KindDestructive,
			Scopes: delHook.Scopes(), Operation: delHook.Root,
			Description: "Delete a webhook from WSO2 Identity Server; its endpoint stops receiving events.",
			Options:     []mcp.ToolOption{mcp.WithString("webhook_id", mcp.Required(), mcp.Description("Webhook id."))},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "webhook_id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, delHook, map[string]string{"webhookId": id}, nil, nil)
			},
		},
		{
			Name: "list_actions", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: toolkit.UnionScopes(types.Root, types.Org, actions.Root, actions.Org), OrgCapable: true, Operation: actions.Root,
			Description: "List the actions (extension endpoints IS calls during flows such as token issuance or password update) in WSO2 Identity Server, for one action type or for all.",
			Options:     []mcp.ToolOption{mcp.WithString("action_type", mcp.Enum(actionTypes...), mcp.Description("Only this action type."))},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				t, err := toolkit.OptString(args, "action_type")
				if err != nil {
					return nil, err
				}
				if t != "" {
					return toolkit.Call(ctx, cl, actions, map[string]string{"actionType": t}, nil, nil)
				}
				resp, err := toolkit.Call(ctx, cl, types, nil, nil, nil)
				if err != nil {
					return nil, err
				}
				// A list, not a map keyed by type: keys such as preIssueAccessToken look like secrets to redaction.
				out := []any{}
				for _, it := range asSlice(resp) {
					m, _ := it.(map[string]any)
					self, _ := m["self"].(string)
					if count, _ := m["count"].(float64); count == 0 || self == "" {
						continue
					}
					typ := self[strings.LastIndex(self, "/")+1:]
					list, err := toolkit.Call(ctx, cl, actions, map[string]string{"actionType": typ}, nil, nil)
					if err != nil {
						return nil, err
					}
					out = append(out, map[string]any{"action_type": typ, "actions": list})
				}
				return out, nil
			},
		},
		{
			Name: "create_action", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: addAction.Scopes(), OrgCapable: true, Operation: addAction.Root,
			Description: "Create an action in WSO2 Identity Server: an external endpoint IS calls during a flow (pre issue access token, pre update password, pre update profile, pre registration).",
			Options: []mcp.ToolOption{actionType,
				mcp.WithString("name", mcp.Required(), mcp.Description("Action name.")),
				mcp.WithString("description", mcp.Description("Description.")),
				mcp.WithString("endpoint_uri", mcp.Required(), mcp.Description("HTTPS endpoint IS calls.")),
				mcp.WithString("auth_type", mcp.Required(), mcp.Enum("NONE", "BASIC", "BEARER", "API_KEY"), mcp.Description("How IS authenticates to the endpoint.")),
				mcp.WithObject("auth_properties", mcp.Description("Credentials for auth_type: BASIC {username, password}, BEARER {accessToken}, API_KEY {header, value}. Sent to IS and never returned.")),
				mcp.WithArray("allowed_headers", mcp.WithStringItems(), mcp.Description("Request headers IS forwards to the endpoint.")),
				mcp.WithArray("allowed_parameters", mcp.WithStringItems(), mcp.Description("Request parameters IS forwards to the endpoint.")),
				mcp.WithObject("rule", mcp.Description("Optional rule that decides when the action runs.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				typ, err := toolkit.RequireString(args, "action_type")
				if err != nil {
					return nil, err
				}
				name, err := toolkit.RequireString(args, "name")
				if err != nil {
					return nil, err
				}
				uri, err := toolkit.RequireString(args, "endpoint_uri")
				if err != nil {
					return nil, err
				}
				authType, err := toolkit.RequireString(args, "auth_type")
				if err != nil {
					return nil, err
				}
				auth := map[string]any{"type": authType}
				toolkit.PutIf(auth, args, map[string]string{"auth_properties": "properties"})
				endpoint := map[string]any{"uri": uri, "authentication": auth}
				toolkit.PutIf(endpoint, args, map[string]string{"allowed_headers": "allowedHeaders", "allowed_parameters": "allowedParameters"})
				body := map[string]any{"name": name, "endpoint": endpoint}
				toolkit.PutIf(body, args, map[string]string{"description": "description", "rule": "rule"})
				return toolkit.Call(ctx, cl, addAction, map[string]string{"actionType": typ}, nil, body)
			},
		},
		{
			Name: "delete_action", Layer: toolkit.LayerCurated, Kind: catalog.KindDestructive,
			Scopes: delAction.Scopes(), OrgCapable: true, Operation: delAction.Root,
			Description: "Delete an action from WSO2 Identity Server; IS stops calling its endpoint.",
			Options:     []mcp.ToolOption{actionType, mcp.WithString("action_id", mcp.Required(), mcp.Description("Action id."))},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				typ, err := toolkit.RequireString(args, "action_type")
				if err != nil {
					return nil, err
				}
				id, err := toolkit.RequireString(args, "action_id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, delAction, map[string]string{"actionType": typ, "actionId": id}, nil, nil)
			},
		},
	}, nil
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
