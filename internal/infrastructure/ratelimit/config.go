// config.go: Dynamic config and reload for rate limiting
package ratelimit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// NewConfigManager creates a new configuration manager
func NewConfigManager(configDir string, logger *zap.Logger) (*ConfigManager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cm := &ConfigManager{
		Configs:   make(map[string]*RateLimitConfig),
		Watcher:   watcher,
		ConfigDir: configDir,
		Logger:    logger,
		Callbacks: make([]ConfigChangeCallback, 0),
	}

	// Start watching for file changes
	if configDir != "" {
		go cm.watchConfigChanges()
		if err := watcher.Add(configDir); err != nil {
			logger.Warn("failed to watch config directory",
				zap.String("dir", configDir),
				zap.Error(err))
		}
	}

	return cm, nil
}

// AddConfigChangeCallback adds a callback for configuration changes
func (cm *ConfigManager) AddConfigChangeCallback(callback ConfigChangeCallback) {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()
	cm.Callbacks = append(cm.Callbacks, callback)
}

// watchConfigChanges watches for file system changes and reloads configuration
func (cm *ConfigManager) watchConfigChanges() {
	for {
		select {
		case event, ok := <-cm.Watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				cm.Logger.Info("config file changed, reloading",
					zap.String("file", event.Name))
				if err := cm.LoadFromFile(event.Name); err != nil {
					cm.Logger.Error("failed to reload config",
						zap.String("file", event.Name),
						zap.Error(err))
				}
			}
		case err, ok := <-cm.Watcher.Errors:
			if !ok {
				return
			}
			cm.Logger.Error("config watcher error", zap.Error(err))
		}
	}
}

// GetConfig returns the config for a given key (user, IP, route, etc)
func (cm *ConfigManager) GetConfig(key string) *RateLimitConfig {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()
	return cm.Configs[key]
}

// SetConfig sets or updates a config
func (cm *ConfigManager) SetConfig(key string, cfg *RateLimitConfig) {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	old := cm.Configs[key]
	cm.Configs[key] = cfg

	// Notify callbacks
	for _, callback := range cm.Callbacks {
		go callback(key, old, cfg)
	}

	if cm.Logger != nil {
		cm.Logger.Debug("rate limit config updated",
			zap.String("key", key),
			zap.String("type", cfg.Type),
			zap.Int("limit", cfg.Limit),
			zap.Duration("window", cfg.Window))
	}
}

// LoadFromFile loads configs from a JSON or YAML file
func (cm *ConfigManager) LoadFromFile(path string) error {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var configs map[string]*RateLimitConfig

	// Try YAML first, then JSON
	ext := filepath.Ext(path)
	if ext == ".yaml" || ext == ".yml" {
		if err := yaml.Unmarshal(data, &configs); err != nil {
			return fmt.Errorf("failed to parse YAML config: %w", err)
		}
	} else if ext == ".json" {
		if err := json.Unmarshal(data, &configs); err != nil {
			return fmt.Errorf("failed to parse JSON config: %w", err)
		}
	} else {
		// Try both formats
		if yaml.Unmarshal(data, &configs) != nil {
			if json.Unmarshal(data, &configs) != nil {
				return fmt.Errorf("config file format not supported")
			}
		}
	}

	// Validate configurations
	for key, cfg := range configs {
		if err := cm.validateConfig(cfg); err != nil {
			return fmt.Errorf("invalid config for key %s: %w", key, err)
		}
	}

	// Update configurations
	oldConfigs := cm.Configs
	cm.Configs = configs

	// Notify callbacks of changes
	for key, newCfg := range configs {
		oldCfg := oldConfigs[key]
		for _, callback := range cm.Callbacks {
			go callback(key, oldCfg, newCfg)
		}
	}

	if cm.Logger != nil {
		cm.Logger.Info("loaded rate limit configuration",
			zap.String("file", path),
			zap.Int("configs", len(configs)))
	}

	return nil
}

// validateConfig validates a rate limit configuration
func (cm *ConfigManager) validateConfig(cfg *RateLimitConfig) error {
	if cfg.Limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	if cfg.Window <= 0 {
		return fmt.Errorf("window must be positive")
	}
	if cfg.Type != LimiterTokenBucket && cfg.Type != LimiterSlidingWindow && cfg.Type != "leaky_bucket" {
		return fmt.Errorf("unsupported limiter type: %s", cfg.Type)
	}
	return nil
}

