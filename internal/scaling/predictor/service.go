// =============================
// Prediction Service Orchestrator
// =============================
// This service orchestrates ML-based load prediction and auto-scaling decisions
// with real-time metric ingestion and model management capabilities.

package predictor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PredictionService orchestrates load prediction and scaling decisions
type PredictionService struct {
	config        *ServiceConfig
	logger        *zap.SugaredLogger
	models        map[string]PredictionModel
	activeModel   string
	metricsClient v1.API

	// State management
	mu                sync.RWMutex
	lastPrediction    *PredictionResult
	predictionHistory []*PredictionResult
	modelMetrics      map[string]*ModelMetrics

	// Real-time processing
	metricsChan      chan *LoadMetrics
	predictionTicker *time.Ticker

	// A/B testing
	abTestConfig  *ABTestConfig
	modelVersions map[string]*ModelVersion

	// Manual overrides
	manualOverride *ManualOverride
	overrideExpiry time.Time

	// Multi-region coordination
	regionCoordinator RegionCoordinator

	// Callbacks
	onScalingDecision func(*ScalingDecision) error
	onAlert           func(*Alert) error
}

// ServiceConfig contains configuration for the prediction service
type ServiceConfig struct {
	PredictionInterval    time.Duration           `json:"prediction_interval"`
	ModelRetryInterval    time.Duration           `json:"model_retry_interval"`
	MetricsBufferSize     int                     `json:"metrics_buffer_size"`
	PredictionHistorySize int                     `json:"prediction_history_size"`
	PrometheusURL         string                  `json:"prometheus_url"`
	Models                map[string]*ModelConfig `json:"models"`
	ABTesting             *ABTestConfig           `json:"ab_testing"`
	ScalingConstraints    *ScalingConstraints     `json:"scaling_constraints"`
	AlertConfig           *AlertConfig            `json:"alert_config"`
	RegionConfig          *RegionConfig           `json:"region_config"`
}

// ModelConfig contains configuration for a specific prediction model
type ModelConfig struct {
	Type           string                 `json:"type"` // "arima", "lstm"
	Parameters     map[string]interface{} `json:"parameters"`
	TrainingConfig *TrainingConfig        `json:"training_config"`
	FeatureConfig  *FeatureConfig         `json:"feature_config"`
	Enabled        bool                   `json:"enabled"`
	Weight         float64                `json:"weight"` // For ensemble models
}

// TrainingConfig contains model training parameters
type TrainingConfig struct {
	RetrainingInterval  time.Duration `json:"retraining_interval"`
	MinTrainingData     int           `json:"min_training_data"`
	ValidationSplit     float64       `json:"validation_split"`
	EarlyStoppingRounds int           `json:"early_stopping_rounds"`
	LearningRate        float64       `json:"learning_rate"`
	BatchSize           int           `json:"batch_size"`
	Epochs              int           `json:"epochs"`
}

// FeatureConfig defines feature engineering parameters
type FeatureConfig struct {
	WindowSizes        []int             `json:"window_sizes"`
	LagFeatures        []int             `json:"lag_features"`
	MovingAverages     []int             `json:"moving_averages"`
	SeasonalityPeriods []int             `json:"seasonality_periods"`
	ExternalFeatures   []string          `json:"external_features"`
	FeatureSelection   *FeatureSelection `json:"feature_selection"`
}

// FeatureSelection contains feature selection parameters
type FeatureSelection struct {
	Method          string  `json:"method"` // "correlation", "mutual_info", "lasso"
	Threshold       float64 `json:"threshold"`
	MaxFeatures     int     `json:"max_features"`
	CrossValidation int     `json:"cross_validation"`
}

// ABTestConfig contains A/B testing configuration
type ABTestConfig struct {
	Enabled           bool               `json:"enabled"`
	TestDuration      time.Duration      `json:"test_duration"`
	TrafficSplit      map[string]float64 `json:"traffic_split"`
	MetricsToTrack    []string           `json:"metrics_to_track"`
	SignificanceLevel float64            `json:"significance_level"`
	MinSampleSize     int                `json:"min_sample_size"`
}

// ModelVersion represents a specific version of a model
type ModelVersion struct {
	Version       string            `json:"version"`
	Model         PredictionModel   `json:"-"`
	TrainedAt     time.Time         `json:"trained_at"`
	Accuracy      float64           `json:"accuracy"`
	Performance   *ModelPerformance `json:"performance"`
	TrafficWeight float64           `json:"traffic_weight"`
	IsChampion    bool              `json:"is_champion"`
}

