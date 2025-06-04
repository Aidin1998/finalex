package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TransactionRecoveryManager provides enhanced recovery capabilities
type TransactionRecoveryManager struct {
	db                 *gorm.DB
	xaManager          *XATransactionManager
	lockManager        *DistributedLockManager
	monitor            *TransactionMonitor
	recoveryStrategies map[XATransactionState]RecoveryStrategy
	config             *RecoveryConfig
	activeRecoveries   map[string]*RecoverySession
	mu                 sync.RWMutex
	stopChan           chan struct{}
	running            bool
}

// RecoveryConfig defines recovery behavior configuration
type RecoveryConfig struct {
	// Basic settings
	RecoveryInterval    time.Duration `json:"recovery_interval"`
	MaxRecoveryAttempts int           `json:"max_recovery_attempts"`
	RecoveryTimeout     time.Duration `json:"recovery_timeout"`

	// Advanced settings
	BackoffStrategy   BackoffStrategy `json:"backoff_strategy"`
	InitialBackoff    time.Duration   `json:"initial_backoff"`
	MaxBackoff        time.Duration   `json:"max_backoff"`
	BackoffMultiplier float64         `json:"backoff_multiplier"`

	// Circuit breaker settings
	FailureThreshold      int           `json:"failure_threshold"`
	SuccessThreshold      int           `json:"success_threshold"`
	CircuitBreakerTimeout time.Duration `json:"circuit_breaker_timeout"`

	// Parallel recovery settings
	MaxConcurrentRecoveries int  `json:"max_concurrent_recoveries"`
	EnableParallelRecovery  bool `json:"enable_parallel_recovery"`

	// Compensation settings
	EnableCompensation  bool          `json:"enable_compensation"`
	CompensationTimeout time.Duration `json:"compensation_timeout"`

	// Notification settings
	NotifyOnRecovery bool `json:"notify_on_recovery"`
	NotifyOnFailure  bool `json:"notify_on_failure"`
}

// BackoffStrategy defines different backoff strategies for recovery
type BackoffStrategy string

const (
	BackoffLinear      BackoffStrategy = "linear"
	BackoffExponential BackoffStrategy = "exponential"
	BackoffFixed       BackoffStrategy = "fixed"
	BackoffJittered    BackoffStrategy = "jittered"
)

// RecoveryStrategy defines how to recover from specific transaction states
type RecoveryStrategy interface {
	Recover(ctx context.Context, txn *XATransaction, session *RecoverySession) error
	CanRecover(txn *XATransaction) bool
	GetPriority() int
	GetDescription() string
}

