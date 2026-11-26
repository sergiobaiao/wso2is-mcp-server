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

// Package transport provides stdio and Streamable HTTP transports for MCP servers.
package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultPort        = "8000"
	readHeaderTimeout  = 10 * time.Second
	shutdownGraceDelay = 10 * time.Second
)

// HTTPOptions configures the Streamable HTTP transport server.
type HTTPOptions struct {
	Port        string
	Stateless   bool
	Middleware  func(http.Handler) http.Handler
	ContextFunc func(context.Context, *http.Request) context.Context
	ExtraRoutes func(mux *http.ServeMux)
	ListenAddr  string
}

// ServeStdio serves s over standard input and standard output with signal handling.
func ServeStdio(s *server.MCPServer) error {
	return server.ServeStdio(s)
}

// ServeStdioStreams serves s over the provided reader and writer until ctx is cancelled or EOF.
func ServeStdioStreams(ctx context.Context, s *server.MCPServer, stdin io.Reader, stdout io.Writer) error {
	return server.NewStdioServer(s).Listen(ctx, stdin, stdout)
}

// NewHandler constructs the HTTP handler mounting /mcp, /healthz, and any ExtraRoutes.
func NewHandler(s *server.MCPServer, opts HTTPOptions) http.Handler {
	httpOpts := []server.StreamableHTTPOption{
		server.WithStateLess(opts.Stateless),
		server.WithStateful(!opts.Stateless),
	}
	if opts.ContextFunc != nil {
		httpOpts = append(httpOpts, server.WithHTTPContextFunc(opts.ContextFunc))
	}

	var mcpHandler http.Handler = server.NewStreamableHTTPServer(s, httpOpts...)
	if opts.Middleware != nil {
		mcpHandler = opts.Middleware(mcpHandler)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if opts.ExtraRoutes != nil {
		opts.ExtraRoutes(mux)
	}
	return mux
}

// NewHTTPServer builds an *http.Server configured with ReadHeaderTimeout and NewHandler.
func NewHTTPServer(s *server.MCPServer, opts HTTPOptions) *http.Server {
	addr := opts.ListenAddr
	if addr == "" {
		port := opts.Port
		if port == "" {
			port = defaultPort
		}
		addr = ":" + port
	}
	return &http.Server{
		Addr:              addr,
		Handler:           NewHandler(s, opts),
		ReadHeaderTimeout: readHeaderTimeout,
	}
}

// ServeHTTP listens and serves s over Streamable HTTP until ctx is cancelled, then shuts down gracefully.
func ServeHTTP(ctx context.Context, s *server.MCPServer, opts HTTPOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	srv := NewHTTPServer(s, opts)
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	return serveListener(ctx, srv, ln)
}

func serveListener(ctx context.Context, srv *http.Server, ln net.Listener) error {
	done := make(chan struct{})
	defer close(done)

	shutdownErr := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			// ctx is already cancelled here; shut down with its values but a fresh deadline.
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGraceDelay)
			defer cancel()
			shutdownErr <- srv.Shutdown(shutdownCtx)
		case <-done:
			shutdownErr <- nil
		}
	}()

	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-shutdownErr
}

// Serve runs s over either "stdio" or "http", handling SIGINT and SIGTERM for graceful shutdown.
func Serve(s *server.MCPServer, mode string, opts HTTPOptions) error {
	if mode == "stdio" {
		return ServeStdio(s)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ServeHTTP(ctx, s, opts)
}

// RequireAuthorization rejects requests without a Bearer Authorization header.
// It is the legacy no-validation middleware used when MCP_RESOURCE is not configured.
func RequireAuthorization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearerToken(r.Header.Get("Authorization")) == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "missing bearer token in the Authorization header", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
