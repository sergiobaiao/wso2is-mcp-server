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

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/redact"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func noopHandler(_ context.Context, _ map[string]any) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestValidateName(t *testing.T) {
	valid := []string{
		"a",
		"list_users",
		"users_get_user_by_id_1",
		strings.Repeat("a", 64),
	}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"A",
		"ListUsers",
		"1abc",
		"_abc",
		"foo-bar",
		"foo.bar",
		"foo bar",
		strings.Repeat("a", 65),
	}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) expected error, got nil", name)
		}
	}
}

func TestToolAnnotationsPerKind(t *testing.T) {
	tests := []struct {
		kind            catalog.Kind
		wantReadOnly    bool
		wantDestructive bool
	}{
		{kind: catalog.KindRead, wantReadOnly: true, wantDestructive: false},
		{kind: catalog.KindWrite, wantReadOnly: false, wantDestructive: false},
		{kind: catalog.KindDestructive, wantReadOnly: false, wantDestructive: true},
	}

	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			st, err := BuildTool(ToolDef{
				Name:        "test_tool",
				Layer:       LayerCurated,
				Kind:        tc.kind,
				Description: "Test tool.",
				Handler:     noopHandler,
			}, 0)
			if err != nil {
				t.Fatalf("BuildTool failed: %v", err)
			}

			ann := st.Tool.Annotations
			if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != tc.wantReadOnly {
				t.Fatalf("ReadOnlyHint = %v, want %t", ann.ReadOnlyHint, tc.wantReadOnly)
			}
			if ann.DestructiveHint == nil || *ann.DestructiveHint != tc.wantDestructive {
				t.Fatalf("DestructiveHint = %v, want %t", ann.DestructiveHint, tc.wantDestructive)
			}
			if ann.OpenWorldHint == nil || !*ann.OpenWorldHint {
				t.Fatalf("OpenWorldHint = %v, want true", ann.OpenWorldHint)
			}
		})
	}
}

