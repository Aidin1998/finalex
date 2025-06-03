package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// SimpleStrongConsistencyConfigManager manages configuration without circular dependencies
type SimpleStrongConsistencyConfigManager struct {
	configPath string
	logger     *zap.Logger

	// Configuration as raw maps to avoid import cycles
	config map[string]interface{}

	// Runtime configuration
	runtimeConfig *RuntimeConfig

	// Change tracking
	lastModified time.Time
	mutex        sync.RWMutex
}

// RuntimeConfig contains runtime configuration that can be changed dynamically
type RuntimeConfig struct {
	// Feature flags
	EnableStrongConsistency    bool `json:"enable_strong_consistency" yaml:"enable_strong_consistency"`
	EnableConsensusForCritical bool `json:"enable_consensus_for_critical" yaml:"enable_consensus_for_critical"`
	EnableDistributedLocking   bool `json:"enable_distributed_locking" yaml:"enable_distributed_locking"`
	EnableBalanceValidation    bool `json:"enable_balance_validation" yaml:"enable_balance_validation"`

	// Dynamic thresholds
	CriticalOperationThreshold float64 `json:"critical_operation_threshold" yaml:"critical_operation_threshold"`
	LargeTransferThreshold     float64 `json:"large_transfer_threshold" yaml:"large_transfer_threshold"`
	ConsensusRequiredThreshold float64 `json:"consensus_required_threshold" yaml:"consensus_required_threshold"`

	// Performance tuning
	MaxConcurrentOperations int   `json:"max_concurrent_operations" yaml:"max_concurrent_operations"`
	BatchSizeLimit          int   `json:"batch_size_limit" yaml:"batch_size_limit"`
	ProcessingTimeoutMs     int64 `json:"processing_timeout_ms" yaml:"processing_timeout_ms"`

	// Circuit breaker configuration
	CircuitBreakerEnabled    bool  `json:"circuit_breaker_enabled" yaml:"circuit_breaker_enabled"`
	CircuitBreakerThreshold  int   `json:"circuit_breaker_threshold" yaml:"circuit_breaker_threshold"`
	CircuitBreakerTimeoutMs  int64 `json:"circuit_breaker_timeout_ms" yaml:"circuit_breaker_timeout_ms"`
	CircuitBreakerRecoveryMs int64 `json:"circuit_breaker_recovery_ms" yaml:"circuit_breaker_recovery_ms"`
}

// NewSimpleStrongConsistencyConfigManager creates a new config manager without circular dependencies
func NewSimpleStrongConsistencyConfigManager(configPath string, logger *zap.Logger) *SimpleStrongConsistencyConfigManager {
	return &SimpleStrongConsistencyConfigManager{
		configPath:    configPath,
		logger:        logger,
		config:        make(map[string]interface{}),
		runtimeConfig: DefaultRuntimeConfig(),
	}
}

// LoadConfig loads configuration from file
func (sccm *SimpleStrongConsistencyConfigManager) LoadConfig() error {
	sccm.mutex.Lock()
	defer sccm.mutex.Unlock()

	if sccm.configPath == "" {
		sccm.logger.Info("No config path specified, using defaults")
		sccm.config = DefaultConfig()
		return nil
	}

	data, err := ioutil.ReadFile(sccm.configPath)
	if err != nil {
		sccm.logger.Warn("Failed to read config file, using defaults", zap.Error(err))
		sccm.config = DefaultConfig()
		return nil
	}

	var config map[string]interface{}

	// Try JSON first, then YAML
	if err := json.Unmarshal(data, &config); err != nil {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	sccm.config = config
	sccm.logger.Info("Loaded strong consistency configuration", zap.String("path", sccm.configPath))

	return nil
}

// GetConfig returns the entire configuration map
func (sccm *SimpleStrongConsistencyConfigManager) GetConfig() map[string]interface{} {
	sccm.mutex.RLock()
	defer sccm.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]interface{})
	for k, v := range sccm.config {
		result[k] = v
	}
	return result
}

// GetSectionConfig returns configuration for a specific section
func (sccm *SimpleStrongConsistencyConfigManager) GetSectionConfig(section string) map[string]interface{} {
	sccm.mutex.RLock()
	defer sccm.mutex.RUnlock()

	if sectionData, ok := sccm.config[section]; ok {
		if sectionMap, ok := sectionData.(map[string]interface{}); ok {
			return sectionMap
		}
	}

	return make(map[string]interface{})
}

// UpdateSectionConfig updates configuration for a specific section
func (sccm *SimpleStrongConsistencyConfigManager) UpdateSectionConfig(section string, config map[string]interface{}) {
	sccm.mutex.Lock()
	defer sccm.mutex.Unlock()

	sccm.config[section] = config
	sccm.lastModified = time.Now()
}

// GetRuntimeConfig returns the runtime configuration
func (sccm *SimpleStrongConsistencyConfigManager) GetRuntimeConfig() *RuntimeConfig {
	sccm.mutex.RLock()
	defer sccm.mutex.RUnlock()

	// Return a copy
	return &RuntimeConfig{
		EnableStrongConsistency:    sccm.runtimeConfig.EnableStrongConsistency,
		EnableConsensusForCritical: sccm.runtimeConfig.EnableConsensusForCritical,
		EnableDistributedLocking:   sccm.runtimeConfig.EnableDistributedLocking,
		EnableBalanceValidation:    sccm.runtimeConfig.EnableBalanceValidation,
		CriticalOperationThreshold: sccm.runtimeConfig.CriticalOperationThreshold,
		LargeTransferThreshold:     sccm.runtimeConfig.LargeTransferThreshold,
		ConsensusRequiredThreshold: sccm.runtimeConfig.ConsensusRequiredThreshold,
		MaxConcurrentOperations:    sccm.runtimeConfig.MaxConcurrentOperations,
		BatchSizeLimit:             sccm.runtimeConfig.BatchSizeLimit,
		ProcessingTimeoutMs:        sccm.runtimeConfig.ProcessingTimeoutMs,
		CircuitBreakerEnabled:      sccm.runtimeConfig.CircuitBreakerEnabled,
		CircuitBreakerThreshold:    sccm.runtimeConfig.CircuitBreakerThreshold,
		CircuitBreakerTimeoutMs:    sccm.runtimeConfig.CircuitBreakerTimeoutMs,
		CircuitBreakerRecoveryMs:   sccm.runtimeConfig.CircuitBreakerRecoveryMs,
	}
}

