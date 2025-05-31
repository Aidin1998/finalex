// =============================
// Auto-Scaling Controller
// =============================
// This controller implements intelligent auto-scaling with Kubernetes HPA integration,
// pre-warming, graceful scale-down, and cost optimization.

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"

	"github.com/Aidin1998/pincex_unified/internal/scaling/predictor"
)

// ScalingConstraints defines global scaling constraints
type ScalingConstraints struct {
	MaxTotalReplicas     int32         `json:"max_total_replicas"`
	MinTotalReplicas     int32         `json:"min_total_replicas"`
	MaxScaleUpPercent    float64       `json:"max_scale_up_percent"`
	MaxScaleDownPercent  float64       `json:"max_scale_down_percent"`
	CooldownPeriod       time.Duration `json:"cooldown_period"`
	StabilizationWindow  time.Duration `json:"stabilization_window"`
	MaxConcurrentScaling int           `json:"max_concurrent_scaling"`
}

// AutoScaler manages intelligent auto-scaling with ML predictions
type AutoScaler struct {
	config            *AutoScalerConfig
	logger            *zap.SugaredLogger
	k8sClient         kubernetes.Interface
	metricsClient     metricsv1beta1.MetricsV1beta1Interface
	predictionService *predictor.PredictionService

	// State management
	mu                  sync.RWMutex
	scalingHistory      []*ScalingEvent
	activeDeployments   map[string]*DeploymentState
	preWarmingInstances map[string]*PreWarmInstance
	costTracker         *CostTracker

	// Scaling coordination
	scalingInProgress map[string]bool
	lastScalingAction map[string]time.Time
	cooldownTracker   map[string]time.Time

	// Health monitoring
	healthChecker HealthChecker
	errorTracker  *ErrorTracker

	// Event handlers
	onScalingEvent func(*ScalingEvent) error
	onCostAlert    func(*CostAlert) error
	onHealthAlert  func(*HealthAlert) error
}

// AutoScalerConfig contains configuration for the auto-scaler
type AutoScalerConfig struct {
	Namespace          string                       `json:"namespace"`
	Deployments        map[string]*DeploymentConfig `json:"deployments"`
	HPAConfigs         map[string]*HPAConfig        `json:"hpa_configs"`
	ScalingConstraints *ScalingConstraints          `json:"scaling_constraints"`
	PreWarmingConfig   *PreWarmingConfig            `json:"pre_warming_config"`
	GracefulScaleDown  *GracefulScaleDownConfig     `json:"graceful_scale_down"`
	CostOptimization   *CostOptimizationConfig      `json:"cost_optimization"`
	HealthChecks       *HealthCheckConfig           `json:"health_checks"`
	MonitoringConfig   *MonitoringConfig            `json:"monitoring_config"`
	EmergencyConfig    *EmergencyConfig             `json:"emergency_config"`
}

// DeploymentConfig contains configuration for a specific deployment
type DeploymentConfig struct {
	Name            string                `json:"name"`
	MinReplicas     int32                 `json:"min_replicas"`
	MaxReplicas     int32                 `json:"max_replicas"`
	TargetCPU       int32                 `json:"target_cpu"`
	TargetMemory    int32                 `json:"target_memory"`
	CustomMetrics   []*CustomMetric       `json:"custom_metrics"`
	ScalingBehavior *ScalingBehavior      `json:"scaling_behavior"`
	Resources       *ResourceRequirements `json:"resources"`
	Labels          map[string]string     `json:"labels"`
	Annotations     map[string]string     `json:"annotations"`
	Strategy        string                `json:"strategy"` // "predictive", "reactive", "hybrid"
	HPAConfig       *HPAConfig            `json:"hpa_config"`
}

// CustomMetric defines custom metrics for HPA
type CustomMetric struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "Pods", "Object", "External"
	TargetValue string            `json:"target_value"`
	Selector    map[string]string `json:"selector,omitempty"`
}

// ScalingBehavior defines how the deployment should scale
type ScalingBehavior struct {
	ScaleUp   *ScalingPolicy `json:"scale_up"`
	ScaleDown *ScalingPolicy `json:"scale_down"`
}

// ScalingPolicy defines scaling policy parameters
type ScalingPolicy struct {
	StabilizationWindow *int32            `json:"stabilization_window,omitempty"`
	SelectPolicy        string            `json:"select_policy,omitempty"` // "Max", "Min", "Disabled"
	Policies            []*HPAScalingRule `json:"policies,omitempty"`
}

// HPAScalingRule defines a scaling rule
type HPAScalingRule struct {
	Type          string `json:"type"` // "Percent", "Pods"
	Value         int32  `json:"value"`
	PeriodSeconds int32  `json:"period_seconds"`
}

// ResourceRequirements defines resource requirements for pods
type ResourceRequirements struct {
	Requests *ResourceList `json:"requests,omitempty"`
	Limits   *ResourceList `json:"limits,omitempty"`
}

// ResourceList defines CPU and memory resources
type ResourceList struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// HPAConfig contains HPA-specific configuration
type HPAConfig struct {
	Enabled           bool             `json:"enabled"`
	MinReplicas       *int32           `json:"min_replicas"`
	MaxReplicas       int32            `json:"max_replicas"`
	Metrics           []*MetricSpec    `json:"metrics"`
	Behavior          *ScalingBehavior `json:"behavior"`
	PredictiveEnabled bool             `json:"predictive_enabled"`
}

// MetricSpec defines metrics for HPA
type MetricSpec struct {
	Type     string                `json:"type"`
	Resource *ResourceMetricSource `json:"resource,omitempty"`
	Pods     *PodsMetricSource     `json:"pods,omitempty"`
	Object   *ObjectMetricSource   `json:"object,omitempty"`
	External *ExternalMetricSource `json:"external,omitempty"`
}

// ResourceMetricSource defines resource-based metrics
type ResourceMetricSource struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// PodsMetricSource defines pod-based metrics
type PodsMetricSource struct {
	Metric map[string]interface{} `json:"metric"`
	Target map[string]interface{} `json:"target"`
}

