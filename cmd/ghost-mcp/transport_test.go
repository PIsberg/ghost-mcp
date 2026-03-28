// transport_test.go — Tests for transport configuration and bearer middleware.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// =============================================================================
// loadTransportConfig
// =============================================================================

func TestLoadTransportConfig_DefaultIsStdio(t *testing.T) {
	t.Setenv(TransportEnvVar, "")
	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.mode != TransportStdio {
		t.Errorf("Expected mode %q, got %q", TransportStdio, cfg.mode)
	}
}

func TestLoadTransportConfig_ExplicitStdio(t *testing.T) {
	t.Setenv(TransportEnvVar, "stdio")
	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.mode != TransportStdio {
		t.Errorf("Expected mode %q, got %q", TransportStdio, cfg.mode)
	}
}

func TestLoadTransportConfig_HTTPMode(t *testing.T) {
	t.Setenv(TransportEnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "")
	t.Setenv(HTTPBaseURLEnvVar, "")
	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.mode != TransportHTTP {
		t.Errorf("Expected mode %q, got %q", TransportHTTP, cfg.mode)
	}
	if cfg.addr != defaultHTTPAddr {
		t.Errorf("Expected default addr %q, got %q", defaultHTTPAddr, cfg.addr)
	}
}

func TestLoadTransportConfig_HTTPModeCustomAddr(t *testing.T) {
	t.Setenv(TransportEnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "0.0.0.0:9999")
	t.Setenv(HTTPBaseURLEnvVar, "")
	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.addr != "0.0.0.0:9999" {
		t.Errorf("Expected addr %q, got %q", "0.0.0.0:9999", cfg.addr)
	}
	if cfg.baseURL != "http://0.0.0.0:9999" {
		t.Errorf("Expected baseURL %q, got %q", "http://0.0.0.0:9999", cfg.baseURL)
	}
}

func TestLoadTransportConfig_HTTPModeCustomBaseURL(t *testing.T) {
	t.Setenv(TransportEnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "localhost:8080")
	t.Setenv(HTTPBaseURLEnvVar, "https://mcp.example.com")
	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.baseURL != "https://mcp.example.com" {
		t.Errorf("Expected baseURL %q, got %q", "https://mcp.example.com", cfg.baseURL)
	}
}

func TestLoadTransportConfig_UnknownMode(t *testing.T) {
	t.Setenv(TransportEnvVar, "grpc")
	_, err := loadTransportConfig()
	if err == nil {
		t.Error("Expected error for unknown transport mode, got nil")
	}
}

func TestLoadTransportConfig_BaseURLDefaultsFromAddr(t *testing.T) {
	t.Setenv(TransportEnvVar, "http")
	t.Setenv(HTTPAddrEnvVar, "localhost:1234")
	t.Setenv(HTTPBaseURLEnvVar, "")
	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "http://localhost:1234"
	if cfg.baseURL != expected {
		t.Errorf("Expected baseURL %q, got %q", expected, cfg.baseURL)
	}
}

// =============================================================================
// newBearerMiddleware
// =============================================================================

func newTestBearerMiddleware(t *testing.T, token string) http.Handler {
	t.Helper()
	t.Setenv(AuditEnvVar, t.TempDir())
	al, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	t.Cleanup(al.Close)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return newBearerMiddleware(token, al, inner)
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

func TestBearerMiddleware_EmptyToken(t *testing.T) {
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
	t.Setenv(AuditEnvVar, t.TempDir())
	al, _ := NewAuditLogger()
	t.Cleanup(al.Close)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := newBearerMiddleware("secret", al, inner)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	// No Authorization header
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Error("Inner handler should not be called when auth fails")
	}
}
