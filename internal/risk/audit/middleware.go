package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AuditMiddleware creates middleware for comprehensive audit logging of administrative actions
func AuditMiddleware(auditService *AuditService, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip non-admin endpoints
		if !isAdminEndpoint(c.Request.URL.Path) {
			c.Next()
			return
		}

		startTime := time.Now()

		// Capture request data
		requestData := captureRequestData(c)

		// Create response writer wrapper to capture response
		responseWriter := &responseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = responseWriter

		// Process request
		c.Next()

		// Create audit event after request processing
		event := createAuditEvent(c, requestData, responseWriter, startTime)

		// Log audit event asynchronously
		go func() {
			ctx := context.Background()
			if err := auditService.LogEvent(ctx, event); err != nil {
				logger.Error("Failed to log audit event",
					zap.String("event_id", event.ID.String()),
					zap.Error(err))
			}
		}()
	}
}

// responseWriter wraps gin.ResponseWriter to capture response data
type responseWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// requestData captures important request information
type requestData struct {
	Headers     map[string]string `json:"headers"`
	Body        interface{}       `json:"body,omitempty"`
	QueryParams map[string]string `json:"query_params"`
	PathParams  map[string]string `json:"path_params"`
}

// isAdminEndpoint checks if the endpoint is an administrative endpoint
func isAdminEndpoint(path string) bool {
	adminPaths := []string{
		"/admin/",
		"/api/v1/admin/",
	}

	for _, adminPath := range adminPaths {
		if strings.Contains(path, adminPath) {
			return true
		}
	}

	return false
}

// captureRequestData captures relevant request data for audit logging
func captureRequestData(c *gin.Context) *requestData {
	data := &requestData{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
	}

	// Capture headers (excluding sensitive ones)
	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-api-key":     true,
	}

	for name, values := range c.Request.Header {
		if !sensitiveHeaders[strings.ToLower(name)] && len(values) > 0 {
			data.Headers[name] = values[0]
		}
	}

	// Capture query parameters
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			data.QueryParams[key] = values[0]
		}
	}

	// Capture path parameters
	for _, param := range c.Params {
		data.PathParams[param.Key] = param.Value
	}

	// Capture request body for certain methods
	if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
		if c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil && len(bodyBytes) > 0 {
				// Reset body for downstream handlers
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// Try to parse as JSON
				var jsonBody interface{}
				if err := json.Unmarshal(bodyBytes, &jsonBody); err == nil {
					data.Body = sanitizeRequestBody(jsonBody)
				} else {
					// Store as string if not JSON
					if len(bodyBytes) < 1024 { // Limit body size in audit logs
						data.Body = string(bodyBytes)
					}
				}
			}
		}
	}

	return data
}

// sanitizeRequestBody removes sensitive information from request body
func sanitizeRequestBody(body interface{}) interface{} {
	if bodyMap, ok := body.(map[string]interface{}); ok {
		sanitized := make(map[string]interface{})
		sensitiveFields := map[string]bool{
			"password":         true,
			"current_password": true,
			"new_password":     true,
			"token":            true,
			"secret":           true,
			"private_key":      true,
			"api_key":          true,
		}

		for key, value := range bodyMap {
			if sensitiveFields[strings.ToLower(key)] {
				sanitized[key] = "[REDACTED]"
			} else if nested, ok := value.(map[string]interface{}); ok {
				sanitized[key] = sanitizeRequestBody(nested)
			} else if array, ok := value.([]interface{}); ok {
				sanitizedArray := make([]interface{}, len(array))
				for i, item := range array {
					sanitizedArray[i] = sanitizeRequestBody(item)
				}
				sanitized[key] = sanitizedArray
			} else {
				sanitized[key] = value
			}
		}
		return sanitized
	}

	return body
}

