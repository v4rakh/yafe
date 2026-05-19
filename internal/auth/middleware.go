package auth

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

const (
	// HeaderAPIKey is the header used for API key authentication.
	HeaderAPIKey = "X-Api-Key" //nolint:gosec
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(errorResponse{Error: message}); err != nil {
		log.Error().Err(err).Msg("Failed to encode error response")
	}
}

// Middleware returns HTTP middleware for authentication.
// If auth is nil and required is false, all requests pass through.
// If auth is nil and required is true, all requests are rejected.
// If auth is not nil, requests with valid API key get user in context.
func Middleware(auth Authenticator, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(HeaderAPIKey)

			// No auth configured
			if auth == nil {
				if required {
					log.Warn().
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("remote", r.RemoteAddr).
						Msg("Auth required but not configured")
					writeError(w, http.StatusUnauthorized, "authentication required")
					return
				}
				// No auth required, pass through
				next.ServeHTTP(w, r)
				return
			}

			// No key provided
			if key == "" {
				if required {
					log.Warn().
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("remote", r.RemoteAddr).
						Msg("Missing API key")
					writeError(w, http.StatusUnauthorized, "authentication required")
					return
				}
				// Auth available but not required, pass through
				next.ServeHTTP(w, r)
				return
			}

			// Validate key
			user := auth.Authenticate(key)
			if user == nil {
				log.Warn().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("remote", r.RemoteAddr).
					Msg("Invalid API key")
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			// Add user to context and continue
			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that checks for a specific role.
// Must be used after Middleware.
func RequireRole(role Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())

			// No user in context = auth not required for this transport
			if user == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Check role
			if !user.HasRole(role) {
				log.Warn().
					Str("user", user.Name).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("required_role", string(role)).
					Msg("Insufficient permissions")
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LogAccess returns middleware that logs successful API access.
func LogAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())

		userName := "anonymous"
		if user != nil {
			userName = user.Name
		}

		log.Info().
			Str("user", userName).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote", r.RemoteAddr).
			Msg("API access")

		next.ServeHTTP(w, r)
	})
}
