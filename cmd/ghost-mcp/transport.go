// transport.go — Transport mode selection (stdio vs HTTP/SSE) for Ghost MCP.
//
// The default transport is stdio (MCP over stdin/stdout), which is the standard
// mode used by Claude Desktop and most MCP clients.
//
// Set GHOST_MCP_TRANSPORT=http to start an HTTP/SSE server instead. This is
// useful for web-based clients or environments where stdio is not available.
// All HTTP requests still require the same GHOST_MCP_TOKEN as bearer auth.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	// TransportEnvVar selects the transport mode ("stdio" or "http").
	TransportEnvVar = "GHOST_MCP_TRANSPORT"

	// HTTPAddrEnvVar sets the listen address for HTTP mode (e.g. "localhost:8080").
	HTTPAddrEnvVar = "GHOST_MCP_HTTP_ADDR"

	// HTTPBaseURLEnvVar sets the public base URL advertised to SSE clients.
	// Defaults to "http://<addr>".
	HTTPBaseURLEnvVar = "GHOST_MCP_HTTP_BASE_URL"

	// TransportStdio is the default transport mode.
	TransportStdio = "stdio"

	// TransportHTTP enables the HTTP/SSE transport.
	TransportHTTP = "http"

	// defaultHTTPAddr is the listen address used when HTTPAddrEnvVar is not set.
	defaultHTTPAddr = "localhost:8080"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// transportConfig holds the resolved HTTP transport settings.
type transportConfig struct {
	mode    string
	addr    string
	baseURL string
}

// loadTransportConfig reads transport settings from the environment.
// It returns an error for unrecognised transport modes.
func loadTransportConfig() (transportConfig, error) {
	mode := os.Getenv(TransportEnvVar)
	if mode == "" {
		mode = TransportStdio
	}

	switch mode {
	case TransportStdio:
		return transportConfig{mode: TransportStdio}, nil

	case TransportHTTP:
		addr := os.Getenv(HTTPAddrEnvVar)
		if addr == "" {
			addr = defaultHTTPAddr
		}
		baseURL := os.Getenv(HTTPBaseURLEnvVar)
		if baseURL == "" {
			baseURL = "http://" + addr
		}
		return transportConfig{mode: TransportHTTP, addr: addr, baseURL: baseURL}, nil

	default:
		return transportConfig{}, fmt.Errorf(
			"unknown transport %q: set %s to %q (default) or %q",
			mode, TransportEnvVar, TransportStdio, TransportHTTP,
		)
	}
}

// =============================================================================
// BEARER TOKEN MIDDLEWARE
// =============================================================================

// newBearerMiddleware returns an http.Handler that requires every request to
// carry a valid "Authorization: Bearer <token>" header. Requests that fail
// the check are logged, audited, and rejected with HTTP 401.
func newBearerMiddleware(token string, al *AuditLogger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		var bearer string
		if strings.HasPrefix(authHeader, "Bearer ") {
			bearer = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if bearer != token {
			logError("HTTP auth failed: invalid or missing Bearer token from %s", r.RemoteAddr)
			al.Log(EventAuthFailure, "", "invalid or missing Bearer token", map[string]interface{}{
				"remote_addr": r.RemoteAddr,
				"path":        r.URL.Path,
			})
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// HTTP/SSE TRANSPORT
// =============================================================================

// serveHTTPTransport starts an HTTP/SSE server wrapping mcpServer. It blocks
// until the global shutdownChan is closed (failsafe/signal) or the server
// encounters a fatal error. The bearer token middleware guards every endpoint.
func serveHTTPTransport(mcpServer *server.MCPServer, cfg transportConfig, token string, al *AuditLogger) error {
	// Create the SSE server first; wire the bearer middleware around it after.
	sseServer := server.NewSSEServer(
		mcpServer,
		server.WithBaseURL(cfg.baseURL),
		server.WithKeepAlive(true),
		server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			// Inject remote address so audit hooks can record the client origin.
			return context.WithValue(ctx, remoteAddrKey{}, r.RemoteAddr)
		}),
	)

	// Wrap with bearer auth middleware. WithHTTPServer injects our http.Server
	// so that sseServer.Start() and sseServer.Shutdown() both operate on it.
	// The SSEServer's ServeHTTP handles /sse and /message routing.
	httpSrv := &http.Server{
		Addr:         cfg.addr,
		Handler:      newBearerMiddleware(token, al, sseServer),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE streams are long-lived; no write deadline
		IdleTimeout:  120 * time.Second,
	}
	server.WithHTTPServer(httpSrv)(sseServer)

	logInfo("HTTP/SSE transport listening on %s (base URL: %s)", cfg.addr, cfg.baseURL)
	logInfo("SSE endpoint:     %s/sse", cfg.baseURL)
	logInfo("Message endpoint: %s/message", cfg.baseURL)

	// Start in background; block on shutdown signal or error.
	errCh := make(chan error, 1)
	go func() {
		if err := sseServer.Start(cfg.addr); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-state.shutdownChan:
		logInfo("Shutdown signal received, stopping HTTP server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return sseServer.Shutdown(ctx)

	case err := <-errCh:
		return err
	}
}

// remoteAddrKey is the context key for the HTTP client's remote address.
type remoteAddrKey struct{}
