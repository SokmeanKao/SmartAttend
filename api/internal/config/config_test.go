package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsSessionTTLAboveEightHours(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SESSION_TTL", "8h1s")

	_, err := Load()

	if err == nil {
		t.Fatal("Load() error = nil, want SESSION_TTL maximum error")
	}
	if !strings.Contains(err.Error(), "SESSION_TTL must not exceed 8h") {
		t.Fatalf("Load() error = %q, want SESSION_TTL maximum error", err)
	}
}

func TestLoadAcceptsEightHourSessionTTL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SESSION_TTL", "8h")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SessionTTL != 8*time.Hour {
		t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, 8*time.Hour)
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEB_ORIGIN", "http://localhost:3000")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("FACE_SERVICE_URL", "http://face-service:8090")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD_HASH", "test-hash")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("BUSINESS_TIMEZONE", "UTC")
}
