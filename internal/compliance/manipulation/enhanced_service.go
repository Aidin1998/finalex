package manipulation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/risk"
	"github.com/Aidin1998/finalex/internal/infrastructure/database"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnhancedManipulationService implements interfaces.ManipulationService
type EnhancedManipulationService struct {
	mu                   sync.RWMutex
	logger               *zap.SugaredLogger
	db                   *gorm.DB
	detector             *ManipulationDetector
	alertingService      *AlertingService
	investigationService *InvestigationService
	config               *interfaces.ManipulationConfig
	riskService          risk.RiskService
	tradingEngine        TradingEngine

	// Service state
	isRunning bool
	startTime time.Time
}

// NewEnhancedManipulationService creates a new enhanced manipulation service
func NewEnhancedManipulationService(
	logger *zap.SugaredLogger,
	db *gorm.DB,
	riskService risk.RiskService,
	tradingEngine TradingEngine,
	config *interfaces.ManipulationConfig,
) *EnhancedManipulationService {
	// Convert interface config to detection config
	detectionConfig := DetectionConfig{
		Enabled:                config.Enabled,
		DetectionWindowMinutes: extractIntFromRules(config.DetectionRules, "detection_window_minutes", 15),
		MinOrdersForAnalysis:   extractIntFromRules(config.DetectionRules, "min_orders_for_analysis", 10),
		MaxConcurrentAnalyses:  extractIntFromRules(config.DetectionRules, "max_concurrent_analyses", 100),
		WashTradingThreshold:   extractDecimalFromThresholds(config.Thresholds, "wash_trading", 0.7),
		SpoofingThreshold:      extractDecimalFromThresholds(config.Thresholds, "spoofing", 0.8),
		PumpDumpThreshold:      extractDecimalFromThresholds(config.Thresholds, "pump_dump", 0.75),
		LayeringThreshold:      extractDecimalFromThresholds(config.Thresholds, "layering", 0.8),
		AutoSuspendEnabled:     extractBoolFromRules(config.DetectionRules, "auto_suspend_enabled", true),
		AutoSuspendThreshold:   extractDecimalFromThresholds(config.Thresholds, "auto_suspend", 0.9),
		RealTimeAlertsEnabled:  config.AlertingEnabled,
		AlertCooldownMinutes:   extractIntFromRules(config.DetectionRules, "alert_cooldown_minutes", 30),
	}

	// Create manipulation detector
	detector := NewManipulationDetector(logger, riskService, detectionConfig, tradingEngine)

	// Create alerting service
	alertingConfig := AlertingConfig{
		Enabled:               config.AlertingEnabled,
		QueueSize:             config.BatchSize,
		MaxRetries:            3,
		RetryDelay:            time.Second * 30,
		RateLimitPerUser:      50,
		RateLimitWindow:       time.Hour,
		DefaultChannels:       []AlertChannel{AlertChannelDatabase, AlertChannelWebSocket},
		RequireAcknowledgment: true,
		AcknowledgmentTimeout: time.Hour * 24,
	}
	alertingService := NewAlertingService(logger, alertingConfig)

	// Create investigation service
	investigationService := NewInvestigationService(logger, &database.Repository{})

	return &EnhancedManipulationService{
		logger:               logger,
		db:                   db,
		detector:             detector,
		alertingService:      alertingService,
		investigationService: investigationService,
		config:               config,
		riskService:          riskService,
		tradingEngine:        tradingEngine,
		isRunning:            false,
	}
}