func TestToolDescriptionsAndOrgCapable(t *testing.T) {
	const orgHint = "Pass organization_id to run inside a sub-organization."
	const confirmSentence = "Before calling, describe the exact change and its target to the user and get their explicit yes to it. The user's request is not a yes, and a yes to an earlier or different change doesn't carry over."

	t.Run("read non-org keeps plain description", func(t *testing.T) {
		st, err := BuildTool(ToolDef{
			Name:        "get_server_info",
			Layer:       LayerCurated,
			Kind:        catalog.KindRead,
			OrgCapable:  false,
			Description: "Get server info.",
			Handler:     noopHandler,
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		if st.Tool.Description != "Get server info." {
			t.Fatalf("Description = %q", st.Tool.Description)
		}
		if _, hasOrg := st.Tool.InputSchema.Properties["organization_id"]; hasOrg {
			t.Fatal("did not expect organization_id property on non-org tool")
		}
	})

	t.Run("read org-capable adds org hint and organization_id", func(t *testing.T) {
		st, err := BuildTool(ToolDef{
			Name:        "list_users",
			Layer:       LayerCurated,
			Kind:        catalog.KindRead,
			OrgCapable:  true,
			Description: "List users.",
			Handler:     noopHandler,
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		want := "List users. " + orgHint
		if st.Tool.Description != want {
			t.Fatalf("Description = %q, want %q", st.Tool.Description, want)
		}
		if _, hasOrg := st.Tool.InputSchema.Properties["organization_id"]; !hasOrg {
			t.Fatal("expected organization_id property on org-capable tool")
		}
	})

	t.Run("write appends confirmation sentence without duplicating", func(t *testing.T) {
		st, err := BuildTool(ToolDef{
			Name:        "update_user",
			Layer:       LayerCurated,
			Kind:        catalog.KindWrite,
			Description: "Update user.",
			Handler:     noopHandler,
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		want := "Update user. " + confirmSentence
		if st.Tool.Description != want {
			t.Fatalf("Description = %q, want %q", st.Tool.Description, want)
		}

		st2, err := BuildTool(ToolDef{
			Name:        "update_user_2",
			Layer:       LayerCurated,
			Kind:        catalog.KindWrite,
			Description: want,
			Handler:     noopHandler,
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		if strings.Count(st2.Tool.Description, confirmSentence) != 1 {
			t.Fatalf("confirmation sentence duplicated: %q", st2.Tool.Description)
		}
	})

	t.Run("destructive org-capable includes both sentences and organization_id", func(t *testing.T) {
		st, err := BuildTool(ToolDef{
			Name:        "delete_user",
			Layer:       LayerCurated,
			Kind:        catalog.KindDestructive,
			OrgCapable:  true,
			Description: "Delete a user.",
			Handler:     noopHandler,
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		if !strings.Contains(st.Tool.Description, orgHint) {
			t.Fatalf("missing org hint in %q", st.Tool.Description)
		}
		if !strings.HasSuffix(st.Tool.Description, confirmSentence) {
			t.Fatalf("expected confirmation sentence at end of %q", st.Tool.Description)
		}
		if _, hasOrg := st.Tool.InputSchema.Properties["organization_id"]; !hasOrg {
			t.Fatal("expected organization_id property on org-capable destructive tool")
		}
	})
}

func TestToolHandlerExecution(t *testing.T) {
	t.Run("redact and truncate and org context", func(t *testing.T) {
		var seenOrg string
		reg := NewRegistry(false, 220)
		op := &catalog.Operation{
			ID: "users.listUsers",
			Params: []catalog.Param{
				{Name: "count", In: "query"},
				{Name: "startIndex", In: "query"},
			},
		}
		reg.MustRegister(ToolDef{
			Name:       "list_items",
			Layer:      LayerCurated,
			Kind:       catalog.KindRead,
			OrgCapable: true,
			Operation:  op,
			Handler: func(ctx context.Context, _ map[string]any) (any, error) {
				cc, _ := client.CredentialContextFrom(ctx)
				seenOrg = cc.OrganizationID
				return []any{
					map[string]any{"id": "1", "clientSecret": "top-secret-1", "label": "item-one"},
					map[string]any{"id": "2", "password": "pw-2", "label": "item-two"},
					map[string]any{"id": "3", "token": "tok-3", "label": "item-three"},
					map[string]any{"id": "4", "secret": "sec-4", "label": "item-four"},
					map[string]any{"id": "5", "apiKey": "key-5", "label": "item-five"},
					map[string]any{"id": "6", "privateKey": "pk-6", "label": "item-six"},
				}, nil
			},
		})

		srv := server.NewMCPServer("test", "1.0.0")
		reg.AddToServer(srv)

		c, err := mcpclient.NewInProcessClient(srv)
		if err != nil {
			t.Fatalf("NewInProcessClient failed: %v", err)
		}
		t.Cleanup(func() { _ = c.Close() })
		if err := c.Start(t.Context()); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if _, err := c.Initialize(t.Context(), mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
			},
		}); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		res, err := c.CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "list_items",
				Arguments: map[string]any{"organization_id": "org-123"},
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected tool error: %s", firstTextContent(t, res))
		}
		if seenOrg != "org-123" {
			t.Fatalf("CredentialContext.OrganizationID = %q, want %q", seenOrg, "org-123")
		}

		text := firstTextContent(t, res)
		for _, secret := range []string{"top-secret-1", "pw-2", "tok-3", "sec-4", "key-5", "pk-6"} {
			if strings.Contains(text, secret) {
				t.Fatalf("secret %q leaked into tool output: %s", secret, text)
			}
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal tool output: %v\n%s", err, text)
		}
		if out["truncated"] != true {
			t.Fatalf("expected truncated=true, got %v", out["truncated"])
		}
		if out["total"] != float64(6) {
			t.Fatalf("expected total=6, got %v", out["total"])
		}
		hint, _ := out["hint"].(string)
		if !strings.Contains(hint, "count") || !strings.Contains(hint, "startIndex") {
			t.Fatalf("expected paging hint to mention count and startIndex, got %q", hint)
		}
		if items, ok := out["items"].([]any); ok && len(items) > 0 {
			first := items[0].(map[string]any)
			if first["clientSecret"] != redact.RedactedPlaceholder {
				t.Fatalf("expected redacted placeholder, got %v", first["clientSecret"])
			}
		}
	})

	t.Run("nil result returns Done", func(t *testing.T) {
		st, err := BuildTool(ToolDef{
			Name:        "noop_action",
			Layer:       LayerCurated,
			Kind:        catalog.KindRead,
			Description: "Noop.",
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return nil, nil
			},
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}

		res, err := st.Handler(t.Context(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatalf("Handler failed: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", firstTextContent(t, res))
		}
		if got := firstTextContent(t, res); got != "Done." {
			t.Fatalf("got %q, want %q", got, "Done.")
		}
	})

	t.Run("error rendering with 403 scopes and generic error", func(t *testing.T) {
		op := &catalog.Operation{
			ID:     "users.getUser",
			Scopes: []string{"internal_user_mgt_view", "internal_user_mgt_list"},
		}
		st403, err := BuildTool(ToolDef{
			Name:      "get_user",
			Layer:     LayerOperation,
			Kind:      catalog.KindRead,
			Operation: op,
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return nil, &client.APIError{
					StatusCode: http.StatusForbidden,
					Message:    "Access denied",
				}
			},
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		res403, err := st403.Handler(t.Context(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatalf("Handler failed: %v", err)
		}
		if !res403.IsError {
			t.Fatal("expected IsError=true for APIError")
		}
		want403 := "request failed with status 403: the credentials lack permission. Access denied. Required scopes: internal_user_mgt_view, internal_user_mgt_list."
		if got := firstTextContent(t, res403); got != want403 {
			t.Fatalf("got %q, want %q", got, want403)
		}

		stGeneric, err := BuildTool(ToolDef{
			Name:  "fail_tool",
			Layer: LayerCurated,
			Kind:  catalog.KindRead,
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				return nil, errors.New("boom")
			},
		}, 0)
		if err != nil {
			t.Fatalf("BuildTool failed: %v", err)
		}
		resGeneric, err := stGeneric.Handler(t.Context(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatalf("Handler failed: %v", err)
		}
		if !resGeneric.IsError || firstTextContent(t, resGeneric) != "boom" {
			t.Fatalf("got IsError=%t text=%q", resGeneric.IsError, firstTextContent(t, resGeneric))
		}
	})

	t.Run("write tool uses elicitation and SummaryFunc", func(t *testing.T) {
		var called atomic.Bool
		var capturedMsg string

		reg := NewRegistry(false, 0)
		reg.MustRegister(ToolDef{
			Name:  "delete_app",
			Layer: LayerCurated,
			Kind:  catalog.KindDestructive,
			SummaryFunc: func(args map[string]any) string {
				return "delete app " + args["id"].(string)
			},
			Options: []mcp.ToolOption{
				mcp.WithString("id", mcp.Required()),
			},
			Handler: func(_ context.Context, _ map[string]any) (any, error) {
				called.Store(true)
				return nil, nil
			},
		})

		srv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
		reg.AddToServer(srv)

		var approve atomic.Bool
		c, err := mcpclient.NewInProcessClientWithOptions(
			srv,
			mcpclient.WithElicitationHandler(&funcElicitationHandler{
				fn: func(_ context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
					capturedMsg = req.Params.Message
					if !approve.Load() {
						return &mcp.ElicitationResult{
							ElicitationResponse: mcp.ElicitationResponse{
								Action: mcp.ElicitationResponseActionDecline,
							},
						}, nil
					}
					return &mcp.ElicitationResult{
						ElicitationResponse: mcp.ElicitationResponse{
							Action:  mcp.ElicitationResponseActionAccept,
							Content: map[string]any{"approve": true},
						},
					}, nil
				},
			}),
		)
		if err != nil {
			t.Fatalf("NewInProcessClientWithOptions failed: %v", err)
		}
		t.Cleanup(func() { _ = c.Close() })
		if err := c.Start(t.Context()); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if _, err := c.Initialize(t.Context(), mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "elicit-client", Version: "1.0.0"},
				Capabilities: mcp.ClientCapabilities{
					Elicitation: &mcp.ElicitationCapability{},
				},
			},
		}); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		resDecline, err := c.CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "delete_app",
				Arguments: map[string]any{"id": "app-9"},
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if !resDecline.IsError || firstTextContent(t, resDecline) != NotApprovedMessage {
			t.Fatalf("unexpected decline result: IsError=%t text=%q", resDecline.IsError, firstTextContent(t, resDecline))
		}
		if called.Load() {
			t.Fatal("handler must not be called when elicitation is declined")
		}
		if capturedMsg != "delete_app: delete app app-9" {
			t.Fatalf("elicitation message = %q, want %q", capturedMsg, "delete_app: delete app app-9")
		}

		approve.Store(true)
		resAccept, err := c.CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "delete_app",
				Arguments: map[string]any{"id": "app-9"},
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if resAccept.IsError || firstTextContent(t, resAccept) != "Done." {
			t.Fatalf("unexpected accept result: IsError=%t text=%q", resAccept.IsError, firstTextContent(t, resAccept))
		}
		if !called.Load() {
			t.Fatal("handler should be called when elicitation is approved")
		}
	})
}

func TestRegistry(t *testing.T) {
	t.Run("rejects duplicates and invalid definitions", func(t *testing.T) {
		reg := NewRegistry(false, 0)

		if err := reg.Register(ToolDef{
			Name:    "InvalidName",
			Kind:    catalog.KindRead,
			Handler: noopHandler,
		}); err == nil {
			t.Fatal("expected error for invalid tool name")
		}

		if err := reg.Register(ToolDef{
			Name:    "missing_kind",
			Handler: noopHandler,
		}); err == nil {
			t.Fatal("expected error for missing kind")
		}

		if err := reg.Register(ToolDef{
			Name: "missing_handler",
			Kind: catalog.KindRead,
		}); err == nil {
			t.Fatal("expected error for nil handler")
		}

		def := ToolDef{
			Name:    "list_apps",
			Layer:   LayerCurated,
			Kind:    catalog.KindRead,
			Scopes:  []string{"internal_application_mgt_view"},
			Handler: noopHandler,
		}
		if err := reg.Register(def); err != nil {
			t.Fatalf("first Register failed: %v", err)
		}
		if err := reg.Register(def); err == nil {
			t.Fatal("expected duplicate name error on second Register")
		}

		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected MustRegister to panic on duplicate name")
			}
		}()
		reg.MustRegister(def)
	})

	t.Run("READ_ONLY filtering and Count Names Scopes", func(t *testing.T) {
		reg := NewRegistry(true, 0)
		reg.MustRegister(
			ToolDef{
				Name:    "list_users",
				Layer:   LayerCurated,
				Kind:    catalog.KindRead,
				Scopes:  []string{"internal_user_mgt_list", "internal_user_mgt_view"},
				Handler: noopHandler,
			},
			ToolDef{
				Name:    "create_user",
				Layer:   LayerCurated,
				Kind:    catalog.KindWrite,
				Scopes:  []string{"internal_user_mgt_create"},
				Handler: noopHandler,
			},
			ToolDef{
				Name:    "delete_user",
				Layer:   LayerCurated,
				Kind:    catalog.KindDestructive,
				Scopes:  []string{"internal_user_mgt_delete"},
				Handler: noopHandler,
			},
			ToolDef{
				Name:    "get_user",
				Layer:   LayerCurated,
				Kind:    catalog.KindRead,
				Scopes:  []string{"internal_user_mgt_view", "internal_org_user_mgt_view"},
				Handler: noopHandler,
			},
		)

		if got := reg.Count(); got != 2 {
			t.Fatalf("Count() = %d, want 2", got)
		}
		if got := reg.Names(); !reflect.DeepEqual(got, []string{"list_users", "get_user"}) {
			t.Fatalf("Names() = %v, want [list_users get_user]", got)
		}
		if got := len(reg.ServerTools()); got != 2 {
			t.Fatalf("len(ServerTools()) = %d, want 2", got)
		}
		wantScopes := []string{
			"internal_org_user_mgt_view",
			"internal_user_mgt_list",
			"internal_user_mgt_view",
		}
		if got := reg.Scopes(); !reflect.DeepEqual(got, wantScopes) {
			t.Fatalf("Scopes() = %v, want %v", got, wantScopes)
		}

		srv := server.NewMCPServer("test", "1.0.0")
		reg.AddToServer(srv)
		c, err := mcpclient.NewInProcessClient(srv)
		if err != nil {
			t.Fatalf("NewInProcessClient failed: %v", err)
		}
		t.Cleanup(func() { _ = c.Close() })
		if err := c.Start(t.Context()); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if _, err := c.Initialize(t.Context(), mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
			},
		}); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}
		listed, err := c.ListTools(t.Context(), mcp.ListToolsRequest{})
		if err != nil {
			t.Fatalf("ListTools failed: %v", err)
		}
		if len(listed.Tools) != 2 {
			t.Fatalf("listed tools = %d, want 2", len(listed.Tools))
		}
	})
}
