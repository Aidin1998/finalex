// moved from internal/bookkeeper/service_test.go
package api_test

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBookkeeperTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err, "failed to open in-memory sqlite DB")

	err = db.AutoMigrate(&models.Account{}, &models.Transaction{})
	assert.NoError(t, err, "failed to migrate models")
	return db
}

func TestCreateAndGetAccount(t *testing.T) {
	db := setupBookkeeperTestDB(t)
	logger := zap.NewNop()
	svc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)

	ctx := context.Background()
	userID := uuid.New().String()
	currency := "USD"

	// Create account
	account, err := svc.CreateAccount(ctx, userID, currency)
	assert.NoError(t, err)
	assert.Equal(t, userID, account.UserID.String())
	assert.Equal(t, currency, account.Currency)

	// Get accounts
	accounts, err := svc.GetAccounts(ctx, userID)
	assert.NoError(t, err)
	assert.Len(t, accounts, 1)
	assert.Equal(t, account.ID, accounts[0].ID)

	// Get single account
	single, err := svc.GetAccount(ctx, userID, currency)
	assert.NoError(t, err)
	assert.Equal(t, account.ID, single.ID)
}

func TestLockAndUnlockFunds(t *testing.T) {
	db := setupBookkeeperTestDB(t)
	logger := zap.NewNop()
	svc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)

	ctx := context.Background()
	userID := uuid.New().String()
	currency := "BTC"

	// Create account and deposit some balance
	acc, err := svc.CreateAccount(ctx, userID, currency)
	assert.NoError(t, err)
	acc.Balance = 10
	acc.Available = 10
	db.Save(acc)

	// Lock funds
	err = svc.LockFunds(ctx, userID, currency, 5)
	assert.NoError(t, err)
	updated, _ := svc.GetAccount(ctx, userID, currency)
	assert.Equal(t, 5.0, updated.Available)
	assert.Equal(t, 5.0, updated.Locked)

	// Unlock funds
	err = svc.UnlockFunds(ctx, userID, currency, 3)
	assert.NoError(t, err)
	updated2, _ := svc.GetAccount(ctx, userID, currency)
	assert.Equal(t, 8.0, updated2.Available)
	assert.Equal(t, 2.0, updated2.Locked)
}

func TestCreateAndCompleteTransaction(t *testing.T) {
	db := setupBookkeeperTestDB(t)
	logger := zap.NewNop()
	svc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)

	ctx := context.Background()
	userID := uuid.New().String()
	currency := "ETH"

	// Create account
	_, err = svc.CreateAccount(ctx, userID, currency)
	assert.NoError(t, err)

	// Initiate transaction (deposit)
	tx, err := svc.CreateTransaction(ctx, userID, "deposit", 2.5, currency, "ref123", "test deposit")
	assert.NoError(t, err)
	assert.Equal(t, "pending", tx.Status)

	// Complete transaction
	err = svc.CompleteTransaction(ctx, tx.ID.String())
	assert.NoError(t, err)

	// Verify balance updated
	updated, _ := svc.GetAccount(ctx, userID, currency)
	assert.Equal(t, 2.5, updated.Balance)
	assert.Equal(t, 2.5, updated.Available)
}
