// admin_api_main.go: Standalone entrypoint to run the rate limit admin API and Prometheus metrics
//go:build admin
// +build admin

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Aidin1998/finalex/internal/middleware/ratelimit"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cm := ratelimit.ConfigManagerInstance
	// Optionally load config from file
	if len(os.Args) > 1 {
		if err := cm.LoadFromFile(os.Args[1]); err != nil {
			log.Printf("Failed to load config: %v", err)
		}
	}
	mux := http.NewServeMux()
	ratelimit.RegisterAdminRoutes(mux, cm)
	mux.Handle("/metrics", promhttp.Handler())
	addr := ":8081"
	log.Printf("[ratelimit] Admin API and Prometheus metrics listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
