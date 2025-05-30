package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GormRepository implements the model.Repository interface using GORM
type GormRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewGormRepository creates a new GORM-based repository
func NewGormRepository(db *gorm.DB, logger *zap.Logger) model.Repository {
	return &GormRepository{
		db:     db,
		logger: logger,
	}
}

// CreateOrder creates a new order in the database
func (r *GormRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	// Convert from internal model to database model
	dbOrder := &models.Order{
		ID:             order.ID,
		UserID:         order.UserID,
		Symbol:         order.Pair,
		Side:           order.Side,
		Type:           order.Type,
		Price:          order.Price.InexactFloat64(),
		Quantity:       order.Quantity.InexactFloat64(),
		FilledQuantity: order.FilledQuantity.InexactFloat64(),
		Status:         order.Status,
		TimeInForce:    order.TimeInForce,
		CreatedAt:      order.CreatedAt,
		UpdatedAt:      order.UpdatedAt,
	}

	// Handle optional decimal fields
	if !order.StopPrice.IsZero() {
		stopPrice := order.StopPrice.InexactFloat64()
		dbOrder.StopPrice = &stopPrice
	}

	if !order.AvgPrice.IsZero() {
		avgPrice := order.AvgPrice.InexactFloat64()
		dbOrder.AveragePrice = &avgPrice
	}

	if !order.DisplayQuantity.IsZero() {
		displayQty := order.DisplayQuantity.InexactFloat64()
		dbOrder.DisplayQuantity = &displayQty
	}

	if !order.TrailingOffset.IsZero() {
		trailingOffset := order.TrailingOffset.InexactFloat64()
		dbOrder.TrailingOffset = &trailingOffset
	}

	// Handle optional time and UUID fields
	if order.ExpireAt != nil {
		dbOrder.ExpiresAt = order.ExpireAt
	}

	if order.OCOGroupID != nil {
		dbOrder.OCOGroupID = order.OCOGroupID
	}

	if order.ParentOrderID != nil {
		dbOrder.ParentOrderID = order.ParentOrderID
	}

	if err := r.db.WithContext(ctx).Create(dbOrder).Error; err != nil {
		r.logger.Error("Failed to create order", zap.Error(err), zap.String("order_id", order.ID.String()))
		return fmt.Errorf("failed to create order: %w", err)
	}

	r.logger.Debug("Order created successfully", zap.String("order_id", order.ID.String()))
	return nil
}

// GetOrderByID retrieves an order by its ID
func (r *GormRepository) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	var dbOrder models.Order
	if err := r.db.WithContext(ctx).Where("id = ?", orderID).First(&dbOrder).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return r.convertToInternalOrder(&dbOrder)
}

// GetOrder retrieves an order by its ID (alias for GetOrderByID)
func (r *GormRepository) GetOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	return r.GetOrderByID(ctx, orderID)
}

// GetOpenOrdersByPair retrieves all open orders for a trading pair
func (r *GormRepository) GetOpenOrdersByPair(ctx context.Context, pair string) ([]*model.Order, error) {
	var dbOrders []models.Order
	if err := r.db.WithContext(ctx).Where("symbol = ? AND status IN ?", pair,
		[]string{model.OrderStatusOpen, model.OrderStatusPartiallyFilled, model.OrderStatusPendingTrigger}).
		Find(&dbOrders).Error; err != nil {
		return nil, fmt.Errorf("failed to get open orders by pair: %w", err)
	}
	orders := make([]*model.Order, len(dbOrders))
	for i, dbOrder := range dbOrders {
		order, err := r.convertToInternalOrder(&dbOrder)
		if err != nil {
			r.logger.Error("Failed to convert order", zap.Error(err), zap.String("order_id", dbOrder.ID.String()))
			continue
		}
		orders[i] = order
	}

	return orders, nil
}

// GetOpenOrdersByUser retrieves all open orders for a user
func (r *GormRepository) GetOpenOrdersByUser(ctx context.Context, userID uuid.UUID) ([]*model.Order, error) {
	var dbOrders []models.Order
	if err := r.db.WithContext(ctx).Where("user_id = ? AND status IN ?", userID.String(),
		[]string{model.OrderStatusOpen, model.OrderStatusPartiallyFilled, model.OrderStatusPendingTrigger}).
		Find(&dbOrders).Error; err != nil {
		return nil, fmt.Errorf("failed to get open orders by user: %w", err)
	}
	orders := make([]*model.Order, len(dbOrders))
	for i, dbOrder := range dbOrders {
		order, err := r.convertToInternalOrder(&dbOrder)
		if err != nil {
			r.logger.Error("Failed to convert order", zap.Error(err), zap.String("order_id", dbOrder.ID.String()))
			continue
		}
		orders[i] = order
	}

	return orders, nil
}

