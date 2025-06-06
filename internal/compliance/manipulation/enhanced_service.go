package manipulation

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// EnhancedDetectionService provides consolidated manipulation detection with enhanced capabilities
type EnhancedDetectionService struct {
	db                  *sql.DB
	auditService        interfaces.AuditService
	logger              *zap.SugaredLogger
	detectors           map[string]interfaces.ManipulationDetector
	patterns            *PatternRegistry
	rules               *RuleEngine
	alertManager        *AlertManager
	investigationEngine *InvestigationEngine
	metrics             *ManipulationMetrics
	config              *ManipulationConfig
	mu                  sync.RWMutex
}

// ManipulationConfig holds configuration for manipulation detection
type ManipulationConfig struct {
	Enabled                  bool                       `json:"enabled"`
	RealTimeDetection        bool                       `json:"real_time_detection"`
	BatchProcessing          bool                       `json:"batch_processing"`
	DetectionWindowMinutes   int                        `json:"detection_window_minutes"`
	MinOrdersForAnalysis     int                        `json:"min_orders_for_analysis"`
	AutoSuspendEnabled       bool                       `json:"auto_suspend_enabled"`
	Thresholds               map[string]decimal.Decimal `json:"thresholds"`
	PatternWeights           map[string]float64         `json:"pattern_weights"`
	InvestigationEnabled     bool                       `json:"investigation_enabled"`
	VisualAnalysisEnabled    bool                       `json:"visual_analysis_enabled"`
	BehaviorProfilingEnabled bool                       `json:"behavior_profiling_enabled"`
}

// PatternRegistry manages different pattern detection algorithms
type PatternRegistry struct {
	detectors      map[string]interfaces.ManipulationDetector
	configurations map[string]interface{}
	mu             sync.RWMutex
}

// RuleEngine processes detection rules and policies
type RuleEngine struct {
	rules        map[string]*DetectionRule
	policyEngine *PolicyEngine
	scoringModel *ScoringModel
	mu           sync.RWMutex
}

// DetectionRule represents a manipulation detection rule
type DetectionRule struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Type              string                 `json:"type"` // "wash_trading", "spoofing", "pump_dump", "layering"
	Conditions        []RuleCondition        `json:"conditions"`
	Action            string                 `json:"action"` // "flag", "block", "investigate", "escalate"
	Severity          interfaces.RiskLevel   `json:"severity"`
	Threshold         decimal.Decimal        `json:"threshold"`
	Weight            float64                `json:"weight"`
	TimeWindow        time.Duration          `json:"time_window"`
	MarketSpecific    bool                   `json:"market_specific"`
	ApplicableMarkets []string               `json:"applicable_markets"`
	Active            bool                   `json:"active"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// RuleCondition represents a condition in a detection rule
type RuleCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // "gt", "lt", "eq", "gte", "lte", "contains"
	Value    interface{} `json:"value"`
	Weight   float64     `json:"weight"`
}

// PolicyEngine handles dynamic policy management
type PolicyEngine struct {
	policies map[string]*ManipulationPolicy
	mu       sync.RWMutex
}

// ScoringModel handles risk scoring algorithms
type ScoringModel struct {
	baseWeights     map[string]float64
	marketFactors   map[string]float64
	temporalFactors map[time.Weekday]float64
	mu              sync.RWMutex
}

// ManipulationPolicy represents a manipulation detection policy
type ManipulationPolicy struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Rules       []string               `json:"rules"` // Rule IDs
	Markets     []string               `json:"markets"`
	Active      bool                   `json:"active"`
	Priority    int                    `json:"priority"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// AlertManager handles manipulation alerts and notifications
type AlertManager struct {
	alerts          []*interfaces.ManipulationAlert
	alertQueue      chan *interfaces.ManipulationAlert
	notifications   *NotificationManager
	escalationRules map[string]*EscalationRule
	mu              sync.RWMutex
}

