// transport_test.go — Tests for transport configuration and bearer middleware.
package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ghost-mcp/internal/audit"
)

// =============================================================================
// Load
// =============================================================================

func TestLoad_DefaultIsStdio(t *testing.T) {
	t.Setenv(EnvVar, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.Mode != Stdio {
		t.Errorf("Expected mode %q, got %q", Stdio, cfg.Mode)
	}
}

func TestLoad_ExplicitStdio(t *testing.T) {
	t.Setenv(EnvVar, "stdio")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.Mode != Stdio {
		t.Errorf("Expected mode %q, got %q", Stdio, cfg.Mode)
	}
}

func TestLoad_HTTPMode(t *testing.T) {
	t.Setenv(EnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "")
	t.Setenv(HTTPBaseURLEnvVar, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.Mode != HTTP {
		t.Errorf("Expected mode %q, got %q", HTTP, cfg.Mode)
	}
	if cfg.Addr != defaultHTTPAddr {
		t.Errorf("Expected default addr %q, got %q", defaultHTTPAddr, cfg.Addr)
	}
}

func TestLoad_HTTPModeCustomAddr(t *testing.T) {
	t.Setenv(EnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "0.0.0.0:9999")
	t.Setenv(HTTPBaseURLEnvVar, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.Addr != "0.0.0.0:9999" {
		t.Errorf("Expected addr %q, got %q", "0.0.0.0:9999", cfg.Addr)
	}
	if cfg.BaseURL != "http://0.0.0.0:9999" {
		t.Errorf("Expected baseURL %q, got %q", "http://0.0.0.0:9999", cfg.BaseURL)
	}
}

func TestLoad_HTTPModeCustomBaseURL(t *testing.T) {
	t.Setenv(EnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "localhost:8080")
	t.Setenv(HTTPBaseURLEnvVar, "https://mcp.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://mcp.example.com" {
		t.Errorf("Expected baseURL %q, got %q", "https://mcp.example.com", cfg.BaseURL)
	}
}

func TestLoad_UnknownMode(t *testing.T) {
	t.Setenv(EnvVar, "grpc")
	if _, err := Load(); err == nil {
		t.Error("Expected error for unknown transport mode, got nil")
	}
}

func TestLoad_BaseURLDefaultsFromAddr(t *testing.T) {
	t.Setenv(EnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "localhost:1234")
	t.Setenv(HTTPBaseURLEnvVar, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.BaseURL != "http://localhost:1234" {
		t.Errorf("Expected baseURL %q, got %q", "http://localhost:1234", cfg.BaseURL)
	}
}

// =============================================================================
// NewBearerMiddleware
// =============================================================================

func newTestAuditLogger(t *testing.T) *audit.Logger {
	t.Helper()
	t.Setenv(audit.EnvVar, t.TempDir())
	al, err := audit.New()
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	t.Cleanup(al.Close)
	return al
}

func newTestBearerMiddleware(t *testing.T, token string) http.Handler {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return NewBearerMiddleware(token, newTestAuditLogger(t), inner)
}

func TestBearerMiddleware_ValidToken(t *testing.T) {
	h := newTestBearerMiddleware(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}
}

func TestBearerMiddleware_MissingHeader(t *testing.T) {
	h := newTestBearerMiddleware(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr.Code)
	}
}

func TestBearerMiddleware_WrongToken(t *testing.T) {
	h := newTestBearerMiddleware(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr.Code)
	}
}

func TestBearerMiddleware_WrongScheme(t *testing.T) {
	h := newTestBearerMiddleware(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Basic secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr.Code)
	}
}

func TestBearerMiddleware_EmptyBearer(t *testing.T) {
	h := newTestBearerMiddleware(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr.Code)
	}
}

func TestBearerMiddleware_InnerHandlerNotCalledOnFailure(t *testing.T) {
	called := false
	al := newTestAuditLogger(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := NewBearerMiddleware("secret", al, inner)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if called {
		t.Error("Inner handler should not be called when auth fails")
	}
}
