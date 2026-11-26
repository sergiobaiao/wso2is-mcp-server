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
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
)

func newTestJWTConfig(is *istest.Server) *config.Config {
	return &config.Config{
		ServerURL:    is.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeBearer,
		Issuer:       is.URL() + "/oauth2/token",
		JWKSURL:      is.URL() + "/oauth2/jwks",
		Resource:     "https://mcp.example.com/mcp",
		PublicURL:    "https://mcp.example.com",
		JWTClockSkew: 60 * time.Second,
	}
}

func signWithKey(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	headerJSON, _ := json.Marshal(map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": kid,
	})
	claimsJSON, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func assertNoISResourceCalls(t *testing.T, is *istest.Server) {
	t.Helper()
	for _, req := range is.Requests() {
		if req.Path != "/oauth2/jwks" && !strings.HasSuffix(req.Path, "/oauth2/jwks") {
			t.Fatalf("unexpected IS call on rejected token: %s %s", req.Method, req.Path)
		}
	}
}

func TestJWTMiddleware_MissingOrNonBearer(t *testing.T) {
	t.Parallel()

	is := istest.New(t)
	cfg := newTestJWTConfig(is)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logBuf bytes.Buffer
	v, err := NewJWTValidator(ctx, cfg, WithHTTPClient(is.Client()), WithLogger(log.New(&logBuf, "", 0)))
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}
	defer func() { _ = v.Close() }()

	var nextCalled atomic.Bool
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	wantWWWAuth := `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource"`

	for _, tc := range []struct {
		name string
		auth string
	}{
		{name: "missing header", auth: ""},
		{name: "basic auth", auth: "Basic dXNlcjpwYXNz"},
		{name: "empty bearer", auth: "Bearer   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled.Store(false)
			is.ResetRequests()

			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"secret_body":"ignore"}`))
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d, want 401", rr.Code)
			}
			if got := rr.Header().Get("WWW-Authenticate"); got != wantWWWAuth {
				t.Fatalf("WWW-Authenticate=%q, want %q", got, wantWWWAuth)
			}
			if nextCalled.Load() {
				t.Fatal("next handler should not be called on 401")
			}
			assertNoISResourceCalls(t, is)
		})
	}
}

func TestJWTMiddleware_InvalidTokens(t *testing.T) {
	t.Parallel()

	is := istest.New(t)
	cfg := newTestJWTConfig(is)
	now := time.Unix(1700000000, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logBuf bytes.Buffer
	v, err := NewJWTValidator(ctx, cfg,
		WithHTTPClient(is.Client()),
		WithLogger(log.New(&logBuf, "", 0)),
		withNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}
	defer func() { _ = v.Close() }()

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate other RSA key: %v", err)
	}

	validClaims := func() map[string]any {
		return map[string]any{
			"iss": cfg.Issuer,
			"aud": []string{"client-1", cfg.Resource},
			"sub": "alice",
			"iat": now.Add(-10 * time.Second).Unix(),
			"nbf": now.Add(-10 * time.Second).Unix(),
			"exp": now.Add(5 * time.Minute).Unix(),
		}
	}

	wrongIss := validClaims()
	wrongIss["iss"] = "https://evil.example.com/oauth2/token"

	wrongAud := validClaims()
	wrongAud["aud"] = []string{"some-other-resource"}

	expired := validClaims()
	expired["exp"] = now.Add(-65 * time.Second).Unix()

	notYetValid := validClaims()
	notYetValid["nbf"] = now.Add(65 * time.Second).Unix()

	cases := []struct {
		name  string
		token string
	}{
		{name: "non-JWT string", token: "opaque-secret-token-should-never-leak"},
		{name: "two-segment JWT", token: "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhbGljZSJ9"},
		{name: "bad signature wrong key", token: signWithKey(t, otherKey, "test-key-1", validClaims())},
		{name: "wrong issuer", token: is.SignJWT(wrongIss)},
		{name: "wrong audience", token: is.SignJWT(wrongAud)},
		{name: "expired beyond skew", token: is.SignJWT(expired)},
		{name: "not yet valid beyond skew", token: is.SignJWT(notYetValid)},
	}

	var nextCalled atomic.Bool
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled.Store(false)
			is.ResetRequests()
			logBuf.Reset()

			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"body":"sensitive"}`))
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d, want 401 (body=%s)", rr.Code, rr.Body.String())
			}
			wwwAuth := rr.Header().Get("WWW-Authenticate")
			if !strings.HasPrefix(wwwAuth, "Bearer ") ||
				!strings.Contains(wwwAuth, `error="invalid_token"`) ||
				!strings.Contains(wwwAuth, `error_description="`) ||
				!strings.Contains(wwwAuth, `resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource"`) {
				t.Fatalf("unexpected WWW-Authenticate header: %q", wwwAuth)
			}
			if strings.Contains(wwwAuth, tc.token) {
				t.Fatalf("WWW-Authenticate leaked raw token")
			}
			if strings.Contains(rr.Body.String(), tc.token) {
				t.Fatalf("response body leaked raw token")
			}
			if strings.Contains(logBuf.String(), tc.token) {
				t.Fatalf("access log leaked raw token")
			}
			if strings.Contains(logBuf.String(), "sensitive") {
				t.Fatalf("access log leaked request body")
			}
			if nextCalled.Load() {
				t.Fatal("next handler should not be called on 401")
			}
			assertNoISResourceCalls(t, is)
		})
	}
}

