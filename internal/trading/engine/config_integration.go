// =============================
// Configuration Integration for Adaptive Engine
// =============================
// This file provides integration between existing engine configuration
// and the new adaptive engine configuration system.

package engine

import (
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/orderbook"
)

// ConfigBuilder helps build adaptive engine configurations
type ConfigBuilder struct {
	config *AdaptiveEngineConfig
}

// NewConfigBuilder creates a new configuration builder
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: DefaultAdaptiveEngineConfig(),
	}
}

// FromExistingConfig initializes the adaptive config from existing engine config
func (cb *ConfigBuilder) FromExistingConfig(existingConfig *Config) *ConfigBuilder {
	cb.config.Config = existingConfig
	return cb
}

// EnableAdaptiveOrderBooks enables adaptive order book functionality
func (cb *ConfigBuilder) EnableAdaptiveOrderBooks(enabled bool) *ConfigBuilder {
	cb.config.EnableAdaptiveOrderBooks = enabled
	return cb
}

// WithMigrationConfig sets the default migration configuration
func (cb *ConfigBuilder) WithMigrationConfig(config *orderbook.MigrationConfig) *ConfigBuilder {
	cb.config.DefaultMigrationConfig = config
	return cb
}

// WithPairSpecificConfig adds pair-specific migration configuration
func (cb *ConfigBuilder) WithPairSpecificConfig(pair string, config *orderbook.MigrationConfig) *ConfigBuilder {
	if cb.config.PairSpecificConfigs == nil {
		cb.config.PairSpecificConfigs = make(map[string]*orderbook.MigrationConfig)
	}
	cb.config.PairSpecificConfigs[pair] = config
	return cb
}

// WithMetricsCollection configures metrics collection
func (cb *ConfigBuilder) WithMetricsCollection(interval time.Duration) *ConfigBuilder {
	cb.config.MetricsCollectionInterval = interval
	return cb
}

// WithPerformanceReporting configures performance reporting
func (cb *ConfigBuilder) WithPerformanceReporting(interval time.Duration) *ConfigBuilder {
	cb.config.PerformanceReportInterval = interval
	return cb
}

// WithAutoMigration configures automatic migration
func (cb *ConfigBuilder) WithAutoMigration(enabled bool, thresholds *AutoMigrationThresholds) *ConfigBuilder {
	cb.config.AutoMigrationEnabled = enabled
	if thresholds != nil {
		cb.config.AutoMigrationThresholds = thresholds
	}
	return cb
}

// WithSafetySettings configures safety settings
func (cb *ConfigBuilder) WithSafetySettings(maxStep int32, cooldown time.Duration, fallbackOnErrors bool, errorThreshold float64) *ConfigBuilder {
	cb.config.MaxMigrationPercentageStep = maxStep
	cb.config.MigrationCooldownPeriod = cooldown
	cb.config.FallbackOnHighErrorRate = fallbackOnErrors
	cb.config.ErrorRateThreshold = errorThreshold
	return cb
}

// Build returns the final adaptive engine configuration
func (cb *ConfigBuilder) Build() *AdaptiveEngineConfig {
	return cb.config
}

// ConfigurationPresets provides pre-configured settings for different scenarios
type ConfigurationPresets struct{}

// ConservativeConfig returns a conservative configuration for gradual rollout
func (cp *ConfigurationPresets) ConservativeConfig() *AdaptiveEngineConfig {
	return NewConfigBuilder().
		EnableAdaptiveOrderBooks(true).
		WithMigrationConfig(&orderbook.MigrationConfig{
			EnableGradualMigration:     true,
			InitialMigrationPercentage: 5,                // Start with 5%
			MaxMigrationPercentage:     50,               // Cap at 50%
			StepSize:                   5,                // 5% increments
			StepInterval:               time.Minute * 10, // 10 minutes between steps
			CircuitBreakerConfig: &orderbook.CircuitBreakerConfig{
				FailureThreshold: 3,
				RecoveryTimeout:  time.Minute * 5,
				HalfOpenMaxCalls: 5,
			},
		}).
		WithMetricsCollection(time.Second*15).
		WithPerformanceReporting(time.Minute*10).
		WithAutoMigration(false, nil). // Disable auto-migration initially
		WithSafetySettings(5, time.Minute*5, true, 0.01).
		Build()
}

