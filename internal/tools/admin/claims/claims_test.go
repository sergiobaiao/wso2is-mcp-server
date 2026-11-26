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

package claims_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/claims"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type testEnv struct {
	srv    *istest.Server
	cat    *catalog.Catalog
	cl     *client.Client
	cfg    *config.Config
	reg    *toolkit.Registry
	mcpCli *mcpclient.Client
}

func setupTestEnv(t *testing.T, customCfg ...func(*config.Config)) *testEnv {
	t.Helper()

	srv := istest.New(t)
	cat, err := catalog.Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("catalog.Load failed: %v", err)
	}

	cfg := &config.Config{
		ServerURL:        srv.URL(),
		TenantDomain:     config.DefaultTenantDomain,
		AuthMode:         config.AuthModeClientCredentials,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		CertPath:         srv.WriteCAPEM(t),
		MaxResponseBytes: config.DefaultMaxResponseBytes,
	}
	for _, fn := range customCfg {
		fn(cfg)
	}

	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New failed: %v", err)
	}

	defs, err := claims.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("claims.Tools failed: %v", err)
	}

	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	reg.MustRegister(defs...)

	mcpSrv := server.NewMCPServer("wso2is-mcp-test", "1.0.0")
	reg.AddToServer(mcpSrv)

	mcpCli, err := mcpclient.NewInProcessClient(mcpSrv)
	if err != nil {
		t.Fatalf("NewInProcessClient failed: %v", err)
	}
	t.Cleanup(func() { _ = mcpCli.Close() })

	if err := mcpCli.Start(context.Background()); err != nil {
		t.Fatalf("mcpCli.Start failed: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test-client", Version: "1.0.0"}
	if _, err := mcpCli.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("mcpCli.Initialize failed: %v", err)
	}

	return &testEnv{
		srv:    srv,
		cat:    cat,
		cl:     cl,
		cfg:    cfg,
		reg:    reg,
		mcpCli: mcpCli,
	}
}

func (e *testEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := e.mcpCli.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("CallTool(%q) protocol error: %v", name, err)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("empty tool result content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func decodeResultArray(t *testing.T, res *mcp.CallToolResult) []any {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, res))
	}
	var out []any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("unmarshal result JSON array: %v\nraw: %s", err, resultText(t, res))
	}
	return out
}

func apiRequests(srv *istest.Server) []istest.RecordedRequest {
	var out []istest.RecordedRequest
	for _, r := range srv.Requests() {
		if strings.HasSuffix(r.Path, "/oauth2/token") || strings.HasSuffix(r.Path, "/oauth2/jwks") {
			continue
		}
		out = append(out, r)
	}
	return out
}

func lastAPIRequest(t *testing.T, srv *istest.Server) istest.RecordedRequest {
	t.Helper()
	reqs := apiRequests(srv)
	if len(reqs) == 0 {
		t.Fatal("expected at least one IS API request")
	}
	return reqs[len(reqs)-1]
}

func TestClaimsToolsRegistrationAndCatalogLookup(t *testing.T) {
	t.Run("missing catalog operation fails", func(t *testing.T) {
		emptyFS := fstest.MapFS{
			"wso2is/7.3.0/SOURCE":     &fstest.MapFile{Data: []byte("repository: test\ncommit: abc\n")},
			"wso2is/7.3.0/root/.keep": &fstest.MapFile{Data: []byte("")},
			"wso2is/7.3.0/org/.keep":  &fstest.MapFile{Data: []byte("")},
		}
		emptyCat, err := catalog.Load(fs.FS(emptyFS), "7.3.0")
		if err != nil {
			t.Fatalf("catalog.Load empty failed: %v", err)
		}
		srv := istest.New(t)
		cfg := &config.Config{ServerURL: srv.URL(), TenantDomain: config.DefaultTenantDomain}
		cl, err := client.New(cfg)
		if err != nil {
			t.Fatalf("client.New failed: %v", err)
		}
		if _, err := claims.Tools(emptyCat, cl, cfg); err == nil {
			t.Fatal("expected error when catalog operations are missing")
		}
	})

	t.Run("registers list_claims and create_local_claim", func(t *testing.T) {
		env := setupTestEnv(t)
		got := env.reg.Names()
		slices.Sort(got)
		want := []string{"create_local_claim", "list_claims"}
		if !slices.Equal(got, want) {
			t.Fatalf("registered tools = %v, want %v", got, want)
		}

		for _, st := range env.reg.ServerTools() {
			ann := st.Tool.Annotations
			isRead := ann.ReadOnlyHint != nil && *ann.ReadOnlyHint
			isDestr := ann.DestructiveHint != nil && *ann.DestructiveHint
			switch st.Tool.Name {
			case "list_claims":
				if !isRead || isDestr {
					t.Errorf("list_claims annotations mismatch: read=%v destr=%v", isRead, isDestr)
				}
			case "create_local_claim":
				if isRead || isDestr {
					t.Errorf("create_local_claim annotations mismatch: read=%v destr=%v", isRead, isDestr)
				}
			}
		}
	})

	t.Run("read_only mode registers only list_claims", func(t *testing.T) {
		env := setupTestEnv(t, func(cfg *config.Config) {
			cfg.ReadOnly = true
		})
		got := env.reg.Names()
		if !slices.Equal(got, []string{"list_claims"}) {
			t.Fatalf("READ_ONLY tools = %v, want [list_claims]", got)
		}
	})
}

