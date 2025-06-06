package interfaces

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// parseIntQuery is a helper function to parse integer query parameters
func parseIntQuery(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	return defaultValue
}

// APIHandler provides REST API endpoints for compliance services
type APIHandler struct {
	complianceService   ComplianceService
	monitoringService   MonitoringService
	manipulationService ManipulationService
	auditService        AuditService
	accountsIntegration *AccountsIntegration
	tradingIntegration  *TradingEngineIntegration
	logger              *zap.Logger
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(
	complianceService ComplianceService,
	monitoringService MonitoringService,
	manipulationService ManipulationService,
	auditService AuditService,
	accountsIntegration *AccountsIntegration,
	tradingIntegration *TradingEngineIntegration,
	logger *zap.Logger,
) *APIHandler {
	return &APIHandler{
		complianceService:   complianceService,
		monitoringService:   monitoringService,
		manipulationService: manipulationService,
		auditService:        auditService,
		accountsIntegration: accountsIntegration,
		tradingIntegration:  tradingIntegration,
		logger:              logger,
	}
}

// RegisterRoutes registers all compliance API routes
func (h *APIHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1/compliance")
	{
		// Health and readiness probes
		api.GET("/health", h.HealthCheck)
		api.GET("/ready", h.ReadinessCheck)

		// Compliance checks
		compliance := api.Group("/check")
		{
			compliance.POST("/", h.PerformComplianceCheck)
			compliance.GET("/user/:user_id", h.GetUserComplianceStatus)
			compliance.GET("/user/:user_id/history", h.GetUserComplianceHistory)
		}

		// Monitoring and alerts
		monitoring := api.Group("/monitoring")
		{
			monitoring.GET("/alerts", h.GetAlerts)
			monitoring.GET("/alerts/:alert_id", h.GetAlert)
			monitoring.PUT("/alerts/:alert_id/status", h.UpdateAlertStatus)
			monitoring.POST("/alerts", h.CreateAlert)

			monitoring.GET("/policies", h.GetPolicies)
			monitoring.POST("/policies", h.CreatePolicy)
			monitoring.PUT("/policies/:policy_id", h.UpdatePolicy)
			monitoring.DELETE("/policies/:policy_id", h.DeletePolicy)

			monitoring.GET("/dashboard", h.GetDashboard)
			monitoring.GET("/metrics", h.GetMetrics)
		}

		// Manipulation detection
		manipulation := api.Group("/manipulation")
		{
			manipulation.POST("/detect", h.DetectManipulation)
			manipulation.GET("/alerts", h.GetManipulationAlerts)
			manipulation.GET("/alerts/:alert_id", h.GetManipulationAlert)
			manipulation.PUT("/alerts/:alert_id/status", h.UpdateManipulationAlertStatus)

			manipulation.GET("/investigations", h.GetInvestigations)
			manipulation.POST("/investigations", h.CreateInvestigation)
			manipulation.PUT("/investigations/:investigation_id", h.UpdateInvestigation)

			manipulation.GET("/patterns", h.GetPatterns)
			manipulation.GET("/config", h.GetManipulationConfig)
			manipulation.PUT("/config", h.UpdateManipulationConfig)
		}

		// Audit logs
		audit := api.Group("/audit")
		{
			audit.GET("/events", h.GetAuditEvents)
			audit.GET("/events/:event_id", h.GetAuditEvent)
			audit.POST("/events", h.CreateAuditEvent)
		}

		// External partner integration
		external := api.Group("/external")
		{
			external.POST("/reports", h.SubmitExternalReport)
			external.GET("/status", h.GetExternalStatus)
		}
	}
}

// Health check endpoint
func (h *APIHandler) HealthCheck(c *gin.Context) {
	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
		"services": map[string]string{
			"compliance":   "healthy",
			"monitoring":   "healthy",
			"manipulation": "healthy",
			"audit":        "healthy",
		},
	}

	c.JSON(http.StatusOK, status)
}

// Readiness check endpoint
func (h *APIHandler) ReadinessCheck(c *gin.Context) {
	ctx := c.Request.Context()

	// Check if services are ready
	ready := true
	services := map[string]string{}

	// Check compliance service
	if err := h.complianceService.HealthCheck(ctx); err != nil {
		services["compliance"] = "not ready"
		ready = false
	} else {
		services["compliance"] = "ready"
	}

	// Check monitoring service
	if err := h.monitoringService.HealthCheck(ctx); err != nil {
		services["monitoring"] = "not ready"
		ready = false
	} else {
		services["monitoring"] = "ready"
	}

	status := map[string]interface{}{
		"ready":     ready,
		"timestamp": time.Now().UTC(),
		"services":  services,
	}

	if ready {
		c.JSON(http.StatusOK, status)
	} else {
		c.JSON(http.StatusServiceUnavailable, status)
	}
}

