package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// SimpleStrongConsistencyConfigManager manages strong consistency configuration
type SimpleStrongConsistencyConfigManager struct {
	configPath string
	logger     *zap.Logger
	config     map[string]interface{}
	mutex      sync.RWMutex
	viper      *viper.Viper
}

// Type alias for the main interface - references the simple implementation
type StrongConsistencyConfigManager = SimpleStrongConsistencyConfigManager

// NewSimpleStrongConsistencyConfigManager creates a new simple config manager
func NewSimpleStrongConsistencyConfigManager(configPath string, logger *zap.Logger) *SimpleStrongConsistencyConfigManager {
	v := viper.New()
	return &SimpleStrongConsistencyConfigManager{
		configPath: configPath,
		logger:     logger.Named("strong-consistency-config"),
		config:     make(map[string]interface{}),
		viper:      v,
	}
}

// NewStrongConsistencyConfigManager creates a new config manager (alias for the simple version)
func NewStrongConsistencyConfigManager(configPath string, logger *zap.Logger) *StrongConsistencyConfigManager {
	return NewSimpleStrongConsistencyConfigManager(configPath, logger)
}

// LoadConfig loads configuration from the specified path
func (sc *SimpleStrongConsistencyConfigManager) LoadConfig() error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	sc.logger.Info("Loading strong consistency configuration", zap.String("path", sc.configPath))

	// Set up viper configuration
	if sc.configPath != "" {
		// Check if file exists
		if _, err := os.Stat(sc.configPath); os.IsNotExist(err) {
			sc.logger.Warn("Configuration file not found, using defaults",
				zap.String("path", sc.configPath))
			sc.setDefaultConfiguration()
			return nil
		}

		sc.viper.SetConfigFile(sc.configPath)
	} else {
		// Set up default configuration search paths
		sc.viper.SetConfigName("strong_consistency")
		sc.viper.SetConfigType("yaml")
		sc.viper.AddConfigPath(".")
		sc.viper.AddConfigPath("./configs")
		sc.viper.AddConfigPath("/etc/pincex")
	}

	// Try to read the configuration file
	if err := sc.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			sc.logger.Warn("Configuration file not found, using defaults")
			sc.setDefaultConfiguration()
			return nil
		}
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Load configuration into our map
	sc.config = sc.viper.AllSettings()

	sc.logger.Info("Strong consistency configuration loaded successfully",
		zap.String("file", sc.viper.ConfigFileUsed()),
		zap.Int("sections", len(sc.config)))

	return nil
}

// GetSectionConfig returns configuration for a specific section
func (sc *SimpleStrongConsistencyConfigManager) GetSectionConfig(section string) map[string]interface{} {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	if sectionConfig, exists := sc.config[section]; exists {
		if configMap, ok := sectionConfig.(map[string]interface{}); ok {
			return configMap
		}
	}

	sc.logger.Debug("Section not found in configuration, returning empty map",
		zap.String("section", section))
	return make(map[string]interface{})
}

// GetConfig returns the entire configuration
func (sc *SimpleStrongConsistencyConfigManager) GetConfig() map[string]interface{} {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]interface{})
	for k, v := range sc.config {
		result[k] = v
	}
	return result
}

// ReloadConfig reloads the configuration from file
func (sc *SimpleStrongConsistencyConfigManager) ReloadConfig() error {
	sc.logger.Info("Reloading strong consistency configuration")
	return sc.LoadConfig()
}

// setDefaultConfiguration sets up default configuration when no file is found
func (sc *SimpleStrongConsistencyConfigManager) setDefaultConfiguration() {
	sc.config = map[string]interface{}{
		"consensus": map[string]interface{}{
			"node_id":                 "node-1",
			"cluster_nodes":           []string{"node-1", "node-2", "node-3"},
			"election_timeout":        "10s",
			"heartbeat_interval":      "2s",
			"consensus_timeout":       "30s",
			"quorum_size":             3,
			"leader_election_timeout": "10s",
		},
		"distributed_lock": map[string]interface{}{
			"lock_timeout":        "30s",
			"acquisition_timeout": "10s",
			"renewal_interval":    "5s",
			"max_retry_attempts":  3,
		},
		"balance": map[string]interface{}{
			"reconciliation_interval": "5m",
			"strict_mode":             true,
			"max_retries":             3,
			"retry_backoff":           "100ms",
		},
		"settlement": map[string]interface{}{
			"batch_size":                     100,
			"batch_timeout":                  "5s",
			"settlement_consensus_threshold": 50000.0,
			"enable_parallel_processing":     true,
			"max_concurrent_operations":      10,
		},
		"order_processing": map[string]interface{}{
			"large_order_threshold":              10000.0,
			"critical_order_threshold":           50000.0,
			"order_processing_timeout":           "30s",
			"consensus_timeout":                  "30s",
			"balance_check_timeout":              "10s",
			"require_consensus_for_large_orders": true,
			"enable_atomic_matching":             true,
		},
		"transaction": map[string]interface{}{
			"transfer_consensus_threshold": 10000.0,
			"max_retry_attempts":           3,
			"transaction_timeout":          "60s",
			"enable_xa_transactions":       true,
		},
	}

	sc.logger.Info("Default strong consistency configuration loaded",
		zap.Int("sections", len(sc.config)))
}