func TestJWTMiddleware_ValidTokenSkewContextAndAccessLog(t *testing.T) {
	t.Parallel()

	is := istest.New(t)
	cfg := newTestJWTConfig(is)
	cfg.CertPath = is.WriteCAPEM(t)

	now := time.Unix(1700000000, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logBuf bytes.Buffer
	// Construct without WithHTTPClient to verify CERT_PATH TLS trust works.
	v, err := NewJWTValidator(ctx, cfg,
		WithLogger(log.New(&logBuf, "", 0)),
		withNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewJWTValidator with CERT_PATH: %v", err)
	}
	defer func() { _ = v.Close() }()

	// Exp 30s in past, nbf 30s in future — both within 60s skew.
	tok := is.SignJWT(map[string]any{
		"iss": cfg.Issuer,
		"aud": []string{"client-app-1", cfg.Resource},
		"sub": "alice-sub-42",
		"iat": now.Add(-30 * time.Second).Unix(),
		"nbf": now.Add(30 * time.Second).Unix(),
		"exp": now.Add(-30 * time.Second).Unix(),
	})

	var gotCC client.CredentialContext
	var gotOK bool
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCC, gotOK = client.CredentialContextFrom(r.Context())
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "ok")
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","sensitive":"payload"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201 (www-auth=%q body=%s)", rr.Code, rr.Header().Get("WWW-Authenticate"), rr.Body.String())
	}
	if !gotOK {
		t.Fatal("CredentialContext missing from request context")
	}
	if gotCC.Mode != config.AuthModeBearer || gotCC.BearerToken != tok || gotCC.Subject != "alice-sub-42" {
		t.Fatalf("unexpected CredentialContext: %+v", gotCC)
	}

	logLine := logBuf.String()
	for _, want := range []string{"POST", "/mcp", "201", "alice-sub-42"} {
		if !strings.Contains(logLine, want) {
			t.Errorf("access log %q missing %q", logLine, want)
		}
	}
	if strings.Contains(logLine, tok) {
		t.Fatalf("access log leaked raw token: %s", logLine)
	}
	if strings.Contains(logLine, "payload") || strings.Contains(logLine, "sensitive") {
		t.Fatalf("access log leaked request body: %s", logLine)
	}
}

func TestJWTMiddleware_UnknownKidRefreshRateLimit(t *testing.T) {
	t.Parallel()

	is := istest.New(t)
	cfg := newTestJWTConfig(is)
	now := time.Unix(1700000000, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logBuf bytes.Buffer
	v, err := NewJWTValidator(ctx, cfg,
		WithHTTPClient(is.Client()),
		WithLogger(log.New(&logBuf, "", 0)),
		withNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}
	defer func() { _ = v.Close() }()

	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeClaims := func() map[string]any {
		return map[string]any{
			"iss": cfg.Issuer,
			"aud": cfg.Resource,
			"sub": "bob",
			"iat": now.Unix(),
			"exp": now.Add(10 * time.Minute).Unix(),
		}
	}

	send := func(tok string) int {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tok))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	// 1. Initial key works without extra JWKS fetch.
	if code := send(is.SignJWT(makeClaims())); code != http.StatusOK {
		t.Fatalf("initial key status=%d, want 200", code)
	}
	if got := is.JWKSFetches(); got != 1 {
		t.Fatalf("initial JWKS fetches=%d, want 1", got)
	}

	// 2. Rotate key -> unknown kid triggers 1 refresh and succeeds.
	is.RotateKey(t, "test-key-2")
	if code := send(is.SignJWT(makeClaims())); code != http.StatusOK {
		t.Fatalf("rotated key 2 status=%d, want 200", code)
	}
	if got := is.JWKSFetches(); got != 2 {
		t.Fatalf("JWKS fetches after key-2 rotation=%d, want 2", got)
	}

	// 3. Rotate key again immediately (< 1m) -> rate-limited, no JWKS fetch, 401.
	is.RotateKey(t, "test-key-3")
	tok3 := is.SignJWT(makeClaims())
	if code := send(tok3); code != http.StatusUnauthorized {
		t.Fatalf("rotated key 3 within 1m status=%d, want 401", code)
	}
	if got := is.JWKSFetches(); got != 2 {
		t.Fatalf("JWKS fetches should remain 2 due to 1/min rate limit, got %d", got)
	}

	// 4. Advance clock past 1 minute -> unknown kid triggers refresh and succeeds.
	now = now.Add(61 * time.Second)
	if code := send(tok3); code != http.StatusOK {
		t.Fatalf("rotated key 3 after 61s status=%d, want 200", code)
	}
	if got := is.JWKSFetches(); got != 3 {
		t.Fatalf("JWKS fetches after 61s=%d, want 3", got)
	}
}
