// admin_api.go: Enhanced Admin API for rate limiting management
package ratelimit

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// AdminAPI provides HTTP endpoints for managing rate limiting
type AdminAPI struct {
	configManager *ConfigManager
	redisClient   *RedisClient
	logger        *zap.Logger
	metrics       *RateLimitMetrics
}

// NewAdminAPI creates a new admin API instance
func NewAdminAPI(configManager *ConfigManager, redisClient *RedisClient, logger *zap.Logger) *AdminAPI {
	return &AdminAPI{
		configManager: configManager,
		redisClient:   redisClient,
		logger:        logger,
		metrics:       &RateLimitMetrics{},
	}
}

// HandleGetConfigs returns all rate limiting configurations
func (api *AdminAPI) HandleGetConfigs(w http.ResponseWriter, r *http.Request) {
	configs := api.configManager.ListConfigs()

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Configurations retrieved successfully",
		Data:      configs,
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleGetConfig returns a specific rate limiting configuration
func (api *AdminAPI) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		api.writeErrorResponse(w, http.StatusBadRequest, "key parameter is required")
		return
	}

	config := api.configManager.GetConfig(key)
	if config == nil {
		api.writeErrorResponse(w, http.StatusNotFound, "configuration not found")
		return
	}

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Configuration retrieved successfully",
		Data:      config,
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleCreateConfig creates a new rate limiting configuration
func (api *AdminAPI) HandleCreateConfig(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		api.writeErrorResponse(w, http.StatusBadRequest, "key parameter is required")
		return
	}

	var config RateLimitConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		api.writeErrorResponse(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	// Validate config before setting
	if config.Limit <= 0 {
		api.writeErrorResponse(w, http.StatusBadRequest, "limit must be positive")
		return
	}
	if config.Window <= 0 {
		api.writeErrorResponse(w, http.StatusBadRequest, "window must be positive")
		return
	}

	api.configManager.SetConfig(key, &config)

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Configuration created successfully",
		Data:      map[string]string{"key": key},
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusCreated, response)
}

// HandleUpdateConfig updates an existing rate limiting configuration
func (api *AdminAPI) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		api.writeErrorResponse(w, http.StatusBadRequest, "key parameter is required")
		return
	}

	var update ConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		api.writeErrorResponse(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := api.configManager.UpdateConfig(key, &update); err != nil {
		api.writeErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Configuration updated successfully",
		Data:      map[string]string{"key": key},
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleDeleteConfig deletes a rate limiting configuration
func (api *AdminAPI) HandleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		api.writeErrorResponse(w, http.StatusBadRequest, "key parameter is required")
		return
	}
	if !api.configManager.DeleteConfig(key) {
		api.writeErrorResponse(w, http.StatusNotFound, "configuration not found")
		return
	}

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Configuration deleted successfully",
		Data:      map[string]string{"key": key},
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleGetStatus returns the current status of a rate limit key
func (api *AdminAPI) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		api.writeErrorResponse(w, http.StatusBadRequest, "key parameter is required")
		return
	}

	config := api.configManager.GetConfig(key)
	if config == nil || !config.Enabled {
		api.writeErrorResponse(w, http.StatusNotFound, "rate limit not found or disabled")
		return
	}

	status, err := api.getRateLimitStatus(r.Context(), key, config)
	if err != nil {
		api.writeErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Status retrieved successfully",
		Data:      status,
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleResetLimit resets a rate limit for a specific key
func (api *AdminAPI) HandleResetLimit(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		api.writeErrorResponse(w, http.StatusBadRequest, "key parameter is required")
		return
	}

	// Delete the key from Redis to reset the limit
	if err := api.redisClient.Client.Del(r.Context(), key).Err(); err != nil {
		api.writeErrorResponse(w, http.StatusInternalServerError, "failed to reset limit")
		return
	}

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Rate limit reset successfully",
		Data:      map[string]string{"key": key},
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleGetMetrics returns rate limiting metrics
func (api *AdminAPI) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	// In a real implementation, these metrics would be collected from monitoring
	response := AdminAPIResponse{
		Success:   true,
		Message:   "Metrics retrieved successfully",
		Data:      api.metrics,
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// HandleHealthCheck returns the health status of the rate limiting system
func (api *AdminAPI) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"redis":     api.checkRedisHealth(r.Context()),
		"configs":   len(api.configManager.ListConfigs()),
		"timestamp": time.Now(),
	}

	response := AdminAPIResponse{
		Success:   true,
		Message:   "Health check completed",
		Data:      health,
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, http.StatusOK, response)
}

