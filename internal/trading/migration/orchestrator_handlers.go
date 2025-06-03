// =============================
// Migration Orchestrator - HTTP Handlers
// =============================
// This file implements HTTP handlers for the migration orchestrator API.

package migration

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ================================
// Middleware Functions (Gin version)
// ================================

// corsMiddleware handles CORS headers (Gin)
func (o *MigrationOrchestrator) corsMiddleware(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
	if c.Request.Method == "OPTIONS" {
		c.AbortWithStatus(http.StatusOK)
		return
	}
	c.Next()
}

// authMiddleware handles API authentication (Gin)
func (o *MigrationOrchestrator) authMiddleware(c *gin.Context) {
	apiKey := c.GetHeader("X-API-Key")
	valid := false
	for _, key := range o.config.APIKeys {
		if apiKey == key {
			valid = true
			break
		}
	}
	if !valid {
		o.writeErrorResponseGin(c, http.StatusUnauthorized, "Invalid or missing API key")
		c.Abort()
		return
	}
	c.Next()
}

// rateLimitMiddleware handles rate limiting (Gin)
func (o *MigrationOrchestrator) rateLimitMiddleware(c *gin.Context) {
	// Simplified rate limiting - in production, use a proper rate limiter
	c.Next()
}

// loggingMiddleware logs HTTP requests (Gin)
func (o *MigrationOrchestrator) loggingMiddleware(c *gin.Context) {
	start := time.Now()

	// Create a response writer wrapper to capture status code
	wrapped := &responseWriter{ResponseWriter: c.Writer, statusCode: http.StatusOK}

	c.Writer = wrapped

	c.Next()

	duration := time.Since(start)

	o.logger.Infow("HTTP request",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"status", wrapped.statusCode,
		"duration_ms", duration.Milliseconds(),
		"remote_addr", c.Request.RemoteAddr,
		"user_agent", c.Request.UserAgent(),
	)
}

// responseWriter wraps gin.ResponseWriter to capture status code
type responseWriter struct {
	gin.ResponseWriter
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
func (o *MigrationOrchestrator) handleDashboard(c *gin.Context) {
	if o.templates == nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Dashboard templates not loaded")
		return
	}

	data := o.buildDashboardData()

	c.Header("Content-Type", "text/html")
	if err := o.templates.ExecuteTemplate(c.Writer, "dashboard.html", data); err != nil {
		o.logger.Errorw("Error rendering dashboard template", "error", err)
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Error rendering dashboard")
		return
	}
}

// handleDashboardData serves dashboard data as JSON
func (o *MigrationOrchestrator) handleDashboardData(c *gin.Context) {
	data := o.buildDashboardData()
	o.writeJSONResponseGin(c, data)
}

// ================================
// Migration Management Handlers
// ================================

// handleListMigrations lists all migrations
func (o *MigrationOrchestrator) handleListMigrations(c *gin.Context) {
	// Parse query parameters
	status := c.Query("status")
	phase := c.Query("phase")
	limit := 50 // default limit

	if limitStr := c.Query("limit"); limitStr != "" {
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

	o.writeJSONResponseGin(c, response)
}

// handleCreateMigration creates a new migration
func (o *MigrationOrchestrator) handleCreateMigration(c *gin.Context) {
	var req MigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// Validate request
	if req.Pair == "" {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "pair is required")
		return
	}
	if req.Config == nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "config is required")
		return
	}

	// Start migration
	ctx := context.Background()
	state, err := o.coordinator.StartMigration(ctx, &req)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Failed to start migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:     true,
		MigrationID: state.ID,
		Message:     "Migration started successfully",
		Data:        o.buildMigrationStateInfo(state),
		Timestamp:   time.Now(),
	}

	c.JSON(http.StatusCreated, response)
}

// handleGetMigration gets details of a specific migration
func (o *MigrationOrchestrator) handleGetMigration(c *gin.Context) {
	idStr := c.Param("id")

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	migration, exists := o.coordinator.GetMigration(migrationID)
	if !exists {
		o.writeErrorResponseGin(c, http.StatusNotFound, "Migration not found")
		return
	}

	info := o.buildMigrationStateInfo(migration)
	o.writeJSONResponseGin(c, info)
}

// handleAbortMigration aborts a migration
func (o *MigrationOrchestrator) handleAbortMigration(c *gin.Context) {
	idStr := c.Param("id")

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.coordinator.AbortMigration(ctx, migrationID, "manual abort requested via API"); err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Failed to abort migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Migration aborted successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponseGin(c, response)
}

// handleResumeMigration resumes a paused migration
func (o *MigrationOrchestrator) handleResumeMigration(c *gin.Context) {
	idStr := c.Param("id")

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.coordinator.ResumeMigration(ctx, migrationID); err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Failed to resume migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Migration resumed successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponseGin(c, response)
}

// handleRetryMigration retries a failed migration
func (o *MigrationOrchestrator) handleRetryMigration(c *gin.Context) {
	idStr := c.Param("id")

	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.coordinator.RetryMigration(ctx, migrationID); err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Failed to retry migration: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Migration retry initiated successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponseGin(c, response)
}

// ================================
// Participant Management Handlers
// ================================