// DetectManipulation implements the main detection logic
func (s *EnhancedManipulationService) DetectManipulation(ctx context.Context, request interfaces.ManipulationRequest) (*interfaces.ManipulationResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.logger.Debugf("Starting manipulation detection for request %s with %d orders and %d trades",
		request.RequestID, len(request.Orders), len(request.Trades))

	startTime := time.Now()

	// Create detection result
	result := &interfaces.ManipulationResult{
		RequestID:       request.RequestID,
		UserID:          request.UserID,
		Market:          request.Market,
		DetectionStatus: interfaces.DetectionStatusProcessing,
		ProcessedAt:     time.Now(),
		Details:         make(map[string]interface{}),
	}

	// Perform manipulation detection
	patterns, alerts, riskScore, err := s.performDetection(ctx, request.Orders, request.Trades, request.UserID, request.Market)
	if err != nil {
		s.logger.Errorf("Failed to perform detection: %v", err)
		result.DetectionStatus = interfaces.DetectionStatusCompleted // Set to completed even with errors
		return result, fmt.Errorf("detection failed: %w", err)
	}

	result.Patterns = patterns
	result.Alerts = alerts
	result.RiskScore = riskScore

	// Store alerts in database
	for i := range result.Alerts {
		if err := s.storeAlert(ctx, &result.Alerts[i]); err != nil {
			s.logger.Errorf("Failed to store alert: %v", err)
		}
	}

	// Send real-time alerts if enabled
	if s.config.AlertingEnabled {
		for _, alert := range result.Alerts {
			localAlert := s.convertToInternalAlert(alert)
			s.alertingService.SendAlert(localAlert)
		}
	}

	result.DetectionStatus = interfaces.DetectionStatusCompleted
	result.ProcessingTime = time.Since(startTime)

	s.logger.Debugf("Detection completed for request %s with %d patterns and %d alerts",
		request.RequestID, len(result.Patterns), len(result.Alerts))

	return result, nil
}

// GetAlerts retrieves manipulation alerts based on filter criteria
func (s *EnhancedManipulationService) GetAlerts(ctx context.Context, filter *interfaces.AlertFilter) ([]*interfaces.ManipulationAlert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.db.WithContext(ctx).Model(&ManipulationAlertModel{})

	// Apply filters
	if filter != nil {
		if !filter.StartTime.IsZero() {
			query = query.Where("detected_at >= ?", filter.StartTime)
		}
		if !filter.EndTime.IsZero() {
			query = query.Where("detected_at <= ?", filter.EndTime)
		}
		if filter.UserID != "" {
			if userUUID, err := uuid.Parse(filter.UserID); err == nil {
				query = query.Where("user_id = ?", userUUID)
			}
		}
		if filter.AlertType != "" {
			query = query.Where("alert_type = ?", filter.AlertType)
		}
		if filter.Severity != 0 {
			query = query.Where("severity = ?", int(filter.Severity))
		}
		if filter.Status != 0 {
			query = query.Where("status = ?", int(filter.Status))
		}

		// Apply pagination
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	var models []ManipulationAlertModel
	if err := query.Order("detected_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve alerts: %w", err)
	}

	// Convert models to interface types
	alerts := make([]*interfaces.ManipulationAlert, len(models))
	for i, model := range models {
		alert := s.convertModelToAlert(&model)
		alerts[i] = &alert
	}

	return alerts, nil
}

// GetAlert retrieves a specific manipulation alert by ID
func (s *EnhancedManipulationService) GetAlert(ctx context.Context, alertID uuid.UUID) (*interfaces.ManipulationAlert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var model ManipulationAlertModel
	if err := s.db.WithContext(ctx).Where("id = ?", alertID).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve alert: %w", err)
	}

	alert := s.convertModelToAlert(&model)
	return &alert, nil
}

// UpdateAlertStatus updates the status of a manipulation alert
func (s *EnhancedManipulationService) UpdateAlertStatus(ctx context.Context, alertID uuid.UUID, status interfaces.AlertStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	updates := map[string]interface{}{
		"status":     int(status),
		"updated_at": time.Now(),
	}

	if status == interfaces.AlertStatusResolved {
		now := time.Now()
		updates["resolved_at"] = &now
	}

	if err := s.db.WithContext(ctx).Model(&ManipulationAlertModel{}).
		Where("id = ?", alertID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update alert status: %w", err)
	}

	s.logger.Infof("Alert %s status updated to %s", alertID, status)
	return nil
}