// getRateLimitStatus gets the current status of a rate limit
func (api *AdminAPI) getRateLimitStatus(ctx context.Context, key string, config *RateLimitConfig) (*RateLimitStatus, error) {
	status := &RateLimitStatus{
		Key:       key,
		Algorithm: config.Type,
		Config:    config,
		IsBlocked: false,
		Metadata:  make(map[string]interface{}),
	}

	switch config.Type {
	case LimiterTokenBucket:
		refillRate := float64(config.Limit) / config.Window.Seconds()
		tokensLeft, nextRefill, err := api.redisClient.PeekTokenBucket(ctx, key, config.Burst, refillRate)
		if err != nil {
			return nil, err
		}

		status.Current = int(float64(config.Burst) - tokensLeft)
		status.Remaining = int(tokensLeft)
		status.ResetTime = nextRefill
		status.IsBlocked = tokensLeft < 1.0

	case LimiterSlidingWindow:
		count, reset, err := api.redisClient.PeekSlidingWindow(ctx, key, config.Window)
		if err != nil {
			return nil, err
		}

		status.Current = int(count)
		status.Remaining = config.Limit - int(count)
		status.ResetTime = reset
		status.IsBlocked = count >= int64(config.Limit)
	}

	return status, nil
}

// checkRedisHealth checks if Redis is healthy
func (api *AdminAPI) checkRedisHealth(ctx context.Context) map[string]interface{} {
	health := map[string]interface{}{
		"connected": false,
		"error":     nil,
	}

	if err := api.redisClient.HealthCheck(ctx); err != nil {
		health["error"] = err.Error()
	} else {
		health["connected"] = true
		health["stats"] = api.redisClient.GetConnectionStats()
	}

	return health
}

// writeJSONResponse writes a JSON response
func (api *AdminAPI) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		api.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

// writeErrorResponse writes an error response
func (api *AdminAPI) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := AdminAPIResponse{
		Success:   false,
		Error:     message,
		Timestamp: time.Now(),
	}

	api.writeJSONResponse(w, statusCode, response)
}

// RegisterRoutes registers admin API routes with a HTTP mux
func (api *AdminAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/ratelimit/configs", api.HandleGetConfigs)
	mux.HandleFunc("/admin/ratelimit/config", api.handleConfigEndpoint)
	mux.HandleFunc("/admin/ratelimit/status", api.HandleGetStatus)
	mux.HandleFunc("/admin/ratelimit/reset", api.HandleResetLimit)
	mux.HandleFunc("/admin/ratelimit/metrics", api.HandleGetMetrics)
	mux.HandleFunc("/admin/ratelimit/health", api.HandleHealthCheck)
}

// handleConfigEndpoint handles different HTTP methods for config endpoint
func (api *AdminAPI) handleConfigEndpoint(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.HandleGetConfig(w, r)
	case http.MethodPost:
		api.HandleCreateConfig(w, r)
	case http.MethodPut:
		api.HandleUpdateConfig(w, r)
	case http.MethodDelete:
		api.HandleDeleteConfig(w, r)
	default:
		api.writeErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// Legacy support functions for backwards compatibility
func RegisterAdminRoutes(mux *http.ServeMux, cm *ConfigManager) {
	mux.HandleFunc("/ratelimit/config", func(w http.ResponseWriter, r *http.Request) {
		configs := cm.ListConfigs()
		json.NewEncoder(w).Encode(configs)
	})
	mux.HandleFunc("/ratelimit/whitelist", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		// Disable rate limiting for this key by setting very high limits
		config := &RateLimitConfig{
			Name:    "Whitelisted",
			Type:    LimiterTokenBucket,
			Limit:   1000000,
			Window:  time.Minute,
			Burst:   1000000,
			Enabled: false, // Disabled means no rate limiting
		}
		cm.SetConfig(key, config)
		w.Write([]byte("whitelisted"))
	})
	mux.HandleFunc("/ratelimit/blacklist", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		// Blacklist by setting very restrictive limits
		config := &RateLimitConfig{
			Name:    "Blacklisted",
			Type:    LimiterTokenBucket,
			Limit:   1,
			Window:  time.Hour,
			Burst:   1,
			Enabled: true,
		}
		cm.SetConfig(key, config)
		w.Write([]byte("blacklisted"))
	})
}

// CreateAdminHandler exposes HTTP endpoints for config inspection and overrides
func CreateAdminHandler(cm *ConfigManager) http.Handler {
	mux := http.NewServeMux()
	RegisterAdminRoutes(mux, cm)
	return mux
}

// StartAdminAPI starts the admin HTTP server for live config management
func StartAdminAPI(cm *ConfigManager, addr string) {
	if cm.Logger != nil {
		cm.Logger.Info("Admin API listening", zap.String("addr", addr))
	}
	http.ListenAndServe(addr, CreateAdminHandler(cm))
}