// NotificationManager handles alert notifications
type NotificationManager struct {
	channels    map[string]NotificationChannel
	templates   map[string]*NotificationTemplate
	preferences map[string]*NotificationPreference
}

// NotificationChannel interface for different notification types
type NotificationChannel interface {
	Send(alert *interfaces.ManipulationAlert, template *NotificationTemplate) error
	GetType() string
	IsAvailable() bool
}

// NotificationTemplate defines how alerts are formatted
type NotificationTemplate struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Subject  string            `json:"subject"`
	Body     string            `json:"body"`
	Headers  map[string]string `json:"headers"`
	Priority string            `json:"priority"`
}

// NotificationPreference defines user notification preferences
type NotificationPreference struct {
	UserID         string            `json:"user_id"`
	Channels       []string          `json:"channels"`
	MinSeverity    string            `json:"min_severity"`
	Frequency      string            `json:"frequency"`
	QuietHours     []string          `json:"quiet_hours"`
	MarketFilters  []string          `json:"market_filters"`
	PatternFilters []string          `json:"pattern_filters"`
	CustomSettings map[string]string `json:"custom_settings"`
}

// EscalationRule defines when and how to escalate alerts
type EscalationRule struct {
	ID            string        `json:"id"`
	Conditions    []string      `json:"conditions"`
	EscalateAfter time.Duration `json:"escalate_after"`
	NextLevel     string        `json:"next_level"`
	Actions       []string      `json:"actions"`
}

// InvestigationEngine handles in-depth investigation of suspicious patterns
type InvestigationEngine struct {
	behaviorProfiler *UserBehaviorProfiler
	visualAnalyzer   *VisualPatternAnalyzer
	networkAnalyzer  *NetworkAnalyzer
	temporalAnalyzer *TemporalAnalyzer
	investigations   map[string]*Investigation
	mu               sync.RWMutex
}

// Investigation represents an active investigation
type Investigation struct {
	ID              string                        `json:"id"`
	Type            string                        `json:"type"`
	Status          string                        `json:"status"`
	Priority        string                        `json:"priority"`
	UserID          string                        `json:"user_id"`
	Market          string                        `json:"market"`
	TriggerAlert    *interfaces.ManipulationAlert `json:"trigger_alert"`
	Evidence        []Evidence                    `json:"evidence"`
	Findings        []Finding                     `json:"findings"`
	BehaviorProfile *UserBehaviorProfile          `json:"behavior_profile"`
	VisualAnalysis  *VisualAnalysisResult         `json:"visual_analysis"`
	NetworkAnalysis *NetworkAnalysisResult        `json:"network_analysis"`
	Conclusion      string                        `json:"conclusion"`
	Recommendations []string                      `json:"recommendations"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
	CompletedAt     *time.Time                    `json:"completed_at"`
	AssignedTo      string                        `json:"assigned_to"`
}

// Evidence represents a piece of evidence in an investigation
type Evidence struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	Source      string                 `json:"source"`
	Reliability string                 `json:"reliability"`
	Timestamp   time.Time              `json:"timestamp"`
}

// Finding represents a finding from an investigation
type Finding struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	Evidence    []string               `json:"evidence"` // Evidence IDs
	Confidence  decimal.Decimal        `json:"confidence"`
	Impact      string                 `json:"impact"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
}

// ManipulationMetrics tracks system performance and detection statistics
type ManipulationMetrics struct {
	TotalDetections      int64                      `json:"total_detections"`
	DetectionsByType     map[string]int64           `json:"detections_by_type"`
	TruePositives        int64                      `json:"true_positives"`
	FalsePositives       int64                      `json:"false_positives"`
	TrueNegatives        int64                      `json:"true_negatives"`
	FalseNegatives       int64                      `json:"false_negatives"`
	AverageDetectionTime time.Duration              `json:"average_detection_time"`
	AlertsGenerated      int64                      `json:"alerts_generated"`
	InvestigationsOpened int64                      `json:"investigations_opened"`
	InvestigationsClosed int64                      `json:"investigations_closed"`
	AutoSuspensions      int64                      `json:"auto_suspensions"`
	PatternAccuracy      map[string]decimal.Decimal `json:"pattern_accuracy"`
	MarketCoverage       map[string]int64           `json:"market_coverage"`
	PerformanceMetrics   *PerformanceMetrics        `json:"performance_metrics"`
	LastUpdated          time.Time                  `json:"last_updated"`
}

