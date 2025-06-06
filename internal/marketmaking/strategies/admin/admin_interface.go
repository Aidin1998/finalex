// Package admin provides admin interface integration for strategy management
package admin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/discovery"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/factory"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/hotreload"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// StrategyAdminInterface provides HTTP endpoints for strategy administration
type StrategyAdminInterface struct {
	factory    *factory.StrategyFactory
	discovery  *discovery.StrategyDiscovery
	hotReload  *hotreload.HotReloadManager
	logger     *zap.SugaredLogger
	httpServer *http.Server
}

// NewStrategyAdminInterface creates a new admin interface
func NewStrategyAdminInterface(
	factory *factory.StrategyFactory,
	discovery *discovery.StrategyDiscovery,
	hotReload *hotreload.HotReloadManager,
	logger *zap.SugaredLogger,
) *StrategyAdminInterface {
	return &StrategyAdminInterface{
		factory:   factory,
		discovery: discovery,
		hotReload: hotReload,
		logger:    logger,
	}
}

// StartServer starts the admin HTTP server
func (sai *StrategyAdminInterface) StartServer(port int) error {
	router := gin.New()
	router.Use(gin.Recovery())

	// Strategy management endpoints
	api := router.Group("/api/v1/strategies")
	{
		// Discovery endpoints
		api.GET("/available", sai.getAvailableStrategies)
		api.GET("/info/:name", sai.getStrategyInfo)
		api.POST("/discover", sai.discoverStrategies)
		api.POST("/reload-plugin", sai.reloadPlugin)

		// Strategy instance management
		api.GET("/instances", sai.getStrategyInstances)
		api.GET("/instances/:id", sai.getStrategyInstance)
		api.POST("/instances", sai.createStrategyInstance)
		api.PUT("/instances/:id", sai.updateStrategyInstance)
		api.DELETE("/instances/:id", sai.deleteStrategyInstance)

		// Hot-reload management
		api.POST("/instances/:id/hot-reload", sai.hotReloadStrategy)
		api.GET("/updates", sai.getPendingUpdates)
		api.POST("/instances/:id/rollback", sai.rollbackStrategy)

		// Monitoring endpoints
		api.GET("/health", sai.getSystemHealth)
		api.GET("/metrics", sai.getSystemMetrics)
		api.GET("/plugins", sai.getLoadedPlugins)
	}

	// Backtest endpoints
	backtest := router.Group("/api/v1/backtest")
	{
		backtest.POST("/run", sai.runBacktest)
		backtest.GET("/results/:id", sai.getBacktestResults)
		backtest.GET("/results", sai.listBacktestResults)
	}

	sai.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	sai.logger.Infow("Starting strategy admin server", "port", port)
	return sai.httpServer.ListenAndServe()
}

// StopServer stops the admin HTTP server
func (sai *StrategyAdminInterface) StopServer() error {
	if sai.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return sai.httpServer.Shutdown(ctx)
	}
	return nil
}

