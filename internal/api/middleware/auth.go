// internal/api/middleware/auth.go
package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
)

// APIKeyAuth returns middleware that validates X-API-Key header.
// If apiKey is empty, authentication is disabled.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no key configured
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			providedKey := r.Header.Get("X-API-Key")
			if providedKey == "" {
				response.Error(w, http.StatusUnauthorized,
					core.WrapError(core.ErrConfigMissing, nil))
				return
			}

			// Constant-time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
				response.Error(w, http.StatusUnauthorized,
					core.WrapError(core.ErrConfigInvalid, nil))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
