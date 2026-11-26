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

package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type tokenCtxKey struct{}

func newTestMCPServer() *server.MCPServer {
	s := server.NewMCPServer("test-mcp", "1.0.0", server.WithToolCapabilities(true))
	s.AddTool(
		mcp.NewTool("whoami", mcp.WithDescription("Returns the bearer token from context.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tok, _ := ctx.Value(tokenCtxKey{}).(string)
			return mcp.NewToolResultText(tok), nil
		},
	)
	return s
}

func postMCP(t *testing.T, client *http.Client, url string, headers map[string]string, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

const initReqBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

const listToolsReqBody = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newTestMCPServer(), HTTPOptions{
		Middleware: RequireAuthorization,
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", string(body))
	}
}

func TestMCPStatefulDefaultAndStateless(t *testing.T) {
	t.Run("stateful by default", func(t *testing.T) {
		srv := httptest.NewServer(NewHandler(newTestMCPServer(), HTTPOptions{}))
		defer srv.Close()

		initResp := postMCP(t, srv.Client(), srv.URL+"/mcp", nil, initReqBody)
		defer initResp.Body.Close()
		if initResp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(initResp.Body)
			t.Fatalf("initialize status=%d body=%s", initResp.StatusCode, b)
		}
		sessionID := initResp.Header.Get(server.HeaderKeySessionID)
		if sessionID == "" {
			t.Fatalf("expected %s header in stateful mode", server.HeaderKeySessionID)
		}

		// Subsequent call with session ID succeeds.
		listResp := postMCP(t, srv.Client(), srv.URL+"/mcp", map[string]string{
			server.HeaderKeySessionID: sessionID,
		}, listToolsReqBody)
		defer listResp.Body.Close()
		if listResp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(listResp.Body)
			t.Fatalf("tools/list with session status=%d body=%s", listResp.StatusCode, b)
		}

		// Subsequent call without session ID is rejected.
		noSessResp := postMCP(t, srv.Client(), srv.URL+"/mcp", nil, listToolsReqBody)
		defer noSessResp.Body.Close()
		if noSessResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 without session id, got %d", noSessResp.StatusCode)
		}

		// DELETE terminates the session; subsequent use fails.
		delReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/mcp", nil)
		if err != nil {
			t.Fatalf("new DELETE request: %v", err)
		}
		delReq.Header.Set(server.HeaderKeySessionID, sessionID)
		delResp, err := srv.Client().Do(delReq)
		if err != nil {
			t.Fatalf("DELETE /mcp: %v", err)
		}
		delResp.Body.Close()
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("DELETE /mcp status=%d", delResp.StatusCode)
		}

		afterDelResp := postMCP(t, srv.Client(), srv.URL+"/mcp", map[string]string{
			server.HeaderKeySessionID: sessionID,
		}, listToolsReqBody)
		defer afterDelResp.Body.Close()
		if afterDelResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after DELETE, got %d", afterDelResp.StatusCode)
		}
	})

	t.Run("stateless when Stateless is true", func(t *testing.T) {
		srv := httptest.NewServer(NewHandler(newTestMCPServer(), HTTPOptions{
			Stateless: true,
		}))
		defer srv.Close()

		initResp := postMCP(t, srv.Client(), srv.URL+"/mcp", nil, initReqBody)
		defer initResp.Body.Close()
		if initResp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(initResp.Body)
			t.Fatalf("initialize status=%d body=%s", initResp.StatusCode, b)
		}
		if got := initResp.Header.Get(server.HeaderKeySessionID); got != "" {
			t.Fatalf("expected no %s header in stateless mode, got %q", server.HeaderKeySessionID, got)
		}

		listResp := postMCP(t, srv.Client(), srv.URL+"/mcp", nil, listToolsReqBody)
		defer listResp.Body.Close()
		if listResp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(listResp.Body)
			t.Fatalf("tools/list without session in stateless mode status=%d body=%s", listResp.StatusCode, b)
		}
	})
}