// getAvailableStrategies returns all available strategy types
func (sai *StrategyAdminInterface) getAvailableStrategies(c *gin.Context) {
	strategies := sai.factory.GetAvailableStrategies()
	allInfo := sai.factory.GetAllStrategyInfo()

	response := make([]map[string]interface{}, 0, len(strategies))
	for _, name := range strategies {
		if info, exists := allInfo[name]; exists {
			response = append(response, map[string]interface{}{
				"name":        name,
				"description": info.Description,
				"risk_level":  info.RiskLevel.String(),
				"complexity":  info.Complexity.String(),
				"version":     info.Version,
				"parameters":  info.Parameters,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": response,
		"count":      len(response),
	})
}

// getStrategyInfo returns detailed information about a specific strategy
func (sai *StrategyAdminInterface) getStrategyInfo(c *gin.Context) {
	strategyName := c.Param("name")

	info, err := sai.factory.GetStrategyInfo(strategyName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"strategy": map[string]interface{}{
			"name":        info.Name,
			"description": info.Description,
			"risk_level":  info.RiskLevel.String(),
			"complexity":  info.Complexity.String(),
			"version":     info.Version,
			"parameters":  info.Parameters,
			"author":      info.Author,
			"tags":        info.Tags,
			"created_at":  info.CreatedAt,
			"updated_at":  info.UpdatedAt,
		},
	})
}

// discoverStrategies triggers strategy discovery
func (sai *StrategyAdminInterface) discoverStrategies(c *gin.Context) {
	if err := sai.discovery.DiscoverStrategies(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy discovery completed"})
}

// reloadPlugin reloads a specific strategy plugin
func (sai *StrategyAdminInterface) reloadPlugin(c *gin.Context) {
	var request struct {
		PluginPath string `json:"plugin_path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := sai.discovery.ReloadStrategy(request.PluginPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Plugin reloaded successfully"})
}

// getStrategyInstances returns all strategy instances
func (sai *StrategyAdminInterface) getStrategyInstances(c *gin.Context) {
	instances := sai.hotReload.GetAllStrategies()

	response := make([]map[string]interface{}, 0, len(instances))
	for id, instance := range instances {
		response = append(response, map[string]interface{}{
			"id":            id,
			"type":          instance.Config.Type,
			"status":        string(instance.Status),
			"version":       instance.Version,
			"start_time":    instance.StartTime,
			"last_activity": instance.LastActivity,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"instances": response,
		"count":     len(response),
	})
}

// getStrategyInstance returns details of a specific strategy instance
func (sai *StrategyAdminInterface) getStrategyInstance(c *gin.Context) {
	instanceID := c.Param("id")

	instance, err := sai.hotReload.GetStrategyStatus(instanceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Get health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	health := instance.Strategy.HealthCheck(ctx)

	c.JSON(http.StatusOK, gin.H{
		"instance": map[string]interface{}{
			"id":            instance.ID,
			"type":          instance.Config.Type,
			"status":        string(instance.Status),
			"version":       instance.Version,
			"start_time":    instance.StartTime,
			"last_activity": instance.LastActivity,
			"config":        instance.Config,
			"health":        health,
		},
	})
}

// createStrategyInstance creates a new strategy instance
func (sai *StrategyAdminInterface) createStrategyInstance(c *gin.Context) {
	var request struct {
		Type   string                 `json:"type" binding:"required"`
		ID     string                 `json:"id" binding:"required"`
		Config map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create strategy config
	config := common.StrategyConfig{
		ID:         request.ID,
		Type:       request.Type,
		Parameters: request.Config,
		Enabled:    true,
	}

	// Create strategy instance
	strategy, err := sai.factory.CreateStrategy(request.Type, config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Initialize and start strategy
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := strategy.Initialize(ctx, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize strategy: %v", err)})
		return
	}

	if err := strategy.Start(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to start strategy: %v", err)})
		return
	}

	// Register with hot-reload manager
	if err := sai.hotReload.RegisterStrategy(request.ID, strategy, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Strategy instance created successfully",
		"id":      request.ID,
	})
}

// updateStrategyInstance updates a strategy instance configuration
func (sai *StrategyAdminInterface) updateStrategyInstance(c *gin.Context) {
	instanceID := c.Param("id")

	var request struct {
		Config map[string]interface{} `json:"config" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	instance, err := sai.hotReload.GetStrategyStatus(instanceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Create updated config
	newConfig := instance.Config
	newConfig.Parameters = request.Config
	newConfig.UpdatedAt = time.Now()

	// Update strategy configuration
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := instance.Strategy.UpdateConfig(ctx, newConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy configuration updated successfully"})
}

// deleteStrategyInstance removes a strategy instance
func (sai *StrategyAdminInterface) deleteStrategyInstance(c *gin.Context) {
	instanceID := c.Param("id")

	if err := sai.hotReload.UnregisterStrategy(instanceID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy instance deleted successfully"})
}

// hotReloadStrategy performs hot-reload of a strategy instance
func (sai *StrategyAdminInterface) hotReloadStrategy(c *gin.Context) {
	instanceID := c.Param("id")

	var request struct {
		Type   string                 `json:"type" binding:"required"`
		Config map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get current instance
	instance, err := sai.hotReload.GetStrategyStatus(instanceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Create new strategy config
	newConfig := instance.Config
	newConfig.Type = request.Type
	if request.Config != nil {
		newConfig.Parameters = request.Config
	}
	newConfig.UpdatedAt = time.Now()

	// Create new strategy instance
	newStrategy, err := sai.factory.CreateStrategy(request.Type, newConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Schedule hot-reload update
	if err := sai.hotReload.ScheduleUpdate(instanceID, newStrategy, newConfig, hotreload.UpdateTypeReload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Hot-reload scheduled successfully"})
}

// getPendingUpdates returns all pending strategy updates
func (sai *StrategyAdminInterface) getPendingUpdates(c *gin.Context) {
	updates := sai.hotReload.GetPendingUpdates()

	response := make([]map[string]interface{}, 0, len(updates))
	for id, update := range updates {
		response = append(response, map[string]interface{}{
			"instance_id":    id,
			"update_type":    string(update.UpdateType),
			"status":         string(update.Status),
			"scheduled_time": update.ScheduledTime,
			"attempts":       update.Attempts,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"updates": response,
		"count":   len(response),
	})
}

// rollbackStrategy performs rollback of a strategy instance
func (sai *StrategyAdminInterface) rollbackStrategy(c *gin.Context) {
	// This would need additional implementation in the hot-reload manager
	// to support manual rollbacks
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Manual rollback not yet implemented"})
}

// getSystemHealth returns overall system health
func (sai *StrategyAdminInterface) getSystemHealth(c *gin.Context) {
	instances := sai.hotReload.GetAllStrategies()
	plugins := sai.discovery.GetLoadedPlugins()

	healthyInstances := 0
	totalInstances := len(instances)

	for _, instance := range instances {
		if instance.Status == hotreload.StatusActive {
			healthyInstances++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"healthy":           healthyInstances == totalInstances,
		"total_instances":   totalInstances,
		"healthy_instances": healthyInstances,
		"loaded_plugins":    len(plugins),
		"timestamp":         time.Now(),
	})
}

// getSystemMetrics returns system-wide metrics
func (sai *StrategyAdminInterface) getSystemMetrics(c *gin.Context) {
	instances := sai.hotReload.GetAllStrategies()

	metrics := make(map[string]interface{})
	for id, instance := range instances {
		strategyMetrics := instance.Strategy.GetMetrics()
		metrics[id] = strategyMetrics
	}

	c.JSON(http.StatusOK, gin.H{
		"metrics":   metrics,
		"timestamp": time.Now(),
	})
}

// getLoadedPlugins returns information about loaded plugins
func (sai *StrategyAdminInterface) getLoadedPlugins(c *gin.Context) {
	plugins := sai.discovery.GetLoadedPlugins()

	response := make([]map[string]interface{}, 0, len(plugins))
	for path, plugin := range plugins {
		response = append(response, map[string]interface{}{
			"path":        path,
			"strategy":    plugin.Metadata.Name,
			"version":     plugin.Metadata.Version,
			"description": plugin.Metadata.Description,
			"mod_time":    plugin.ModTime,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"plugins": response,
		"count":   len(response),
	})
}

// runBacktest runs a strategy backtest
func (sai *StrategyAdminInterface) runBacktest(c *gin.Context) {
	// This would integrate with the backtest system
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Backtesting not yet implemented"})
}

// getBacktestResults returns backtest results
func (sai *StrategyAdminInterface) getBacktestResults(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Backtesting not yet implemented"})
}

// listBacktestResults lists all backtest results
func (sai *StrategyAdminInterface) listBacktestResults(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Backtesting not yet implemented"})
}
