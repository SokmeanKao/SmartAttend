package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/smartattend/api/internal/auth"
	"github.com/smartattend/api/internal/config"
)

func NewRouter(cfg config.Config, employeeStores ...employeeStore) http.Handler {
	sessionStore := auth.NewMemorySessionStore(10_000)
	authAPI := authHandlers{cfg: cfg, store: sessionStore}
	var employeeStore employeeStore = unavailableEmployeeStore{}
	if len(employeeStores) > 0 && employeeStores[0] != nil {
		employeeStore = employeeStores[0]
	}
	employeeAPI := employeeHandlers{
		store:       employeeStore,
		invalidator: noOpEmployeeInvalidator{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("POST /api/v1/auth/login", authAPI.login)
	mux.Handle("GET /api/v1/auth/me", requireSession(sessionStore, http.HandlerFunc(authAPI.me)))
	mux.Handle("POST /api/v1/auth/logout", requireSession(sessionStore, http.HandlerFunc(authAPI.logout)))
	mux.Handle("GET /api/v1/employees", requireSession(sessionStore, http.HandlerFunc(employeeAPI.list)))
	mux.Handle("POST /api/v1/employees", requireSession(sessionStore, http.HandlerFunc(employeeAPI.create)))
	mux.Handle("GET /api/v1/employees/{id}", requireSession(sessionStore, http.HandlerFunc(employeeAPI.get)))
	mux.Handle("PATCH /api/v1/employees/{id}", requireSession(sessionStore, http.HandlerFunc(employeeAPI.patch)))
	mux.Handle("DELETE /api/v1/employees/{id}", requireSession(sessionStore, http.HandlerFunc(employeeAPI.deactivate)))
	return CORSMiddleware(cfg)(mux)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
