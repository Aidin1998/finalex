// config.go: Dynamic config and reload for rate limiting
package ratelimit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

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
		return err
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	var configs map[string]*RateLimitConfig
	if yaml.Unmarshal(data, &configs) == nil {
		cm.configs = configs
		return nil
	}
	if json.Unmarshal(data, &configs) == nil {
		cm.configs = configs
		return nil
	}
	return err
}

// Admin override: whitelist a key (disable rate limit)
func (cm *ConfigManager) Whitelist(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cfg, ok := cm.configs[key]; ok {
		cfg.Enabled = false
	}
}

// Admin override: blacklist a key (force rate limit block)
func (cm *ConfigManager) Blacklist(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cfg, ok := cm.configs[key]; ok {
		cfg.Enabled = true
		cfg.Limit = 0
	}
}

// TODO: Add live reload (fsnotify) and admin API integration