func TestListClaimsAndCreateLocalClaim(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("list_claims root", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/claim-dialects/local/claims", http.StatusOK, []any{
			map[string]any{
				"id":          "c-1",
				"claimURI":    "http://wso2.org/claims/username",
				"displayName": "Username",
			},
		})
		res := env.callTool(t, "list_claims", nil)
		arr := decodeResultArray(t, res)
		if len(arr) != 1 || arr[0].(map[string]any)["claimURI"] != "http://wso2.org/claims/username" {
			t.Fatalf("unexpected list_claims output: %#v", arr)
		}
		req := lastAPIRequest(t, env.srv)
		if !strings.Contains(req.RawQuery, "exclude-hidden-claims=true") {
			t.Fatalf("expected exclude-hidden-claims=true, got %q", req.RawQuery)
		}
	})

	t.Run("list_claims org", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/claim-dialects/local/claims", http.StatusOK, []any{
			map[string]any{"id": "oc-1", "claimURI": "http://wso2.org/claims/orgattr"},
		})
		res := env.callTool(t, "list_claims", map[string]any{"organization_id": "org-1"})
		arr := decodeResultArray(t, res)
		if len(arr) != 1 || arr[0].(map[string]any)["id"] != "oc-1" {
			t.Fatalf("unexpected org list_claims output: %#v", arr)
		}
		req := lastAPIRequest(t, env.srv)
		if req.Path != "/o/api/server/v1/claim-dialects/local/claims" {
			t.Fatalf("expected /o/ path, got %s", req.Path)
		}
	})

	t.Run("create_local_claim root and org", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodPost, "/api/server/v1/claim-dialects/local/claims", http.StatusCreated, nil)
		res := env.callTool(t, "create_local_claim", map[string]any{
			"claim_uri":    "http://wso2.org/claims/tier",
			"display_name": "Customer Tier",
			"description":  "Customer subscription tier",
			"attribute_mapping": []any{
				map[string]any{"mappedAttribute": "tier", "userstore": "PRIMARY"},
			},
			"supported_by_default": true,
			"data_type":            "string",
		})
		if res.IsError {
			t.Fatalf("create_local_claim failed: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal claim body: %v", err)
		}
		if body["claimURI"] != "http://wso2.org/claims/tier" || body["displayName"] != "Customer Tier" {
			t.Fatalf("unexpected claim body: %#v", body)
		}

		env.srv.HandleJSON(http.MethodPost, "/o/api/server/v1/claim-dialects/local/claims", http.StatusCreated, nil)
		orgRes := env.callTool(t, "create_local_claim", map[string]any{
			"claim_uri":       "http://wso2.org/claims/orgtier",
			"display_name":    "Org Tier",
			"description":     "Org tier",
			"organization_id": "org-1",
			"attribute_mapping": []any{
				map[string]any{"mappedAttribute": "orgTier", "userstore": "PRIMARY"},
			},
		})
		if orgRes.IsError {
			t.Fatalf("org create_local_claim failed: %s", resultText(t, orgRes))
		}
		orgReq := lastAPIRequest(t, env.srv)
		if orgReq.Path != "/o/api/server/v1/claim-dialects/local/claims" {
			t.Fatalf("expected /o/ path, got %s", orgReq.Path)
		}
	})
}
