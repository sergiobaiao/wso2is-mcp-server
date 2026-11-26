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

package scim_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/scim"
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

var (
	sharedCatOnce sync.Once
	sharedCat     *catalog.Catalog
	sharedCatErr  error
)

func loadSharedCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	sharedCatOnce.Do(func() {
		sharedCat, sharedCatErr = catalog.Load(apispecs.FS, "7.3.0")
	})
	if sharedCatErr != nil {
		t.Fatalf("catalog.Load failed: %v", sharedCatErr)
	}
	return sharedCat
}

func setupTestEnv(t *testing.T, customCfg ...func(*config.Config)) *testEnv {
	t.Helper()

	srv := istest.New(t)
	cat := loadSharedCatalog(t)

	cfg := &config.Config{
		ServerURL:             srv.URL(),
		TenantDomain:          config.DefaultTenantDomain,
		AuthMode:              config.AuthModeClientCredentials,
		ClientID:              "test-client-id",
		ClientSecret:          "test-client-secret",
		CertPath:              srv.WriteCAPEM(t),
		MaxResponseBytes:      config.DefaultMaxResponseBytes,
		SCIMProfileAttributes: strings.Split(config.DefaultSCIMProfileAttributes, ","),
	}
	for _, fn := range customCfg {
		fn(cfg)
	}

	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New failed: %v", err)
	}

	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	if err := scim.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("scim.Register failed: %v", err)
	}

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
		t.Fatal("expected non-empty content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func decodeResultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, res))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("unmarshal tool output JSON: %v\nraw:\n%s", err, resultText(t, res))
	}
	return out
}

func lastAPIRequest(t *testing.T, srv *istest.Server) istest.RecordedRequest {
	t.Helper()
	reqs := srv.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Path != "/oauth2/token" && reqs[i].Path != "/oauth2/jwks" {
			return reqs[i]
		}
	}
	t.Fatal("expected at least one non-token IS request")
	return istest.RecordedRequest{}
}

func apiRequests(srv *istest.Server) []istest.RecordedRequest {
	var out []istest.RecordedRequest
	for _, r := range srv.Requests() {
		if r.Path != "/oauth2/token" && r.Path != "/oauth2/jwks" {
			out = append(out, r)
		}
	}
	return out
}

func TestListUsers(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.SCIMProfileAttributes = []string{
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department",
			"urn:scim:schemas:extension:custom:User:customerTier",
			"addresses.work",
		}
	})

	env.srv.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources": []any{
			map[string]any{"id": "u-1", "userName": "PRIMARY/emma"},
		},
	})

	res := env.callTool(t, "list_users", map[string]any{"count": 25})
	out := decodeResultJSON(t, res)

	if out["totalResults"] != float64(1) {
		t.Fatalf("totalResults = %v, want 1", out["totalResults"])
	}
	users, ok := out["users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("users = %#v, want 1 user", out["users"])
	}

	req := lastAPIRequest(t, env.srv)
	if !strings.Contains(req.RawQuery, "startIndex=1") || !strings.Contains(req.RawQuery, "count=25") {
		t.Fatalf("unexpected query: %q", req.RawQuery)
	}
	if !strings.Contains(req.RawQuery, "userName") || !strings.Contains(req.RawQuery, "urn%3Ascim%3Aschemas%3Aextension%3Acustom%3AUser") {
		t.Fatalf("expected compact attributes with custom schema in query, got %q", req.RawQuery)
	}
}

func TestSearchUsers(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodPost, "/scim2/Users/.search", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources": []any{
			map[string]any{"id": "u-2", "userName": "PRIMARY/alice"},
		},
	})

	res := env.callTool(t, "search_users", map[string]any{
		"filter": "emails co example.com",
		"count":  15,
	})
	out := decodeResultJSON(t, res)

	if out["totalResults"] != float64(1) {
		t.Fatalf("totalResults = %v, want 1", out["totalResults"])
	}
	users, ok := out["users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("users = %#v", out["users"])
	}

	req := lastAPIRequest(t, env.srv)
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body["filter"] != "emails co example.com" {
		t.Fatalf("filter = %v", body["filter"])
	}
	if body["count"] != float64(15) || body["startIndex"] != float64(1) {
		t.Fatalf("unexpected paging in search body: %#v", body)
	}
	attrs, ok := body["attributes"].([]any)
	if !ok || len(attrs) == 0 {
		t.Fatalf("expected compact attributes array in search body, got %#v", body["attributes"])
	}
}

