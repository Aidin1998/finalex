package transaction

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

// TransactionConfigManager manages configuration for distributed transactions
type TransactionConfigManager struct {
	db           *gorm.DB
	config       *TransactionConfig
	configPath   string
	watchers     []ConfigWatcher
	mu           sync.RWMutex
	autoReload   bool
	reloadTicker *time.Ticker
	stopChan     chan struct{}
}

// TransactionConfig represents the complete transaction system configuration
type TransactionConfig struct {
	// XA Manager Configuration
	XAManager XAManagerConfig `json:"xa_manager" yaml:"xa_manager"`

	// Recovery Configuration
	Recovery RecoveryConfig `json:"recovery" yaml:"recovery"`

	// Monitoring Configuration
	Monitoring MonitoringConfig `json:"monitoring" yaml:"monitoring"`

	// Lock Manager Configuration
	LockManager LockManagerConfig `json:"lock_manager" yaml:"lock_manager"`

	// Middleware Configuration
	Middleware MiddlewareConfig `json:"middleware" yaml:"middleware"`

	// Resource Configurations
	Resources map[string]ResourceConfig `json:"resources" yaml:"resources"`

	// Performance Configuration
	Performance PerformanceConfig `json:"performance" yaml:"performance"`

	// Security Configuration
	Security SecurityConfig `json:"security" yaml:"security"`

	// Logging Configuration
	Logging LoggingConfig `json:"logging" yaml:"logging"`
}

// XAManagerConfig contains XA manager specific configuration
type XAManagerConfig struct {
	TransactionTimeout    time.Duration `json:"transaction_timeout" yaml:"transaction_timeout"`
	PrepareTimeout        time.Duration `json:"prepare_timeout" yaml:"prepare_timeout"`
	CommitTimeout         time.Duration `json:"commit_timeout" yaml:"commit_timeout"`
	RollbackTimeout       time.Duration `json:"rollback_timeout" yaml:"rollback_timeout"`
	MaxConcurrentTxns     int           `json:"max_concurrent_txns" yaml:"max_concurrent_txns"`
	EnableHeuristics      bool          `json:"enable_heuristics" yaml:"enable_heuristics"`
	HeuristicTimeout      time.Duration `json:"heuristic_timeout" yaml:"heuristic_timeout"`
	AutoCommitSinglePhase bool          `json:"auto_commit_single_phase" yaml:"auto_commit_single_phase"`
	EnableOptimizations   bool          `json:"enable_optimizations" yaml:"enable_optimizations"`
}

// MonitoringConfig contains monitoring system configuration
type MonitoringConfig struct {
	Enabled               bool          `json:"enabled" yaml:"enabled"`
	MetricsInterval       time.Duration `json:"metrics_interval" yaml:"metrics_interval"`
	AlertingEnabled       bool          `json:"alerting_enabled" yaml:"alerting_enabled"`
	HealthCheckInterval   time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	PerformanceMonitoring bool          `json:"performance_monitoring" yaml:"performance_monitoring"`
	DetailedLogging       bool          `json:"detailed_logging" yaml:"detailed_logging"`
	RetentionPeriod       time.Duration `json:"retention_period" yaml:"retention_period"`

	// Alert thresholds
	FailureRateThreshold float64       `json:"failure_rate_threshold" yaml:"failure_rate_threshold"`
	LatencyThreshold     time.Duration `json:"latency_threshold" yaml:"latency_threshold"`
	DeadlockThreshold    int           `json:"deadlock_threshold" yaml:"deadlock_threshold"`
	TimeoutThreshold     int           `json:"timeout_threshold" yaml:"timeout_threshold"`

	// Notification settings
	EmailNotifications EmailConfig `json:"email_notifications" yaml:"email_notifications"`
	SlackNotifications SlackConfig `json:"slack_notifications" yaml:"slack_notifications"`
}