// PerformanceMetrics tracks system performance
type PerformanceMetrics struct {
	ProcessingLatency    time.Duration `json:"processing_latency"`
	QueueDepth           int           `json:"queue_depth"`
	MemoryUsage          int64         `json:"memory_usage"`
	CacheHitRatio        float64       `json:"cache_hit_ratio"`
	DatabaseConnections  int           `json:"database_connections"`
	ConcurrentDetections int           `json:"concurrent_detections"`
}

// NewEnhancedDetectionService creates a new enhanced manipulation detection service
func NewEnhancedDetectionService(db *sql.DB, auditService interfaces.AuditService, logger *zap.SugaredLogger) *EnhancedDetectionService {
	service := &EnhancedDetectionService{
		db:                  db,
		auditService:        auditService,
		logger:              logger,
		detectors:           make(map[string]interfaces.ManipulationDetector),
		patterns:            NewPatternRegistry(),
		rules:               NewRuleEngine(),
		alertManager:        NewAlertManager(),
		investigationEngine: NewInvestigationEngine(),
		metrics: &ManipulationMetrics{
			DetectionsByType:   make(map[string]int64),
			PatternAccuracy:    make(map[string]decimal.Decimal),
			MarketCoverage:     make(map[string]int64),
			PerformanceMetrics: &PerformanceMetrics{},
		},
		config: DefaultManipulationConfig(),
	}

	// Initialize detectors
	service.initializeDetectors()

	// Load configuration
	if err := service.loadConfiguration(); err != nil {
		logger.Warnw("Failed to load manipulation detection configuration", "error", err)
	}

	// Initialize database
	if err := service.initDatabase(); err != nil {
		logger.Errorf("Failed to initialize manipulation detection database: %v", err)
	}

	return service
}

// DetectManipulation performs comprehensive manipulation detection
func (eds *EnhancedDetectionService) DetectManipulation(ctx context.Context, req *interfaces.ManipulationRequest) (*interfaces.ManipulationResult, error) {
	startTime := time.Now()

	// Create audit event
	auditEvent := &interfaces.AuditEvent{
		ID:        uuid.New().String(),
		Type:      interfaces.ActivityTypeManipulationDetection,
		UserID:    req.UserID,
		Details:   map[string]interface{}{"request": req},
		Timestamp: time.Now(),
		IPAddress: req.IPAddress,
	}

	result := &interfaces.ManipulationResult{
		RequestID:       req.RequestID,
		UserID:          req.UserID,
		Market:          req.Market,
		DetectionStatus: interfaces.DetectionStatusProcessing,
		RiskScore:       decimal.Zero,
		Patterns:        []interfaces.ManipulationPattern{},
		Alerts:          []interfaces.ManipulationAlert{},
		ProcessedAt:     time.Now(),
		Details:         make(map[string]interface{}),
	}

	// Prepare trading activity data
	activity := &interfaces.TradingActivity{
		UserID:     req.UserID,
		Market:     req.Market,
		Orders:     req.Orders,
		Trades:     req.Trades,
		TimeWindow: time.Duration(eds.config.DetectionWindowMinutes) * time.Minute,
	}

	// Run pattern detection
	patterns, err := eds.runPatternDetection(ctx, activity)
	if err != nil {
		result.DetectionStatus = interfaces.DetectionStatusError
		result.Details["error"] = err.Error()
		return result, err
	}

	// Calculate overall risk score
	riskScore := eds.calculateOverallRiskScore(patterns)
	result.RiskScore = riskScore
	result.Patterns = patterns

	// Apply rules and generate alerts
	alerts, err := eds.applyDetectionRules(ctx, activity, patterns)
	if err != nil {
		eds.logger.Warnw("Failed to apply detection rules", "error", err)
	} else {
		result.Alerts = alerts
	}

	// Determine final status
	result.DetectionStatus = eds.determineFinalStatus(riskScore, patterns, alerts)

	// Trigger investigation if needed
	if eds.shouldTriggerInvestigation(result) {
		investigationID, err := eds.triggerInvestigation(ctx, result)
		if err != nil {
			eds.logger.Warnw("Failed to trigger investigation", "error", err)
		} else {
			result.Details["investigation_id"] = investigationID
		}
	}

	// Update metrics
	eds.updateMetrics(result, time.Since(startTime))

	// Save result
	if err := eds.saveDetectionResult(ctx, result); err != nil {
		eds.logger.Warnw("Failed to save detection result", "error", err)
	}

	// Audit the detection
	auditEvent.Details["result"] = result
	auditEvent.Details["duration_ms"] = time.Since(startTime).Milliseconds()
	if err := eds.auditService.LogEvent(ctx, auditEvent); err != nil {
		eds.logger.Warnw("Failed to audit manipulation detection", "error", err)
	}

	return result, nil
}