// GetInvestigations retrieves investigations based on filter criteria
func (s *EnhancedManipulationService) GetInvestigations(ctx context.Context, filter *interfaces.InvestigationFilter) ([]*interfaces.Investigation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.db.WithContext(ctx).Model(&ManipulationInvestigationModel{})

	// Apply filters
	if filter != nil {
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.Priority != "" {
			query = query.Where("priority = ?", filter.Priority)
		}
		if filter.Type != "" {
			query = query.Where("title ILIKE ?", "%"+filter.Type+"%")
		}
		if filter.UserID != nil {
			query = query.Where("user_id = ?", *filter.UserID)
		}
		if filter.AssignedTo != nil {
			query = query.Where("investigator_id = ?", *filter.AssignedTo)
		}
		if filter.CreatedBy != nil {
			query = query.Where("investigator_id = ?", *filter.CreatedBy)
		}
		if !filter.StartTime.IsZero() {
			query = query.Where("created_at >= ?", filter.StartTime)
		}
		if !filter.EndTime.IsZero() {
			query = query.Where("created_at <= ?", filter.EndTime)
		}

		// Apply pagination
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	var models []ManipulationInvestigationModel
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve investigations: %w", err)
	}

	// Convert models to interface types
	investigations := make([]*interfaces.Investigation, len(models))
	for i, model := range models {
		inv := s.convertModelToInvestigation(&model)
		investigations[i] = inv
	}

	return investigations, nil
}

// CreateInvestigation creates a new investigation
func (s *EnhancedManipulationService) CreateInvestigation(ctx context.Context, request *interfaces.CreateInvestigationRequest) (*interfaces.Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate investigation ID
	investigationID := fmt.Sprintf("INV_%d_%s", time.Now().Unix(), uuid.New().String()[:8])

	// Determine market from alerts if not provided
	market := request.Market
	if market == "" && len(request.AlertIDs) > 0 {
		// Get market from first alert
		var alert ManipulationAlertModel
		if err := s.db.WithContext(ctx).First(&alert, "id = ?", request.AlertIDs[0]).Error; err == nil {
			market = alert.Market
		}
	}

	// Use a default UUID for UserID if not provided
	userID := uuid.New()
	if request.UserID != nil {
		userID = *request.UserID
	}

	// Use AssignedTo as investigator, or create a new UUID
	investigatorID := uuid.New()
	if request.AssignedTo != nil {
		investigatorID = *request.AssignedTo
	}

	// Create the database model
	model := ManipulationInvestigationModel{
		ID:             investigationID,
		UserID:         userID,
		Market:         market,
		InvestigatorID: investigatorID,
		Status:         "open",
		Priority:       request.Priority,
		Title:          request.Subject,
		Description:    request.Description,
		Evidence:       []string{}, // Initialize empty evidence slice
		Notes:          "",         // Initialize empty notes
		Conclusion:     "",         // Initialize empty conclusion
		StartedAt:      time.Now(),
		DueDate:        request.DueDate,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Findings:       request.Metadata,
	}

	// Set alert ID if provided
	if len(request.AlertIDs) > 0 {
		model.AlertID = request.AlertIDs[0] // Use first alert ID
	} else {
		// Create a placeholder alert ID if none provided
		model.AlertID = uuid.New()
	}

	// Save to database
	if err := s.db.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, fmt.Errorf("failed to create investigation: %w", err)
	}

	s.logger.Infow("Created new investigation",
		"investigation_id", investigationID,
		"user_id", userID,
		"type", request.Type,
		"priority", request.Priority)

	// Convert and return the created investigation
	return s.convertModelToInvestigation(&model), nil
}

