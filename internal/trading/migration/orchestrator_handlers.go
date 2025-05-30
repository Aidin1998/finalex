// =============================
// Migration Orchestrator - HTTP Handlers
// =============================
// This file implements HTTP handlers for the migration orchestrator API.

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// ================================
// Middleware Functions
// ================================

// corsMiddleware handles CORS headers
func (o *MigrationOrchestrator) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles API authentication
func (o *MigrationOrchestrator) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check and dashboard
		if strings.HasSuffix(r.URL.Path, "/health") ||
			strings.HasPrefix(r.URL.Path, o.config.DashboardPath) {
			next.ServeHTTP(w, r)
			return
		}

		// Check API key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if !o.isValidAPIKey(apiKey) {
			o.writeErrorResponse(w, http.StatusUnauthorized, "Invalid or missing API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware handles rate limiting
func (o *MigrationOrchestrator) rateLimitMiddleware(next http.Handler) http.Handler {
	// Simplified rate limiting - in production, use a proper rate limiter
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement proper rate limiting
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests
func (o *MigrationOrchestrator) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		o.logger.Infow("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ================================
// Dashboard Handlers
// ================================

// handleDashboard serves the main dashboard page
func (o *MigrationOrchestrator) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if o.templates == nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Dashboard templates not loaded")
		return
	}

	data := o.buildDashboardData()

	w.Header().Set("Content-Type", "text/html")
	if err := o.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		o.logger.Errorw("Error rendering dashboard template", "error", err)
		o.writeErrorResponse(w, http.StatusInternalServerError, "Error rendering dashboard")
		return
	}
}

// handleDashboardData serves dashboard data as JSON
func (o *MigrationOrchestrator) handleDashboardData(w http.ResponseWriter, r *http.Request) {
	data := o.buildDashboardData()
	o.writeJSONResponse(w, data)
}

// ================================
// Migration Management Handlers
// ================================

// handleListMigrations lists all migrations
func (o *MigrationOrchestrator) handleListMigrations(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	status := r.URL.Query().Get("status")
	phase := r.URL.Query().Get("phase")
	limit := 50 // default limit

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	// Get migrations from coordinator
	allMigrations := o.coordinator.GetAllMigrations()
	var filteredMigrations []MigrationStateInfo

	for _, migration := range allMigrations {
		info := o.buildMigrationStateInfo(migration)

		// Apply filters
		if status != "" && string(info.Status) != status {
			continue
		}
		if phase != "" && string(info.Phase) != phase {
			continue
		}

		filteredMigrations = append(filteredMigrations, info)

		// Apply limit
		if len(filteredMigrations) >= limit {
			break
		}
	}

	// Sort by start time (most recent first)
	for i := 0; i < len(filteredMigrations)-1; i++ {
		for j := i + 1; j < len(filteredMigrations); j++ {
			if filteredMigrations[i].StartTime.Before(filteredMigrations[j].StartTime) {
				filteredMigrations[i], filteredMigrations[j] = filteredMigrations[j], filteredMigrations[i]
			}
		}
	}

	response := map[string]interface{}{
		"migrations": filteredMigrations,
		"total":      len(allMigrations),
		"filtered":   len(filteredMigrations),
		"limit":      limit,
	}

	o.writeJSONResponse(w, response)
}

// handleCreateMigration creates a new migration
func (o *MigrationOrchestrator) handleCreateMigration(w http.ResponseWriter, r *http.Request) {
	var req MigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// Validate request
	if req.OrderBookID == "" {
		o.writeErrorResponse(w, http.StatusBadRequest, "order_book_id is required")
		return
	}
	if req.SourceStrategy == "" {
		o.writeErrorResponse(w, http.StatusBadRequest, "source_strategy is required")
		return
	}
	if req.TargetStrategy == "" {
		o.writeErrorResponse(w, http.StatusBadRequest, "target_strategy is required")
		return
	}

	// Create migration config
	migrationConfig := &MigrationConfig{
		OrderBookID:   req.OrderBookID,
		Mode:          req.MigrationMode,
		Timeout:       30 * time.Minute, // default timeout
		MaxRetries:    3,
		RetryInterval: 5 * time.Second,
		Parameters:    req.Config,
		DryRun:        req.DryRun,
	}

	// Set defaults
	if migrationConfig.Mode == "" {
		migrationConfig.Mode = ModeGradual
	}

	// Start migration
	ctx := context.Background()
	migrationID, err := o.coordinator.StartMigration(ctx, migrationConfig)
	if err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Failed to start migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:     true,
		MigrationID: migrationID,
		Message:     "Migration started successfully",
		Timestamp:   time.Now(),
	}

	w.WriteHeader(http.StatusCreated)
	o.writeJSONResponse(w, response)
}

// handleGetMigration gets details of a specific migration
func (o *MigrationOrchestrator) handleGetMigration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	migration, exists := o.coordinator.GetMigration(migrationID)
	if !exists {
		o.writeErrorResponse(w, http.StatusNotFound, "Migration not found")
		return
	}

	info := o.buildMigrationStateInfo(migration)
	o.writeJSONResponse(w, info)
}

// handleAbortMigration aborts a migration
func (o *MigrationOrchestrator) handleAbortMigration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.coordinator.AbortMigration(ctx, migrationID); err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Failed to abort migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Migration aborted successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponse(w, response)
}

// handleResumeMigration resumes a paused migration
func (o *MigrationOrchestrator) handleResumeMigration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.coordinator.ResumeMigration(ctx, migrationID); err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Failed to resume migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Migration resumed successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponse(w, response)
}