// AddDetectionRule adds or updates a detection rule
func (eds *EnhancedDetectionService) AddDetectionRule(ctx context.Context, rule *DetectionRule) error {
	eds.mu.Lock()
	defer eds.mu.Unlock()

	// Validate rule
	if err := eds.validateDetectionRule(rule); err != nil {
		return fmt.Errorf("invalid detection rule: %w", err)
	}

	// Set timestamps
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	rule.UpdatedAt = time.Now()

	// Save to database
	if err := eds.saveDetectionRule(ctx, rule); err != nil {
		return fmt.Errorf("failed to save detection rule: %w", err)
	}

	// Update in-memory cache
	eds.rules.AddRule(rule)

	// Audit the rule creation/update
	auditEvent := &interfaces.AuditEvent{
		ID:        uuid.New().String(),
		Type:      interfaces.ActivityTypeRuleUpdate,
		Details:   map[string]interface{}{"rule": rule},
		Timestamp: time.Now(),
	}

	if err := eds.auditService.LogEvent(ctx, auditEvent); err != nil {
		eds.logger.Warnw("Failed to audit rule update", "error", err)
	}

	return nil
}

// GetInvestigation retrieves an investigation by ID
func (eds *EnhancedDetectionService) GetInvestigation(ctx context.Context, investigationID string) (*Investigation, error) {
	eds.investigationEngine.mu.RLock()
	investigation, exists := eds.investigationEngine.investigations[investigationID]
	eds.investigationEngine.mu.RUnlock()

	if !exists {
		// Try loading from database
		return eds.loadInvestigationFromDatabase(ctx, investigationID)
	}

	return investigation, nil
}

// UpdateInvestigation updates an investigation
func (eds *EnhancedDetectionService) UpdateInvestigation(ctx context.Context, investigation *Investigation) error {
	investigation.UpdatedAt = time.Now()

	// Save to database
	if err := eds.saveInvestigationToDatabase(ctx, investigation); err != nil {
		return err
	}

	// Update in-memory cache
	eds.investigationEngine.mu.Lock()
	eds.investigationEngine.investigations[investigation.ID] = investigation
	eds.investigationEngine.mu.Unlock()

	return nil
}

// GetMetrics returns current manipulation detection metrics
func (eds *EnhancedDetectionService) GetMetrics() *ManipulationMetrics {
	eds.mu.RLock()
	defer eds.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := *eds.metrics
	metrics.LastUpdated = time.Now()

	return &metrics
}

