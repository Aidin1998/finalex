// High-performance repository layer with atomic operations and distributed locking
package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Repository provides ultra-high concurrency data access with atomic operations
type Repository struct {
	writeDB    *gorm.DB
	readDB     *gorm.DB
	pgxPool    *pgxpool.Pool
	cache      *CacheLayer
	redsync    *redsync.Redsync
	logger     *zap.Logger
	metrics    *RepositoryMetrics
	partitions *PartitionManager
}

// RepositoryMetrics holds Prometheus metrics for repository operations
type RepositoryMetrics struct {
	QueryDuration     *prometheus.HistogramVec
	QueryCount        *prometheus.CounterVec
	TransactionCount  *prometheus.CounterVec
	LockWaitTime      *prometheus.HistogramVec
	LockAcquisitions  *prometheus.CounterVec
	OptimisticRetries *prometheus.CounterVec
	PartitionUsage    *prometheus.GaugeVec
	ConnectionPool    *prometheus.GaugeVec
}

// BalanceUpdate represents an atomic balance update operation
type BalanceUpdate struct {
	UserID          uuid.UUID
	Currency        string
	BalanceDelta    decimal.Decimal
	LockedDelta     decimal.Decimal
	Type            string
	ReferenceID     string
	ExpectedVersion int64
}

// TransactionContext represents a database transaction with caching
type TransactionContext struct {
	tx      pgx.Tx
	cache   *CacheLayer
	metrics *RepositoryMetrics
	logger  *zap.Logger
}

// AuditLogEntry represents an immutable audit log for balance changes
// (not persisted here, but sent to an async audit log processor)
type AuditLogEntry struct {
	UserID     uuid.UUID
	Currency   string
	OldBalance decimal.Decimal
	NewBalance decimal.Decimal
	Delta      decimal.Decimal
	Timestamp  int64
	TraceID    string
}

// AuditLogQueue is a global async channel for audit entries (buffered)
var AuditLogQueue = make(chan AuditLogEntry, 100000)

// getTraceID returns a trace ID for audit logging (stub)
func getTraceID() string {
	return uuid.New().String()
}

// NewRepository creates a new high-performance repository
func NewRepository(writeDB, readDB *gorm.DB, pgxPool *pgxpool.Pool, cache *CacheLayer, redsync *redsync.Redsync, logger *zap.Logger) *Repository {
	metrics := &RepositoryMetrics{
		QueryDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "accounts_query_duration_seconds",
			Help:    "Duration of database queries",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 12),
		}, []string{"operation", "table"}),
		QueryCount: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_query_total",
			Help: "Total number of database queries",
		}, []string{"operation", "table", "status"}),
		TransactionCount: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_transaction_total",
			Help: "Total number of database transactions",
		}, []string{"operation", "status"}),
		LockWaitTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "accounts_lock_wait_duration_seconds",
			Help:    "Time spent waiting for locks",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		}, []string{"lock_type"}),
		LockAcquisitions: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_lock_acquisitions_total",
			Help: "Total number of lock acquisitions",
		}, []string{"lock_type", "status"}),
		OptimisticRetries: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_optimistic_retries_total",
			Help: "Total number of optimistic concurrency retries",
		}, []string{"operation"}),
		PartitionUsage: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_partition_usage",
			Help: "Usage statistics for database partitions",
		}, []string{"partition", "metric"}),
		ConnectionPool: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_connection_pool",
			Help: "Database connection pool statistics",
		}, []string{"pool", "metric"}),
	}

	return &Repository{
		writeDB:    writeDB,
		readDB:     readDB,
		pgxPool:    pgxPool,
		cache:      cache,
		redsync:    redsync,
		logger:     logger,
		metrics:    metrics,
		partitions: NewPartitionManager(writeDB, logger),
	}
}