// ModelPerformance tracks model performance metrics
type ModelPerformance struct {
	MAE               float64       `json:"mae"`      // Mean Absolute Error
	RMSE              float64       `json:"rmse"`     // Root Mean Square Error
	MAPE              float64       `json:"mape"`     // Mean Absolute Percentage Error
	R2Score           float64       `json:"r2_score"` // R-squared
	PredictionLatency time.Duration `json:"prediction_latency"`
	LastUpdated       time.Time     `json:"last_updated"`
}

// ModelMetrics tracks runtime metrics for models
type ModelMetrics struct {
	PredictionCount int64         `json:"prediction_count"`
	ErrorCount      int64         `json:"error_count"`
	AverageLatency  time.Duration `json:"average_latency"`
	AccuracyScore   float64       `json:"accuracy_score"`
	LastPrediction  time.Time     `json:"last_prediction"`
	LastError       *time.Time    `json:"last_error,omitempty"`
}

// ScalingConstraints define limits for auto-scaling
type ScalingConstraints struct {
	MinInstances        int                `json:"min_instances"`
	MaxInstances        int                `json:"max_instances"`
	MaxScaleUpRate      float64            `json:"max_scale_up_rate"`   // instances per minute
	MaxScaleDownRate    float64            `json:"max_scale_down_rate"` // instances per minute
	CooldownPeriod      time.Duration      `json:"cooldown_period"`
	PreWarmingThreshold float64            `json:"pre_warming_threshold"`
	ScaleDownThreshold  float64            `json:"scale_down_threshold"`
	CostConstraints     *CostConstraints   `json:"cost_constraints"`
	MarketHoursScaling  *MarketHoursConfig `json:"market_hours_scaling"`
}

// CostConstraints define cost optimization parameters
type CostConstraints struct {
	MaxHourlyCost       decimal.Decimal `json:"max_hourly_cost"`
	CostPerInstance     decimal.Decimal `json:"cost_per_instance"`
	PreferSpotInstances bool            `json:"prefer_spot_instances"`
	SpotInstanceRatio   float64         `json:"spot_instance_ratio"`
	BudgetAlert         decimal.Decimal `json:"budget_alert"`
}

// MarketHoursConfig defines scaling behavior during market hours
type MarketHoursConfig struct {
	Markets          map[string]*MarketSchedule `json:"markets"`
	PreMarketBuffer  time.Duration              `json:"pre_market_buffer"`
	PostMarketBuffer time.Duration              `json:"post_market_buffer"`
	WeekendScaling   *WeekendScalingConfig      `json:"weekend_scaling"`
}

// MarketSchedule defines market operating hours
type MarketSchedule struct {
	TimeZone        string      `json:"timezone"`
	OpenTime        time.Time   `json:"open_time"`
	CloseTime       time.Time   `json:"close_time"`
	IsActive        bool        `json:"is_active"`
	HolidaySchedule []time.Time `json:"holiday_schedule"`
}

// WeekendScalingConfig defines weekend scaling behavior
type WeekendScalingConfig struct {
	MinInstances  int     `json:"min_instances"`
	MaxInstances  int     `json:"max_instances"`
	ScalingFactor float64 `json:"scaling_factor"`
}

// AlertConfig defines alerting configuration
type AlertConfig struct {
	Enabled          bool              `json:"enabled"`
	Channels         []string          `json:"channels"` // slack, email, webhook
	Thresholds       *AlertThresholds  `json:"thresholds"`
	EscalationPolicy *EscalationPolicy `json:"escalation_policy"`
}

// AlertThresholds define when to trigger alerts
type AlertThresholds struct {
	HighLoadThreshold      float64         `json:"high_load_threshold"`
	LatencyThreshold       float64         `json:"latency_threshold"`
	ErrorRateThreshold     float64         `json:"error_rate_threshold"`
	ModelAccuracyThreshold float64         `json:"model_accuracy_threshold"`
	CostThreshold          decimal.Decimal `json:"cost_threshold"`
}

// EscalationPolicy defines alert escalation
type EscalationPolicy struct {
	Levels          []*EscalationLevel `json:"levels"`
	EscalationDelay time.Duration      `json:"escalation_delay"`
}