// Implementation of core detection methods
func (eds *EnhancedDetectionService) runPatternDetection(ctx context.Context, activity *interfaces.TradingActivity) ([]interfaces.ManipulationPattern, error) {
	var patterns []interfaces.ManipulationPattern
	var detectionErrors []error

	// Run each detector
	for detectorName, detector := range eds.detectors {
		pattern, err := detector.Detect(ctx, activity)
		if err != nil {
			eds.logger.Warnw("Detector failed", "detector", detectorName, "error", err)
			detectionErrors = append(detectionErrors, fmt.Errorf("detector %s: %w", detectorName, err))
			continue
		}

		if pattern != nil {
			patterns = append(patterns, *pattern)
			eds.logger.Infow("Pattern detected",
				"detector", detectorName,
				"pattern_type", pattern.Type,
				"confidence", pattern.Confidence.String(),
				"user_id", activity.UserID,
				"market", activity.Market)
		}
	}

	// Return error only if all detectors failed
	if len(detectionErrors) > 0 && len(patterns) == 0 {
		return nil, fmt.Errorf("all detectors failed: %v", detectionErrors)
	}

	return patterns, nil
}

func (eds *EnhancedDetectionService) calculateOverallRiskScore(patterns []interfaces.ManipulationPattern) decimal.Decimal {
	if len(patterns) == 0 {
		return decimal.Zero
	}

	var totalScore decimal.Decimal
	var totalWeight float64

	for _, pattern := range patterns {
		weight, exists := eds.config.PatternWeights[pattern.Type]
		if !exists {
			weight = 1.0 // Default weight
		}

		weightedScore := pattern.Confidence.Mul(decimal.NewFromFloat(weight))
		totalScore = totalScore.Add(weightedScore)
		totalWeight += weight
	}

	if totalWeight == 0 {
		return decimal.Zero
	}

	return totalScore.Div(decimal.NewFromFloat(totalWeight))
}

func (eds *EnhancedDetectionService) applyDetectionRules(ctx context.Context, activity *interfaces.TradingActivity, patterns []interfaces.ManipulationPattern) ([]interfaces.ManipulationAlert, error) {
	var alerts []interfaces.ManipulationAlert

	for _, pattern := range patterns {
		// Find applicable rules
		applicableRules := eds.rules.GetApplicableRules(pattern.Type, activity.Market)

		for _, rule := range applicableRules {
			if eds.evaluateRule(rule, pattern, activity) {
				alert := interfaces.ManipulationAlert{
					ID:              uuid.New().String(),
					UserID:          activity.UserID,
					Market:          activity.Market,
					Pattern:         pattern,
					RiskScore:       pattern.Confidence,
					Severity:        rule.Severity,
					Status:          interfaces.AlertStatusOpen,
					RuleID:          rule.ID,
					ActionRequired:  rule.Action,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
					RelatedOrderIDs: eds.extractOrderIDs(activity.Orders),
					RelatedTradeIDs: eds.extractTradeIDs(activity.Trades),
				}

				alerts = append(alerts, alert)

				// Queue alert for processing
				eds.alertManager.QueueAlert(&alert)
			}
		}
	}

	return alerts, nil
}

func (eds *EnhancedDetectionService) evaluateRule(rule *DetectionRule, pattern interfaces.ManipulationPattern, activity *interfaces.TradingActivity) bool {
	// Check pattern type matches
	if rule.Type != pattern.Type {
		return false
	}

	// Check threshold
	if pattern.Confidence.LessThan(rule.Threshold) {
		return false
	}

	// Check market applicability
	if rule.MarketSpecific && !eds.isMarketApplicable(rule.ApplicableMarkets, activity.Market) {
		return false
	}

	// Evaluate conditions
	for _, condition := range rule.Conditions {
		if !eds.evaluateCondition(condition, pattern, activity) {
			return false
		}
	}

	return true
}

