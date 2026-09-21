package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/smartattend/api/internal/auth"
	"github.com/smartattend/api/internal/config"
)

const sessionCookieName = "smartattend_session"

type authHandlers struct {
	cfg   config.Config
	store auth.SessionStore
}

func (h authHandlers) login(w http.ResponseWriter, r *http.Request) {
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil {
		WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
		return
	}

	username := strings.TrimSpace(credentials.Username)
	// Length-equal compare only; mismatched lengths must not panic/false-negative oddly.
	usernameMatches := len(username) == len(h.cfg.AdminUsername) &&
		subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.AdminUsername)) == 1
	passwordMatches := auth.VerifyPassword(h.cfg.AdminPasswordHash, credentials.Password)
	if !usernameMatches || !passwordMatches {
		WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
		return
	}

	token, expiresAt, err := h.store.Create(h.cfg.AdminUsername, h.cfg.SessionTTL)
	if err != nil {
		WriteError(w, http.StatusServiceUnavailable, "SESSION_UNAVAILABLE", "Unable to create session")
		return
	}
	http.SetCookie(w, h.sessionCookie(token, expiresAt, 0))
	writeJSON(w, http.StatusOK, map[string]string{"username": h.cfg.AdminUsername})
}

func (h authHandlers) me(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"username": session.username})
}

func (h authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	h.store.Delete(session.token)
	http.SetCookie(w, h.sessionCookie("", time.Unix(1, 0), -1))
	w.WriteHeader(http.StatusNoContent)
}

func (h authHandlers) sessionCookie(token string, expiresAt time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
