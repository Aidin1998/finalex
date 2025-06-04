// admin_api.go: Admin API for live config, inspection, and control
package ratelimit

import (
	"encoding/json"
	"log"
	"net/http"
)

// RegisterAdminRoutes registers admin endpoints for rate limiting
func RegisterAdminRoutes(mux *http.ServeMux, cm *ConfigManager) {
	mux.HandleFunc("/ratelimit/config", func(w http.ResponseWriter, r *http.Request) {
		cm.mu.RLock()
		defer cm.mu.RUnlock()
		json.NewEncoder(w).Encode(cm.configs)
	})
	mux.HandleFunc("/ratelimit/whitelist", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		cm.Whitelist(key)
		w.Write([]byte("whitelisted"))
	})
	mux.HandleFunc("/ratelimit/blacklist", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		cm.Blacklist(key)
		w.Write([]byte("blacklisted"))
	})
}

// AdminAPI exposes HTTP endpoints for config inspection and overrides
func AdminAPI(cm *ConfigManager) http.Handler {
	mux := http.NewServeMux()
	RegisterAdminRoutes(mux, cm)
	return mux
}

// StartAdminAPI starts the admin HTTP server for live config management
func StartAdminAPI(cm *ConfigManager, addr string) {
	log.Printf("[ratelimit] Admin API listening on %s", addr)
	http.ListenAndServe(addr, AdminAPI(cm))
}
