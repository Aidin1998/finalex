package interfaces

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// APIHandler provides REST API endpoints for compliance services
type APIHandler struct {
	complianceService   ComplianceService
	monitoringService   MonitoringService
	manipulationService ManipulationService
	auditService        AuditService
	logger              *zap.Logger
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(
	complianceService ComplianceService,
	monitoringService MonitoringService,
	manipulationService ManipulationService,
	auditService AuditService,
	logger *zap.Logger,
) *APIHandler {
	return &APIHandler{
		complianceService:   complianceService,
		monitoringService:   monitoringService,
		manipulationService: manipulationService,
		auditService:        auditService,
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
	result, err := h.complianceService.CheckCompliance(ctx, request)
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
	alerts, err := h.monitoringService.GetAlerts(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to get alerts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get alerts"})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

// GetAlert gets a specific alert
func (h *APIHandler) GetAlert(c *gin.Context) {
	alertID := c.Param("alert_id")

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
	alertID := c.Param("alert_id")

	var request struct {
		Status     string `json:"status" binding:"required"`
		Resolution string `json:"resolution,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	ctx := c.Request.Context()
	err := h.monitoringService.UpdateAlertStatus(ctx, alertID, request.Status)
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
	err := h.monitoringService.GenerateAlert(ctx, alert)
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
	metrics := h.monitoringService.GetMetrics()
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
	// Similar to GetAlerts but for manipulation alerts
	ctx := c.Request.Context()
	alerts, err := h.manipulationService.GetAlerts(ctx, ManipulationAlertFilter{})
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
	err := h.complianceService.ProcessExternalReport(ctx, report)
	if err != nil {
		h.logger.Error("Failed to process external report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process report"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "report_id": report.ID})
}

// Helper functions
func parseIntQuery(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return defaultValue
}

// Additional handler methods would be implemented for:
// - GetManipulationAlert
// - UpdateManipulationAlertStatus
// - GetInvestigations
// - CreateInvestigation
// - UpdateInvestigation
// - GetPatterns
// - GetManipulationConfig
// - UpdateManipulationConfig
// - GetPolicies
// - CreatePolicy
// - UpdatePolicy
// - DeletePolicy
// - GetAuditEvents
// - GetAuditEvent
// - CreateAuditEvent
// - GetExternalStatus

// ExternalComplianceReport represents an external compliance report
type ExternalComplianceReport struct {
	ID          uuid.UUID              `json:"id"`
	Source      string                 `json:"source"`
	Type        string                 `json:"type"`
	UserID      *uuid.UUID             `json:"user_id,omitempty"`
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details"`
	Severity    string                 `json:"severity"`
	Timestamp   time.Time              `json:"timestamp"`
	ReporterID  string                 `json:"reporter_id"`
}

// ManipulationAlertFilter represents filter criteria for manipulation alerts
type ManipulationAlertFilter struct {
	UserID    *uuid.UUID            `json:"user_id,omitempty"`
	Market    string                `json:"market,omitempty"`
	AlertType ManipulationAlertType `json:"alert_type,omitempty"`
	Severity  AlertSeverity         `json:"severity,omitempty"`
	Status    AlertStatus           `json:"status,omitempty"`
	StartTime time.Time             `json:"start_time,omitempty"`
	EndTime   time.Time             `json:"end_time,omitempty"`
	Limit     int                   `json:"limit,omitempty"`
	Offset    int                   `json:"offset,omitempty"`
}
