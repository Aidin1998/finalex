// Package hotreload provides zero-downtime strategy hot-reloading capabilities
package hotreload

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"go.uber.org/zap"
)

// HotReloadManager manages zero-downtime strategy updates
type HotReloadManager struct {
	mu                sync.RWMutex
	strategies        map[string]*StrategyInstance
	pendingUpdates    map[string]*StrategyUpdate
	logger            *zap.SugaredLogger
	gracePeriod       time.Duration
	maxUpdateAttempts int
}

// StrategyInstance represents a running strategy instance
type StrategyInstance struct {
	ID           string
	Strategy     common.MarketMakingStrategy
	Config       common.StrategyConfig
	Status       StrategyInstanceStatus
	StartTime    time.Time
	LastActivity time.Time
	Version      string
	mu           sync.RWMutex
}

// StrategyUpdate represents a pending strategy update
type StrategyUpdate struct {
	InstanceID    string
	NewStrategy   common.MarketMakingStrategy
	NewConfig     common.StrategyConfig
	OldStrategy   common.MarketMakingStrategy
	OldConfig     common.StrategyConfig
	UpdateType    UpdateType
	ScheduledTime time.Time
	Attempts      int
	Status        UpdateStatus
}

// StrategyInstanceStatus represents the status of a strategy instance
type StrategyInstanceStatus string

const (
	StatusActive      StrategyInstanceStatus = "active"
	StatusGracePeriod StrategyInstanceStatus = "grace_period"
	StatusDraining    StrategyInstanceStatus = "draining"
	StatusUpdating    StrategyInstanceStatus = "updating"
	StatusTerminating StrategyInstanceStatus = "terminating"
	StatusTerminated  StrategyInstanceStatus = "terminated"
	StatusError       StrategyInstanceStatus = "error"
)

// UpdateType represents the type of update being performed
type UpdateType string

const (
	UpdateTypeReload       UpdateType = "reload"
	UpdateTypeConfigChange UpdateType = "config_change"
	UpdateTypeUpgrade      UpdateType = "upgrade"
	UpdateTypeRollback     UpdateType = "rollback"
)

// UpdateStatus represents the status of an update operation
type UpdateStatus string

const (
	UpdateStatusPending    UpdateStatus = "pending"
	UpdateStatusInProgress UpdateStatus = "in_progress"
	UpdateStatusCompleted  UpdateStatus = "completed"
	UpdateStatusFailed     UpdateStatus = "failed"
	UpdateStatusRolledBack UpdateStatus = "rolled_back"
)

// NewHotReloadManager creates a new hot-reload manager
func NewHotReloadManager(logger *zap.SugaredLogger) *HotReloadManager {
	return &HotReloadManager{
		strategies:        make(map[string]*StrategyInstance),
		pendingUpdates:    make(map[string]*StrategyUpdate),
		logger:            logger,
		gracePeriod:       30 * time.Second,
		maxUpdateAttempts: 3,
	}
}

// RegisterStrategy registers a strategy instance for hot-reload management
func (hrm *HotReloadManager) RegisterStrategy(id string, strategy common.MarketMakingStrategy, config common.StrategyConfig) error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()

	if _, exists := hrm.strategies[id]; exists {
		return fmt.Errorf("strategy instance %s already registered", id)
	}

	instance := &StrategyInstance{
		ID:           id,
		Strategy:     strategy,
		Config:       config,
		Status:       StatusActive,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Version:      strategy.Version(),
	}

	hrm.strategies[id] = instance
	hrm.logger.Infow("Strategy registered for hot-reload", "id", id, "type", config.Type)

	return nil
}

// UnregisterStrategy removes a strategy instance from hot-reload management
func (hrm *HotReloadManager) UnregisterStrategy(id string) error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()

	instance, exists := hrm.strategies[id]
	if !exists {
		return fmt.Errorf("strategy instance %s not found", id)
	}

	// Stop the strategy gracefully
	ctx, cancel := context.WithTimeout(context.Background(), hrm.gracePeriod)
	defer cancel()

	if err := instance.Strategy.Stop(ctx); err != nil {
		hrm.logger.Warnw("Failed to stop strategy gracefully", "id", id, "error", err)
	}

	delete(hrm.strategies, id)
	hrm.logger.Infow("Strategy unregistered from hot-reload", "id", id)

	return nil
}

