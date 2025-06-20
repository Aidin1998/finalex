package bookkeeper

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Enhanced error types for better error handling
var (
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrAccountNotFound     = errors.New("account not found")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrTransactionTimeout  = errors.New("transaction timeout")
	ErrDeadlockDetected    = errors.New("deadlock detected")
	ErrConcurrencyConflict = errors.New("concurrency conflict")
)

// BatchOperationResult holds the result of a batch operation
type BatchOperationResult struct {
	SuccessCount int
	FailedItems  map[string]error
	Duration     time.Duration
}

// AccountBalance represents account balance for batch operations
type AccountBalance struct {
	UserID    string
	Currency  string
	Balance   decimal.Decimal
	Available decimal.Decimal
	Locked    decimal.Decimal
}

// FundsOperation represents a funds lock/unlock operation
type FundsOperation struct {
	UserID   string
	Currency string
	Amount   decimal.Decimal
	OrderID  string
	Reason   string
}

// TransactionOptions defines options for enhanced transaction handling
type TransactionOptions struct {
	Timeout             time.Duration
	MaxRetries          int
	RetryBackoff        time.Duration
	RequireRowLocking   bool
	PreValidationChecks bool
	AuditLogging        bool
	DeadlockDetection   bool
}

// DefaultTransactionOptions returns default transaction options
func DefaultTransactionOptions() *TransactionOptions {
	return &TransactionOptions{
		Timeout:             30 * time.Second,
		MaxRetries:          3,
		RetryBackoff:        100 * time.Millisecond,
		RequireRowLocking:   true,
		PreValidationChecks: true,
		AuditLogging:        true,
		DeadlockDetection:   true,
	}
}

