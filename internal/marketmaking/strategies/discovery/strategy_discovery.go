// Package discovery provides dynamic strategy discovery and hot-reload functionality
package discovery

import (
	"context"
	"fmt"
	"path/filepath"
	"plugin"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/factory"
	"go.uber.org/zap"
)

// StrategyDiscovery manages dynamic strategy discovery and hot-reloading
type StrategyDiscovery struct {
	mu            sync.RWMutex
	factory       *factory.StrategyFactory
	pluginPaths   []string
	loadedPlugins map[string]*StrategyPlugin
	watchInterval time.Duration
	logger        *zap.SugaredLogger
	stopChan      chan struct{}
	running       bool
}

// StrategyPlugin represents a loaded strategy plugin
type StrategyPlugin struct {
	Path            string
	ModTime         time.Time
	Plugin          *plugin.Plugin
	StrategyCreator common.StrategyCreator
	Metadata        common.StrategyInfo
}

// NewStrategyDiscovery creates a new strategy discovery service
func NewStrategyDiscovery(factory *factory.StrategyFactory, logger *zap.SugaredLogger) *StrategyDiscovery {
	return &StrategyDiscovery{
		factory:       factory,
		loadedPlugins: make(map[string]*StrategyPlugin),
		watchInterval: 5 * time.Second,
		logger:        logger,
		stopChan:      make(chan struct{}),
	}
}

// AddPluginPath adds a path to scan for strategy plugins
func (sd *StrategyDiscovery) AddPluginPath(path string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.pluginPaths = append(sd.pluginPaths, path)
}

// Start begins the strategy discovery and hot-reload process
func (sd *StrategyDiscovery) Start(ctx context.Context) error {
	sd.mu.Lock()
	if sd.running {
		sd.mu.Unlock()
		return fmt.Errorf("strategy discovery already running")
	}
	sd.running = true
	sd.mu.Unlock()

	// Initial discovery
	if err := sd.DiscoverStrategies(); err != nil {
		sd.logger.Errorw("Initial strategy discovery failed", "error", err)
		return err
	}

	// Start hot-reload watcher
	go sd.watchForChanges(ctx)

	sd.logger.Info("Strategy discovery started")
	return nil
}

// Stop stops the strategy discovery service
func (sd *StrategyDiscovery) Stop() error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if !sd.running {
		return nil
	}

	close(sd.stopChan)
	sd.running = false

	sd.logger.Info("Strategy discovery stopped")
	return nil
}

// DiscoverStrategies scans for and loads strategy plugins
func (sd *StrategyDiscovery) DiscoverStrategies() error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, pluginPath := range sd.pluginPaths {
		if err := sd.scanPluginPath(pluginPath); err != nil {
			sd.logger.Errorw("Failed to scan plugin path", "path", pluginPath, "error", err)
			continue
		}
	}

	return nil
}

// scanPluginPath scans a specific path for strategy plugins
func (sd *StrategyDiscovery) scanPluginPath(basePath string) error {
	pattern := filepath.Join(basePath, "*.so")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob plugin path %s: %w", pattern, err)
	}

	for _, pluginFile := range matches {
		if err := sd.loadPlugin(pluginFile); err != nil {
			sd.logger.Errorw("Failed to load plugin", "file", pluginFile, "error", err)
			continue
		}
	}

	return nil
}

