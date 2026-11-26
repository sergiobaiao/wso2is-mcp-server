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
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	jwksMinRefreshInterval    = 5 * time.Minute
	unknownKidRefreshCooldown = 1 * time.Minute
	jwksHTTPTimeout           = 30 * time.Second
)

// ValidatorOption configures a JWTValidator.
type ValidatorOption func(*validatorOptions)

type validatorOptions struct {
	httpClient *http.Client
	logger     *log.Logger
	now        func() time.Time
}

// WithHTTPClient overrides the HTTP client used to fetch JWKS documents.
func WithHTTPClient(hc *http.Client) ValidatorOption {
	return func(o *validatorOptions) {
		if hc != nil {
			o.httpClient = hc
		}
	}
}

// WithLogger configures the logger used for HTTP access log lines.
func WithLogger(l *log.Logger) ValidatorOption {
	return func(o *validatorOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

func withNow(fn func() time.Time) ValidatorOption {
	return func(o *validatorOptions) {
		if fn != nil {
			o.now = fn
		}
	}
}

// JWTValidator validates incoming OAuth 2.0 Bearer JWTs against WSO2 IS JWKS
// and attaches CredentialContext to authenticated requests.
type JWTValidator struct {
	cfg         *config.Config
	cache       *jwk.Cache
	cacheCancel context.CancelFunc
	jwksURL     string
	metadataURL string
	logger      *log.Logger
	now         func() time.Time

	refreshMu   sync.Mutex
	lastRefresh time.Time
}

// NewJWTValidator constructs a JWTValidator configured from cfg, initializing the JWKS cache.
func NewJWTValidator(ctx context.Context, cfg *config.Config, opts ...ValidatorOption) (*JWTValidator, error) {
	if cfg == nil {
		return nil, errors.New("jwt validator config is required")
	}
	if cfg.JWKSURL == "" {
		return nil, errors.New("jwks url is required")
	}
	if cfg.Issuer == "" {
		return nil, errors.New("oauth issuer is required")
	}
	if cfg.Resource == "" {
		return nil, errors.New("mcp resource is required")
	}

	var o validatorOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	httpClient := o.httpClient
	if httpClient == nil {
		var err error
		httpClient, err = buildJWKSHTTPClient(cfg.CertPath)
		if err != nil {
			return nil, err
		}
	}

	logger := o.logger
	if logger == nil {
		logger = log.Default()
	}

	nowFn := o.now
	if nowFn == nil {
		nowFn = time.Now
	}

	if ctx == nil {
		ctx = context.Background()
	}
	cacheCtx, cacheCancel := context.WithCancel(ctx)

	cache, err := jwk.NewCache(cacheCtx, httprc.NewClient())
	if err != nil {
		cacheCancel()
		return nil, fmt.Errorf("create JWKS cache: %w", err)
	}

	if err := cache.Register(
		cacheCtx,
		cfg.JWKSURL,
		jwk.WithMinInterval(jwksMinRefreshInterval),
		jwk.WithHTTPClient(httpClient),
	); err != nil {
		cacheCancel()
		return nil, fmt.Errorf("register JWKS URL: %w", err)
	}

	publicBase := strings.TrimRight(cfg.PublicURL, "/")
	if publicBase == "" {
		port := cfg.Port
		if port == "" {
			port = defaultPort
		}
		publicBase = "http://localhost:" + port
	}

	return &JWTValidator{
		cfg:         cfg,
		cache:       cache,
		cacheCancel: cacheCancel,
		jwksURL:     cfg.JWKSURL,
		metadataURL: publicBase + protectedResourceMetadataPath,
		logger:      logger,
		now:         nowFn,
	}, nil
}

// Close stops background JWKS refresh workers.
func (v *JWTValidator) Close() error {
	if v == nil {
		return nil
	}
	if v.cacheCancel != nil {
		v.cacheCancel()
	}
	if v.cache != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return v.cache.Shutdown(shutdownCtx)
	}
	return nil
}

// Middleware enforces Bearer JWT authentication, sets RFC 9728 WWW-Authenticate challenges,
// injects CredentialContext on success, and emits access log lines without tokens or bodies.
func (v *JWTValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q", v.metadataURL))
			http.Error(w, "missing bearer token in the Authorization header", http.StatusUnauthorized)
			v.logAccess(r.Method, r.URL.Path, http.StatusUnauthorized, time.Since(start), "-")
			return
		}

		tok, err := v.ValidateToken(r.Context(), token)
		if err != nil {
			reason := tokenErrorReason(err)
			w.Header().Set(
				"WWW-Authenticate",
				fmt.Sprintf(`Bearer error="invalid_token", error_description=%q, resource_metadata=%q`, reason, v.metadataURL),
			)
			http.Error(w, reason, http.StatusUnauthorized)
			v.logAccess(r.Method, r.URL.Path, http.StatusUnauthorized, time.Since(start), "-")
			return
		}

		sub, _ := tok.Subject()
		ctx := client.WithCredentialContext(r.Context(), client.CredentialContext{
			Mode:        config.AuthModeBearer,
			BearerToken: token,
			Subject:     sub,
		})

		srw := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(srw, r.WithContext(ctx))

		status := srw.status
		if status == 0 {
			status = http.StatusOK
		}
		v.logAccess(r.Method, r.URL.Path, status, time.Since(start), sub)
	})
}

