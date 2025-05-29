// moved from internal/fiat/service_test.go
package test

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/fiat"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFiatTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Account{}, &models.Transaction{}))
	return db
}

func TestInitiateDepositAndGetDeposits(t *testing.T) {
	db := setupFiatTestDB(t)
	logger := zap.NewNop()
	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)
	svc, err := fiat.NewService(logger, db, bkSvc, nil)
	assert.NoError(t, err)
	ctx := context.Background()
	userID := uuid.New().String()
	currency := "USD"
	// Create account first
	_, err = bkSvc.CreateAccount(ctx, userID, currency)
	assert.NoError(t, err)

	// Deposit
	tx, err := svc.InitiateDeposit(ctx, userID, currency, 100.0, "provider1")
	assert.NoError(t, err)
	assert.Equal(t, "deposit", tx.Type)

	// Get deposits
	deposits, total, err := svc.GetDeposits(ctx, userID, currency, 10, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, deposits, 1)

	// Complete deposit
	err = svc.CompleteDeposit(ctx, tx.ID.String())
	assert.NoError(t, err)
}

func TestInitiateWithdrawalAndGetWithdrawals(t *testing.T) {
	db := setupFiatTestDB(t)
	logger := zap.NewNop()
	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)
	svc, err := fiat.NewService(logger, db, bkSvc, nil)
	assert.NoError(t, err)
	ctx := context.Background()
	userID := uuid.New().String()
	currency := "USD"
	// Create account and deposit funds
	acc, err := bkSvc.CreateAccount(ctx, userID, currency)
	assert.NoError(t, err)
	acc.Balance = 500
	acc.Available = 500
	_ = db.Save(acc)

	// Withdrawal
	tx, err := svc.InitiateWithdrawal(ctx, userID, currency, 200.0, map[string]interface{}{"account": "123"})
	assert.NoError(t, err)
	assert.Equal(t, "withdrawal", tx.Type)

	// Get withdrawals
	withs, total, err := svc.GetWithdrawals(ctx, userID, currency, 10, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, withs, 1)

	// Complete withdrawal
	err = svc.CompleteWithdrawal(ctx, tx.ID.String())
	assert.NoError(t, err)
}