// handleRetryMigration retries a failed migration
func (o *MigrationOrchestrator) handleRetryMigration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.coordinator.RetryMigration(ctx, migrationID); err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retry migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Migration retry initiated successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponse(w, response)
}

// ================================
// Participant Management Handlers
// ================================

// handleListParticipants lists all participants
func (o *MigrationOrchestrator) handleListParticipants(w http.ResponseWriter, r *http.Request) {
	participants := o.coordinator.GetAllParticipants()
	var participantStatus []ParticipantStatus

	for id, participant := range participants {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		healthCheck, err := participant.HealthCheck(ctx)
		cancel()

		status := ParticipantStatus{
			ID:            id,
			Type:          fmt.Sprintf("%T", participant),
			IsHealthy:     err == nil && healthCheck.IsHealthy,
			LastHeartbeat: time.Now(), // TODO: Get actual heartbeat time
			Status:        "active",
		}

		if err != nil {
			status.ErrorMessage = err.Error()
		}

		if healthCheck != nil {
			status.Metrics = healthCheck.Metrics
		}

		participantStatus = append(participantStatus, status)
	}

	response := map[string]interface{}{
		"participants": participantStatus,
		"total":        len(participantStatus),
	}

	o.writeJSONResponse(w, response)
}

// handleGetParticipant gets details of a specific participant
func (o *MigrationOrchestrator) handleGetParticipant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	participantID := vars["id"]

	participant, exists := o.coordinator.GetParticipant(participantID)
	if !exists {
		o.writeErrorResponse(w, http.StatusNotFound, "Participant not found")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthCheck, err := participant.HealthCheck(ctx)

	status := ParticipantStatus{
		ID:            participantID,
		Type:          fmt.Sprintf("%T", participant),
		IsHealthy:     err == nil && (healthCheck != nil && healthCheck.IsHealthy),
		LastHeartbeat: time.Now(), // TODO: Get actual heartbeat time
		Status:        "active",
	}

	if err != nil {
		status.ErrorMessage = err.Error()
	}

	if healthCheck != nil {
		status.Metrics = healthCheck.Metrics
	}

	o.writeJSONResponse(w, status)
}

// handleParticipantHealth gets health status of a specific participant
func (o *MigrationOrchestrator) handleParticipantHealth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	participantID := vars["id"]

	participant, exists := o.coordinator.GetParticipant(participantID)
	if !exists {
		o.writeErrorResponse(w, http.StatusNotFound, "Participant not found")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthCheck, err := participant.HealthCheck(ctx)
	if err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Health check failed: "+err.Error())
		return
	}

	o.writeJSONResponse(w, healthCheck)
}

// ================================
// System Status Handlers
// ================================

// handleHealth handles health check requests
func (o *MigrationOrchestrator) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
		"services": map[string]string{
			"coordinator":    "healthy",
			"safety_manager": "healthy",
			"orchestrator":   "healthy",
		},
	}

	o.writeJSONResponse(w, health)
}

// handleSystemHealth handles system health requests
func (o *MigrationOrchestrator) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	healthInfo := o.buildSystemHealthInfo()
	o.writeJSONResponse(w, healthInfo)
}

// handleSystemMetrics handles system metrics requests
func (o *MigrationOrchestrator) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"total_migrations":      o.coordinator.GetTotalMigrations(),
		"successful_migrations": o.coordinator.GetSuccessfulMigrations(),
		"failed_migrations":     o.coordinator.GetFailedMigrations(),
		"active_migrations":     len(o.coordinator.GetActiveMigrations()),
		"timestamp":             time.Now(),
	}

	o.writeJSONResponse(w, metrics)
}

// handlePerformanceMetrics handles performance metrics requests
func (o *MigrationOrchestrator) handlePerformanceMetrics(w http.ResponseWriter, r *http.Request) {
	performance := o.buildPerformanceSnapshot()
	o.writeJSONResponse(w, performance)
}

// ================================
// Safety Handlers
// ================================

// handleSafetyStatus handles safety status requests
func (o *MigrationOrchestrator) handleSafetyStatus(w http.ResponseWriter, r *http.Request) {
	if o.safetyManager == nil {
		o.writeErrorResponse(w, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	ctx := context.Background()
	status := o.safetyManager.GetSafetyStatus(ctx)
	o.writeJSONResponse(w, status)
}

// handleSafetyRollback handles safety rollback requests
func (o *MigrationOrchestrator) handleSafetyRollback(w http.ResponseWriter, r *http.Request) {
	if o.safetyManager == nil {
		o.writeErrorResponse(w, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	vars := mux.Vars(r)
	idStr := vars["id"]

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.safetyManager.TriggerRollback(ctx, migrationID, "Manual rollback requested via API"); err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Failed to trigger rollback: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Rollback triggered successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponse(w, response)
}

// handleCircuitBreakerStatus handles circuit breaker status requests
func (o *MigrationOrchestrator) handleCircuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	if o.safetyManager == nil {
		o.writeErrorResponse(w, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	status := o.safetyManager.GetCircuitBreakerStatus()
	o.writeJSONResponse(w, status)
}

// handleCircuitBreakerReset handles circuit breaker reset requests
func (o *MigrationOrchestrator) handleCircuitBreakerReset(w http.ResponseWriter, r *http.Request) {
	if o.safetyManager == nil {
		o.writeErrorResponse(w, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	ctx := context.Background()
	if err := o.safetyManager.ResetCircuitBreaker(ctx); err != nil {
		o.writeErrorResponse(w, http.StatusInternalServerError, "Failed to reset circuit breaker: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Circuit breaker reset successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponse(w, response)
}