// ScheduleUpdate schedules a hot-reload update for a strategy
func (hrm *HotReloadManager) ScheduleUpdate(instanceID string, newStrategy common.MarketMakingStrategy, newConfig common.StrategyConfig, updateType UpdateType) error {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()

	instance, exists := hrm.strategies[instanceID]
	if !exists {
		return fmt.Errorf("strategy instance %s not found", instanceID)
	}

	// Check if there's already a pending update
	if _, pending := hrm.pendingUpdates[instanceID]; pending {
		return fmt.Errorf("update already pending for strategy instance %s", instanceID)
	}

	update := &StrategyUpdate{
		InstanceID:    instanceID,
		NewStrategy:   newStrategy,
		NewConfig:     newConfig,
		OldStrategy:   instance.Strategy,
		OldConfig:     instance.Config,
		UpdateType:    updateType,
		ScheduledTime: time.Now(),
		Attempts:      0,
		Status:        UpdateStatusPending,
	}

	hrm.pendingUpdates[instanceID] = update
	hrm.logger.Infow("Update scheduled", "id", instanceID, "type", updateType)

	// Execute the update asynchronously
	go hrm.executeUpdate(instanceID)

	return nil
}

// executeUpdate performs the actual hot-reload update
func (hrm *HotReloadManager) executeUpdate(instanceID string) {
	hrm.mu.Lock()
	update, exists := hrm.pendingUpdates[instanceID]
	if !exists {
		hrm.mu.Unlock()
		return
	}
	update.Status = UpdateStatusInProgress
	hrm.mu.Unlock()

	defer func() {
		hrm.mu.Lock()
		delete(hrm.pendingUpdates, instanceID)
		hrm.mu.Unlock()
	}()

	instance, exists := hrm.strategies[instanceID]
	if !exists {
		hrm.logger.Errorw("Strategy instance not found during update", "id", instanceID)
		update.Status = UpdateStatusFailed
		return
	}

	// Perform zero-downtime update
	if err := hrm.performZeroDowntimeUpdate(instance, update); err != nil {
		hrm.logger.Errorw("Hot-reload update failed", "id", instanceID, "error", err, "attempts", update.Attempts)

		update.Attempts++
		if update.Attempts < hrm.maxUpdateAttempts {
			// Retry after a delay
			time.Sleep(5 * time.Second)
			go hrm.executeUpdate(instanceID)
			return
		}

		// Max attempts reached, perform rollback
		update.Status = UpdateStatusFailed
		if rollbackErr := hrm.performRollback(instance, update); rollbackErr != nil {
			hrm.logger.Errorw("Rollback failed", "id", instanceID, "error", rollbackErr)
			instance.Status = StatusError
		} else {
			update.Status = UpdateStatusRolledBack
			instance.Status = StatusActive
		}
		return
	}

	update.Status = UpdateStatusCompleted
	hrm.logger.Infow("Hot-reload update completed successfully", "id", instanceID, "type", update.UpdateType)
}

// performZeroDowntimeUpdate performs the actual zero-downtime strategy update
func (hrm *HotReloadManager) performZeroDowntimeUpdate(instance *StrategyInstance, update *StrategyUpdate) error {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), hrm.gracePeriod)
	defer cancel()

	// Step 1: Put instance in grace period
	instance.Status = StatusGracePeriod
	hrm.logger.Infow("Entering grace period", "id", instance.ID)

	// Step 2: Wait for current operations to complete
	time.Sleep(2 * time.Second) // Allow current quotes to expire

	// Step 3: Initialize new strategy
	instance.Status = StatusUpdating
	if err := update.NewStrategy.Initialize(ctx, update.NewConfig); err != nil {
		return fmt.Errorf("failed to initialize new strategy: %w", err)
	}

	// Step 4: Start new strategy
	if err := update.NewStrategy.Start(ctx); err != nil {
		return fmt.Errorf("failed to start new strategy: %w", err)
	}

	// Step 5: Stop old strategy gracefully
	if err := update.OldStrategy.Stop(ctx); err != nil {
		hrm.logger.Warnw("Failed to stop old strategy gracefully", "id", instance.ID, "error", err)
		// Continue with update despite stop error
	}

	// Step 6: Update instance
	instance.Strategy = update.NewStrategy
	instance.Config = update.NewConfig
	instance.Version = update.NewStrategy.Version()
	instance.Status = StatusActive
	instance.LastActivity = time.Now()

	return nil
}