// EscalationLevel defines a single escalation level
type EscalationLevel struct {
	Level    int      `json:"level"`
	Contacts []string `json:"contacts"`
	Actions  []string `json:"actions"`
}

// RegionConfig defines multi-region coordination
type RegionConfig struct {
	CurrentRegion      string        `json:"current_region"`
	PeerRegions        []string      `json:"peer_regions"`
	CoordinationMode   string        `json:"coordination_mode"` // "leader", "peer", "follower"
	SyncInterval       time.Duration `json:"sync_interval"`
	ConflictResolution string        `json:"conflict_resolution"` // "priority", "consensus", "manual"
}

// ManualOverride represents a manual scaling override
type ManualOverride struct {
	OverrideType    string    `json:"override_type"` // "scale_to", "disable_scaling", "force_scale_up", "force_scale_down"
	TargetInstances *int      `json:"target_instances,omitempty"`
	Reason          string    `json:"reason"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	IsActive        bool      `json:"is_active"`
}

// ScalingDecision represents a scaling decision made by the service
type ScalingDecision struct {
	Timestamp         time.Time       `json:"timestamp"`
	Action            string          `json:"action"` // "scale_up", "scale_down", "no_change", "pre_warm"
	CurrentInstances  int             `json:"current_instances"`
	TargetInstances   int             `json:"target_instances"`
	Reason            string          `json:"reason"`
	Confidence        float64         `json:"confidence"`
	PredictedLoad     float64         `json:"predicted_load"`
	ModelUsed         string          `json:"model_used"`
	CostImpact        decimal.Decimal `json:"cost_impact"`
	ManualOverride    bool            `json:"manual_override"`
	RegionCoordinated bool            `json:"region_coordinated"`
}

// RegionCoordinator handles multi-region scaling coordination
type RegionCoordinator interface {
	SyncScalingDecision(ctx context.Context, decision *ScalingDecision) error
	GetPeerRegionStatus(ctx context.Context) (map[string]*RegionStatus, error)
	ResolveConflict(ctx context.Context, decisions []*ScalingDecision) (*ScalingDecision, error)
	NotifyPeerRegions(ctx context.Context, event *RegionEvent) error
}

// RegionStatus represents the status of a peer region
type RegionStatus struct {
	Region        string           `json:"region"`
	LastSync      time.Time        `json:"last_sync"`
	CurrentLoad   float64          `json:"current_load"`
	PredictedLoad float64          `json:"predicted_load"`
	InstanceCount int              `json:"instance_count"`
	IsHealthy     bool             `json:"is_healthy"`
	LastDecision  *ScalingDecision `json:"last_decision"`
}

// RegionEvent represents an event to be shared with peer regions
type RegionEvent struct {
	EventType string      `json:"event_type"` // "scaling_decision", "load_spike", "system_alert"
	Region    string      `json:"region"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	Priority  int         `json:"priority"`
}

// NewPredictionService creates a new prediction service
func NewPredictionService(
	config *ServiceConfig,
	logger *zap.SugaredLogger,
	regionCoordinator RegionCoordinator,
) (*PredictionService, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Initialize Prometheus client
	promClient, err := api.NewClient(api.Config{
		Address: config.PrometheusURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	service := &PredictionService{
		config:            config,
		logger:            logger,
		models:            make(map[string]PredictionModel),
		modelMetrics:      make(map[string]*ModelMetrics),
		predictionHistory: make([]*PredictionResult, 0, config.PredictionHistorySize),
		metricsChan:       make(chan *LoadMetrics, config.MetricsBufferSize),
		metricsClient:     v1.NewAPI(promClient),
		regionCoordinator: regionCoordinator,
		modelVersions:     make(map[string]*ModelVersion),
		abTestConfig:      config.ABTesting,
	}

	// Initialize models
	err = service.initializeModels()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize models: %w", err)
	}

	// Set default active model
	service.setActiveModel()

	return service, nil
}

// Start begins the prediction service
func (s *PredictionService) Start(ctx context.Context) error {
	s.logger.Info("Starting prediction service")

	// Start metrics ingestion
	go s.metricsIngestionLoop(ctx)

	// Start prediction loop
	s.predictionTicker = time.NewTicker(s.config.PredictionInterval)
	go s.predictionLoop(ctx)

	// Start model retraining
	go s.modelRetrainingLoop(ctx)

	// Start A/B testing if enabled
	if s.config.ABTesting != nil && s.config.ABTesting.Enabled {
		go s.abTestingLoop(ctx)
	}

	// Start region coordination
	go s.regionCoordinationLoop(ctx)

	s.logger.Info("Prediction service started successfully")
	return nil
}

// Stop gracefully stops the prediction service
func (s *PredictionService) Stop(ctx context.Context) error {
	s.logger.Info("Stopping prediction service")

	if s.predictionTicker != nil {
		s.predictionTicker.Stop()
	}

	// Close channels
	close(s.metricsChan)

	s.logger.Info("Prediction service stopped")
	return nil
}

// SetScalingDecisionCallback sets the callback for scaling decisions
func (s *PredictionService) SetScalingDecisionCallback(callback func(*ScalingDecision) error) {
	s.onScalingDecision = callback
}

// SetAlertCallback sets the callback for alerts
func (s *PredictionService) SetAlertCallback(callback func(*Alert) error) {
	s.onAlert = callback
}

// SetManualOverride sets a manual scaling override
func (s *PredictionService) SetManualOverride(override *ManualOverride) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.manualOverride = override
	s.overrideExpiry = override.ExpiresAt

	s.logger.Infow("Manual override set",
		"type", override.OverrideType,
		"target_instances", override.TargetInstances,
		"reason", override.Reason,
		"expires_at", override.ExpiresAt,
	)

	return nil
}

