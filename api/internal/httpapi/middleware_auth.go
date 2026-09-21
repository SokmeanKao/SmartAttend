package httpapi

import (
	"context"
	"net/http"

	"github.com/smartattend/api/internal/auth"
)

type authContextKey struct{}

type authenticatedSession struct {
	token    string
	username string
}

func requireSession(store auth.SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
			return
		}
		username, ok := store.Get(cookie.Value)
		if !ok {
			WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
			return
		}
		session := authenticatedSession{token: cookie.Value, username: username}
		ctx := context.WithValue(r.Context(), authContextKey{}, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func sessionFromContext(ctx context.Context) authenticatedSession {
	session, _ := ctx.Value(authContextKey{}).(authenticatedSession)
	return session
}