// AggressiveConfig returns an aggressive configuration for faster rollout
func (cp *ConfigurationPresets) AggressiveConfig() *AdaptiveEngineConfig {
	return NewConfigBuilder().
		EnableAdaptiveOrderBooks(true).
		WithMigrationConfig(&orderbook.MigrationConfig{
			EnableGradualMigration:     true,
			InitialMigrationPercentage: 20,              // Start with 20%
			MaxMigrationPercentage:     100,             // Full migration
			StepSize:                   20,              // 20% increments
			StepInterval:               time.Minute * 3, // 3 minutes between steps
			CircuitBreakerConfig: &orderbook.CircuitBreakerConfig{
				FailureThreshold: 5,
				RecoveryTimeout:  time.Minute * 2,
				HalfOpenMaxCalls: 10,
			},
		}).
		WithMetricsCollection(time.Second*5).
		WithPerformanceReporting(time.Minute*2).
		WithAutoMigration(true, &AutoMigrationThresholds{
			LatencyP95ThresholdMs:       30.0,
			ThroughputDegradationPct:    15.0,
			ErrorRateThreshold:          0.005, // 0.5%
			ContentionThreshold:         50,
			NewImplErrorRateThreshold:   0.02, // 2%
			ConsecutiveFailureThreshold: 3,
			PerformanceDegradationPct:   20.0,
		}).
		WithSafetySettings(20, time.Minute*2, true, 0.02).
		Build()
}

// ProductionConfig returns a production-ready configuration
func (cp *ConfigurationPresets) ProductionConfig() *AdaptiveEngineConfig {
	return NewConfigBuilder().
		EnableAdaptiveOrderBooks(true).
		WithMigrationConfig(&orderbook.MigrationConfig{
			EnableGradualMigration:     true,
			InitialMigrationPercentage: 10,              // Start with 10%
			MaxMigrationPercentage:     100,             // Full migration allowed
			StepSize:                   10,              // 10% increments
			StepInterval:               time.Minute * 5, // 5 minutes between steps
			CircuitBreakerConfig: &orderbook.CircuitBreakerConfig{
				FailureThreshold: 3,
				RecoveryTimeout:  time.Minute * 3,
				HalfOpenMaxCalls: 5,
			},
		}).
		WithMetricsCollection(time.Second*10).
		WithPerformanceReporting(time.Minute*5).
		WithAutoMigration(true, &AutoMigrationThresholds{
			LatencyP95ThresholdMs:       50.0,
			ThroughputDegradationPct:    25.0,
			ErrorRateThreshold:          0.01, // 1%
			ContentionThreshold:         100,
			NewImplErrorRateThreshold:   0.03, // 3%
			ConsecutiveFailureThreshold: 5,
			PerformanceDegradationPct:   30.0,
		}).
		WithSafetySettings(10, time.Minute*3, true, 0.02).
		Build()
}

// TestingConfig returns a configuration optimized for testing
func (cp *ConfigurationPresets) TestingConfig() *AdaptiveEngineConfig {
	return NewConfigBuilder().
		EnableAdaptiveOrderBooks(true).
		WithMigrationConfig(&orderbook.MigrationConfig{
			EnableGradualMigration:     true,
			InitialMigrationPercentage: 50,               // Start with 50% for faster testing
			MaxMigrationPercentage:     100,              // Full migration
			StepSize:                   25,               // 25% increments
			StepInterval:               time.Second * 30, // 30 seconds between steps
			CircuitBreakerConfig: &orderbook.CircuitBreakerConfig{
				FailureThreshold: 2,
				RecoveryTimeout:  time.Second * 30,
				HalfOpenMaxCalls: 3,
			},
		}).
		WithMetricsCollection(time.Second*2).
		WithPerformanceReporting(time.Second*30).
		WithAutoMigration(true, &AutoMigrationThresholds{
			LatencyP95ThresholdMs:       100.0,
			ThroughputDegradationPct:    50.0,
			ErrorRateThreshold:          0.05, // 5%
			ContentionThreshold:         200,
			NewImplErrorRateThreshold:   0.1, // 10%
			ConsecutiveFailureThreshold: 2,
			PerformanceDegradationPct:   40.0,
		}).
		WithSafetySettings(25, time.Second*30, true, 0.05).
		Build()
}