func TestGetUser(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/scim2/Users/u-123", http.StatusOK, map[string]any{
		"id":       "u-123",
		"userName": "PRIMARY/kim",
	})

	res := env.callTool(t, "get_user", map[string]any{"userId": "u-123"})
	out := decodeResultJSON(t, res)

	u, ok := out["user"].(map[string]any)
	if !ok || u["id"] != "u-123" {
		t.Fatalf("user = %#v, want id u-123", out["user"])
	}
}

func TestCreateUser(t *testing.T) {
	t.Run("SCIM inputs and password never echoed", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.HandleJSON(http.MethodPost, "/scim2/Users", http.StatusCreated, map[string]any{
			"id":       "u-new-1",
			"userName": "PRIMARY/emma",
			"password": "SuperSecretPassword123!",
		})

		res := env.callTool(t, "create_user", map[string]any{
			"userName":   "emma",
			"password":   "SuperSecretPassword123!",
			"givenName":  "Emma",
			"familyName": "Clarke",
			"email":      "emma@example.com",
		})
		text := resultText(t, res)
		if strings.Contains(text, "SuperSecretPassword123!") || strings.Contains(text, "password") {
			t.Fatalf("password must never be echoed in create_user output: %s", text)
		}
		out := decodeResultJSON(t, res)
		if out["id"] != "u-new-1" || out["userName"] != "PRIMARY/emma" {
			t.Fatalf("unexpected output: %#v", out)
		}

		req := lastAPIRequest(t, env.srv)
		var sent map[string]any
		if err := json.Unmarshal(req.Body, &sent); err != nil {
			t.Fatalf("unmarshal sent body: %v", err)
		}
		if sent["userName"] != "PRIMARY/emma" {
			t.Fatalf("sent userName = %v, want PRIMARY/emma", sent["userName"])
		}
	})

	t.Run("legacy admin inputs with custom userstore_domain", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.HandleJSON(http.MethodPost, "/scim2/Users", http.StatusCreated, map[string]any{
			"id":       "u-new-2",
			"userName": "SECONDARY/bob",
		})

		res := env.callTool(t, "create_user", map[string]any{
			"username":         "bob",
			"password":         "Pass!1234",
			"first_name":       "Bob",
			"last_name":        "Smith",
			"email":            "bob@example.com",
			"userstore_domain": "SECONDARY",
		})
		out := decodeResultJSON(t, res)
		if out["id"] != "u-new-2" || out["userName"] != "SECONDARY/bob" {
			t.Fatalf("unexpected output: %#v", out)
		}

		req := lastAPIRequest(t, env.srv)
		var sent map[string]any
		if err := json.Unmarshal(req.Body, &sent); err != nil {
			t.Fatalf("unmarshal sent body: %v", err)
		}
		if sent["userName"] != "SECONDARY/bob" {
			t.Fatalf("sent userName = %v, want SECONDARY/bob", sent["userName"])
		}
		nameObj, _ := sent["name"].(map[string]any)
		if nameObj["givenName"] != "Bob" || nameObj["familyName"] != "Smith" {
			t.Fatalf("sent name = %#v", nameObj)
		}
	})

	t.Run("conflicting spellings return input error without calling IS", func(t *testing.T) {
		env := setupTestEnv(t)

		res := env.callTool(t, "create_user", map[string]any{
			"userName":   "emma",
			"username":   "other",
			"password":   "Pass!1234",
			"givenName":  "Emma",
			"familyName": "Clarke",
			"email":      "emma@example.com",
		})
		if !res.IsError {
			t.Fatal("expected error for conflicting userName/username")
		}
		if !strings.Contains(resultText(t, res), "invalid input") {
			t.Fatalf("expected invalid input message, got %q", resultText(t, res))
		}
		if got := len(apiRequests(env.srv)); got != 0 {
			t.Fatalf("expected 0 IS requests on validation error, got %d", got)
		}
	})
}

func TestUpdateUserAttribute(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/Users/u-1", http.StatusOK, map[string]any{
		"id":       "u-1",
		"userName": "PRIMARY/emma",
	})

	res := env.callTool(t, "update_user_attribute", map[string]any{
		"userId":    "u-1",
		"attribute": "name.givenName",
		"value":     "Eleanor",
	})
	out := decodeResultJSON(t, res)
	if out["id"] != "u-1" || out["userName"] != "PRIMARY/emma" {
		t.Fatalf("unexpected output: %#v", out)
	}

	req := lastAPIRequest(t, env.srv)
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("unmarshal PATCH body: %v", err)
	}
	ops, _ := body["Operations"].([]any)
	if len(ops) != 1 {
		t.Fatalf("expected 1 PATCH operation, got %#v", body["Operations"])
	}
	op0, _ := ops[0].(map[string]any)
	if op0["op"] != "replace" || op0["path"] != "name.givenName" || op0["value"] != "Eleanor" {
		t.Fatalf("unexpected PATCH op: %#v", op0)
	}
}

