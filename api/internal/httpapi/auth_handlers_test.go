package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartattend/api/internal/config"
)

const testPasswordHash = "$argon2id$v=19$m=65536,t=1,p=12$3XMoKQWRevFuU7xwlYJkeA$7HS15wjxGdiO97QGiVdvlWq0bPXdqonWOHn3GY7O7bc"

func authTestConfig() config.Config {
	return config.Config{
		WebOrigin:         "http://localhost:3000",
		AdminUsername:     "admin",
		AdminPasswordHash: testPasswordHash,
		SessionTTL:        8 * time.Hour,
	}
}

func TestLoginSetsSecureSessionCookie(t *testing.T) {
	cfg := authTestConfig()
	cfg.CookieSecure = true
	handler := NewRouter(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
		`{"username":"admin","password":"correct horse battery staple"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", cfg.WebOrigin)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != sessionCookieName || cookie.Value == "" {
		t.Errorf("session cookie = %#v, want non-empty %q cookie", cookie, sessionCookieName)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie flags = %#v, want HttpOnly, Secure, SameSite=Lax", cookie)
	}
}

func TestLoginRejectsInvalidCredentialsGenerically(t *testing.T) {
	cfg := authTestConfig()
	handler := NewRouter(cfg)

	for name, body := range map[string]string{
		"bad username": `{"username":"someone","password":"correct horse battery staple"}`,
		"bad password": `{"username":"admin","password":"wrong"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", cfg.WebOrigin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			assertErrorCode(t, rec, "INVALID_CREDENTIALS")
		})
	}
}

func TestMeWithoutSessionIsUnauthorized(t *testing.T) {
	handler := NewRouter(authTestConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	cfg := authTestConfig()
	handler := NewRouter(cfg)
	sessionCookie := login(t, handler, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", cfg.WebOrigin)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != sessionCookieName || cleared[0].MaxAge >= 0 {
		t.Fatalf("cleared cookie = %#v, want expired %q cookie", cleared, sessionCookieName)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want %d", meRec.Code, http.StatusUnauthorized)
	}
}

func TestLoginRejectsMissingOrForeignOrigin(t *testing.T) {
	cfg := authTestConfig()
	handler := NewRouter(cfg)

	for name, origin := range map[string]string{"missing": "", "foreign": "http://evil.example"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
				`{"username":"admin","password":"correct horse battery staple"}`,
			))
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
			assertErrorCode(t, rec, "ORIGIN_NOT_ALLOWED")
		})
	}
}

func login(t *testing.T, handler http.Handler, cfg config.Config) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
		`{"username":"admin","password":"correct horse battery staple"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", cfg.WebOrigin)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != want {
		t.Errorf("error code = %q, want %q", response.Error.Code, want)
	}
}