func (eds *EnhancedDetectionService) evaluateCondition(condition RuleCondition, pattern interfaces.ManipulationPattern, activity *interfaces.TradingActivity) bool {
	// Simplified condition evaluation - would be more sophisticated in production
	switch condition.Field {
	case "confidence":
		threshold, ok := condition.Value.(float64)
		if !ok {
			return false
		}
		switch condition.Operator {
		case "gt":
			return pattern.Confidence.GreaterThan(decimal.NewFromFloat(threshold))
		case "gte":
			return pattern.Confidence.GreaterThanOrEqual(decimal.NewFromFloat(threshold))
		case "lt":
			return pattern.Confidence.LessThan(decimal.NewFromFloat(threshold))
		case "lte":
			return pattern.Confidence.LessThanOrEqual(decimal.NewFromFloat(threshold))
		case "eq":
			return pattern.Confidence.Equal(decimal.NewFromFloat(threshold))
		}
	case "order_count":
		threshold, ok := condition.Value.(float64)
		if !ok {
			return false
		}
		orderCount := float64(len(activity.Orders))
		switch condition.Operator {
		case "gt":
			return orderCount > threshold
		case "gte":
			return orderCount >= threshold
		case "lt":
			return orderCount < threshold
		case "lte":
			return orderCount <= threshold
		case "eq":
			return orderCount == threshold
		}
	}
	return false
}

func (eds *EnhancedDetectionService) isMarketApplicable(applicableMarkets []string, market string) bool {
	for _, applicableMarket := range applicableMarkets {
		if applicableMarket == market || applicableMarket == "*" {
			return true
		}
	}
	return false
}

// Helper methods for status determination and investigation triggers
func (eds *EnhancedDetectionService) determineFinalStatus(riskScore decimal.Decimal, patterns []interfaces.ManipulationPattern, alerts []interfaces.ManipulationAlert) interfaces.DetectionStatus {
	if len(alerts) > 0 {
		// Check for high-risk alerts
		for _, alert := range alerts {
			if alert.Severity == interfaces.RiskLevelCritical || alert.Severity == interfaces.RiskLevelHigh {
				return interfaces.DetectionStatusSuspicious
			}
		}
		return interfaces.DetectionStatusFlagged
	}

	if riskScore.GreaterThan(decimal.NewFromFloat(7.0)) {
		return interfaces.DetectionStatusSuspicious
	} else if riskScore.GreaterThan(decimal.NewFromFloat(4.0)) {
		return interfaces.DetectionStatusFlagged
	}

	return interfaces.DetectionStatusClean
}

func (eds *EnhancedDetectionService) shouldTriggerInvestigation(result *interfaces.ManipulationResult) bool {
	if !eds.config.InvestigationEnabled {
		return false
	}

	// Trigger investigation for suspicious cases
	if result.DetectionStatus == interfaces.DetectionStatusSuspicious {
		return true
	}

	// Trigger for high-risk patterns
	for _, pattern := range result.Patterns {
		if pattern.Confidence.GreaterThan(decimal.NewFromFloat(8.0)) {
			return true
		}
	}

	// Trigger for critical alerts
	for _, alert := range result.Alerts {
		if alert.Severity == interfaces.RiskLevelCritical {
			return true
		}
	}

	return false
}