// UpdateRuntimeConfig updates the runtime configuration
func (sccm *SimpleStrongConsistencyConfigManager) UpdateRuntimeConfig(config *RuntimeConfig) {
	sccm.mutex.Lock()
	defer sccm.mutex.Unlock()

	sccm.runtimeConfig = config
	sccm.lastModified = time.Now()
}

// GetConsensusConfig returns configuration for the consensus system
func (sccm *SimpleStrongConsistencyConfigManager) GetConsensusConfig() interface{} {
	return sccm.GetSectionConfig("consensus")
}

// GetLockConfig returns configuration for the distributed lock manager
func (sccm *SimpleStrongConsistencyConfigManager) GetLockConfig() interface{} {
	return sccm.GetSectionConfig("distributed_lock")
}

// GetBalanceConfig returns configuration for the balance manager
func (sccm *SimpleStrongConsistencyConfigManager) GetBalanceConfig() interface{} {
	return sccm.GetSectionConfig("balance")
}

// GetSettlementConfig returns configuration for the settlement coordinator
func (sccm *SimpleStrongConsistencyConfigManager) GetSettlementConfig() interface{} {
	return sccm.GetSectionConfig("settlement")
}

// GetOrderConfig returns configuration for the order processor
func (sccm *SimpleStrongConsistencyConfigManager) GetOrderConfig() interface{} {
	return sccm.GetSectionConfig("order_processing")
}

// GetTransactionConfig returns configuration for the transaction manager
func (sccm *SimpleStrongConsistencyConfigManager) GetTransactionConfig() interface{} {
	return sccm.GetSectionConfig("transaction")
}

// Testing struct for testing configuration
type Testing struct {
	EnableStartupValidation bool `json:"enable_startup_validation" yaml:"enable_startup_validation"`
}

// Testing returns testing configuration (mock implementation)
func (sccm *SimpleStrongConsistencyConfigManager) Testing() *Testing {
	return &Testing{
		EnableStartupValidation: true, // Default to true for testing
	}
}

// DefaultConfig returns default configuration
func DefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"environment": "development",
		"transaction": map[string]interface{}{
			"settlement_consensus_threshold": 100000.0,
			"transfer_consensus_threshold":   50000.0,
			"consensus_timeout":              "30s",
			"lock_timeout":                   "10s",
			"max_retry_attempts":             3,
			"batch_size":                     100,
			"enable_parallel_processing":     true,
		},
		"consensus": map[string]interface{}{
			"cluster_nodes":           []string{"node-1", "node-2", "node-3"},
			"consensus_timeout":       "15s",
			"leader_election_timeout": "30s",
			"heartbeat_interval":      "5s",
			"max_retries":             3,
		},
		"balance": map[string]interface{}{
			"strict_mode":                true,
			"reconcile_interval":         "5m",
			"max_retries":                3,
			"retry_backoff":              "100ms",
			"consistency_check_interval": "1m",
		},
		"distributed_lock": map[string]interface{}{
			"default_timeout":      "30s",
			"heartbeat_interval":   "10s",
			"cleanup_interval":     "1m",
			"max_concurrent_locks": 1000,
		},
		"settlement": map[string]interface{}{
			"batch_size":          50,
			"batch_timeout":       "5s",
			"consensus_threshold": 100000.0,
			"max_retry_attempts":  3,
			"settlement_timeout":  "60s",
		},
		"order_processing": map[string]interface{}{
			"large_order_threshold":    100000.0,
			"critical_order_threshold": 1000000.0,
			"processing_timeout":       "30s",
			"consensus_timeout":        "15s",
			"balance_check_timeout":    "5s",
			"enable_atomic_matching":   true,
			"max_concurrent_orders":    100,
			"batch_size":               50,
		},
	}
}

// DefaultRuntimeConfig returns default runtime configuration
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		EnableStrongConsistency:    true,
		EnableConsensusForCritical: true,
		EnableDistributedLocking:   true,
		EnableBalanceValidation:    true,
		CriticalOperationThreshold: 100000.0,
		LargeTransferThreshold:     50000.0,
		ConsensusRequiredThreshold: 100000.0,
		MaxConcurrentOperations:    100,
		BatchSizeLimit:             50,
		ProcessingTimeoutMs:        30000,
		CircuitBreakerEnabled:      true,
		CircuitBreakerThreshold:    10,
		CircuitBreakerTimeoutMs:    60000,
		CircuitBreakerRecoveryMs:   30000,
	}
}

// Type alias for backward compatibility
type StrongConsistencyConfigManager = SimpleStrongConsistencyConfigManager

// NewStrongConsistencyConfigManager creates a new config manager (alias for the simple version)
func NewStrongConsistencyConfigManager(configPath string, logger *zap.Logger) *StrongConsistencyConfigManager {
	return NewSimpleStrongConsistencyConfigManager(configPath, logger)
}