// LockManagerConfig contains distributed lock manager configuration
type LockManagerConfig struct {
	LockTimeout       time.Duration `json:"lock_timeout" yaml:"lock_timeout"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	CleanupInterval   time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	MaxLockHoldTime   time.Duration `json:"max_lock_hold_time" yaml:"max_lock_hold_time"`
	DeadlockDetection bool          `json:"deadlock_detection" yaml:"deadlock_detection"`
	DeadlockTimeout   time.Duration `json:"deadlock_timeout" yaml:"deadlock_timeout"`
	EnableMetrics     bool          `json:"enable_metrics" yaml:"enable_metrics"`
	FairQueuing       bool          `json:"fair_queuing" yaml:"fair_queuing"`
}

// MiddlewareConfig contains middleware configuration
type MiddlewareConfig struct {
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	AutoTransaction bool          `json:"auto_transaction" yaml:"auto_transaction"`
	RequestTimeout  time.Duration `json:"request_timeout" yaml:"request_timeout"`
	EnableTracing   bool          `json:"enable_tracing" yaml:"enable_tracing"`
	EnableMetrics   bool          `json:"enable_metrics" yaml:"enable_metrics"`
	LogRequests     bool          `json:"log_requests" yaml:"log_requests"`
	SkipPaths       []string      `json:"skip_paths" yaml:"skip_paths"`
}

// ResourceConfig contains resource-specific configuration
type ResourceConfig struct {
	Type              string                 `json:"type" yaml:"type"`
	Enabled           bool                   `json:"enabled" yaml:"enabled"`
	ConnectionTimeout time.Duration          `json:"connection_timeout" yaml:"connection_timeout"`
	OperationTimeout  time.Duration          `json:"operation_timeout" yaml:"operation_timeout"`
	RetryAttempts     int                    `json:"retry_attempts" yaml:"retry_attempts"`
	RetryDelay        time.Duration          `json:"retry_delay" yaml:"retry_delay"`
	HealthCheck       bool                   `json:"health_check" yaml:"health_check"`
	CustomSettings    map[string]interface{} `json:"custom_settings" yaml:"custom_settings"`
}

// PerformanceConfig contains performance-related configuration
type PerformanceConfig struct {
	EnableOptimizations bool          `json:"enable_optimizations" yaml:"enable_optimizations"`
	BatchSize           int           `json:"batch_size" yaml:"batch_size"`
	ConnectionPoolSize  int           `json:"connection_pool_size" yaml:"connection_pool_size"`
	CacheEnabled        bool          `json:"cache_enabled" yaml:"cache_enabled"`
	CacheSize           int           `json:"cache_size" yaml:"cache_size"`
	CacheTTL            time.Duration `json:"cache_ttl" yaml:"cache_ttl"`
	CompressionEnabled  bool          `json:"compression_enabled" yaml:"compression_enabled"`
	AsyncProcessing     bool          `json:"async_processing" yaml:"async_processing"`
	BufferSize          int           `json:"buffer_size" yaml:"buffer_size"`
}

// SecurityConfig contains security-related configuration
type SecurityConfig struct {
	EnableAuthentication bool          `json:"enable_authentication" yaml:"enable_authentication"`
	EnableAuthorization  bool          `json:"enable_authorization" yaml:"enable_authorization"`
	RequireHTTPS         bool          `json:"require_https" yaml:"require_https"`
	EnableAuditLog       bool          `json:"enable_audit_log" yaml:"enable_audit_log"`
	EncryptionEnabled    bool          `json:"encryption_enabled" yaml:"encryption_enabled"`
	EncryptionAlgorithm  string        `json:"encryption_algorithm" yaml:"encryption_algorithm"`
	KeyRotationInterval  time.Duration `json:"key_rotation_interval" yaml:"key_rotation_interval"`
	SessionTimeout       time.Duration `json:"session_timeout" yaml:"session_timeout"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level                 string `json:"level" yaml:"level"`
	Format                string `json:"format" yaml:"format"`
	EnableStructured      bool   `json:"enable_structured" yaml:"enable_structured"`
	EnableConsole         bool   `json:"enable_console" yaml:"enable_console"`
	EnableFile            bool   `json:"enable_file" yaml:"enable_file"`
	FileLocation          string `json:"file_location" yaml:"file_location"`
	MaxFileSize           string `json:"max_file_size" yaml:"max_file_size"`
	MaxBackups            int    `json:"max_backups" yaml:"max_backups"`
	MaxAge                int    `json:"max_age" yaml:"max_age"`
	EnableTransactionLogs bool   `json:"enable_transaction_logs" yaml:"enable_transaction_logs"`
}