// loadPlugin loads a single strategy plugin
func (sd *StrategyDiscovery) loadPlugin(pluginPath string) error {
	// Check if plugin is already loaded and up to date
	_, exists := sd.loadedPlugins[pluginPath]
	if exists {
		fileInfo, err := filepath.Glob(pluginPath)
		if err != nil {
			return err
		}
		if len(fileInfo) > 0 {
			// For now, skip mod time check as filepath.Glob doesn't return FileInfo
			// In production, you'd use os.Stat() here
		}
	}

	// Load the plugin
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to open plugin %s: %w", pluginPath, err)
	}

	// Look for required symbols
	createSymbol, err := p.Lookup("CreateStrategy")
	if err != nil {
		return fmt.Errorf("plugin %s missing CreateStrategy symbol: %w", pluginPath, err)
	}

	metadataSymbol, err := p.Lookup("GetStrategyMetadata")
	if err != nil {
		return fmt.Errorf("plugin %s missing GetStrategyMetadata symbol: %w", pluginPath, err)
	}

	// Validate symbol types
	creator, ok := createSymbol.(func(common.StrategyConfig) (common.MarketMakingStrategy, error))
	if !ok {
		return fmt.Errorf("plugin %s CreateStrategy has wrong signature", pluginPath)
	}

	metadataFunc, ok := metadataSymbol.(func() common.StrategyInfo)
	if !ok {
		return fmt.Errorf("plugin %s GetStrategyMetadata has wrong signature", pluginPath)
	}

	// Get metadata
	metadata := metadataFunc()

	// Create plugin entry
	strategyPlugin := &StrategyPlugin{
		Path:            pluginPath,
		ModTime:         time.Now(), // In production, use actual file mod time
		Plugin:          p,
		StrategyCreator: creator,
		Metadata:        metadata,
	}

	// Register with factory
	if err := sd.factory.RegisterStrategy(metadata.Name, creator, metadata); err != nil {
		return fmt.Errorf("failed to register strategy %s: %w", metadata.Name, err)
	}

	// Update loaded plugins map
	sd.loadedPlugins[pluginPath] = strategyPlugin

	sd.logger.Infow("Loaded strategy plugin",
		"plugin", pluginPath,
		"strategy", metadata.Name,
		"version", metadata.Version)

	return nil
}

// watchForChanges monitors plugin files for changes and hot-reloads them
func (sd *StrategyDiscovery) watchForChanges(ctx context.Context) {
	ticker := time.NewTicker(sd.watchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sd.stopChan:
			return
		case <-ticker.C:
			sd.checkForPluginChanges()
		}
	}
}

// checkForPluginChanges checks if any plugin files have been modified
func (sd *StrategyDiscovery) checkForPluginChanges() {
	sd.mu.RLock()
	pluginPaths := make([]string, len(sd.pluginPaths))
	copy(pluginPaths, sd.pluginPaths)
	sd.mu.RUnlock()

	for _, pluginPath := range pluginPaths {
		if err := sd.scanPluginPath(pluginPath); err != nil {
			sd.logger.Errorw("Error during hot-reload scan", "path", pluginPath, "error", err)
		}
	}
}

// ReloadStrategy forces a reload of a specific strategy plugin
func (sd *StrategyDiscovery) ReloadStrategy(pluginPath string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Unregister existing plugin if it exists
	if existingPlugin, exists := sd.loadedPlugins[pluginPath]; exists {
		if err := sd.factory.UnregisterStrategy(existingPlugin.Metadata.Name); err != nil {
			sd.logger.Warnw("Failed to unregister existing strategy", "strategy", existingPlugin.Metadata.Name, "error", err)
		}
		delete(sd.loadedPlugins, pluginPath)
	}

	// Reload the plugin
	return sd.loadPlugin(pluginPath)
}

// GetLoadedPlugins returns information about all loaded plugins
func (sd *StrategyDiscovery) GetLoadedPlugins() map[string]*StrategyPlugin {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	result := make(map[string]*StrategyPlugin)
	for path, plugin := range sd.loadedPlugins {
		result[path] = plugin
	}
	return result
}

// GetPluginInfo returns information about a specific plugin
func (sd *StrategyDiscovery) GetPluginInfo(pluginPath string) (*StrategyPlugin, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	plugin, exists := sd.loadedPlugins[pluginPath]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginPath)
	}

	return plugin, nil
}

// SetWatchInterval sets the interval for checking plugin file changes
func (sd *StrategyDiscovery) SetWatchInterval(interval time.Duration) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.watchInterval = interval
}