// UpdateInvestigation updates an existing investigation
func (s *EnhancedManipulationService) UpdateInvestigation(ctx context.Context, investigationID uuid.UUID, updates *interfaces.InvestigationUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert UUID to string for database lookup
	investigationIDStr := investigationID.String()

	// Find the investigation in the database
	var model ManipulationInvestigationModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", investigationIDStr).Error; err != nil {
		return fmt.Errorf("investigation not found: %w", err)
	}

	// Prepare update map
	updateMap := make(map[string]interface{})

	// Apply updates only for non-nil fields
	if updates.Status != nil {
		updateMap["status"] = *updates.Status
		// Set completion time if status is being set to closed
		if *updates.Status == "closed" || *updates.Status == "completed" {
			now := time.Now()
			updateMap["completed_at"] = &now
		}
	}

	if updates.Priority != nil {
		updateMap["priority"] = *updates.Priority
	}

	if updates.Subject != nil {
		updateMap["title"] = *updates.Subject
	}

	if updates.Description != nil {
		updateMap["description"] = *updates.Description
	}

	if updates.Findings != nil {
		updateMap["notes"] = *updates.Findings
	}

	if updates.Conclusion != nil {
		updateMap["conclusion"] = *updates.Conclusion
	}

	if updates.AssignedTo != nil {
		updateMap["investigator_id"] = *updates.AssignedTo
	}

	if updates.DueDate != nil {
		updateMap["due_date"] = updates.DueDate
	}

	if updates.Metadata != nil {
		updateMap["findings"] = updates.Metadata
	}

	// Always update the UpdatedAt timestamp
	updateMap["updated_at"] = time.Now()

	// Perform the database update
	if err := s.db.WithContext(ctx).Model(&model).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("failed to update investigation: %w", err)
	}

	s.logger.Infow("Updated investigation",
		"investigation_id", investigationIDStr,
		"updates", updateMap)

	return nil
}

// GetPatterns retrieves manipulation patterns based on filter criteria
func (s *EnhancedManipulationService) GetPatterns(ctx context.Context, filter *interfaces.PatternFilter) ([]*interfaces.ManipulationPattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.db.WithContext(ctx).Model(&ManipulationPatternModel{})

	// Apply filters
	if filter != nil {
		if filter.Type.String() != "" {
			query = query.Where("pattern_type = ?", filter.Type.String())
		}
		if filter.Market != "" {
			query = query.Where("market = ?", filter.Market)
		}
		if filter.UserID != nil {
			query = query.Where("user_id = ?", *filter.UserID)
		}
		if !filter.StartTime.IsZero() {
			query = query.Where("detected_at >= ?", filter.StartTime)
		}
		if !filter.EndTime.IsZero() {
			query = query.Where("detected_at <= ?", filter.EndTime)
		}

		// Apply pagination if specified
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	var models []ManipulationPatternModel
	if err := query.Order("detected_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve patterns: %w", err)
	}

	// Convert models to interface types
	patterns := make([]*interfaces.ManipulationPattern, len(models))
	for i, model := range models {
		patterns[i] = s.convertModelToPattern(&model)
	}

	return patterns, nil
}

// GetConfig retrieves the current manipulation detection configuration
func (s *EnhancedManipulationService) GetConfig(ctx context.Context) (*interfaces.ManipulationConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy of the current configuration
	configCopy := *s.config
	return &configCopy, nil
}

// UpdateConfig updates the manipulation detection configuration
func (s *EnhancedManipulationService) UpdateConfig(ctx context.Context, config *interfaces.ManipulationConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate configuration
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	// Update the configuration
	s.config = config

	// Update detector configuration
	detectionConfig := DetectionConfig{
		Enabled:                config.Enabled,
		DetectionWindowMinutes: extractIntFromRules(config.DetectionRules, "detection_window_minutes", 15),
		MinOrdersForAnalysis:   extractIntFromRules(config.DetectionRules, "min_orders_for_analysis", 10),
		MaxConcurrentAnalyses:  extractIntFromRules(config.DetectionRules, "max_concurrent_analyses", 100),
		WashTradingThreshold:   extractDecimalFromThresholds(config.Thresholds, "wash_trading", 0.7),
		SpoofingThreshold:      extractDecimalFromThresholds(config.Thresholds, "spoofing", 0.8),
		PumpDumpThreshold:      extractDecimalFromThresholds(config.Thresholds, "pump_dump", 0.75),
		LayeringThreshold:      extractDecimalFromThresholds(config.Thresholds, "layering", 0.8),
		AutoSuspendEnabled:     extractBoolFromRules(config.DetectionRules, "auto_suspend_enabled", true),
		AutoSuspendThreshold:   extractDecimalFromThresholds(config.Thresholds, "auto_suspend", 0.9),
		RealTimeAlertsEnabled:  config.AlertingEnabled,
		AlertCooldownMinutes:   extractIntFromRules(config.DetectionRules, "alert_cooldown_minutes", 30),
	}

	s.detector.UpdateConfig(detectionConfig)

	s.logger.Info("Manipulation detection configuration updated")
	return nil
}

