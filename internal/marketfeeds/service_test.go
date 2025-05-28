package marketfeeds_test

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/marketfeeds"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&models.TradingPair{})
	assert.NoError(t, err)
	return db
}

func TestGetMarketPricesEmpty(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop() // Use a no-op logger
	svc, err := marketfeeds.NewService(logger, db)
	assert.NoError(t, err)

	// Start service to initialize internal state
	err = svc.Start()
	assert.NoError(t, err)

	prices, err := svc.GetMarketPrices(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, prices)
}

func TestGetMarketPricesWithPair(t *testing.T) {
	db := setupTestDB(t)
	// Insert an active trading pair
	pair := &models.TradingPair{Symbol: "ETHUSD", Status: "active"}
	_ = db.Create(pair)

	logger := zap.NewNop()
	svc, err := marketfeeds.NewService(logger, db)
	assert.NoError(t, err)

	err = svc.Start()
	assert.NoError(t, err)

	prices, err := svc.GetMarketPrices(context.Background())
	assert.NoError(t, err)
	assert.Len(t, prices, 1)
	assert.Equal(t, "ETHUSD", prices[0].Symbol)
}