func TestRequireAuthorization(t *testing.T) {
	var logBuf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(origOut) })

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("allowed"))
	})
	handler := RequireAuthorization(next)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "missing header", authHeader: "", wantStatus: http.StatusUnauthorized},
		{name: "basic scheme", authHeader: "Basic dXNlcjpwYXNz", wantStatus: http.StatusUnauthorized},
		{name: "empty bearer", authHeader: "Bearer   ", wantStatus: http.StatusUnauthorized},
		{name: "valid bearer", authHeader: "Bearer super-secret-token-value", wantStatus: http.StatusOK},
		{name: "lowercase bearer", authHeader: "bearer super-secret-token-value", wantStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("got status %d, want %d", rr.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusUnauthorized {
				if got := rr.Header().Get("WWW-Authenticate"); got != "Bearer" {
					t.Fatalf("got WWW-Authenticate=%q, want %q", got, "Bearer")
				}
			}
		})
	}

	if strings.Contains(logBuf.String(), "super-secret-token-value") {
		t.Fatalf("RequireAuthorization leaked token to logs: %s", logBuf.String())
	}
}

func TestAuthorizationForwardedToContext(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newTestMCPServer(), HTTPOptions{
		Stateless:  true,
		Middleware: RequireAuthorization,
		ContextFunc: func(ctx context.Context, r *http.Request) context.Context {
			_, tok, _ := strings.Cut(r.Header.Get("Authorization"), " ")
			return context.WithValue(ctx, tokenCtxKey{}, strings.TrimSpace(tok))
		},
	}))
	defer srv.Close()

	// Unauthenticated request is rejected before reaching MCP server.
	unauth := postMCP(t, srv.Client(), srv.URL+"/mcp", nil, listToolsReqBody)
	unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauth.StatusCode)
	}

	// Authenticated tool call receives bearer token via context.
	callReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"whoami","arguments":{}}}`
	resp := postMCP(t, srv.Client(), srv.URL+"/mcp", map[string]string{
		"Authorization": "Bearer forwarded-token-xyz",
	}, callReq)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("tools/call status=%d body=%s", resp.StatusCode, b)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "forwarded-token-xyz") {
		t.Fatalf("expected forwarded token in tool result, got %s", string(body))
	}
}

func TestExtraRoutes(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newTestMCPServer(), HTTPOptions{
		Middleware: RequireAuthorization,
		ExtraRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"resource":"https://mcp.example.com"}`))
			})
		},
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatalf("GET extra route: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHTTPServerReadHeaderTimeoutAndGracefulShutdown(t *testing.T) {
	srv := NewHTTPServer(newTestMCPServer(), HTTPOptions{Port: "9090"})
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("expected ReadHeaderTimeout=10s, got %v", srv.ReadHeaderTimeout)
	}
	if srv.Addr != ":9090" {
		t.Fatalf("expected Addr=:9090, got %q", srv.Addr)
	}

	defaultSrv := NewHTTPServer(newTestMCPServer(), HTTPOptions{})
	if defaultSrv.Addr != ":8000" {
		t.Fatalf("expected default Addr=:8000, got %q", defaultSrv.Addr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeHTTP(ctx, newTestMCPServer(), HTTPOptions{
			ListenAddr: "127.0.0.1:0",
		})
	}()

	// Give ServeHTTP a moment to start listening, then trigger graceful shutdown.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeHTTP returned error on graceful shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ServeHTTP did not shut down within 3s")
	}
}

func TestServeStdioStreams(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdioStreams(ctx, newTestMCPServer(), inR, outW)
	}()

	if _, err := io.WriteString(inW, initReqBody+"\n"); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}

	lineCh := make(chan string, 1)
	readErrCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(outR).ReadString('\n')
		if err != nil {
			readErrCh <- err
			return
		}
		lineCh <- line
	}()

	select {
	case line := <-lineCh:
		var rpcResp struct {
			ID     int            `json:"id"`
			Result map[string]any `json:"result"`
			Error  any            `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &rpcResp); err != nil {
			t.Fatalf("unmarshal stdio response %q: %v", line, err)
		}
		if rpcResp.Error != nil {
			t.Fatalf("unexpected stdio JSON-RPC error: %v", rpcResp.Error)
		}
		if rpcResp.Result == nil {
			t.Fatalf("expected initialize result, got nil")
		}
	case err := <-readErrCh:
		t.Fatalf("read stdio response: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stdio initialize response")
	}

	_ = inW.Close()
	cancel()
	_ = outR.Close()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("ServeStdioStreams returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ServeStdioStreams did not exit cleanly")
	}
}