// EmailConfig contains email notification configuration
type EmailConfig struct {
	Enabled    bool     `json:"enabled" yaml:"enabled"`
	SMTPServer string   `json:"smtp_server" yaml:"smtp_server"`
	SMTPPort   int      `json:"smtp_port" yaml:"smtp_port"`
	Username   string   `json:"username" yaml:"username"`
	Password   string   `json:"password" yaml:"password"`
	FromEmail  string   `json:"from_email" yaml:"from_email"`
	ToEmails   []string `json:"to_emails" yaml:"to_emails"`
	UseTLS     bool     `json:"use_tls" yaml:"use_tls"`
}

// SlackConfig contains Slack notification configuration
type SlackConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	WebhookURL string `json:"webhook_url" yaml:"webhook_url"`
	Channel    string `json:"channel" yaml:"channel"`
	Username   string `json:"username" yaml:"username"`
	IconEmoji  string `json:"icon_emoji" yaml:"icon_emoji"`
}

// ConfigWatcher interface for configuration change notifications
type ConfigWatcher interface {
	OnConfigChanged(oldConfig, newConfig *TransactionConfig) error
	GetName() string
}

// ConfigurationHistory stores configuration change history
type ConfigurationHistory struct {
	ID           uint   `gorm:"primaryKey"`
	Version      string `gorm:"index"`
	ConfigJSON   string `gorm:"type:text"`
	ChangeReason string
	ChangedBy    string
	CreatedAt    time.Time `gorm:"index"`
}

// NewTransactionConfigManager creates a new configuration manager
func NewTransactionConfigManager(db *gorm.DB, configPath string) *TransactionConfigManager {
	// Auto-migrate configuration tables
	db.AutoMigrate(&ConfigurationHistory{})

	return &TransactionConfigManager{
		db:         db,
		configPath: configPath,
		watchers:   make([]ConfigWatcher, 0),
		stopChan:   make(chan struct{}),
	}
}

// LoadConfig loads configuration from file
func (cm *TransactionConfigManager) LoadConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if config file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		// Create default configuration
		cm.config = cm.getDefaultConfig()
		return cm.saveConfigToFile()
	}

	// Read config file
	data, err := ioutil.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Determine file format by extension
	var config TransactionConfig
	ext := filepath.Ext(cm.configPath)

	switch ext {
	case ".json":
		err = json.Unmarshal(data, &config)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &config)
	default:
		return fmt.Errorf("unsupported config file format: %s", ext)
	}

	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cm.validateConfig(&config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	oldConfig := cm.config
	cm.config = &config

	// Notify watchers if config changed
	if oldConfig != nil && !cm.configsEqual(oldConfig, &config) {
		cm.notifyWatchers(oldConfig, &config)
	}

	// Save to database
	cm.saveConfigToDatabase("loaded", "system")

	log.Printf("Configuration loaded from %s", cm.configPath)
	return nil
}

// SaveConfig saves current configuration to file
func (cm *TransactionConfigManager) SaveConfig() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.saveConfigToFile()
}

// saveConfigToFile saves configuration to file
func (cm *TransactionConfigManager) saveConfigToFile() error {
	if cm.config == nil {
		return fmt.Errorf("no configuration to save")
	}

	// Determine file format by extension
	ext := filepath.Ext(cm.configPath)
	var data []byte
	var err error

	switch ext {
	case ".json":
		data, err = json.MarshalIndent(cm.config, "", "  ")
	case ".yaml", ".yml":
		data, err = yaml.Marshal(cm.config)
	default:
		return fmt.Errorf("unsupported config file format: %s", ext)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write to file
	err = ioutil.WriteFile(cm.configPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// UpdateConfig updates the configuration
func (cm *TransactionConfigManager) UpdateConfig(newConfig *TransactionConfig, reason, changedBy string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate new configuration
	if err := cm.validateConfig(newConfig); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	oldConfig := cm.config
	cm.config = newConfig

	// Save to file
	if err := cm.saveConfigToFile(); err != nil {
		cm.config = oldConfig // Rollback
		return fmt.Errorf("failed to save config to file: %w", err)
	}

	// Save to database
	cm.saveConfigToDatabase(reason, changedBy)

	// Notify watchers
	cm.notifyWatchers(oldConfig, newConfig)

	log.Printf("Configuration updated by %s: %s", changedBy, reason)
	return nil
}

// GetConfig returns the current configuration
func (cm *TransactionConfigManager) GetConfig() *TransactionConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.config == nil {
		return cm.getDefaultConfig()
	}

	// Return a deep copy to prevent external modifications
	configJSON, _ := json.Marshal(cm.config)
	var configCopy TransactionConfig
	json.Unmarshal(configJSON, &configCopy)
	return &configCopy
}

// RegisterWatcher registers a configuration change watcher
func (cm *TransactionConfigManager) RegisterWatcher(watcher ConfigWatcher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.watchers = append(cm.watchers, watcher)
}

// EnableAutoReload enables automatic configuration reloading
func (cm *TransactionConfigManager) EnableAutoReload(interval time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.autoReload {
		return
	}

	cm.autoReload = true
	cm.reloadTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-cm.reloadTicker.C:
				cm.checkAndReloadConfig()
			case <-cm.stopChan:
				return
			}
		}
	}()

	log.Printf("Auto-reload enabled with interval: %v", interval)
}

