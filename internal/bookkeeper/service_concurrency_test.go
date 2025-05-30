// service_concurrency_test.go
// Concurrency and race condition tests for bookkeeper Service
//
// Scenarios:
// 1. Concurrent TransferFunds between same accounts
// 2. Concurrent LockFunds/UnlockFunds on same account
// 3. Concurrent CreateAccount for same user/currency
// 4. Stress test: 1000+ concurrent transfers/locks
// 5. Integration: deposit, transfer, lock, unlock, complete/fail transaction
//
// Expected: No race conditions (run with -race), no double-spend, no duplicate accounts, balances always correct.

package bookkeeper

import (
	"context"
	"sync"
	"testing"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestService(t *testing.T) *Service {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	db.AutoMigrate(&models.Account{}, &models.Transaction{})
	return &Service{db: db}
}

func TestConcurrentTransferFunds(t *testing.T) {
	s := setupTestService(t)
	ctx := context.Background()
	userA := uuid.New().String()
	userB := uuid.New().String()
	currency := "USD"
	_, _ = s.CreateAccount(ctx, userA, currency)
	_, _ = s.CreateAccount(ctx, userB, currency)
	// Seed userA with balance
	db := s.db
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userA, currency).Update("balance", 10000.0)
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userA, currency).Update("available", 10000.0)

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
	if accA.Balance != 10000.0-10.0*float64(n) {
		t.Errorf("userA balance wrong: got %v", accA.Balance)
	}
	if accB.Balance != 10.0*float64(n) {
		t.Errorf("userB balance wrong: got %v", accB.Balance)
	}
}

func TestConcurrentLockUnlockFunds(t *testing.T) {
	s := setupTestService(t)
	ctx := context.Background()
	user := uuid.New().String()
	currency := "USD"
	_, _ = s.CreateAccount(ctx, user, currency)
	db := s.db
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", user, currency).Update("balance", 1000.0)
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", user, currency).Update("available", 1000.0)

	wg := sync.WaitGroup{}
	n := 50
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			err := s.LockFunds(ctx, user, currency, 5.0)
			if err != nil {
				t.Errorf("lock failed: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			err := s.UnlockFunds(ctx, user, currency, 5.0)
			if err != nil && err.Error() != "insufficient locked funds" {
				t.Errorf("unlock failed: %v", err)
			}
		}()
	}
	wg.Wait()
	acc, _ := s.GetAccount(ctx, user, currency)
	if acc.Balance != 1000.0 {
		t.Errorf("balance changed: got %v", acc.Balance)
	}
	if acc.Available+acc.Locked != 1000.0 {
		t.Errorf("available+locked mismatch: got %v+%v", acc.Available, acc.Locked)
	}
}

func TestConcurrentCreateAccount(t *testing.T) {
	s := setupTestService(t)
	ctx := context.Background()
	user := uuid.New().String()
	currency := "USD"
	wg := sync.WaitGroup{}
	n := 20
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := s.CreateAccount(ctx, user, currency)
			errs[idx] = err
		}(i)
	}
	wg.Wait()
	count := 0
	for _, err := range errs {
		if err == nil {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 account created, got %d", count)
	}
}

func TestStressConcurrentTransfers(t *testing.T) {
	s := setupTestService(t)
	ctx := context.Background()
	userA := uuid.New().String()
	userB := uuid.New().String()
	currency := "USD"
	_, _ = s.CreateAccount(ctx, userA, currency)
	_, _ = s.CreateAccount(ctx, userB, currency)
	db := s.db
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userA, currency).Update("balance", 1e6)
	db.Model(&models.Account{}).Where("user_id = ? AND currency = ?", userA, currency).Update("available", 1e6)

	wg := sync.WaitGroup{}
	n := 1000
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.TransferFunds(ctx, userA, userB, currency, 1.0, "stress")
			if err != nil {
				t.Errorf("transfer failed: %v", err)
			}
		}()
	}
	wg.Wait()
	accA, _ := s.GetAccount(ctx, userA, currency)
	accB, _ := s.GetAccount(ctx, userB, currency)
	if accA.Balance != 1e6-1.0*float64(n) {
		t.Errorf("userA balance wrong: got %v", accA.Balance)
	}
	if accB.Balance != 1.0*float64(n) {
		t.Errorf("userB balance wrong: got %v", accB.Balance)
	}
}

func TestIntegrationScenario(t *testing.T) {
	s := setupTestService(t)
	ctx := context.Background()
	user := uuid.New().String()
	currency := "USD"
	_, err := s.CreateAccount(ctx, user, currency)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	// Simulate deposit
	tx, err := s.CreateTransaction(ctx, user, "deposit", 1000.0, currency, "ref1", "deposit")
	if err != nil {
		t.Fatalf("create deposit txn: %v", err)
	}
	err = s.CompleteTransaction(ctx, tx.ID.String())
	if err != nil {
		t.Fatalf("complete txn: %v", err)
	}
	// Lock funds
	err = s.LockFunds(ctx, user, currency, 500.0)
	if err != nil {
		t.Fatalf("lock funds: %v", err)
	}
	// Unlock funds
	err = s.UnlockFunds(ctx, user, currency, 200.0)
	if err != nil {
		t.Fatalf("unlock funds: %v", err)
	}
	// Withdraw
	tx2, err := s.CreateTransaction(ctx, user, "withdrawal", 100.0, currency, "ref2", "withdraw")
	if err != nil {
		t.Fatalf("create withdraw txn: %v", err)
	}
	err = s.CompleteTransaction(ctx, tx2.ID.String())
	if err != nil {
		t.Fatalf("complete withdraw: %v", err)
	}
	// Fail a transaction
	tx3, err := s.CreateTransaction(ctx, user, "withdrawal", 9999.0, currency, "ref3", "fail")
	if err != nil {
		t.Fatalf("create fail txn: %v", err)
	}
	err = s.FailTransaction(ctx, tx3.ID.String())
	if err != nil {
		t.Fatalf("fail txn: %v", err)
	}
	// Final balance check
	final, _ := s.GetAccount(ctx, user, currency)
	if final.Balance != 1000.0-100.0 {
		t.Errorf("final balance wrong: got %v", final.Balance)
	}
	if final.Available+final.Locked != final.Balance {
		t.Errorf("available+locked != balance: %v+%v != %v", final.Available, final.Locked, final.Balance)
	}
}

// END OF TEST FILE
