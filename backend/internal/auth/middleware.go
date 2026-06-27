package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/api/response"
)

// Middleware enforces bearer-token authentication and writes the authenticated
// principal into request context.
func Middleware(authenticator Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing bearer token.")
				return
			}

			principal, err := authenticator.Authenticate(r.Context(), token)
			if err != nil {
				status := http.StatusInternalServerError
				code := "auth_failed"
				message := "Could not authenticate request."
				if errors.Is(err, ErrUnauthenticated) {
					status = http.StatusUnauthorized
					code = "unauthenticated"
					message = "Invalid bearer token."
				}
				response.Error(w, status, code, message)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
}

// bearerToken extracts the token value from an Authorization header.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}
