package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ConfigManager manages dynamic validation configuration
type ConfigManager struct {
	logger           *zap.Logger
	configPath       string
	config           *DynamicValidationConfig
	mutex            sync.RWMutex
	watchers         []ConfigWatcher
	reloadInterval   time.Duration
	lastModified     time.Time
	hotReloadEnabled bool
	ctx              context.Context
	cancel           context.CancelFunc
}

// DynamicValidationConfig represents the complete validation configuration
type DynamicValidationConfig struct {
	Version          string                             `json:"version" yaml:"version"`
	LastUpdated      time.Time                          `json:"last_updated" yaml:"last_updated"`
	ValidationConfig *EnhancedValidationConfig          `json:"validation_config" yaml:"validation_config"`
	SecurityConfig   *SecurityConfig                    `json:"security_config" yaml:"security_config"`
	EndpointRules    map[string]*EndpointValidationRule `json:"endpoint_rules" yaml:"endpoint_rules"`
	GlobalRules      *GlobalValidationRules             `json:"global_rules" yaml:"global_rules"`
	CustomValidators map[string]*CustomValidator        `json:"custom_validators" yaml:"custom_validators"`
	RateLimitRules   map[string]*RateLimitRule          `json:"rate_limit_rules" yaml:"rate_limit_rules"`
	IPWhitelist      []string                           `json:"ip_whitelist" yaml:"ip_whitelist"`
	IPBlacklist      []string                           `json:"ip_blacklist" yaml:"ip_blacklist"`
	FeatureFlags     map[string]bool                    `json:"feature_flags" yaml:"feature_flags"`
}

// GlobalValidationRules defines global validation constraints
type GlobalValidationRules struct {
	MaxRequestSize         int64         `json:"max_request_size" yaml:"max_request_size"`
	MaxFieldLength         int           `json:"max_field_length" yaml:"max_field_length"`
	MaxArraySize           int           `json:"max_array_size" yaml:"max_array_size"`
	MaxNestingDepth        int           `json:"max_nesting_depth" yaml:"max_nesting_depth"`
	AllowedContentTypes    []string      `json:"allowed_content_types" yaml:"allowed_content_types"`
	ForbiddenPatterns      []string      `json:"forbidden_patterns" yaml:"forbidden_patterns"`
	RequiredHeaders        []string      `json:"required_headers" yaml:"required_headers"`
	SessionTimeout         time.Duration `json:"session_timeout" yaml:"session_timeout"`
	EnableStrictValidation bool          `json:"enable_strict_validation" yaml:"enable_strict_validation"`
}

// CustomValidator defines custom validation logic
type CustomValidator struct {
	Name         string            `json:"name" yaml:"name"`
	Type         string            `json:"type" yaml:"type"` // regex, function, lua_script
	Pattern      string            `json:"pattern" yaml:"pattern"`
	Script       string            `json:"script" yaml:"script"`
	Parameters   map[string]string `json:"parameters" yaml:"parameters"`
	ErrorMessage string            `json:"error_message" yaml:"error_message"`
	Enabled      bool              `json:"enabled" yaml:"enabled"`
}

// RateLimitRule defines rate limiting rules per endpoint or IP
type RateLimitRule struct {
	Name            string        `json:"name" yaml:"name"`
	Pattern         string        `json:"pattern" yaml:"pattern"`
	RequestsPerMin  int           `json:"requests_per_min" yaml:"requests_per_min"`
	RequestsPerHour int           `json:"requests_per_hour" yaml:"requests_per_hour"`
	RequestsPerDay  int           `json:"requests_per_day" yaml:"requests_per_day"`
	BurstSize       int           `json:"burst_size" yaml:"burst_size"`
	BlockDuration   time.Duration `json:"block_duration" yaml:"block_duration"`
	Enabled         bool          `json:"enabled" yaml:"enabled"`
}

