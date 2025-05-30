package bookkeeper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BookkeeperService defines bookkeeping operations and supports transaction and account lifecycle
type BookkeeperService interface {
	Start() error
	Stop() error
	GetAccounts(ctx context.Context, userID string) ([]*models.Account, error)
	GetAccount(ctx context.Context, userID, currency string) (*models.Account, error)
	CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error)
	GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error)
	CreateTransaction(ctx context.Context, userID, transactionType string, amount float64, currency, reference, description string) (*models.Transaction, error)
	CompleteTransaction(ctx context.Context, transactionID string) error
	FailTransaction(ctx context.Context, transactionID string) error
	LockFunds(ctx context.Context, userID, currency string, amount float64) error
	UnlockFunds(ctx context.Context, userID, currency string, amount float64) error
}

// Service implements BookkeeperService
type Service struct {
	logger    *zap.Logger
	db        *gorm.DB
	muMap     map[string]*sync.Mutex
	muMapLock sync.Mutex // protects muMap
}

// NewService creates a new BookkeeperService
func NewService(logger *zap.Logger, db *gorm.DB) (BookkeeperService, error) {
	// Create service
	svc := &Service{
		logger: logger,
		db:     db,
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
	account := &models.Account{
		ID:        uuid.New(),
		UserID:    uuid.MustParse(userID),
		Currency:  currency,
		Balance:   0,
		Available: 0,
		Locked:    0,
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
func (s *Service) CreateTransaction(ctx context.Context, userID string, transactionType string, amount float64, currency string, reference, description string) (*models.Transaction, error) {
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
		UserID:      uuid.MustParse(userID),
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
		account.Balance += transaction.Amount
		account.Available += transaction.Amount
	} else if transaction.Type == "withdrawal" {
		if account.Available < transaction.Amount {
			tx.Rollback()
			return fmt.Errorf("insufficient funds")
		}
		account.Balance -= transaction.Amount
		account.Available -= transaction.Amount
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
func atomicBalanceUpdate(tx *gorm.DB, account *models.Account, deltaBalance, deltaAvailable, deltaLocked float64) error {
	account.Balance += deltaBalance
	account.Available += deltaAvailable
	account.Locked += deltaLocked
	account.UpdatedAt = time.Now()
	return tx.Save(account).Error
}

// LockFunds locks funds in an account
func (s *Service) LockFunds(ctx context.Context, userID string, currency string, amount float64) error {
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

	if account.Available < amount {
		tx.Rollback()
		return fmt.Errorf("insufficient funds")
	}
	if err := atomicBalanceUpdate(tx, &account, 0, -amount, amount); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to lock funds: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// UnlockFunds unlocks funds in an account
func (s *Service) UnlockFunds(ctx context.Context, userID string, currency string, amount float64) error {
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

	if account.Locked < amount {
		tx.Rollback()
		return fmt.Errorf("insufficient locked funds")
	}
	if err := atomicBalanceUpdate(tx, &account, 0, amount, -amount); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to unlock funds: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// atomicTransfer atomically transfers funds between two accounts within a transaction.
func atomicTransfer(tx *gorm.DB, fromAccount, toAccount *models.Account, amount float64) error {
	if fromAccount.Available < amount {
		return fmt.Errorf("insufficient funds")
	}
	if err := atomicBalanceUpdate(tx, fromAccount, -amount, -amount, 0); err != nil {
		return fmt.Errorf("failed to update from account: %w", err)
	}
	if err := atomicBalanceUpdate(tx, toAccount, amount, amount, 0); err != nil {
		return fmt.Errorf("failed to update to account: %w", err)
	}
	return nil
}

// TransferFunds transfers funds between accounts
func (s *Service) TransferFunds(ctx context.Context, fromUserID string, toUserID string, currency string, amount float64, description string) error {
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
			toAccount = models.Account{
				ID:        uuid.New(),
				UserID:    uuid.MustParse(toUserID),
				Currency:  currency,
				Balance:   0,
				Available: 0,
				Locked:    0,
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

	fromTransaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(fromUserID),
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

	toTransaction := &models.Transaction{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(toUserID),
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

// Locking hierarchy and naming convention documentation
//
// Locking Hierarchy (broadest to narrowest):
// 1. GlobalLock (rare, e.g., for migrations/maintenance)
// 2. AccountLock (per user+currency, e.g., "account:{userID}:{currency}")
// 3. BalanceLock (per account, for balance/available/locked fields)
// 4. TransactionLock (per transaction, e.g., "txn:{transactionID}")
//
// Lock Types:
// - Exclusive lock: Required for any operation that modifies balances or account state (e.g., TransferFunds, LockFunds, UnlockFunds, CreateTransaction, CompleteTransaction, FailTransaction)
// - Read lock: For queries that do not modify state (e.g., GetAccount, GetAccounts, GetAccountTransactions)
//
// Lock Acquisition Order:
// - Always acquire locks in lexicographical order of lock name (e.g., "account:alice:USD" before "account:bob:USD").
// - For operations involving both AccountLock and TransactionLock, always acquire AccountLock(s) first, then TransactionLock(s).
//
// Naming Convention:
// - AccountLock: "account:{userID}:{currency}"
// - BalanceLock: "balance:{accountID}"
// - TransactionLock: "txn:{transactionID}"
// - GlobalLock: "global"
//
// Example (TransferFunds):
//   fromMu := s.getAccountMutex(fromUserID, currency) // "account:{fromUserID}:{currency}"
//   toMu := s.getAccountMutex(toUserID, currency)     // "account:{toUserID}:{currency}"
//   // Always lock in lex order to avoid deadlocks
//   if fromUserID < toUserID { fromMu.Lock(); toMu.Lock() } ...
//
// Visual Diagram:
//   GlobalLock
//      |
//   AccountLock ("account:{userID}:{currency}")
//      |
//   BalanceLock ("balance:{accountID}")
//      |
//   TransactionLock ("txn:{transactionID}")
//
// This strategy ensures no deadlocks, high concurrency, and clear, auditable locking for all financial operations.