func (eds *EnhancedDetectionService) triggerInvestigation(ctx context.Context, result *interfaces.ManipulationResult) (string, error) {
	investigation := &Investigation{
		ID:        uuid.New().String(),
		Type:      "manipulation_detection",
		Status:    "open",
		Priority:  eds.determinePriority(result),
		UserID:    result.UserID,
		Market:    result.Market,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set trigger alert if any
	if len(result.Alerts) > 0 {
		investigation.TriggerAlert = &result.Alerts[0]
	}

	// Start investigation processes
	if eds.config.BehaviorProfilingEnabled {
		go eds.investigationEngine.StartBehaviorProfiling(ctx, investigation)
	}

	if eds.config.VisualAnalysisEnabled {
		go eds.investigationEngine.StartVisualAnalysis(ctx, investigation)
	}

	// Save investigation
	eds.investigationEngine.mu.Lock()
	eds.investigationEngine.investigations[investigation.ID] = investigation
	eds.investigationEngine.mu.Unlock()

	if err := eds.saveInvestigationToDatabase(ctx, investigation); err != nil {
		return "", err
	}

	eds.metrics.InvestigationsOpened++

	return investigation.ID, nil
}

func (eds *EnhancedDetectionService) determinePriority(result *interfaces.ManipulationResult) string {
	if result.RiskScore.GreaterThan(decimal.NewFromFloat(9.0)) {
		return "critical"
	} else if result.RiskScore.GreaterThan(decimal.NewFromFloat(7.0)) {
		return "high"
	} else if result.RiskScore.GreaterThan(decimal.NewFromFloat(5.0)) {
		return "medium"
	}
	return "low"
}

func (eds *EnhancedDetectionService) updateMetrics(result *interfaces.ManipulationResult, duration time.Duration) {
	eds.mu.Lock()
	defer eds.mu.Unlock()

	eds.metrics.TotalDetections++

	// Update detections by type
	for _, pattern := range result.Patterns {
		eds.metrics.DetectionsByType[pattern.Type]++
	}

	// Update average detection time
	if eds.metrics.TotalDetections == 1 {
		eds.metrics.AverageDetectionTime = duration
	} else {
		totalTime := eds.metrics.AverageDetectionTime * time.Duration(eds.metrics.TotalDetections-1)
		eds.metrics.AverageDetectionTime = (totalTime + duration) / time.Duration(eds.metrics.TotalDetections)
	}

	// Update market coverage
	eds.metrics.MarketCoverage[result.Market]++

	// Update alert counts
	eds.metrics.AlertsGenerated += int64(len(result.Alerts))

	eds.metrics.LastUpdated = time.Now()
}

func (eds *EnhancedDetectionService) extractOrderIDs(orders []interfaces.Order) []string {
	var ids []string
	for _, order := range orders {
		ids = append(ids, order.ID)
	}
	return ids
}

func (eds *EnhancedDetectionService) extractTradeIDs(trades []interfaces.Trade) []string {
	var ids []string
	for _, trade := range trades {
		ids = append(ids, trade.ID)
	}
	return ids
}

// Placeholder methods for initialization and database operations
func (eds *EnhancedDetectionService) initializeDetectors() {
	// Initialize all pattern detectors
	// These would integrate with existing detectors from the manipulation package
	eds.logger.Info("Initializing enhanced manipulation detectors")
}

func (eds *EnhancedDetectionService) loadConfiguration() error {
	// Load configuration from database or config file
	return nil
}

func (eds *EnhancedDetectionService) initDatabase() error {
	// Initialize database tables for enhanced detection
	return nil
}

func (eds *EnhancedDetectionService) validateDetectionRule(rule *DetectionRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if rule.Type == "" {
		return fmt.Errorf("rule type is required")
	}
	return nil
}

func (eds *EnhancedDetectionService) saveDetectionRule(ctx context.Context, rule *DetectionRule) error {
	// Save rule to database
	return nil
}

func (eds *EnhancedDetectionService) loadInvestigationFromDatabase(ctx context.Context, investigationID string) (*Investigation, error) {
	// Load investigation from database
	return nil, fmt.Errorf("investigation not found")
}

func (eds *EnhancedDetectionService) saveInvestigationToDatabase(ctx context.Context, investigation *Investigation) error {
	// Save investigation to database
	return nil
}

func (eds *EnhancedDetectionService) saveDetectionResult(ctx context.Context, result *interfaces.ManipulationResult) error {
	// Save result to database
	return nil
}

func DefaultManipulationConfig() *ManipulationConfig {
	return &ManipulationConfig{
		Enabled:                  true,
		RealTimeDetection:        true,
		BatchProcessing:          true,
		DetectionWindowMinutes:   60,
		MinOrdersForAnalysis:     10,
		AutoSuspendEnabled:       true,
		InvestigationEnabled:     true,
		VisualAnalysisEnabled:    true,
		BehaviorProfilingEnabled: true,
		Thresholds: map[string]decimal.Decimal{
			"wash_trading": decimal.NewFromFloat(7.0),
			"spoofing":     decimal.NewFromFloat(7.5),
			"pump_dump":    decimal.NewFromFloat(8.0),
			"layering":     decimal.NewFromFloat(7.0),
		},
		PatternWeights: map[string]float64{
			"wash_trading": 1.0,
			"spoofing":     1.2,
			"pump_dump":    1.5,
			"layering":     1.1,
		},
	}
}

// Factory methods for components
func NewPatternRegistry() *PatternRegistry {
	return &PatternRegistry{
		detectors:      make(map[string]interfaces.ManipulationDetector),
		configurations: make(map[string]interface{}),
	}
}

func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules:        make(map[string]*DetectionRule),
		policyEngine: &PolicyEngine{policies: make(map[string]*ManipulationPolicy)},
		scoringModel: &ScoringModel{
			baseWeights:     make(map[string]float64),
			marketFactors:   make(map[string]float64),
			temporalFactors: make(map[time.Weekday]float64),
		},
	}
}