func TestUpdateUserProfile(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.SCIMProfileAttributes = []string{
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:costCenter",
			"urn:scim:schemas:extension:custom:User:region",
			"urn:scim:schemas:extension:custom:User:customerTier",
			"urn:scim:schemas:extension:custom:User:contractEndDate",
			"addresses.work",
		}
	})

	env.srv.HandleJSON(http.MethodPatch, "/scim2/Users/u-1", http.StatusOK, map[string]any{
		"id":       "u-1",
		"userName": "PRIMARY/emma",
		"addresses": []any{
			map[string]any{"type": "work", "formatted": "100 Main St"},
		},
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": map[string]any{
			"department":     "Engineering",
			"employeeNumber": "E-100",
			"costCenter":     "CC-42",
		},
		"urn:scim:schemas:extension:custom:User": map[string]any{
			"region":          "EU",
			"customerTier":    "gold",
			"contractEndDate": "2027-12-31",
		},
	})

	res := env.callTool(t, "update_user_profile", map[string]any{
		"userId":          "u-1",
		"department":      "Engineering",
		"employeeNumber":  "E-100",
		"costCenter":      "CC-42",
		"region":          "EU",
		"customerTier":    "gold",
		"contractEndDate": "2027-12-31",
		"work":            "100 Main St",
	})
	out := decodeResultJSON(t, res)
	if out["id"] != "u-1" || out["userName"] != "PRIMARY/emma" {
		t.Fatalf("unexpected id/userName: %#v", out)
	}
	if out["enterprise"] == nil || out["custom"] == nil || out["addresses"] == nil {
		t.Fatalf("expected addresses, enterprise, custom in output: %#v", out)
	}
}

func TestDeleteUser(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodDelete, "/scim2/Users/u-del", http.StatusNoContent, nil)

	res := env.callTool(t, "delete_user", map[string]any{"userId": "u-del"})
	out := decodeResultJSON(t, res)
	if out["status"] != "deleted" {
		t.Fatalf("status = %v, want deleted", out["status"])
	}
}

func TestPatchUser(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/Users/u-p1", http.StatusOK, map[string]any{
		"id":       "u-p1",
		"userName": "PRIMARY/kim",
		"nickName": "k",
	})

	res := env.callTool(t, "patch_user", map[string]any{
		"userId": "u-p1",
		"operations": []any{
			map[string]any{
				"op":    "replace",
				"path":  "nickName",
				"value": "k",
			},
		},
	})
	out := decodeResultJSON(t, res)
	u, _ := out["user"].(map[string]any)
	if u["nickName"] != "k" {
		t.Fatalf("unexpected patch_user output: %#v", out)
	}
}

func TestUserSCIMErrorMapping(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPost, "/scim2/Users", http.StatusConflict, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
		"status":  "409",
		"detail":  "User PRIMARY/emma already exists in the system.",
	})

	res := env.callTool(t, "create_user", map[string]any{
		"userName":   "emma",
		"password":   "Pass!1234",
		"givenName":  "Emma",
		"familyName": "Clarke",
		"email":      "emma@example.com",
	})
	if !res.IsError {
		t.Fatal("expected IsError=true on 409")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "request failed with status 409: it already exists or conflicts with the current state") {
		t.Fatalf("missing tool-conventions actionable error text: %s", text)
	}
	if !strings.Contains(text, `"error"`) || !strings.Contains(text, `"errorStatus"`) || !strings.Contains(text, "409") {
		t.Fatalf("missing SCIM error/errorStatus fields in error output: %s", text)
	}
}

func TestMissingCatalogOperationsFails(t *testing.T) {
	srv := istest.New(t)
	cfg := &config.Config{
		ServerURL:    srv.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeClientCredentials,
		ClientID:     "cid",
		ClientSecret: "csec",
		CertPath:     srv.WriteCAPEM(t),
	}
	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New failed: %v", err)
	}

	emptyCat := &catalog.Catalog{}
	if _, err := scim.Tools(emptyCat, cl, cfg); err == nil {
		t.Fatal("expected error when catalog operations are missing")
	}
}
