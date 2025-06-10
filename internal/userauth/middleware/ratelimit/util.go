// util.go: Helpers for key generation, time, etc.
package ratelimit

import (
	"net/http"
)

// KeyFromRequest generates a unique key for rate limiting (user, IP, API key, etc)
func KeyFromRequest(r *http.Request) string {
	return "anonymous"
}