// ConfigWatcher interface for components that need config updates
type ConfigWatcher interface {
	OnConfigUpdate(config *DynamicValidationConfig) error
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(logger *zap.Logger, configPath string) *ConfigManager {
	ctx, cancel := context.WithCancel(context.Background())

	cm := &ConfigManager{
		logger:           logger,
		configPath:       configPath,
		reloadInterval:   30 * time.Second,
		hotReloadEnabled: true,
		ctx:              ctx,
		cancel:           cancel,
		watchers:         make([]ConfigWatcher, 0),
	}

	// Load initial configuration
	if err := cm.loadConfig(); err != nil {
		logger.Error("Failed to load initial configuration", zap.Error(err))
		// Use default configuration
		cm.config = cm.getDefaultConfig()
	}

	// Start hot reload if enabled
	if cm.hotReloadEnabled {
		go cm.watchConfigFile()
	}

	return cm
}

// GetConfig returns the current configuration (thread-safe)
func (cm *ConfigManager) GetConfig() *DynamicValidationConfig {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Return a deep copy to prevent external modifications
	return cm.deepCopyConfig(cm.config)
}

// UpdateConfig updates the configuration and notifies watchers
func (cm *ConfigManager) UpdateConfig(config *DynamicValidationConfig) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Validate the new configuration
	if err := cm.validateConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Update timestamp and version
	config.LastUpdated = time.Now()
	if config.Version == "" {
		config.Version = fmt.Sprintf("v%d", time.Now().Unix())
	}

	// Store the new configuration
	oldConfig := cm.config
	cm.config = config

	// Save to file
	if err := cm.saveConfig(); err != nil {
		// Rollback on save failure
		cm.config = oldConfig
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Notify watchers
	cm.notifyWatchers(config)

	cm.logger.Info("Configuration updated successfully",
		zap.String("version", config.Version),
		zap.Time("timestamp", config.LastUpdated))

	return nil
}

// AddWatcher adds a configuration watcher
func (cm *ConfigManager) AddWatcher(watcher ConfigWatcher) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.watchers = append(cm.watchers, watcher)
}

// RemoveWatcher removes a configuration watcher
func (cm *ConfigManager) RemoveWatcher(watcher ConfigWatcher) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for i, w := range cm.watchers {
		if w == watcher {
			cm.watchers = append(cm.watchers[:i], cm.watchers[i+1:]...)
			break
		}
	}
}

// ReloadConfig manually reloads configuration from file
func (cm *ConfigManager) ReloadConfig() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.loadConfig()
}

// Close stops the configuration manager
func (cm *ConfigManager) Close() error {
	cm.cancel()
	return nil
}

// loadConfig loads configuration from file
func (cm *ConfigManager) loadConfig() error {
	if cm.configPath == "" {
		cm.config = cm.getDefaultConfig()
		return nil
	}

	// Check if file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		cm.logger.Warn("Configuration file not found, using defaults", zap.String("path", cm.configPath))
		cm.config = cm.getDefaultConfig()
		return cm.saveConfig() // Create default config file
	}

	// Read file
	data, err := ioutil.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse based on file extension
	var config DynamicValidationConfig
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
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := cm.validateConfig(&config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Compile regex patterns
	if err := cm.compilePatterns(&config); err != nil {
		return fmt.Errorf("failed to compile patterns: %w", err)
	}

	cm.config = &config
	cm.lastModified = time.Now()

	return nil
}