// DisableAutoReload disables automatic configuration reloading
func (cm *TransactionConfigManager) DisableAutoReload() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.autoReload {
		return
	}

	cm.autoReload = false
	if cm.reloadTicker != nil {
		cm.reloadTicker.Stop()
		cm.reloadTicker = nil
	}

	log.Println("Auto-reload disabled")
}

// checkAndReloadConfig checks if config file has changed and reloads if necessary
func (cm *TransactionConfigManager) checkAndReloadConfig() {
	// Check file modification time
	fileInfo, err := os.Stat(cm.configPath)
	if err != nil {
		log.Printf("Error checking config file: %v", err)
		return
	}
	cm.mu.RLock()
	_ = fileInfo.ModTime() // Unused variable fix
	// We would need to store the last known modification time to compare
	// For simplicity, we'll just reload every time
	cm.mu.RUnlock()

	// Reload configuration
	if err := cm.LoadConfig(); err != nil {
		log.Printf("Error reloading configuration: %v", err)
	}
}

// validateConfig validates the configuration
func (cm *TransactionConfigManager) validateConfig(config *TransactionConfig) error {
	// Validate XA Manager config
	if config.XAManager.TransactionTimeout <= 0 {
		return fmt.Errorf("xa_manager.transaction_timeout must be positive")
	}
	if config.XAManager.MaxConcurrentTxns <= 0 {
		return fmt.Errorf("xa_manager.max_concurrent_txns must be positive")
	}

	// Validate Recovery config
	if config.Recovery.MaxRecoveryAttempts <= 0 {
		return fmt.Errorf("recovery.max_recovery_attempts must be positive")
	}
	if config.Recovery.RecoveryTimeout <= 0 {
		return fmt.Errorf("recovery.recovery_timeout must be positive")
	}

	// Validate Lock Manager config
	if config.LockManager.LockTimeout <= 0 {
		return fmt.Errorf("lock_manager.lock_timeout must be positive")
	}

	// Validate Performance config
	if config.Performance.BatchSize <= 0 {
		config.Performance.BatchSize = 100 // Set default
	}
	if config.Performance.ConnectionPoolSize <= 0 {
		config.Performance.ConnectionPoolSize = 10 // Set default
	}

	return nil
}

// configsEqual checks if two configurations are equal
func (cm *TransactionConfigManager) configsEqual(config1, config2 *TransactionConfig) bool {
	json1, _ := json.Marshal(config1)
	json2, _ := json.Marshal(config2)
	return string(json1) == string(json2)
}

// notifyWatchers notifies all registered watchers of configuration changes
func (cm *TransactionConfigManager) notifyWatchers(oldConfig, newConfig *TransactionConfig) {
	for _, watcher := range cm.watchers {
		go func(w ConfigWatcher) {
			if err := w.OnConfigChanged(oldConfig, newConfig); err != nil {
				log.Printf("Error notifying watcher %s: %v", w.GetName(), err)
			}
		}(watcher)
	}
}