// Service lifecycle methods

// Start starts the manipulation detection service
func (s *EnhancedManipulationService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("service is already running")
	}

	s.logger.Info("Starting Enhanced Manipulation Service")
	// Start the detector
	if err := s.detector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start detector: %w", err)
	}
	// Start the alerting service
	if err := s.alertingService.Start(ctx); err != nil {
		s.detector.Stop()
		return fmt.Errorf("failed to start alerting service: %w", err)
	}

	s.isRunning = true
	s.startTime = time.Now()

	s.logger.Info("Enhanced Manipulation Service started successfully")
	return nil
}

// Stop stops the manipulation detection service
func (s *EnhancedManipulationService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	s.logger.Info("Stopping Enhanced Manipulation Service")

	// Stop services in reverse order
	s.alertingService.Stop()
	s.detector.Stop()

	s.isRunning = false

	s.logger.Info("Enhanced Manipulation Service stopped")
	return nil
}

// HealthCheck performs a health check of the service
func (s *EnhancedManipulationService) HealthCheck(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isRunning {
		return fmt.Errorf("service is not running")
	}

	// Check database connectivity
	if err := s.db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	// Check if detector is healthy
	if s.detector == nil {
		return fmt.Errorf("detector is not initialized")
	}

	// Check if alerting service is healthy
	if s.alertingService == nil {
		return fmt.Errorf("alerting service is not initialized")
	}

	return nil
}

// Helper functions for config extraction
func extractDecimalFromThresholds(thresholds map[string]interface{}, key string, defaultValue float64) decimal.Decimal {
	if thresholds == nil {
		return decimal.NewFromFloat(defaultValue)
	}
	if value, exists := thresholds[key]; exists {
		switch v := value.(type) {
		case float64:
			return decimal.NewFromFloat(v)
		case string:
			if d, err := decimal.NewFromString(v); err == nil {
				return d
			}
		case decimal.Decimal:
			return v
		}
	}
	return decimal.NewFromFloat(defaultValue)
}

