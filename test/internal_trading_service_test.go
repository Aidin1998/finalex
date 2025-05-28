package api

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	// Migrate models
	err = db.AutoMigrate(&models.TradingPair{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{})
	assert.NoError(t, err)
	return db
}

func TestTradingServicePlaceAndCancelOrder(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()

	// Set up bookkeeper and initial account and funds
	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(t, err)
	ctx := context.Background()
	userID := uuid.New().String()
	currency := "USD"
	// create account with balance
	acct, err := bkSvc.CreateAccount(ctx, userID, currency)
	assert.NoError(t, err)
	acct.Balance = 1000
	acct.Available = 1000
	_ = db.Save(acct)

	// Create trading pair
	pair := &models.TradingPair{Symbol: "USDUSD", BaseCurrency: "USD", QuoteCurrency: "USD", Status: "active"}
	err = db.Create(pair).Error
	assert.NoError(t, err)

	// Create trading service
	svc, err := trading.NewService(logger, db, bkSvc)
	assert.NoError(t, err)
	// Start engine
	err = svc.Start()
	assert.NoError(t, err)

	// Place buy order
	order := &models.Order{
		UserID:   uuid.MustParse(userID),
		Symbol:   "USDUSD",
		Side:     "buy",
		Type:     "limit",
		Price:    1,
		Quantity: 10,
	}
	placed, err := svc.PlaceOrder(ctx, order)
	assert.NoError(t, err)
	assert.Equal(t, "filled", placed.Status)

	// Cancel order - expect no error even if filled
	err = svc.CancelOrder(ctx, placed.ID.String())
	assert.NoError(t, err)

	// Stop service
	err = svc.Stop()
	assert.NoError(t, err)
}