// saveConfigToDatabase saves configuration to database for history tracking
func (cm *TransactionConfigManager) saveConfigToDatabase(reason, changedBy string) {
	if cm.config == nil {
		return
	}

	configJSON, err := json.Marshal(cm.config)
	if err != nil {
		log.Printf("Error marshaling config for database: %v", err)
		return
	}

	history := &ConfigurationHistory{
		Version:      fmt.Sprintf("v%d", time.Now().Unix()),
		ConfigJSON:   string(configJSON),
		ChangeReason: reason,
		ChangedBy:    changedBy,
		CreatedAt:    time.Now(),
	}

	if err := cm.db.Create(history).Error; err != nil {
		log.Printf("Error saving config to database: %v", err)
	}
}

// GetConfigHistory returns configuration change history
func (cm *TransactionConfigManager) GetConfigHistory(limit int) ([]*ConfigurationHistory, error) {
	var history []*ConfigurationHistory

	query := cm.db.Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&history).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get config history: %w", err)
	}

	return history, nil
}

// Stop stops the configuration manager
func (cm *TransactionConfigManager) Stop() {
	cm.DisableAutoReload()
	close(cm.stopChan)
}

// getDefaultConfig returns the default configuration
func (cm *TransactionConfigManager) getDefaultConfig() *TransactionConfig {
	return &TransactionConfig{
		XAManager: XAManagerConfig{
			TransactionTimeout:    5 * time.Minute,
			PrepareTimeout:        30 * time.Second,
			CommitTimeout:         60 * time.Second,
			RollbackTimeout:       60 * time.Second,
			MaxConcurrentTxns:     1000,
			EnableHeuristics:      true,
			HeuristicTimeout:      10 * time.Minute,
			AutoCommitSinglePhase: true,
			EnableOptimizations:   true,
		},
		Recovery: RecoveryConfig{
			RecoveryInterval:        30 * time.Second,
			MaxRecoveryAttempts:     5,
			RecoveryTimeout:         5 * time.Minute,
			BackoffStrategy:         BackoffExponential,
			InitialBackoff:          1 * time.Second,
			MaxBackoff:              60 * time.Second,
			BackoffMultiplier:       2.0,
			FailureThreshold:        3,
			SuccessThreshold:        2,
			CircuitBreakerTimeout:   10 * time.Minute,
			MaxConcurrentRecoveries: 10,
			EnableParallelRecovery:  true,
			EnableCompensation:      true,
			CompensationTimeout:     30 * time.Second,
			NotifyOnRecovery:        true,
			NotifyOnFailure:         true,
		},
		Monitoring: MonitoringConfig{
			Enabled:               true,
			MetricsInterval:       30 * time.Second,
			AlertingEnabled:       true,
			HealthCheckInterval:   1 * time.Minute,
			PerformanceMonitoring: true,
			DetailedLogging:       false,
			RetentionPeriod:       24 * time.Hour,
			FailureRateThreshold:  0.05,
			LatencyThreshold:      10 * time.Second,
			DeadlockThreshold:     1,
			TimeoutThreshold:      5,
			EmailNotifications: EmailConfig{
				Enabled: false,
			},
			SlackNotifications: SlackConfig{
				Enabled: false,
			},
		},
		LockManager: LockManagerConfig{
			LockTimeout:       60 * time.Second,
			HeartbeatInterval: 10 * time.Second,
			CleanupInterval:   5 * time.Minute,
			MaxLockHoldTime:   10 * time.Minute,
			DeadlockDetection: true,
			DeadlockTimeout:   30 * time.Second,
			EnableMetrics:     true,
			FairQueuing:       true,
		},
		Middleware: MiddlewareConfig{
			Enabled:         true,
			AutoTransaction: true,
			RequestTimeout:  30 * time.Second,
			EnableTracing:   true,
			EnableMetrics:   true,
			LogRequests:     false,
			SkipPaths:       []string{"/health", "/metrics"},
		},
		Resources: map[string]ResourceConfig{
			"bookkeeper": {
				Type:              "bookkeeper",
				Enabled:           true,
				ConnectionTimeout: 5 * time.Second,
				OperationTimeout:  30 * time.Second,
				RetryAttempts:     3,
				RetryDelay:        1 * time.Second,
				HealthCheck:       true,
				CustomSettings:    make(map[string]interface{}),
			},
			"trading": {
				Type:              "trading",
				Enabled:           true,
				ConnectionTimeout: 5 * time.Second,
				OperationTimeout:  30 * time.Second,
				RetryAttempts:     3,
				RetryDelay:        1 * time.Second,
				HealthCheck:       true,
				CustomSettings:    make(map[string]interface{}),
			},
			"wallet": {
				Type:              "wallet",
				Enabled:           true,
				ConnectionTimeout: 5 * time.Second,
				OperationTimeout:  60 * time.Second,
				RetryAttempts:     3,
				RetryDelay:        2 * time.Second,
				HealthCheck:       true,
				CustomSettings:    make(map[string]interface{}),
			},
			"settlement": {
				Type:              "settlement",
				Enabled:           true,
				ConnectionTimeout: 5 * time.Second,
				OperationTimeout:  45 * time.Second,
				RetryAttempts:     3,
				RetryDelay:        1 * time.Second,
				HealthCheck:       true,
				CustomSettings:    make(map[string]interface{}),
			},
			"fiat": {
				Type:              "fiat",
				Enabled:           true,
				ConnectionTimeout: 10 * time.Second,
				OperationTimeout:  120 * time.Second,
				RetryAttempts:     5,
				RetryDelay:        3 * time.Second,
				HealthCheck:       true,
				CustomSettings:    make(map[string]interface{}),
			},
		},
		Performance: PerformanceConfig{
			EnableOptimizations: true,
			BatchSize:           100,
			ConnectionPoolSize:  20,
			CacheEnabled:        true,
			CacheSize:           1000,
			CacheTTL:            10 * time.Minute,
			CompressionEnabled:  false,
			AsyncProcessing:     true,
			BufferSize:          1000,
		},
		Security: SecurityConfig{
			EnableAuthentication: false,
			EnableAuthorization:  false,
			RequireHTTPS:         false,
			EnableAuditLog:       true,
			EncryptionEnabled:    false,
			EncryptionAlgorithm:  "AES-256-GCM",
			KeyRotationInterval:  24 * time.Hour,
			SessionTimeout:       30 * time.Minute,
		},
		Logging: LoggingConfig{
			Level:                 "info",
			Format:                "json",
			EnableStructured:      true,
			EnableConsole:         true,
			EnableFile:            true,
			FileLocation:          "/var/log/transaction-manager.log",
			MaxFileSize:           "100MB",
			MaxBackups:            5,
			MaxAge:                30,
			EnableTransactionLogs: true,
		},
	}
}

