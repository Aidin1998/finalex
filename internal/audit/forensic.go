package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ForensicService provides forensic analysis and retrieval capabilities for audit logs
type ForensicService struct {
	db           *gorm.DB
	auditService *AuditService
	logger       *zap.Logger
}

// ForensicQuery represents a forensic search query
type ForensicQuery struct {
	// Time range
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`

	// Actors
	ActorIDs  []uuid.UUID `json:"actor_ids,omitempty"`
	ActorRole *string     `json:"actor_role,omitempty"`

	// Events
	EventTypes []EventType     `json:"event_types,omitempty"`
	Categories []EventCategory `json:"categories,omitempty"`
	Severity   []SeverityLevel `json:"severity,omitempty"`
	Outcomes   []EventOutcome  `json:"outcomes,omitempty"`

	// Targets and resources
	TargetIDs    []uuid.UUID `json:"target_ids,omitempty"`
	ResourceIDs  []string    `json:"resource_ids,omitempty"`
	ResourceType *string     `json:"resource_type,omitempty"`

	// Network and session
	ClientIPs  []string    `json:"client_ips,omitempty"`
	SessionIDs []uuid.UUID `json:"session_ids,omitempty"`

	// Content search
	SearchText *string  `json:"search_text,omitempty"`
	Actions    []string `json:"actions,omitempty"`

	// Risk and compliance
	MinRiskScore    *float64 `json:"min_risk_score,omitempty"`
	MaxRiskScore    *float64 `json:"max_risk_score,omitempty"`
	ComplianceFlags []string `json:"compliance_flags,omitempty"`

	// Business impact
	BusinessImpactLevels []string `json:"business_impact_levels,omitempty"`

	// Pagination
	Offset int `json:"offset"`
	Limit  int `json:"limit"`

	// Sorting
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"` // "asc" or "desc"
}

// ForensicResult represents the result of a forensic search
type ForensicResult struct {
	Events     []AuditEvent   `json:"events"`
	TotalCount int64          `json:"total_count"`
	Query      ForensicQuery  `json:"query"`
	ExecutedAt time.Time      `json:"executed_at"`
	Duration   time.Duration  `json:"duration"`
	Summary    *SearchSummary `json:"summary"`
}

// SearchSummary provides statistical summary of search results
type SearchSummary struct {
	EventsByType     map[EventType]int     `json:"events_by_type"`
	EventsByCategory map[EventCategory]int `json:"events_by_category"`
	EventsBySeverity map[SeverityLevel]int `json:"events_by_severity"`
	EventsByOutcome  map[EventOutcome]int  `json:"events_by_outcome"`
	UniqueActors     int                   `json:"unique_actors"`
	UniqueSessions   int                   `json:"unique_sessions"`
	UniqueIPs        int                   `json:"unique_ips"`
	TimeSpan         time.Duration         `json:"time_span"`
	AverageRiskScore float64               `json:"average_risk_score"`
}

// Timeline represents a timeline of related events
type Timeline struct {
	Events     []TimelineEvent `json:"events"`
	ActorID    uuid.UUID       `json:"actor_id"`
	StartTime  time.Time       `json:"start_time"`
	EndTime    time.Time       `json:"end_time"`
	Duration   time.Duration   `json:"duration"`
	EventCount int             `json:"event_count"`
	RiskScore  float64         `json:"risk_score"`
	Summary    string          `json:"summary"`
}

// TimelineEvent represents an event in a timeline
type TimelineEvent struct {
	ID          uuid.UUID    `json:"id"`
	EventType   EventType    `json:"event_type"`
	Action      string       `json:"action"`
	Outcome     EventOutcome `json:"outcome"`
	Timestamp   time.Time    `json:"timestamp"`
	Description string       `json:"description"`
	RiskScore   *float64     `json:"risk_score,omitempty"`
	TargetID    *uuid.UUID   `json:"target_id,omitempty"`
	ClientIP    string       `json:"client_ip"`
}

