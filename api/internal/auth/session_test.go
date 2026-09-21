package auth

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestMemorySessionStoreCreatesOpaque256BitToken(t *testing.T) {
	store := NewMemorySessionStore(100)

	token, expiresAt, err := store.Create("admin", time.Hour)

	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not raw URL-safe base64: %v", err)
	}
	if len(raw) < 32 {
		t.Errorf("token entropy bytes = %d, want at least 32", len(raw))
	}
	if time.Until(expiresAt) <= 0 {
		t.Errorf("expiresAt = %v, want future time", expiresAt)
	}
	if username, ok := store.Get(token); !ok || username != "admin" {
		t.Errorf("Get() = %q, %v; want admin, true", username, ok)
	}
}

func TestMemorySessionStoreDeleteInvalidatesSession(t *testing.T) {
	store := NewMemorySessionStore(100)
	token, _, err := store.Create("admin", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	store.Delete(token)

	if username, ok := store.Get(token); ok {
		t.Errorf("Get() after Delete = %q, true; want empty, false", username)
	}
}

func TestMemorySessionStoreUsesAbsoluteExpiry(t *testing.T) {
	store := NewMemorySessionStore(100)
	token, _, err := store.Create("admin", time.Millisecond)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	if username, ok := store.Get(token); ok {
		t.Errorf("Get() after expiry = %q, true; want empty, false", username)
	}
}

func TestMemorySessionStoreEnforcesCapacity(t *testing.T) {
	store := NewMemorySessionStore(1)
	if _, _, err := store.Create("admin", time.Hour); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	if _, _, err := store.Create("admin", time.Hour); err == nil {
		t.Fatal("second Create() error = nil, want capacity error")
	}
}