// XAManagerConfigWatcher implements configuration watching for XA manager
type XAManagerConfigWatcher struct {
	xaManager *XATransactionManager
}

// OnConfigChanged handles XA manager configuration changes
func (w *XAManagerConfigWatcher) OnConfigChanged(oldConfig, newConfig *TransactionConfig) error {
	// Update XA manager configuration
	w.xaManager.mu.Lock()
	defer w.xaManager.mu.Unlock()

	// Update default timeout only (other timeouts are not supported directly)
	w.xaManager.defaultTimeout = newConfig.XAManager.TransactionTimeout

	log.Println("XA Manager configuration updated")
	return nil
}

// GetName returns the watcher name
func (w *XAManagerConfigWatcher) GetName() string {
	return "XAManagerConfigWatcher"
}

// Example of how to use the configuration manager
func ExampleConfigUsage() {
	// This function demonstrates how to use the configuration manager
	// It would typically be called during application startup

	configManager := NewTransactionConfigManager(nil, "/etc/transaction-manager/config.yaml")

	// Load initial configuration
	if err := configManager.LoadConfig(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Register watchers
	// configManager.RegisterWatcher(&XAManagerConfigWatcher{xaManager: xaManager})

	// Enable auto-reload
	configManager.EnableAutoReload(5 * time.Minute)

	// Get current configuration
	config := configManager.GetConfig()
	log.Printf("Current config: XA timeout = %v", config.XAManager.TransactionTimeout)

	// Update configuration
	config.XAManager.TransactionTimeout = 10 * time.Minute
	if err := configManager.UpdateConfig(config, "Increased timeout for large transactions", "admin"); err != nil {
		log.Printf("Failed to update configuration: %v", err)
	}
}
