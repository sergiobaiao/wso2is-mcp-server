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
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type funcElicitationHandler struct {
	fn func(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
}

func (h *funcElicitationHandler) Elicit(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return h.fn(ctx, req)
}

func newApprovalTestServer(timeout time.Duration, targetCalled *atomic.Bool) *server.MCPServer {
	srv := server.NewMCPServer("test-server", "1.0.0", server.WithElicitation())
	srv.AddTool(
		mcp.NewTool("delete_user", mcp.WithDescription("Delete a user.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			approved, supported, err := RequestApproval(ctx, "delete_user", "delete user u-123", timeout)
			if err != nil {
				if !errors.Is(err, ErrNotApproved) {
					return mcp.NewToolResultError(fmt.Sprintf("unexpected error: %v", err)), nil
				}
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !approved {
				return mcp.NewToolResultError(NotApprovedMessage), nil
			}
			targetCalled.Store(true)
			return mcp.NewToolResultText(fmt.Sprintf("proceeded:supported=%t", supported)), nil
		},
	)
	return srv
}

func firstTextContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("expected non-empty tool result content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func TestRequestApprovalDefaults(t *testing.T) {
	if DefaultElicitationTimeout != 120*time.Second {
		t.Fatalf("DefaultElicitationTimeout = %v, want 120s", DefaultElicitationTimeout)
	}
	if NotApprovedMessage != "Not approved by the user. Nothing was changed." {
		t.Fatalf("NotApprovedMessage = %q", NotApprovedMessage)
	}
	if ErrNotApproved == nil || ErrNotApproved.Error() != NotApprovedMessage {
		t.Fatalf("ErrNotApproved = %v, want %q", ErrNotApproved, NotApprovedMessage)
	}
}

func TestRequestApproval_WithoutElicitationCapability(t *testing.T) {
	var targetCalled atomic.Bool
	srv := newApprovalTestServer(DefaultElicitationTimeout, &targetCalled)

	c, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient failed: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("client Start failed: %v", err)
	}
	if _, err := c.Initialize(t.Context(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "no-elicit-client", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	}); err != nil {
		t.Fatalf("client Initialize failed: %v", err)
	}

	res, err := c.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "delete_user"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected tool to succeed without elicitation capability, got error: %s", firstTextContent(t, res))
	}
	if got := firstTextContent(t, res); got != "proceeded:supported=false" {
		t.Fatalf("got text %q, want %q", got, "proceeded:supported=false")
	}
	if !targetCalled.Load() {
		t.Fatal("expected target callback to be invoked when client lacks elicitation capability")
	}
}

func TestRequestApproval_WithElicitationCapability(t *testing.T) {
	tests := []struct {
		name           string
		timeout        time.Duration
		handler        func(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
		wantErr        bool
		wantText       string
		wantTargetCall bool
		checkRequest   func(t *testing.T, req mcp.ElicitationRequest)
	}{
		{
			name:    "accept approve true proceeds",
			timeout: DefaultElicitationTimeout,
			handler: func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
				return &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action:  mcp.ElicitationResponseActionAccept,
						Content: map[string]any{"approve": true},
					},
				}, nil
			},
			wantErr:        false,
			wantText:       "proceeded:supported=true",
			wantTargetCall: true,
			checkRequest: func(t *testing.T, req mcp.ElicitationRequest) {
				t.Helper()
				if req.Params.Message != "delete_user: delete user u-123" {
					t.Fatalf("elicitation message = %q, want %q", req.Params.Message, "delete_user: delete user u-123")
				}
				schema, ok := req.Params.RequestedSchema.(map[string]any)
				if !ok {
					t.Fatalf("RequestedSchema type = %T, want map[string]any", req.Params.RequestedSchema)
				}
				if schema["type"] != "object" {
					t.Fatalf("schema type = %v, want object", schema["type"])
				}
				if !reflect.DeepEqual(schema["required"], []string{"approve"}) {
					t.Fatalf("schema required = %v, want [approve]", schema["required"])
				}
				props, ok := schema["properties"].(map[string]any)
				if !ok {
					t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
				}
				approveProp, ok := props["approve"].(map[string]any)
				if !ok {
					t.Fatalf("approve property = %T, want map[string]any", props["approve"])
				}
				if approveProp["type"] != "boolean" {
					t.Fatalf("approve type = %v, want boolean", approveProp["type"])
				}
				if approveProp["title"] != "Approve this change?" {
					t.Fatalf("approve title = %v, want %q", approveProp["title"], "Approve this change?")
				}
			},
		},
		{
			name:    "accept approve false rejects",
			timeout: DefaultElicitationTimeout,
			handler: func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
				return &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action:  mcp.ElicitationResponseActionAccept,
						Content: map[string]any{"approve": false},
					},
				}, nil
			},
			wantErr:        true,
			wantText:       NotApprovedMessage,
			wantTargetCall: false,
		},
		{
			name:    "decline rejects",
			timeout: DefaultElicitationTimeout,
			handler: func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
				return &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action: mcp.ElicitationResponseActionDecline,
					},
				}, nil
			},
			wantErr:        true,
			wantText:       NotApprovedMessage,
			wantTargetCall: false,
		},
		{
			name:    "cancel rejects",
			timeout: DefaultElicitationTimeout,
			handler: func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
				return &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action: mcp.ElicitationResponseActionCancel,
					},
				}, nil
			},
			wantErr:        true,
			wantText:       NotApprovedMessage,
			wantTargetCall: false,
		},
		{
			name:    "timeout rejects",
			timeout: 30 * time.Millisecond,
			handler: func(ctx context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			wantErr:        true,
			wantText:       NotApprovedMessage,
			wantTargetCall: false,
		},
		{
			name:    "rpc error rejects",
			timeout: DefaultElicitationTimeout,
			handler: func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
				return nil, errors.New("elicitation transport failure")
			},
			wantErr:        true,
			wantText:       NotApprovedMessage,
			wantTargetCall: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var targetCalled atomic.Bool
			var capturedReq mcp.ElicitationRequest
			var handlerCalls atomic.Int32

			srv := newApprovalTestServer(tc.timeout, &targetCalled)
			c, err := client.NewInProcessClientWithOptions(
				srv,
				client.WithElicitationHandler(&funcElicitationHandler{
					fn: func(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
						handlerCalls.Add(1)
						capturedReq = req
						return tc.handler(ctx, req)
					},
				}),
			)
			if err != nil {
				t.Fatalf("NewInProcessClientWithOptions failed: %v", err)
			}
			t.Cleanup(func() { _ = c.Close() })

			if err := c.Start(t.Context()); err != nil {
				t.Fatalf("client Start failed: %v", err)
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
				t.Fatalf("client Initialize failed: %v", err)
			}

			res, err := c.CallTool(t.Context(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: "delete_user"},
			})
			if err != nil {
				t.Fatalf("CallTool failed: %v", err)
			}
			if handlerCalls.Load() != 1 {
				t.Fatalf("elicitation handler calls = %d, want 1", handlerCalls.Load())
			}
			if res.IsError != tc.wantErr {
				t.Fatalf("res.IsError = %t, want %t (text=%q)", res.IsError, tc.wantErr, firstTextContent(t, res))
			}
			if got := firstTextContent(t, res); got != tc.wantText {
				t.Fatalf("res text = %q, want %q", got, tc.wantText)
			}
			if targetCalled.Load() != tc.wantTargetCall {
				t.Fatalf("targetCalled = %t, want %t", targetCalled.Load(), tc.wantTargetCall)
			}
			if tc.checkRequest != nil {
				tc.checkRequest(t, capturedReq)
			}
		})
	}
}
