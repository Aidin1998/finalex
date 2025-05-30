// Package transaction provides distributed transaction coordination and locking mechanisms
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DistributedLockManager manages distributed locks across services
type DistributedLockManager struct {
	db     *gorm.DB
	logger *zap.Logger
	locks  map[string]*DistributedLock
	mu     sync.RWMutex

	// Configuration
	defaultTimeout  time.Duration
	heartbeatTicker *time.Ticker
	cleanupTicker   *time.Ticker
	stopChan        chan struct{}
}

// DistributedLock represents a distributed lock
type DistributedLock struct {
	ID         string `gorm:"primaryKey"`
	Resource   string `gorm:"index"`
	Owner      string
	AcquiredAt time.Time
	ExpiresAt  time.Time
	Heartbeat  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewDistributedLockManager creates a new distributed lock manager
func NewDistributedLockManager(db *gorm.DB, logger *zap.Logger) *DistributedLockManager {
	dlm := &DistributedLockManager{
		db:              db,
		logger:          logger,
		locks:           make(map[string]*DistributedLock),
		defaultTimeout:  30 * time.Second,
		heartbeatTicker: time.NewTicker(10 * time.Second),
		cleanupTicker:   time.NewTicker(60 * time.Second),
		stopChan:        make(chan struct{}),
	}

	// Ensure the locks table exists
	db.AutoMigrate(&DistributedLock{})

	// Start background processes
	go dlm.heartbeatProcess()
	go dlm.cleanupProcess()

	return dlm
}

// AcquireLock attempts to acquire a distributed lock
func (dlm *DistributedLockManager) AcquireLock(ctx context.Context, resource, owner string, timeout time.Duration) (*DistributedLock, error) {
	if timeout == 0 {
		timeout = dlm.defaultTimeout
	}

	lockID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(timeout)

	lock := &DistributedLock{
		ID:         lockID,
		Resource:   resource,
		Owner:      owner,
		AcquiredAt: now,
		ExpiresAt:  expiresAt,
		Heartbeat:  now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Try to acquire lock in database
	err := dlm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if lock already exists and is not expired
		var existingLock DistributedLock
		err := tx.Where("resource = ? AND expires_at > ?", resource, now).First(&existingLock).Error

		if err == nil {
			// Lock exists and is not expired
			return fmt.Errorf("resource already locked by %s", existingLock.Owner)
		}

		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to check existing lock: %w", err)
		}

		// Clean up any expired locks for this resource
		if err := tx.Where("resource = ? AND expires_at <= ?", resource, now).Delete(&DistributedLock{}).Error; err != nil {
			return fmt.Errorf("failed to clean up expired locks: %w", err)
		}

		// Create new lock
		if err := tx.Create(lock).Error; err != nil {
			return fmt.Errorf("failed to create lock: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Store lock locally for heartbeat
	dlm.mu.Lock()
	dlm.locks[lockID] = lock
	dlm.mu.Unlock()

	dlm.logger.Info("Acquired distributed lock",
		zap.String("lock_id", lockID),
		zap.String("resource", resource),
		zap.String("owner", owner),
		zap.Duration("timeout", timeout))

	return lock, nil
}

// ReleaseLock releases a distributed lock
func (dlm *DistributedLockManager) ReleaseLock(ctx context.Context, lockID string) error {
	dlm.mu.Lock()
	lock, exists := dlm.locks[lockID]
	if exists {
		delete(dlm.locks, lockID)
	}
	dlm.mu.Unlock()

	if !exists {
		return fmt.Errorf("lock not found: %s", lockID)
	}

	// Remove lock from database
	err := dlm.db.WithContext(ctx).Where("id = ?", lockID).Delete(&DistributedLock{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	dlm.logger.Info("Released distributed lock",
		zap.String("lock_id", lockID),
		zap.String("resource", lock.Resource),
		zap.String("owner", lock.Owner))

	return nil
}

// ExtendLock extends the expiration time of a lock
func (dlm *DistributedLockManager) ExtendLock(ctx context.Context, lockID string, extension time.Duration) error {
	dlm.mu.Lock()
	lock, exists := dlm.locks[lockID]
	dlm.mu.Unlock()

	if !exists {
		return fmt.Errorf("lock not found: %s", lockID)
	}

	now := time.Now()
	newExpiresAt := lock.ExpiresAt.Add(extension)

	// Update lock in database
	err := dlm.db.WithContext(ctx).Model(&DistributedLock{}).
		Where("id = ?", lockID).
		Updates(map[string]interface{}{
			"expires_at": newExpiresAt,
			"heartbeat":  now,
			"updated_at": now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to extend lock: %w", err)
	}

	// Update local lock
	dlm.mu.Lock()
	if lock, exists := dlm.locks[lockID]; exists {
		lock.ExpiresAt = newExpiresAt
		lock.Heartbeat = now
		lock.UpdatedAt = now
	}
	dlm.mu.Unlock()

	dlm.logger.Debug("Extended distributed lock",
		zap.String("lock_id", lockID),
		zap.Duration("extension", extension))

	return nil
}

// GetActiveLocks returns a slice of currently held distributed locks
func (dlm *DistributedLockManager) GetActiveLocks() []*DistributedLock {
	dlm.mu.RLock()
	defer dlm.mu.RUnlock()
	locks := make([]*DistributedLock, 0, len(dlm.locks))
	for _, lock := range dlm.locks {
		locks = append(locks, lock)
	}
	return locks
}

// heartbeatProcess sends periodic heartbeats for held locks
func (dlm *DistributedLockManager) heartbeatProcess() {
	for {
		select {
		case <-dlm.heartbeatTicker.C:
			dlm.sendHeartbeats()
		case <-dlm.stopChan:
			return
		}
	}
}

// cleanupProcess cleans up expired locks
func (dlm *DistributedLockManager) cleanupProcess() {
	for {
		select {
		case <-dlm.cleanupTicker.C:
			dlm.cleanupExpiredLocks()
		case <-dlm.stopChan:
			return
		}
	}
}

// sendHeartbeats updates heartbeat timestamps for all held locks
func (dlm *DistributedLockManager) sendHeartbeats() {
	dlm.mu.RLock()
	lockIDs := make([]string, 0, len(dlm.locks))
	for id := range dlm.locks {
		lockIDs = append(lockIDs, id)
	}
	dlm.mu.RUnlock()

	if len(lockIDs) == 0 {
		return
	}

	now := time.Now()
	err := dlm.db.Model(&DistributedLock{}).
		Where("id IN ?", lockIDs).
		Updates(map[string]interface{}{
			"heartbeat":  now,
			"updated_at": now,
		}).Error

	if err != nil {
		dlm.logger.Error("Failed to send heartbeats", zap.Error(err))
	} else {
		dlm.logger.Debug("Sent heartbeats for locks", zap.Int("count", len(lockIDs)))
	}
}

// cleanupExpiredLocks removes expired locks from the database
func (dlm *DistributedLockManager) cleanupExpiredLocks() {
	now := time.Now()
	result := dlm.db.Where("expires_at <= ?", now).Delete(&DistributedLock{})

	if result.Error != nil {
		dlm.logger.Error("Failed to cleanup expired locks", zap.Error(result.Error))
	} else if result.RowsAffected > 0 {
		dlm.logger.Info("Cleaned up expired locks", zap.Int64("count", result.RowsAffected))
	}
}

// Stop stops the distributed lock manager
func (dlm *DistributedLockManager) Stop() {
	close(dlm.stopChan)
	dlm.heartbeatTicker.Stop()
	dlm.cleanupTicker.Stop()

	// Release all held locks
	dlm.mu.Lock()
	for lockID := range dlm.locks {
		dlm.db.Where("id = ?", lockID).Delete(&DistributedLock{})
	}
	dlm.locks = make(map[string]*DistributedLock)
	dlm.mu.Unlock()

	dlm.logger.Info("Distributed lock manager stopped")
}

// TransactionSaga implements the Saga pattern for complex distributed transactions
type TransactionSaga struct {
	ID          string
	Steps       []SagaStep
	CurrentStep int
	State       string // "running", "completed", "compensating", "failed"
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	mu          sync.RWMutex
	logger      *zap.Logger
}

// SagaStep represents a step in a saga transaction
type SagaStep struct {
	ID           string
	Name         string
	Action       func(ctx context.Context) error
	Compensation func(ctx context.Context) error
	Executed     bool
	Compensated  bool
	Error        error
}

// NewTransactionSaga creates a new saga transaction
func NewTransactionSaga(id string, logger *zap.Logger) *TransactionSaga {
	return &TransactionSaga{
		ID:        id,
		Steps:     make([]SagaStep, 0),
		State:     "running",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		logger:    logger,
	}
}

// AddStep adds a step to the saga
func (s *TransactionSaga) AddStep(name string, action, compensation func(ctx context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	step := SagaStep{
		ID:           uuid.New().String(),
		Name:         name,
		Action:       action,
		Compensation: compensation,
	}

	s.Steps = append(s.Steps, step)
}

// Execute executes the saga transaction
func (s *TransactionSaga) Execute(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Starting saga execution",
		zap.String("saga_id", s.ID),
		zap.Int("step_count", len(s.Steps)))

	for i, step := range s.Steps {
		s.CurrentStep = i
		s.UpdatedAt = time.Now()

		s.logger.Info("Executing saga step",
			zap.String("saga_id", s.ID),
			zap.Int("step", i),
			zap.String("step_name", step.Name))

		err := step.Action(ctx)
		if err != nil {
			s.logger.Error("Saga step failed",
				zap.String("saga_id", s.ID),
				zap.Int("step", i),
				zap.String("step_name", step.Name),
				zap.Error(err))

			s.Steps[i].Error = err
			s.State = "compensating"

			// Execute compensation for all executed steps in reverse order
			return s.compensate(ctx, i)
		}

		s.Steps[i].Executed = true
	}

	s.State = "completed"
	now := time.Now()
	s.CompletedAt = &now
	s.UpdatedAt = now

	s.logger.Info("Saga execution completed",
		zap.String("saga_id", s.ID))

	return nil
}

// compensate executes compensation actions for executed steps
func (s *TransactionSaga) compensate(ctx context.Context, failedStep int) error {
	s.logger.Info("Starting saga compensation",
		zap.String("saga_id", s.ID),
		zap.Int("failed_step", failedStep))

	var compensationErrors []error

	// Compensate executed steps in reverse order
	for i := failedStep - 1; i >= 0; i-- {
		step := s.Steps[i]
		if !step.Executed || step.Compensated {
			continue
		}

		s.logger.Info("Compensating saga step",
			zap.String("saga_id", s.ID),
			zap.Int("step", i),
			zap.String("step_name", step.Name))

		err := step.Compensation(ctx)
		if err != nil {
			s.logger.Error("Saga compensation failed",
				zap.String("saga_id", s.ID),
				zap.Int("step", i),
				zap.String("step_name", step.Name),
				zap.Error(err))

			compensationErrors = append(compensationErrors, err)
		} else {
			s.Steps[i].Compensated = true
		}
	}

	if len(compensationErrors) > 0 {
		s.State = "failed"
		return fmt.Errorf("saga compensation failed: %v", compensationErrors)
	}

	s.State = "compensated"
	now := time.Now()
	s.CompletedAt = &now
	s.UpdatedAt = now

	s.logger.Info("Saga compensation completed",
		zap.String("saga_id", s.ID))

	return fmt.Errorf("saga failed at step %d: %w", failedStep, s.Steps[failedStep].Error)
}

// GetState returns the current state of the saga
func (s *TransactionSaga) GetState() (string, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastError error
	if s.CurrentStep < len(s.Steps) && s.Steps[s.CurrentStep].Error != nil {
		lastError = s.Steps[s.CurrentStep].Error
	}

	return s.State, s.CurrentStep, lastError
}

// GetMetrics returns saga execution metrics
func (s *TransactionSaga) GetMetrics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	executed := 0
	compensated := 0
	for _, step := range s.Steps {
		if step.Executed {
			executed++
		}
		if step.Compensated {
			compensated++
		}
	}

	metrics := map[string]interface{}{
		"id":           s.ID,
		"state":        s.State,
		"total_steps":  len(s.Steps),
		"current_step": s.CurrentStep,
		"executed":     executed,
		"compensated":  compensated,
		"created_at":   s.CreatedAt,
		"updated_at":   s.UpdatedAt,
	}

	if s.CompletedAt != nil {
		metrics["completed_at"] = *s.CompletedAt
		metrics["duration"] = s.CompletedAt.Sub(s.CreatedAt)
	}

	return metrics
}
