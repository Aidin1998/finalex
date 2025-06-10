// Package balance provides a unified atomic balance movement service for trading operations
package balance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/Aidin1998/finalex/internal/trading/consistency"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BalanceService provides atomic balance movement operations with strong consistency guarantees
type BalanceService interface {
	// Credit adds funds to a user's account balance
	Credit(ctx context.Context, req *CreditRequest) (*CreditResponse, error)

	// Debit removes funds from a user's account balance
	Debit(ctx context.Context, req *DebitRequest) (*DebitResponse, error)

	// AtomicTransfer performs an atomic balance transfer between two accounts
	AtomicTransfer(ctx context.Context, req *AtomicTransferRequest) (*AtomicTransferResponse, error)

	// AtomicMultiAssetTransfer performs atomic transfers across multiple assets
	// This is essential for cross-pair and multi-leg trades
	AtomicMultiAssetTransfer(ctx context.Context, req *AtomicMultiAssetTransferRequest) (*AtomicMultiAssetTransferResponse, error)

	// LockFunds temporarily locks funds for pending operations
	LockFunds(ctx context.Context, req *LockFundsRequest) (*LockFundsResponse, error)

	// UnlockFunds releases previously locked funds
	UnlockFunds(ctx context.Context, req *UnlockFundsRequest) (*UnlockFundsResponse, error)

	// GetBalance retrieves current balance information
	GetBalance(ctx context.Context, userID, currency string) (*BalanceInfo, error)

	// ValidateTransaction validates a transaction before execution
	ValidateTransaction(ctx context.Context, req *ValidationRequest) (*ValidationResponse, error)

	// GetTransactionStatus retrieves the status of a balance operation
	GetTransactionStatus(ctx context.Context, transactionID string) (*TransactionStatus, error)

	// Health and metrics
	IsHealthy() bool
	GetMetrics() *ServiceMetrics
}

// CreditRequest represents a request to credit funds to an account
type CreditRequest struct {
	UserID      string                 `json:"user_id" validate:"required"`
	Currency    string                 `json:"currency" validate:"required"`
	Amount      decimal.Decimal        `json:"amount" validate:"required,gt=0"`
	Reference   string                 `json:"reference" validate:"required"`
	Description string                 `json:"description"`
	Idempotency string                 `json:"idempotency_key" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreditResponse represents the response to a credit operation
type CreditResponse struct {
	TransactionID string          `json:"transaction_id"`
	UserID        string          `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	NewBalance    decimal.Decimal `json:"new_balance"`
	Status        string          `json:"status"`
	Timestamp     time.Time       `json:"timestamp"`
}

// DebitRequest represents a request to debit funds from an account
type DebitRequest struct {
	UserID      string                 `json:"user_id" validate:"required"`
	Currency    string                 `json:"currency" validate:"required"`
	Amount      decimal.Decimal        `json:"amount" validate:"required,gt=0"`
	Reference   string                 `json:"reference" validate:"required"`
	Description string                 `json:"description"`
	Idempotency string                 `json:"idempotency_key" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// DebitResponse represents the response to a debit operation
type DebitResponse struct {
	TransactionID string          `json:"transaction_id"`
	UserID        string          `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	NewBalance    decimal.Decimal `json:"new_balance"`
	Status        string          `json:"status"`
	Timestamp     time.Time       `json:"timestamp"`
}