// ActivityPattern represents a detected pattern in user activity
type ActivityPattern struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	ActorID     uuid.UUID              `json:"actor_id"`
	Frequency   int                    `json:"frequency"`
	TimeSpan    time.Duration          `json:"time_span"`
	RiskLevel   string                 `json:"risk_level"`
	Events      []uuid.UUID            `json:"events"`
	Metadata    map[string]interface{} `json:"metadata"`
	DetectedAt  time.Time              `json:"detected_at"`
}

// NewForensicService creates a new forensic service
func NewForensicService(db *gorm.DB, auditService *AuditService, logger *zap.Logger) *ForensicService {
	return &ForensicService{
		db:           db,
		auditService: auditService,
		logger:       logger,
	}
}

// Search performs a forensic search of audit logs
func (fs *ForensicService) Search(ctx context.Context, query ForensicQuery) (*ForensicResult, error) {
	startTime := time.Now()

	// Validate and set defaults
	if err := fs.validateQuery(&query); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	// Build the database query
	dbQuery := fs.buildQuery(query)

	// Count total results
	var totalCount int64
	if err := dbQuery.Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count results: %w", err)
	}

	// Apply pagination and sorting
	dbQuery = fs.applySorting(dbQuery, query)
	dbQuery = dbQuery.Offset(query.Offset).Limit(query.Limit)

	// Execute the search
	var events []AuditEvent
	if err := dbQuery.Find(&events).Error; err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Decrypt sensitive data if needed
	for i := range events {
		if err := fs.decryptEvent(&events[i]); err != nil {
			fs.logger.Warn("Failed to decrypt audit event",
				zap.String("event_id", events[i].ID.String()),
				zap.Error(err))
		}
	}

	// Generate summary
	summary := fs.generateSummary(events, totalCount)

	result := &ForensicResult{
		Events:     events,
		TotalCount: totalCount,
		Query:      query,
		ExecutedAt: startTime,
		Duration:   time.Since(startTime),
		Summary:    summary,
	}

	fs.logger.Info("Forensic search completed",
		zap.Int64("total_count", totalCount),
		zap.Int("returned_count", len(events)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// SearchEvents wraps the Search method returning events and total count for handlers
func (fs *ForensicService) SearchEvents(ctx context.Context, query ForensicQuery) ([]AuditEvent, int64, error) {
	res, err := fs.Search(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	return res.Events, res.TotalCount, nil
}

// GetTimeline builds a timeline of events for a specific actor
func (fs *ForensicService) GetTimeline(ctx context.Context, actorID uuid.UUID, startTime, endTime time.Time) (*Timeline, error) {
	var events []AuditEvent
	err := fs.db.Where("actor_id = ? AND timestamp BETWEEN ? AND ?", actorID, startTime, endTime).
		Order("timestamp ASC").Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch timeline events: %w", err)
	}

	// Decrypt events
	for i := range events {
		if err := fs.decryptEvent(&events[i]); err != nil {
			fs.logger.Warn("Failed to decrypt timeline event",
				zap.String("event_id", events[i].ID.String()),
				zap.Error(err))
		}
	}

	// Build timeline events
	timelineEvents := make([]TimelineEvent, len(events))
	totalRiskScore := 0.0
	riskScoreCount := 0

	for i, event := range events {
		timelineEvents[i] = TimelineEvent{
			ID:          event.ID,
			EventType:   event.EventType,
			Action:      event.Action,
			Outcome:     event.Outcome,
			Timestamp:   event.Timestamp,
			Description: event.Description,
			RiskScore:   event.RiskScore,
			TargetID:    event.TargetID,
			ClientIP:    event.ClientIP,
		}

		if event.RiskScore != nil {
			totalRiskScore += *event.RiskScore
			riskScoreCount++
		}
	}

	// Calculate average risk score
	avgRiskScore := 0.0
	if riskScoreCount > 0 {
		avgRiskScore = totalRiskScore / float64(riskScoreCount)
	}

	// Generate summary
	summary := fs.generateTimelineSummary(events)

	timeline := &Timeline{
		Events:     timelineEvents,
		ActorID:    actorID,
		StartTime:  startTime,
		EndTime:    endTime,
		Duration:   endTime.Sub(startTime),
		EventCount: len(events),
		RiskScore:  avgRiskScore,
		Summary:    summary,
	}

	return timeline, nil
}

// DetectPatterns analyzes audit logs to detect suspicious patterns
func (fs *ForensicService) DetectPatterns(ctx context.Context, lookbackPeriod time.Duration) ([]ActivityPattern, error) {
	var patterns []ActivityPattern

	startTime := time.Now().Add(-lookbackPeriod)

	// Detect rapid consecutive failures
	rapidFailures, err := fs.detectRapidFailures(startTime)
	if err != nil {
		fs.logger.Error("Failed to detect rapid failures", zap.Error(err))
	} else {
		patterns = append(patterns, rapidFailures...)
	}

	// Detect unusual access patterns
	unusualAccess, err := fs.detectUnusualAccess(startTime)
	if err != nil {
		fs.logger.Error("Failed to detect unusual access", zap.Error(err))
	} else {
		patterns = append(patterns, unusualAccess...)
	}

	// Detect privilege escalation attempts
	privEscalation, err := fs.detectPrivilegeEscalation(startTime)
	if err != nil {
		fs.logger.Error("Failed to detect privilege escalation", zap.Error(err))
	} else {
		patterns = append(patterns, privEscalation...)
	}

	// Detect bulk operations
	bulkOps, err := fs.detectBulkOperations(startTime)
	if err != nil {
		fs.logger.Error("Failed to detect bulk operations", zap.Error(err))
	} else {
		patterns = append(patterns, bulkOps...)
	}

	fs.logger.Info("Pattern detection completed",
		zap.Int("patterns_found", len(patterns)),
		zap.Duration("lookback_period", lookbackPeriod))

	return patterns, nil
}

// validateQuery validates and sets defaults for the forensic query
func (fs *ForensicService) validateQuery(query *ForensicQuery) error {
	// Set default pagination
	if query.Limit <= 0 || query.Limit > 1000 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	// Set default sorting
	if query.SortBy == "" {
		query.SortBy = "timestamp"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}

	// Validate sort order
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		return fmt.Errorf("invalid sort order: %s", query.SortOrder)
	}

	// Validate time range
	if query.StartTime != nil && query.EndTime != nil {
		if query.EndTime.Before(*query.StartTime) {
			return fmt.Errorf("end time cannot be before start time")
		}
	}

	// Set default time range if not specified (last 30 days)
	if query.StartTime == nil && query.EndTime == nil {
		now := time.Now()
		thirtyDaysAgo := now.AddDate(0, 0, -30)
		query.StartTime = &thirtyDaysAgo
		query.EndTime = &now
	}

	return nil
}

// buildQuery builds the GORM query from the forensic query
func (fs *ForensicService) buildQuery(query ForensicQuery) *gorm.DB {
	dbQuery := fs.db.Model(&AuditEvent{})

	// Time range
	if query.StartTime != nil {
		dbQuery = dbQuery.Where("timestamp >= ?", *query.StartTime)
	}
	if query.EndTime != nil {
		dbQuery = dbQuery.Where("timestamp <= ?", *query.EndTime)
	}

	// Actor filters
	if len(query.ActorIDs) > 0 {
		dbQuery = dbQuery.Where("actor_id IN ?", query.ActorIDs)
	}
	if query.ActorRole != nil {
		dbQuery = dbQuery.Where("actor_role = ?", *query.ActorRole)
	}

	// Event filters
	if len(query.EventTypes) > 0 {
		dbQuery = dbQuery.Where("event_type IN ?", query.EventTypes)
	}
	if len(query.Categories) > 0 {
		dbQuery = dbQuery.Where("category IN ?", query.Categories)
	}
	if len(query.Severity) > 0 {
		dbQuery = dbQuery.Where("severity IN ?", query.Severity)
	}
	if len(query.Outcomes) > 0 {
		dbQuery = dbQuery.Where("outcome IN ?", query.Outcomes)
	}

	// Target and resource filters
	if len(query.TargetIDs) > 0 {
		dbQuery = dbQuery.Where("target_id IN ?", query.TargetIDs)
	}
	if len(query.ResourceIDs) > 0 {
		dbQuery = dbQuery.Where("resource_id IN ?", query.ResourceIDs)
	}
	if query.ResourceType != nil {
		dbQuery = dbQuery.Where("resource_type = ?", *query.ResourceType)
	}

	// Network filters
	if len(query.ClientIPs) > 0 {
		dbQuery = dbQuery.Where("client_ip IN ?", query.ClientIPs)
	}
	if len(query.SessionIDs) > 0 {
		dbQuery = dbQuery.Where("session_id IN ?", query.SessionIDs)
	}

	// Content search
	if query.SearchText != nil {
		searchPattern := "%" + *query.SearchText + "%"
		dbQuery = dbQuery.Where("description ILIKE ? OR action ILIKE ?", searchPattern, searchPattern)
	}
	if len(query.Actions) > 0 {
		dbQuery = dbQuery.Where("action IN ?", query.Actions)
	}

	// Risk score filters
	if query.MinRiskScore != nil {
		dbQuery = dbQuery.Where("risk_score >= ?", *query.MinRiskScore)
	}
	if query.MaxRiskScore != nil {
		dbQuery = dbQuery.Where("risk_score <= ?", *query.MaxRiskScore)
	}

	// Compliance flags (JSON array contains)
	if len(query.ComplianceFlags) > 0 {
		for _, flag := range query.ComplianceFlags {
			dbQuery = dbQuery.Where("compliance_flags ? ?", flag)
		}
	}

	// Business impact levels
	if len(query.BusinessImpactLevels) > 0 {
		dbQuery = dbQuery.Where("business_impact->>'level' IN ?", query.BusinessImpactLevels)
	}

	return dbQuery
}

// applySorting applies sorting to the query
func (fs *ForensicService) applySorting(dbQuery *gorm.DB, query ForensicQuery) *gorm.DB {
	validSortFields := map[string]bool{
		"timestamp":  true,
		"event_type": true,
		"severity":   true,
		"risk_score": true,
		"actor_id":   true,
		"client_ip":  true,
		"created_at": true,
	}

	if !validSortFields[query.SortBy] {
		query.SortBy = "timestamp"
	}

	orderClause := fmt.Sprintf("%s %s", query.SortBy, strings.ToUpper(query.SortOrder))
	return dbQuery.Order(orderClause)
}

// decryptEvent decrypts sensitive data in an audit event if encrypted
func (fs *ForensicService) decryptEvent(event *AuditEvent) error {
	if event.EncryptedData == nil || fs.auditService.encryptionSvc == nil {
		return nil
	}

	// Parse EncryptedData JSON
	var encData EncryptedData
	if err := json.Unmarshal([]byte(*event.EncryptedData), &encData); err != nil {
		return fmt.Errorf("failed to parse encrypted data: %w", err)
	}

	// Decrypt the data
	plaintext, err := fs.auditService.encryptionSvc.Decrypt(&encData)
	if err != nil {
		return fmt.Errorf("failed to decrypt audit data: %w", err)
	}

	// Parse the decrypted JSON
	var sensitiveData map[string]interface{}
	if err := json.Unmarshal(plaintext, &sensitiveData); err != nil {
		return fmt.Errorf("failed to parse decrypted data: %w", err)
	}

	// Restore the fields
	if details, exists := sensitiveData["details"]; exists {
		if detailsMap, ok := details.(map[string]interface{}); ok {
			event.Details = detailsMap
		}
	}

	if beforeState, exists := sensitiveData["before_state"]; exists {
		if beforeMap, ok := beforeState.(map[string]interface{}); ok {
			event.BeforeState = beforeMap
		}
	}

	if afterState, exists := sensitiveData["after_state"]; exists {
		if afterMap, ok := afterState.(map[string]interface{}); ok {
			event.AfterState = afterMap
		}
	}

	if changes, exists := sensitiveData["changes"]; exists {
		if changesMap, ok := changes.(map[string]interface{}); ok {
			event.Changes = changesMap
		}
	}

	if metadata, exists := sensitiveData["metadata"]; exists {
		if metadataMap, ok := metadata.(map[string]interface{}); ok {
			event.Metadata = metadataMap
		}
	}

	if forensicData, exists := sensitiveData["forensic_data"]; exists {
		if forensicMap, ok := forensicData.(map[string]interface{}); ok {
			// Convert back to ForensicData struct
			forensicBytes, _ := json.Marshal(forensicMap)
			var fd ForensicData
			if json.Unmarshal(forensicBytes, &fd) == nil {
				event.ForensicData = &fd
			}
		}
	}

	return nil
}

// generateSummary generates a statistical summary of the search results
func (fs *ForensicService) generateSummary(events []AuditEvent, totalCount int64) *SearchSummary {
	summary := &SearchSummary{
		EventsByType:     make(map[EventType]int),
		EventsByCategory: make(map[EventCategory]int),
		EventsBySeverity: make(map[SeverityLevel]int),
		EventsByOutcome:  make(map[EventOutcome]int),
	}

	uniqueActors := make(map[uuid.UUID]bool)
	uniqueSessions := make(map[uuid.UUID]bool)
	uniqueIPs := make(map[string]bool)

	var totalRiskScore float64
	var riskScoreCount int
	var minTime, maxTime time.Time

	for i, event := range events {
		// Count by type, category, severity, outcome
		summary.EventsByType[event.EventType]++
		summary.EventsByCategory[event.Category]++
		summary.EventsBySeverity[event.Severity]++
		summary.EventsByOutcome[event.Outcome]++

		// Track unique values
		uniqueActors[event.ActorID] = true
		if event.SessionID != nil {
			uniqueSessions[*event.SessionID] = true
		}
		uniqueIPs[event.ClientIP] = true

		// Calculate average risk score
		if event.RiskScore != nil {
			totalRiskScore += *event.RiskScore
			riskScoreCount++
		}

		// Track time span
		if i == 0 {
			minTime = event.Timestamp
			maxTime = event.Timestamp
		} else {
			if event.Timestamp.Before(minTime) {
				minTime = event.Timestamp
			}
			if event.Timestamp.After(maxTime) {
				maxTime = event.Timestamp
			}
		}
	}

	summary.UniqueActors = len(uniqueActors)
	summary.UniqueSessions = len(uniqueSessions)
	summary.UniqueIPs = len(uniqueIPs)

	if !minTime.IsZero() && !maxTime.IsZero() {
		summary.TimeSpan = maxTime.Sub(minTime)
	}

	if riskScoreCount > 0 {
		summary.AverageRiskScore = totalRiskScore / float64(riskScoreCount)
	}

	return summary
}

// generateTimelineSummary generates a summary for a timeline
func (fs *ForensicService) generateTimelineSummary(events []AuditEvent) string {
	if len(events) == 0 {
		return "No events in timeline"
	}

	eventTypes := make(map[EventType]int)
	failureCount := 0

	for _, event := range events {
		eventTypes[event.EventType]++
		if event.Outcome == OutcomeFailure {
			failureCount++
		}
	}

	summary := fmt.Sprintf("%d events spanning %s", len(events),
		events[len(events)-1].Timestamp.Sub(events[0].Timestamp).String())

	if failureCount > 0 {
		summary += fmt.Sprintf(", %d failures", failureCount)
	}

	// Add most common event type
	var mostCommonType EventType
	var maxCount int
	for eventType, count := range eventTypes {
		if count > maxCount {
			maxCount = count
			mostCommonType = eventType
		}
	}

	if maxCount > 1 {
		summary += fmt.Sprintf(", most common: %s (%d)", mostCommonType, maxCount)
	}

	return summary
}

// detectRapidFailures detects patterns of rapid consecutive failures
func (fs *ForensicService) detectRapidFailures(since time.Time) ([]ActivityPattern, error) {
	var events []AuditEvent
	err := fs.db.Where("timestamp >= ? AND outcome = ?", since, OutcomeFailure).
		Order("actor_id, timestamp").Find(&events).Error
	if err != nil {
		return nil, err
	}

	var patterns []ActivityPattern
	threshold := 5 // 5 failures within 10 minutes
	timeWindow := 10 * time.Minute

	eventsByActor := make(map[uuid.UUID][]AuditEvent)
	for _, event := range events {
		eventsByActor[event.ActorID] = append(eventsByActor[event.ActorID], event)
	}

	for actorID, actorEvents := range eventsByActor {
		if len(actorEvents) < threshold {
			continue
		}

		for i := 0; i <= len(actorEvents)-threshold; i++ {
			windowEnd := actorEvents[i].Timestamp.Add(timeWindow)
			count := 0
			var windowEvents []uuid.UUID

			for j := i; j < len(actorEvents) && actorEvents[j].Timestamp.Before(windowEnd); j++ {
				count++
				windowEvents = append(windowEvents, actorEvents[j].ID)
			}

			if count >= threshold {
				patterns = append(patterns, ActivityPattern{
					Type:        "rapid_failures",
					Description: fmt.Sprintf("%d failures within %s", count, timeWindow),
					ActorID:     actorID,
					Frequency:   count,
					TimeSpan:    timeWindow,
					RiskLevel:   "high",
					Events:      windowEvents,
					DetectedAt:  time.Now(),
				})
				break // Only report the first pattern per actor
			}
		}
	}

	return patterns, nil
}

// detectUnusualAccess detects unusual access patterns (e.g., access from new IPs)
func (fs *ForensicService) detectUnusualAccess(since time.Time) ([]ActivityPattern, error) {
	// This is a simplified implementation
	// In practice, you'd want to compare against historical patterns
	var patterns []ActivityPattern

	// Detect multiple IP addresses for same user in short time
	var events []AuditEvent
	err := fs.db.Where("timestamp >= ?", since).
		Order("actor_id, timestamp").Find(&events).Error
	if err != nil {
		return nil, err
	}

	ipsByActor := make(map[uuid.UUID]map[string]time.Time)
	for _, event := range events {
		if ipsByActor[event.ActorID] == nil {
			ipsByActor[event.ActorID] = make(map[string]time.Time)
		}
		ipsByActor[event.ActorID][event.ClientIP] = event.Timestamp
	}

	for actorID, ips := range ipsByActor {
		if len(ips) > 3 { // More than 3 different IPs
			var eventIDs []uuid.UUID
			for _, event := range events {
				if event.ActorID == actorID {
					eventIDs = append(eventIDs, event.ID)
				}
			}

			patterns = append(patterns, ActivityPattern{
				Type:        "multiple_ips",
				Description: fmt.Sprintf("Access from %d different IP addresses", len(ips)),
				ActorID:     actorID,
				Frequency:   len(ips),
				RiskLevel:   "medium",
				Events:      eventIDs,
				Metadata: map[string]interface{}{
					"ip_count": len(ips),
				},
				DetectedAt: time.Now(),
			})
		}
	}

	return patterns, nil
}

// detectPrivilegeEscalation detects potential privilege escalation attempts
func (fs *ForensicService) detectPrivilegeEscalation(since time.Time) ([]ActivityPattern, error) {
	var patterns []ActivityPattern

	// Look for role changes to higher privileges
	var events []AuditEvent
	err := fs.db.Where("timestamp >= ? AND event_type = ?", since, EventUserRoleChanged).
		Order("timestamp").Find(&events).Error
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		// Check if this is a privilege escalation
		if event.Details != nil {
			if newRole, exists := event.Details["new_role"]; exists {
				if role, ok := newRole.(string); ok && isHighPrivilegeRole(role) {
					patterns = append(patterns, ActivityPattern{
						Type:        "privilege_escalation",
						Description: fmt.Sprintf("Role changed to %s", role),
						ActorID:     event.ActorID,
						Frequency:   1,
						RiskLevel:   "high",
						Events:      []uuid.UUID{event.ID},
						Metadata: map[string]interface{}{
							"new_role":    role,
							"target_user": event.TargetID,
						},
						DetectedAt: time.Now(),
					})
				}
			}
		}
	}

	return patterns, nil
}