// ObjectMetricSource defines object-based metrics
type ObjectMetricSource struct {
	DescribedObject map[string]interface{} `json:"described_object"`
	Metric          map[string]interface{} `json:"metric"`
	Target          map[string]interface{} `json:"target"`
}

// ExternalMetricSource defines external metrics
type ExternalMetricSource struct {
	Metric map[string]interface{} `json:"metric"`
	Target map[string]interface{} `json:"target"`
}

// PreWarmingConfig defines pre-warming behavior
type PreWarmingConfig struct {
	Enabled             bool               `json:"enabled"`
	PredictionThreshold float64            `json:"prediction_threshold"`
	PreWarmInstances    int32              `json:"pre_warm_instances"`
	PreWarmDuration     time.Duration      `json:"pre_warm_duration"`
	MaxPreWarmInstances int32              `json:"max_pre_warm_instances"`
	CostLimit           decimal.Decimal    `json:"cost_limit"`
	Strategies          []*PreWarmStrategy `json:"strategies"`
}

// PreWarmStrategy defines different pre-warming strategies
type PreWarmStrategy struct {
	Name          string                 `json:"name"`
	TriggerMetric string                 `json:"trigger_metric"`
	Threshold     float64                `json:"threshold"`
	Action        string                 `json:"action"` // "scale_up", "prepare_instances", "warm_cache"
	Parameters    map[string]interface{} `json:"parameters"`
}

// GracefulScaleDownConfig defines graceful scale-down behavior
type GracefulScaleDownConfig struct {
	Enabled                bool                `json:"enabled"`
	DrainTimeout           time.Duration       `json:"drain_timeout"`
	TerminationGracePeriod time.Duration       `json:"termination_grace_period"`
	PreStopHook            *PreStopHook        `json:"pre_stop_hook"`
	ConnectionDraining     *ConnectionDraining `json:"connection_draining"`
	DataPersistence        *DataPersistence    `json:"data_persistence"`
	NotificationConfig     *NotificationConfig `json:"notification_config"`
}

// PreStopHook defines pre-stop hook configuration
type PreStopHook struct {
	Command     []string      `json:"command"`
	Timeout     time.Duration `json:"timeout"`
	RetryPolicy string        `json:"retry_policy"`
}

// ConnectionDraining defines connection draining configuration
type ConnectionDraining struct {
	Enabled         bool          `json:"enabled"`
	DrainDelay      time.Duration `json:"drain_delay"`
	MaxDrainTime    time.Duration `json:"max_drain_time"`
	HealthCheckPath string        `json:"health_check_path"`
}

// DataPersistence defines data persistence during scale-down
type DataPersistence struct {
	Enabled        bool     `json:"enabled"`
	BackupStrategy string   `json:"backup_strategy"` // "snapshot", "export", "replicate"
	StoragePaths   []string `json:"storage_paths"`
	RetentionDays  int      `json:"retention_days"`
}

// NotificationConfig defines notification settings
type NotificationConfig struct {
	Enabled  bool     `json:"enabled"`
	Channels []string `json:"channels"`
	Template string   `json:"template"`
}

// CostOptimizationConfig defines cost optimization parameters
type CostOptimizationConfig struct {
	Enabled            bool                    `json:"enabled"`
	MaxHourlyCost      decimal.Decimal         `json:"max_hourly_cost"`
	CostThresholds     []*CostThreshold        `json:"cost_thresholds"`
	SpotInstanceConfig *SpotInstanceConfig     `json:"spot_instance_config"`
	ScheduledScaling   *ScheduledScalingConfig `json:"scheduled_scaling"`
	RightSizingConfig  *RightSizingConfig      `json:"right_sizing_config"`
	CostAllocationTags map[string]string       `json:"cost_allocation_tags"`
}

// CostThreshold defines cost-based scaling thresholds
type CostThreshold struct {
	Threshold decimal.Decimal `json:"threshold"`
	Action    string          `json:"action"` // "alert", "scale_down", "pause_scaling"
	Severity  string          `json:"severity"`
}

// SpotInstanceConfig defines spot instance usage
type SpotInstanceConfig struct {
	Enabled          bool            `json:"enabled"`
	MaxSpotRatio     float64         `json:"max_spot_ratio"`
	FallbackStrategy string          `json:"fallback_strategy"`
	SpotPriceLimit   decimal.Decimal `json:"spot_price_limit"`
	InstanceTypes    []string        `json:"instance_types"`
}

// ScheduledScalingConfig defines scheduled scaling rules
type ScheduledScalingConfig struct {
	Enabled  bool                    `json:"enabled"`
	Rules    []*ScheduledScalingRule `json:"rules"`
	TimeZone string                  `json:"timezone"`
}

// ScheduledScalingRule defines a scheduled scaling rule
type ScheduledScalingRule struct {
	Name        string    `json:"name"`
	Schedule    string    `json:"schedule"` // Cron expression
	MinReplicas int32     `json:"min_replicas"`
	MaxReplicas int32     `json:"max_replicas"`
	Enabled     bool      `json:"enabled"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
}

// RightSizingConfig defines right-sizing parameters
type RightSizingConfig struct {
	Enabled              bool          `json:"enabled"`
	AnalysisPeriod       time.Duration `json:"analysis_period"`
	UtilizationThreshold float64       `json:"utilization_threshold"`
	RecommendationEngine string        `json:"recommendation_engine"`
}

// HealthCheckConfig defines health checking configuration
type HealthCheckConfig struct {
	Enabled             bool              `json:"enabled"`
	HealthCheckInterval time.Duration     `json:"health_check_interval"`
	UnhealthyThreshold  int               `json:"unhealthy_threshold"`
	HealthyThreshold    int               `json:"healthy_threshold"`
	TimeoutSeconds      int               `json:"timeout_seconds"`
	Endpoints           []*HealthEndpoint `json:"endpoints"`
}

// HealthEndpoint defines a health check endpoint
type HealthEndpoint struct {
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	Expected *ExpectedResponse `json:"expected"`
}

// ExpectedResponse defines expected health check response
type ExpectedResponse struct {
	StatusCode int               `json:"status_code"`
	Body       string            `json:"body,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Timeout    time.Duration     `json:"timeout"`
}

