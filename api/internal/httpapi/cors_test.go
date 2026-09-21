package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smartattend/api/internal/config"
)

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	cfg := config.Config{WebOrigin: "http://localhost:3000"}
	handler := NewRouter(cfg)

	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
	}
}

func TestCORSRejectsOtherOrigin(t *testing.T) {
	cfg := config.Config{WebOrigin: "http://localhost:3000"}
	handler := NewRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://evil.example")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got == "true" {
		t.Errorf("Access-Control-Allow-Credentials should not be true for foreign origin")
	}
}