// createAuditEvent creates an audit event from the request/response data
func createAuditEvent(c *gin.Context, requestData *requestData, responseWriter *responseWriter, startTime time.Time) *AuditEvent {
	// Get user information from context
	userID := getUserID(c)
	userRole := getUserRole(c)
	sessionID := getSessionID(c)

	// Determine event type and details based on endpoint and method
	eventType, category, action := determineEventType(c.Request.URL.Path, c.Request.Method)

	// Determine outcome based on status code
	outcome := determineOutcome(responseWriter.status)

	// Determine severity
	severity := determineSeverity(eventType, outcome, responseWriter.status)

	// Create the audit event
	event := &AuditEvent{
		ID:          uuid.New(),
		EventType:   eventType,
		Category:    category,
		Severity:    severity,
		ActorID:     userID,
		ActorType:   ActorTypeAdmin,
		ActorRole:   userRole,
		Action:      action,
		Outcome:     outcome,
		Timestamp:   startTime,
		SessionID:   sessionID,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		Description: generateDescription(eventType, action, outcome, requestData),
		Details: map[string]interface{}{
			"endpoint":      c.Request.URL.Path,
			"method":        c.Request.Method,
			"status_code":   responseWriter.status,
			"response_time": time.Since(startTime).Milliseconds(),
			"request_data":  requestData,
			"response_size": responseWriter.body.Len(),
		},
	}

	// Add target information if available
	addTargetInformation(event, c, requestData)

	// Add business impact assessment
	addBusinessImpact(event)

	// Add compliance flags
	addComplianceFlags(event)

	// Calculate risk score
	event.RiskScore = calculateRiskScore(event)

	// Add geolocation if available
	if geo := c.GetHeader("X-Forwarded-Geo"); geo != "" {
		event.Geolocation = &geo
	}

	// Add forensic data for critical events
	if event.Severity == SeverityCritical || event.Outcome == OutcomeFailure {
		addForensicInformation(event, c, responseWriter)
	}

	return event
}

// getUserID extracts user ID from context
func getUserID(c *gin.Context) uuid.UUID {
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(string); ok {
			if parsed, err := uuid.Parse(uid); err == nil {
				return parsed
			}
		}
		if uid, ok := userID.(uuid.UUID); ok {
			return uid
		}
	}
	if userID, exists := c.Get("userID"); exists {
		if uid, ok := userID.(string); ok {
			if parsed, err := uuid.Parse(uid); err == nil {
				return parsed
			}
		}
	}
	return uuid.Nil
}

// getUserRole extracts user role from context
func getUserRole(c *gin.Context) string {
	if role, exists := c.Get("user_role"); exists {
		if r, ok := role.(string); ok {
			return r
		}
	}
	if role, exists := c.Get("userRole"); exists {
		if r, ok := role.(string); ok {
			return r
		}
	}
	return "unknown"
}

// getSessionID extracts session ID from context
func getSessionID(c *gin.Context) *uuid.UUID {
	if sessionID, exists := c.Get("session_id"); exists {
		if sid, ok := sessionID.(string); ok {
			if parsed, err := uuid.Parse(sid); err == nil {
				return &parsed
			}
		}
		if sid, ok := sessionID.(uuid.UUID); ok {
			return &sid
		}
	}
	return nil
}

// determineEventType determines the event type based on endpoint and method
func determineEventType(path, method string) (EventType, EventCategory, string) {
	// User management endpoints
	if strings.Contains(path, "/admin/users") {
		switch method {
		case "POST":
			return EventUserCreated, CategoryUserManagement, "create_user"
		case "PUT", "PATCH":
			if strings.Contains(path, "/kyc") {
				return EventUserKYCUpdated, CategoryUserManagement, "update_user_kyc"
			}
			if strings.Contains(path, "/suspend") {
				return EventUserSuspended, CategoryUserManagement, "suspend_user"
			}
			if strings.Contains(path, "/role") {
				return EventUserRoleChanged, CategoryUserManagement, "change_user_role"
			}
			return EventUserUpdated, CategoryUserManagement, "update_user"
		case "DELETE":
			return EventUserDeleted, CategoryUserManagement, "delete_user"
		case "GET":
			return EventDataViewed, CategoryDataAccess, "view_user_data"
		}
	}

	// Risk management endpoints
	if strings.Contains(path, "/admin/risk") {
		if strings.Contains(path, "/limits") {
			switch method {
			case "POST":
				return EventRiskLimitCreated, CategoryRiskManagement, "create_risk_limit"
			case "PUT", "PATCH":
				return EventRiskLimitUpdated, CategoryRiskManagement, "update_risk_limit"
			case "DELETE":
				return EventRiskLimitDeleted, CategoryRiskManagement, "delete_risk_limit"
			case "GET":
				return EventDataViewed, CategoryDataAccess, "view_risk_limits"
			}
		}
		if strings.Contains(path, "/exemptions") {
			switch method {
			case "POST":
				return EventRiskExemptionCreated, CategoryRiskManagement, "create_risk_exemption"
			case "DELETE":
				return EventRiskExemptionDeleted, CategoryRiskManagement, "delete_risk_exemption"
			case "GET":
				return EventDataViewed, CategoryDataAccess, "view_risk_exemptions"
			}
		}
		if strings.Contains(path, "/compliance") {
			if strings.Contains(path, "/alerts") {
				switch method {
				case "PUT", "PATCH":
					return EventComplianceAlertCreated, CategoryCompliance, "update_compliance_alert"
				case "GET":
					return EventDataViewed, CategoryDataAccess, "view_compliance_alerts"
				}
			}
			if strings.Contains(path, "/rules") {
				return EventComplianceRuleAdded, CategoryCompliance, "add_compliance_rule"
			}
		}
	}

	// Rate limiting endpoints
	if strings.Contains(path, "/admin/rate-limits") {
		switch method {
		case "POST", "PUT", "PATCH":
			return EventRateLimitUpdated, CategorySystemAdmin, "update_rate_limits"
		case "GET":
			return EventDataViewed, CategoryDataAccess, "view_rate_limits"
		}
		if strings.Contains(path, "/emergency-mode") {
			return EventEmergencyMode, CategorySystemAdmin, "set_emergency_mode"
		}
	}

	// Trading pair management
	if strings.Contains(path, "/admin/trading/pairs") {
		switch method {
		case "POST":
			return EventSystemConfigChanged, CategorySystemAdmin, "create_trading_pair"
		case "PUT", "PATCH":
			return EventSystemConfigChanged, CategorySystemAdmin, "update_trading_pair"
		case "DELETE":
			return EventSystemConfigChanged, CategorySystemAdmin, "delete_trading_pair"
		case "GET":
			return EventDataViewed, CategoryDataAccess, "view_trading_pairs"
		}
	}

	// Default for admin endpoints
	if strings.Contains(path, "/admin/") {
		switch method {
		case "POST":
			return EventSystemConfigChanged, CategorySystemAdmin, "admin_create"
		case "PUT", "PATCH":
			return EventSystemConfigChanged, CategorySystemAdmin, "admin_update"
		case "DELETE":
			return EventSystemConfigChanged, CategorySystemAdmin, "admin_delete"
		case "GET":
			return EventDataViewed, CategoryDataAccess, "admin_view"
		}
	}

	return EventDataViewed, CategoryDataAccess, "unknown_action"
}