// ClearManualOverride clears the current manual override
func (s *PredictionService) ClearManualOverride() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.manualOverride = nil
	s.overrideExpiry = time.Time{}

	s.logger.Info("Manual override cleared")
}

// GetCurrentPrediction returns the most recent prediction
func (s *PredictionService) GetCurrentPrediction() *PredictionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.lastPrediction
}

// GetPredictionHistory returns recent prediction history
func (s *PredictionService) GetPredictionHistory(limit int) []*PredictionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.predictionHistory) {
		limit = len(s.predictionHistory)
	}

	start := len(s.predictionHistory) - limit
	return s.predictionHistory[start:]
}

// GetModelMetrics returns metrics for all models
func (s *PredictionService) GetModelMetrics() map[string]*ModelMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := make(map[string]*ModelMetrics)
	for name, metric := range s.modelMetrics {
		metricCopy := *metric
		metrics[name] = &metricCopy
	}

	return metrics
}

// Private methods

func (s *PredictionService) initializeModels() error {
	for modelName, modelConfig := range s.config.Models {
		if !modelConfig.Enabled {
			continue
		}

		var model PredictionModel
		var err error

		switch modelConfig.Type {
		case "arima":
			model, err = NewARIMAModel(modelConfig.Parameters)
		case "lstm":
			model, err = NewLSTMModel(modelConfig.Parameters)
		default:
			return fmt.Errorf("unknown model type: %s", modelConfig.Type)
		}

		if err != nil {
			return fmt.Errorf("failed to create model %s: %w", modelName, err)
		}

		s.models[modelName] = model
		s.modelMetrics[modelName] = &ModelMetrics{
			LastPrediction: time.Now(),
		}

		s.logger.Infow("Initialized model", "name", modelName, "type", modelConfig.Type)
	}

	return nil
}

func (s *PredictionService) setActiveModel() {
	// Simple logic: use the first enabled model
	// In production, this could be more sophisticated
	for modelName, modelConfig := range s.config.Models {
		if modelConfig.Enabled {
			s.activeModel = modelName
			s.logger.Infow("Set active model", "model", modelName)
			return
		}
	}

	s.logger.Warn("No active model found")
}

func (s *PredictionService) metricsIngestionLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Ingest metrics every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics, err := s.fetchMetricsFromPrometheus(ctx)
			if err != nil {
				s.logger.Errorw("Failed to fetch metrics", "error", err)
				continue
			}

			select {
			case s.metricsChan <- metrics:
			default:
				s.logger.Warn("Metrics channel full, dropping metrics")
			}
		}
	}
}

func (s *PredictionService) predictionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.predictionTicker.C:
			err := s.makePrediction(ctx)
			if err != nil {
				s.logger.Errorw("Prediction failed", "error", err)
			}
		}
	}
}