// AtomicTransferRequest represents a request for an atomic transfer between accounts
type AtomicTransferRequest struct {
	FromUserID  string                 `json:"from_user_id" validate:"required"`
	ToUserID    string                 `json:"to_user_id" validate:"required"`
	Currency    string                 `json:"currency" validate:"required"`
	Amount      decimal.Decimal        `json:"amount" validate:"required,gt=0"`
	Reference   string                 `json:"reference" validate:"required"`
	Description string                 `json:"description"`
	Idempotency string                 `json:"idempotency_key" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AtomicTransferResponse represents the response to an atomic transfer
type AtomicTransferResponse struct {
	TransactionID  string          `json:"transaction_id"`
	FromUserID     string          `json:"from_user_id"`
	ToUserID       string          `json:"to_user_id"`
	Currency       string          `json:"currency"`
	Amount         decimal.Decimal `json:"amount"`
	FromNewBalance decimal.Decimal `json:"from_new_balance"`
	ToNewBalance   decimal.Decimal `json:"to_new_balance"`
	Status         string          `json:"status"`
	Timestamp      time.Time       `json:"timestamp"`
}

// MultiAssetTransfer represents a single transfer in a multi-asset operation
type MultiAssetTransfer struct {
	FromUserID  string          `json:"from_user_id" validate:"required"`
	ToUserID    string          `json:"to_user_id" validate:"required"`
	Currency    string          `json:"currency" validate:"required"`
	Amount      decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Reference   string          `json:"reference"`
	Description string          `json:"description"`
}

// AtomicMultiAssetTransferRequest represents a request for atomic multi-asset transfers
type AtomicMultiAssetTransferRequest struct {
	Transfers   []MultiAssetTransfer   `json:"transfers" validate:"required,min=1"`
	Reference   string                 `json:"reference" validate:"required"`
	Description string                 `json:"description"`
	Idempotency string                 `json:"idempotency_key" validate:"required"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AtomicMultiAssetTransferResponse represents the response to a multi-asset transfer
type AtomicMultiAssetTransferResponse struct {
	TransactionID string           `json:"transaction_id"`
	Transfers     []TransferResult `json:"transfers"`
	Status        string           `json:"status"`
	Timestamp     time.Time        `json:"timestamp"`
	FailureReason string           `json:"failure_reason,omitempty"`
}

// TransferResult represents the result of a single transfer within a multi-asset operation
type TransferResult struct {
	FromUserID     string          `json:"from_user_id"`
	ToUserID       string          `json:"to_user_id"`
	Currency       string          `json:"currency"`
	Amount         decimal.Decimal `json:"amount"`
	FromNewBalance decimal.Decimal `json:"from_new_balance"`
	ToNewBalance   decimal.Decimal `json:"to_new_balance"`
	Status         string          `json:"status"`
}

// LockFundsRequest represents a request to lock funds
type LockFundsRequest struct {
	UserID      string          `json:"user_id" validate:"required"`
	Currency    string          `json:"currency" validate:"required"`
	Amount      decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Reference   string          `json:"reference" validate:"required"`
	Description string          `json:"description"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	Idempotency string          `json:"idempotency_key" validate:"required"`
}

// LockFundsResponse represents the response to a lock funds operation
type LockFundsResponse struct {
	LockID        string          `json:"lock_id"`
	UserID        string          `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	LockedBalance decimal.Decimal `json:"locked_balance"`
	Status        string          `json:"status"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
}

// UnlockFundsRequest represents a request to unlock funds
type UnlockFundsRequest struct {
	LockID      string          `json:"lock_id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	Currency    string          `json:"currency,omitempty"`
	Amount      decimal.Decimal `json:"amount,omitempty"`
	Reference   string          `json:"reference,omitempty"`
	Idempotency string          `json:"idempotency_key" validate:"required"`
}

// UnlockFundsResponse represents the response to an unlock funds operation
type UnlockFundsResponse struct {
	LockID           string          `json:"lock_id"`
	UserID           string          `json:"user_id"`
	Currency         string          `json:"currency"`
	Amount           decimal.Decimal `json:"amount"`
	AvailableBalance decimal.Decimal `json:"available_balance"`
	Status           string          `json:"status"`
	Timestamp        time.Time       `json:"timestamp"`
}

// BalanceInfo represents current balance information
type BalanceInfo struct {
	UserID    string          `json:"user_id"`
	Currency  string          `json:"currency"`
	Total     decimal.Decimal `json:"total"`
	Available decimal.Decimal `json:"available"`
	Locked    decimal.Decimal `json:"locked"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ValidationRequest represents a request to validate a transaction
type ValidationRequest struct {
	UserID    string          `json:"user_id" validate:"required"`
	Currency  string          `json:"currency" validate:"required"`
	Amount    decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Operation string          `json:"operation" validate:"required,oneof=credit debit transfer"`
	ToUserID  string          `json:"to_user_id,omitempty"`
}

// ValidationResponse represents the response to a validation request
type ValidationResponse struct {
	Valid       bool                   `json:"valid"`
	Errors      []string               `json:"errors,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
	Suggestions map[string]interface{} `json:"suggestions,omitempty"`
}

// TransactionStatus represents the status of a balance operation
type TransactionStatus struct {
	TransactionID string                 `json:"transaction_id"`
	Status        string                 `json:"status"`
	Type          string                 `json:"type"`
	Progress      decimal.Decimal        `json:"progress"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ServiceMetrics represents service performance metrics
type ServiceMetrics struct {
	TotalOperations      int64         `json:"total_operations"`
	SuccessfulOperations int64         `json:"successful_operations"`
	FailedOperations     int64         `json:"failed_operations"`
	AverageLatency       time.Duration `json:"average_latency"`
	ActiveTransactions   int64         `json:"active_transactions"`
	IdempotencyHits      int64         `json:"idempotency_hits"`
	ValidationFailures   int64         `json:"validation_failures"`
	LastResetAt          time.Time     `json:"last_reset_at"`
}

// BalanceServiceImpl implements the BalanceService interface
type BalanceServiceImpl struct {
	db             *gorm.DB
	bookkeeper     bookkeeper.BookkeeperService
	consistencyMgr *consistency.BalanceConsistencyManager
	logger         *zap.Logger

	// Idempotency management
	idempotencyCache sync.Map
	idempotencyTTL   time.Duration

	// Transaction tracking
	activeTransactions sync.Map
	metrics            *ServiceMetrics
	metricsLock        sync.RWMutex

	// Configuration
	config *ServiceConfig
}

// ServiceConfig contains configuration for the balance service
type ServiceConfig struct {
	MaxRetries         int             `json:"max_retries"`
	RetryBackoff       time.Duration   `json:"retry_backoff"`
	IdempotencyTTL     time.Duration   `json:"idempotency_ttl"`
	TransactionTimeout time.Duration   `json:"transaction_timeout"`
	EnableConsensus    bool            `json:"enable_consensus"`
	ConsensusThreshold decimal.Decimal `json:"consensus_threshold"`
	EnableAuditLogging bool            `json:"enable_audit_logging"`
	EnableValidation   bool            `json:"enable_validation"`
}

// DefaultServiceConfig returns a default configuration
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		MaxRetries:         3,
		RetryBackoff:       100 * time.Millisecond,
		IdempotencyTTL:     24 * time.Hour,
		TransactionTimeout: 30 * time.Second,
		EnableConsensus:    true,
		ConsensusThreshold: decimal.NewFromFloat(10000), // $10,000 threshold for consensus
		EnableAuditLogging: true,
		EnableValidation:   true,
	}
}

// NewBalanceService creates a new balance service instance
func NewBalanceService(
	db *gorm.DB,
	bookkeeper bookkeeper.BookkeeperService,
	consistencyMgr *consistency.BalanceConsistencyManager,
	logger *zap.Logger,
	config *ServiceConfig,
) BalanceService {
	if config == nil {
		config = DefaultServiceConfig()
	}

	return &BalanceServiceImpl{
		db:             db,
		bookkeeper:     bookkeeper,
		consistencyMgr: consistencyMgr,
		logger:         logger,
		config:         config,
		idempotencyTTL: config.IdempotencyTTL,
		metrics: &ServiceMetrics{
			LastResetAt: time.Now(),
		},
	}
}

// Credit implements BalanceService.Credit
func (bs *BalanceServiceImpl) Credit(ctx context.Context, req *CreditRequest) (*CreditResponse, error) {
	if err := bs.validateCreditRequest(req); err != nil {
		bs.incrementMetric("validation_failures")
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	if resp, exists := bs.checkIdempotency(req.Idempotency); exists {
		bs.incrementMetric("idempotency_hits")
		return resp.(*CreditResponse), nil
	}

	startTime := time.Now()
	transactionID := uuid.New().String()

	// Track active transaction
	bs.activeTransactions.Store(transactionID, &TransactionStatus{
		TransactionID: transactionID,
		Status:        "processing",
		Type:          "credit",
		Progress:      decimal.Zero,
		CreatedAt:     startTime,
		UpdatedAt:     startTime,
	})
	defer bs.activeTransactions.Delete(transactionID) // Create transaction record
	transaction := &models.Transaction{
		ID:          uuid.MustParse(transactionID),
		UserID:      uuid.MustParse(req.UserID),
		Type:        "credit",
		Amount:      req.Amount,
		Currency:    req.Currency,
		Reference:   req.Reference,
		Description: req.Description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	// Execute atomic credit operation
	err := bs.db.Transaction(func(tx *gorm.DB) error {
		// Create transaction record
		if err := tx.Create(transaction).Error; err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		// Update account balance
		result := tx.Model(&models.Account{}).
			Where("user_id = ? AND currency = ?", req.UserID, req.Currency).
			UpdateColumns(map[string]interface{}{
				"balance":    gorm.Expr("balance + ?", req.Amount.InexactFloat64()),
				"available":  gorm.Expr("available + ?", req.Amount.InexactFloat64()),
				"updated_at": time.Now(),
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update account balance: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("account not found for user %s and currency %s", req.UserID, req.Currency)
		}

		return nil
	})

	if err != nil {
		transaction.Status = "failed"
		bs.incrementMetric("failed_operations")
		return nil, err
	}

	// Update transaction status
	transaction.Status = "completed"
	completedAt := time.Now()
	transaction.CompletedAt = &completedAt
	bs.db.Save(transaction)

	// Get updated balance
	updatedAccount, _ := bs.bookkeeper.GetAccount(ctx, req.UserID, req.Currency)
	response := &CreditResponse{
		TransactionID: transactionID,
		UserID:        req.UserID,
		Currency:      req.Currency,
		Amount:        req.Amount,
		NewBalance:    updatedAccount.Balance,
		Status:        "completed",
		Timestamp:     completedAt,
	}

	// Store in idempotency cache
	bs.storeIdempotency(req.Idempotency, response)

	// Update metrics
	bs.incrementMetric("successful_operations")
	bs.updateLatency(time.Since(startTime))

	bs.logger.Info("Credit operation completed",
		zap.String("transaction_id", transactionID),
		zap.String("user_id", req.UserID),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()),
		zap.Duration("duration", time.Since(startTime)))

	return response, nil
}

// Debit implements BalanceService.Debit
func (bs *BalanceServiceImpl) Debit(ctx context.Context, req *DebitRequest) (*DebitResponse, error) {
	if err := bs.validateDebitRequest(req); err != nil {
		bs.incrementMetric("validation_failures")
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	if resp, exists := bs.checkIdempotency(req.Idempotency); exists {
		bs.incrementMetric("idempotency_hits")
		return resp.(*DebitResponse), nil
	}

	startTime := time.Now()
	transactionID := uuid.New().String()
	// Track active transaction
	bs.activeTransactions.Store(transactionID, &TransactionStatus{
		TransactionID: transactionID,
		Status:        "processing",
		Type:          "debit",
		Progress:      decimal.Zero,
		CreatedAt:     startTime,
		UpdatedAt:     startTime,
	})
	defer bs.activeTransactions.Delete(transactionID)

	// Check sufficient funds first
	account, err := bs.bookkeeper.GetAccount(ctx, req.UserID, req.Currency)
	if err != nil {
		bs.incrementMetric("failed_operations")
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	if account.Available.Cmp(req.Amount) < 0 {
		bs.incrementMetric("failed_operations")
		return nil, fmt.Errorf("insufficient funds: available=%s, required=%s",
			account.Available.String(), req.Amount.String())
	}
	// Create transaction record
	transaction := &models.Transaction{
		ID:          uuid.MustParse(transactionID),
		UserID:      uuid.MustParse(req.UserID),
		Type:        "debit",
		Amount:      req.Amount.Neg(),
		Currency:    req.Currency,
		Reference:   req.Reference,
		Description: req.Description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Execute atomic debit operation
	err = bs.db.Transaction(func(tx *gorm.DB) error {
		// Create transaction record
		if err := tx.Create(transaction).Error; err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		// Update account balance with row locking
		result := tx.Model(&models.Account{}).
			Where("user_id = ? AND currency = ? AND available >= ?", req.UserID, req.Currency, req.Amount.InexactFloat64()).
			UpdateColumns(map[string]interface{}{
				"balance":    gorm.Expr("balance - ?", req.Amount.InexactFloat64()),
				"available":  gorm.Expr("available - ?", req.Amount.InexactFloat64()),
				"updated_at": time.Now(),
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update account balance: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("insufficient funds or account not found")
		}

		return nil
	})

	if err != nil {
		transaction.Status = "failed"
		bs.incrementMetric("failed_operations")
		return nil, err
	}

	// Update transaction status
	transaction.Status = "completed"
	completedAt := time.Now()
	transaction.CompletedAt = &completedAt
	bs.db.Save(transaction)

	// Get updated balance
	updatedAccount, _ := bs.bookkeeper.GetAccount(ctx, req.UserID, req.Currency)
	response := &DebitResponse{
		TransactionID: transactionID,
		UserID:        req.UserID,
		Currency:      req.Currency,
		Amount:        req.Amount,
		NewBalance:    updatedAccount.Balance,
		Status:        "completed",
		Timestamp:     completedAt,
	}

	// Store in idempotency cache
	bs.storeIdempotency(req.Idempotency, response)

	// Update metrics
	bs.incrementMetric("successful_operations")
	bs.updateLatency(time.Since(startTime))

	bs.logger.Info("Debit operation completed",
		zap.String("transaction_id", transactionID),
		zap.String("user_id", req.UserID),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()),
		zap.Duration("duration", time.Since(startTime)))

	return response, nil
}

// AtomicTransfer implements BalanceService.AtomicTransfer
func (bs *BalanceServiceImpl) AtomicTransfer(ctx context.Context, req *AtomicTransferRequest) (*AtomicTransferResponse, error) {
	if err := bs.validateAtomicTransferRequest(req); err != nil {
		bs.incrementMetric("validation_failures")
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	if resp, exists := bs.checkIdempotency(req.Idempotency); exists {
		bs.incrementMetric("idempotency_hits")
		return resp.(*AtomicTransferResponse), nil
	}

	startTime := time.Now()

	// Use existing balance consistency manager for atomic transfer
	err := bs.consistencyMgr.AtomicTransfer(
		ctx,
		req.FromUserID,
		req.ToUserID,
		req.Currency,
		req.Amount,
		req.Reference,
		req.Description,
	)

	if err != nil {
		bs.incrementMetric("failed_operations")
		return nil, fmt.Errorf("atomic transfer failed: %w", err)
	}

	// Get updated balances
	fromAccount, _ := bs.bookkeeper.GetAccount(ctx, req.FromUserID, req.Currency)
	toAccount, _ := bs.bookkeeper.GetAccount(ctx, req.ToUserID, req.Currency)
	response := &AtomicTransferResponse{
		TransactionID:  uuid.New().String(),
		FromUserID:     req.FromUserID,
		ToUserID:       req.ToUserID,
		Currency:       req.Currency,
		Amount:         req.Amount,
		FromNewBalance: fromAccount.Balance,
		ToNewBalance:   toAccount.Balance,
		Status:         "completed",
		Timestamp:      time.Now(),
	}

	// Store in idempotency cache
	bs.storeIdempotency(req.Idempotency, response)

	// Update metrics
	bs.incrementMetric("successful_operations")
	bs.updateLatency(time.Since(startTime))

	bs.logger.Info("Atomic transfer completed",
		zap.String("from_user_id", req.FromUserID),
		zap.String("to_user_id", req.ToUserID),
		zap.String("currency", req.Currency),
		zap.String("amount", req.Amount.String()),
		zap.Duration("duration", time.Since(startTime)))

	return response, nil
}

// AtomicMultiAssetTransfer implements BalanceService.AtomicMultiAssetTransfer
func (bs *BalanceServiceImpl) AtomicMultiAssetTransfer(ctx context.Context, req *AtomicMultiAssetTransferRequest) (*AtomicMultiAssetTransferResponse, error) {
	if err := bs.validateAtomicMultiAssetTransferRequest(req); err != nil {
		bs.incrementMetric("validation_failures")
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	if resp, exists := bs.checkIdempotency(req.Idempotency); exists {
		bs.incrementMetric("idempotency_hits")
		return resp.(*AtomicMultiAssetTransferResponse), nil
	}

	startTime := time.Now()
	transactionID := uuid.New().String()

	// Track active transaction
	bs.activeTransactions.Store(transactionID, &TransactionStatus{
		TransactionID: transactionID,
		Status:        "processing",
		Type:          "multi_asset_transfer",
		Progress:      decimal.Zero,
		CreatedAt:     startTime,
		UpdatedAt:     startTime,
	})
	defer bs.activeTransactions.Delete(transactionID)

	// Pre-validate all transfers before execution
	if err := bs.preValidateTransfers(ctx, req.Transfers); err != nil {
		bs.incrementMetric("failed_operations")
		return &AtomicMultiAssetTransferResponse{
			TransactionID: transactionID,
			Status:        "failed",
			FailureReason: err.Error(),
			Timestamp:     time.Now(),
		}, err
	}

	// Execute all transfers atomically in a database transaction
	var transferResults []TransferResult
	err := bs.db.Transaction(func(tx *gorm.DB) error {
		for i, transfer := range req.Transfers {
			// Update progress
			progress := decimal.NewFromFloat(float64(i) / float64(len(req.Transfers)))
			if status, exists := bs.activeTransactions.Load(transactionID); exists {
				status.(*TransactionStatus).Progress = progress
				status.(*TransactionStatus).UpdatedAt = time.Now()
			}

			// Execute single transfer within the transaction
			result, err := bs.executeSingleTransferInTx(ctx, tx, transfer, fmt.Sprintf("%s_transfer_%d", transactionID, i))
			if err != nil {
				return fmt.Errorf("transfer %d failed: %w", i, err)
			}
			transferResults = append(transferResults, *result)
		}
		return nil
	})

	if err != nil {
		bs.incrementMetric("failed_operations")
		response := &AtomicMultiAssetTransferResponse{
			TransactionID: transactionID,
			Status:        "failed",
			FailureReason: err.Error(),
			Timestamp:     time.Now(),
		}

		// Store failed response in idempotency cache to prevent retries
		bs.storeIdempotency(req.Idempotency, response)
		return response, err
	}

	// All transfers completed successfully
	response := &AtomicMultiAssetTransferResponse{
		TransactionID: transactionID,
		Transfers:     transferResults,
		Status:        "completed",
		Timestamp:     time.Now(),
	}

	// Store successful response in idempotency cache
	bs.storeIdempotency(req.Idempotency, response)

	// Update metrics
	bs.incrementMetric("successful_operations")
	bs.updateLatency(time.Since(startTime))

	bs.logger.Info("Multi-asset transfer completed",
		zap.String("transaction_id", transactionID),
		zap.Int("transfer_count", len(req.Transfers)),
		zap.String("reference", req.Reference),
		zap.Duration("duration", time.Since(startTime)))

	return response, nil
}

// preValidateTransfers validates all transfers before execution
func (bs *BalanceServiceImpl) preValidateTransfers(ctx context.Context, transfers []MultiAssetTransfer) error {
	// Check for sufficient funds and validate all transfers
	for i, transfer := range transfers {
		// Get account balance
		account, err := bs.bookkeeper.GetAccount(ctx, transfer.FromUserID, transfer.Currency)
		if err != nil {
			return fmt.Errorf("transfer %d: failed to get account for user %s: %w", i, transfer.FromUserID, err)
		}

		// Check sufficient funds
		if account.Available.Cmp(transfer.Amount) < 0 {
			return fmt.Errorf("transfer %d: insufficient funds for user %s in %s: available=%s, required=%s",
				i, transfer.FromUserID, transfer.Currency, account.Available.String(), transfer.Amount.String())
		}

		// Validate user IDs are different
		if transfer.FromUserID == transfer.ToUserID {
			return fmt.Errorf("transfer %d: from_user_id and to_user_id cannot be the same", i)
		}
	}

	// Check for potential deadlocks by analyzing transfer patterns
	if err := bs.validateTransferOrdering(transfers); err != nil {
		return fmt.Errorf("transfer ordering validation failed: %w", err)
	}

	return nil
}

// validateTransferOrdering ensures transfers are ordered to prevent deadlocks
func (bs *BalanceServiceImpl) validateTransferOrdering(transfers []MultiAssetTransfer) error {
	// Create a map to track user interactions
	userInteractions := make(map[string]map[string]bool)

	for i, transfer := range transfers {
		from := transfer.FromUserID
		to := transfer.ToUserID

		// Initialize maps if needed
		if userInteractions[from] == nil {
			userInteractions[from] = make(map[string]bool)
		}
		if userInteractions[to] == nil {
			userInteractions[to] = make(map[string]bool)
		}

		// Check for circular dependencies
		if userInteractions[to][from] {
			return fmt.Errorf("circular dependency detected between users %s and %s at transfer %d", from, to, i)
		}

		userInteractions[from][to] = true
	}

	return nil
}

// executeSingleTransferInTx executes a single transfer within a database transaction
func (bs *BalanceServiceImpl) executeSingleTransferInTx(ctx context.Context, tx *gorm.DB, transfer MultiAssetTransfer, operationRef string) (*TransferResult, error) {
	// Lock accounts in deterministic order to prevent deadlocks
	var fromAccount, toAccount models.Account

	if transfer.FromUserID < transfer.ToUserID {
		// Lock from account first
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND currency = ?", transfer.FromUserID, transfer.Currency).
			First(&fromAccount).Error; err != nil {
			return nil, fmt.Errorf("failed to lock from account: %w", err)
		}

		// Lock to account second
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND currency = ?", transfer.ToUserID, transfer.Currency).
			First(&toAccount).Error; err != nil {
			return nil, fmt.Errorf("failed to lock to account: %w", err)
		}
	} else {
		// Lock to account first
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND currency = ?", transfer.ToUserID, transfer.Currency).
			First(&toAccount).Error; err != nil {
			return nil, fmt.Errorf("failed to lock to account: %w", err)
		}

		// Lock from account second
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND currency = ?", transfer.FromUserID, transfer.Currency).
			First(&fromAccount).Error; err != nil {
			return nil, fmt.Errorf("failed to lock from account: %w", err)
		}
	}

	// Verify sufficient funds again within the transaction
	if fromAccount.Available.Cmp(transfer.Amount) < 0 {
		return nil, fmt.Errorf("insufficient funds: available=%s, required=%s",
			fromAccount.Available.String(), transfer.Amount.String())
	}

	// Update balances atomically
	err := tx.Model(&models.Account{}).
		Where("user_id = ? AND currency = ?", transfer.FromUserID, transfer.Currency).
		UpdateColumns(map[string]interface{}{
			"balance":    gorm.Expr("balance - ?", transfer.Amount.InexactFloat64()),
			"available":  gorm.Expr("available - ?", transfer.Amount.InexactFloat64()),
			"updated_at": time.Now(),
		}).Error
	if err != nil {
		return nil, fmt.Errorf("failed to debit from account: %w", err)
	}

	err = tx.Model(&models.Account{}).
		Where("user_id = ? AND currency = ?", transfer.ToUserID, transfer.Currency).
		UpdateColumns(map[string]interface{}{
			"balance":    gorm.Expr("balance + ?", transfer.Amount.InexactFloat64()),
			"available":  gorm.Expr("available + ?", transfer.Amount.InexactFloat64()),
			"updated_at": time.Now(),
		}).Error
	if err != nil {
		return nil, fmt.Errorf("failed to credit to account: %w", err)
	}

	// Create transaction records
	fromUserUUID, err := uuid.Parse(transfer.FromUserID)
	if err != nil {
		return nil, fmt.Errorf("invalid from user ID: %w", err)
	}

	toUserUUID, err := uuid.Parse(transfer.ToUserID)
	if err != nil {
		return nil, fmt.Errorf("invalid to user ID: %w", err)
	}

	now := time.Now()
	fromTx := &models.Transaction{
		ID:          uuid.New(),
		UserID:      fromUserUUID,
		Type:        "multi_transfer_out",
		Amount:      transfer.Amount.Neg(),
		Currency:    transfer.Currency,
		Reference:   operationRef,
		Description: transfer.Description,
		Status:      "completed",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	toTx := &models.Transaction{
		ID:          uuid.New(),
		UserID:      toUserUUID,
		Type:        "multi_transfer_in",
		Amount:      transfer.Amount,
		Currency:    transfer.Currency,
		Reference:   operationRef,
		Description: transfer.Description,
		Status:      "completed",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := tx.Create(fromTx).Error; err != nil {
		return nil, fmt.Errorf("failed to create from transaction: %w", err)
	}

	if err := tx.Create(toTx).Error; err != nil {
		return nil, fmt.Errorf("failed to create to transaction: %w", err)
	}
	// Calculate new balances for response
	fromNewBalance := fromAccount.Balance.Sub(transfer.Amount)
	toNewBalance := toAccount.Balance.Add(transfer.Amount)
	return &TransferResult{
		FromUserID:     transfer.FromUserID,
		ToUserID:       transfer.ToUserID,
		Currency:       transfer.Currency,
		Amount:         transfer.Amount,
		FromNewBalance: fromNewBalance,
		ToNewBalance:   toNewBalance,
		Status:         "completed",
	}, nil
}

// validateAtomicMultiAssetTransferRequest validates a multi-asset transfer request
func (bs *BalanceServiceImpl) validateAtomicMultiAssetTransferRequest(req *AtomicMultiAssetTransferRequest) error {
	if len(req.Transfers) == 0 {
		return fmt.Errorf("transfers list cannot be empty")
	}

	if len(req.Transfers) > 100 { // Reasonable limit to prevent abuse
		return fmt.Errorf("too many transfers: maximum 100 allowed, got %d", len(req.Transfers))
	}

	if req.Reference == "" {
		return fmt.Errorf("reference is required")
	}

	if req.Idempotency == "" {
		return fmt.Errorf("idempotency_key is required")
	}

	// Validate individual transfers
	for i, transfer := range req.Transfers {
		if transfer.FromUserID == "" {
			return fmt.Errorf("transfer %d: from_user_id is required", i)
		}
		if transfer.ToUserID == "" {
			return fmt.Errorf("transfer %d: to_user_id is required", i)
		}
		if transfer.FromUserID == transfer.ToUserID {
			return fmt.Errorf("transfer %d: from_user_id and to_user_id cannot be the same", i)
		}
		if transfer.Currency == "" {
			return fmt.Errorf("transfer %d: currency is required", i)
		}
		if transfer.Amount.IsZero() || transfer.Amount.IsNegative() {
			return fmt.Errorf("transfer %d: amount must be positive", i)
		}
	}

	return nil
}

// validateCreditRequest validates a credit request
func (bs *BalanceServiceImpl) validateCreditRequest(req *CreditRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return fmt.Errorf("amount must be positive")
	}
	if req.Reference == "" {
		return fmt.Errorf("reference is required")
	}
	if req.Idempotency == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	return nil
}

// validateDebitRequest validates a debit request
func (bs *BalanceServiceImpl) validateDebitRequest(req *DebitRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return fmt.Errorf("amount must be positive")
	}
	if req.Reference == "" {
		return fmt.Errorf("reference is required")
	}
	if req.Idempotency == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	return nil
}

// validateAtomicTransferRequest validates an atomic transfer request
func (bs *BalanceServiceImpl) validateAtomicTransferRequest(req *AtomicTransferRequest) error {
	if req.FromUserID == "" {
		return fmt.Errorf("from_user_id is required")
	}
	if req.ToUserID == "" {
		return fmt.Errorf("to_user_id is required")
	}
	if req.FromUserID == req.ToUserID {
		return fmt.Errorf("from_user_id and to_user_id cannot be the same")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return fmt.Errorf("amount must be positive")
	}
	if req.Reference == "" {
		return fmt.Errorf("reference is required")
	}
	if req.Idempotency == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	return nil
}

// checkIdempotency checks if an operation has already been performed
func (bs *BalanceServiceImpl) checkIdempotency(key string) (interface{}, bool) {
	if entry, exists := bs.idempotencyCache.Load(key); exists {
		if cached, ok := entry.(*idempotencyEntry); ok {
			if time.Since(cached.Timestamp) < bs.idempotencyTTL {
				return cached.Response, true
			}
			// Entry expired, remove it
			bs.idempotencyCache.Delete(key)
		}
	}
	return nil, false
}

// storeIdempotency stores a response for idempotency checking
func (bs *BalanceServiceImpl) storeIdempotency(key string, response interface{}) {
	entry := &idempotencyEntry{
		Response:  response,
		Timestamp: time.Now(),
	}
	bs.idempotencyCache.Store(key, entry)
}

// idempotencyEntry represents a cached idempotency entry
type idempotencyEntry struct {
	Response  interface{}
	Timestamp time.Time
}

// incrementMetric increments a metric counter
func (bs *BalanceServiceImpl) incrementMetric(metric string) {
	bs.metricsLock.Lock()
	defer bs.metricsLock.Unlock()

	switch metric {
	case "successful_operations":
		bs.metrics.SuccessfulOperations++
	case "failed_operations":
		bs.metrics.FailedOperations++
	case "validation_failures":
		bs.metrics.ValidationFailures++
	case "idempotency_hits":
		bs.metrics.IdempotencyHits++
	}
	bs.metrics.TotalOperations++
}

// updateLatency updates the average latency metric
func (bs *BalanceServiceImpl) updateLatency(duration time.Duration) {
	bs.metricsLock.Lock()
	defer bs.metricsLock.Unlock()

	if bs.metrics.AverageLatency == 0 {
		bs.metrics.AverageLatency = duration
	} else {
		bs.metrics.AverageLatency = (bs.metrics.AverageLatency + duration) / 2
	}
}

// GetBalance implements BalanceService.GetBalance
func (bs *BalanceServiceImpl) GetBalance(ctx context.Context, userID, currency string) (*BalanceInfo, error) {
	account, err := bs.bookkeeper.GetAccount(ctx, userID, currency)
	if err != nil {
		return nil, err
	}
	return &BalanceInfo{
		UserID:    userID,
		Currency:  currency,
		Total:     account.Balance,
		Available: account.Available,
		Locked:    account.Locked,
		UpdatedAt: account.UpdatedAt,
	}, nil
}

// IsHealthy implements BalanceService.IsHealthy
func (bs *BalanceServiceImpl) IsHealthy() bool {
	return bs.consistencyMgr.IsHealthy()
}

// GetMetrics implements BalanceService.GetMetrics
func (bs *BalanceServiceImpl) GetMetrics() *ServiceMetrics {
	bs.metricsLock.RLock()
	defer bs.metricsLock.RUnlock()

	// Count active transactions
	var activeCount int64
	bs.activeTransactions.Range(func(_, _ interface{}) bool {
		activeCount++
		return true
	})
	bs.metrics.ActiveTransactions = activeCount

	// Return a copy of metrics
	return &ServiceMetrics{
		TotalOperations:      bs.metrics.TotalOperations,
		SuccessfulOperations: bs.metrics.SuccessfulOperations,
		FailedOperations:     bs.metrics.FailedOperations,
		AverageLatency:       bs.metrics.AverageLatency,
		ActiveTransactions:   bs.metrics.ActiveTransactions,
		IdempotencyHits:      bs.metrics.IdempotencyHits,
		ValidationFailures:   bs.metrics.ValidationFailures,
		LastResetAt:          bs.metrics.LastResetAt,
	}
}

// ValidateTransaction implements BalanceService.ValidateTransaction
func (bs *BalanceServiceImpl) ValidateTransaction(ctx context.Context, req *ValidationRequest) (*ValidationResponse, error) {
	response := &ValidationResponse{
		Valid:       true,
		Errors:      []string{},
		Warnings:    []string{},
		Suggestions: make(map[string]interface{}),
	}

	// Basic validation
	if req.UserID == "" {
		response.Valid = false
		response.Errors = append(response.Errors, "user_id is required")
	}

	if req.Currency == "" {
		response.Valid = false
		response.Errors = append(response.Errors, "currency is required")
	}

	if req.Amount.IsZero() || req.Amount.IsNegative() {
		response.Valid = false
		response.Errors = append(response.Errors, "amount must be positive")
	}

	// Operation-specific validation
	switch req.Operation {
	case "debit", "transfer":
		// Check sufficient funds
		account, err := bs.bookkeeper.GetAccount(ctx, req.UserID, req.Currency)
		if err != nil {
			response.Valid = false
			response.Errors = append(response.Errors, "failed to retrieve account information")
		} else if account.Available.Cmp(req.Amount) < 0 {
			response.Valid = false
			response.Errors = append(response.Errors, fmt.Sprintf("insufficient funds: available=%s, required=%s",
				account.Available.String(), req.Amount.String()))
		}

		if req.Operation == "transfer" && req.ToUserID == "" {
			response.Valid = false
			response.Errors = append(response.Errors, "to_user_id is required for transfer operations")
		}
	}

	return response, nil
}

// GetTransactionStatus implements BalanceService.GetTransactionStatus
func (bs *BalanceServiceImpl) GetTransactionStatus(ctx context.Context, transactionID string) (*TransactionStatus, error) {
	// Check active transactions first
	if status, exists := bs.activeTransactions.Load(transactionID); exists {
		return status.(*TransactionStatus), nil
	}

	// Check database for completed transactions
	var transaction models.Transaction
	err := bs.db.Where("id = ?", transactionID).First(&transaction).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("transaction not found")
		}
		return nil, fmt.Errorf("failed to retrieve transaction: %w", err)
	}

	status := &TransactionStatus{
		TransactionID: transactionID,
		Status:        transaction.Status,
		Type:          transaction.Type,
		Progress:      decimal.NewFromFloat(100), // Completed transactions are 100%
		CreatedAt:     transaction.CreatedAt,
		UpdatedAt:     transaction.UpdatedAt,
		CompletedAt:   transaction.CompletedAt,
	}

	return status, nil
}

// LockFunds implements BalanceService.LockFunds
func (bs *BalanceServiceImpl) LockFunds(ctx context.Context, req *LockFundsRequest) (*LockFundsResponse, error) {
	if err := bs.validateLockFundsRequest(req); err != nil {
		bs.incrementMetric("validation_failures")
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	if resp, exists := bs.checkIdempotency(req.Idempotency); exists {
		bs.incrementMetric("idempotency_hits")
		return resp.(*LockFundsResponse), nil
	}

	startTime := time.Now()

	// Use existing balance consistency manager for lock operation
	err := bs.consistencyMgr.AtomicLockFunds(
		ctx,
		req.UserID,
		req.Currency,
		req.Amount,
		req.Reference,
	)

	if err != nil {
		bs.incrementMetric("failed_operations")
		return nil, fmt.Errorf("lock funds failed: %w", err)
	}

	// Get updated balance
	account, _ := bs.bookkeeper.GetAccount(ctx, req.UserID, req.Currency)
	response := &LockFundsResponse{
		LockID:        uuid.New().String(),
		UserID:        req.UserID,
		Currency:      req.Currency,
		Amount:        req.Amount,
		LockedBalance: account.Locked,
		Status:        "completed",
		ExpiresAt:     req.ExpiresAt,
		Timestamp:     time.Now(),
	}

	// Store in idempotency cache
	bs.storeIdempotency(req.Idempotency, response)

	// Update metrics
	bs.incrementMetric("successful_operations")
	bs.updateLatency(time.Since(startTime))

	return response, nil
}

// UnlockFunds implements BalanceService.UnlockFunds
func (bs *BalanceServiceImpl) UnlockFunds(ctx context.Context, req *UnlockFundsRequest) (*UnlockFundsResponse, error) {
	if err := bs.validateUnlockFundsRequest(req); err != nil {
		bs.incrementMetric("validation_failures")
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	if resp, exists := bs.checkIdempotency(req.Idempotency); exists {
		bs.incrementMetric("idempotency_hits")
		return resp.(*UnlockFundsResponse), nil
	}

	startTime := time.Now()

	// Use existing balance consistency manager for unlock operation
	err := bs.consistencyMgr.AtomicUnlockFunds(
		ctx,
		req.UserID,
		req.Currency,
		req.Amount,
		req.Reference,
	)

	if err != nil {
		bs.incrementMetric("failed_operations")
		return nil, fmt.Errorf("unlock funds failed: %w", err)
	}

	// Get updated balance
	account, _ := bs.bookkeeper.GetAccount(ctx, req.UserID, req.Currency)
	response := &UnlockFundsResponse{
		LockID:           req.LockID,
		UserID:           req.UserID,
		Currency:         req.Currency,
		Amount:           req.Amount,
		AvailableBalance: account.Available,
		Status:           "completed",
		Timestamp:        time.Now(),
	}

	// Store in idempotency cache
	bs.storeIdempotency(req.Idempotency, response)

	// Update metrics
	bs.incrementMetric("successful_operations")
	bs.updateLatency(time.Since(startTime))

	return response, nil
}

// validateLockFundsRequest validates a lock funds request
func (bs *BalanceServiceImpl) validateLockFundsRequest(req *LockFundsRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return fmt.Errorf("amount must be positive")
	}
	if req.Reference == "" {
		return fmt.Errorf("reference is required")
	}
	if req.Idempotency == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	return nil
}

// validateUnlockFundsRequest validates an unlock funds request
func (bs *BalanceServiceImpl) validateUnlockFundsRequest(req *UnlockFundsRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return fmt.Errorf("amount must be positive")
	}
	if req.Idempotency == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	return nil
}