// determineOutcome determines the outcome based on HTTP status code
func determineOutcome(statusCode int) EventOutcome {
	if statusCode == 0 {
		statusCode = 200 // Default to success if not set
	}

	switch {
	case statusCode >= 200 && statusCode < 300:
		return OutcomeSuccess
	case statusCode >= 400 && statusCode < 500:
		return OutcomeFailure
	case statusCode >= 500:
		return OutcomeFailure
	case statusCode == 202:
		return OutcomePending
	default:
		return OutcomeSuccess
	}
}

// determineSeverity determines the severity based on event type and outcome
func determineSeverity(eventType EventType, outcome EventOutcome, statusCode int) SeverityLevel {
	// Critical events
	criticalEvents := map[EventType]bool{
		EventUserDeleted:            true,
		EventUserSuspended:          true,
		EventSystemShutdown:         true,
		EventEmergencyMode:          true,
		EventComplianceSARGenerated: true,
	}

	if criticalEvents[eventType] {
		return SeverityCritical
	}

	// Error events
	if outcome == OutcomeFailure {
		if statusCode >= 500 {
			return SeverityError
		}
		return SeverityWarning
	}

	// High-impact events
	highImpactEvents := map[EventType]bool{
		EventUserRoleChanged:     true,
		EventRiskLimitUpdated:    true,
		EventRiskLimitDeleted:    true,
		EventSystemConfigChanged: true,
		EventRateLimitUpdated:    true,
	}

	if highImpactEvents[eventType] {
		return SeverityWarning
	}

	return SeverityInfo
}

// generateDescription generates a human-readable description
func generateDescription(eventType EventType, action string, outcome EventOutcome, requestData *requestData) string {
	outcomeStr := ""
	if outcome != OutcomeSuccess {
		outcomeStr = fmt.Sprintf(" (%s)", outcome)
	}

	// Add target information if available
	targetInfo := ""
	if userID, exists := requestData.PathParams["id"]; exists {
		targetInfo = fmt.Sprintf(" for user %s", userID)
	} else if userID, exists := requestData.PathParams["userID"]; exists {
		targetInfo = fmt.Sprintf(" for user %s", userID)
	}

	return fmt.Sprintf("Administrative action: %s%s%s", action, targetInfo, outcomeStr)
}