// RecoverySession tracks an ongoing recovery process
type RecoverySession struct {
	ID             string                 `json:"id"`
	TransactionID  string                 `json:"transaction_id"`
	Strategy       string                 `json:"strategy"`
	StartTime      time.Time              `json:"start_time"`
	LastAttempt    time.Time              `json:"last_attempt"`
	AttemptCount   int                    `json:"attempt_count"`
	Status         RecoveryStatus         `json:"status"`
	Error          error                  `json:"error,omitempty"`
	CircuitBreaker *CircuitBreaker        `json:"circuit_breaker"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// RecoveryStatus represents the status of a recovery session
type RecoveryStatus string

const (
	RecoveryStatusActive    RecoveryStatus = "active"
	RecoveryStatusSucceeded RecoveryStatus = "succeeded"
	RecoveryStatusFailed    RecoveryStatus = "failed"
	RecoveryStatusSkipped   RecoveryStatus = "skipped"
	RecoveryStatusTimeout   RecoveryStatus = "timeout"
)

// CircuitBreaker implements circuit breaker pattern for recovery
type CircuitBreaker struct {
	State           CircuitState    `json:"state"`
	FailureCount    int             `json:"failure_count"`
	SuccessCount    int             `json:"success_count"`
	LastFailureTime time.Time       `json:"last_failure_time"`
	LastSuccessTime time.Time       `json:"last_success_time"`
	NextAttemptTime time.Time       `json:"next_attempt_time"`
	Config          *RecoveryConfig `json:"-"`
}

// CircuitState represents circuit breaker states
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// RecoveryEvent represents a recovery event for auditing
type RecoveryEvent struct {
	ID            uint   `gorm:"primaryKey"`
	SessionID     string `gorm:"index"`
	TransactionID string `gorm:"index"`
	EventType     string `gorm:"index"`
	Strategy      string
	AttemptNumber int
	Status        string
	Message       string
	ErrorDetails  string    `gorm:"type:text"`
	Duration      int64     // microseconds
	CreatedAt     time.Time `gorm:"index"`
}

// NewTransactionRecoveryManager creates a new recovery manager
func NewTransactionRecoveryManager(
	db *gorm.DB,
	xaManager *XATransactionManager,
	lockManager *DistributedLockManager,
	monitor *TransactionMonitor,
) *TransactionRecoveryManager {

	// Auto-migrate recovery tables
	db.AutoMigrate(&RecoveryEvent{})

	config := &RecoveryConfig{
		RecoveryInterval:        30 * time.Second,
		MaxRecoveryAttempts:     5,
		RecoveryTimeout:         5 * time.Minute,
		BackoffStrategy:         BackoffExponential,
		InitialBackoff:          1 * time.Second,
		MaxBackoff:              60 * time.Second,
		BackoffMultiplier:       2.0,
		FailureThreshold:        3,
		SuccessThreshold:        2,
		CircuitBreakerTimeout:   10 * time.Minute,
		MaxConcurrentRecoveries: 10,
		EnableParallelRecovery:  true,
		EnableCompensation:      true,
		CompensationTimeout:     30 * time.Second,
		NotifyOnRecovery:        true,
		NotifyOnFailure:         true,
	}

	manager := &TransactionRecoveryManager{
		db:               db,
		xaManager:        xaManager,
		lockManager:      lockManager,
		monitor:          monitor,
		config:           config,
		activeRecoveries: make(map[string]*RecoverySession),
		stopChan:         make(chan struct{}),
	}

	// Initialize recovery strategies
	manager.initializeRecoveryStrategies()

	return manager
}

// initializeRecoveryStrategies sets up default recovery strategies
func (rm *TransactionRecoveryManager) initializeRecoveryStrategies() {
	rm.recoveryStrategies = map[XATransactionState]RecoveryStrategy{
		XAStatePreparing:  &PreparingRecoveryStrategy{manager: rm},
		XAStatePrepared:   &PreparedRecoveryStrategy{manager: rm},
		XAStateCommitting: &CommittingRecoveryStrategy{manager: rm},
		XAStateAborting:   &AbortingRecoveryStrategy{manager: rm},
		XAStateUnknown:    &UnknownStateRecoveryStrategy{manager: rm},
	}
}

// Start begins the recovery manager
func (rm *TransactionRecoveryManager) Start(ctx context.Context) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.running {
		return fmt.Errorf("recovery manager is already running")
	}

	rm.running = true

	// Start recovery loop
	go rm.recoveryLoop(ctx)

	// Start cleanup loop
	go rm.cleanupLoop(ctx)

	log.Println("Transaction recovery manager started")
	return nil
}

// Stop stops the recovery manager
func (rm *TransactionRecoveryManager) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.running {
		return
	}

	close(rm.stopChan)
	rm.running = false
	log.Println("Transaction recovery manager stopped")
}

// recoveryLoop runs the main recovery process
func (rm *TransactionRecoveryManager) recoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(rm.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.performRecoveryCheck(ctx)
		}
	}
}

// performRecoveryCheck identifies and recovers stuck transactions
func (rm *TransactionRecoveryManager) performRecoveryCheck(ctx context.Context) {
	// Find stuck transactions
	stuckTransactions, err := rm.findStuckTransactions(ctx)
	if err != nil {
		log.Printf("Error finding stuck transactions: %v", err)
		return
	}

	log.Printf("Found %d stuck transactions for recovery", len(stuckTransactions))

	// Process recoveries (with concurrency control)
	semaphore := make(chan struct{}, rm.config.MaxConcurrentRecoveries)
	var wg sync.WaitGroup

	for _, txn := range stuckTransactions {
		// Check if recovery is already in progress
		txnIDStr := txn.ID.String()
		if rm.isRecoveryInProgress(txnIDStr) {
			continue
		}

		if rm.config.EnableParallelRecovery {
			wg.Add(1)
			go func(transaction *XATransaction) {
				defer wg.Done()
				semaphore <- struct{}{}        // Acquire
				defer func() { <-semaphore }() // Release

				rm.recoverTransaction(ctx, transaction)
			}(txn)
		} else {
			rm.recoverTransaction(ctx, txn)
		}
	}

	if rm.config.EnableParallelRecovery {
		wg.Wait()
	}
}

// findStuckTransactions identifies transactions that need recovery
func (rm *TransactionRecoveryManager) findStuckTransactions(ctx context.Context) ([]*XATransaction, error) {
	var transactions []*XATransaction

	// Find transactions older than recovery timeout in non-final states
	cutoffTime := time.Now().Add(-rm.config.RecoveryTimeout)

	err := rm.db.Where("state IN ? AND updated_at < ?",
		[]string{string(XAStatePreparing), string(XAStatePrepared), string(XAStateCommitting), string(XAStateAborting), string(XAStateUnknown)},
		cutoffTime,
	).Find(&transactions).Error

	if err != nil {
		return nil, fmt.Errorf("failed to find stuck transactions: %w", err)
	}

	// Filter out transactions that are locked by other processes
	filteredTransactions := make([]*XATransaction, 0)
	for _, txn := range transactions {
		lockKey := fmt.Sprintf("recovery:%s", txn.ID.String())
		// TODO: Replace TryLock with AcquireLock or implement a non-blocking lock if needed
		if lock, err := rm.lockManager.AcquireLock(ctx, lockKey, "recovery", 1*time.Minute); err == nil && lock != nil {
			filteredTransactions = append(filteredTransactions, txn)
			// Optionally: defer lock.Release() or track locks for later release
		}
	}

	return filteredTransactions, nil
}

// recoverTransaction attempts to recover a single transaction
func (rm *TransactionRecoveryManager) recoverTransaction(ctx context.Context, txn *XATransaction) {
	sessionID := uuid.New().String()

	session := &RecoverySession{
		ID:            sessionID,
		TransactionID: txn.ID.String(),
		StartTime:     time.Now(),
		Status:        RecoveryStatusActive,
		CircuitBreaker: &CircuitBreaker{
			State:  CircuitClosed,
			Config: rm.config,
		},
		Metadata: make(map[string]interface{}),
	}

	rm.mu.Lock()
	rm.activeRecoveries[txn.ID.String()] = session
	rm.mu.Unlock()

	defer func() {
		rm.mu.Lock()
		delete(rm.activeRecoveries, txn.ID.String())
		rm.mu.Unlock()

		// Unlock the transaction
		lockKey := fmt.Sprintf("recovery:%s", txn.ID.String())
		rm.lockManager.ReleaseLock(ctx, lockKey)
	}()

	// Record recovery start event
	rm.recordRecoveryEvent(sessionID, txn.ID.String(), "recovery_started", "", 0, RecoveryStatusActive, "", nil, 0)

	// Find appropriate recovery strategy
	strategy, exists := rm.recoveryStrategies[txn.State]
	if !exists {
		rm.recordRecoveryEvent(sessionID, txn.ID.String(), "recovery_failed", "", 0, RecoveryStatusFailed, "No recovery strategy for state", fmt.Errorf("no strategy for state %s", txn.State), 0)
		session.Status = RecoveryStatusFailed
		session.Error = fmt.Errorf("no recovery strategy for state %s", txn.State)
		return
	}

	session.Strategy = strategy.GetDescription()

	// Attempt recovery with backoff and circuit breaker
	maxAttempts := rm.config.MaxRecoveryAttempts
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		session.AttemptCount = attempt
		session.LastAttempt = time.Now()

		// Check circuit breaker
		if !session.CircuitBreaker.CanAttempt() {
			rm.recordRecoveryEvent(sessionID, txn.ID.String(), "recovery_skipped", session.Strategy, attempt, RecoveryStatusSkipped, "Circuit breaker open", nil, 0)
			session.Status = RecoveryStatusSkipped
			return
		}

		// Apply backoff delay (except for first attempt)
		if attempt > 1 {
			delay := rm.calculateBackoff(attempt - 1)
			time.Sleep(delay)
		}

		// Attempt recovery
		start := time.Now()
		err := strategy.Recover(ctx, txn, session)
		duration := time.Since(start)

		if err == nil {
			// Success
			session.CircuitBreaker.RecordSuccess()
			session.Status = RecoveryStatusSucceeded

			rm.recordRecoveryEvent(sessionID, txn.ID.String(), "recovery_succeeded", session.Strategy, attempt, RecoveryStatusSucceeded, "Recovery completed successfully", nil, duration.Microseconds())

			if rm.config.NotifyOnRecovery && rm.monitor != nil {
				rm.monitor.RecordTransactionEvent(ctx, txn.ID.String(), "recovery_success", "Transaction successfully recovered", SeverityLow, map[string]interface{}{
					"session_id": sessionID,
					"strategy":   session.Strategy,
					"attempts":   attempt,
					"duration":   duration.String(),
				})
			}

			log.Printf("Successfully recovered transaction %s using %s strategy (attempt %d)", txn.ID, session.Strategy, attempt)
			return
		}

		// Failure
		session.CircuitBreaker.RecordFailure()
		session.Error = err

		rm.recordRecoveryEvent(sessionID, txn.ID.String(), "recovery_attempt_failed", session.Strategy, attempt, RecoveryStatusFailed, "Recovery attempt failed", err, duration.Microseconds())

		log.Printf("Recovery attempt %d failed for transaction %s: %v", attempt, txn.ID, err)

		// Check if we should continue
		if attempt == maxAttempts || session.CircuitBreaker.State == CircuitOpen {
			break
		}
	}

	// Final failure
	session.Status = RecoveryStatusFailed
	rm.recordRecoveryEvent(sessionID, txn.ID.String(), "recovery_failed", session.Strategy, session.AttemptCount, RecoveryStatusFailed, "Max attempts exceeded", session.Error, 0)

	if rm.config.NotifyOnFailure && rm.monitor != nil {
		rm.monitor.RecordTransactionEvent(ctx, txn.ID.String(), "recovery_failure", "Transaction recovery failed", SeverityHigh, map[string]interface{}{
			"session_id": sessionID,
			"strategy":   session.Strategy,
			"attempts":   session.AttemptCount,
			"error":      session.Error.Error(),
		})
	}

	log.Printf("Failed to recover transaction %s after %d attempts", txn.ID, session.AttemptCount)
}

// calculateBackoff calculates backoff delay based on strategy
func (rm *TransactionRecoveryManager) calculateBackoff(attempt int) time.Duration {
	switch rm.config.BackoffStrategy {
	case BackoffFixed:
		return rm.config.InitialBackoff

	case BackoffLinear:
		delay := time.Duration(attempt) * rm.config.InitialBackoff
		if delay > rm.config.MaxBackoff {
			return rm.config.MaxBackoff
		}
		return delay

	case BackoffExponential:
		delay := time.Duration(float64(rm.config.InitialBackoff) * (rm.config.BackoffMultiplier * float64(attempt)))
		if delay > rm.config.MaxBackoff {
			return rm.config.MaxBackoff
		}
		return delay

	case BackoffJittered:
		base := time.Duration(float64(rm.config.InitialBackoff) * (rm.config.BackoffMultiplier * float64(attempt)))
		if base > rm.config.MaxBackoff {
			base = rm.config.MaxBackoff
		}
		// Add up to 20% jitter
		jitter := time.Duration(float64(base) * 0.2 * (0.5 - (float64(time.Now().UnixNano()%1000) / 1000.0)))
		return base + jitter

	default:
		return rm.config.InitialBackoff
	}
}

// isRecoveryInProgress checks if recovery is already in progress for a transaction
func (rm *TransactionRecoveryManager) isRecoveryInProgress(txnID string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	session, exists := rm.activeRecoveries[txnID]
	return exists && session.Status == RecoveryStatusActive
}

// recordRecoveryEvent records a recovery event for auditing
func (rm *TransactionRecoveryManager) recordRecoveryEvent(sessionID, txnID, eventType, strategy string, attempt int, status RecoveryStatus, message string, err error, duration int64) {
	event := &RecoveryEvent{
		SessionID:     sessionID,
		TransactionID: txnID,
		EventType:     eventType,
		Strategy:      strategy,
		AttemptNumber: attempt,
		Status:        string(status),
		Message:       message,
		Duration:      duration,
		CreatedAt:     time.Now(),
	}

	if err != nil {
		event.ErrorDetails = err.Error()
	}

	if dbErr := rm.db.Create(event).Error; dbErr != nil {
		log.Printf("Failed to record recovery event: %v", dbErr)
	}
}

// cleanupLoop cleans up completed recovery sessions
func (rm *TransactionRecoveryManager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.cleanupOldSessions()
		}
	}
}

// cleanupOldSessions removes old recovery sessions
func (rm *TransactionRecoveryManager) cleanupOldSessions() {
	cutoff := time.Now().Add(-1 * time.Hour)

	rm.mu.Lock()
	for txnID, session := range rm.activeRecoveries {
		if session.StartTime.Before(cutoff) && session.Status != RecoveryStatusActive {
			delete(rm.activeRecoveries, txnID)
		}
	}
	rm.mu.Unlock()

	// Clean up old recovery events
	rm.db.Where("created_at < ?", time.Now().Add(-24*time.Hour)).Delete(&RecoveryEvent{})
}

// CanAttempt checks if the circuit breaker allows an attempt
func (cb *CircuitBreaker) CanAttempt() bool {
	now := time.Now()

	switch cb.State {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if now.After(cb.NextAttemptTime) {
			cb.State = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.LastSuccessTime = time.Now()
	cb.SuccessCount++

	if cb.State == CircuitHalfOpen {
		if cb.SuccessCount >= cb.Config.SuccessThreshold {
			cb.State = CircuitClosed
			cb.FailureCount = 0
		}
	} else if cb.State == CircuitClosed {
		cb.FailureCount = 0 // Reset failure count on success
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.LastFailureTime = time.Now()
	cb.FailureCount++
	cb.SuccessCount = 0

	if cb.State == CircuitClosed {
		if cb.FailureCount >= cb.Config.FailureThreshold {
			cb.State = CircuitOpen
			cb.NextAttemptTime = time.Now().Add(cb.Config.CircuitBreakerTimeout)
		}
	} else if cb.State == CircuitHalfOpen {
		cb.State = CircuitOpen
		cb.NextAttemptTime = time.Now().Add(cb.Config.CircuitBreakerTimeout)
	}
}

// GetRecoverySession returns the current recovery session for a transaction
func (rm *TransactionRecoveryManager) GetRecoverySession(txnID string) (*RecoverySession, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	session, exists := rm.activeRecoveries[txnID]
	return session, exists
}

// GetRecoveryHistory returns the recovery history for a transaction
func (rm *TransactionRecoveryManager) GetRecoveryHistory(ctx context.Context, txnID string) ([]*RecoveryEvent, error) {
	var events []*RecoveryEvent

	err := rm.db.Where("transaction_id = ?", txnID).
		Order("created_at DESC").
		Find(&events).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get recovery history: %w", err)
	}

	return events, nil
}

// UpdateRecoveryConfig updates the recovery configuration
func (rm *TransactionRecoveryManager) UpdateRecoveryConfig(config *RecoveryConfig) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Validate configuration
	if config.MaxRecoveryAttempts <= 0 {
		return fmt.Errorf("max recovery attempts must be positive")
	}
	if config.RecoveryTimeout <= 0 {
		return fmt.Errorf("recovery timeout must be positive")
	}
	if config.InitialBackoff <= 0 {
		return fmt.Errorf("initial backoff must be positive")
	}

	rm.config = config
	log.Println("Recovery configuration updated")
	return nil
}

// GetRecoveryConfig returns the current recovery configuration
func (rm *TransactionRecoveryManager) GetRecoveryConfig() *RecoveryConfig {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Return a copy to prevent external modification
	configJSON, _ := json.Marshal(rm.config)
	var configCopy RecoveryConfig
	json.Unmarshal(configJSON, &configCopy)
	return &configCopy
}
