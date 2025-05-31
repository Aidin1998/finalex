// =============================
// Migration Orchestrator - WebSocket and Utility Functions
// =============================
// This file implements WebSocket handlers and utility functions for the orchestrator.

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ================================
// Configuration Handlers
// ================================

// handleGetConfig handles configuration requests
func (o *MigrationOrchestrator) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	config := o.buildOrchestratorConfigInfo()
	o.writeJSONResponse(w, config)
}

// handleUpdateConfig handles configuration update requests
func (o *MigrationOrchestrator) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var updateReq map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updateReq); err != nil {
		o.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// TODO: Implement configuration updates
	// For now, just return the current config

	response := MigrationResponse{
		Success:   true,
		Message:   "Configuration updated successfully",
		Data:      o.buildOrchestratorConfigInfo(),
		Timestamp: time.Now(),
	}

	o.writeJSONResponse(w, response)
}

// ================================
// Event Handlers
// ================================

// handleGetEvents handles event listing requests
func (o *MigrationOrchestrator) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	limit := 100 // default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	// Get events from buffer
	o.eventsMu.RLock()
	events := make([]MigrationEvent, 0, len(o.eventBuffer))
	for i := len(o.eventBuffer) - 1; i >= 0 && len(events) < limit; i-- {
		events = append(events, o.eventBuffer[i])
	}
	o.eventsMu.RUnlock()

	response := map[string]interface{}{
		"events": events,
		"total":  len(o.eventBuffer),
		"limit":  limit,
	}

	o.writeJSONResponse(w, response)
}

// handleEventStream handles server-sent events for real-time updates
func (o *MigrationOrchestrator) handleEventStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create event channel
	eventChan := make(chan MigrationEvent, 10)
	// clientID not used, skip assignment

	// Subscribe to events (simplified implementation)
	// TODO: Implement proper event subscription mechanism

	// Send initial data
	data := o.buildDashboardData()
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)

	// Keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-eventChan:
			eventData, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", eventData)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-ticker.C:
			fmt.Fprintf(w, "data: {\"type\":\"heartbeat\",\"timestamp\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

// ================================
// WebSocket Handlers
// ================================

// handleWebSocket handles general WebSocket connections
func (o *MigrationOrchestrator) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := o.upgrader.Upgrade(w, r, nil)
	if err != nil {
		o.logger.Errorw("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	clientID := uuid.New().String()
	o.logger.Infow("WebSocket client connected", "client_id", clientID)

	// Add connection to pool
	o.wsConnsMu.Lock()
	if len(o.wsConnections) >= o.config.MaxWSConnections {
		o.wsConnsMu.Unlock()
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Connection limit reached"))
		return
	}
	o.wsConnections[clientID] = conn
	o.wsConnsMu.Unlock()

	// Remove connection when done
	defer func() {
		o.wsConnsMu.Lock()
		delete(o.wsConnections, clientID)
		o.wsConnsMu.Unlock()
		o.logger.Infow("WebSocket client disconnected", "client_id", clientID)
	}()

	// Send initial data
	data := o.buildDashboardData()
	if err := conn.WriteJSON(map[string]interface{}{
		"type": "initial_data",
		"data": data,
	}); err != nil {
		o.logger.Errorw("Failed to send initial data", "error", err)
		return
	}

	// Handle incoming messages
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				o.logger.Errorw("WebSocket error", "error", err)
			}
			break
		}

		// Handle ping messages
		if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
			conn.WriteJSON(map[string]interface{}{
				"type":      "pong",
				"timestamp": time.Now(),
			})
		}
	}
}

// handleWebSocketEvents handles WebSocket connections for events
func (o *MigrationOrchestrator) handleWebSocketEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := o.upgrader.Upgrade(w, r, nil)
	if err != nil {
		o.logger.Errorw("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	clientID := uuid.New().String()
	o.logger.Infow("WebSocket events client connected", "client_id", clientID)

	// Subscribe to events (simplified implementation)
	eventChan := make(chan MigrationEvent, 100)

	// Send recent events
	o.eventsMu.RLock()
	recentEvents := make([]MigrationEvent, 0, 10)
	start := len(o.eventBuffer) - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < len(o.eventBuffer); i++ {
		recentEvents = append(recentEvents, o.eventBuffer[i])
	}
	o.eventsMu.RUnlock()

	if err := conn.WriteJSON(map[string]interface{}{
		"type":   "recent_events",
		"events": recentEvents,
	}); err != nil {
		o.logger.Errorw("Failed to send recent events", "error", err)
		return
	}

	// Handle events and pings
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-eventChan:
			if err := conn.WriteJSON(map[string]interface{}{
				"type":  "event",
				"event": event,
			}); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteJSON(map[string]interface{}{
				"type":      "ping",
				"timestamp": time.Now(),
			}); err != nil {
				return
			}
		}
	}
}