// HighVolumeConfig returns a configuration optimized for high-volume trading
func (cp *ConfigurationPresets) HighVolumeConfig() *AdaptiveEngineConfig {
	return NewConfigBuilder().
		EnableAdaptiveOrderBooks(true).
		WithMigrationConfig(&orderbook.MigrationConfig{
			EnableGradualMigration:     true,
			InitialMigrationPercentage: 5,                // Conservative start for high volume
			MaxMigrationPercentage:     75,               // Conservative max
			StepSize:                   5,                // Small steps
			StepInterval:               time.Minute * 15, // Longer intervals
			CircuitBreakerConfig: &orderbook.CircuitBreakerConfig{
				FailureThreshold: 2, // Lower threshold for high volume
				RecoveryTimeout:  time.Minute * 10,
				HalfOpenMaxCalls: 3,
			},
		}).
		WithMetricsCollection(time.Second*5).    // More frequent metrics
		WithPerformanceReporting(time.Minute*3). // More frequent reporting
		WithAutoMigration(true, &AutoMigrationThresholds{
			LatencyP95ThresholdMs:       20.0,  // Stricter latency requirements
			ThroughputDegradationPct:    10.0,  // Stricter throughput requirements
			ErrorRateThreshold:          0.005, // 0.5% error rate
			ContentionThreshold:         25,    // Lower contention threshold
			NewImplErrorRateThreshold:   0.01,  // 1%
			ConsecutiveFailureThreshold: 2,     // Lower failure threshold
			PerformanceDegradationPct:   15.0,
		}).
		WithSafetySettings(5, time.Minute*5, true, 0.005). // Conservative safety
		Build()
}

// ConfigValidation provides validation utilities for configurations
type ConfigValidation struct{}

// ValidateConfig validates an adaptive engine configuration
func (cv *ConfigValidation) ValidateConfig(config *AdaptiveEngineConfig) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	// Validate base configuration
	if config.Config == nil {
		return fmt.Errorf("base engine configuration cannot be nil")
	}

	// Validate migration configuration
	if config.EnableAdaptiveOrderBooks && config.DefaultMigrationConfig == nil {
		return fmt.Errorf("migration configuration required when adaptive order books are enabled")
	}

	// Validate intervals
	if config.MetricsCollectionInterval <= 0 {
		return fmt.Errorf("metrics collection interval must be positive")
	}

	if config.PerformanceReportInterval <= 0 {
		return fmt.Errorf("performance report interval must be positive")
	}

	// Validate auto-migration thresholds
	if config.AutoMigrationEnabled && config.AutoMigrationThresholds == nil {
		return fmt.Errorf("auto-migration thresholds required when auto-migration is enabled")
	}

	// Validate safety settings
	if config.MaxMigrationPercentageStep <= 0 || config.MaxMigrationPercentageStep > 100 {
		return fmt.Errorf("migration percentage step must be between 1 and 100")
	}

	if config.MigrationCooldownPeriod <= 0 {
		return fmt.Errorf("migration cooldown period must be positive")
	}

	if config.ErrorRateThreshold < 0 || config.ErrorRateThreshold > 1 {
		return fmt.Errorf("error rate threshold must be between 0 and 1")
	}

	return nil
}

// GetPresets returns all available configuration presets
func GetPresets() *ConfigurationPresets {
	return &ConfigurationPresets{}
}

// NewValidation returns a new configuration validation instance
func NewValidation() *ConfigValidation {
	return &ConfigValidation{}
}

// Helper functions for configuration management

// MergeConfigs merges pair-specific configurations with default configuration
func MergeConfigs(defaultConfig *orderbook.MigrationConfig, pairConfig *orderbook.MigrationConfig) *orderbook.MigrationConfig {
	if pairConfig == nil {
		return defaultConfig
	}

	merged := *defaultConfig // Copy default

	// Override with pair-specific settings if they are set
	if pairConfig.InitialMigrationPercentage > 0 {
		merged.InitialMigrationPercentage = pairConfig.InitialMigrationPercentage
	}

	if pairConfig.MaxMigrationPercentage > 0 {
		merged.MaxMigrationPercentage = pairConfig.MaxMigrationPercentage
	}

	if pairConfig.StepSize > 0 {
		merged.StepSize = pairConfig.StepSize
	}

	if pairConfig.StepInterval > 0 {
		merged.StepInterval = pairConfig.StepInterval
	}

	if pairConfig.CircuitBreakerConfig != nil {
		merged.CircuitBreakerConfig = pairConfig.CircuitBreakerConfig
	}

	return &merged
}

// GetEffectiveConfig returns the effective configuration for a specific pair
func GetEffectiveConfig(adaptiveConfig *AdaptiveEngineConfig, pair string) *orderbook.MigrationConfig {
	pairConfig := adaptiveConfig.PairSpecificConfigs[pair]
	return MergeConfigs(adaptiveConfig.DefaultMigrationConfig, pairConfig)
}
