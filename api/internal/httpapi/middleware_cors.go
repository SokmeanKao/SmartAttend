package httpapi

import (
	"net/http"
	"slices"

	"github.com/smartattend/api/internal/config"
)

const allowedMethods = "GET, POST, PATCH, DELETE, OPTIONS"

var unsafeMethods = []string{
	http.MethodPost,
	http.MethodPatch,
	http.MethodDelete,
}

func CORSMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin == "" {
				if slices.Contains(unsafeMethods, r.Method) {
					WriteError(w, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Origin not allowed")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if origin == cfg.WebOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Add("Vary", "Origin")

				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			if r.Method == http.MethodOptions {
				WriteError(w, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Origin not allowed")
				return
			}

			if slices.Contains(unsafeMethods, r.Method) {
				WriteError(w, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Origin not allowed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
