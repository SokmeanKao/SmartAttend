package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/smartattend/api/internal/auth"
	"github.com/smartattend/api/internal/config"
)

func NewRouter(cfg config.Config) http.Handler {
	sessionStore := auth.NewMemorySessionStore(10_000)
	authAPI := authHandlers{cfg: cfg, store: sessionStore}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("POST /api/v1/auth/login", authAPI.login)
	mux.Handle("GET /api/v1/auth/me", requireSession(sessionStore, http.HandlerFunc(authAPI.me)))
	mux.Handle("POST /api/v1/auth/logout", requireSession(sessionStore, http.HandlerFunc(authAPI.logout)))
	return CORSMiddleware(cfg)(mux)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