// GetAccount retrieves account with caching and read replica optimization
func (r *Repository) GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*Account, error) {
	start := time.Now()
	defer r.metrics.QueryDuration.WithLabelValues("get_account", "accounts").Observe(time.Since(start).Seconds())

	// Try cache first
	if cachedAccount, err := r.cache.GetAccount(ctx, userID, currency); err == nil {
		account := &Account{
			ID:          cachedAccount.ID,
			UserID:      cachedAccount.UserID,
			Currency:    cachedAccount.Currency,
			Balance:     cachedAccount.Balance,
			Available:   cachedAccount.Available,
			Locked:      cachedAccount.Locked,
			Version:     cachedAccount.Version,
			AccountType: cachedAccount.AccountType,
			Status:      cachedAccount.Status,
			UpdatedAt:   cachedAccount.UpdatedAt,
		}
		r.metrics.QueryCount.WithLabelValues("get_account", "accounts", "cache_hit").Inc()
		return account, nil
	}

	// Cache miss - query database
	var account Account
	partition := r.partitions.GetAccountPartition(userID)

	err := r.readDB.Table(fmt.Sprintf("accounts_%s", partition)).
		Where("user_id = ? AND currency = ?", userID, currency).
		First(&account).Error

	if err != nil {
		r.metrics.QueryCount.WithLabelValues("get_account", "accounts", "error").Inc()
		return nil, err
	}

	// Cache the result
	cachedAccount := &CachedAccount{
		ID:           account.ID,
		UserID:       account.UserID,
		Currency:     account.Currency,
		Balance:      account.Balance,
		Available:    account.Available,
		Locked:       account.Locked,
		Version:      account.Version,
		AccountType:  account.AccountType,
		Status:       account.Status,
		UpdatedAt:    account.UpdatedAt,
		CachedAt:     time.Now(),
		AccessCount:  1,
		LastAccessed: time.Now(),
	}
	r.cache.SetAccount(ctx, cachedAccount)

	r.metrics.QueryCount.WithLabelValues("get_account", "accounts", "success").Inc()
	return &account, nil
}

// GetAccountBalance retrieves only balance information (optimized for high frequency)
func (r *Repository) GetAccountBalance(ctx context.Context, userID uuid.UUID, currency string) (decimal.Decimal, decimal.Decimal, int64, error) {
	start := time.Now()
	defer r.metrics.QueryDuration.WithLabelValues("get_balance", "accounts").Observe(time.Since(start).Seconds())

	// Try cache first
	if balance, available, version, err := r.cache.GetAccountBalance(ctx, userID, currency); err == nil {
		r.metrics.QueryCount.WithLabelValues("get_balance", "accounts", "cache_hit").Inc()
		return balance, available, version, nil
	}

	// Cache miss - query database with optimized query
	partition := r.partitions.GetAccountPartition(userID)

	var result struct {
		Balance   decimal.Decimal `gorm:"column:balance"`
		Available decimal.Decimal `gorm:"column:available"`
		Version   int64           `gorm:"column:version"`
	}

	err := r.readDB.Table(fmt.Sprintf("accounts_%s", partition)).
		Select("balance, available, version").
		Where("user_id = ? AND currency = ?", userID, currency).
		First(&result).Error

	if err != nil {
		r.metrics.QueryCount.WithLabelValues("get_balance", "accounts", "error").Inc()
		return decimal.Zero, decimal.Zero, 0, err
	}

	// Cache the result
	r.cache.SetAccountBalance(ctx, userID, currency, result.Balance, result.Available, result.Version)

	r.metrics.QueryCount.WithLabelValues("get_balance", "accounts", "success").Inc()
	return result.Balance, result.Available, result.Version, nil
}

// UpdateBalanceAtomic performs atomic balance updates with optimistic concurrency control
func (r *Repository) UpdateBalanceAtomic(ctx context.Context, update *BalanceUpdate) error {
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			r.metrics.OptimisticRetries.WithLabelValues("update_balance").Inc()
			// Exponential backoff
			time.Sleep(time.Duration(attempt*10) * time.Millisecond)
		}

		if err := r.performBalanceUpdate(ctx, update); err != nil {
			if isOptimisticLockError(err) && attempt < maxRetries-1 {
				continue // Retry on optimistic lock failure
			}
			return err
		}

		return nil
	}

	return fmt.Errorf("failed to update balance after %d attempts", maxRetries)
}

