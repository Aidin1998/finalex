// =============================
// Distributed Lock Manager
// =============================
// This file implements distributed locking for migration coordination
// ensuring only one migration can occur per resource at a time.

package migration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// LockManager manages distributed locks for migration coordination
type LockManager struct {
	logger *zap.SugaredLogger
	locks  map[string]*LockInfo
	mu     sync.RWMutex

	// Configuration
	defaultTTL    time.Duration
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// NewLockManager creates a new distributed lock manager
func NewLockManager(logger *zap.SugaredLogger) *LockManager {
	lm := &LockManager{
		logger:     logger,
		locks:      make(map[string]*LockInfo),
		defaultTTL: 1 * time.Hour,
		stopChan:   make(chan struct{}),
	}

	// Start cleanup worker
	lm.cleanupTicker = time.NewTicker(30 * time.Second)
	go lm.cleanupWorker()

	return lm
}

// AcquireLock attempts to acquire a distributed lock
func (lm *LockManager) AcquireLock(ctx context.Context, resource, owner string, ttl time.Duration) (uuid.UUID, error) {
	if resource == "" {
		return uuid.Nil, fmt.Errorf("resource cannot be empty")
	}
	if owner == "" {
		return uuid.Nil, fmt.Errorf("owner cannot be empty")
	}
	if ttl <= 0 {
		ttl = lm.defaultTTL
	}

	lockID := uuid.New()
	now := time.Now()

	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Check if resource is already locked
	if existingLock, exists := lm.locks[resource]; exists {
		if existingLock.IsActive && now.Before(existingLock.ExpiresAt) {
			return uuid.Nil, fmt.Errorf("resource %s is already locked by %s until %v",
				resource, existingLock.Owner, existingLock.ExpiresAt)
		}
		// Lock expired, we can proceed
	}

	// Create new lock
	lock := &LockInfo{
		ID:         lockID,
		Resource:   resource,
		Owner:      owner,
		AcquiredAt: now,
		ExpiresAt:  now.Add(ttl),
		TTL:        ttl,
		IsActive:   true,
		Metadata:   make(map[string]interface{}),
	}

	lm.locks[resource] = lock

	lm.logger.Infow("Lock acquired",
		"lock_id", lockID,
		"resource", resource,
		"owner", owner,
		"ttl", ttl,
		"expires_at", lock.ExpiresAt)

	return lockID, nil
}

// ReleaseLock releases a distributed lock
func (lm *LockManager) ReleaseLock(lockID uuid.UUID) error {
	if lockID == uuid.Nil {
		return fmt.Errorf("lock ID cannot be nil")
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Find the lock by ID
	var resourceToDelete string
	for resource, lock := range lm.locks {
		if lock.ID == lockID {
			resourceToDelete = resource
			break
		}
	}

	if resourceToDelete == "" {
		return fmt.Errorf("lock %s not found", lockID)
	}

	lock := lm.locks[resourceToDelete]
	lock.IsActive = false
	delete(lm.locks, resourceToDelete)

	lm.logger.Infow("Lock released",
		"lock_id", lockID,
		"resource", resourceToDelete,
		"owner", lock.Owner)

	return nil
}

// RenewLock extends the TTL of an existing lock
func (lm *LockManager) RenewLock(lockID uuid.UUID, additionalTTL time.Duration) error {
	if lockID == uuid.Nil {
		return fmt.Errorf("lock ID cannot be nil")
	}
	if additionalTTL <= 0 {
		return fmt.Errorf("additional TTL must be positive")
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Find the lock by ID
	var lock *LockInfo
	for _, l := range lm.locks {
		if l.ID == lockID {
			lock = l
			break
		}
	}

	if lock == nil {
		return fmt.Errorf("lock %s not found", lockID)
	}

	if !lock.IsActive {
		return fmt.Errorf("lock %s is not active", lockID)
	}

	// Check if lock has expired
	now := time.Now()
	if now.After(lock.ExpiresAt) {
		lock.IsActive = false
		return fmt.Errorf("lock %s has expired", lockID)
	}

	// Extend the expiration time
	lock.ExpiresAt = lock.ExpiresAt.Add(additionalTTL)
	lock.TTL = lock.TTL + additionalTTL

	lm.logger.Infow("Lock renewed",
		"lock_id", lockID,
		"resource", lock.Resource,
		"additional_ttl", additionalTTL,
		"new_expires_at", lock.ExpiresAt)

	return nil
}

// GetLockInfo returns information about a lock
func (lm *LockManager) GetLockInfo(lockID uuid.UUID) (*LockInfo, error) {
	if lockID == uuid.Nil {
		return nil, fmt.Errorf("lock ID cannot be nil")
	}

	lm.mu.RLock()
	defer lm.mu.RUnlock()

	for _, lock := range lm.locks {
		if lock.ID == lockID {
			// Return a copy to prevent external modifications
			lockCopy := *lock
			return &lockCopy, nil
		}
	}

	return nil, fmt.Errorf("lock %s not found", lockID)
}

// ListLocks returns all active locks
func (lm *LockManager) ListLocks() []*LockInfo {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var locks []*LockInfo
	now := time.Now()

	for _, lock := range lm.locks {
		if lock.IsActive && now.Before(lock.ExpiresAt) {
			lockCopy := *lock
			locks = append(locks, &lockCopy)
		}
	}

	return locks
}

// IsResourceLocked checks if a resource is currently locked
func (lm *LockManager) IsResourceLocked(resource string) bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	lock, exists := lm.locks[resource]
	if !exists {
		return false
	}

	now := time.Now()
	return lock.IsActive && now.Before(lock.ExpiresAt)
}

// GetResourceLock returns the lock information for a resource
func (lm *LockManager) GetResourceLock(resource string) (*LockInfo, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	lock, exists := lm.locks[resource]
	if !exists {
		return nil, fmt.Errorf("no lock found for resource %s", resource)
	}

	now := time.Now()
	if !lock.IsActive || now.After(lock.ExpiresAt) {
		return nil, fmt.Errorf("resource %s is not locked", resource)
	}

	lockCopy := *lock
	return &lockCopy, nil
}

// ForceReleaseLock forcibly releases a lock (admin operation)
func (lm *LockManager) ForceReleaseLock(resource string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lock, exists := lm.locks[resource]
	if !exists {
		return fmt.Errorf("no lock found for resource %s", resource)
	}

	lock.IsActive = false
	delete(lm.locks, resource)

	lm.logger.Warnw("Lock forcibly released",
		"lock_id", lock.ID,
		"resource", resource,
		"owner", lock.Owner)

	return nil
}

// Shutdown gracefully shuts down the lock manager
func (lm *LockManager) Shutdown() error {
	lm.logger.Info("Shutting down lock manager")

	// Stop cleanup worker
	close(lm.stopChan)
	if lm.cleanupTicker != nil {
		lm.cleanupTicker.Stop()
	}

	// Release all active locks
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for resource, lock := range lm.locks {
		if lock.IsActive {
			lock.IsActive = false
			lm.logger.Infow("Released lock during shutdown",
				"lock_id", lock.ID,
				"resource", resource,
				"owner", lock.Owner)
		}
	}

	// Clear all locks
	lm.locks = make(map[string]*LockInfo)

	lm.logger.Info("Lock manager shutdown complete")
	return nil
}

// cleanupWorker periodically cleans up expired locks
func (lm *LockManager) cleanupWorker() {
	for {
		select {
		case <-lm.cleanupTicker.C:
			lm.cleanupExpiredLocks()
		case <-lm.stopChan:
			return
		}
	}
}

// cleanupExpiredLocks removes expired locks
func (lm *LockManager) cleanupExpiredLocks() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	now := time.Now()
	var expiredResources []string

	for resource, lock := range lm.locks {
		if lock.IsActive && now.After(lock.ExpiresAt) {
			lock.IsActive = false
			expiredResources = append(expiredResources, resource)
		}
	}

	// Remove expired locks
	for _, resource := range expiredResources {
		lock := lm.locks[resource]
		delete(lm.locks, resource)

		lm.logger.Infow("Expired lock cleaned up",
			"lock_id", lock.ID,
			"resource", resource,
			"owner", lock.Owner,
			"expired_at", lock.ExpiresAt)
	}

	if len(expiredResources) > 0 {
		lm.logger.Debugw("Cleaned up expired locks", "count", len(expiredResources))
	}
}

// GetLockMetrics returns metrics about the lock manager
func (lm *LockManager) GetLockMetrics() map[string]interface{} {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	now := time.Now()
	activeLocks := 0
	expiredLocks := 0

	for _, lock := range lm.locks {
		if lock.IsActive && now.Before(lock.ExpiresAt) {
			activeLocks++
		} else {
			expiredLocks++
		}
	}

	return map[string]interface{}{
		"total_locks":   len(lm.locks),
		"active_locks":  activeLocks,
		"expired_locks": expiredLocks,
		"default_ttl":   lm.defaultTTL.String(),
	}
}