// MonitoringConfig defines monitoring and alerting configuration
type MonitoringConfig struct {
	MetricsEnabled   bool               `json:"metrics_enabled"`
	AlertingEnabled  bool               `json:"alerting_enabled"`
	MetricsNamespace string             `json:"metrics_namespace"`
	AlertChannels    []string           `json:"alert_channels"`
	CustomMetrics    []*CustomMetric    `json:"custom_metrics"`
	Dashboards       []*DashboardConfig `json:"dashboards"`
}

// DashboardConfig defines dashboard configuration
type DashboardConfig struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"` // "grafana", "datadog", "cloudwatch"
	Config  map[string]interface{} `json:"config"`
	Enabled bool                   `json:"enabled"`
}

// EmergencyConfig defines emergency scaling configuration
type EmergencyConfig struct {
	Enabled            bool                `json:"enabled"`
	TriggerConditions  []*EmergencyTrigger `json:"trigger_conditions"`
	EmergencyActions   []*EmergencyAction  `json:"emergency_actions"`
	NotificationConfig *NotificationConfig `json:"notification_config"`
	RecoveryConfig     *RecoveryConfig     `json:"recovery_config"`
}

// EmergencyTrigger defines conditions that trigger emergency scaling
type EmergencyTrigger struct {
	Name       string                 `json:"name"`
	Metric     string                 `json:"metric"`
	Threshold  float64                `json:"threshold"`
	Duration   time.Duration          `json:"duration"`
	Operator   string                 `json:"operator"` // ">", "<", ">=", "<=", "=="
	Parameters map[string]interface{} `json:"parameters"`
}

// EmergencyAction defines actions to take during emergency
type EmergencyAction struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // "scale_up", "circuit_breaker", "throttle", "failover"
	Parameters map[string]interface{} `json:"parameters"`
	Priority   int                    `json:"priority"`
}

// RecoveryConfig defines recovery procedures
type RecoveryConfig struct {
	AutoRecovery        bool          `json:"auto_recovery"`
	RecoveryDelay       time.Duration `json:"recovery_delay"`
	RecoveryThreshold   float64       `json:"recovery_threshold"`
	MaxRecoveryAttempts int           `json:"max_recovery_attempts"`
}

// State tracking structures

// DeploymentState tracks the state of a deployment
type DeploymentState struct {
	Name              string             `json:"name"`
	CurrentReplicas   int32              `json:"current_replicas"`
	TargetReplicas    int32              `json:"target_replicas"`
	LastScaled        time.Time          `json:"last_scaled"`
	ScalingInProgress bool               `json:"scaling_in_progress"`
	Health            *DeploymentHealth  `json:"health"`
	Metrics           *DeploymentMetrics `json:"metrics"`
	Cost              *DeploymentCost    `json:"cost"`
}

// DeploymentHealth tracks deployment health status
type DeploymentHealth struct {
	Status            string    `json:"status"` // "healthy", "unhealthy", "degraded"
	ReadyReplicas     int32     `json:"ready_replicas"`
	AvailableReplicas int32     `json:"available_replicas"`
	LastHealthCheck   time.Time `json:"last_health_check"`
	HealthScore       float64   `json:"health_score"`
	Issues            []string  `json:"issues"`
}

// DeploymentMetrics tracks deployment metrics
type DeploymentMetrics struct {
	CPUUtilization    float64   `json:"cpu_utilization"`
	MemoryUtilization float64   `json:"memory_utilization"`
	RequestsPerSecond float64   `json:"requests_per_second"`
	ErrorRate         float64   `json:"error_rate"`
	ResponseTime      float64   `json:"response_time"`
	LastUpdated       time.Time `json:"last_updated"`
}

// DeploymentCost tracks deployment cost information
type DeploymentCost struct {
	HourlyCost     decimal.Decimal `json:"hourly_cost"`
	DailyCost      decimal.Decimal `json:"daily_cost"`
	MonthlyCost    decimal.Decimal `json:"monthly_cost"`
	CostTrend      string          `json:"cost_trend"` // "increasing", "decreasing", "stable"
	LastCalculated time.Time       `json:"last_calculated"`
}

// PreWarmInstance represents a pre-warmed instance
type PreWarmInstance struct {
	InstanceID     string          `json:"instance_id"`
	CreatedAt      time.Time       `json:"created_at"`
	ExpiresAt      time.Time       `json:"expires_at"`
	Status         string          `json:"status"` // "warming", "ready", "used", "expired"
	DeploymentName string          `json:"deployment_name"`
	Cost           decimal.Decimal `json:"cost"`
}

