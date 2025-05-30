// =============================
// ML Auto-scaling Integration Service
// =============================
// This service integrates the feature engineering, training pipeline, and auto-scaling controller
// to provide a complete ML-based load prediction and auto-scaling solution.

package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/scaling/controller"
	"github.com/Aidin1998/pincex_unified/internal/scaling/features"
	"github.com/Aidin1998/pincex_unified/internal/scaling/predictor"
	"github.com/Aidin1998/pincex_unified/internal/scaling/training"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// MLAutoScalingService integrates all ML auto-scaling components
type MLAutoScalingService struct {
	config *MLAutoScalingConfig
	logger *zap.SugaredLogger

	// Core components
	featureEngineer   *features.FeatureEngineer
	trainingPipeline  *training.TrainingPipeline
	predictionService *predictor.PredictionService
	autoScaler        *controller.AutoScaler

	// Clients
	k8sClient     kubernetes.Interface
	prometheusAPI v1.API

	// State management
	mu           sync.RWMutex
	running      bool
	healthStatus *HealthStatus

	// Monitoring
	metrics *ServiceMetrics
	alerts  chan *Alert

	// Callbacks
	onPrediction func(*PredictionResult) error
	onScaling    func(*ScalingEvent) error
	onAlert      func(*Alert) error
}