// BalanceReservation represents a balance reservation for trading operations
type BalanceReservation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Asset     string    `json:"asset"`
	Amount    float64   `json:"amount"`
	Purpose   string    `json:"purpose"`
	OrderID   string    `json:"order_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// TradeProcessRequest represents a request to process a trade
type TradeProcessRequest struct {
	TradeID      string    `json:"trade_id"`
	BuyerID      string    `json:"buyer_id"`
	SellerID     string    `json:"seller_id"`
	Symbol       string    `json:"symbol"`
	Quantity     float64   `json:"quantity"`
	Price        float64   `json:"price"`
	BuyOrderID   string    `json:"buy_order_id"`
	SellOrderID  string    `json:"sell_order_id"`
	ExecutedAt   time.Time `json:"executed_at"`
	TakerSide    string    `json:"taker_side"` // "buy" or "sell"
	MakerFeeRate float64   `json:"maker_fee_rate"`
	TakerFeeRate float64   `json:"taker_fee_rate"`
}

// BookkeeperService defines bookkeeping operations and supports transaction and account lifecycle
type BookkeeperService interface {
	Start() error
	Stop() error
	GetAccounts(ctx context.Context, userID string) ([]*models.Account, error)
	GetAccount(ctx context.Context, userID, currency string) (*models.Account, error)
	CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error)
	GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error)
	CreateTransaction(ctx context.Context, userID, transactionType string, amount decimal.Decimal, currency, reference, description string) (*models.Transaction, error)
	CompleteTransaction(ctx context.Context, transactionID string) error
	FailTransaction(ctx context.Context, transactionID string) error
	LockFunds(ctx context.Context, userID, currency string, amount decimal.Decimal) error
	UnlockFunds(ctx context.Context, userID, currency string, amount decimal.Decimal) error
	// Batch operations for N+1 query resolution
	BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error)
	BatchLockFunds(ctx context.Context, operations []FundsOperation) (*BatchOperationResult, error)
	BatchUnlockFunds(ctx context.Context, operations []FundsOperation) (*BatchOperationResult, error)
}

// Service implements BookkeeperService and BookkeeperXAService
type Service struct {
	logger    *zap.Logger
	db        *gorm.DB
	muMap     map[string]*sync.Mutex
	muMapLock sync.Mutex // protects muMap

	// XA transaction management
	pendingXATransactions map[string]*XATransaction
	xaMutex               sync.RWMutex
}

// NewService creates a new BookkeeperService
func NewService(logger *zap.Logger, db *gorm.DB) (BookkeeperService, error) {
	// Create service
	svc := &Service{
		logger:                logger,
		db:                    db,
		muMap:                 make(map[string]*sync.Mutex),
		pendingXATransactions: make(map[string]*XATransaction),
	}

	return svc, nil
}

// Start starts the bookkeeper service
func (s *Service) Start() error {
	s.logger.Info("Bookkeeper service started")
	return nil
}

// Stop stops the bookkeeper service
func (s *Service) Stop() error {
	s.logger.Info("Bookkeeper service stopped")
	return nil
}

// GetAccounts gets all accounts for a user
func (s *Service) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	// Find accounts
	var accounts []*models.Account
	if err := s.db.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("failed to find accounts: %w", err)
	}

	return accounts, nil
}

// GetAccount gets an account for a user
func (s *Service) GetAccount(ctx context.Context, userID string, currency string) (*models.Account, error) {
	// Find account
	var account models.Account
	if err := s.db.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("failed to find account: %w", err)
	}

	return &account, nil
}

// CreateAccount creates an account for a user
func (s *Service) CreateAccount(ctx context.Context, userID string, currency string) (*models.Account, error) {
	// Check if account already exists
	var count int64
	if err := s.db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userID, currency).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to check account: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("account already exists")
	}

	// Create account
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid userID: %w", err)
	}
	account := &models.Account{
		ID:        uuid.New(),
		UserID:    parsedUserID,
		Currency:  currency,
		Balance:   decimal.Zero,
		Available: decimal.Zero,
		Locked:    decimal.Zero,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save account to database
	if err := s.db.Create(account).Error; err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}

// GetAccountTransactions gets transactions for an account
func (s *Service) GetAccountTransactions(ctx context.Context, userID string, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	// Find account
	var account models.Account
	if err := s.db.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, fmt.Errorf("account not found")
		}
		return nil, 0, fmt.Errorf("failed to find account: %w", err)
	}

	// Count transactions
	var count int64
	if err := s.db.Model(&models.Transaction{}).Where("user_id = ? AND currency = ?", userID, currency).Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	// Find transactions
	var transactions []*models.Transaction
	if err := s.db.Where("user_id = ? AND currency = ?", userID, currency).Order("created_at DESC").Limit(limit).Offset(offset).Find(&transactions).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to find transactions: %w", err)
	}

	return transactions, count, nil
}

// CreateTransaction creates a transaction
func (s *Service) CreateTransaction(ctx context.Context, userID string, transactionType string, amount decimal.Decimal, currency string, reference, description string) (*models.Transaction, error) {
	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Find account
	var account models.Account
	if err := tx.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("failed to find account: %w", err)
	}

	// Create transaction
	now := time.Now()
	transaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      account.UserID,
		Type:        transactionType,
		Amount:      amount,
		Currency:    currency,
		Status:      "pending",
		Reference:   reference,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Save transaction to database
	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return transaction, nil
}

// CompleteTransaction completes a transaction
func (s *Service) CompleteTransaction(ctx context.Context, transactionID string) error {
	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Find transaction
	var transaction models.Transaction
	if err := tx.Where("id = ?", transactionID).First(&transaction).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("transaction not found")
		}
		return fmt.Errorf("failed to find transaction: %w", err)
	}

	// Check if transaction is already completed
	if transaction.Status == "completed" {
		tx.Rollback()
		return fmt.Errorf("transaction already completed")
	}

	// Find account
	var account models.Account
	if err := tx.Where("user_id = ? AND currency = ?", transaction.UserID, transaction.Currency).First(&account).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Update account balance
	if transaction.Type == "deposit" {
		account.Balance = account.Balance.Add(transaction.Amount)
		account.Available = account.Available.Add(transaction.Amount)
	} else if transaction.Type == "withdrawal" {
		if account.Available.Cmp(transaction.Amount) < 0 {
			tx.Rollback()
			return fmt.Errorf("insufficient funds")
		}
		account.Balance = account.Balance.Sub(transaction.Amount)
		account.Available = account.Available.Sub(transaction.Amount)
	}
	account.UpdatedAt = time.Now()

	// Save account to database
	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save account: %w", err)
	}

	// Update transaction status
	now := time.Now()
	transaction.Status = "completed"
	transaction.UpdatedAt = now
	transaction.CompletedAt = &now

	// Save transaction to database
	if err := tx.Save(&transaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FailTransaction fails a transaction
func (s *Service) FailTransaction(ctx context.Context, transactionID string) error {
	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Find transaction
	var transaction models.Transaction
	if err := tx.Where("id = ?", transactionID).First(&transaction).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("transaction not found")
		}
		return fmt.Errorf("failed to find transaction: %w", err)
	}

	// Check if transaction is already completed or failed
	if transaction.Status == "completed" || transaction.Status == "failed" {
		tx.Rollback()
		return fmt.Errorf("transaction already %s", transaction.Status)
	}

	// Update transaction status
	transaction.Status = "failed"
	transaction.UpdatedAt = time.Now()

	// Save transaction to database
	if err := tx.Save(&transaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// atomicBalanceUpdate safely updates balance, available, and locked fields atomically within a transaction.
func atomicBalanceUpdate(tx *gorm.DB, account *models.Account, deltaBalance, deltaAvailable, deltaLocked decimal.Decimal) error {
	account.Balance = account.Balance.Add(deltaBalance)
	account.Available = account.Available.Add(deltaAvailable)
	account.Locked = account.Locked.Add(deltaLocked)
	account.UpdatedAt = time.Now()
	return tx.Save(account).Error
}

// LockFunds locks funds in an account
func (s *Service) LockFunds(ctx context.Context, userID string, currency string, amount decimal.Decimal) error {
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var account models.Account
	if err := tx.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	if account.Available.Cmp(amount) < 0 {
		tx.Rollback()
		return fmt.Errorf("insufficient funds")
	}
	if err := atomicBalanceUpdate(tx, &account, decimal.Zero, amount.Neg(), amount); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to lock funds: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// UnlockFunds unlocks funds in an account
func (s *Service) UnlockFunds(ctx context.Context, userID string, currency string, amount decimal.Decimal) error {
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var account models.Account
	if err := tx.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	if account.Locked.Cmp(amount) < 0 {
		tx.Rollback()
		return fmt.Errorf("insufficient locked funds")
	}
	if err := atomicBalanceUpdate(tx, &account, decimal.Zero, amount, amount.Neg()); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to unlock funds: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// atomicTransfer atomically transfers funds between two accounts within a transaction.
func atomicTransfer(tx *gorm.DB, fromAccount, toAccount *models.Account, amount decimal.Decimal) error {
	if fromAccount.Available.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient funds")
	}
	if err := atomicBalanceUpdate(tx, fromAccount, amount.Neg(), amount.Neg(), decimal.Zero); err != nil {
		return fmt.Errorf("failed to update from account: %w", err)
	}
	if err := atomicBalanceUpdate(tx, toAccount, amount, amount, decimal.Zero); err != nil {
		return fmt.Errorf("failed to update to account: %w", err)
	}
	return nil
}

// TransferFunds transfers funds between accounts
func (s *Service) TransferFunds(ctx context.Context, fromUserID string, toUserID string, currency string, amount decimal.Decimal, description string) error {
	fromMu := s.getAccountMutex(fromUserID, currency)
	toMu := s.getAccountMutex(toUserID, currency)
	// Always lock in a consistent order to avoid deadlocks
	if fromUserID < toUserID {
		fromMu.Lock()
		toMu.Lock()
	} else if fromUserID > toUserID {
		toMu.Lock()
		fromMu.Lock()
	} else {
		fromMu.Lock()
	}
	defer func() {
		fromMu.Unlock()
		if fromUserID != toUserID {
			toMu.Unlock()
		}
	}()

	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var fromAccount models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND currency = ?", fromUserID, currency).First(&fromAccount).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("from account not found")
		}
		return fmt.Errorf("failed to find from account: %w", err)
	}

	var toAccount models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND currency = ?", toUserID, currency).First(&toAccount).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create to account
			parsedToUserID, err := uuid.Parse(toUserID)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("invalid toUserID: %w", err)
			}
			toAccount = models.Account{
				ID:        uuid.New(),
				UserID:    parsedToUserID,
				Currency:  currency,
				Balance:   decimal.Zero,
				Available: decimal.Zero,
				Locked:    decimal.Zero,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := tx.Create(&toAccount).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create to account: %w", err)
			}
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to find to account: %w", err)
		}
	}

	if err := atomicTransfer(tx, &fromAccount, &toAccount, amount); err != nil {
		tx.Rollback()
		return err
	}

	parsedFromUserID, err := uuid.Parse(fromUserID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("invalid fromUserID: %w", err)
	}
	fromTransaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      parsedFromUserID,
		Type:        "transfer_out",
		Amount:      amount,
		Currency:    currency,
		Status:      "completed",
		Reference:   toUserID,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CompletedAt: func() *time.Time { now := time.Now(); return &now }(),
	}
	if err := tx.Create(fromTransaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create from transaction: %w", err)
	}

	parsedToUserID, err := uuid.Parse(toUserID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("invalid toUserID: %w", err)
	}
	toTransaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      parsedToUserID,
		Type:        "transfer_in",
		Amount:      amount,
		Currency:    currency,
		Status:      "completed",
		Reference:   fromUserID,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CompletedAt: func() *time.Time { now := time.Now(); return &now }(),
	}
	if err := tx.Create(toTransaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create to transaction: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// getAccountMutex returns a mutex for the given user+currency (account-level lock)
func (s *Service) getAccountMutex(userID, currency string) *sync.Mutex {
	key := userID + ":" + currency
	s.muMapLock.Lock()
	if s.muMap == nil {
		s.muMap = make(map[string]*sync.Mutex)
	}
	mu, ok := s.muMap[key]
	if !ok {
		mu = &sync.Mutex{}
		s.muMap[key] = mu
	}
	s.muMapLock.Unlock()
	return mu
}

// EnhancedLockFunds locks funds with SELECT FOR UPDATE and enhanced validation
func (s *Service) EnhancedLockFunds(ctx context.Context, userID, currency string, amount decimal.Decimal, opts *TransactionOptions) error {
	if opts == nil {
		opts = DefaultTransactionOptions()
	}

	if amount.Cmp(decimal.Zero) <= 0 {
		return ErrInvalidAmount
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(opts.RetryBackoff)
		}

		err := s.executeLockFundsWithRetry(timeoutCtx, userID, currency, amount, opts)
		if err == nil {
			return nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break
		}

		s.logger.Warn("Retrying lock funds operation",
			zap.String("user_id", userID),
			zap.String("currency", currency),
			zap.Float64("amount", amount.InexactFloat64()),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	return fmt.Errorf("failed to lock funds after %d attempts: %w", opts.MaxRetries+1, lastErr)
}

// executeLockFundsWithRetry performs the actual lock funds operation with enhanced locking
func (s *Service) executeLockFundsWithRetry(ctx context.Context, userID, currency string, amount decimal.Decimal, opts *TransactionOptions) error {
	// Start transaction with timeout
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			s.logger.Error("Panic during lock funds operation",
				zap.String("user_id", userID),
				zap.String("currency", currency),
				zap.Any("panic", r))
		}
	}()

	// Pre-transaction validation
	if opts.PreValidationChecks {
		if err := s.validateLockFundsPreConditions(ctx, tx, userID, currency, amount); err != nil {
			tx.Rollback()
			return err
		}
	}

	// Acquire row-level lock with SELECT FOR UPDATE
	var account models.Account
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND currency = ?", userID, currency)

	if opts.RequireRowLocking {
		// Add NOWAIT to detect deadlocks quickly
		query = query.Clauses(clause.Locking{Options: "NOWAIT"})
	}

	if err := query.First(&account).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		if isDeadlockError(err) {
			return ErrDeadlockDetected
		}
		return fmt.Errorf("failed to find and lock account: %w", err)
	}

	// Verify sufficient funds with row-level lock held
	if account.Available.Cmp(amount) < 0 {
		tx.Rollback()
		return fmt.Errorf("%w: available %.8f, required %.8f", ErrInsufficientFunds, account.Available.InexactFloat64(), amount.InexactFloat64())
	}

	// Log audit trail
	if opts.AuditLogging {
		s.logger.Info("Locking funds",
			zap.String("user_id", userID),
			zap.String("currency", currency),
			zap.Float64("amount", amount.InexactFloat64()),
			zap.Float64("available_before", account.Available.InexactFloat64()),
			zap.Float64("locked_before", account.Locked.InexactFloat64()))
	}

	// Perform atomic balance update
	if err := atomicBalanceUpdate(tx, &account, decimal.Zero, amount.Neg(), amount); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to lock funds: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Log successful operation
	if opts.AuditLogging {
		s.logger.Info("Successfully locked funds",
			zap.String("user_id", userID),
			zap.String("currency", currency),
			zap.Float64("amount", amount.InexactFloat64()),
			zap.Float64("available_after", account.Available.Sub(amount).InexactFloat64()),
			zap.Float64("locked_after", account.Locked.Add(amount).InexactFloat64()))
	}

	return nil
}

// validateLockFundsPreConditions performs pre-transaction validation
func (s *Service) validateLockFundsPreConditions(ctx context.Context, tx *gorm.DB, userID, currency string, amount decimal.Decimal) error {
	// Check if account exists
	var count int64
	if err := tx.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userID, currency).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check account existence: %w", err)
	}
	if count == 0 {
		return ErrAccountNotFound
	}

	// Additional business logic validations can be added here
	// For example: check for suspended accounts, currency limits, etc.

	return nil
}

// EnhancedTransferFunds performs fund transfer with enhanced transaction handling
func (s *Service) EnhancedTransferFunds(ctx context.Context, fromUserID, toUserID, currency string, amount decimal.Decimal, description string, opts *TransactionOptions) error {
	if opts == nil {
		opts = DefaultTransactionOptions()
	}

	if amount.Cmp(decimal.Zero) <= 0 {
		return ErrInvalidAmount
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(opts.RetryBackoff)
		}

		err := s.executeTransferWithEnhancedLocking(timeoutCtx, fromUserID, toUserID, currency, amount, description, opts)
		if err == nil {
			return nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break
		}

		s.logger.Warn("Retrying transfer operation",
			zap.String("from_user_id", fromUserID),
			zap.String("to_user_id", toUserID),
			zap.String("currency", currency),
			zap.Float64("amount", amount.InexactFloat64()),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	return fmt.Errorf("failed to transfer funds after %d attempts: %w", opts.MaxRetries+1, lastErr)
}

// executeTransferWithEnhancedLocking performs the actual transfer with enhanced locking
func (s *Service) executeTransferWithEnhancedLocking(ctx context.Context, fromUserID, toUserID, currency string, amount decimal.Decimal, description string, opts *TransactionOptions) error {
	// Get account mutexes in consistent order to avoid deadlocks
	fromMu := s.getAccountMutex(fromUserID, currency)
	toMu := s.getAccountMutex(toUserID, currency)

	// Always lock in lexicographical order to prevent deadlocks
	if fromUserID < toUserID {
		fromMu.Lock()
		toMu.Lock()
	} else if fromUserID > toUserID {
		toMu.Lock()
		fromMu.Lock()
	} else {
		fromMu.Lock()
	}
	defer func() {
		fromMu.Unlock()
		if fromUserID != toUserID {
			toMu.Unlock()
		}
	}()

	// Start transaction with timeout
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			s.logger.Error("Panic during transfer operation",
				zap.String("from_user_id", fromUserID),
				zap.String("to_user_id", toUserID),
				zap.String("currency", currency),
				zap.Any("panic", r))
		}
	}()

	// Pre-transaction validation
	if opts.PreValidationChecks {
		if err := s.validateTransferPreConditions(ctx, tx, fromUserID, toUserID, currency, amount); err != nil {
			tx.Rollback()
			return err
		}
	}

	// Lock both accounts with SELECT FOR UPDATE in consistent order
	var fromAccount, toAccount models.Account

	// Lock from account first
	fromQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND currency = ?", fromUserID, currency)
	if opts.RequireRowLocking {
		fromQuery = fromQuery.Clauses(clause.Locking{Options: "NOWAIT"})
	}

	if err := fromQuery.First(&fromAccount).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("from account not found")
		}
		if isDeadlockError(err) {
			return ErrDeadlockDetected
		}
		return fmt.Errorf("failed to find and lock from account: %w", err)
	}

	// Lock to account (or create if doesn't exist)
	toQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND currency = ?", toUserID, currency)
	if opts.RequireRowLocking {
		toQuery = toQuery.Clauses(clause.Locking{Options: "NOWAIT"})
	}

	err := toQuery.First(&toAccount).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create to account within the transaction
			parsedToUserID, err := uuid.Parse(toUserID)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("invalid toUserID: %w", err)
			}
			toAccount = models.Account{
				ID:        uuid.New(),
				UserID:    parsedToUserID,
				Currency:  currency,
				Balance:   decimal.Zero,
				Available: decimal.Zero,
				Locked:    decimal.Zero,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := tx.Create(&toAccount).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create to account: %w", err)
			}
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to find and lock to account: %w", err)
		}
	}

	// Verify sufficient funds with locks held
	if fromAccount.Available.Cmp(amount) < 0 {
		tx.Rollback()
		return fmt.Errorf("%w: available %.8f, required %.8f", ErrInsufficientFunds, fromAccount.Available.InexactFloat64(), amount.InexactFloat64())
	}

	// Log audit trail
	if opts.AuditLogging {
		s.logger.Info("Transferring funds",
			zap.String("from_user_id", fromUserID),
			zap.String("to_user_id", toUserID),
			zap.String("currency", currency),
			zap.Float64("amount", amount.InexactFloat64()),
			zap.Float64("from_available_before", fromAccount.Available.InexactFloat64()),
			zap.Float64("to_available_before", toAccount.Available.InexactFloat64()))
	}

	// Perform atomic transfer
	if err := atomicTransfer(tx, &fromAccount, &toAccount, amount); err != nil {
		tx.Rollback()
		return err
	}

	// Create transaction records
	now := time.Now()
	fromTransaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(fromUserID),
		Type:        "transfer_out",
		Amount:      amount,
		Currency:    currency,
		Status:      "completed",
		Reference:   toUserID,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
	}

	toTransaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(toUserID),
		Type:        "transfer_in",
		Amount:      amount,
		Currency:    currency,
		Status:      "completed",
		Reference:   fromUserID,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
	}
	if err := tx.Create(fromTransaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create from transaction: %w", err)
	}

	if err := tx.Create(toTransaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create to transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// validateTransferPreConditions performs pre-transaction validation for transfers
func (s *Service) validateTransferPreConditions(ctx context.Context, tx *gorm.DB, fromUserID, toUserID, currency string, amount decimal.Decimal) error {
	// Check if from account exists
	var count int64
	if err := tx.Model(&models.Account{}).Where("user_id = ? AND currency = ?", fromUserID, currency).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check from account existence: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("from account not found: %w", ErrAccountNotFound)
	}

	// Additional business logic validations can be added here
	// For example: check for suspended accounts, transfer limits, etc.

	return nil
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	if errors.Is(err, ErrDeadlockDetected) {
		return true
	}
	if errors.Is(err, ErrConcurrencyConflict) {
		return true
	}
	// Check for database-specific retryable errors
	return isDeadlockError(err) || isConcurrencyError(err)
}

// isDeadlockError checks if the error is a deadlock error
func isDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// PostgreSQL deadlock detection
	return contains(errStr, "deadlock detected") ||
		contains(errStr, "could not serialize access") ||
		contains(errStr, "lock_timeout") ||
		contains(errStr, "lock not available")
}

// isConcurrencyError checks if the error is a concurrency-related error
func isConcurrencyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "concurrent update") ||
		contains(errStr, "serialization failure") ||
		contains(errStr, "retry transaction")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && containsIgnoreCase(s, substr)))
}

func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			result[i] = s[i] + 32
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}

// BatchUpdateBalances updates account balances in batch
func (s *Service) BatchUpdateBalances(ctx context.Context, userID string, currency string, updates []AccountBalance) (*BatchOperationResult, error) {
	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Result container
	result := &BatchOperationResult{
		SuccessCount: 0,
		FailedItems:  make(map[string]error),
		Duration:     0,
	}

	// Process each update
	startTime := time.Now()
	for _, update := range updates {
		// Find account
		var account models.Account
		if err := tx.Where("user_id = ? AND currency = ?", update.UserID, update.Currency).First(&account).Error; err != nil {
			tx.Rollback()
			if err == gorm.ErrRecordNotFound {
				result.FailedItems[update.UserID] = fmt.Errorf("account not found")
			} else {
				result.FailedItems[update.UserID] = fmt.Errorf("failed to find account: %w", err)
			}
			continue
		}

		// Update balance
		account.Balance = update.Balance
		account.Available = update.Available
		account.Locked = update.Locked
		account.UpdatedAt = time.Now()

		// Save account to database
		if err := tx.Save(&account).Error; err != nil {
			tx.Rollback()
			result.FailedItems[update.UserID] = fmt.Errorf("failed to update balance: %w", err)
			continue
		}

		result.SuccessCount++
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit batch balance update: %w", err)
	}

	result.Duration = time.Since(startTime)
	s.logger.Info("Batch balance update completed",
		zap.Int("success_count", result.SuccessCount),
		zap.Int("failed_count", len(result.FailedItems)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// BatchGetAccounts retrieves accounts for multiple users and currencies efficiently (resolves N+1 queries)
func (s *Service) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	if len(userIDs) == 0 {
		return make(map[string]map[string]*models.Account), nil
	}

	start := time.Now()
	defer func() {
		s.logger.Debug("BatchGetAccounts completed",
			zap.Int("user_count", len(userIDs)),
			zap.Int("currency_count", len(currencies)),
			zap.Duration("duration", time.Since(start)))
	}()

	query := s.db.WithContext(ctx).Model(&models.Account{})

	// Add user ID filter
	query = query.Where("user_id IN ?", userIDs)

	// Add currency filter if specified
	if len(currencies) > 0 {
		query = query.Where("currency IN ?", currencies)
	}

	var accounts []models.Account
	if err := query.Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get accounts: %w", err)
	}

	// Group accounts by user ID and currency
	result := make(map[string]map[string]*models.Account)
	for _, account := range accounts {
		userID := account.UserID.String()
		if result[userID] == nil {
			result[userID] = make(map[string]*models.Account)
		}
		result[userID][account.Currency] = &account
	}

	return result, nil
}

// BatchLockFunds locks funds for multiple operations efficiently
func (s *Service) BatchLockFunds(ctx context.Context, operations []FundsOperation) (*BatchOperationResult, error) {
	start := time.Now()
	result := &BatchOperationResult{
		SuccessCount: 0,
		FailedItems:  make(map[string]error),
	}

	if len(operations) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Process operations in batches to avoid transaction timeout
	batchSize := 50
	for i := 0; i < len(operations); i += batchSize {
		end := i + batchSize
		if end > len(operations) {
			end = len(operations)
		}

		batchOps := operations[i:end]
		if err := s.processBatchLockFunds(ctx, batchOps, result); err != nil {
			s.logger.Error("Failed to process batch lock funds", zap.Error(err))
		}
	}

	result.Duration = time.Since(start)
	s.logger.Info("Batch lock funds completed",
		zap.Int("total_operations", len(operations)),
		zap.Int("success_count", result.SuccessCount),
		zap.Int("failed_count", len(result.FailedItems)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// BatchUnlockFunds unlocks funds for multiple operations efficiently
func (s *Service) BatchUnlockFunds(ctx context.Context, operations []FundsOperation) (*BatchOperationResult, error) {
	start := time.Now()
	result := &BatchOperationResult{
		SuccessCount: 0,
		FailedItems:  make(map[string]error),
	}

	if len(operations) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Process operations in batches to avoid transaction timeout
	batchSize := 50
	for i := 0; i < len(operations); i += batchSize {
		end := i + batchSize
		if end > len(operations) {
			end = len(operations)
		}

		batchOps := operations[i:end]
		if err := s.processBatchUnlockFunds(ctx, batchOps, result); err != nil {
			s.logger.Error("Failed to process batch unlock funds", zap.Error(err))
		}
	}

	result.Duration = time.Since(start)
	s.logger.Info("Batch unlock funds completed",
		zap.Int("total_operations", len(operations)),
		zap.Int("success_count", result.SuccessCount),
		zap.Int("failed_count", len(result.FailedItems)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// processBatchLockFunds processes a batch of lock funds operations in a single transaction
func (s *Service) processBatchLockFunds(ctx context.Context, operations []FundsOperation, result *BatchOperationResult) error {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, op := range operations {
		opKey := fmt.Sprintf("%s-%s", op.UserID, op.Currency)

		var account models.Account
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND currency = ?", op.UserID, op.Currency).
			First(&account).Error; err != nil {
			result.FailedItems[opKey] = fmt.Errorf("failed to find account: %w", err)
			continue
		}

		if account.Available.Cmp(op.Amount) < 0 {
			result.FailedItems[opKey] = fmt.Errorf("insufficient funds: available %.8f, required %.8f",
				account.Available.InexactFloat64(), op.Amount.InexactFloat64())
			continue
		}

		if err := atomicBalanceUpdate(tx, &account, decimal.Zero, op.Amount.Neg(), op.Amount); err != nil {
			result.FailedItems[opKey] = fmt.Errorf("failed to lock funds: %w", err)
			continue
		}

		result.SuccessCount++
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit batch lock transaction: %w", err)
	}

	return nil
}

// processBatchUnlockFunds processes a batch of unlock funds operations in a single transaction
func (s *Service) processBatchUnlockFunds(ctx context.Context, operations []FundsOperation, result *BatchOperationResult) error {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, op := range operations {
		opKey := fmt.Sprintf("%s-%s", op.UserID, op.Currency)

		var account models.Account
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND currency = ?", op.UserID, op.Currency).
			First(&account).Error; err != nil {
			result.FailedItems[opKey] = fmt.Errorf("failed to find account: %w", err)
			continue
		}

		if account.Locked.Cmp(op.Amount) < 0 {
			result.FailedItems[opKey] = fmt.Errorf("insufficient locked funds: locked %.8f, required %.8f",
				account.Locked.InexactFloat64(), op.Amount.InexactFloat64())
			continue
		}

		if err := atomicBalanceUpdate(tx, &account, decimal.Zero, op.Amount, op.Amount.Neg()); err != nil {
			result.FailedItems[opKey] = fmt.Errorf("failed to unlock funds: %w", err)
			continue
		}

		result.SuccessCount++
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit batch unlock transaction: %w", err)
	}

	return nil
}

// XA Transaction Support

// XID represents a distributed transaction identifier
type XID struct {
	FormatID    int32
	BranchID    []byte
	GlobalTxnID []byte
}

func (x XID) String() string {
	return fmt.Sprintf("%d-%x-%x", x.FormatID, x.GlobalTxnID, x.BranchID)
}

// XAResource defines the interface for XA transaction resources
type XAResource interface {
	GetResourceName() string
	StartXA(xid XID) error
	EndXA(xid XID, flags int) error
	PrepareXA(xid XID) error
	CommitXA(xid XID, onePhase bool) error
	RollbackXA(xID XID) error
	RecoverXA() ([]XID, error)
}

// XATransaction represents an XA transaction context
type XATransaction struct {
	XID              XID
	Operations       []XAOperation
	CompensationData map[string]interface{}
	DBTransaction    *gorm.DB
	State            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// XAOperation represents an operation within an XA transaction
type XAOperation struct {
	Type        string // "lock_funds", "unlock_funds", "transfer", "create_transaction"
	UserID      string
	Currency    string
	Amount      decimal.Decimal
	FromUserID  string
	ToUserID    string
	Reference   string
	Description string
	Result      interface{}
	Error       error
	Timestamp   time.Time
}

// BookkeeperXAService extends BookkeeperService with XA transaction support
type BookkeeperXAService interface {
	BookkeeperService
	XAResource

	// XA-specific bookkeeper operations
	LockFundsXA(ctx context.Context, xid XID, userID, currency string, amount decimal.Decimal) error
	UnlockFundsXA(ctx context.Context, xid XID, userID, currency string, amount decimal.Decimal) error
	TransferFundsXA(ctx context.Context, xid XID, fromUserID, toUserID, currency string, amount decimal.Decimal, reference string) error
	CreateTransactionXA(ctx context.Context, xid XID, userID, transactionType string, amount decimal.Decimal, currency, reference, description string) (*models.Transaction, error)
}

// XA Resource Implementation

// GetResourceName returns the name of this XA resource
func (s *Service) GetResourceName() string {
	return "bookkeeper"
}

// StartXA begins a new XA transaction
func (s *Service) StartXA(xid XID) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	if _, exists := s.pendingXATransactions[xidStr]; exists {
		return fmt.Errorf("XA transaction already exists: %s", xidStr)
	}

	// Begin database transaction
	dbTx := s.db.Begin()
	if dbTx.Error != nil {
		return fmt.Errorf("failed to begin database transaction: %w", dbTx.Error)
	}

	s.pendingXATransactions[xidStr] = &XATransaction{
		XID:              xid,
		Operations:       make([]XAOperation, 0),
		CompensationData: make(map[string]interface{}),
		DBTransaction:    dbTx,
		State:            "active",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	s.logger.Info("Started XA transaction",
		zap.String("xid", xidStr))

	return nil
}

// EndXA ends an XA transaction
func (s *Service) EndXA(xid XID, flags int) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	txn.State = "ended"
	txn.UpdatedAt = time.Now()

	s.logger.Info("Ended XA transaction",
		zap.String("xid", xidStr),
		zap.Int("flags", flags))

	return nil
}

// PrepareXA prepares an XA transaction for commit
func (s *Service) PrepareXA(xid XID) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	if txn.State != "ended" {
		return fmt.Errorf("XA transaction not in ended state: %s", txn.State)
	}

	// Validate all operations can be committed
	for _, op := range txn.Operations {
		if op.Error != nil {
			s.logger.Error("XA transaction has failed operations",
				zap.String("xid", xidStr),
				zap.String("operation", op.Type),
				zap.Error(op.Error))
			return fmt.Errorf("XA transaction has failed operations")
		}
	}

	txn.State = "prepared"
	txn.UpdatedAt = time.Now()

	s.logger.Info("Prepared XA transaction",
		zap.String("xid", xidStr))

	return nil
}

// CommitXA commits an XA transaction
func (s *Service) CommitXA(xid XID, onePhase bool) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	if !onePhase && txn.State != "prepared" {
		return fmt.Errorf("XA transaction not in prepared state: %s", txn.State)
	}

	// Commit the database transaction
	if err := txn.DBTransaction.Commit().Error; err != nil {
		s.logger.Error("Failed to commit XA transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
		return fmt.Errorf("failed to commit XA transaction: %w", err)
	}

	// Remove from pending transactions
	delete(s.pendingXATransactions, xidStr)

	s.logger.Info("Committed XA transaction",
		zap.String("xid", xidStr),
		zap.Bool("one_phase", onePhase))

	return nil
}

// RollbackXA rolls back an XA transaction
func (s *Service) RollbackXA(xid XID) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	// Rollback the database transaction
	if err := txn.DBTransaction.Rollback().Error; err != nil {
		s.logger.Error("Failed to rollback XA transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
		return fmt.Errorf("failed to rollback XA transaction: %w", err)
	}

	// Execute compensation operations if needed
	s.executeCompensations(txn)

	// Remove from pending transactions
	delete(s.pendingXATransactions, xidStr)

	s.logger.Info("Rolled back XA transaction",
		zap.String("xid", xidStr))

	return nil
}

// RecoverXA returns a list of prepared XA transactions for recovery
func (s *Service) RecoverXA() ([]XID, error) {
	s.xaMutex.RLock()
	defer s.xaMutex.RUnlock()

	var preparedXIDs []XID
	for _, txn := range s.pendingXATransactions {
		if txn.State == "prepared" {
			preparedXIDs = append(preparedXIDs, txn.XID)
		}
	}

	s.logger.Info("XA recovery found prepared transactions",
		zap.Int("count", len(preparedXIDs)))

	return preparedXIDs, nil
}

// XA-specific bookkeeper operations

// LockFundsXA locks funds within an XA transaction
func (s *Service) LockFundsXA(ctx context.Context, xid XID, userID, currency string, amount decimal.Decimal) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	// Get current account state
	var account models.Account
	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Store compensation data
	compensationKey := fmt.Sprintf("lock_funds_%s_%s", userID, currency)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"original_available": account.Available,
		"original_locked":    account.Locked,
		"amount":             amount,
	}

	// Check if account has enough available funds
	if account.Available.Cmp(amount) < 0 {
		return ErrInsufficientFunds
	}

	// Update account within transaction
	account.Available = account.Available.Sub(amount)
	account.Locked = account.Locked.Add(amount)
	account.UpdatedAt = time.Now()

	if err := txn.DBTransaction.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to lock funds: %w", err)
	}

	// Record operation
	op := XAOperation{
		Type:      "lock_funds",
		UserID:    userID,
		Currency:  currency,
		Amount:    amount,
		Timestamp: time.Now(),
	}
	txn.Operations = append(txn.Operations, op)

	s.logger.Debug("Locked funds in XA transaction",
		zap.String("xid", xidStr),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount.InexactFloat64()))

	return nil
}

// UnlockFundsXA unlocks funds within an XA transaction
func (s *Service) UnlockFundsXA(ctx context.Context, xid XID, userID, currency string, amount decimal.Decimal) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	// Get current account state
	var account models.Account
	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Store compensation data
	compensationKey := fmt.Sprintf("unlock_funds_%s_%s", userID, currency)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"original_available": account.Available,
		"original_locked":    account.Locked,
		"amount":             amount,
	}

	// Check if account has enough locked funds
	if account.Locked.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient locked funds: locked %.8f, required %.8f", account.Locked.InexactFloat64(), amount.InexactFloat64())
	}

	// Update account within transaction
	account.Available = account.Available.Add(amount)
	account.Locked = account.Locked.Sub(amount)
	account.UpdatedAt = time.Now()

	if err := txn.DBTransaction.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to unlock funds: %w", err)
	}

	// Record operation
	op := XAOperation{
		Type:      "unlock_funds",
		UserID:    userID,
		Currency:  currency,
		Amount:    amount,
		Timestamp: time.Now(),
	}
	txn.Operations = append(txn.Operations, op)

	s.logger.Debug("Unlocked funds in XA transaction",
		zap.String("xid", xidStr),
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("amount", amount.InexactFloat64()))

	return nil
}

// TransferFundsXA transfers funds between accounts within an XA transaction
func (s *Service) TransferFundsXA(ctx context.Context, xid XID, fromUserID, toUserID, currency string, amount decimal.Decimal, reference string) error {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	// Get both accounts
	var fromAccount, toAccount models.Account

	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", fromUserID, currency).First(&fromAccount).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("from account not found")
		}
		return fmt.Errorf("failed to find from account: %w", err)
	}

	if err := txn.DBTransaction.Where("user_id = ? AND currency = ?", toUserID, currency).First(&toAccount).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("to account not found")
		}
		return fmt.Errorf("failed to find to account: %w", err)
	}

	// Store compensation data
	compensationKey := fmt.Sprintf("transfer_%s_%s_to_%s", fromUserID, currency, toUserID)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"from_original_available": fromAccount.Available,
		"from_original_locked":    fromAccount.Locked,
		"to_original_available":   toAccount.Available,
		"to_original_locked":      toAccount.Locked,
		"amount":                  amount,
	}

	// Check if from account has enough available funds
	if fromAccount.Available.Cmp(amount) < 0 {
		return ErrInsufficientFunds
	}

	// Update accounts
	fromAccount.Available = fromAccount.Available.Sub(amount)
	toAccount.Available = toAccount.Available.Add(amount)

	now := time.Now()
	fromAccount.UpdatedAt = now
	toAccount.UpdatedAt = now

	// Save both accounts
	if err := txn.DBTransaction.Save(&fromAccount).Error; err != nil {
		return fmt.Errorf("failed to update from account: %w", err)
	}

	if err := txn.DBTransaction.Save(&toAccount).Error; err != nil {
		return fmt.Errorf("failed to update to account: %w", err)
	}

	// Record operation
	op := XAOperation{
		Type:       "transfer",
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Currency:   currency,
		Amount:     amount,
		Reference:  reference,
		Timestamp:  now,
	}
	txn.Operations = append(txn.Operations, op)

	s.logger.Debug("Transferred funds in XA transaction",
		zap.String("xid", xidStr),
		zap.String("from_user_id", fromUserID),
		zap.String("to_user_id", toUserID),
		zap.String("currency", currency),
		zap.Float64("amount", amount.InexactFloat64()),
		zap.String("reference", reference))

	return nil
}

// CreateTransactionXA creates a transaction within an XA transaction
func (s *Service) CreateTransactionXA(ctx context.Context, xid XID, userID, transactionType string, amount decimal.Decimal, currency, reference, description string) (*models.Transaction, error) {
	s.xaMutex.Lock()
	defer s.xaMutex.Unlock()

	xidStr := xid.String()
	txn, exists := s.pendingXATransactions[xidStr]
	if !exists {
		return nil, fmt.Errorf("XA transaction not found: %s", xidStr)
	}

	// Create transaction record
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid userID: %w", err)
	}
	// Create transaction record
	transaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      parsedUserID,
		Type:        transactionType,
		Amount:      amount,
		Currency:    currency,
		Reference:   reference,
		Description: description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := txn.DBTransaction.Create(transaction).Error; err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Record operation
	op := XAOperation{
		Type:        "create_transaction",
		UserID:      userID, // XAOperation.UserID is string
		Currency:    currency,
		Amount:      amount,
		Reference:   reference,
		Description: description,
		Result:      transaction,
		Timestamp:   time.Now(),
	}
	txn.Operations = append(txn.Operations, op)

	s.logger.Debug("Created transaction in XA transaction",
		zap.String("xid", xidStr),
		zap.String("user_id", userID),
		zap.String("type", transactionType),
		zap.Float64("amount", amount.InexactFloat64()),
		zap.String("currency", currency))

	return transaction, nil
}

// executeCompensations executes compensation operations for a failed transaction
func (s *Service) executeCompensations(txn *XATransaction) {
	for key, data := range txn.CompensationData {
		s.logger.Info("Executing compensation",
			zap.String("xid", txn.XID.String()),
			zap.String("compensation", key))

		// Implementation depends on the specific compensation logic needed
		// This is a placeholder for compensation execution
		_ = data
	}
}