// ScalingEvent represents a scaling event
type ScalingEvent struct {
	Timestamp        time.Time              `json:"timestamp"`
	DeploymentName   string                 `json:"deployment_name"`
	EventType        string                 `json:"event_type"` // "scale_up", "scale_down", "pre_warm", "drain"
	PreviousReplicas int32                  `json:"previous_replicas"`
	NewReplicas      int32                  `json:"new_replicas"`
	Trigger          string                 `json:"trigger"` // "prediction", "reactive", "scheduled", "manual"
	Reason           string                 `json:"reason"`
	Success          bool                   `json:"success"`
	Duration         time.Duration          `json:"duration"`
	CostImpact       decimal.Decimal        `json:"cost_impact"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// CostTracker tracks costs across deployments
type CostTracker struct {
	mu              sync.RWMutex
	deploymentCosts map[string]*DeploymentCost
	totalHourlyCost decimal.Decimal
	budgetAlerts    []*CostAlert
	lastUpdate      time.Time
}

// CostAlert represents a cost-related alert
type CostAlert struct {
	Timestamp       time.Time       `json:"timestamp"`
	AlertType       string          `json:"alert_type"` // "budget_exceeded", "cost_spike", "optimization_opportunity"
	Severity        string          `json:"severity"`
	Message         string          `json:"message"`
	DeploymentName  string          `json:"deployment_name,omitempty"`
	CurrentCost     decimal.Decimal `json:"current_cost"`
	ThresholdCost   decimal.Decimal `json:"threshold_cost"`
	Recommendations []string        `json:"recommendations"`
}

// HealthAlert represents a health-related alert
type HealthAlert struct {
	Timestamp      time.Time `json:"timestamp"`
	AlertType      string    `json:"alert_type"`
	Severity       string    `json:"severity"`
	Message        string    `json:"message"`
	DeploymentName string    `json:"deployment_name"`
	HealthScore    float64   `json:"health_score"`
	Issues         []string  `json:"issues"`
}

// ErrorTracker tracks errors and failures
type ErrorTracker struct {
	mu          sync.RWMutex
	errors      []*ScalingError
	errorCounts map[string]int
	lastError   time.Time
}

// ScalingError represents a scaling error
type ScalingError struct {
	Timestamp      time.Time `json:"timestamp"`
	DeploymentName string    `json:"deployment_name"`
	Operation      string    `json:"operation"`
	ErrorType      string    `json:"error_type"`
	Message        string    `json:"message"`
	Retryable      bool      `json:"retryable"`
	Attempts       int       `json:"attempts"`
}

// HealthChecker interface for health checking
type HealthChecker interface {
	CheckHealth(ctx context.Context, deployment string) (*DeploymentHealth, error)
	StartMonitoring(ctx context.Context) error
	StopMonitoring() error
}

// NewAutoScaler creates a new auto-scaler instance
func NewAutoScaler(
	config *AutoScalerConfig,
	logger *zap.SugaredLogger,
	predictionService *predictor.PredictionService,
) (*AutoScaler, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Initialize Kubernetes client
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Initialize metrics client
	metricsClient, err := metricsv1beta1.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	autoscaler := &AutoScaler{
		config:              config,
		logger:              logger,
		k8sClient:           k8sClient,
		metricsClient:       metricsClient,
		predictionService:   predictionService,
		activeDeployments:   make(map[string]*DeploymentState),
		preWarmingInstances: make(map[string]*PreWarmInstance),
		scalingInProgress:   make(map[string]bool),
		lastScalingAction:   make(map[string]time.Time),
		cooldownTracker:     make(map[string]time.Time),
		scalingHistory:      make([]*ScalingEvent, 0),
		costTracker: &CostTracker{
			deploymentCosts: make(map[string]*DeploymentCost),
			budgetAlerts:    make([]*CostAlert, 0),
		},
		errorTracker: &ErrorTracker{
			errors:      make([]*ScalingError, 0),
			errorCounts: make(map[string]int),
		},
	}

	// Initialize health checker
	autoscaler.healthChecker = NewDefaultHealthChecker(k8sClient, config.HealthChecks, logger)

	return autoscaler, nil
}

// Start begins the auto-scaling operations
func (a *AutoScaler) Start(ctx context.Context) error {
	a.logger.Info("Starting auto-scaler")

	// Set up prediction service callback
	a.predictionService.SetScalingDecisionCallback(a.handleScalingDecision)

	// Initialize deployments and create HPAs
	err := a.initializeDeployments(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize deployments: %w", err)
	}

	// Create or update HPAs for each deployment
	for deploymentName, deploymentConfig := range a.config.Deployments {
		if deploymentConfig.HPAConfig != nil && deploymentConfig.HPAConfig.Enabled {
			err = a.createOrUpdateHPA(ctx, deploymentName, deploymentConfig)
			if err != nil {
				a.logger.Errorw("Failed to create/update HPA",
					"deployment", deploymentName, "error", err)
			}
		}
	}

	// Start health monitoring
	if a.config.HealthChecks != nil && a.config.HealthChecks.Enabled {
		err = a.healthChecker.StartMonitoring(ctx)
		if err != nil {
			a.logger.Errorw("Failed to start health monitoring", "error", err)
		}
	}

	// Start monitoring loops
	go a.deploymentMonitoringLoop(ctx)
	go a.costTrackingLoop(ctx)
	go a.preWarmingManagementLoop(ctx)
	go a.healthMonitoringLoop(ctx)
	go a.emergencyMonitoringLoop(ctx)

	// Start scheduled scaling if enabled
	if a.config.CostOptimization != nil &&
		a.config.CostOptimization.ScheduledScaling != nil &&
		a.config.CostOptimization.ScheduledScaling.Enabled {
		go a.scheduledScalingLoop(ctx)
	}

	a.logger.Info("Auto-scaler started successfully")
	return nil
}

// createOrUpdateHPA creates or updates HPA for a deployment
func (a *AutoScaler) createOrUpdateHPA(ctx context.Context, deploymentName string, deploymentConfig *DeploymentConfig) error {
	hpaConfig := deploymentConfig.HPAConfig

	// Convert our HPA config to Kubernetes HPA spec
	hpaSpec := &autoscalingv2.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       deploymentName,
		},
		MinReplicas: hpaConfig.MinReplicas,
		MaxReplicas: hpaConfig.MaxReplicas,
	}

	// Add metrics
	for _, metricSpec := range hpaConfig.Metrics {
		metric, err := a.convertMetricSpec(metricSpec)
		if err != nil {
			return fmt.Errorf("failed to convert metric spec: %w", err)
		}
		hpaSpec.Metrics = append(hpaSpec.Metrics, metric)
	}

	// Add scaling behavior if configured
	if hpaConfig.Behavior != nil {
		behavior, err := a.convertScalingBehavior(hpaConfig.Behavior)
		if err != nil {
			return fmt.Errorf("failed to convert scaling behavior: %w", err)
		}
		hpaSpec.Behavior = behavior
	}

	// Create HPA object
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-hpa", deploymentName),
			Namespace: a.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "pincex-autoscaler",
				"app.kubernetes.io/component":  "hpa",
				"app.kubernetes.io/part-of":    "trading-platform",
			},
			Annotations: map[string]string{
				"pincex.io/predictive-scaling": fmt.Sprintf("%t", hpaConfig.PredictiveEnabled),
				"pincex.io/deployment":         deploymentName,
			},
		},
		Spec: *hpaSpec,
	}

	// Try to get existing HPA
	existing, err := a.k8sClient.AutoscalingV2().HorizontalPodAutoscalers(a.config.Namespace).Get(
		ctx, hpa.Name, metav1.GetOptions{})

	if err != nil {
		// HPA doesn't exist, create it
		_, err = a.k8sClient.AutoscalingV2().HorizontalPodAutoscalers(a.config.Namespace).Create(
			ctx, hpa, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create HPA: %w", err)
		}
		a.logger.Infow("Created HPA", "deployment", deploymentName, "hpa", hpa.Name)
	} else {
		// HPA exists, update it
		existing.Spec = *hpaSpec
		_, err = a.k8sClient.AutoscalingV2().HorizontalPodAutoscalers(a.config.Namespace).Update(
			ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update HPA: %w", err)
		}
		a.logger.Infow("Updated HPA", "deployment", deploymentName, "hpa", hpa.Name)
	}

	return nil
}

// convertMetricSpec converts our metric spec to Kubernetes metric spec
func (a *AutoScaler) convertMetricSpec(spec *MetricSpec) (autoscalingv2.MetricSpec, error) {
	switch spec.Type {
	case "Resource":
		return autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceName(spec.Resource.Name),
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: a.parseTargetValue(spec.Resource.Target),
				},
			},
		}, nil
	case "Pods":
		return autoscalingv2.MetricSpec{
			Type: autoscalingv2.PodsMetricSourceType,
			Pods: &autoscalingv2.PodsMetricSource{
				Metric: autoscalingv2.MetricIdentifier{
					Name: a.extractMetricName(spec.Pods.Metric),
					Selector: &metav1.LabelSelector{
						MatchLabels: a.extractMetricSelector(spec.Pods.Metric),
					},
				},
				Target: autoscalingv2.MetricTarget{
					Type:         autoscalingv2.AverageValueMetricType,
					AverageValue: a.parseTargetQuantity(spec.Pods.Target),
				},
			},
		}, nil
	case "External":
		return autoscalingv2.MetricSpec{
			Type: autoscalingv2.ExternalMetricSourceType,
			External: &autoscalingv2.ExternalMetricSource{
				Metric: autoscalingv2.MetricIdentifier{
					Name: a.extractMetricName(spec.External.Metric),
					Selector: &metav1.LabelSelector{
						MatchLabels: a.extractMetricSelector(spec.External.Metric),
					},
				},
				Target: autoscalingv2.MetricTarget{
					Type:  autoscalingv2.ValueMetricType,
					Value: a.parseTargetQuantity(spec.External.Target),
				},
			},
		}, nil
	default:
		return autoscalingv2.MetricSpec{}, fmt.Errorf("unsupported metric type: %s", spec.Type)
	}
}

// Stop gracefully stops the auto-scaler
func (a *AutoScaler) Stop(ctx context.Context) error {
	a.logger.Info("Stopping auto-scaler")

	// Stop health monitoring
	if a.healthChecker != nil {
		a.healthChecker.StopMonitoring()
	}

	// Clean up pre-warming instances
	err := a.cleanupPreWarmingInstances(ctx)
	if err != nil {
		a.logger.Errorw("Failed to cleanup pre-warming instances", "error", err)
	}

	a.logger.Info("Auto-scaler stopped")
	return nil
}

// SetScalingEventCallback sets the callback for scaling events
func (a *AutoScaler) SetScalingEventCallback(callback func(*ScalingEvent) error) {
	a.onScalingEvent = callback
}

// SetCostAlertCallback sets the callback for cost alerts
func (a *AutoScaler) SetCostAlertCallback(callback func(*CostAlert) error) {
	a.onCostAlert = callback
}

// SetHealthAlertCallback sets the callback for health alerts
func (a *AutoScaler) SetHealthAlertCallback(callback func(*HealthAlert) error) {
	a.onHealthAlert = callback
}

// GetDeploymentState returns the current state of a deployment
func (a *AutoScaler) GetDeploymentState(deploymentName string) (*DeploymentState, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	state, exists := a.activeDeployments[deploymentName]
	if !exists {
		return nil, fmt.Errorf("deployment %s not found", deploymentName)
	}

	// Return a copy to avoid race conditions
	stateCopy := *state
	return &stateCopy, nil
}

// GetScalingHistory returns recent scaling history
func (a *AutoScaler) GetScalingHistory(limit int) []*ScalingEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 || limit > len(a.scalingHistory) {
		limit = len(a.scalingHistory)
	}

	start := len(a.scalingHistory) - limit
	return a.scalingHistory[start:]
}

// GetCostSummary returns current cost summary
func (a *AutoScaler) GetCostSummary() *CostSummary {
	a.costTracker.mu.RLock()
	defer a.costTracker.mu.RUnlock()

	summary := &CostSummary{
		TotalHourlyCost: a.costTracker.totalHourlyCost,
		DeploymentCosts: make(map[string]*DeploymentCost),
		LastUpdated:     a.costTracker.lastUpdate,
		BudgetAlerts:    len(a.costTracker.budgetAlerts),
	}

	for name, cost := range a.costTracker.deploymentCosts {
		costCopy := *cost
		summary.DeploymentCosts[name] = &costCopy
	}

	return summary
}

// CostSummary provides a summary of current costs
type CostSummary struct {
	TotalHourlyCost decimal.Decimal            `json:"total_hourly_cost"`
	DeploymentCosts map[string]*DeploymentCost `json:"deployment_costs"`
	LastUpdated     time.Time                  `json:"last_updated"`
	BudgetAlerts    int                        `json:"budget_alerts"`
}

// Private methods

func (a *AutoScaler) handleScalingDecision(decision *predictor.ScalingDecision) error {
	a.logger.Infow("Received scaling decision",
		"action", decision.Action,
		"current_instances", decision.CurrentInstances,
		"target_instances", decision.TargetInstances,
		"confidence", decision.Confidence,
	)

	// For now, we'll apply this to all configured deployments
	// In production, you'd have logic to map instances to specific deployments
	for deploymentName := range a.config.Deployments {
		err := a.executeScalingAction(context.Background(), deploymentName, decision)
		if err != nil {
			a.logger.Errorw("Failed to execute scaling action",
				"deployment", deploymentName,
				"error", err,
			)
			continue
		}
	}

	return nil
}

func (a *AutoScaler) executeScalingAction(ctx context.Context, deploymentName string, decision *predictor.ScalingDecision) error {
	// Check if scaling is in progress
	a.mu.RLock()
	inProgress := a.scalingInProgress[deploymentName]
	a.mu.RUnlock()

	if inProgress {
		a.logger.Infow("Scaling already in progress", "deployment", deploymentName)
		return nil
	}

	// Check cooldown period
	if !a.canScale(deploymentName) {
		a.logger.Infow("Deployment in cooldown period", "deployment", deploymentName)
		return nil
	}

	// Mark scaling as in progress
	a.mu.Lock()
	a.scalingInProgress[deploymentName] = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.scalingInProgress[deploymentName] = false
		a.lastScalingAction[deploymentName] = time.Now()
		a.mu.Unlock()
	}()

	startTime := time.Now()
	var err error

	switch decision.Action {
	case "scale_up":
		err = a.scaleUp(ctx, deploymentName, int32(decision.TargetInstances))
	case "scale_down":
		err = a.scaleDown(ctx, deploymentName, int32(decision.TargetInstances))
	case "pre_warm":
		err = a.preWarmInstances(ctx, deploymentName)
	case "no_change":
		// No action needed
		return nil
	default:
		return fmt.Errorf("unknown scaling action: %s", decision.Action)
	}

	duration := time.Since(startTime)

	// Record scaling event
	event := &ScalingEvent{
		Timestamp:        startTime,
		DeploymentName:   deploymentName,
		EventType:        decision.Action,
		PreviousReplicas: int32(decision.CurrentInstances),
		NewReplicas:      int32(decision.TargetInstances),
		Trigger:          "prediction",
		Reason:           decision.Reason,
		Success:          err == nil,
		Duration:         duration,
		CostImpact:       decision.CostImpact,
		Metadata: map[string]interface{}{
			"confidence":     decision.Confidence,
			"predicted_load": decision.PredictedLoad,
			"model_used":     decision.ModelUsed,
		},
	}

	a.recordScalingEvent(event)

	if a.onScalingEvent != nil {
		go func() {
			if callbackErr := a.onScalingEvent(event); callbackErr != nil {
				a.logger.Errorw("Scaling event callback failed", "error", callbackErr)
			}
		}()
	}

	return err
}

func (a *AutoScaler) scaleUp(ctx context.Context, deploymentName string, targetReplicas int32) error {
	a.logger.Infow("Scaling up deployment",
		"deployment", deploymentName,
		"target_replicas", targetReplicas,
	)

	deployment, err := a.k8sClient.AppsV1().Deployments(a.config.Namespace).Get(
		ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check if we have pre-warmed instances
	preWarmCount := a.getAvailablePreWarmInstances(deploymentName)
	if preWarmCount > 0 {
		a.logger.Infow("Using pre-warmed instances",
			"deployment", deploymentName,
			"pre_warm_count", preWarmCount,
		)
		targetReplicas = min(targetReplicas, *deployment.Spec.Replicas+int32(preWarmCount))
	}

	// Apply scaling constraints
	deploymentConfig := a.config.Deployments[deploymentName]
	if targetReplicas > deploymentConfig.MaxReplicas {
		targetReplicas = deploymentConfig.MaxReplicas
		a.logger.Infow("Limited target replicas to max",
			"deployment", deploymentName,
			"max_replicas", deploymentConfig.MaxReplicas,
		)
	}

	// Update deployment
	deployment.Spec.Replicas = &targetReplicas
	_, err = a.k8sClient.AppsV1().Deployments(a.config.Namespace).Update(
		ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	// Consume pre-warmed instances
	a.consumePreWarmInstances(deploymentName, int(targetReplicas-*deployment.Spec.Replicas))

	return nil
}

func (a *AutoScaler) scaleDown(ctx context.Context, deploymentName string, targetReplicas int32) error {
	a.logger.Infow("Scaling down deployment",
		"deployment", deploymentName,
		"target_replicas", targetReplicas,
	)

	deployment, err := a.k8sClient.AppsV1().Deployments(a.config.Namespace).Get(
		ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Apply scaling constraints
	deploymentConfig := a.config.Deployments[deploymentName]
	if targetReplicas < deploymentConfig.MinReplicas {
		targetReplicas = deploymentConfig.MinReplicas
		a.logger.Infow("Limited target replicas to min",
			"deployment", deploymentName,
			"min_replicas", deploymentConfig.MinReplicas,
		)
	}

	// Graceful scale-down if enabled
	if a.config.GracefulScaleDown != nil && a.config.GracefulScaleDown.Enabled {
		err = a.gracefulScaleDown(ctx, deploymentName, targetReplicas)
		if err != nil {
			a.logger.Errorw("Graceful scale-down failed, proceeding with normal scale-down",
				"deployment", deploymentName,
				"error", err,
			)
		}
	}

	// Update deployment
	deployment.Spec.Replicas = &targetReplicas
	_, err = a.k8sClient.AppsV1().Deployments(a.config.Namespace).Update(
		ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

func (a *AutoScaler) preWarmInstances(ctx context.Context, deploymentName string) error {
	if a.config.PreWarmingConfig == nil || !a.config.PreWarmingConfig.Enabled {
		return nil
	}

	a.logger.Infow("Pre-warming instances", "deployment", deploymentName)

	// Check pre-warming limits
	currentPreWarm := len(a.getPreWarmInstances(deploymentName))
	if currentPreWarm >= int(a.config.PreWarmingConfig.MaxPreWarmInstances) {
		a.logger.Infow("Pre-warming limit reached", "deployment", deploymentName)
		return nil
	}

	// Check cost limits
	preWarmCost := a.calculatePreWarmCost(deploymentName, int(a.config.PreWarmingConfig.PreWarmInstances))
	if preWarmCost.GreaterThan(a.config.PreWarmingConfig.CostLimit) {
		a.logger.Infow("Pre-warming cost limit reached",
			"deployment", deploymentName,
			"cost", preWarmCost,
		)
		return nil
	}

	// Create pre-warm instances
	for i := 0; i < int(a.config.PreWarmingConfig.PreWarmInstances); i++ {
		instance := &PreWarmInstance{
			InstanceID:     fmt.Sprintf("%s-prewarm-%d-%d", deploymentName, time.Now().Unix(), i),
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(a.config.PreWarmingConfig.PreWarmDuration),
			Status:         "warming",
			DeploymentName: deploymentName,
			Cost:           a.calculateInstanceCost(deploymentName),
		}

		a.mu.Lock()
		a.preWarmingInstances[instance.InstanceID] = instance
		a.mu.Unlock()

		a.logger.Infow("Created pre-warm instance",
			"deployment", deploymentName,
			"instance_id", instance.InstanceID,
		)
	}

	return nil
}

func (a *AutoScaler) gracefulScaleDown(ctx context.Context, deploymentName string, targetReplicas int32) error {
	config := a.config.GracefulScaleDown

	a.logger.Infow("Starting graceful scale-down",
		"deployment", deploymentName,
		"target_replicas", targetReplicas,
		"drain_timeout", config.DrainTimeout,
	)

	// Get pods to be terminated
	pods, err := a.getPodsToTerminate(ctx, deploymentName, targetReplicas)
	if err != nil {
		return fmt.Errorf("failed to get pods to terminate: %w", err)
	}

	// Drain connections if enabled
	if config.ConnectionDraining != nil && config.ConnectionDraining.Enabled {
		for _, pod := range pods {
			err = a.drainPodConnections(ctx, pod, config.ConnectionDraining)
			if err != nil {
				a.logger.Errorw("Failed to drain pod connections",
					"pod", pod.Name,
					"error", err,
				)
			}
		}
	}

	// Execute pre-stop hooks if configured
	if config.PreStopHook != nil {
		for _, pod := range pods {
			err = a.executePreStopHook(ctx, pod, config.PreStopHook)
			if err != nil {
				a.logger.Errorw("Failed to execute pre-stop hook",
					"pod", pod.Name,
					"error", err,
				)
			}
		}
	}

	// Persist data if enabled
	if config.DataPersistence != nil && config.DataPersistence.Enabled {
		for _, pod := range pods {
			err = a.persistPodData(ctx, pod, config.DataPersistence)
			if err != nil {
				a.logger.Errorw("Failed to persist pod data",
					"pod", pod.Name,
					"error", err,
				)
			}
		}
	}

	// Send notifications if enabled
	if config.NotificationConfig != nil && config.NotificationConfig.Enabled {
		a.sendScaleDownNotification(deploymentName, len(pods))
	}

	return nil
}

// Monitoring loops

func (a *AutoScaler) deploymentMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.updateDeploymentStates(ctx)
		}
	}
}

func (a *AutoScaler) costTrackingLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.updateCostTracking(ctx)
		}
	}
}

func (a *AutoScaler) preWarmingManagementLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.managePreWarmingInstances(ctx)
		}
	}
}

func (a *AutoScaler) healthMonitoringLoop(ctx context.Context) {
	if a.config.HealthChecks == nil || !a.config.HealthChecks.Enabled {
		return
	}

	ticker := time.NewTicker(a.config.HealthChecks.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.performHealthChecks(ctx)
		}
	}
}

func (a *AutoScaler) emergencyMonitoringLoop(ctx context.Context) {
	if a.config.EmergencyConfig == nil || !a.config.EmergencyConfig.Enabled {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkEmergencyConditions(ctx)
		}
	}
}

func (a *AutoScaler) scheduledScalingLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 5) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkScheduledScaling(ctx)
		}
	}
}

// NewDefaultHealthChecker creates a default health checker implementation
func NewDefaultHealthChecker(k8sClient kubernetes.Interface, config *HealthCheckConfig, logger *zap.SugaredLogger) HealthChecker {
	return &defaultHealthChecker{
		k8sClient: k8sClient,
		config:    config,
		logger:    logger,
	}
}

type defaultHealthChecker struct {
	k8sClient kubernetes.Interface
	config    *HealthCheckConfig
	logger    *zap.SugaredLogger
}

func (h *defaultHealthChecker) CheckHealth(ctx context.Context, deployment string) (*DeploymentHealth, error) {
	// Stub implementation - returns healthy status
	return &DeploymentHealth{
		Status:            "healthy",
		ReadyReplicas:     1,
		AvailableReplicas: 1,
		LastHealthCheck:   time.Now(),
		HealthScore:       1.0,
		Issues:            []string{},
	}, nil
}

func (h *defaultHealthChecker) StartMonitoring(ctx context.Context) error {
	// Stub implementation
	return nil
}

func (h *defaultHealthChecker) StopMonitoring() error {
	// Stub implementation
	return nil
}

// initializeDeployments initializes deployment states and configurations
func (a *AutoScaler) initializeDeployments(ctx context.Context) error {
	// Stub implementation
	for deploymentName := range a.config.Deployments {
		a.activeDeployments[deploymentName] = &DeploymentState{
			Name:              deploymentName,
			CurrentReplicas:   1,
			TargetReplicas:    1,
			LastScaled:        time.Now(),
			ScalingInProgress: false,
			Health: &DeploymentHealth{
				Status:      "healthy",
				HealthScore: 1.0,
			},
			Metrics: &DeploymentMetrics{
				CPUUtilization:    50.0,
				MemoryUtilization: 50.0,
				LastUpdated:       time.Now(),
			},
			Cost: &DeploymentCost{
				HourlyCost:     decimal.NewFromFloat(1.0),
				LastCalculated: time.Now(),
			},
		}
	}
	return nil
}

// convertScalingBehavior converts scaling behavior configuration
func (a *AutoScaler) convertScalingBehavior(behavior *ScalingBehavior) (*autoscalingv2.HorizontalPodAutoscalerBehavior, error) {
	if behavior == nil {
		return nil, nil
	}
	// Stub implementation - returns empty behavior
	return &autoscalingv2.HorizontalPodAutoscalerBehavior{}, nil
}

// parseTargetValue parses target value from string
func (a *AutoScaler) parseTargetValue(target string) *int32 {
	// Stub implementation
	val := int32(80)
	return &val
}

// extractMetricName extracts metric name from metric configuration
func (a *AutoScaler) extractMetricName(metric map[string]interface{}) string {
	if name, ok := metric["name"].(string); ok {
		return name
	}
	return "unknown"
}

// extractMetricSelector extracts metric selector from metric configuration
func (a *AutoScaler) extractMetricSelector(metric map[string]interface{}) map[string]string {
	// Stub implementation
	return map[string]string{}
}

// parseTargetQuantity parses target quantity from configuration
func (a *AutoScaler) parseTargetQuantity(target map[string]interface{}) *resource.Quantity {
	// Stub implementation
	quantity := resource.MustParse("100m")
	return &quantity
}

// cleanupPreWarmingInstances cleans up expired pre-warming instances
func (a *AutoScaler) cleanupPreWarmingInstances(ctx context.Context) error {
	// Stub implementation
	return nil
}

// canScale checks if a deployment can be scaled based on constraints
func (a *AutoScaler) canScale(deploymentName string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check if scaling is already in progress
	if a.scalingInProgress[deploymentName] {
		return false
	}

	// Check cooldown period
	if lastScaling, exists := a.lastScalingAction[deploymentName]; exists {
		if time.Since(lastScaling) < a.config.ScalingConstraints.CooldownPeriod {
			return false
		}
	}

	return true
}

// recordScalingEvent records a scaling event in history
func (a *AutoScaler) recordScalingEvent(event *ScalingEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.scalingHistory = append(a.scalingHistory, event)

	// Keep only last 1000 events
	if len(a.scalingHistory) > 1000 {
		a.scalingHistory = a.scalingHistory[len(a.scalingHistory)-1000:]
	}
}

// getAvailablePreWarmInstances returns count of available pre-warm instances
func (a *AutoScaler) getAvailablePreWarmInstances(deploymentName string) int {
	// Stub implementation
	return 0
}

// consumePreWarmInstances consumes pre-warm instances for scaling
func (a *AutoScaler) consumePreWarmInstances(deploymentName string, count int) {
	// Stub implementation
}

// getPreWarmInstances returns pre-warm instances for a deployment
func (a *AutoScaler) getPreWarmInstances(deploymentName string) []*PreWarmInstance {
	// Stub implementation
	return []*PreWarmInstance{}
}

// calculatePreWarmCost calculates cost of pre-warming instances
func (a *AutoScaler) calculatePreWarmCost(deploymentName string, count int) decimal.Decimal {
	// Stub implementation
	return decimal.NewFromFloat(float64(count) * 0.1)
}

// calculateInstanceCost calculates cost of a single instance
func (a *AutoScaler) calculateInstanceCost(deploymentName string) decimal.Decimal {
	// Stub implementation
	return decimal.NewFromFloat(0.1)
}

// getPodsToTerminate returns pods that should be terminated during scale-down
func (a *AutoScaler) getPodsToTerminate(ctx context.Context, deploymentName string, targetReplicas int32) ([]corev1.Pod, error) {
	// Stub implementation
	return []corev1.Pod{}, nil
}

// drainPodConnections drains connections from a pod before termination
func (a *AutoScaler) drainPodConnections(ctx context.Context, pod corev1.Pod, config *ConnectionDraining) error {
	// Stub implementation
	return nil
}

// executePreStopHook executes pre-stop hook for a pod
func (a *AutoScaler) executePreStopHook(ctx context.Context, pod corev1.Pod, config *PreStopHook) error {
	// Stub implementation
	return nil
}

// persistPodData persists pod data during scale-down
func (a *AutoScaler) persistPodData(ctx context.Context, pod corev1.Pod, config *DataPersistence) error {
	// Stub implementation
	return nil
}

// sendScaleDownNotification sends notification about scale-down event
func (a *AutoScaler) sendScaleDownNotification(deploymentName string, podCount int) {
	// Stub implementation
	a.logger.Infow("Scale down notification", "deployment", deploymentName, "pods", podCount)
}

// updateDeploymentStates updates the state of all deployments
func (a *AutoScaler) updateDeploymentStates(ctx context.Context) {
	// Stub implementation
}

// updateCostTracking updates cost tracking information
func (a *AutoScaler) updateCostTracking(ctx context.Context) {
	// Stub implementation
}

// managePreWarmingInstances manages pre-warming instances
func (a *AutoScaler) managePreWarmingInstances(ctx context.Context) {
	// Stub implementation
}

// performHealthChecks performs health checks on deployments
func (a *AutoScaler) performHealthChecks(ctx context.Context) {
	// Stub implementation
}

// checkEmergencyConditions checks for emergency scaling conditions
func (a *AutoScaler) checkEmergencyConditions(ctx context.Context) {
	// Stub implementation
}

// checkScheduledScaling checks for scheduled scaling rules
func (a *AutoScaler) checkScheduledScaling(ctx context.Context) {
	// Stub implementation
}