// saveConfig saves current configuration to file
func (cm *ConfigManager) saveConfig() error {
	if cm.configPath == "" {
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var data []byte
	var err error

	ext := filepath.Ext(cm.configPath)
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

	err = ioutil.WriteFile(cm.configPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// validateConfig validates configuration structure and values
func (cm *ConfigManager) validateConfig(config *DynamicValidationConfig) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	// Validate validation config
	if config.ValidationConfig != nil {
		if config.ValidationConfig.MaxRequestBodySize <= 0 {
			return fmt.Errorf("max_request_body_size must be positive")
		}
		if config.ValidationConfig.MaxQueryParamCount <= 0 {
			return fmt.Errorf("max_query_param_count must be positive")
		}
	}

	// Validate endpoint rules
	for path, rule := range config.EndpointRules {
		if rule.Method == "" {
			return fmt.Errorf("endpoint rule for %s missing method", path)
		}
		if rule.Path == "" {
			return fmt.Errorf("endpoint rule for %s missing path", path)
		}

		// Validate parameter rules
		for paramName, paramRule := range rule.AllowedParams {
			if err := cm.validateParamRule(paramName, paramRule); err != nil {
				return fmt.Errorf("invalid param rule %s.%s: %w", path, paramName, err)
			}
		}
	}

	// Validate custom validators
	for name, validator := range config.CustomValidators {
		if validator.Type == "regex" && validator.Pattern == "" {
			return fmt.Errorf("regex validator %s missing pattern", name)
		}
		if validator.Type == "lua_script" && validator.Script == "" {
			return fmt.Errorf("lua validator %s missing script", name)
		}
	}

	// Validate rate limit rules
	for name, rule := range config.RateLimitRules {
		if rule.RequestsPerMin <= 0 && rule.RequestsPerHour <= 0 && rule.RequestsPerDay <= 0 {
			return fmt.Errorf("rate limit rule %s must have at least one positive limit", name)
		}
	}

	return nil
}

// validateParamRule validates a parameter rule
func (cm *ConfigManager) validateParamRule(name string, rule ParamRule) error {
	// Validate type
	validTypes := []string{"string", "int", "float", "bool", "uuid", "decimal"}
	isValidType := false
	for _, validType := range validTypes {
		if rule.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return fmt.Errorf("invalid type %s", rule.Type)
	}

	// Validate length constraints
	if rule.MinLength != nil && *rule.MinLength < 0 {
		return fmt.Errorf("min_length cannot be negative")
	}
	if rule.MaxLength != nil && *rule.MaxLength < 0 {
		return fmt.Errorf("max_length cannot be negative")
	}
	if rule.MinLength != nil && rule.MaxLength != nil && *rule.MinLength > *rule.MaxLength {
		return fmt.Errorf("min_length cannot be greater than max_length")
	}
	// Validate value constraints
	if rule.MinValue != nil && rule.MaxValue != nil && rule.MinValue.GreaterThan(*rule.MaxValue) {
		return fmt.Errorf("min_value cannot be greater than max_value")
	}

	// Validate pattern
	if rule.Pattern != "" {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}
	}

	return nil
}

// compilePatterns compiles regex patterns in the configuration
func (cm *ConfigManager) compilePatterns(config *DynamicValidationConfig) error {
	// Compile endpoint rule patterns
	for _, rule := range config.EndpointRules {
		if rule.PathPattern == nil && rule.Path != "" {
			// Convert path with parameters to regex
			pattern := rule.Path
			pattern = regexp.QuoteMeta(pattern)
			pattern = fmt.Sprintf("^%s/?$", pattern)
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("failed to compile path pattern for %s: %w", rule.Path, err)
			}
			rule.PathPattern = compiled
		}

		// Compile parameter patterns
		for field, paramRule := range rule.AllowedParams {
			if paramRule.Pattern != "" && paramRule.PatternRegex == nil {
				compiled, err := regexp.Compile(paramRule.Pattern)
				if err != nil {
					return fmt.Errorf("failed to compile pattern for %s.%s: %w", rule.Path, field, err)
				}
				paramRule.PatternRegex = compiled
				rule.AllowedParams[field] = paramRule
			}
		}
	}

	return nil
}

// watchConfigFile watches for configuration file changes
func (cm *ConfigManager) watchConfigFile() {
	ticker := time.NewTicker(cm.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			if cm.configPath == "" {
				continue
			}

			stat, err := os.Stat(cm.configPath)
			if err != nil {
				cm.logger.Error("Failed to stat config file", zap.Error(err))
				continue
			}

			if stat.ModTime().After(cm.lastModified) {
				cm.logger.Info("Config file changed, reloading...")
				if err := cm.ReloadConfig(); err != nil {
					cm.logger.Error("Failed to reload config", zap.Error(err))
				} else {
					cm.logger.Info("Config reloaded successfully")
					cm.notifyWatchers(cm.config)
				}
			}
		}
	}
}