// performRollback rolls back a failed update
func (hrm *HotReloadManager) performRollback(instance *StrategyInstance, update *StrategyUpdate) error {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), hrm.gracePeriod)
	defer cancel()

	hrm.logger.Infow("Performing rollback", "id", instance.ID)

	// Stop the new strategy if it was started
	if update.NewStrategy != nil {
		if err := update.NewStrategy.Stop(ctx); err != nil {
			hrm.logger.Warnw("Failed to stop new strategy during rollback", "id", instance.ID, "error", err)
		}
	}

	// Restart the old strategy
	if err := update.OldStrategy.Start(ctx); err != nil {
		return fmt.Errorf("failed to restart old strategy during rollback: %w", err)
	}

	// Restore instance state
	instance.Strategy = update.OldStrategy
	instance.Config = update.OldConfig
	instance.Status = StatusActive

	return nil
}

// GetStrategyStatus returns the current status of a strategy instance
func (hrm *HotReloadManager) GetStrategyStatus(instanceID string) (*StrategyInstance, error) {
	hrm.mu.RLock()
	defer hrm.mu.RUnlock()

	instance, exists := hrm.strategies[instanceID]
	if !exists {
		return nil, fmt.Errorf("strategy instance %s not found", instanceID)
	}

	// Return a copy to avoid race conditions
	return &StrategyInstance{
		ID:           instance.ID,
		Strategy:     instance.Strategy,
		Config:       instance.Config,
		Status:       instance.Status,
		StartTime:    instance.StartTime,
		LastActivity: instance.LastActivity,
		Version:      instance.Version,
	}, nil
}

// GetAllStrategies returns status of all managed strategy instances
func (hrm *HotReloadManager) GetAllStrategies() map[string]*StrategyInstance {
	hrm.mu.RLock()
	defer hrm.mu.RUnlock()

	result := make(map[string]*StrategyInstance)
	for id, instance := range hrm.strategies {
		result[id] = &StrategyInstance{
			ID:           instance.ID,
			Strategy:     instance.Strategy,
			Config:       instance.Config,
			Status:       instance.Status,
			StartTime:    instance.StartTime,
			LastActivity: instance.LastActivity,
			Version:      instance.Version,
		}
	}

	return result
}

// GetPendingUpdates returns all pending updates
func (hrm *HotReloadManager) GetPendingUpdates() map[string]*StrategyUpdate {
	hrm.mu.RLock()
	defer hrm.mu.RUnlock()

	result := make(map[string]*StrategyUpdate)
	for id, update := range hrm.pendingUpdates {
		result[id] = &StrategyUpdate{
			InstanceID:    update.InstanceID,
			UpdateType:    update.UpdateType,
			ScheduledTime: update.ScheduledTime,
			Attempts:      update.Attempts,
			Status:        update.Status,
		}
	}

	return result
}

// UpdateActivity updates the last activity time for a strategy instance
func (hrm *HotReloadManager) UpdateActivity(instanceID string) {
	hrm.mu.RLock()
	instance, exists := hrm.strategies[instanceID]
	hrm.mu.RUnlock()

	if exists {
		instance.mu.Lock()
		instance.LastActivity = time.Now()
		instance.mu.Unlock()
	}
}

// SetGracePeriod sets the grace period for strategy updates
func (hrm *HotReloadManager) SetGracePeriod(period time.Duration) {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()
	hrm.gracePeriod = period
}

// SetMaxUpdateAttempts sets the maximum number of update attempts
func (hrm *HotReloadManager) SetMaxUpdateAttempts(attempts int) {
	hrm.mu.Lock()
	defer hrm.mu.Unlock()
	hrm.maxUpdateAttempts = attempts
}
