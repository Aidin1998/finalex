package consistency

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/consensus"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DistributedLockManager provides distributed locking capabilities for critical operations
type DistributedLockManager struct {
	db              *gorm.DB
	logger          *zap.Logger
	raftCoordinator *consensus.RaftCoordinator

	// Local state
	localLocks      map[string]*LocalLock
	localLocksMutex sync.RWMutex

	// Configuration
	config *DistributedLockConfig

	// Metrics
	metrics      *DistributedLockMetrics
	metricsMutex sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DistributedLockConfig configures distributed locking behavior
type DistributedLockConfig struct {
	// Lock timeouts
	DefaultLockTimeout time.Duration `json:"default_lock_timeout" yaml:"default_lock_timeout"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	CleanupInterval    time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`

	// Consensus configuration
	ConsensusTimeout time.Duration `json:"consensus_timeout" yaml:"consensus_timeout"`
	RequireConsensus bool          `json:"require_consensus" yaml:"require_consensus"`

	// Performance tuning
	MaxConcurrentLocks int  `json:"max_concurrent_locks" yaml:"max_concurrent_locks"`
	EnableLocalCaching bool `json:"enable_local_caching" yaml:"enable_local_caching"`

	// Deadlock prevention
	EnableDeadlockDetection bool          `json:"enable_deadlock_detection" yaml:"enable_deadlock_detection"`
	DeadlockTimeout         time.Duration `json:"deadlock_timeout" yaml:"deadlock_timeout"`
}

// DistributedLockMetrics tracks locking performance and health
type DistributedLockMetrics struct {
	LocksAcquired         int64 `json:"locks_acquired"`
	LocksReleased         int64 `json:"locks_released"`
	LockAcquisitionFailed int64 `json:"lock_acquisition_failed"`
	LockTimeouts          int64 `json:"lock_timeouts"`
	DeadlocksDetected     int64 `json:"deadlocks_detected"`

	AverageLockDuration time.Duration `json:"average_lock_duration"`
	AverageWaitTime     time.Duration `json:"average_wait_time"`

	ActiveLocks  int64 `json:"active_locks"`
	PendingLocks int64 `json:"pending_locks"`

	LastUpdate time.Time `json:"last_update"`
}

// DistributedLock represents a distributed lock
type DistributedLock struct {
	ID         string `json:"id" db:"id"`
	ResourceID string `json:"resource_id" db:"resource_id"`
	NodeID     string `json:"node_id" db:"node_id"`
	HolderID   string `json:"holder_id" db:"holder_id"`
	LockType   string `json:"lock_type" db:"lock_type"` // "read", "write", "exclusive"

	AcquiredAt    time.Time `json:"acquired_at" db:"acquired_at"`
	ExpiresAt     time.Time `json:"expires_at" db:"expires_at"`
	LastHeartbeat time.Time `json:"last_heartbeat" db:"last_heartbeat"`

	Status   string `json:"status" db:"status"` // "pending", "acquired", "released", "expired"
	Priority int    `json:"priority" db:"priority"`

	Metadata map[string]interface{} `json:"metadata" db:"metadata"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// LocalLock represents a local lock state
type LocalLock struct {
	DistributedLock *DistributedLock
	Context         context.Context
	Cancel          context.CancelFunc
	Heartbeat       *time.Ticker
	LastActivity    time.Time
}

// LockRequest represents a lock acquisition request
type LockRequest struct {
	ResourceID string                 `json:"resource_id"`
	LockType   string                 `json:"lock_type"`
	HolderID   string                 `json:"holder_id"`
	Timeout    time.Duration          `json:"timeout"`
	Priority   int                    `json:"priority"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// LockResult represents the result of a lock operation
type LockResult struct {
	LockID   string           `json:"lock_id"`
	Success  bool             `json:"success"`
	WaitTime time.Duration    `json:"wait_time"`
	Error    string           `json:"error,omitempty"`
	Lock     *DistributedLock `json:"lock,omitempty"`
}

// NewDistributedLockManager creates a new distributed lock manager
func NewDistributedLockManager(
	db *gorm.DB,
	logger *zap.Logger,
	raftCoordinator *consensus.RaftCoordinator,
	config *DistributedLockConfig,
) *DistributedLockManager {
	if config == nil {
		config = DefaultDistributedLockConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DistributedLockManager{
		db:              db,
		logger:          logger,
		raftCoordinator: raftCoordinator,
		localLocks:      make(map[string]*LocalLock),
		config:          config,
		metrics:         &DistributedLockMetrics{},
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start initializes the distributed lock manager
func (dlm *DistributedLockManager) Start(ctx context.Context) error {
	dlm.logger.Info("Starting Distributed Lock Manager")

	// Create locks table if it doesn't exist
	if err := dlm.createLocksTable(); err != nil {
		return fmt.Errorf("failed to create locks table: %w", err)
	}

	// Start background workers
	dlm.wg.Add(3)
	go dlm.heartbeatWorker()
	go dlm.cleanupWorker()
	go dlm.deadlockDetectionWorker()

	dlm.logger.Info("Distributed Lock Manager started successfully")
	return nil
}

// Stop gracefully shuts down the lock manager
func (dlm *DistributedLockManager) Stop(ctx context.Context) error {
	dlm.logger.Info("Stopping Distributed Lock Manager")

	// Cancel all operations
	dlm.cancel()

	// Release all local locks
	dlm.releaseAllLocalLocks()

	// Wait for workers to finish
	dlm.wg.Wait()

	dlm.logger.Info("Distributed Lock Manager stopped")
	return nil
}

// AcquireLock attempts to acquire a distributed lock
func (dlm *DistributedLockManager) AcquireLock(ctx context.Context, request LockRequest) (*LockResult, error) {
	startTime := time.Now()
	lockID := uuid.New().String()

	dlm.logger.Info("Attempting to acquire distributed lock",
		zap.String("lock_id", lockID),
		zap.String("resource_id", request.ResourceID),
		zap.String("lock_type", request.LockType),
		zap.String("holder_id", request.HolderID))

	// Create lock record
	lock := &DistributedLock{
		ID:            lockID,
		ResourceID:    request.ResourceID,
		NodeID:        dlm.getNodeID(),
		HolderID:      request.HolderID,
		LockType:      request.LockType,
		AcquiredAt:    time.Now(),
		ExpiresAt:     time.Now().Add(request.Timeout),
		LastHeartbeat: time.Now(),
		Status:        "pending",
		Priority:      request.Priority,
		Metadata:      request.Metadata,
	}

	// Check for deadlock potential
	if dlm.config.EnableDeadlockDetection {
		if deadlock := dlm.detectPotentialDeadlock(ctx, lock); deadlock {
			return &LockResult{
				LockID:   lockID,
				Success:  false,
				WaitTime: time.Since(startTime),
				Error:    "potential deadlock detected",
			}, nil
		}
	}

	// Attempt to acquire lock with consensus if required
	if dlm.config.RequireConsensus {
		if err := dlm.acquireWithConsensus(ctx, lock); err != nil {
			return &LockResult{
				LockID:   lockID,
				Success:  false,
				WaitTime: time.Since(startTime),
				Error:    fmt.Sprintf("consensus acquisition failed: %v", err),
			}, err
		}
	} else {
		if err := dlm.acquireWithoutConsensus(ctx, lock); err != nil {
			return &LockResult{
				LockID:   lockID,
				Success:  false,
				WaitTime: time.Since(startTime),
				Error:    fmt.Sprintf("lock acquisition failed: %v", err),
			}, err
		}
	}

	// Update local state
	dlm.addLocalLock(lock)

	// Update metrics
	dlm.updateMetrics(func(m *DistributedLockMetrics) {
		m.LocksAcquired++
		m.AverageWaitTime = (m.AverageWaitTime + time.Since(startTime)) / 2
		m.ActiveLocks++
	})

	dlm.logger.Info("Distributed lock acquired successfully",
		zap.String("lock_id", lockID),
		zap.Duration("wait_time", time.Since(startTime)))

	return &LockResult{
		LockID:   lockID,
		Success:  true,
		WaitTime: time.Since(startTime),
		Lock:     lock,
	}, nil
}

// ReleaseLock releases a distributed lock
func (dlm *DistributedLockManager) ReleaseLock(ctx context.Context, lockID string) error {
	dlm.logger.Info("Releasing distributed lock", zap.String("lock_id", lockID))

	// Get local lock
	localLock := dlm.getLocalLock(lockID)
	if localLock == nil {
		return fmt.Errorf("lock not found locally: %s", lockID)
	}

	// Update lock status in database
	if err := dlm.updateLockStatus(ctx, lockID, "released"); err != nil {
		return fmt.Errorf("failed to update lock status: %w", err)
	}

	// Remove from local state
	dlm.removeLocalLock(lockID)

	// Update metrics
	dlm.updateMetrics(func(m *DistributedLockMetrics) {
		m.LocksReleased++
		m.ActiveLocks--
		if localLock.DistributedLock != nil {
			lockDuration := time.Since(localLock.DistributedLock.AcquiredAt)
			m.AverageLockDuration = (m.AverageLockDuration + lockDuration) / 2
		}
	})

	dlm.logger.Info("Distributed lock released successfully", zap.String("lock_id", lockID))
	return nil
}

// RefreshLock extends the lease on a distributed lock
func (dlm *DistributedLockManager) RefreshLock(ctx context.Context, lockID string, additionalTime time.Duration) error {
	localLock := dlm.getLocalLock(lockID)
	if localLock == nil {
		return fmt.Errorf("lock not found: %s", lockID)
	}

	// Update expiration time in database
	newExpiration := time.Now().Add(additionalTime)
	if err := dlm.updateLockExpiration(ctx, lockID, newExpiration); err != nil {
		return fmt.Errorf("failed to refresh lock: %w", err)
	}

	// Update local state
	localLock.DistributedLock.ExpiresAt = newExpiration
	localLock.LastActivity = time.Now()

	return nil
}

// IsLockHeld checks if a lock is currently held
func (dlm *DistributedLockManager) IsLockHeld(ctx context.Context, resourceID string) (bool, error) {
	var count int64
	err := dlm.db.WithContext(ctx).
		Model(&DistributedLock{}).
		Where("resource_id = ? AND status = ? AND expires_at > ?", resourceID, "acquired", time.Now()).
		Count(&count).Error

	return count > 0, err
}

// GetActiveLocks returns all active locks
func (dlm *DistributedLockManager) GetActiveLocks() []*DistributedLock {
	dlm.localLocksMutex.RLock()
	defer dlm.localLocksMutex.RUnlock()

	locks := make([]*DistributedLock, 0, len(dlm.localLocks))
	for _, localLock := range dlm.localLocks {
		if localLock.DistributedLock != nil {
			locks = append(locks, localLock.DistributedLock)
		}
	}

	return locks
}

// acquireWithConsensus acquires a lock with consensus approval
func (dlm *DistributedLockManager) acquireWithConsensus(ctx context.Context, lock *DistributedLock) error {
	operation := consensus.Operation{
		ID:   fmt.Sprintf("lock-%s", lock.ID),
		Type: consensus.OperationTypeLockAcquisition,
		Data: map[string]interface{}{
			"lock_id":     lock.ID,
			"resource_id": lock.ResourceID,
			"holder_id":   lock.HolderID,
			"lock_type":   lock.LockType,
			"expires_at":  lock.ExpiresAt,
		},
		Timestamp: time.Now(),
	}

	consensusCtx, cancel := context.WithTimeout(ctx, dlm.config.ConsensusTimeout)
	defer cancel()

	approved, err := dlm.raftCoordinator.ProposeGenericOperation(consensusCtx, operation)
	if err != nil {
		return fmt.Errorf("consensus proposal failed: %w", err)
	}

	if !approved {
		return fmt.Errorf("lock acquisition not approved by consensus")
	}

	// Store lock in database after consensus approval
	return dlm.storeLock(ctx, lock)
}

// acquireWithoutConsensus acquires a lock without consensus
func (dlm *DistributedLockManager) acquireWithoutConsensus(ctx context.Context, lock *DistributedLock) error {
	// Check for conflicting locks
	if conflict, err := dlm.checkForConflicts(ctx, lock); err != nil {
		return fmt.Errorf("conflict check failed: %w", err)
	} else if conflict {
		return fmt.Errorf("conflicting lock exists for resource: %s", lock.ResourceID)
	}

	// Store lock in database
	return dlm.storeLock(ctx, lock)
}

// checkForConflicts checks for conflicting locks
func (dlm *DistributedLockManager) checkForConflicts(ctx context.Context, lock *DistributedLock) (bool, error) {
	var conflicts []DistributedLock
	query := dlm.db.WithContext(ctx).
		Where("resource_id = ? AND status = ? AND expires_at > ?", lock.ResourceID, "acquired", time.Now())

	// For write locks, any existing lock is a conflict
	if lock.LockType == "write" || lock.LockType == "exclusive" {
		query = query.Where("1 = 1") // Any lock conflicts
	} else if lock.LockType == "read" {
		// For read locks, only write/exclusive locks conflict
		query = query.Where("lock_type IN (?, ?)", "write", "exclusive")
	}

	err := query.Find(&conflicts).Error
	return len(conflicts) > 0, err
}

// storeLock stores a lock in the database
func (dlm *DistributedLockManager) storeLock(ctx context.Context, lock *DistributedLock) error {
	lock.Status = "acquired"
	lock.CreatedAt = time.Now()
	lock.UpdatedAt = time.Now()

	return dlm.db.WithContext(ctx).Create(lock).Error
}

// detectPotentialDeadlock checks for potential deadlocks
func (dlm *DistributedLockManager) detectPotentialDeadlock(ctx context.Context, lock *DistributedLock) bool {
	// Simple deadlock detection based on circular dependencies
	// This is a simplified implementation - real deadlock detection is more complex

	// Check if the holder already holds locks that might create cycles
	var holderLocks []DistributedLock
	err := dlm.db.WithContext(ctx).
		Where("holder_id = ? AND status = ?", lock.HolderID, "acquired").
		Find(&holderLocks).Error

	if err != nil {
		dlm.logger.Warn("Failed to check for deadlock", zap.Error(err))
		return false
	}

	// If holder has many locks, there's higher deadlock risk
	return len(holderLocks) > 5
}

// Background workers

// heartbeatWorker maintains heartbeats for all local locks
func (dlm *DistributedLockManager) heartbeatWorker() {
	defer dlm.wg.Done()

	ticker := time.NewTicker(dlm.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dlm.ctx.Done():
			return
		case <-ticker.C:
			dlm.sendHeartbeats()
		}
	}
}

// cleanupWorker periodically cleans up expired locks
func (dlm *DistributedLockManager) cleanupWorker() {
	defer dlm.wg.Done()

	ticker := time.NewTicker(dlm.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dlm.ctx.Done():
			return
		case <-ticker.C:
			dlm.cleanupExpiredLocks()
		}
	}
}

// deadlockDetectionWorker periodically checks for deadlocks
func (dlm *DistributedLockManager) deadlockDetectionWorker() {
	defer dlm.wg.Done()

	if !dlm.config.EnableDeadlockDetection {
		return
	}

	ticker := time.NewTicker(dlm.config.DeadlockTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-dlm.ctx.Done():
			return
		case <-ticker.C:
			dlm.detectAndResolveDeadlocks()
		}
	}
}

// sendHeartbeats sends heartbeats for all local locks
func (dlm *DistributedLockManager) sendHeartbeats() {
	dlm.localLocksMutex.RLock()
	locks := make([]*LocalLock, 0, len(dlm.localLocks))
	for _, lock := range dlm.localLocks {
		locks = append(locks, lock)
	}
	dlm.localLocksMutex.RUnlock()

	for _, localLock := range locks {
		if err := dlm.sendHeartbeat(localLock); err != nil {
			dlm.logger.Error("Failed to send heartbeat",
				zap.String("lock_id", localLock.DistributedLock.ID),
				zap.Error(err))
		}
	}
}

// sendHeartbeat sends a heartbeat for a specific lock
func (dlm *DistributedLockManager) sendHeartbeat(localLock *LocalLock) error {
	now := time.Now()

	err := dlm.db.Model(&DistributedLock{}).
		Where("id = ?", localLock.DistributedLock.ID).
		Update("last_heartbeat", now).Error

	if err == nil {
		localLock.DistributedLock.LastHeartbeat = now
		localLock.LastActivity = now
	}

	return err
}

// cleanupExpiredLocks removes expired locks from the database
func (dlm *DistributedLockManager) cleanupExpiredLocks() {
	result := dlm.db.Where("expires_at < ? AND status != ?", time.Now(), "released").
		Delete(&DistributedLock{})

	if result.Error != nil {
		dlm.logger.Error("Failed to cleanup expired locks", zap.Error(result.Error))
	} else if result.RowsAffected > 0 {
		dlm.logger.Info("Cleaned up expired locks", zap.Int64("count", result.RowsAffected))

		dlm.updateMetrics(func(m *DistributedLockMetrics) {
			m.LockTimeouts += result.RowsAffected
		})
	}
}

// detectAndResolveDeadlocks detects and resolves deadlocks
func (dlm *DistributedLockManager) detectAndResolveDeadlocks() {
	// This is a simplified deadlock detection and resolution
	// Real implementation would use more sophisticated algorithms

	var oldLocks []DistributedLock
	cutoff := time.Now().Add(-dlm.config.DeadlockTimeout)

	err := dlm.db.Where("acquired_at < ? AND status = ?", cutoff, "acquired").Find(&oldLocks).Error
	if err != nil {
		dlm.logger.Error("Failed to check for old locks", zap.Error(err))
		return
	}

	for _, lock := range oldLocks {
		dlm.logger.Warn("Potentially deadlocked lock detected",
			zap.String("lock_id", lock.ID),
			zap.String("resource_id", lock.ResourceID),
			zap.Duration("age", time.Since(lock.AcquiredAt)))

		dlm.updateMetrics(func(m *DistributedLockMetrics) {
			m.DeadlocksDetected++
		})
	}
}

// Helper methods for local lock management

func (dlm *DistributedLockManager) addLocalLock(lock *DistributedLock) {
	dlm.localLocksMutex.Lock()
	defer dlm.localLocksMutex.Unlock()

	ctx, cancel := context.WithCancel(dlm.ctx)
	localLock := &LocalLock{
		DistributedLock: lock,
		Context:         ctx,
		Cancel:          cancel,
		LastActivity:    time.Now(),
	}

	dlm.localLocks[lock.ID] = localLock
}

func (dlm *DistributedLockManager) getLocalLock(lockID string) *LocalLock {
	dlm.localLocksMutex.RLock()
	defer dlm.localLocksMutex.RUnlock()

	return dlm.localLocks[lockID]
}

func (dlm *DistributedLockManager) removeLocalLock(lockID string) {
	dlm.localLocksMutex.Lock()
	defer dlm.localLocksMutex.Unlock()

	if localLock, exists := dlm.localLocks[lockID]; exists {
		localLock.Cancel()
		if localLock.Heartbeat != nil {
			localLock.Heartbeat.Stop()
		}
		delete(dlm.localLocks, lockID)
	}
}

func (dlm *DistributedLockManager) releaseAllLocalLocks() {
	dlm.localLocksMutex.Lock()
	defer dlm.localLocksMutex.Unlock()

	for lockID, localLock := range dlm.localLocks {
		localLock.Cancel()
		if localLock.Heartbeat != nil {
			localLock.Heartbeat.Stop()
		}

		// Update database
		dlm.updateLockStatus(context.Background(), lockID, "released")
	}

	dlm.localLocks = make(map[string]*LocalLock)
}

// Database operations

func (dlm *DistributedLockManager) createLocksTable() error {
	return dlm.db.AutoMigrate(&DistributedLock{})
}

func (dlm *DistributedLockManager) updateLockStatus(ctx context.Context, lockID, status string) error {
	return dlm.db.WithContext(ctx).
		Model(&DistributedLock{}).
		Where("id = ?", lockID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func (dlm *DistributedLockManager) updateLockExpiration(ctx context.Context, lockID string, expiresAt time.Time) error {
	return dlm.db.WithContext(ctx).
		Model(&DistributedLock{}).
		Where("id = ?", lockID).
		Updates(map[string]interface{}{
			"expires_at": expiresAt,
			"updated_at": time.Now(),
		}).Error
}

// Utility methods

func (dlm *DistributedLockManager) getNodeID() string {
	// This should return a unique node identifier
	// For now, we'll use a simple implementation
	return "node-1"
}

func (dlm *DistributedLockManager) updateMetrics(updateFunc func(*DistributedLockMetrics)) {
	dlm.metricsMutex.Lock()
	defer dlm.metricsMutex.Unlock()

	updateFunc(dlm.metrics)
	dlm.metrics.LastUpdate = time.Now()
}

// GetMetrics returns current distributed lock metrics
func (dlm *DistributedLockManager) GetMetrics() *DistributedLockMetrics {
	dlm.metricsMutex.RLock()
	defer dlm.metricsMutex.RUnlock()

	metricsCopy := *dlm.metrics
	return &metricsCopy
}

// DefaultDistributedLockConfig returns default configuration
func DefaultDistributedLockConfig() *DistributedLockConfig {
	return &DistributedLockConfig{
		DefaultLockTimeout:      30 * time.Second,
		HeartbeatInterval:       5 * time.Second,
		CleanupInterval:         1 * time.Minute,
		ConsensusTimeout:        10 * time.Second,
		RequireConsensus:        true,
		MaxConcurrentLocks:      1000,
		EnableLocalCaching:      true,
		EnableDeadlockDetection: true,
		DeadlockTimeout:         2 * time.Minute,
	}
}
