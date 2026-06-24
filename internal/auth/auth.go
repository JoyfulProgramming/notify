// Package auth is the MVP's entire authentication system (plan section 5):
// a static API-key map. The infrastructure is real — every HTTP entry point
// calls FromRequest exactly once, at the boundary — but the implementation is
// a one-liner. Swapping in JWT validation later changes only this lookup.
package auth

import (
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

var keys = map[string]string{
	"local-key": "local",
}

// FromRequest resolves a user_id from the Authorization header
// ("Bearer <key>"), or, for SSE connections that can't set custom headers,
// a "token" query parameter.
func FromRequest(r *http.Request) (userID string, ok bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, bearerPrefix) {
		id, found := keys[strings.TrimPrefix(h, bearerPrefix)]
		return id, found
	}
	if token := r.URL.Query().Get("token"); token != "" {
		id, found := keys[token]
		return id, found
	}
	return "", false
}