// detectBulkOperations detects bulk administrative operations
func (fs *ForensicService) detectBulkOperations(since time.Time) ([]ActivityPattern, error) {
	var patterns []ActivityPattern

	// Look for many similar operations by the same actor
	var events []AuditEvent
	err := fs.db.Where("timestamp >= ? AND category IN ?", since,
		[]EventCategory{CategoryUserManagement, CategoryRiskManagement}).
		Order("actor_id, timestamp").Find(&events).Error
	if err != nil {
		return nil, err
	}

	operationsByActor := make(map[uuid.UUID]map[string][]uuid.UUID)
	for _, event := range events {
		if operationsByActor[event.ActorID] == nil {
			operationsByActor[event.ActorID] = make(map[string][]uuid.UUID)
		}
		operationsByActor[event.ActorID][event.Action] = append(
			operationsByActor[event.ActorID][event.Action], event.ID)
	}

	for actorID, operations := range operationsByActor {
		for action, eventIDs := range operations {
			if len(eventIDs) > 10 { // More than 10 similar operations
				patterns = append(patterns, ActivityPattern{
					Type:        "bulk_operations",
					Description: fmt.Sprintf("Bulk %s operations (%d instances)", action, len(eventIDs)),
					ActorID:     actorID,
					Frequency:   len(eventIDs),
					RiskLevel:   "medium",
					Events:      eventIDs,
					Metadata: map[string]interface{}{
						"operation_type":  action,
						"operation_count": len(eventIDs),
					},
					DetectedAt: time.Now(),
				})
			}
		}
	}

	return patterns, nil
}