// ValidateToken verifies rawToken signature against cached JWKS and validates iss, aud, exp, and nbf.
func (v *JWTValidator) ValidateToken(ctx context.Context, rawToken string) (jwt.Token, error) {
	msg, err := jws.Parse([]byte(rawToken), jws.WithCompact())
	if err != nil || len(msg.Signatures()) == 0 {
		return nil, &tokenValidationError{reason: "token is not a valid JWT"}
	}

	kid, _ := msg.Signatures()[0].ProtectedHeaders().KeyID()
	set, err := v.keySetForKid(ctx, kid)
	if err != nil {
		return nil, err
	}

	tok, err := jwt.Parse(
		[]byte(rawToken),
		jwt.WithContext(ctx),
		jwt.WithKeySet(set, jws.WithInferAlgorithmFromKey(true)),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Resource),
		jwt.WithAcceptableSkew(v.cfg.JWTClockSkew),
		jwt.WithClock(jwt.ClockFunc(v.now)),
		jwt.WithValidate(true),
	)
	if err != nil {
		return nil, &tokenValidationError{reason: classifyJWTError(err)}
	}
	return tok, nil
}

func (v *JWTValidator) keySetForKid(ctx context.Context, kid string) (jwk.Set, error) {
	set, err := v.cache.Lookup(ctx, v.jwksURL)
	if err == nil && (kid == "" || hasKeyID(set, kid)) {
		return set, nil
	}

	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()

	// Re-check after acquiring lock in case another goroutine already refreshed.
	if set, err = v.cache.Lookup(ctx, v.jwksURL); err == nil && (kid == "" || hasKeyID(set, kid)) {
		return set, nil
	}

	now := v.now()
	if v.lastRefresh.IsZero() || now.Sub(v.lastRefresh) >= unknownKidRefreshCooldown {
		v.lastRefresh = now
		refreshed, refreshErr := v.cache.Refresh(ctx, v.jwksURL)
		if refreshErr != nil {
			if set == nil {
				return nil, &tokenValidationError{reason: "unable to retrieve JWKS"}
			}
		} else {
			set = refreshed
		}
	}

	if set == nil {
		return nil, &tokenValidationError{reason: "unable to retrieve JWKS"}
	}
	if kid != "" && !hasKeyID(set, kid) {
		return nil, &tokenValidationError{reason: "unknown signing key"}
	}
	return set, nil
}

func hasKeyID(set jwk.Set, kid string) bool {
	_, ok := set.LookupKeyID(kid)
	return ok
}

func (v *JWTValidator) logAccess(method, path string, status int, duration time.Duration, sub string) {
	if v.logger == nil {
		return
	}
	safeSub := sanitizeLogField(sub)
	if safeSub == "" {
		safeSub = "-"
	}
	v.logger.Printf(
		"method=%s path=%s status=%d duration=%s sub=%s",
		sanitizeLogField(method),
		sanitizeLogField(path),
		status,
		duration.Round(time.Microsecond),
		safeSub,
	)
}

func sanitizeLogField(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func buildJWKSHTTPClient(certPath string) (*http.Client, error) {
	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if certPath != "" {
		cleanPath := filepath.Clean(certPath)
		pemBytes, err := os.ReadFile(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("read CERT_PATH %q: %w", cleanPath, err)
		}
		if !rootCAs.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("no valid PEM certificates found in %q", cleanPath)
		}
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    rootCAs,
				MinVersion: tls.VersionTLS12,
			},
		},
		Timeout: jwksHTTPTimeout,
	}, nil
}

type tokenValidationError struct {
	reason string
}

func (e *tokenValidationError) Error() string {
	return e.reason
}

func tokenErrorReason(err error) string {
	var ve *tokenValidationError
	if errors.As(err, &ve) && ve.reason != "" {
		return ve.reason
	}
	return "token validation failed"
}

func classifyJWTError(err error) string {
	switch {
	case errors.Is(err, jwt.InvalidIssuerError()):
		return "invalid token issuer"
	case errors.Is(err, jwt.InvalidAudienceError()):
		return "invalid token audience"
	case errors.Is(err, jwt.TokenExpiredError()):
		return "token expired"
	case errors.Is(err, jwt.TokenNotYetValidError()):
		return "token not yet valid"
	case errors.Is(err, jwt.InvalidIssuedAtError()):
		return "invalid token issued-at time"
	case errors.Is(err, jws.VerificationError()), errors.Is(err, jws.VerifyError()):
		return "invalid token signature"
	case errors.Is(err, jwt.UnknownPayloadTypeError()), errors.Is(err, jwt.ParseError()):
		return "token is not a valid JWT"
	default:
		return "token validation failed"
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