// performBalanceUpdate executes a single balance update attempt
func (r *Repository) performBalanceUpdate(ctx context.Context, update *BalanceUpdate) error {
	start := time.Now()
	defer r.metrics.QueryDuration.WithLabelValues("update_balance", "accounts").Observe(time.Since(start).Seconds())

	// Acquire distributed lock
	lockKey := fmt.Sprintf("account:lock:%s:%s", update.UserID.String(), update.Currency)
	mutex := r.redsync.NewMutex(lockKey, redsync.WithExpiry(LockTTL))

	lockStart := time.Now()
	if err := mutex.Lock(); err != nil {
		r.metrics.LockAcquisitions.WithLabelValues("account", "failed").Inc()
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock()

	r.metrics.LockWaitTime.WithLabelValues("account").Observe(time.Since(lockStart).Seconds())
	r.metrics.LockAcquisitions.WithLabelValues("account", "success").Inc()

	// Start database transaction
	tx, err := r.pgxPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	partition := r.partitions.GetAccountPartition(update.UserID)
	tableName := fmt.Sprintf("accounts_%s", partition)

	// Get current account with row-level lock
	var account Account
	query := fmt.Sprintf(`
		SELECT id, user_id, currency, balance, available, locked, version, account_type, status, updated_at
		FROM %s 
		WHERE user_id = $1 AND currency = $2 
		FOR UPDATE
	`, tableName)

	err = tx.QueryRow(ctx, query, update.UserID, update.Currency).Scan(
		&account.ID, &account.UserID, &account.Currency,
		&account.Balance, &account.Available, &account.Locked,
		&account.Version, &account.AccountType, &account.Status, &account.UpdatedAt,
	)

	if err != nil {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "error").Inc()
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Check optimistic concurrency control
	if update.ExpectedVersion > 0 && account.Version != update.ExpectedVersion {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "optimistic_lock_failed").Inc()
		return fmt.Errorf("optimistic lock failed: expected version %d, got %d", update.ExpectedVersion, account.Version)
	}

	// Calculate new balances
	newBalance := account.Balance.Add(update.BalanceDelta)
	newLocked := account.Locked.Add(update.LockedDelta)
	newAvailable := newBalance.Sub(newLocked)

	// Validate balance constraints
	if newBalance.IsNegative() || newAvailable.IsNegative() || newLocked.IsNegative() {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "insufficient_funds").Inc()
		return fmt.Errorf("insufficient funds: balance=%s, available=%s, locked=%s",
			newBalance, newAvailable, newLocked)
	}

	// Update account
	updateQuery := fmt.Sprintf(`
		UPDATE %s 
		SET balance = $1, available = $2, locked = $3, version = version + 1, updated_at = NOW()
		WHERE user_id = $4 AND currency = $5 AND version = $6
	`, tableName)

	cmdTag, err := tx.Exec(ctx, updateQuery, newBalance, newAvailable, newLocked,
		update.UserID, update.Currency, account.Version)

	if err != nil {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "error").Inc()
		return fmt.Errorf("failed to update account: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "optimistic_lock_failed").Inc()
		return fmt.Errorf("optimistic lock failed: no rows affected")
	}

	// Create transaction journal entry
	journalEntry := &TransactionJournal{
		ID:              uuid.New(),
		UserID:          update.UserID,
		Currency:        update.Currency,
		Type:            update.Type,
		Amount:          update.BalanceDelta,
		BalanceBefore:   account.Balance,
		BalanceAfter:    newBalance,
		AvailableBefore: account.Available,
		AvailableAfter:  newAvailable,
		LockedBefore:    account.Locked,
		LockedAfter:     newLocked,
		ReferenceID:     update.ReferenceID,
		Status:          "completed",
		CreatedAt:       time.Now(),
	}

	journalQuery := `
		INSERT INTO transaction_journal 
		(id, user_id, currency, type, amount, balance_before, balance_after, 
		 available_before, available_after, locked_before, locked_after, 
		 reference_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err = tx.Exec(ctx, journalQuery,
		journalEntry.ID, journalEntry.UserID, journalEntry.Currency,
		journalEntry.Type, journalEntry.Amount, journalEntry.BalanceBefore,
		journalEntry.BalanceAfter, journalEntry.AvailableBefore,
		journalEntry.AvailableAfter, journalEntry.LockedBefore,
		journalEntry.LockedAfter, journalEntry.ReferenceID,
		journalEntry.Status, journalEntry.CreatedAt,
	)

	if err != nil {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "error").Inc()
		return fmt.Errorf("failed to create journal entry: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		r.metrics.TransactionCount.WithLabelValues("update_balance", "commit_failed").Inc()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update cache
	r.cache.SetAccountBalance(ctx, update.UserID, update.Currency, newBalance, newAvailable, account.Version+1)

	// Invalidate related cache entries
	r.cache.InvalidateAccount(ctx, update.UserID, update.Currency)

	// After commit, queue async audit log (never blocks hot path)
	go func(entry AuditLogEntry) {
		select {
		case AuditLogQueue <- entry:
			// queued
		default:
			// audit log queue full, drop (metrics can be added)
		}
	}(AuditLogEntry{
		UserID:     update.UserID,
		Currency:   update.Currency,
		OldBalance: account.Balance,
		NewBalance: newBalance,
		Delta:      update.BalanceDelta,
		Timestamp:  time.Now().UnixNano(),
		TraceID:    getTraceID(),
	})

	r.metrics.TransactionCount.WithLabelValues("update_balance", "success").Inc()
	return nil
}

// CreateReservation creates a new fund reservation with atomic operations
func (r *Repository) CreateReservation(ctx context.Context, reservation *Reservation) error {
	start := time.Now()
	defer r.metrics.QueryDuration.WithLabelValues("create_reservation", "reservations").Observe(time.Since(start).Seconds())

	// Acquire distributed lock
	lockKey := fmt.Sprintf("account:lock:%s:%s", reservation.UserID.String(), reservation.Currency)
	mutex := r.redsync.NewMutex(lockKey, redsync.WithExpiry(LockTTL))

	if err := mutex.Lock(); err != nil {
		r.metrics.LockAcquisitions.WithLabelValues("reservation", "failed").Inc()
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock()

	r.metrics.LockAcquisitions.WithLabelValues("reservation", "success").Inc()

	// Start transaction
	tx, err := r.pgxPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update account (lock funds)
	balanceUpdate := &BalanceUpdate{
		UserID:       reservation.UserID,
		Currency:     reservation.Currency,
		BalanceDelta: decimal.Zero,
		LockedDelta:  reservation.Amount,
		Type:         "reservation_create",
		ReferenceID:  reservation.ID.String(),
	}

	if err := r.performBalanceUpdateInTx(ctx, tx, balanceUpdate); err != nil {
		r.metrics.TransactionCount.WithLabelValues("create_reservation", "error").Inc()
		return err
	}

	// Create reservation record
	query := `
		INSERT INTO reservations 
		(id, user_id, currency, amount, type, reference_id, status, expires_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = tx.Exec(ctx, query,
		reservation.ID, reservation.UserID, reservation.Currency,
		reservation.Amount, reservation.Type, reservation.ReferenceID,
		reservation.Status, reservation.ExpiresAt, reservation.Version,
		reservation.CreatedAt, reservation.UpdatedAt,
	)

	if err != nil {
		r.metrics.TransactionCount.WithLabelValues("create_reservation", "error").Inc()
		return fmt.Errorf("failed to create reservation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		r.metrics.TransactionCount.WithLabelValues("create_reservation", "commit_failed").Inc()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	r.cache.InvalidateAccount(ctx, reservation.UserID, reservation.Currency)

	r.metrics.TransactionCount.WithLabelValues("create_reservation", "success").Inc()
	return nil
}

// ReleaseReservation releases a fund reservation and unlocks funds
func (r *Repository) ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error {
	start := time.Now()
	defer r.metrics.QueryDuration.WithLabelValues("release_reservation", "reservations").Observe(time.Since(start).Seconds())

	// Get reservation details
	var reservation Reservation
	err := r.readDB.Where("id = ?", reservationID).First(&reservation).Error
	if err != nil {
		return fmt.Errorf("reservation not found: %w", err)
	}

	if reservation.Status != "active" {
		return fmt.Errorf("reservation is not active: %s", reservation.Status)
	}

	// Acquire distributed lock
	lockKey := fmt.Sprintf("account:lock:%s:%s", reservation.UserID.String(), reservation.Currency)
	mutex := r.redsync.NewMutex(lockKey, redsync.WithExpiry(LockTTL))

	if err := mutex.Lock(); err != nil {
		r.metrics.LockAcquisitions.WithLabelValues("reservation", "failed").Inc()
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock()

	r.metrics.LockAcquisitions.WithLabelValues("reservation", "success").Inc()

	// Start transaction
	tx, err := r.pgxPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update reservation status
	updateQuery := `
		UPDATE reservations 
		SET status = 'released', updated_at = NOW(), version = version + 1
		WHERE id = $1 AND status = 'active' AND version = $2
	`

	cmdTag, err := tx.Exec(ctx, updateQuery, reservationID, reservation.Version)
	if err != nil {
		return fmt.Errorf("failed to update reservation: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("reservation update failed: optimistic lock conflict")
	}

	// Update account (unlock funds)
	balanceUpdate := &BalanceUpdate{
		UserID:       reservation.UserID,
		Currency:     reservation.Currency,
		BalanceDelta: decimal.Zero,
		LockedDelta:  reservation.Amount.Neg(),
		Type:         "reservation_release",
		ReferenceID:  reservationID.String(),
	}

	if err := r.performBalanceUpdateInTx(ctx, tx, balanceUpdate); err != nil {
		r.metrics.TransactionCount.WithLabelValues("release_reservation", "error").Inc()
		return err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		r.metrics.TransactionCount.WithLabelValues("release_reservation", "commit_failed").Inc()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	r.cache.InvalidateAccount(ctx, reservation.UserID, reservation.Currency)

	r.metrics.TransactionCount.WithLabelValues("release_reservation", "success").Inc()
	return nil
}

// GetUserAccounts retrieves all accounts for a user with caching
func (r *Repository) GetUserAccounts(ctx context.Context, userID uuid.UUID) ([]*Account, error) {
	start := time.Now()
	defer r.metrics.QueryDuration.WithLabelValues("get_user_accounts", "accounts").Observe(time.Since(start).Seconds())

	partition := r.partitions.GetAccountPartition(userID)
	tableName := fmt.Sprintf("accounts_%s", partition)

	var accounts []*Account
	err := r.readDB.Table(tableName).
		Where("user_id = ?", userID).
		Find(&accounts).Error

	if err != nil {
		r.metrics.QueryCount.WithLabelValues("get_user_accounts", "accounts", "error").Inc()
		return nil, err
	}

	r.metrics.QueryCount.WithLabelValues("get_user_accounts", "accounts", "success").Inc()
	return accounts, nil
}

// Helper methods

func (r *Repository) performBalanceUpdateInTx(ctx context.Context, tx pgx.Tx, update *BalanceUpdate) error {
	// This is a simplified version - in a real implementation,
	// you would need to implement the full balance update logic within the transaction
	// Similar to performBalanceUpdate but using the existing transaction

	partition := r.partitions.GetAccountPartition(update.UserID)
	tableName := fmt.Sprintf("accounts_%s", partition)

	// Get current account
	var account Account
	query := fmt.Sprintf(`
		SELECT balance, available, locked, version
		FROM %s 
		WHERE user_id = $1 AND currency = $2 
		FOR UPDATE
	`, tableName)

	err := tx.QueryRow(ctx, query, update.UserID, update.Currency).Scan(
		&account.Balance, &account.Available, &account.Locked, &account.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Calculate new balances
	newBalance := account.Balance.Add(update.BalanceDelta)
	newLocked := account.Locked.Add(update.LockedDelta)
	newAvailable := newBalance.Sub(newLocked)

	// Validate constraints
	if newBalance.IsNegative() || newAvailable.IsNegative() || newLocked.IsNegative() {
		return fmt.Errorf("insufficient funds")
	}

	// Update account
	updateQuery := fmt.Sprintf(`
		UPDATE %s 
		SET balance = $1, available = $2, locked = $3, version = version + 1, updated_at = NOW()
		WHERE user_id = $4 AND currency = $5 AND version = $6
	`, tableName)

	cmdTag, err := tx.Exec(ctx, updateQuery, newBalance, newAvailable, newLocked,
		update.UserID, update.Currency, account.Version)

	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("optimistic lock failed")
	}

	return nil
}

func isOptimisticLockError(err error) bool {
	// Check if error is related to optimistic locking
	return err != nil && (err.Error() == "optimistic lock failed" ||
		err.Error() == "optimistic lock failed: no rows affected")
}

// Health checks repository connectivity
func (r *Repository) Health(ctx context.Context) error {
	// Check database connections
	sqlDB, err := r.writeDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get write DB: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("write DB unhealthy: %w", err)
	}

	sqlDB, err = r.readDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get read DB: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("read DB unhealthy: %w", err)
	}

	// Check pgx pool
	if err := r.pgxPool.Ping(ctx); err != nil {
		return fmt.Errorf("pgx pool unhealthy: %w", err)
	}

	// Check cache
	if err := r.cache.Health(ctx); err != nil {
		return fmt.Errorf("cache unhealthy: %w", err)
	}

	return nil
}