// notifyWatchers notifies all registered watchers of config changes
func (cm *ConfigManager) notifyWatchers(config *DynamicValidationConfig) {
	for _, watcher := range cm.watchers {
		if err := watcher.OnConfigUpdate(config); err != nil {
			cm.logger.Error("Config watcher failed to update", zap.Error(err))
		}
	}
}

// getDefaultConfig returns the default configuration
func (cm *ConfigManager) getDefaultConfig() *DynamicValidationConfig {
	return &DynamicValidationConfig{
		Version:          "v1.0.0",
		LastUpdated:      time.Now(),
		ValidationConfig: DefaultEnhancedValidationConfig(),
		SecurityConfig:   DefaultSecurityConfig(),
		EndpointRules:    make(map[string]*EndpointValidationRule),
		GlobalRules: &GlobalValidationRules{
			MaxRequestSize:         10 * 1024 * 1024, // 10MB
			MaxFieldLength:         10000,
			MaxArraySize:           1000,
			MaxNestingDepth:        10,
			AllowedContentTypes:    []string{"application/json", "application/x-www-form-urlencoded"},
			ForbiddenPatterns:      []string{},
			RequiredHeaders:        []string{"Authorization"},
			SessionTimeout:         30 * time.Minute,
			EnableStrictValidation: true,
		},
		CustomValidators: make(map[string]*CustomValidator),
		RateLimitRules:   make(map[string]*RateLimitRule),
		IPWhitelist:      []string{},
		IPBlacklist:      []string{},
		FeatureFlags: map[string]bool{
			"enable_advanced_validation": true,
			"enable_security_hardening":  true,
			"enable_rate_limiting":       true,
			"enable_custom_validators":   true,
		},
	}
}

// deepCopyConfig creates a deep copy of the configuration
func (cm *ConfigManager) deepCopyConfig(config *DynamicValidationConfig) *DynamicValidationConfig {
	// Simple implementation using JSON marshaling
	// In production, consider using a more efficient deep copy library
	data, _ := json.Marshal(config)
	var copy DynamicValidationConfig
	json.Unmarshal(data, &copy)
	return &copy
}

// GetEndpointRule returns validation rule for a specific endpoint
func (cm *ConfigManager) GetEndpointRule(method, path string) *EndpointValidationRule {
	config := cm.GetConfig()
	key := fmt.Sprintf("%s:%s", method, path)

	if rule, exists := config.EndpointRules[key]; exists {
		return rule
	}

	// Try pattern matching
	for _, rule := range config.EndpointRules {
		if rule.Method == method && rule.PathPattern != nil && rule.PathPattern.MatchString(path) {
			return rule
		}
	}

	return nil
}

// IsFeatureEnabled checks if a feature flag is enabled
func (cm *ConfigManager) IsFeatureEnabled(feature string) bool {
	config := cm.GetConfig()
	if enabled, exists := config.FeatureFlags[feature]; exists {
		return enabled
	}
	return false
}

// GetRateLimitRule returns rate limit rule for a pattern
func (cm *ConfigManager) GetRateLimitRule(path string) *RateLimitRule {
	config := cm.GetConfig()

	for _, rule := range config.RateLimitRules {
		if rule.Pattern == "" || !rule.Enabled {
			continue
		}

		if matched, _ := regexp.MatchString(rule.Pattern, path); matched {
			return rule
		}
	}

	return nil
}

// GetCustomValidator returns a custom validator by name
func (cm *ConfigManager) GetCustomValidator(name string) *CustomValidator {
	config := cm.GetConfig()

	if validator, exists := config.CustomValidators[name]; exists && validator.Enabled {
		return validator
	}

	return nil
}