func (s *PredictionService) makePrediction(ctx context.Context) error {
	// Check for manual override
	s.mu.RLock()
	hasOverride := s.manualOverride != nil && time.Now().Before(s.overrideExpiry)
	s.mu.RUnlock()

	if hasOverride {
		return s.handleManualOverride(ctx)
	}

	// Get current metrics
	metrics, err := s.fetchMetricsFromPrometheus(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// Make prediction using active model
	model := s.models[s.activeModel]
	if model == nil {
		return fmt.Errorf("active model %s not found", s.activeModel)
	}

	startTime := time.Now()
	prediction, err := model.Predict(ctx, metrics)
	predictionLatency := time.Since(startTime)

	if err != nil {
		s.updateModelError(s.activeModel)
		return fmt.Errorf("prediction failed: %w", err)
	}

	s.updateModelMetrics(s.activeModel, predictionLatency)

	// Store prediction
	s.mu.Lock()
	s.lastPrediction = prediction
	s.predictionHistory = append(s.predictionHistory, prediction)
	if len(s.predictionHistory) > s.config.PredictionHistorySize {
		s.predictionHistory = s.predictionHistory[1:]
	}
	s.mu.Unlock()

	// Generate scaling decision
	decision, err := s.generateScalingDecision(ctx, prediction, metrics)
	if err != nil {
		return fmt.Errorf("failed to generate scaling decision: %w", err)
	}

	// Apply constraints
	decision = s.applyScalingConstraints(decision)

	// Coordinate with other regions if needed
	if s.regionCoordinator != nil {
		decision, err = s.coordinateWithRegions(ctx, decision)
		if err != nil {
			s.logger.Errorw("Region coordination failed", "error", err)
			// Continue with local decision
		}
	}

	// Execute scaling decision
	if s.onScalingDecision != nil {
		err = s.onScalingDecision(decision)
		if err != nil {
			s.logger.Errorw("Failed to execute scaling decision", "error", err)
		}
	}

	// Handle alerts
	for _, alert := range prediction.Alerts {
		if s.onAlert != nil {
			err = s.onAlert(alert)
			if err != nil {
				s.logger.Errorw("Failed to send alert", "error", err)
			}
		}
	}

	s.logger.Infow("Prediction completed",
		"predicted_load", prediction.PredictedLoad,
		"confidence", prediction.Confidence,
		"scaling_action", decision.Action,
		"target_instances", decision.TargetInstances,
		"alerts", len(prediction.Alerts),
	)

	return nil
}

func (s *PredictionService) fetchMetricsFromPrometheus(ctx context.Context) (*LoadMetrics, error) {
	// This is a simplified implementation
	// In production, you would fetch actual metrics from Prometheus
	now := time.Now()

	// Example metrics - replace with actual Prometheus queries
	return &LoadMetrics{
		Timestamp: now,
		LoadMetrics: &LoadMetrics{
			CPU:    0.7,  // 70% CPU usage
			Memory: 0.6,  // 60% memory usage
			RPS:    1000, // 1000 requests per second
		},
		TradingMetrics: &TradingMetrics{
			OrdersPerSecond:  50,
			TradesPerSecond:  25,
			ActiveOrders:     1000,
			OrderBookDepth:   500,
			SpreadVolatility: decimal.NewFromFloat(0.02),
			MarketVolatility: decimal.NewFromFloat(0.15),
		},
		SystemMetrics: &SystemMetrics{
			DiskIOPS:            100,
			NetworkIOBytes:      1024 * 1024, // 1MB
			DatabaseConnections: 50,
			MessageQueueSize:    100,
		},
	}, nil
}

func (s *PredictionService) generateScalingDecision(ctx context.Context, prediction *PredictionResult, metrics *LoadMetrics) (*ScalingDecision, error) {
	// Simple scaling logic - can be made more sophisticated
	currentInstances := 5 // This should come from actual infrastructure
	targetInstances := currentInstances
	action := "no_change"
	reason := "Load within normal range"

	// Determine scaling action based on prediction
	if prediction.PredictedLoad > 0.8 {
		targetInstances = int(float64(currentInstances) * 1.5)
		action = "scale_up"
		reason = "High load predicted"
	} else if prediction.PredictedLoad < 0.3 && currentInstances > s.config.ScalingConstraints.MinInstances {
		targetInstances = int(float64(currentInstances) * 0.8)
		action = "scale_down"
		reason = "Low load predicted"
	}

	// Pre-warming logic
	if prediction.PredictedLoad > s.config.ScalingConstraints.PreWarmingThreshold {
		if action == "no_change" {
			action = "pre_warm"
			targetInstances = currentInstances + 1
			reason = "Pre-warming for predicted load increase"
		}
	}

	decision := &ScalingDecision{
		Timestamp:         time.Now(),
		Action:            action,
		CurrentInstances:  currentInstances,
		TargetInstances:   targetInstances,
		Reason:            reason,
		Confidence:        prediction.Confidence,
		PredictedLoad:     prediction.PredictedLoad,
		ModelUsed:         s.activeModel,
		CostImpact:        s.calculateCostImpact(currentInstances, targetInstances),
		ManualOverride:    false,
		RegionCoordinated: false,
	}

	return decision, nil
}

func (s *PredictionService) applyScalingConstraints(decision *ScalingDecision) *ScalingDecision {
	constraints := s.config.ScalingConstraints

	// Apply min/max instance limits
	if decision.TargetInstances < constraints.MinInstances {
		decision.TargetInstances = constraints.MinInstances
		decision.Reason += " (limited by min instances)"
	}

	if decision.TargetInstances > constraints.MaxInstances {
		decision.TargetInstances = constraints.MaxInstances
		decision.Reason += " (limited by max instances)"
	}

	// Apply scaling rate limits
	instanceDiff := decision.TargetInstances - decision.CurrentInstances
	maxChange := int(constraints.MaxScaleUpRate * s.config.PredictionInterval.Minutes())

	if instanceDiff > 0 && instanceDiff > maxChange {
		decision.TargetInstances = decision.CurrentInstances + maxChange
		decision.Reason += " (limited by scale-up rate)"
	}

	maxDownChange := int(constraints.MaxScaleDownRate * s.config.PredictionInterval.Minutes())
	if instanceDiff < 0 && -instanceDiff > maxDownChange {
		decision.TargetInstances = decision.CurrentInstances - maxDownChange
		decision.Reason += " (limited by scale-down rate)"
	}

	// Apply cost constraints
	if constraints.CostConstraints != nil {
		newCost := s.calculateHourlyCost(decision.TargetInstances)
		if newCost.GreaterThan(constraints.CostConstraints.MaxHourlyCost) {
			maxInstances := int(constraints.CostConstraints.MaxHourlyCost.Div(constraints.CostConstraints.CostPerInstance).IntPart())
			decision.TargetInstances = maxInstances
			decision.Reason += " (limited by cost constraints)"
		}
	}

	return decision
}

func (s *PredictionService) calculateCostImpact(current, target int) decimal.Decimal {
	if s.config.ScalingConstraints.CostConstraints == nil {
		return decimal.Zero
	}

	costPerInstance := s.config.ScalingConstraints.CostConstraints.CostPerInstance
	instanceDiff := decimal.NewFromInt(int64(target - current))

	return instanceDiff.Mul(costPerInstance)
}

func (s *PredictionService) calculateHourlyCost(instances int) decimal.Decimal {
	if s.config.ScalingConstraints.CostConstraints == nil {
		return decimal.Zero
	}

	costPerInstance := s.config.ScalingConstraints.CostConstraints.CostPerInstance
	return costPerInstance.Mul(decimal.NewFromInt(int64(instances)))
}

func (s *PredictionService) updateModelMetrics(modelName string, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	metrics := s.modelMetrics[modelName]
	metrics.PredictionCount++
	metrics.LastPrediction = time.Now()

	// Update average latency (simplified)
	if metrics.AverageLatency == 0 {
		metrics.AverageLatency = latency
	} else {
		metrics.AverageLatency = (metrics.AverageLatency + latency) / 2
	}
}

func (s *PredictionService) updateModelError(modelName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	metrics := s.modelMetrics[modelName]
	metrics.ErrorCount++
	now := time.Now()
	metrics.LastError = &now
}

func (s *PredictionService) handleManualOverride(ctx context.Context) error {
	s.mu.RLock()
	override := s.manualOverride
	s.mu.RUnlock()

	decision := &ScalingDecision{
		Timestamp:         time.Now(),
		Action:            override.OverrideType,
		CurrentInstances:  5, // This should come from actual infrastructure
		TargetInstances:   *override.TargetInstances,
		Reason:            fmt.Sprintf("Manual override: %s", override.Reason),
		Confidence:        1.0,
		PredictedLoad:     0,
		ModelUsed:         "manual_override",
		CostImpact:        s.calculateCostImpact(5, *override.TargetInstances),
		ManualOverride:    true,
		RegionCoordinated: false,
	}

	if s.onScalingDecision != nil {
		return s.onScalingDecision(decision)
	}

	return nil
}

func (s *PredictionService) coordinateWithRegions(ctx context.Context, decision *ScalingDecision) (*ScalingDecision, error) {
	// Sync decision with peer regions
	err := s.regionCoordinator.SyncScalingDecision(ctx, decision)
	if err != nil {
		return decision, err
	}

	// Get peer region status
	peerStatus, err := s.regionCoordinator.GetPeerRegionStatus(ctx)
	if err != nil {
		return decision, err
	}

	// Check for conflicts and resolve them
	decisions := []*ScalingDecision{decision}
	for _, status := range peerStatus {
		if status.LastDecision != nil {
			decisions = append(decisions, status.LastDecision)
		}
	}

	if len(decisions) > 1 {
		resolvedDecision, err := s.regionCoordinator.ResolveConflict(ctx, decisions)
		if err != nil {
			return decision, err
		}
		resolvedDecision.RegionCoordinated = true
		return resolvedDecision, nil
	}

	decision.RegionCoordinated = true
	return decision, nil
}

func (s *PredictionService) modelRetrainingLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour) // Check for retraining daily
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkModelRetraining(ctx)
		}
	}
}

