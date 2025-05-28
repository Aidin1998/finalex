package bookkeeper

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
	logger *zap.Logger
	db     *gorm.DB
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

// LockFunds locks funds in an account
func (s *Service) LockFunds(ctx context.Context, userID string, currency string, amount float64) error {
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

	// Find account
	var account models.Account
	if err := tx.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Check if account has enough available funds
	if account.Available < amount {
		tx.Rollback()
		return fmt.Errorf("insufficient funds")
	}

	// Update account
	account.Available -= amount
	account.Locked += amount
	account.UpdatedAt = time.Now()

	// Save account to database
	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save account: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UnlockFunds unlocks funds in an account
func (s *Service) UnlockFunds(ctx context.Context, userID string, currency string, amount float64) error {
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

	// Find account
	var account models.Account
	if err := tx.Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("account not found")
		}
		return fmt.Errorf("failed to find account: %w", err)
	}

	// Check if account has enough locked funds
	if account.Locked < amount {
		tx.Rollback()
		return fmt.Errorf("insufficient locked funds")
	}

	// Update account
	account.Available += amount
	account.Locked -= amount
	account.UpdatedAt = time.Now()

	// Save account to database
	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save account: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// TransferFunds transfers funds between accounts
func (s *Service) TransferFunds(ctx context.Context, fromUserID string, toUserID string, currency string, amount float64, description string) error {
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

	// Find from account
	var fromAccount models.Account
	if err := tx.Where("user_id = ? AND currency = ?", fromUserID, currency).First(&fromAccount).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("from account not found")
		}
		return fmt.Errorf("failed to find from account: %w", err)
	}

	// Check if from account has enough available funds
	if fromAccount.Available < amount {
		tx.Rollback()
		return fmt.Errorf("insufficient funds")
	}

	// Find to account
	var toAccount models.Account
	if err := tx.Where("user_id = ? AND currency = ?", toUserID, currency).First(&toAccount).Error; err != nil {
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

	// Update from account
	fromAccount.Balance -= amount
	fromAccount.Available -= amount
	fromAccount.UpdatedAt = time.Now()

	// Save from account to database
	if err := tx.Save(&fromAccount).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save from account: %w", err)
	}

	// Update to account
	toAccount.Balance += amount
	toAccount.Available += amount
	toAccount.UpdatedAt = time.Now()

	// Save to account to database
	if err := tx.Save(&toAccount).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save to account: %w", err)
	}

	// Create from transaction
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

	// Save from transaction to database
	if err := tx.Create(fromTransaction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create from transaction: %w", err)
	}

	// Create to transaction
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

	// Save to transaction to database
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
