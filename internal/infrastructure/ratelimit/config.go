// config.go: Dynamic config and reload for rate limiting
package ratelimit

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// ConfigManager manages all rate limit configs and live reload
// Supports per-user, per-IP, per-route, global, etc.
type ConfigManager struct {
	configs map[string]*RateLimitConfig
	mu      sync.RWMutex
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
	cm.configs[key] = cfg
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