// handleListParticipants lists all participants
func (o *MigrationOrchestrator) handleListParticipants(c *gin.Context) {
	o.coordinator.participantsMu.RLock()
	participants := make(map[string]MigrationParticipant, len(o.coordinator.participants))
	for id, p := range o.coordinator.participants {
		participants[id] = p
	}
	o.coordinator.participantsMu.RUnlock()
	var participantStatus []ParticipantStatus

	for id, participant := range participants {
		status := ParticipantStatus{
			ID:   id,
			Type: participant.GetType(),
			// ...additional fields as before...
		}
		participantStatus = append(participantStatus, status)
	}

	response := map[string]interface{}{
		"participants": participantStatus,
		"total":        len(participantStatus),
	}
	o.writeJSONResponseGin(c, response)
}

// handleGetParticipant retrieves details of a single participant
func (o *MigrationOrchestrator) handleGetParticipant(c *gin.Context) {
	participantID := c.Param("participant_id")
	o.coordinator.participantsMu.RLock()
	participant, exists := o.coordinator.participants[participantID]
	o.coordinator.participantsMu.RUnlock()
	if !exists {
		o.writeErrorResponseGin(c, http.StatusNotFound, "Participant not found")
		return
	}

	status := ParticipantStatus{
		ID:   participant.GetID(),
		Type: participant.GetType(),
		// ...additional fields as before...
	}
	o.writeJSONResponseGin(c, status)
}

// handleParticipantHealth gets health status of a specific participant
func (o *MigrationOrchestrator) handleParticipantHealth(c *gin.Context) {
	participantID := c.Param("id")
	o.coordinator.participantsMu.RLock()
	participant, exists := o.coordinator.participants[participantID]
	o.coordinator.participantsMu.RUnlock()
	if !exists {
		o.writeErrorResponseGin(c, http.StatusNotFound, "Participant not found")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthCheck, err := participant.HealthCheck(ctx)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Health check failed: "+err.Error())
		return
	}

	o.writeJSONResponseGin(c, healthCheck)
}

// ================================
// System Status Handlers
// ================================

// handleHealth handles health check requests
func (o *MigrationOrchestrator) handleHealth(c *gin.Context) {
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

	o.writeJSONResponseGin(c, health)
}

// handleSystemHealth handles system health requests
func (o *MigrationOrchestrator) handleSystemHealth(c *gin.Context) {
	healthInfo := o.buildSystemHealthInfo()
	o.writeJSONResponseGin(c, healthInfo)
}

// handleSystemMetrics handles system metrics requests
func (o *MigrationOrchestrator) handleSystemMetrics(c *gin.Context) {
	metrics := map[string]interface{}{
		"total_migrations":      o.coordinator.GetTotalMigrations(),
		"successful_migrations": o.coordinator.GetSuccessfulMigrations(),
		"failed_migrations":     o.coordinator.GetFailedMigrations(),
		"active_migrations":     len(o.coordinator.GetActiveMigrations()),
		"timestamp":             time.Now(),
	}

	o.writeJSONResponseGin(c, metrics)
}

// handlePerformanceMetrics handles performance metrics requests
func (o *MigrationOrchestrator) handlePerformanceMetrics(c *gin.Context) {
	performance := o.buildPerformanceSnapshot()
	o.writeJSONResponseGin(c, performance)
}

// ================================
// Safety Handlers
// ================================

// handleSafetyStatus handles safety status requests
func (o *MigrationOrchestrator) handleSafetyStatus(c *gin.Context) {
	if o.safetyManager == nil {
		o.writeErrorResponseGin(c, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	ctx := context.Background()
	status := o.safetyManager.GetSafetyStatus(ctx)
	o.writeJSONResponseGin(c, status)
}

// handleSafetyRollback handles safety rollback requests
func (o *MigrationOrchestrator) handleSafetyRollback(c *gin.Context) {
	if o.safetyManager == nil {
		o.writeErrorResponseGin(c, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	idStr := c.Param("id")
	migrationID, err := uuid.Parse(idStr)
	if err != nil {
		o.writeErrorResponseGin(c, http.StatusBadRequest, "Invalid migration ID")
		return
	}

	ctx := context.Background()
	if err := o.safetyManager.TriggerRollback(ctx, migrationID, "Manual rollback requested via API"); err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Failed to trigger rollback: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Rollback triggered successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponseGin(c, response)
}

// handleCircuitBreakerStatus handles circuit breaker status requests
func (o *MigrationOrchestrator) handleCircuitBreakerStatus(c *gin.Context) {
	if o.safetyManager == nil {
		o.writeErrorResponseGin(c, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	status := o.safetyManager.GetCircuitBreakerStatus()
	o.writeJSONResponseGin(c, status)
}

// handleCircuitBreakerReset handles circuit breaker reset requests
func (o *MigrationOrchestrator) handleCircuitBreakerReset(c *gin.Context) {
	if o.safetyManager == nil {
		o.writeErrorResponseGin(c, http.StatusServiceUnavailable, "Safety manager not available")
		return
	}

	ctx := context.Background()
	if err := o.safetyManager.ResetCircuitBreaker(ctx); err != nil {
		o.writeErrorResponseGin(c, http.StatusInternalServerError, "Failed to reset circuit breaker: "+err.Error())
		return
	}

	response := MigrationResponse{
		Success:   true,
		Message:   "Circuit breaker reset successfully",
		Timestamp: time.Now(),
	}

	o.writeJSONResponseGin(c, response)
}

// Gin-compatible JSON response helper
func (o *MigrationOrchestrator) writeJSONResponseGin(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}

// Gin-compatible error response helper
func (o *MigrationOrchestrator) writeErrorResponseGin(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