func NewAlertManager() *AlertManager {
	return &AlertManager{
		alerts:     make([]*interfaces.ManipulationAlert, 0),
		alertQueue: make(chan *interfaces.ManipulationAlert, 1000),
		notifications: &NotificationManager{
			channels:    make(map[string]NotificationChannel),
			templates:   make(map[string]*NotificationTemplate),
			preferences: make(map[string]*NotificationPreference),
		},
		escalationRules: make(map[string]*EscalationRule),
	}
}

func NewInvestigationEngine() *InvestigationEngine {
	return &InvestigationEngine{
		investigations: make(map[string]*Investigation),
	}
}

// Additional methods for RuleEngine
func (re *RuleEngine) AddRule(rule *DetectionRule) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.rules[rule.ID] = rule
}

func (re *RuleEngine) GetApplicableRules(patternType, market string) []*DetectionRule {
	re.mu.RLock()
	defer re.mu.RUnlock()

	var applicable []*DetectionRule
	for _, rule := range re.rules {
		if !rule.Active {
			continue
		}
		if rule.Type == patternType || rule.Type == "*" {
			if !rule.MarketSpecific || re.isMarketApplicable(rule.ApplicableMarkets, market) {
				applicable = append(applicable, rule)
			}
		}
	}
	return applicable
}

func (re *RuleEngine) isMarketApplicable(applicableMarkets []string, market string) bool {
	for _, applicableMarket := range applicableMarkets {
		if applicableMarket == market || applicableMarket == "*" {
			return true
		}
	}
	return false
}

// AlertManager methods
func (am *AlertManager) QueueAlert(alert *interfaces.ManipulationAlert) {
	select {
	case am.alertQueue <- alert:
		am.mu.Lock()
		am.alerts = append(am.alerts, alert)
		am.mu.Unlock()
	default:
		log.Printf("Alert queue is full, dropping alert: %s", alert.ID)
	}
}

// InvestigationEngine methods
func (ie *InvestigationEngine) StartBehaviorProfiling(ctx context.Context, investigation *Investigation) {
	// Placeholder for behavior profiling
	log.Printf("Starting behavior profiling for investigation: %s", investigation.ID)
}

func (ie *InvestigationEngine) StartVisualAnalysis(ctx context.Context, investigation *Investigation) {
	// Placeholder for visual analysis
	log.Printf("Starting visual analysis for investigation: %s", investigation.ID)
}
