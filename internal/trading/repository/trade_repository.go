package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GormTradeRepository implements the engine.TradeRepository interface using GORM
type GormTradeRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewGormTradeRepository creates a new GORM-based trade repository
func NewGormTradeRepository(db *gorm.DB, logger *zap.Logger) engine.TradeRepository {
	return &GormTradeRepository{
		db:     db,
		logger: logger,
	}
}

// CreateTrade creates a new trade record in the database
func (r *GormTradeRepository) CreateTrade(ctx context.Context, trade *model.Trade) error {
	dbTrade := &models.Trade{
		ID:        trade.ID,
		OrderID:   trade.OrderID,
		Symbol:    trade.Pair,
		Price:     trade.Price.InexactFloat64(),
		Quantity:  trade.Quantity.InexactFloat64(),
		Side:      trade.Side,
		IsMaker:   trade.Maker,
		CreatedAt: trade.CreatedAt,
	}

	if err := r.db.WithContext(ctx).Create(dbTrade).Error; err != nil {
		r.logger.Error("Failed to create trade",
			zap.Error(err),
			zap.String("trade_id", trade.ID.String()),
			zap.String("order_id", trade.OrderID.String()),
			zap.String("pair", trade.Pair))
		return fmt.Errorf("failed to create trade: %w", err)
	}

	r.logger.Debug("Trade created successfully",
		zap.String("trade_id", trade.ID.String()),
		zap.String("order_id", trade.OrderID.String()),
		zap.String("pair", trade.Pair),
		zap.String("price", trade.Price.String()),
		zap.String("quantity", trade.Quantity.String()))

	return nil
}

// ListTradesByUser returns all trades for a given user
func (r *GormTradeRepository) ListTradesByUser(ctx context.Context, userID uuid.UUID) ([]*models.Trade, error) {
	var trades []*models.Trade
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&trades).Error
	if err != nil {
		return nil, err
	}
	return trades, nil
}