func extractIntFromRules(rules map[string]interface{}, key string, defaultValue int) int {
	if rules == nil {
		return defaultValue
	}
	if value, exists := rules[key]; exists {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

func extractBoolFromRules(rules map[string]interface{}, key string, defaultValue bool) bool {
	if rules == nil {
		return defaultValue
	}
	if value, exists := rules[key]; exists {
		if v, ok := value.(bool); ok {
			return v
		}
	}
	return defaultValue
}

// Helper methods for internal operations

// performDetection performs the actual manipulation detection
func (s *EnhancedManipulationService) performDetection(ctx context.Context, orders []interfaces.Order, trades []interfaces.Trade, userID uuid.UUID, market string) ([]interfaces.ManipulationPattern, []interfaces.ManipulationAlert, decimal.Decimal, error) {
	patterns := []interfaces.ManipulationPattern{}
	alerts := []interfaces.ManipulationAlert{}
	riskScore := decimal.Zero

	// Basic wash trading detection
	if washPattern, washAlert := s.detectWashTrading(orders, trades, userID, market); washPattern != nil {
		patterns = append(patterns, *washPattern)
		if washAlert != nil {
			alerts = append(alerts, *washAlert)
			riskScore = riskScore.Add(washAlert.RiskScore)
		}
	}

	// Basic spoofing detection
	if spoofPattern, spoofAlert := s.detectSpoofing(orders, userID, market); spoofPattern != nil {
		patterns = append(patterns, *spoofPattern)
		if spoofAlert != nil {
			alerts = append(alerts, *spoofAlert)
			riskScore = riskScore.Add(spoofAlert.RiskScore)
		}
	}

	// Normalize risk score
	if len(alerts) > 0 {
		riskScore = riskScore.Div(decimal.NewFromInt(int64(len(alerts))))
	}

	return patterns, alerts, riskScore, nil
}

// detectWashTrading performs basic wash trading detection
func (s *EnhancedManipulationService) detectWashTrading(orders []interfaces.Order, trades []interfaces.Trade, userID uuid.UUID, market string) (*interfaces.ManipulationPattern, *interfaces.ManipulationAlert) {
	if len(trades) < 2 {
		return nil, nil
	}

	// Check for trades between same user accounts (simplified)
	for i, trade1 := range trades {
		for j, trade2 := range trades[i+1:] {
			_ = j // avoid unused variable warning
			if trade1.BuyerID == trade2.SellerID && trade1.SellerID == trade2.BuyerID {
				confidence := decimal.NewFromFloat(0.8)
				if confidence.GreaterThan(s.detector.config.WashTradingThreshold) {
					pattern := &interfaces.ManipulationPattern{
						Type:        interfaces.ManipulationAlertWashTrading,
						Confidence:  confidence,
						Description: "Potential wash trading detected - reciprocal trades between same parties",
						Evidence: []interfaces.PatternEvidence{{
							Type:        "trade_pair",
							Description: "Reciprocal trades",
							Value:       map[string]interface{}{"trade1": trade1, "trade2": trade2},
							Timestamp:   time.Now(),
						}},
						TimeWindow: time.Hour,
						DetectedAt: time.Now(),
					}

					alert := &interfaces.ManipulationAlert{
						ID:          uuid.New(),
						UserID:      userID,
						Market:      market,
						AlertType:   interfaces.ManipulationAlertWashTrading,
						Severity:    s.convertConfidenceToSeverity(confidence),
						RiskScore:   confidence,
						Description: "Potential wash trading detected",
						Details:     map[string]interface{}{"pattern_type": "wash_trading", "confidence": confidence},
						Evidence:    []string{"Reciprocal trades between same parties"},
						Status:      interfaces.AlertStatusPending,
						DetectedAt:  time.Now(),
					}

					return pattern, alert
				}
			}
		}
	}

	return nil, nil
}

// detectSpoofing performs basic spoofing detection
func (s *EnhancedManipulationService) detectSpoofing(orders []interfaces.Order, userID uuid.UUID, market string) (*interfaces.ManipulationPattern, *interfaces.ManipulationAlert) {
	if len(orders) == 0 {
		return nil, nil
	}

	cancelledOrders := 0
	totalOrders := len(orders)

	for _, order := range orders {
		if order.Status == "cancelled" {
			cancelledOrders++
		}
	}

	if totalOrders > 10 && float64(cancelledOrders)/float64(totalOrders) > 0.8 {
		confidence := decimal.NewFromFloat(0.75)
		if confidence.GreaterThan(s.detector.config.SpoofingThreshold) {
			pattern := &interfaces.ManipulationPattern{
				Type:        interfaces.ManipulationAlertSpoofing,
				Confidence:  confidence,
				Description: fmt.Sprintf("High cancellation rate detected: %d/%d orders cancelled", cancelledOrders, totalOrders),
				Evidence: []interfaces.PatternEvidence{{
					Type:        "cancellation_rate",
					Description: "High order cancellation rate",
					Value:       map[string]interface{}{"cancelled": cancelledOrders, "total": totalOrders},
					Timestamp:   time.Now(),
				}},
				TimeWindow: time.Hour,
				DetectedAt: time.Now(),
			}

			alert := &interfaces.ManipulationAlert{
				ID:          uuid.New(),
				UserID:      userID,
				Market:      market,
				AlertType:   interfaces.ManipulationAlertSpoofing,
				Severity:    s.convertConfidenceToSeverity(confidence),
				RiskScore:   confidence,
				Description: "High order cancellation rate detected",
				Details:     map[string]interface{}{"pattern_type": "spoofing", "confidence": confidence},
				Evidence:    []string{fmt.Sprintf("High cancellation rate: %d/%d orders", cancelledOrders, totalOrders)},
				Status:      interfaces.AlertStatusPending,
				DetectedAt:  time.Now(),
			}

			return pattern, alert
		}
	}

	return nil, nil
}

// storeAlert stores an alert in the database
func (s *EnhancedManipulationService) storeAlert(ctx context.Context, alert *interfaces.ManipulationAlert) error {
	model := ManipulationAlertModel{
		ID:          alert.ID,
		UserID:      alert.UserID,
		Market:      alert.Market,
		AlertType:   alert.AlertType.String(),
		Severity:    alert.Severity.String(),
		RiskScore:   alert.RiskScore,
		Description: alert.Description,
		Evidence:    alert.Evidence,
		Status:      alert.Status.String(),
		DetectedAt:  alert.DetectedAt,
		ResolvedAt:  alert.ResolvedAt,
		ResolvedBy:  alert.ResolvedBy,
		Resolution:  alert.Resolution,
	}

	return s.db.WithContext(ctx).Create(&model).Error
}

// convertModelToAlert converts a database model to a manipulation alert
func (s *EnhancedManipulationService) convertModelToAlert(model *ManipulationAlertModel) interfaces.ManipulationAlert {
	return interfaces.ManipulationAlert{
		ID:              model.ID,
		UserID:          model.UserID,
		Market:          model.Market,
		AlertType:       s.stringToAlertType(model.AlertType),
		Severity:        s.stringToSeverity(model.Severity),
		RiskScore:       model.RiskScore,
		Description:     model.Description,
		Details:         map[string]interface{}{},
		Evidence:        model.Evidence,
		Status:          s.stringToStatus(model.Status),
		DetectedAt:      model.DetectedAt,
		ResolvedAt:      model.ResolvedAt,
		ResolvedBy:      model.ResolvedBy,
		Resolution:      model.Resolution,
		InvestigationID: model.InvestigationID,
	}
}

// convertConfidenceToSeverity converts confidence score to alert severity
func (s *EnhancedManipulationService) convertConfidenceToSeverity(confidence decimal.Decimal) interfaces.AlertSeverity {
	if confidence.GreaterThanOrEqual(decimal.NewFromFloat(0.9)) {
		return interfaces.AlertSeverityCritical
	} else if confidence.GreaterThanOrEqual(decimal.NewFromFloat(0.7)) {
		return interfaces.AlertSeverityHigh
	} else if confidence.GreaterThanOrEqual(decimal.NewFromFloat(0.5)) {
		return interfaces.AlertSeverityMedium
	} else {
		return interfaces.AlertSeverityLow
	}
}

// convertToInternalAlert converts interface alert to internal alert for alerting service
func (s *EnhancedManipulationService) convertToInternalAlert(alert interfaces.ManipulationAlert) ManipulationAlert {
	// Create a basic manipulation pattern for the internal alert
	pattern := ManipulationPattern{
		Type:        alert.AlertType.String(),
		Description: alert.Description,
		Confidence:  alert.RiskScore,
		Severity:    alert.Severity.String(),
		Evidence:    []Evidence{},
		DetectedAt:  alert.DetectedAt,
		Metadata:    alert.Details,
	}

	// Create pattern interface
	patternIface := interfaces.ManipulationPattern{
		Type:        alert.AlertType,
		Confidence:  alert.RiskScore,
		Description: alert.Description,
		DetectedAt:  alert.DetectedAt,
	}

	return ManipulationAlert{
		ID:              alert.ID.String(),
		UserID:          alert.UserID.String(),
		Market:          alert.Market,
		Pattern:         pattern,
		PatternIface:    patternIface,
		RiskScore:       alert.RiskScore,
		ActionRequired:  s.determineActionRequired(alert.Severity),
		Status:          alert.Status.String(),
		CreatedAt:       alert.DetectedAt,
		UpdatedAt:       alert.DetectedAt,
		AutoSuspended:   alert.Severity == interfaces.AlertSeverityCritical,
		RelatedOrderIDs: []string{},
		RelatedTradeIDs: []string{},
	}
}

// determineActionRequired determines the required action based on alert severity
func (s *EnhancedManipulationService) determineActionRequired(severity interfaces.AlertSeverity) string {
	switch severity {
	case interfaces.AlertSeverityCritical:
		return "suspend"
	case interfaces.AlertSeverityHigh:
		return "investigate"
	case interfaces.AlertSeverityMedium:
		return "monitor"
	case interfaces.AlertSeverityLow:
		return "monitor"
	default:
		return "monitor"
	}
}

// String conversion helpers
func (s *EnhancedManipulationService) stringToAlertType(str string) interfaces.ManipulationAlertType {
	switch str {
	case "wash_trading":
		return interfaces.ManipulationAlertWashTrading
	case "spoofing":
		return interfaces.ManipulationAlertSpoofing
	case "layering":
		return interfaces.ManipulationAlertLayering
	case "pump_and_dump":
		return interfaces.ManipulationAlertPumpAndDump
	case "front_running":
		return interfaces.ManipulationAlertFrontRunning
	case "insider_trading":
		return interfaces.ManipulationAlertInsiderTrading
	default:
		return interfaces.ManipulationAlertWashTrading
	}
}

func (s *EnhancedManipulationService) stringToSeverity(str string) interfaces.AlertSeverity {
	switch str {
	case "low":
		return interfaces.AlertSeverityLow
	case "medium":
		return interfaces.AlertSeverityMedium
	case "high":
		return interfaces.AlertSeverityHigh
	case "critical":
		return interfaces.AlertSeverityCritical
	default:
		return interfaces.AlertSeverityLow
	}
}

func (s *EnhancedManipulationService) stringToStatus(str string) interfaces.AlertStatus {
	switch str {
	case "pending":
		return interfaces.AlertStatusPending
	case "investigating":
		return interfaces.AlertStatusInvestigating
	case "resolved":
		return interfaces.AlertStatusResolved
	case "dismissed":
		return interfaces.AlertStatusDismissed
	default:
		return interfaces.AlertStatusPending
	}
}

// convertModelToInvestigation converts a database model to an Investigation interface type
func (s *EnhancedManipulationService) convertModelToInvestigation(model *ManipulationInvestigationModel) *interfaces.Investigation {
	// Parse the investigation ID as UUID if possible, otherwise create a new one
	investigationID, err := uuid.Parse(model.ID)
	if err != nil {
		investigationID = uuid.New()
	}

	// Convert investigation status and priority
	status := model.Status
	if status == "" {
		status = "pending"
	}

	priority := model.Priority
	if priority == "" {
		priority = "medium"
	}
	// Convert optional investigator ID
	var assignedTo *uuid.UUID
	nilUUID := uuid.UUID{}
	if model.InvestigatorID != nilUUID {
		assignedTo = &model.InvestigatorID
	}

	// Convert optional completion date
	var completedAt *time.Time
	if model.CompletedAt != nil {
		completedAt = model.CompletedAt
	}

	// Convert the model to interface type
	return &interfaces.Investigation{
		ID:          investigationID,
		Type:        model.Title, // Use title as type for now
		Status:      status,
		Priority:    priority,
		Subject:     model.Title,
		Description: model.Description,
		UserID:      &model.UserID,
		Market:      model.Market,
		AlertIDs:    []uuid.UUID{model.AlertID}, // Convert single alert ID to slice
		Evidence:    model.Evidence,
		Findings:    model.Notes, // Use notes as findings for now
		Conclusion:  model.Conclusion,
		AssignedTo:  assignedTo,
		CreatedBy:   model.InvestigatorID, // Use investigator as creator for now
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
		DueDate:     model.DueDate,
		CompletedAt: completedAt,
		Metadata:    model.Findings, // Use findings as metadata for now
	}
}

// convertModelToPattern converts a database model to a ManipulationPattern interface type
func (s *EnhancedManipulationService) convertModelToPattern(model *ManipulationPatternModel) *interfaces.ManipulationPattern {
	// Convert pattern type string to enum
	patternType := s.stringToAlertType(model.PatternType)
	// Convert evidence map to PatternEvidence slice
	var evidence []interfaces.PatternEvidence
	if model.Evidence != nil {
		// Evidence is already a map[string]interface{}
		for key, value := range model.Evidence {
			evidence = append(evidence, interfaces.PatternEvidence{
				Type:        key,
				Description: fmt.Sprintf("Evidence for %s", key),
				Value:       value,
				Timestamp:   model.DetectedAt,
				Metadata:    nil,
			})
		}
	}

	// Convert time window from nanoseconds to duration
	timeWindow := time.Duration(model.TimeWindow)

	return &interfaces.ManipulationPattern{
		Type:        patternType,
		Confidence:  model.Confidence,
		Description: model.Description,
		Evidence:    evidence,
		TimeWindow:  timeWindow,
		DetectedAt:  model.DetectedAt,
		Metadata:    model.Metadata,
	}
}