// UpdateOrderStatus updates an order's status
func (r *GormRepository) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	result := r.db.WithContext(ctx).Model(&models.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update order status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// UpdateOrderStatusAndFilledQuantity updates an order's status and filled quantity
func (r *GormRepository) UpdateOrderStatusAndFilledQuantity(ctx context.Context, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	result := r.db.WithContext(ctx).Model(&models.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"status":          status,
			"filled_quantity": filledQty.InexactFloat64(),
			"average_price":   avgPrice.InexactFloat64(),
			"updated_at":      time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update order status and filled quantity: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// CancelOrder cancels an order
func (r *GormRepository) CancelOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.UpdateOrderStatus(ctx, orderID, model.OrderStatusCancelled)
}

// ConvertStopOrderToLimit converts a stop order to a limit order when triggered
func (r *GormRepository) ConvertStopOrderToLimit(ctx context.Context, order *model.Order) error {
	result := r.db.WithContext(ctx).Model(&models.Order{}).
		Where("id = ?", order.ID.String()).
		Updates(map[string]interface{}{
			"type":       model.OrderTypeLimit,
			"price":      order.Price,
			"status":     model.OrderStatusOpen,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to convert stop order to limit: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// CreateTrade creates a new trade record
func (r *GormRepository) CreateTrade(ctx context.Context, trade *model.Trade) error {
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
		r.logger.Error("Failed to create trade", zap.Error(err), zap.String("trade_id", trade.ID.String()))
		return fmt.Errorf("failed to create trade: %w", err)
	}

	r.logger.Debug("Trade created successfully", zap.String("trade_id", trade.ID.String()))
	return nil
}

// CreateTradeTx creates a trade within a transaction
func (r *GormRepository) CreateTradeTx(ctx context.Context, tx *gorm.DB, trade *model.Trade) error {
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

	if err := tx.WithContext(ctx).Create(dbTrade).Error; err != nil {
		return fmt.Errorf("failed to create trade in transaction: %w", err)
	}

	return nil
}

// UpdateOrderStatusTx updates order status within a transaction
func (r *GormRepository) UpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	result := tx.WithContext(ctx).Model(&models.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"status":          status,
			"filled_quantity": filledQty.InexactFloat64(),
			"average_price":   avgPrice.InexactFloat64(),
			"updated_at":      time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update order status in transaction: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// BatchUpdateOrderStatusTx updates multiple order statuses within a transaction
func (r *GormRepository) BatchUpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, updates []struct {
	OrderID uuid.UUID
	Status  string
}) error {
	for _, update := range updates {
		result := tx.WithContext(ctx).Model(&models.Order{}).
			Where("id = ?", update.OrderID).
			Update("status", update.Status)

		if result.Error != nil {
			return fmt.Errorf("failed to batch update order status: %w", result.Error)
		}
	}

	return nil
}

// ExecuteInTransaction executes a function within a database transaction
func (r *GormRepository) ExecuteInTransaction(ctx context.Context, txFunc func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(txFunc)
}

// UpdateOrderHoldID updates the hold ID for an order (for bookkeeper integration)
func (r *GormRepository) UpdateOrderHoldID(ctx context.Context, orderID uuid.UUID, holdID string) error {
	result := r.db.WithContext(ctx).Model(&models.Order{}).
		Where("id = ?", orderID).
		Update("hold_id", holdID)

	if result.Error != nil {
		return fmt.Errorf("failed to update order hold ID: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// GetExpiredGTDOrders retrieves expired Good Till Date orders
func (r *GormRepository) GetExpiredGTDOrders(ctx context.Context, pair string, now time.Time) ([]*model.Order, error) {
	var dbOrders []models.Order
	if err := r.db.WithContext(ctx).Where("symbol = ? AND type = ? AND expires_at < ? AND status IN ?",
		pair, model.OrderTypeGTD, now, []string{model.OrderStatusOpen, model.OrderStatusPartiallyFilled}).
		Find(&dbOrders).Error; err != nil {
		return nil, fmt.Errorf("failed to get expired GTD orders: %w", err)
	}
	orders := make([]*model.Order, len(dbOrders))
	for i, dbOrder := range dbOrders {
		order, err := r.convertToInternalOrder(&dbOrder)
		if err != nil {
			r.logger.Error("Failed to convert expired order", zap.Error(err), zap.String("order_id", dbOrder.ID.String()))
			continue
		}
		orders[i] = order
	}

	return orders, nil
}

// GetOCOSiblingOrder retrieves the sibling order in an OCO group
func (r *GormRepository) GetOCOSiblingOrder(ctx context.Context, ocoGroupID uuid.UUID, excludeOrderID uuid.UUID) (*model.Order, error) {
	var dbOrder models.Order
	if err := r.db.WithContext(ctx).Where("oco_group_id = ? AND id != ? AND status IN ?",
		ocoGroupID, excludeOrderID,
		[]string{model.OrderStatusOpen, model.OrderStatusPartiallyFilled, model.OrderStatusPendingTrigger}).
		First(&dbOrder).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No sibling found
		}
		return nil, fmt.Errorf("failed to get OCO sibling order: %w", err)
	}

	return r.convertToInternalOrder(&dbOrder)
}

// GetOpenTrailingStopOrders retrieves all open trailing stop orders for a pair
func (r *GormRepository) GetOpenTrailingStopOrders(ctx context.Context, pair string) ([]*model.Order, error) {
	var dbOrders []models.Order
	if err := r.db.WithContext(ctx).Where("symbol = ? AND type = ? AND status IN ?",
		pair, model.OrderTypeTrailing, []string{model.OrderStatusOpen, model.OrderStatusPendingTrigger}).
		Find(&dbOrders).Error; err != nil {
		return nil, fmt.Errorf("failed to get trailing stop orders: %w", err)
	}
	orders := make([]*model.Order, len(dbOrders))
	for i, dbOrder := range dbOrders {
		order, err := r.convertToInternalOrder(&dbOrder)
		if err != nil {
			r.logger.Error("Failed to convert trailing stop order", zap.Error(err), zap.String("order_id", dbOrder.ID.String()))
			continue
		}
		orders[i] = order
	}

	return orders, nil
}

// UpdateOrder updates all fields of an order in the database
func (r *GormRepository) UpdateOrder(ctx context.Context, order *model.Order) error {
	updates := map[string]interface{}{
		"user_id":         order.UserID,
		"symbol":          order.Pair,
		"side":            order.Side,
		"type":            order.Type,
		"price":           order.Price.InexactFloat64(),
		"quantity":        order.Quantity.InexactFloat64(),
		"filled_quantity": order.FilledQuantity.InexactFloat64(),
		"status":          order.Status,
		"time_in_force":   order.TimeInForce,
		"updated_at":      time.Now(),
	}
	if !order.StopPrice.IsZero() {
		stopPrice := order.StopPrice.InexactFloat64()
		updates["stop_price"] = stopPrice
	}
	if !order.AvgPrice.IsZero() {
		avgPrice := order.AvgPrice.InexactFloat64()
		updates["average_price"] = avgPrice
	}
	if !order.DisplayQuantity.IsZero() {
		displayQty := order.DisplayQuantity.InexactFloat64()
		updates["display_quantity"] = displayQty
	}
	if !order.TrailingOffset.IsZero() {
		trailingOffset := order.TrailingOffset.InexactFloat64()
		updates["trailing_offset"] = trailingOffset
	}
	if order.ExpireAt != nil {
		updates["expires_at"] = order.ExpireAt
	}
	if order.OCOGroupID != nil {
		updates["oco_group_id"] = order.OCOGroupID
	}
	if order.ParentOrderID != nil {
		updates["parent_order_id"] = order.ParentOrderID
	}
	result := r.db.WithContext(ctx).Model(&models.Order{}).
		Where("id = ?", order.ID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update order: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// convertToInternalOrder converts a database model to internal trading model
func (r *GormRepository) convertToInternalOrder(dbOrder *models.Order) (*model.Order, error) {
	order := &model.Order{
		ID:             dbOrder.ID,
		UserID:         dbOrder.UserID,
		Pair:           dbOrder.Symbol,
		Side:           dbOrder.Side,
		Type:           dbOrder.Type,
		Price:          decimal.NewFromFloat(dbOrder.Price),
		Quantity:       decimal.NewFromFloat(dbOrder.Quantity),
		FilledQuantity: decimal.NewFromFloat(dbOrder.FilledQuantity),
		Status:         dbOrder.Status,
		CreatedAt:      dbOrder.CreatedAt,
		UpdatedAt:      dbOrder.UpdatedAt,
		TimeInForce:    dbOrder.TimeInForce,
		ExpireAt:       dbOrder.ExpiresAt,
	}
	// Handle optional fields
	if dbOrder.StopPrice != nil {
		order.StopPrice = decimal.NewFromFloat(*dbOrder.StopPrice)
	}
	if dbOrder.AveragePrice != nil {
		order.AvgPrice = decimal.NewFromFloat(*dbOrder.AveragePrice)
	}
	if dbOrder.DisplayQuantity != nil {
		order.DisplayQuantity = decimal.NewFromFloat(*dbOrder.DisplayQuantity)
	}
	if dbOrder.TrailingOffset != nil {
		order.TrailingOffset = decimal.NewFromFloat(*dbOrder.TrailingOffset)
	}
	if dbOrder.OCOGroupID != nil {
		order.OCOGroupID = dbOrder.OCOGroupID
	}
	if dbOrder.ParentOrderID != nil {
		order.ParentOrderID = dbOrder.ParentOrderID
	}
	return order, nil
}