// isHighPrivilegeRole checks if a role has high privileges
func isHighPrivilegeRole(role string) bool {
	highPrivilegeRoles := map[string]bool{
		"admin":       true,
		"super_admin": true,
		"root":        true,
		"system":      true,
	}
	return highPrivilegeRoles[strings.ToLower(role)]
}

// GetStatistics returns basic audit event statistics
func (fs *ForensicService) GetStatistics(ctx context.Context, userID string, startTime, endTime time.Time) (map[string]interface{}, error) {
	// Basic stub: count events for user and overall
	query := fs.db.Model(&AuditEvent{}).Where("created_at BETWEEN ? AND ?", startTime, endTime)
	if userID != "" {
		query = query.Where("actor_id = ?", userID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to count events: %w", err)
	}
	return map[string]interface{}{"total_events": count}, nil
}

// ExportLogs exports events matching the query in JSON or CSV format
func (fs *ForensicService) ExportLogs(ctx context.Context, query ForensicQuery, format string) ([]byte, string, string, error) {
	res, err := fs.Search(ctx, query)
	if err != nil {
		return nil, "", "", err
	}
	// Default to JSON export
	if format != "csv" {
		data, err := json.Marshal(res.Events)
		if err != nil {
			return nil, "", "", err
		}
		return data, "application/json", "audit_export.json", nil
	}
	// CSV export stub
	// TODO: implement CSV formatting
	data := []byte("id,event_type,timestamp\n")
	for _, e := range res.Events {
		data = append(data, []byte(fmt.Sprintf("%s,%s,%s\n", e.ID, e.EventType, e.Timestamp.Format(time.RFC3339)))...)
	}
	return data, "text/csv", "audit_export.csv", nil
}