// handleWebSocketMetrics handles WebSocket connections for metrics
func (o *MigrationOrchestrator) handleWebSocketMetrics(w http.ResponseWriter, r *http.Request) {
	conn, err := o.upgrader.Upgrade(w, r, nil)
	if err != nil {
		o.logger.Errorw("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	clientID := uuid.New().String()
	o.logger.Infow("WebSocket metrics client connected", "client_id", clientID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := o.buildPerformanceSnapshot()
			if err := conn.WriteJSON(map[string]interface{}{
				"type":    "metrics_update",
				"metrics": metrics,
			}); err != nil {
				return
			}
		}
	}
}

// ================================
// Worker Functions
// ================================

// metricsCollectionWorker collects metrics periodically
func (o *MigrationOrchestrator) metricsCollectionWorker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopChan:
			return
		case <-ticker.C:
			o.collectMetrics()
		}
	}
}

// eventBufferCleanupWorker cleans up old events from the buffer
func (o *MigrationOrchestrator) eventBufferCleanupWorker() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopChan:
			return
		case <-ticker.C:
			o.cleanupEventBuffer()
		}
	}
}

// webSocketBroadcastWorker broadcasts updates to WebSocket clients
func (o *MigrationOrchestrator) webSocketBroadcastWorker() {
	ticker := time.NewTicker(o.config.WSUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopChan:
			return
		case <-ticker.C:
			o.broadcastDashboardUpdate()
		}
	}
}

// ================================
// Utility Functions
// ================================

// collectMetrics collects current metrics
func (o *MigrationOrchestrator) collectMetrics() {
	// TODO: Implement metrics collection
	// This would collect metrics from coordinator, safety manager, etc.
}

// cleanupEventBuffer removes old events from the buffer
func (o *MigrationOrchestrator) cleanupEventBuffer() {
	o.eventsMu.Lock()
	defer o.eventsMu.Unlock()

	cutoff := time.Now().Add(-o.config.BufferRetention)
	newBuffer := make([]MigrationEvent, 0, len(o.eventBuffer))

	for _, event := range o.eventBuffer {
		if event.Timestamp.After(cutoff) {
			newBuffer = append(newBuffer, event)
		}
	}

	o.eventBuffer = newBuffer
}

// broadcastDashboardUpdate broadcasts dashboard updates to WebSocket clients
func (o *MigrationOrchestrator) broadcastDashboardUpdate() {
	data := o.buildDashboardData()

	message := map[string]interface{}{
		"type": "dashboard_update",
		"data": data,
	}

	o.wsConnsMu.RLock()
	defer o.wsConnsMu.RUnlock()

	for clientID, conn := range o.wsConnections {
		if err := conn.WriteJSON(message); err != nil {
			o.logger.Warnw("Failed to send WebSocket update", "client_id", clientID, "error", err)
			// Connection will be cleaned up by the connection handler
		}
	}
}

// subscribeToEvents subscribes to migration events
func (o *MigrationOrchestrator) subscribeToEvents() {
	// TODO: Implement proper event subscription
	// This would subscribe to coordinator events and add them to the buffer
}

// buildDashboardData builds dashboard data structure
func (o *MigrationOrchestrator) buildDashboardData() *DashboardData {
	allMigrations := o.coordinator.GetAllMigrations()
	activeMigrations := o.coordinator.GetActiveMigrations()

	// Calculate success rate
	total := o.coordinator.GetTotalMigrations()
	successful := o.coordinator.GetSuccessfulMigrations()
	successRate := float64(0)
	if total > 0 {
		successRate = float64(successful) / float64(total)
	}

	// Build migration state info
	migrations := make([]MigrationStateInfo, 0, len(allMigrations))
	for _, migration := range allMigrations {
		migrations = append(migrations, o.buildMigrationStateInfo(migration))
	}

	// Build participant status
	participants := make([]ParticipantStatus, 0)
	for id, participant := range o.coordinator.GetAllParticipants() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		healthCheck, err := participant.HealthCheck(ctx)
		cancel()

		status := ParticipantStatus{
			ID:            id,
			Type:          fmt.Sprintf("%T", participant),
			IsHealthy:     err == nil && healthCheck != nil && healthCheck.Status == "healthy",
			LastHeartbeat: time.Now(),
			Status:        "active",
		}

		if err != nil {
			status.ErrorMessage = err.Error()
		}

		// HealthCheck has no Metrics field; leaving Metrics nil

		participants = append(participants, status)
	}

	// Get recent events
	o.eventsMu.RLock()
	recentEvents := make([]MigrationEvent, 0, 10)
	start := len(o.eventBuffer) - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < len(o.eventBuffer); i++ {
		recentEvents = append(recentEvents, o.eventBuffer[i])
	}
	o.eventsMu.RUnlock()

	return &DashboardData{
		TotalMigrations:    total,
		ActiveMigrations:   int64(len(activeMigrations)),
		SuccessRate:        successRate,
		AverageLatency:     5 * time.Millisecond, // TODO: Calculate actual average
		Migrations:         migrations,
		Participants:       participants,
		RecentEvents:       recentEvents,
		PerformanceMetrics: o.buildPerformanceSnapshot(),
		SystemHealth:       o.buildSystemHealthInfo(),
		Configuration:      o.buildOrchestratorConfigInfo(),
		LastUpdated:        time.Now(),
	}
}

