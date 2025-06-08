// config.go: Dynamic config and reload for rate limiting
package ratelimit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ConfigManager manages all rate limit configs and live reload
// Supports per-user, per-IP, per-route, global, etc.
type ConfigManager struct {
	configs   map[string]*RateLimitConfig
	mu        sync.RWMutex
	watcher   *fsnotify.Watcher
	configDir string
	logger    *zap.Logger
	callbacks []ConfigChangeCallback
}

// ConfigChangeCallback is called when configuration changes
type ConfigChangeCallback func(key string, old, new *RateLimitConfig)

// NewConfigManager creates a new configuration manager
func NewConfigManager(configDir string, logger *zap.Logger) (*ConfigManager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cm := &ConfigManager{
		configs:   make(map[string]*RateLimitConfig),
		watcher:   watcher,
		configDir: configDir,
		logger:    logger,
		callbacks: make([]ConfigChangeCallback, 0),
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
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.callbacks = append(cm.callbacks, callback)
}

// watchConfigChanges watches for file system changes and reloads configuration
func (cm *ConfigManager) watchConfigChanges() {
	for {
		select {
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				cm.logger.Info("config file changed, reloading",
					zap.String("file", event.Name))
				if err := cm.LoadFromFile(event.Name); err != nil {
					cm.logger.Error("failed to reload config",
						zap.String("file", event.Name),
						zap.Error(err))
				}
			}
		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			cm.logger.Error("config watcher error", zap.Error(err))
		}
	}
}

// GetConfig returns the config for a given key (user, IP, route, etc)
func (cm *ConfigManager) GetConfig(key string) *RateLimitConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.configs[key]
}

// SetConfig sets or updates a config
func (cm *ConfigManager) SetConfig(key string, cfg *RateLimitConfig) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	old := cm.configs[key]
	cm.configs[key] = cfg

	// Notify callbacks
	for _, callback := range cm.callbacks {
		go callback(key, old, cfg)
	}

	if cm.logger != nil {
		cm.logger.Debug("rate limit config updated",
			zap.String("key", key),
			zap.String("type", cfg.Type),
			zap.Int("limit", cfg.Limit),
			zap.Duration("window", cfg.Window))
	}
}

// LoadFromFile loads configs from a JSON or YAML file
func (cm *ConfigManager) LoadFromFile(path string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

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
	oldConfigs := cm.configs
	cm.configs = configs

	// Notify callbacks of changes
	for key, newCfg := range configs {
		oldCfg := oldConfigs[key]
		for _, callback := range cm.callbacks {
			go callback(key, oldCfg, newCfg)
		}
	}

	if cm.logger != nil {
		cm.logger.Info("loaded rate limit configuration",
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
				if cm.logger != nil {
					cm.logger.Error("failed to load config file",
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
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cfg, ok := cm.configs[key]; ok {
		old := *cfg // Copy old config
		cfg.Enabled = false

		// Notify callbacks
		for _, callback := range cm.callbacks {
			go callback(key, &old, cfg)
		}

		if cm.logger != nil {
			cm.logger.Info("whitelisted rate limit key", zap.String("key", key))
		}
	}
}

// Admin override: blacklist a key (force rate limit block)
func (cm *ConfigManager) Blacklist(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cfg, ok := cm.configs[key]; ok {
		old := *cfg // Copy old config
		cfg.Enabled = true
		cfg.Limit = 0

		// Notify callbacks
		for _, callback := range cm.callbacks {
			go callback(key, &old, cfg)
		}

		if cm.logger != nil {
			cm.logger.Info("blacklisted rate limit key", zap.String("key", key))
		}
	}
}

// GetAllConfigs returns a copy of all configurations
func (cm *ConfigManager) GetAllConfigs() map[string]*RateLimitConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	configs := make(map[string]*RateLimitConfig)
	for key, cfg := range cm.configs {
		// Deep copy
		configCopy := *cfg
		configs[key] = &configCopy
	}
	return configs
}

// DeleteConfig removes a configuration
func (cm *ConfigManager) DeleteConfig(key string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if old, exists := cm.configs[key]; exists {
		delete(cm.configs, key)

		// Notify callbacks
		for _, callback := range cm.callbacks {
			go callback(key, old, nil)
		}

		if cm.logger != nil {
			cm.logger.Info("deleted rate limit config", zap.String("key", key))
		}
		return true
	}
	return false
}

// SaveToFile saves current configurations to a file
func (cm *ConfigManager) SaveToFile(path string) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ext := filepath.Ext(path)
	var data []byte
	var err error

	if ext == ".yaml" || ext == ".yml" {
		data, err = yaml.Marshal(cm.configs)
	} else {
		data, err = json.MarshalIndent(cm.configs, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal configs: %w", err)
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if cm.logger != nil {
		cm.logger.Info("saved rate limit configuration",
			zap.String("file", path),
			zap.Int("configs", len(cm.configs)))
	}

	return nil
}

// Close cleanly shuts down the configuration manager
func (cm *ConfigManager) Close() error {
	if cm.watcher != nil {
		return cm.watcher.Close()
	}
	return nil
}
