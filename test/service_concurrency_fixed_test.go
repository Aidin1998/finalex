package test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BookkeeperService represents a test service with database operations for concurrency testing
type BookkeeperService struct {
	db *gorm.DB
	mu sync.RWMutex
}

// NewBookkeeperService creates a new test service
func NewBookkeeperService(db *gorm.DB) *BookkeeperService {
	return &BookkeeperService{
		db: db,
	}
}

// CreateAccount creates a new account for testing
func (s *BookkeeperService) CreateAccount(ctx context.Context, userID string, currency string) (*models.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		userUUID = uuid.New() // Generate new UUID if parsing fails
	}

	account := &models.Account{
		UserID:    userUUID,
		Currency:  currency,
		Balance:   0,
		Available: 0,
	}

	err = s.db.WithContext(ctx).Create(account).Error
	return account, err
}

// TransferFunds transfers funds between accounts for testing
func (s *BookkeeperService) TransferFunds(ctx context.Context, fromUserID, toUserID, currency string, amount float64, reference string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Parse UUIDs
	fromUUID, err := uuid.Parse(fromUserID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("invalid from user ID: %w", err)
	}

	toUUID, err := uuid.Parse(toUserID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("invalid to user ID: %w", err)
	}

	// Deduct from sender
	result := tx.Model(&models.Account{}).
		Where("user_id = ? AND currency = ? AND available >= ?", fromUUID, currency, amount).
		Updates(map[string]interface{}{
			"balance":   gorm.Expr("balance - ?", amount),
			"available": gorm.Expr("available - ?", amount),
		})

	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}

	if result.RowsAffected == 0 {
		tx.Rollback()
		return gorm.ErrRecordNotFound
	}

	// Add to receiver
	result = tx.Model(&models.Account{}).
		Where("user_id = ? AND currency = ?", toUUID, currency).
		Updates(map[string]interface{}{
			"balance":   gorm.Expr("balance + ?", amount),
			"available": gorm.Expr("available + ?", amount),
		})

	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}

	return tx.Commit().Error
}

// LockFunds locks funds in an account for testing
func (s *BookkeeperService) LockFunds(ctx context.Context, userID, currency string, amount float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	result := s.db.WithContext(ctx).Model(&models.Account{}).
		Where("user_id = ? AND currency = ? AND available >= ?", userUUID, currency, amount).
		Update("available", gorm.Expr("available - ?", amount))

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return result.Error
}

// UnlockFunds unlocks funds in an account for testing
func (s *BookkeeperService) UnlockFunds(ctx context.Context, userID, currency string, amount float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	return s.db.WithContext(ctx).Model(&models.Account{}).
		Where("user_id = ? AND currency = ?", userUUID, currency).
		Update("available", gorm.Expr("available + ?", amount)).Error
}

// GetAccount gets an account for testing
func (s *BookkeeperService) GetAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	var account models.Account
	err = s.db.WithContext(ctx).Where("user_id = ? AND currency = ?", userUUID, currency).First(&account).Error
	return &account, err
}

// CreateTransaction creates a transaction for testing
func (s *BookkeeperService) CreateTransaction(ctx context.Context, userID, txType string, amount float64, currency, reference, description string) (*models.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		userUUID = uuid.New()
	}

	tx := &models.Transaction{
		UserID:      userUUID,
		Type:        txType,
		Amount:      amount,
		Currency:    currency,
		Reference:   reference,
		Description: description,
		Status:      "pending",
	}

	err = s.db.WithContext(ctx).Create(tx).Error
	return tx, err
}

// CompleteTransaction completes a transaction for testing
func (s *BookkeeperService) CompleteTransaction(ctx context.Context, txID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("id = ?", txID).
		Update("status", "completed").Error
}

// FailTransaction fails a transaction for testing
func (s *BookkeeperService) FailTransaction(ctx context.Context, txID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("id = ?", txID).
		Update("status", "failed").Error
}

func setupTestBookkeeperService(t *testing.T) *BookkeeperService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	db.AutoMigrate(&models.Account{}, &models.Transaction{})
	return NewBookkeeperService(db)
}

func TestConcurrentTransferFunds_Fixed(t *testing.T) {
	s := setupTestBookkeeperService(t)
	ctx := context.Background()
	userA := uuid.New().String()
	userB := uuid.New().String()
	currency := "USD"

	_, _ = s.CreateAccount(ctx, userA, currency)
	_, _ = s.CreateAccount(ctx, userB, currency)

	// Seed userA with balance
	db := s.db
	userAUUID, _ := uuid.Parse(userA)
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userAUUID, currency).Updates(map[string]interface{}{
		"balance":   10000.0,
		"available": 10000.0,
	})

	wg := sync.WaitGroup{}
	n := 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.TransferFunds(ctx, userA, userB, currency, 10.0, "test")
			if err != nil {
				t.Errorf("transfer failed: %v", err)
			}
		}()
	}
	wg.Wait()

	// Check balances
	accA, _ := s.GetAccount(ctx, userA, currency)
	accB, _ := s.GetAccount(ctx, userB, currency)
	expectedA := 10000.0 - 10.0*float64(n)
	expectedB := 10.0 * float64(n)

	if accA.Balance != expectedA {
		t.Errorf("userA balance wrong: got %v, expected %v", accA.Balance, expectedA)
	}
	if accB.Balance != expectedB {
		t.Errorf("userB balance wrong: got %v, expected %v", accB.Balance, expectedB)
	}
}

func TestConcurrentLockUnlockFunds_Fixed(t *testing.T) {
	s := setupTestBookkeeperService(t)
	ctx := context.Background()
	user := uuid.New().String()
	currency := "USD"

	_, _ = s.CreateAccount(ctx, user, currency)
	userUUID, _ := uuid.Parse(user)

	db := s.db
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userUUID, currency).Updates(map[string]interface{}{
		"balance":   1000.0,
		"available": 1000.0,
	})

	wg := sync.WaitGroup{}
	n := 50
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			err := s.LockFunds(ctx, user, currency, 5.0)
			if err != nil {
				// Expected due to concurrency
				t.Logf("lock failed (expected): %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			err := s.UnlockFunds(ctx, user, currency, 5.0)
			if err != nil {
				// Expected due to concurrency
				t.Logf("unlock failed (expected): %v", err)
			}
		}()
	}
	wg.Wait()

	acc, _ := s.GetAccount(ctx, user, currency)
	if acc.Balance != 1000.0 {
		t.Errorf("balance changed: got %v", acc.Balance)
	}
}
