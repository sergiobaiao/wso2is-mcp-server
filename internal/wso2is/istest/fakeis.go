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

// Package istest provides an httptest-backed fake WSO2 Identity Server for unit tests.
package istest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// RecordedRequest captures an HTTP request received by the fake IS.
type RecordedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Headers  http.Header
	Body     []byte
}

// Server is an httptest.NewTLSServer simulating WSO2 IS endpoints.
type Server struct {
	srv *httptest.Server

	mu            sync.Mutex
	routes        map[string]http.HandlerFunc
	fallback      http.HandlerFunc
	requests      []RecordedRequest
	rejectedOrgs  map[string]bool
	tokenCounter  int
	key           *rsa.PrivateKey
	kid           string
	jwksFetches   int
	tokenHandlers map[string]http.HandlerFunc
}

// New starts a TLS fake IS and registers cleanup on t.
func New(t *testing.T) *Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	s := &Server{
		routes:        make(map[string]http.HandlerFunc),
		rejectedOrgs:  make(map[string]bool),
		tokenHandlers: make(map[string]http.HandlerFunc),
		key:           key,
		kid:           "test-key-1",
	}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(s.srv.Close)
	return s
}

// URL returns the base URL of the fake IS server.
func (s *Server) URL() string {
	return s.srv.URL
}

// Client returns an HTTP client configured to trust the fake IS certificate.
func (s *Server) Client() *http.Client {
	return s.srv.Client()
}

// WriteCAPEM writes the server's TLS certificate to a PEM file and returns its path.
func (s *Server) WriteCAPEM(t *testing.T) string {
	t.Helper()
	cert := s.srv.Certificate()
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	path := filepath.Join(t.TempDir(), "is-ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("write CA PEM: %v", err)
	}
	return path
}

// Handle registers a handler for an exact HTTP method and path.
func (s *Server) Handle(method, path string, fn http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[strings.ToUpper(method)+" "+path] = fn
}

// HandleJSON registers a handler that replies with status and JSON-encoded body.
func (s *Server) HandleJSON(method, path string, status int, body any) {
	s.Handle(method, path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	})
}

// SetFallback sets a handler used when no exact route or built-in OAuth route matches.
func (s *Server) SetFallback(fn http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallback = fn
}

// RejectOrg marks an organization id to fail organization_switch with 403.
func (s *Server) RejectOrg(orgID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectedOrgs[orgID] = true
}

// RotateKey generates a new RSA signing key with a new kid.
func (s *Server) RotateKey(t *testing.T, kid string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = key
	s.kid = kid
}

// JWKSFetches returns the number of requests to /oauth2/jwks.
func (s *Server) JWKSFetches() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jwksFetches
}

// Requests returns a copy of recorded HTTP requests.
func (s *Server) Requests() []RecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RecordedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

// ResetRequests clears recorded HTTP requests.
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

// SignJWT creates an RS256 JWT signed by the server's active RSA key.
func (s *Server) SignJWT(claims map[string]any) string {
	s.mu.Lock()
	key := s.key
	kid := s.kid
	s.mu.Unlock()

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
		panic(fmt.Sprintf("sign JWT: %v", err))
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	s.mu.Lock()
	s.requests = append(s.requests, RecordedRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Headers:  r.Header.Clone(),
		Body:     bodyBytes,
	})
	route := s.routes[strings.ToUpper(r.Method)+" "+r.URL.Path]
	fallback := s.fallback
	s.mu.Unlock()

	if route != nil {
		route(w, r)
		return
	}

	if r.Method == http.MethodPost && (r.URL.Path == "/oauth2/token" || strings.HasSuffix(r.URL.Path, "/oauth2/token")) {
		s.handleToken(w, r)
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/oauth2/jwks" || strings.HasSuffix(r.URL.Path, "/oauth2/jwks")) {
		s.handleJWKS(w)
		return
	}

	if fallback != nil {
		fallback(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": fmt.Sprintf("no fake IS route for %s %s", r.Method, r.URL.Path),
	})
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	grantType := r.FormValue("grant_type")

	s.mu.Lock()
	s.tokenCounter++
	seq := s.tokenCounter
	rejectedOrgs := s.rejectedOrgs
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch grantType {
	case "client_credentials":
		scope := r.FormValue("scope")
		if scope == "" {
			scope = "SYSTEM"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("cc-token-%d", seq),
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        scope,
		})
	case "organization_switch":
		orgID := r.FormValue("switching_organization")
		rootTok := r.FormValue("token")
		if orgID == "" || rootTok == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "invalid_request",
				"error_description": "token and switching_organization are required",
			})
			return
		}
		if rejectedOrgs[orgID] {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "access_denied",
				"error_description": fmt.Sprintf("access to organization %s refused", orgID),
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("org-token-%s-%d", orgID, seq),
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        r.FormValue("scope"),
		})
	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "unsupported_grant_type",
			"error_description": "unsupported grant_type: " + grantType,
		})
	}
}

func (s *Server) handleJWKS(w http.ResponseWriter) {
	s.mu.Lock()
	s.jwksFetches++
	pub := s.key.PublicKey
	kid := s.kid
	s.mu.Unlock()

	nBytes := pub.N.Bytes()
	eBytes := big.NewInt(int64(pub.E)).Bytes()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	})
}

// PublicKeyDER returns the PKIX DER bytes of the active RSA public key.
func (s *Server) PublicKeyDER() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	return b
}