// addTargetInformation adds target user/resource information
func addTargetInformation(event *AuditEvent, c *gin.Context, requestData *requestData) {
	// Try to extract target user ID from path parameters
	if userID, exists := requestData.PathParams["id"]; exists {
		if parsed, err := uuid.Parse(userID); err == nil {
			event.TargetID = &parsed
			targetType := "user"
			event.TargetType = &targetType
		}
	} else if userID, exists := requestData.PathParams["userID"]; exists {
		if parsed, err := uuid.Parse(userID); err == nil {
			event.TargetID = &parsed
			targetType := "user"
			event.TargetType = &targetType
		}
	}

	// Add resource information
	if strings.Contains(c.Request.URL.Path, "/limits/") {
		resourceType := "risk_limit"
		event.ResourceType = &resourceType
		if limitID, exists := requestData.PathParams["id"]; exists {
			event.ResourceID = &limitID
		}
	} else if strings.Contains(c.Request.URL.Path, "/exemptions/") {
		resourceType := "risk_exemption"
		event.ResourceType = &resourceType
		if userID, exists := requestData.PathParams["userID"]; exists {
			event.ResourceID = &userID
		}
	}
}

// addBusinessImpact assesses and adds business impact information
func addBusinessImpact(event *AuditEvent) {
	impact := &BusinessImpact{
		Categories: []string{},
	}

	switch event.EventType {
	case EventUserDeleted, EventUserSuspended:
		impact.Level = "high"
		impact.Description = "User account modification affecting trading capabilities"
		impact.Categories = []string{"operational", "compliance"}

	case EventRiskLimitUpdated, EventRiskLimitDeleted:
		impact.Level = "medium"
		impact.Description = "Risk management configuration change"
		impact.Categories = []string{"financial", "operational"}

	case EventSystemConfigChanged:
		impact.Level = "medium"
		impact.Description = "System configuration modification"
		impact.Categories = []string{"operational"}

	case EventEmergencyMode:
		impact.Level = "critical"
		impact.Description = "Emergency mode activation affecting all operations"
		impact.Categories = []string{"financial", "operational", "compliance"}

	default:
		impact.Level = "low"
		impact.Description = "Standard administrative operation"
		impact.Categories = []string{"operational"}
	}

	event.BusinessImpact = impact
}

// addComplianceFlags adds compliance-related flags
func addComplianceFlags(event *AuditEvent) {
	flags := []string{}

	// Add flags based on event type
	switch event.Category {
	case CategoryCompliance:
		flags = append(flags, "aml_kyc")

	case CategoryFinancial:
		flags = append(flags, "financial_operation")

	case CategoryUserManagement:
		flags = append(flags, "pii_access")

	case CategoryRiskManagement:
		flags = append(flags, "risk_control")
	}

	// Add flags based on severity
	if event.Severity == SeverityCritical {
		flags = append(flags, "high_risk_operation")
	}

	// Add regulatory flags
	if event.EventType == EventComplianceSARGenerated {
		flags = append(flags, "sar_filing")
	}

	event.ComplianceFlags = flags
}

// calculateRiskScore calculates a risk score for the event
func calculateRiskScore(event *AuditEvent) *float64 {
	score := 0.0

	// Base score by severity
	switch event.Severity {
	case SeverityInfo:
		score += 10
	case SeverityWarning:
		score += 30
	case SeverityError:
		score += 60
	case SeverityCritical:
		score += 90
	}

	// Increase score for failed operations
	if event.Outcome == OutcomeFailure {
		score += 20
	}

	// Increase score for high-impact categories
	switch event.Category {
	case CategoryCompliance, CategorySecurity:
		score += 15
	case CategoryFinancial, CategoryUserManagement:
		score += 10
	}

	// Increase score for certain event types
	highRiskEvents := map[EventType]bool{
		EventUserDeleted:      true,
		EventUserSuspended:    true,
		EventRiskLimitDeleted: true,
		EventEmergencyMode:    true,
	}

	if highRiskEvents[event.EventType] {
		score += 25
	}

	// Cap the score at 100
	if score > 100 {
		score = 100
	}

	return &score
}

// addForensicInformation adds detailed forensic information for critical events
func addForensicInformation(event *AuditEvent, c *gin.Context, responseWriter *responseWriter) {
	forensicData := &ForensicData{
		HTTPHeaders: make(map[string]string),
		Environment: make(map[string]string),
	}

	// Capture HTTP headers (excluding sensitive ones)
	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-api-key":     true,
	}

	for name, values := range c.Request.Header {
		if !sensitiveHeaders[strings.ToLower(name)] && len(values) > 0 {
			forensicData.HTTPHeaders[name] = values[0]
		}
	}

	// Add response information
	forensicData.ResponseStatus = &responseWriter.status

	// Add environment information
	forensicData.Environment["request_id"] = c.GetHeader("X-Request-ID")
	forensicData.Environment["trace_id"] = c.GetHeader("X-Trace-ID")

	event.ForensicData = forensicData
}