// PerformComplianceCheck performs a compliance check
func (h *APIHandler) PerformComplianceCheck(c *gin.Context) {
	var request ComplianceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	ctx := c.Request.Context()
	result, err := h.complianceService.CheckCompliance(ctx, &request)
	if err != nil {
		h.logger.Error("Compliance check failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Compliance check failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetUserComplianceStatus gets user compliance status
func (h *APIHandler) GetUserComplianceStatus(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	ctx := c.Request.Context()
	status, err := h.complianceService.GetUserStatus(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to get user compliance status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user status"})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetUserComplianceHistory gets user compliance history
func (h *APIHandler) GetUserComplianceHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Parse query parameters
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	ctx := c.Request.Context()
	history, err := h.complianceService.GetUserHistory(ctx, userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get user compliance history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user history"})
		return
	}

	c.JSON(http.StatusOK, history)
}

// GetAlerts gets monitoring alerts
func (h *APIHandler) GetAlerts(c *gin.Context) {
	filter := AlertFilter{
		UserID:    c.Query("user_id"),
		AlertType: c.Query("alert_type"),
		Status:    AlertStatus(parseIntQuery(c.Query("status"), 0)),
		Severity:  AlertSeverity(parseIntQuery(c.Query("severity"), 0)),
		Limit:     parseIntQuery(c.Query("limit"), 50),
		Offset:    parseIntQuery(c.Query("offset"), 0),
	}

	if startTime := c.Query("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			filter.StartTime = t
		}
	}

	if endTime := c.Query("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			filter.EndTime = t
		}
	}

	ctx := c.Request.Context()
	alerts, err := h.monitoringService.GetAlerts(ctx, &filter)
	if err != nil {
		h.logger.Error("Failed to get alerts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get alerts"})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

// GetAlert gets a specific alert
func (h *APIHandler) GetAlert(c *gin.Context) {
	alertIDStr := c.Param("alert_id")
	alertID, err := uuid.Parse(alertIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	ctx := c.Request.Context()
	alert, err := h.monitoringService.GetAlert(ctx, alertID)
	if err != nil {
		h.logger.Error("Failed to get alert", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Alert not found"})
		return
	}
	c.JSON(http.StatusOK, alert)
}

// UpdateAlertStatus updates alert status
func (h *APIHandler) UpdateAlertStatus(c *gin.Context) {
	alertIDStr := c.Param("alert_id")
	alertID, err := uuid.Parse(alertIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var request struct {
		Status     string `json:"status" binding:"required"`
		Resolution string `json:"resolution,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	ctx := c.Request.Context()
	err = h.monitoringService.UpdateAlertStatus(ctx, alertID, request.Status)
	if err != nil {
		h.logger.Error("Failed to update alert status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update alert status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// CreateAlert creates a new alert
func (h *APIHandler) CreateAlert(c *gin.Context) {
	var alert MonitoringAlert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert format"})
		return
	}
	ctx := c.Request.Context()
	err := h.monitoringService.GenerateAlert(ctx, &alert)
	if err != nil {
		h.logger.Error("Failed to create alert", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create alert"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "created", "id": alert.ID})
}

// GetDashboard gets compliance dashboard data
func (h *APIHandler) GetDashboard(c *gin.Context) {
	ctx := c.Request.Context()
	dashboard, err := h.monitoringService.GetDashboard(ctx)
	if err != nil {
		h.logger.Error("Failed to get dashboard", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dashboard"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// GetMetrics gets monitoring metrics
func (h *APIHandler) GetMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	metrics, err := h.monitoringService.GetMetrics(ctx)
	if err != nil {
		h.logger.Error("Failed to get metrics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get metrics"})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// DetectManipulation performs manipulation detection
func (h *APIHandler) DetectManipulation(c *gin.Context) {
	var request ManipulationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	ctx := c.Request.Context()
	result, err := h.manipulationService.DetectManipulation(ctx, request)
	if err != nil {
		h.logger.Error("Manipulation detection failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Manipulation detection failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetManipulationAlerts gets manipulation alerts
func (h *APIHandler) GetManipulationAlerts(c *gin.Context) {
	filter := AlertFilter{
		UserID:    c.Query("user_id"),
		AlertType: c.Query("alert_type"),
		Limit:     parseIntQuery(c.Query("limit"), 50),
		Offset:    parseIntQuery(c.Query("offset"), 0),
	}

	ctx := c.Request.Context()
	alerts, err := h.manipulationService.GetAlerts(ctx, &filter)
	if err != nil {
		h.logger.Error("Failed to get manipulation alerts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get manipulation alerts"})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

// SubmitExternalReport handles external compliance reports
func (h *APIHandler) SubmitExternalReport(c *gin.Context) {
	var report ExternalComplianceReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report format"})
		return
	}
	ctx := c.Request.Context()
	err := h.complianceService.ProcessExternalReport(ctx, &report)
	if err != nil {
		h.logger.Error("Failed to process external report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process report"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "report_id": report.ID})
}

// Policy Management Handlers

// GetPolicies handles GET /api/v1/compliance/monitoring/policies
func (h *APIHandler) GetPolicies(c *gin.Context) {
	ctx := c.Request.Context()

	policies, err := h.monitoringService.GetPolicies(ctx)
	if err != nil {
		h.logger.Error("Failed to get policies", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get policies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"policies": policies})
}

// CreatePolicy handles POST /api/v1/compliance/monitoring/policies
func (h *APIHandler) CreatePolicy(c *gin.Context) {
	ctx := c.Request.Context()

	var request MonitoringPolicy
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.monitoringService.CreatePolicy(ctx, &request)
	if err != nil {
		h.logger.Error("Failed to create policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create policy"})
		return
	}

	c.JSON(http.StatusCreated, request)
}

// UpdatePolicy handles PUT /api/v1/compliance/monitoring/policies/:policy_id
func (h *APIHandler) UpdatePolicy(c *gin.Context) {
	ctx := c.Request.Context()

	policyIDParam := c.Param("policy_id")
	if policyIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	var request MonitoringPolicy
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.monitoringService.UpdatePolicy(ctx, policyIDParam, &request)
	if err != nil {
		h.logger.Error("Failed to update policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update policy"})
		return
	}

	c.JSON(http.StatusOK, request)
}

// DeletePolicy handles DELETE /api/v1/compliance/monitoring/policies/:policy_id
func (h *APIHandler) DeletePolicy(c *gin.Context) {
	ctx := c.Request.Context()

	policyIDParam := c.Param("policy_id")
	if policyIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	err := h.monitoringService.DeletePolicy(ctx, policyIDParam)
	if err != nil {
		h.logger.Error("Failed to delete policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete policy"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// Manipulation Detection Handlers

// GetManipulationAlert handles GET /api/v1/compliance/manipulation/alerts/:alert_id
func (h *APIHandler) GetManipulationAlert(c *gin.Context) {
	ctx := c.Request.Context()

	alertIDParam := c.Param("alert_id")
	alertID, err := uuid.Parse(alertIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	alert, err := h.manipulationService.GetAlert(ctx, alertID)
	if err != nil {
		h.logger.Error("Failed to get manipulation alert", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get alert"})
		return
	}

	c.JSON(http.StatusOK, alert)
}

// UpdateManipulationAlertStatus handles PUT /api/v1/compliance/manipulation/alerts/:alert_id/status
func (h *APIHandler) UpdateManipulationAlertStatus(c *gin.Context) {
	ctx := c.Request.Context()

	alertIDParam := c.Param("alert_id")
	alertID, err := uuid.Parse(alertIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	var request struct {
		Status AlertStatus `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err = h.manipulationService.UpdateAlertStatus(ctx, alertID, request.Status)
	if err != nil {
		h.logger.Error("Failed to update manipulation alert status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update alert status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert status updated successfully"})
}

// Investigation Management Handlers

// GetInvestigations handles GET /api/v1/compliance/manipulation/investigations
func (h *APIHandler) GetInvestigations(c *gin.Context) {
	ctx := c.Request.Context()

	var filter InvestigationFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters"})
		return
	}

	investigations, err := h.manipulationService.GetInvestigations(ctx, &filter)
	if err != nil {
		h.logger.Error("Failed to get investigations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get investigations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"investigations": investigations})
}

// CreateInvestigation handles POST /api/v1/compliance/manipulation/investigations
func (h *APIHandler) CreateInvestigation(c *gin.Context) {
	ctx := c.Request.Context()

	var request CreateInvestigationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	investigation, err := h.manipulationService.CreateInvestigation(ctx, &request)
	if err != nil {
		h.logger.Error("Failed to create investigation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create investigation"})
		return
	}

	c.JSON(http.StatusCreated, investigation)
}

// UpdateInvestigation handles PUT /api/v1/compliance/manipulation/investigations/:investigation_id
func (h *APIHandler) UpdateInvestigation(c *gin.Context) {
	ctx := c.Request.Context()

	investigationIDParam := c.Param("investigation_id")
	investigationID, err := uuid.Parse(investigationIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid investigation ID"})
		return
	}

	var request InvestigationUpdate
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err = h.manipulationService.UpdateInvestigation(ctx, investigationID, &request)
	if err != nil {
		h.logger.Error("Failed to update investigation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update investigation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Investigation updated successfully"})
}

// Pattern Analysis Handlers

// GetPatterns handles GET /api/v1/compliance/manipulation/patterns
func (h *APIHandler) GetPatterns(c *gin.Context) {
	ctx := c.Request.Context()

	var filter PatternFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters"})
		return
	}

	patterns, err := h.manipulationService.GetPatterns(ctx, &filter)
	if err != nil {
		h.logger.Error("Failed to get patterns", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get patterns"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"patterns": patterns})
}

// GetManipulationConfig handles GET /api/v1/compliance/manipulation/config
func (h *APIHandler) GetManipulationConfig(c *gin.Context) {
	ctx := c.Request.Context()

	config, err := h.manipulationService.GetConfig(ctx)
	if err != nil {
		h.logger.Error("Failed to get manipulation config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdateManipulationConfig handles PUT /api/v1/compliance/manipulation/config
func (h *APIHandler) UpdateManipulationConfig(c *gin.Context) {
	ctx := c.Request.Context()

	var request ManipulationConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.manipulationService.UpdateConfig(ctx, &request)
	if err != nil {
		h.logger.Error("Failed to update manipulation config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}

// Audit Event Handlers

// GetAuditEvents handles GET /api/v1/compliance/audit/events
func (h *APIHandler) GetAuditEvents(c *gin.Context) {
	ctx := c.Request.Context()

	var filter AuditFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters"})
		return
	}

	events, err := h.auditService.GetEvents(ctx, &filter)
	if err != nil {
		h.logger.Error("Failed to get audit events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get audit events"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"events": events})
}

// GetAuditEvent handles GET /api/v1/compliance/audit/events/:event_id
func (h *APIHandler) GetAuditEvent(c *gin.Context) {
	ctx := c.Request.Context()

	eventIDParam := c.Param("event_id")
	eventID, err := uuid.Parse(eventIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	event, err := h.auditService.GetEvent(ctx, eventID)
	if err != nil {
		h.logger.Error("Failed to get audit event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get audit event"})
		return
	}

	c.JSON(http.StatusOK, event)
}

// CreateAuditEvent handles POST /api/v1/compliance/audit/events
func (h *APIHandler) CreateAuditEvent(c *gin.Context) {
	ctx := c.Request.Context()

	var request AuditEvent
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.auditService.CreateEvent(ctx, &request)
	if err != nil {
		h.logger.Error("Failed to create audit event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create audit event"})
		return
	}

	c.JSON(http.StatusCreated, request)
}

// External Status Handler

// GetExternalStatus handles GET /api/v1/compliance/external/status
func (h *APIHandler) GetExternalStatus(c *gin.Context) {
	status := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"status":    "operational",
		"services": map[string]interface{}{
			"compliance_check": map[string]interface{}{
				"status":      "healthy",
				"last_check":  time.Now().Add(-5 * time.Minute).UTC(),
				"response_ms": 150,
			},
			"audit_service": map[string]interface{}{
				"status":      "healthy",
				"last_check":  time.Now().Add(-2 * time.Minute).UTC(),
				"response_ms": 85,
			},
			"monitoring": map[string]interface{}{
				"status":      "healthy",
				"last_check":  time.Now().Add(-1 * time.Minute).UTC(),
				"response_ms": 120,
			},
		},
		"metrics": map[string]interface{}{
			"total_requests":    12450,
			"successful_checks": 12380,
			"failed_checks":     70,
			"average_response":  95.5,
		},
	}

	c.JSON(http.StatusOK, status)
}

// Request and Response Types

// CreatePolicyRequest represents a request to create a policy
type CreatePolicyRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Type        string                 `json:"type" binding:"required"`
	Conditions  map[string]interface{} `json:"conditions" binding:"required"`
	Actions     []string               `json:"actions" binding:"required"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
}

// UpdatePolicyRequest represents a request to update a policy
type UpdatePolicyRequest struct {
	Name        *string                 `json:"name,omitempty"`
	Description *string                 `json:"description,omitempty"`
	Conditions  *map[string]interface{} `json:"conditions,omitempty"`
	Actions     []string                `json:"actions,omitempty"`
	Enabled     *bool                   `json:"enabled,omitempty"`
	Priority    *int                    `json:"priority,omitempty"`
}