func (s *PredictionService) checkModelRetraining(ctx context.Context) {
	for modelName, modelConfig := range s.config.Models {
		if !modelConfig.Enabled {
			continue
		}

		// Check if model needs retraining
		model := s.models[modelName]
		if trainable, ok := model.(TrainableModel); ok {
			needsRetraining := s.shouldRetrainModel(modelName, modelConfig.TrainingConfig)
			if needsRetraining {
				s.logger.Infow("Starting model retraining", "model", modelName)
				go s.retrainModel(ctx, modelName, trainable, modelConfig.TrainingConfig)
			}
		}
	}
}

func (s *PredictionService) shouldRetrainModel(modelName string, config *TrainingConfig) bool {
	metrics := s.modelMetrics[modelName]

	// Check if enough time has passed since last training
	// This is simplified - in production you'd check actual training timestamps
	return time.Since(metrics.LastPrediction) > config.RetrainingInterval
}

func (s *PredictionService) retrainModel(ctx context.Context, modelName string, model TrainableModel, config *TrainingConfig) {
	// Fetch training data
	trainingData, err := s.fetchTrainingData(ctx, config.MinTrainingData)
	if err != nil {
		s.logger.Errorw("Failed to fetch training data", "model", modelName, "error", err)
		return
	}

	// Train model
	err = model.Train(ctx, trainingData)
	if err != nil {
		s.logger.Errorw("Model training failed", "model", modelName, "error", err)
		return
	}

	s.logger.Infow("Model retrained successfully", "model", modelName)
}