// LoadFromDirectory loads all config files from a directory
func (cm *ConfigManager) LoadFromDirectory(dirPath string) error {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read config directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := filepath.Ext(file.Name())
		if ext == ".yaml" || ext == ".yml" || ext == ".json" {
			filePath := filepath.Join(dirPath, file.Name())
			if err := cm.LoadFromFile(filePath); err != nil {
				if cm.Logger != nil {
					cm.Logger.Error("failed to load config file",
						zap.String("file", filePath),
						zap.Error(err))
				}
			}
		}
	}

	return nil
}

// Admin override: whitelist a key (disable rate limit)
func (cm *ConfigManager) Whitelist(key string) {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	if cfg, ok := cm.Configs[key]; ok {
		old := *cfg // Copy old config
		cfg.Enabled = false

		// Notify callbacks
		for _, callback := range cm.Callbacks {
			go callback(key, &old, cfg)
		}

		if cm.Logger != nil {
			cm.Logger.Info("whitelisted rate limit key", zap.String("key", key))
		}
	}
}

// Admin override: blacklist a key (force rate limit block)
func (cm *ConfigManager) Blacklist(key string) {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	if cfg, ok := cm.Configs[key]; ok {
		old := *cfg // Copy old config
		cfg.Enabled = true
		cfg.Limit = 0

		// Notify callbacks
		for _, callback := range cm.Callbacks {
			go callback(key, &old, cfg)
		}

		if cm.Logger != nil {
			cm.Logger.Info("blacklisted rate limit key", zap.String("key", key))
		}
	}
}

// GetAllConfigs returns a copy of all configurations
func (cm *ConfigManager) GetAllConfigs() map[string]*RateLimitConfig {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()

	configs := make(map[string]*RateLimitConfig)
	for key, cfg := range cm.Configs {
		// Deep copy
		configCopy := *cfg
		configs[key] = &configCopy
	}
	return configs
}

// ListConfigs returns a list of configuration keys and basic info
func (cm *ConfigManager) ListConfigs() map[string]interface{} {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()

	result := make(map[string]interface{})
	for key, cfg := range cm.Configs {
		result[key] = map[string]interface{}{
			"type":        cfg.Type,
			"limit":       cfg.Limit,
			"window":      cfg.Window.String(),
			"enabled":     cfg.Enabled,
			"description": cfg.Description,
		}
	}
	return result
}

// DeleteConfig removes a configuration
func (cm *ConfigManager) DeleteConfig(key string) bool {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	if old, exists := cm.Configs[key]; exists {
		delete(cm.Configs, key)

		// Notify callbacks
		for _, callback := range cm.Callbacks {
			go callback(key, old, nil)
		}

		if cm.Logger != nil {
			cm.Logger.Info("deleted rate limit config", zap.String("key", key))
		}
		return true
	}
	return false
}

// UpdateConfig updates a configuration using a ConfigUpdate
func (cm *ConfigManager) UpdateConfig(key string, update *ConfigUpdate) error {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	config, exists := cm.Configs[key]
	if !exists {
		return fmt.Errorf("configuration not found: %s", key)
	}

	old := *config // Make a copy for callbacks

	// Apply updates
	if update.Limit != nil {
		if *update.Limit <= 0 {
			return fmt.Errorf("limit must be positive")
		}
		config.Limit = *update.Limit
	}
	if update.Window != nil {
		if *update.Window <= 0 {
			return fmt.Errorf("window must be positive")
		}
		config.Window = *update.Window
	}
	if update.Enabled != nil {
		config.Enabled = *update.Enabled
	}
	if update.Burst != nil {
		config.Burst = *update.Burst
	}
	if update.Description != nil {
		config.Description = *update.Description
	}

	// Notify callbacks
	for _, callback := range cm.Callbacks {
		go callback(key, &old, config)
	}

	if cm.Logger != nil {
		cm.Logger.Debug("rate limit config updated via patch",
			zap.String("key", key),
			zap.String("type", config.Type),
			zap.Int("limit", config.Limit),
			zap.Duration("window", config.Window))
	}

	return nil
}

// SaveToFile saves current configurations to a file
func (cm *ConfigManager) SaveToFile(path string) error {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()

	ext := filepath.Ext(path)
	var data []byte
	var err error

	if ext == ".yaml" || ext == ".yml" {
		data, err = yaml.Marshal(cm.Configs)
	} else {
		data, err = json.MarshalIndent(cm.Configs, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal configs: %w", err)
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if cm.Logger != nil {
		cm.Logger.Info("saved rate limit configuration",
			zap.String("file", path),
			zap.Int("configs", len(cm.Configs)))
	}

	return nil
}

// Close cleanly shuts down the configuration manager
func (cm *ConfigManager) Close() error {
	if cm.Watcher != nil {
		return cm.Watcher.Close()
	}
	return nil
}
