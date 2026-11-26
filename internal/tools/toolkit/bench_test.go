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
	"net/http"
	"testing"
	"time"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const maxToolOverhead = 50 * time.Millisecond

func newBenchHarness(tb testing.TB, fakeIS *istest.Server, caPath string) *mcpclient.Client {
	tb.Helper()

	fakeIS.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources": []any{
			map[string]any{"id": "u-1", "userName": "alice"},
		},
	})
	fakeIS.HandleJSON(http.MethodPost, "/scim2/Users", http.StatusCreated, map[string]any{
		"id":       "u-2",
		"userName": "bob",
	})

	cfg := &config.Config{
		ServerURL:    fakeIS.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeClientCredentials,
		ClientID:     "bench-client",
		ClientSecret: "bench-secret",
		CertPath:     caPath,
	}
	isClient, err := client.New(cfg)
	if err != nil {
		tb.Fatalf("client.New failed: %v", err)
	}
	if caPath == "" {
		isClient.SetHTTPClient(fakeIS.Client())
	}

	readOp := &catalog.Operation{
		ID:     "users.listUsers",
		Area:   "users",
		Family: catalog.FamilyRoot,
		Method: http.MethodGet,
		Path:   "/scim2/Users",
		Kind:   catalog.KindRead,
		Scopes: []string{"internal_user_mgt_list"},
	}
	writeOp := &catalog.Operation{
		ID:     "users.createUser",
		Area:   "users",
		Family: catalog.FamilyRoot,
		Method: http.MethodPost,
		Path:   "/scim2/Users",
		Kind:   catalog.KindWrite,
		Scopes: []string{"internal_user_mgt_create"},
		Body: &catalog.BodySpec{
			ContentType: "application/scim+json",
			Required:    true,
		},
	}

	reg := NewRegistry(false, 0)
	reg.MustRegister(
		ToolDef{
			Name:        "list_users",
			Layer:       LayerCurated,
			Kind:        catalog.KindRead,
			Scopes:      readOp.Scopes,
			Operation:   readOp,
			Description: "List users in WSO2 IS.",
			Handler: func(ctx context.Context, _ map[string]any) (any, error) {
				return isClient.Do(ctx, readOp, nil, nil, nil)
			},
		},
		ToolDef{
			Name:        "create_user",
			Layer:       LayerCurated,
			Kind:        catalog.KindWrite,
			Scopes:      writeOp.Scopes,
			Operation:   writeOp,
			Description: "Create a user in WSO2 IS.",
			SummaryFunc: func(args map[string]any) string {
				if u, ok := args["userName"].(string); ok && u != "" {
					return "create user " + u
				}
				return "create user"
			},
			Options: []mcp.ToolOption{
				mcp.WithString("userName", mcp.Required()),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				return isClient.Do(ctx, writeOp, nil, nil, map[string]any{
					"userName": args["userName"],
				})
			},
		},
	)

	srv := server.NewMCPServer("bench-server", "1.0.0", server.WithElicitation())
	reg.AddToServer(srv)

	c, err := mcpclient.NewInProcessClientWithOptions(
		srv,
		mcpclient.WithElicitationHandler(&funcElicitationHandler{
			fn: func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
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
		tb.Fatalf("NewInProcessClientWithOptions failed: %v", err)
	}
	tb.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		tb.Fatalf("client Start failed: %v", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "bench-client", Version: "1.0.0"},
			Capabilities: mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapability{},
			},
		},
	}); err != nil {
		tb.Fatalf("client Initialize failed: %v", err)
	}

	// Warm up client-credentials token once before measuring steady-state per-call overhead.
	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "list_users"},
	})
	if err != nil || res.IsError {
		tb.Fatalf("warmup CallTool failed: err=%v res=%v", err, res)
	}
	return c
}

func BenchmarkReadToolCall(b *testing.B) {
	fakeIS := istest.New(&testing.T{})
	c := newBenchHarness(b, fakeIS, "")
	ctx := context.Background()
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "list_users"},
	}

	for b.Loop() {
		res, err := c.CallTool(ctx, req)
		if err != nil {
			b.Fatalf("CallTool failed: %v", err)
		}
		if res.IsError {
			b.Fatalf("unexpected tool error")
		}
	}

	if b.N > 0 {
		perOp := b.Elapsed() / time.Duration(b.N)
		if perOp >= maxToolOverhead {
			b.Fatalf("read tool overhead %v >= %v", perOp, maxToolOverhead)
		}
	}
}

func BenchmarkWriteToolCall(b *testing.B) {
	fakeIS := istest.New(&testing.T{})
	c := newBenchHarness(b, fakeIS, "")
	ctx := context.Background()
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "create_user",
			Arguments: map[string]any{"userName": "bob"},
		},
	}

	for b.Loop() {
		res, err := c.CallTool(ctx, req)
		if err != nil {
			b.Fatalf("CallTool failed: %v", err)
		}
		if res.IsError {
			b.Fatalf("unexpected tool error")
		}
	}

	if b.N > 0 {
		perOp := b.Elapsed() / time.Duration(b.N)
		if perOp >= maxToolOverhead {
			b.Fatalf("write tool overhead %v >= %v", perOp, maxToolOverhead)
		}
	}
}

func TestToolCallOverheadBudget(t *testing.T) {
	fakeIS := istest.New(t)
	c := newBenchHarness(t, fakeIS, fakeIS.WriteCAPEM(t))

	const iterations = 10
	startRead := time.Now()
	for range iterations {
		res, err := c.CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: "list_users"},
		})
		if err != nil || res.IsError {
			t.Fatalf("read CallTool failed: err=%v isErr=%v", err, res != nil && res.IsError)
		}
	}
	avgRead := time.Since(startRead) / iterations
	if avgRead >= maxToolOverhead {
		t.Fatalf("average read overhead %v >= %v", avgRead, maxToolOverhead)
	}

	startWrite := time.Now()
	for range iterations {
		res, err := c.CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "create_user",
				Arguments: map[string]any{"userName": "bob"},
			},
		})
		if err != nil || res.IsError {
			t.Fatalf("write CallTool failed: err=%v isErr=%v", err, res != nil && res.IsError)
		}
	}
	avgWrite := time.Since(startWrite) / iterations
	if avgWrite >= maxToolOverhead {
		t.Fatalf("average write overhead %v >= %v", avgWrite, maxToolOverhead)
	}
}