func (s *PredictionService) fetchTrainingData(ctx context.Context, minSamples int) ([]*TrainingData, error) {
	// This would fetch historical data from your data store
	// For now, return empty slice
	return make([]*TrainingData, 0), nil
}

func (s *PredictionService) abTestingLoop(ctx context.Context) {
	if s.abTestConfig == nil || !s.abTestConfig.Enabled {
		return
	}

	ticker := time.NewTicker(s.abTestConfig.TestDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateABTest(ctx)
		}
	}
}

func (s *PredictionService) evaluateABTest(ctx context.Context) {
	// Evaluate A/B test results and potentially switch models
	s.logger.Info("Evaluating A/B test results")

	// This is where you would implement statistical significance testing
	// and decide whether to promote a challenger model to champion
}

func (s *PredictionService) regionCoordinationLoop(ctx context.Context) {
	if s.regionCoordinator == nil {
		return
	}

	ticker := time.NewTicker(s.config.RegionConfig.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncWithPeerRegions(ctx)
		}
	}
}

func (s *PredictionService) syncWithPeerRegions(ctx context.Context) {
	// Sync current status with peer regions
	event := &RegionEvent{
		EventType: "status_sync",
		Region:    s.config.RegionConfig.CurrentRegion,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"last_prediction": s.lastPrediction,
			"model_metrics":   s.modelMetrics,
		},
		Priority: 1,
	}

	err := s.regionCoordinator.NotifyPeerRegions(ctx, event)
	if err != nil {
		s.logger.Errorw("Failed to sync with peer regions", "error", err)
	}
}

// TrainableModel interface for models that support training
type TrainableModel interface {
	Train(ctx context.Context, data []*TrainingData) error
}
