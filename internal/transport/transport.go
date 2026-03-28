// Package transport handles transport mode selection (stdio vs HTTP/SSE).
//
// The default transport is stdio (MCP over stdin/stdout), which is the standard
// mode used by Claude Desktop and most MCP clients.
//
// Set GHOST_MCP_TRANSPORT=http to start an HTTP/SSE server instead. All HTTP
// requests still require the same GHOST_MCP_TOKEN as Bearer auth.
package transport

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ghost-mcp/internal/audit"
	"github.com/ghost-mcp/internal/logging"
	"github.com/mark3labs/mcp-go/server"
)

// Environment variable names.
const (
	EnvVar            = "GHOST_MCP_TRANSPORT"
	HTTPAddrEnvVar    = "GHOST_MCP_HTTP_ADDR"
	HTTPBaseURLEnvVar = "GHOST_MCP_HTTP_BASE_URL"
)

// Transport mode values.
const (
	Stdio = "stdio"
	HTTP  = "http"
)

const defaultHTTPAddr = "localhost:8080"

// Config holds resolved transport settings.
type Config struct {
	Mode    string
	Addr    string // HTTP mode only
	BaseURL string // HTTP mode only
}

// Load reads transport settings from the environment.
// Returns an error for unrecognised transport modes.
func Load() (Config, error) {
	mode := os.Getenv(EnvVar)
	if mode == "" {
		mode = Stdio
	}

	switch mode {
	case Stdio:
		return Config{Mode: Stdio}, nil

	case HTTP:
		addr := os.Getenv(HTTPAddrEnvVar)
		if addr == "" {
			addr = defaultHTTPAddr
		}
		baseURL := os.Getenv(HTTPBaseURLEnvVar)
		if baseURL == "" {
			baseURL = "http://" + addr
		}
		return Config{Mode: HTTP, Addr: addr, BaseURL: baseURL}, nil

	default:
		return Config{}, fmt.Errorf(
			"unknown transport %q: set %s to %q (default) or %q",
			mode, EnvVar, Stdio, HTTP,
		)
	}
}

// NewBearerMiddleware returns an http.Handler that requires every request to
// carry a valid "Authorization: Bearer <token>" header. Requests that fail
// are logged, audited, and rejected with HTTP 401.
func NewBearerMiddleware(token string, al *audit.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		var bearer string
		if strings.HasPrefix(authHeader, "Bearer ") {
			bearer = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if bearer != token {
			logging.Error("HTTP auth failed: invalid or missing Bearer token from %s", r.RemoteAddr)
			al.Log(audit.EventAuthFailure, "", "invalid or missing Bearer token", map[string]interface{}{
				"remote_addr": r.RemoteAddr,
				"path":        r.URL.Path,
			})
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// remoteAddrKey is the context key for the HTTP client's remote address.
type remoteAddrKey struct{}

// ServeHTTP starts an HTTP/SSE server wrapping mcpServer. It blocks until
// shutdownCh is closed (failsafe/signal) or the server encounters a fatal
// error. The Bearer token middleware guards every endpoint.
func ServeHTTP(shutdownCh <-chan struct{}, mcpServer *server.MCPServer, cfg Config, token string, al *audit.Logger) error {
	sseServer := server.NewSSEServer(
		mcpServer,
		server.WithBaseURL(cfg.BaseURL),
		server.WithKeepAlive(true),
		server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, remoteAddrKey{}, r.RemoteAddr)
		}),
	)

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      NewBearerMiddleware(token, al, sseServer),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE streams are long-lived; no write deadline
		IdleTimeout:  120 * time.Second,
	}
	server.WithHTTPServer(httpSrv)(sseServer)

	logging.Info("HTTP/SSE transport listening on %s (base URL: %s)", cfg.Addr, cfg.BaseURL)
	logging.Info("SSE endpoint:     %s/sse", cfg.BaseURL)
	logging.Info("Message endpoint: %s/message", cfg.BaseURL)

	errCh := make(chan error, 1)
	go func() {
		if err := sseServer.Start(cfg.Addr); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-shutdownCh:
		logging.Info("Shutdown signal received, stopping HTTP server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return sseServer.Shutdown(ctx)

	case err := <-errCh:
		return err
	}
}