// MLAutoScalingConfig contains configuration for the ML auto-scaling service
type MLAutoScalingConfig struct {
	// Component configurations
	FeatureEngineering *features.FeatureConfig      `yaml:"feature_engineering"`
	TrainingPipeline   *training.TrainingConfig     `yaml:"training_pipeline"`
	AutoScaler         *controller.AutoScalerConfig `yaml:"autoscaler"`

	// Integration settings
	Integration *IntegrationConfig `yaml:"integration"`
	Monitoring  *MonitoringConfig  `yaml:"monitoring"`
	Logging     *LoggingConfig     `yaml:"logging"`

	// Service settings
	UpdateInterval      time.Duration `yaml:"update_interval"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	AlertingEnabled     bool          `yaml:"alerting_enabled"`
}

// IntegrationConfig defines integration settings
type IntegrationConfig struct {
	Kubernetes *KubernetesConfig `yaml:"kubernetes"`
	Prometheus *PrometheusConfig `yaml:"prometheus"`
	Grafana    *GrafanaConfig    `yaml:"grafana"`
}

// KubernetesConfig defines Kubernetes connection settings
type KubernetesConfig struct {
	ConfigPath string `yaml:"config_path"`
	Namespace  string `yaml:"namespace"`
}

// PrometheusConfig defines Prometheus connection settings
type PrometheusConfig struct {
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

// GrafanaConfig defines Grafana settings
type GrafanaConfig struct {
	URL         string `yaml:"url"`
	DashboardID string `yaml:"dashboard_id"`
}

// MonitoringConfig defines monitoring settings
type MonitoringConfig struct {
	Prometheus *PrometheusMonitoring `yaml:"prometheus"`
	Alerting   *AlertingConfig       `yaml:"alerting"`
}

// PrometheusMonitoring defines Prometheus monitoring
type PrometheusMonitoring struct {
	Enabled       bool                  `yaml:"enabled"`
	MetricsPort   int                   `yaml:"metrics_port"`
	CustomMetrics []*CustomMetricConfig `yaml:"custom_metrics"`
}

// CustomMetricConfig defines custom metrics
type CustomMetricConfig struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// AlertingConfig defines alerting configuration
type AlertingConfig struct {
	Enabled    bool         `yaml:"enabled"`
	Channels   []string     `yaml:"channels"`
	AlertRules []*AlertRule `yaml:"alert_rules"`
}

// AlertRule defines an alerting rule
type AlertRule struct {
	Name      string        `yaml:"name"`
	Condition string        `yaml:"condition"`
	Severity  string        `yaml:"severity"`
	Duration  time.Duration `yaml:"duration"`
}

// LoggingConfig defines logging configuration
type LoggingConfig struct {
	Level  string                 `yaml:"level"`
	Format string                 `yaml:"format"`
	Output string                 `yaml:"output"`
	Fields map[string]interface{} `yaml:"fields"`
}

// Data structures for monitoring and alerting

// HealthStatus represents the health status of the service
type HealthStatus struct {
	Overall         string                      `json:"overall"`
	Components      map[string]*ComponentHealth `json:"components"`
	LastHealthCheck time.Time                   `json:"last_health_check"`
	Uptime          time.Duration               `json:"uptime"`
	StartTime       time.Time                   `json:"start_time"`
}

// ComponentHealth represents the health of a specific component
type ComponentHealth struct {
	Status     string    `json:"status"`
	LastCheck  time.Time `json:"last_check"`
	ErrorCount int       `json:"error_count"`
	LastError  string    `json:"last_error,omitempty"`
}

// ServiceMetrics contains service-level metrics
type ServiceMetrics struct {
	PredictionsGenerated int64     `json:"predictions_generated"`
	ScalingEventsTotal   int64     `json:"scaling_events_total"`
	FeaturesProcessed    int64     `json:"features_processed"`
	ModelRetrains        int64     `json:"model_retrains"`
	AlertsTriggered      int64     `json:"alerts_triggered"`
	LastUpdate           time.Time `json:"last_update"`
}

// Alert represents a service alert
type Alert struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	Type       string                 `json:"type"`
	Severity   string                 `json:"severity"`
	Component  string                 `json:"component"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details"`
	Resolved   bool                   `json:"resolved"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
}

// PredictionResult contains the result of a load prediction
type PredictionResult struct {
	Timestamp      time.Time                  `json:"timestamp"`
	PredictionType string                     `json:"prediction_type"`
	Predictions    map[string]*LoadPrediction `json:"predictions"`
	Confidence     float64                    `json:"confidence"`
	ModelUsed      string                     `json:"model_used"`
	Features       map[string]float64         `json:"features"`
	ProcessingTime time.Duration              `json:"processing_time"`
}

// LoadPrediction contains a specific load prediction
type LoadPrediction struct {
	Service           string        `json:"service"`
	CurrentLoad       float64       `json:"current_load"`
	PredictedLoad     float64       `json:"predicted_load"`
	Horizon           time.Duration `json:"horizon"`
	Confidence        float64       `json:"confidence"`
	RecommendedAction string        `json:"recommended_action"`
}

// ScalingEvent represents a scaling event
type ScalingEvent struct {
	ID              string                 `json:"id"`
	Timestamp       time.Time              `json:"timestamp"`
	Service         string                 `json:"service"`
	EventType       string                 `json:"event_type"`
	CurrentReplicas int32                  `json:"current_replicas"`
	TargetReplicas  int32                  `json:"target_replicas"`
	Reason          string                 `json:"reason"`
	Prediction      *LoadPrediction        `json:"prediction,omitempty"`
	Success         bool                   `json:"success"`
	Error           string                 `json:"error,omitempty"`
	Details         map[string]interface{} `json:"details"`
}

// NewMLAutoScalingService creates a new ML auto-scaling service
func NewMLAutoScalingService(config *MLAutoScalingConfig, logger *zap.SugaredLogger) (*MLAutoScalingService, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	service := &MLAutoScalingService{
		config:  config,
		logger:  logger,
		running: false,
		alerts:  make(chan *Alert, 1000),
		healthStatus: &HealthStatus{
			Overall:    "initializing",
			Components: make(map[string]*ComponentHealth),
			StartTime:  time.Now(),
		},
		metrics: &ServiceMetrics{
			LastUpdate: time.Now(),
		},
	}

	// Initialize Kubernetes client
	if err := service.initializeKubernetesClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	// Initialize Prometheus client
	if err := service.initializePrometheusClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize Prometheus client: %w", err)
	}

	// Initialize components
	if err := service.initializeComponents(); err != nil {
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	// Setup callbacks
	service.setupCallbacks()

	return service, nil
}

// Start starts the ML auto-scaling service
func (s *MLAutoScalingService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("service is already running")
	}

	s.logger.Info("Starting ML auto-scaling service")

	// Start feature engineering pipeline
	if err := s.featureEngineer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start feature engineering: %w", err)
	}

	// Start training pipeline
	if err := s.trainingPipeline.Start(ctx); err != nil {
		return fmt.Errorf("failed to start training pipeline: %w", err)
	}

	// Start prediction service
	if err := s.predictionService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start prediction service: %w", err)
	}

	// Start auto-scaler
	if err := s.autoScaler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start auto-scaler: %w", err)
	}

	// Start service loops
	go s.predictionLoop(ctx)
	go s.healthCheckLoop(ctx)

	if s.config.AlertingEnabled {
		go s.alertingLoop(ctx)
	}

	s.running = true
	s.healthStatus.Overall = "running"
	s.logger.Info("ML auto-scaling service started successfully")

	return nil
}

// Stop stops the ML auto-scaling service
func (s *MLAutoScalingService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("service is not running")
	}

	s.logger.Info("Stopping ML auto-scaling service")

	// Stop components
	if err := s.autoScaler.Stop(ctx); err != nil {
		s.logger.Errorw("Failed to stop auto-scaler", "error", err)
	}

	if err := s.predictionService.Stop(ctx); err != nil {
		s.logger.Errorw("Failed to stop prediction service", "error", err)
	}

	if err := s.trainingPipeline.Stop(ctx); err != nil {
		s.logger.Errorw("Failed to stop training pipeline", "error", err)
	}

	if err := s.featureEngineer.Stop(ctx); err != nil {
		s.logger.Errorw("Failed to stop feature engineering", "error", err)
	}

	// Close channels
	close(s.alerts)

	s.running = false
	s.healthStatus.Overall = "stopped"
	s.logger.Info("ML auto-scaling service stopped")

	return nil
}

// GetHealthStatus returns the current health status
func (s *MLAutoScalingService) GetHealthStatus() *HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy to avoid race conditions
	status := &HealthStatus{
		Overall:         s.healthStatus.Overall,
		Components:      make(map[string]*ComponentHealth),
		LastHealthCheck: s.healthStatus.LastHealthCheck,
		Uptime:          time.Since(s.healthStatus.StartTime),
		StartTime:       s.healthStatus.StartTime,
	}

	for name, health := range s.healthStatus.Components {
		healthCopy := *health
		status.Components[name] = &healthCopy
	}

	return status
}

// GetMetrics returns the current service metrics
func (s *MLAutoScalingService) GetMetrics() *ServiceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy
	metrics := *s.metrics
	return &metrics
}

// SetPredictionCallback sets the callback for predictions
func (s *MLAutoScalingService) SetPredictionCallback(callback func(*PredictionResult) error) {
	s.onPrediction = callback
}

// SetScalingCallback sets the callback for scaling events
func (s *MLAutoScalingService) SetScalingCallback(callback func(*ScalingEvent) error) {
	s.onScaling = callback
}

// SetAlertCallback sets the callback for alerts
func (s *MLAutoScalingService) SetAlertCallback(callback func(*Alert) error) {
	s.onAlert = callback
}

// initializeKubernetesClient initializes the Kubernetes client
func (s *MLAutoScalingService) initializeKubernetesClient() error {
	var config *rest.Config
	var err error

	if s.config.Integration.Kubernetes.ConfigPath != "" {
		// Use kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", s.config.Integration.Kubernetes.ConfigPath)
	} else {
		// Use in-cluster config
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	s.k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return nil
}

// initializePrometheusClient initializes the Prometheus client
func (s *MLAutoScalingService) initializePrometheusClient() error {
	promClient, err := api.NewClient(api.Config{
		Address: s.config.Integration.Prometheus.URL,
	})
	if err != nil {
		return fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	s.prometheusAPI = v1.NewAPI(promClient)
	return nil
}

// initializeComponents initializes all service components
func (s *MLAutoScalingService) initializeComponents() error {
	var err error

	// Initialize feature engineer
	s.featureEngineer, err = features.NewFeatureEngineer(
		s.config.FeatureEngineering,
		s.logger.Named("feature-engineer"),
		s.prometheusAPI,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize feature engineer: %w", err)
	}

	// Initialize training pipeline
	s.trainingPipeline, err = training.NewTrainingPipeline(
		s.config.TrainingPipeline,
		s.logger.Named("training-pipeline"),
		nil, // dataCollector - would need to implement
		nil, // featureEngineering - would use s.featureEngineer
		nil, // modelRegistry - would need to implement
		nil, // evaluationEngine - would need to implement
		nil, // abTestManager - would need to implement
		nil, // driftDetector - would need to implement
	)
	if err != nil {
		return fmt.Errorf("failed to initialize training pipeline: %w", err)
	} // Initialize prediction service
	s.predictionService, err = predictor.NewPredictionService(
		&predictor.ServiceConfig{
			PredictionInterval:    5 * time.Minute,
			ModelRetryInterval:    30 * time.Second,
			MetricsBufferSize:     1000,
			PredictionHistorySize: 100,
			PrometheusURL:         s.config.Integration.Prometheus.URL,
		},
		s.logger.Named("prediction-service"),
		nil, // regionCoordinator - would need to implement
	)
	if err != nil {
		return fmt.Errorf("failed to initialize prediction service: %w", err)
	}
	// Initialize auto-scaler
	s.autoScaler, err = controller.NewAutoScaler(
		s.config.AutoScaler,
		s.logger.Named("auto-scaler"),
		s.predictionService,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize auto-scaler: %w", err)
	}

	return nil
}

// setupCallbacks sets up callbacks between components
func (s *MLAutoScalingService) setupCallbacks() {
	// Feature engineer -> Training pipeline
	s.featureEngineer.SetFeaturesProcessedCallback(func(features *features.ProcessedFeatures) error {
		s.metrics.FeaturesProcessed++
		// Convert and send to training pipeline
		return nil
	})

	// Feature engineer -> Quality alerts
	s.featureEngineer.SetQualityAlertCallback(func(alert *features.FeatureQualityAlert) error {
		s.sendAlert(&Alert{
			ID:        fmt.Sprintf("feature-quality-%d", time.Now().Unix()),
			Timestamp: time.Now(),
			Type:      "feature_quality",
			Severity:  alert.Severity,
			Component: "feature-engineer",
			Message:   alert.Description,
		})
		return nil
	})

	// Auto-scaler -> Scaling events
	s.autoScaler.SetScalingEventCallback(func(event *controller.ScalingEvent) error {
		s.metrics.ScalingEventsTotal++
		scalingEvent := &ScalingEvent{
			ID:              fmt.Sprintf("scaling-%d", time.Now().Unix()),
			Timestamp:       event.Timestamp,
			Service:         event.DeploymentName,
			EventType:       event.EventType,
			CurrentReplicas: event.PreviousReplicas,
			TargetReplicas:  event.NewReplicas,
			Reason:          event.Reason,
			Success:         event.Success,
		}

		if s.onScaling != nil {
			return s.onScaling(scalingEvent)
		}
		return nil
	})
}

// predictionLoop runs the main prediction loop
func (s *MLAutoScalingService) predictionLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runPredictionCycle(ctx)
		}
	}
}

// runPredictionCycle runs a single prediction cycle
func (s *MLAutoScalingService) runPredictionCycle(ctx context.Context) {
	startTime := time.Now()

	// Get latest features
	features, err := s.featureEngineer.GetFeatures(ctx)
	if err != nil {
		s.logger.Errorw("Failed to get features", "error", err)
		return
	}
	// Get current prediction from the prediction service
	currentPrediction := s.predictionService.GetCurrentPrediction()
	if currentPrediction == nil {
		s.logger.Warnw("No current prediction available")
		return
	}

	// Create prediction result
	result := &PredictionResult{
		Timestamp:      time.Now(),
		PredictionType: "load",
		Predictions:    make(map[string]*LoadPrediction),
		Confidence:     currentPrediction.Confidence,
		ModelUsed:      currentPrediction.ModelMetadata.ModelType,
		Features:       features.ScaledFeatures,
		ProcessingTime: time.Since(startTime),
	}
	// Convert prediction to load predictions (simplified for trading service)
	currentLoad := 0.5 // This would come from current metrics
	result.Predictions["trading-service"] = &LoadPrediction{
		Service:           "trading-service",
		CurrentLoad:       currentLoad,
		PredictedLoad:     currentPrediction.PredictedLoad.CPUUtilization,
		Horizon:           currentPrediction.PredictionHorizon,
		Confidence:        currentPrediction.Confidence,
		RecommendedAction: s.determineRecommendedAction(currentLoad, currentPrediction.PredictedLoad.CPUUtilization),
	}

	s.metrics.PredictionsGenerated++

	// Call callback if set
	if s.onPrediction != nil {
		if err := s.onPrediction(result); err != nil {
			s.logger.Errorw("Prediction callback failed", "error", err)
		}
	}
}

// determineRecommendedAction determines the recommended scaling action
func (s *MLAutoScalingService) determineRecommendedAction(currentLoad, predictedLoad float64) string {
	changePercent := (predictedLoad - currentLoad) / currentLoad * 100

	if changePercent > 20 {
		return "scale_up"
	} else if changePercent < -20 {
		return "scale_down"
	}
	return "maintain"
}

// healthCheckLoop runs periodic health checks
func (s *MLAutoScalingService) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performHealthCheck(ctx)
		}
	}
}

// performHealthCheck performs a health check on all components
func (s *MLAutoScalingService) performHealthCheck(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.healthStatus.LastHealthCheck = time.Now()
	s.healthStatus.Uptime = time.Since(s.healthStatus.StartTime)

	// Check each component
	components := map[string]interface{}{
		"feature-engineer":   s.featureEngineer,
		"training-pipeline":  s.trainingPipeline,
		"prediction-service": s.predictionService,
		"auto-scaler":        s.autoScaler,
	}

	allHealthy := true
	for name, component := range components {
		health := &ComponentHealth{
			Status:    "healthy",
			LastCheck: time.Now(),
		}

		// Perform component-specific health checks
		if err := s.checkComponentHealth(ctx, name, component); err != nil {
			health.Status = "unhealthy"
			health.LastError = err.Error()
			health.ErrorCount++
			allHealthy = false
		}

		s.healthStatus.Components[name] = health
	}

	if allHealthy {
		s.healthStatus.Overall = "healthy"
	} else {
		s.healthStatus.Overall = "degraded"
	}
}

// checkComponentHealth performs health check on a specific component
func (s *MLAutoScalingService) checkComponentHealth(ctx context.Context, name string, component interface{}) error {
	// Component-specific health check logic would go here
	// For now, return nil (healthy)
	return nil
}

// alertingLoop processes alerts
func (s *MLAutoScalingService) alertingLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case alert := <-s.alerts:
			s.processAlert(alert)
		}
	}
}

// sendAlert sends an alert
func (s *MLAutoScalingService) sendAlert(alert *Alert) {
	select {
	case s.alerts <- alert:
		s.metrics.AlertsTriggered++
	default:
		s.logger.Warn("Alert channel full, dropping alert", "alert_id", alert.ID)
	}
}

// processAlert processes an alert
func (s *MLAutoScalingService) processAlert(alert *Alert) {
	s.logger.Warnw("Processing alert",
		"alert_id", alert.ID,
		"type", alert.Type,
		"severity", alert.Severity,
		"message", alert.Message,
	)

	// Call callback if set
	if s.onAlert != nil {
		if err := s.onAlert(alert); err != nil {
			s.logger.Errorw("Alert callback failed", "error", err)
		}
	}
}