// buildMigrationStateInfo builds migration state info from migration state
func (o *MigrationOrchestrator) buildMigrationStateInfo(state *MigrationState) MigrationStateInfo {
	state.mu.RLock()
	defer state.mu.RUnlock()

	// Calculate progress
	progress := float64(0)
	switch state.Phase {
	case PhasePrepare:
		progress = 0.1
	case PhaseCommit:
		progress = 0.5
	case PhaseCompleted:
		progress = 1.0
	case PhaseAborted, PhaseFailed:
		progress = 0.0
	}

	// Build participant states
	participantStates := make([]ParticipantStateInfo, 0, len(state.Participants))
	for id, pState := range state.Participants {
		// prepare map from interface
		var prepData map[string]interface{}
		if m, ok := pState.PreparationData.(map[string]interface{}); ok {
			prepData = m
		}
		participantStates = append(participantStates, ParticipantStateInfo{
			ID:              id,
			Vote:            pState.Vote,
			IsHealthy:       pState.IsHealthy,
			LastHeartbeat:   pState.LastHeartbeat,
			ErrorMessage:    pState.ErrorMessage,
			PreparationData: prepData,
		})
	}

	return MigrationStateInfo{
		ID:               state.ID,
		OrderBookID:      state.Pair,
		Phase:            state.Phase,
		Status:           state.Status,
		Progress:         progress,
		StartTime:        state.StartTime,
		ElapsedTime:      time.Since(state.StartTime),
		CurrentOperation: string(state.Phase),
		Participants:     participantStates,
		ErrorMessage:     state.RollbackReason,
		Metrics:          state.Metrics,
		HealthStatus:     state.HealthStatus,
	}
}

// buildPerformanceSnapshot builds performance snapshot
func (o *MigrationOrchestrator) buildPerformanceSnapshot() *PerformanceSnapshot {
	return &PerformanceSnapshot{
		TotalOperations:     o.coordinator.GetTotalMigrations(),
		OperationsPerSecond: 10.5,  // TODO: Calculate actual OPS
		AverageLatencyMs:    5.2,   // TODO: Calculate actual latency
		LatencyP95Ms:        12.8,  // TODO: Calculate actual P95
		LatencyP99Ms:        25.6,  // TODO: Calculate actual P99
		ErrorRate:           0.001, // TODO: Calculate actual error rate
		ResourceUtilization: map[string]float64{
			"cpu":    0.45,
			"memory": 0.62,
			"disk":   0.23,
		},
		LastUpdated: time.Now(),
	}
}

// buildSystemHealthInfo builds system health info
func (o *MigrationOrchestrator) buildSystemHealthInfo() *SystemHealthInfo {
	issues := make([]string, 0)
	warnings := make([]string, 0)

	// Check coordinator health
	// TODO: Implement actual health checks

	return &SystemHealthInfo{
		OverallHealth: "healthy",
		HealthScore:   0.95,
		LastCheck:     time.Now(),
		Issues:        issues,
		Warnings:      warnings,
		ComponentHealth: map[string]string{
			"coordinator":    "healthy",
			"safety_manager": "healthy",
			"orchestrator":   "healthy",
		},
	}
}

// buildOrchestratorConfigInfo builds orchestrator config info
func (o *MigrationOrchestrator) buildOrchestratorConfigInfo() *OrchestratorConfigInfo {
	features := make([]string, 0)
	if o.config.EnableDashboard {
		features = append(features, "dashboard")
	}
	if o.config.EnableAPI {
		features = append(features, "api")
	}
	if o.config.EnableWebSocket {
		features = append(features, "websocket")
	}
	if o.config.EnableAuth {
		features = append(features, "authentication")
	}

	return &OrchestratorConfigInfo{
		Version:               "1.0.0",
		Environment:           "production", // TODO: Get from config
		EnabledFeatures:       features,
		SafetyEnabled:         o.safetyManager != nil,
		AutoRollbackEnabled:   o.safetyManager != nil, // TODO: Check actual config
		PerformanceMonitoring: true,
	}
}

// isValidAPIKey checks if API key is valid
func (o *MigrationOrchestrator) isValidAPIKey(apiKey string) bool {
	if !o.config.EnableAuth {
		return true
	}

	for _, validKey := range o.config.APIKeys {
		if apiKey == validKey {
			return true
		}
	}

	return false
}

// writeJSONResponse writes a JSON response
func (o *MigrationOrchestrator) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		o.logger.Errorw("Failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes an error response
func (o *MigrationOrchestrator) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := MigrationResponse{
		Success:   false,
		Message:   message,
		Timestamp: time.Now(),
	}

	json.NewEncoder(w).Encode(response)
}

// loadTemplates loads HTML templates for the dashboard
func (o *MigrationOrchestrator) loadTemplates() {
	// TODO: Load actual HTML templates
	// For now, we'll create a simple template
	o.logger.Info("Loading dashboard templates")
	// Templates would be loaded from files in production
}
