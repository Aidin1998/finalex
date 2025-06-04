package audit

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handlers provides HTTP handlers for audit log operations
type Handlers struct {
	logger         *zap.Logger
	auditService   *AuditService
	forensicEngine *ForensicService
}

// NewHandlers creates a new audit handlers instance
func NewHandlers(logger *zap.Logger, auditService *AuditService, forensicEngine *ForensicService) *Handlers {
	return &Handlers{
		logger:         logger,
		auditService:   auditService,
		forensicEngine: forensicEngine,
	}
}

// SearchRequest represents the audit log search request
type SearchRequest struct {
	UserID     string    `json:"user_id,omitempty"`
	ActionType string    `json:"action_type,omitempty"`
	Resource   string    `json:"resource,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty"`
	StartTime  time.Time `json:"start_time,omitempty"`
	EndTime    time.Time `json:"end_time,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Offset     int       `json:"offset,omitempty"`
}

// SearchAuditLogs handles searching audit logs
func (h *Handlers) SearchAuditLogs(c *gin.Context) {
	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}

	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	// Build forensic query
	var actorIDs []uuid.UUID
	if req.UserID != "" {
		if id, err := uuid.Parse(req.UserID); err == nil {
			actorIDs = []uuid.UUID{id}
		}
	}
	fq := ForensicQuery{
		StartTime:   &req.StartTime,
		EndTime:     &req.EndTime,
		ActorIDs:    actorIDs,
		Actions:     []string{req.ActionType},
		ResourceIDs: []string{req.Resource},
		ClientIPs:   []string{req.IPAddress},
		Offset:      req.Offset,
		Limit:       req.Limit,
	}

	events, total, err := h.forensicEngine.SearchEvents(c.Request.Context(), fq)
	if err != nil {
		h.logger.Error("Search failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search_failed", "message": err.Error()})
		return
	}

	// Pagination
	page := (req.Offset/req.Limit + 1)
	totalPages := int((total + int64(req.Limit) - 1) / int64(req.Limit))

	c.JSON(http.StatusOK, gin.H{
		"events":      events,
		"total":       total,
		"page":        page,
		"page_size":   req.Limit,
		"total_pages": totalPages,
	})
}

// @Summary Get audit event by ID
// @Description Retrieve a specific audit event by its ID
// @Tags audit
// @Produce json
// @Param id path string true "Event ID"
// @Success 200 {object} AuditEvent
// @Failure 400 {object} gin.H
// @Failure 404 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /admin/audit/events/{id} [get]
func (h *Handlers) GetAuditEvent(c *gin.Context) {
	eventID := c.Param("id")
	if eventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Event ID is required",
		})
		return
	}

	event, err := h.auditService.GetEventByID(c.Request.Context(), eventID)
	if err != nil {
		h.logger.Error("Failed to get audit event", zap.String("event_id", eventID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "event_not_found",
			"message": "Audit event not found",
		})
		return
	}

	c.JSON(http.StatusOK, event)
}

// TimelineRequest represents the timeline analysis request
type TimelineRequest struct {
	UserID    string    `json:"user_id" binding:"required"`
	StartTime time.Time `json:"start_time" binding:"required"`
	EndTime   time.Time `json:"end_time" binding:"required"`
}

// @Summary Generate user timeline
// @Description Generate a chronological timeline of user activities
// @Tags audit
// @Accept json
// @Produce json
// @Param request body TimelineRequest true "Timeline request"
// @Success 200 {object} UserTimeline
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /admin/audit/timeline [post]
func (h *Handlers) GenerateUserTimeline(c *gin.Context) {
	var req TimelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}

	// Parse actor ID
	actorID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id", "message": "User ID must be a valid UUID"})
		return
	}

	// Call service
	timeline, err := h.forensicEngine.GetTimeline(c.Request.Context(), actorID, req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Error("Failed to generate timeline", zap.String("user_id", req.UserID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "timeline_failed", "message": "Failed to generate user timeline"})
		return
	}

	c.JSON(http.StatusOK, timeline)
}

// PatternRequest represents the pattern detection request
type PatternRequest struct {
	UserID    string    `json:"user_id,omitempty"`
	StartTime time.Time `json:"start_time" binding:"required"`
	EndTime   time.Time `json:"end_time" binding:"required"`
	Patterns  []string  `json:"patterns,omitempty"` // specific patterns to check
}

// @Summary Detect suspicious patterns
// @Description Analyze audit logs for suspicious behavior patterns
// @Tags audit
// @Accept json
// @Produce json
// @Param request body PatternRequest true "Pattern detection request"
// @Success 200 {object} []SuspiciousPattern
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /admin/audit/patterns [post]
func (h *Handlers) DetectSuspiciousPatterns(c *gin.Context) {
	var req PatternRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}

	// Use lookback period between times
	lookback := req.EndTime.Sub(req.StartTime)
	patterns, err := h.forensicEngine.DetectPatterns(c.Request.Context(), lookback)
	if err != nil {
		h.logger.Error("Failed to detect patterns", zap.String("user_id", req.UserID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pattern_detection_failed", "message": "Failed to detect suspicious patterns"})
		return
	}

	c.JSON(http.StatusOK, patterns)
}

// @Summary Get audit statistics
// @Description Get statistical summary of audit events
// @Tags audit
// @Produce json
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param user_id query string false "User ID filter"
// @Success 200 {object} AuditStatistics
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /admin/audit/statistics [get]
func (h *Handlers) GetAuditStatistics(c *gin.Context) {
	var startTime, endTime time.Time
	var err error

	// Parse query parameters
	if startStr := c.Query("start_time"); startStr != "" {
		startTime, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_time_format",
				"message": "Invalid start_time format, use RFC3339",
			})
			return
		}
	} else {
		startTime = time.Now().Add(-24 * time.Hour) // Default to last 24 hours
	}

	if endStr := c.Query("end_time"); endStr != "" {
		endTime, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_time_format",
				"message": "Invalid end_time format, use RFC3339",
			})
			return
		}
	} else {
		endTime = time.Now()
	}

	userID := c.Query("user_id")

	stats, err := h.forensicEngine.GetStatistics(c.Request.Context(), userID, startTime, endTime)
	if err != nil {
		h.logger.Error("Failed to get audit statistics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "statistics_failed",
			"message": "Failed to get audit statistics",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// IntegrityRequest represents the integrity verification request
type IntegrityRequest struct {
	StartTime time.Time `json:"start_time" binding:"required"`
	EndTime   time.Time `json:"end_time" binding:"required"`
}

// IntegrityResponse represents the integrity verification response
type IntegrityResponse struct {
	Valid          bool                  `json:"valid"`
	TotalEvents    int64                 `json:"total_events"`
	VerifiedEvents int64                 `json:"verified_events"`
	Violations     []IntegrityViolation  `json:"violations,omitempty"`
	Summary        IntegrityCheckSummary `json:"summary"`
}

// IntegrityViolation represents a detected integrity violation
type IntegrityViolation struct {
	EventID       string    `json:"event_id"`
	Timestamp     time.Time `json:"timestamp"`
	ViolationType string    `json:"violation_type"`
	Description   string    `json:"description"`
	Severity      string    `json:"severity"`
}

// IntegrityCheckSummary provides a summary of the integrity check
type IntegrityCheckSummary struct {
	CheckDuration      time.Duration `json:"check_duration"`
	HashMismatches     int           `json:"hash_mismatches"`
	ChainBreaks        int           `json:"chain_breaks"`
	TimestampAnomalies int           `json:"timestamp_anomalies"`
	EncryptionErrors   int           `json:"encryption_errors"`
}

// @Summary Verify audit log integrity
// @Description Verify the cryptographic integrity of audit logs
// @Tags audit
// @Accept json
// @Produce json
// @Param request body IntegrityRequest true "Integrity check request"
// @Success 200 {object} IntegrityResponse
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /admin/audit/integrity [post]
func (h *Handlers) VerifyIntegrity(c *gin.Context) {
	var req IntegrityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Invalid integrity request format",
			"details": err.Error(),
		})
		return
	}

	startCheck := time.Now()

	// Verify integrity
	report, err := h.auditService.VerifyIntegrity(c.Request.Context(), req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Error("Failed to verify integrity", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "integrity_check_failed",
			"message": "Failed to verify audit log integrity",
		})
		return
	}

	checkDuration := time.Since(startCheck)

	// Map integrity issues to response format
	responseViolations := make([]IntegrityViolation, len(report.Issues))
	for i, issue := range report.Issues {
		responseViolations[i] = IntegrityViolation{
			EventID:       issue.EventID.String(),
			Timestamp:     issue.DetectedAt,
			ViolationType: issue.IssueType,
			Description:   issue.Description,
			Severity:      issue.Severity,
		}
	}

	// Calculate summary statistics
	summary := IntegrityCheckSummary{
		CheckDuration: checkDuration,
	}
	for _, issue := range report.Issues {
		switch issue.IssueType {
		case "hash_mismatch":
			summary.HashMismatches++
		case "chain_break":
			summary.ChainBreaks++
		case "timestamp_anomaly":
			summary.TimestampAnomalies++
		case "encryption_error":
			summary.EncryptionErrors++
		}
	}

	// Build response
	response := IntegrityResponse{
		Valid:          report.InvalidEvents == 0,
		TotalEvents:    int64(report.TotalEvents),
		VerifiedEvents: int64(report.ValidEvents),
		Violations:     responseViolations,
		Summary:        summary,
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Export audit logs
// @Description Export audit logs in various formats (JSON, CSV)
// @Tags audit
// @Produce json
// @Param format query string false "Export format (json, csv)" default(json)
// @Param start_time query string true "Start time (RFC3339)"
// @Param end_time query string true "End time (RFC3339)"
// @Param user_id query string false "User ID filter"
// @Param action_type query string false "Action type filter"
// @Success 200 {file} file
// @Failure 400 {object} gin.H
// @Failure 500 {object} gin.H
// @Router /admin/audit/export [get]
func (h *Handlers) ExportAuditLogs(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	userID := c.Query("user_id")
	actionType := c.Query("action_type")
	resource := c.Query("resource")

	if startTimeStr == "" || endTimeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_parameters", "message": "start_time and end_time are required"})
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_time_format", "message": "Invalid start_time format, use RFC3339"})
		return
	}
	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_time_format", "message": "Invalid end_time format, use RFC3339"})
		return
	}

	// Build forensic query
	var actorIDs []uuid.UUID
	if userID != "" {
		if id, err := uuid.Parse(userID); err == nil {
			actorIDs = []uuid.UUID{id}
		}
	}
	fq := ForensicQuery{
		StartTime:   &startTime,
		EndTime:     &endTime,
		ActorIDs:    actorIDs,
		Actions:     []string{actionType},
		ResourceIDs: []string{resource},
		Offset:      0,
		Limit:       10000,
	}

	// Export audit logs
	data, contentType, filename, err := h.forensicEngine.ExportLogs(c.Request.Context(), fq, format)
	if err != nil {
		h.logger.Error("Failed to export audit logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "export_failed", "message": "Failed to export audit logs"})
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, data)
}

// RegisterRoutes registers audit handlers with the router
func (h *Handlers) RegisterRoutes(router gin.IRouter) {
	audit := router.Group("/audit")
	{
		audit.POST("/search", h.SearchAuditLogs)
		audit.GET("/events/:id", h.GetAuditEvent)
		audit.POST("/timeline", h.GenerateUserTimeline)
		audit.POST("/patterns", h.DetectSuspiciousPatterns)
		audit.GET("/statistics", h.GetAuditStatistics)
		audit.POST("/integrity", h.VerifyIntegrity)
		audit.GET("/export", h.ExportAuditLogs)
	}
}
