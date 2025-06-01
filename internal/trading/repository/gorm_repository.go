package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GormRepository implements the model.Repository interface using GORM
type GormRepository struct {
	db             *gorm.DB
	logger         *zap.Logger
	cache          *redis.Client
	enableBatchOps bool
	batchSize      int
}

// NewGormRepository creates a new GORM-based repository
func NewGormRepository(db *gorm.DB, logger *zap.Logger) model.Repository {
	// Try to initialize Redis cache (optional)
	var cache *redis.Client
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis not available, proceeding without cache", zap.Error(err))
		cache = nil
	} else {
		cache = rdb
		logger.Info("Redis cache initialized successfully")
	}

	return &GormRepository{
		db:             db,
		logger:         logger,
		cache:          cache,
		enableBatchOps: true,
		batchSize:      100,
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

// Batch Operations for N+1 Query Resolution

// BatchGetOrdersByIDs retrieves multiple orders by IDs efficiently (resolves N+1 queries)
func (r *GormRepository) BatchGetOrdersByIDs(ctx context.Context, orderIDs []uuid.UUID) (map[uuid.UUID]*model.Order, error) {
	if len(orderIDs) == 0 {
		return make(map[uuid.UUID]*model.Order), nil
	}

	start := time.Now()
	defer func() {
		r.logger.Debug("BatchGetOrdersByIDs completed",
			zap.Int("order_count", len(orderIDs)),
			zap.Duration("duration", time.Since(start)))
	}()

	// Check cache first if available
	result := make(map[uuid.UUID]*model.Order)
	uncachedIDs := make([]uuid.UUID, 0, len(orderIDs))

	if r.cache != nil {
		for _, orderID := range orderIDs {
			cacheKey := fmt.Sprintf("order:%s", orderID.String())
			cached, err := r.cache.Get(ctx, cacheKey).Result()
			if err == nil {
				var dbOrder models.Order
				if err := json.Unmarshal([]byte(cached), &dbOrder); err == nil {
					order, err := r.convertToInternalOrder(&dbOrder)
					if err == nil {
						result[orderID] = order
						continue
					}
				}
			}
			uncachedIDs = append(uncachedIDs, orderID)
		}
	} else {
		uncachedIDs = orderIDs
	}

	// Batch query for uncached orders
	if len(uncachedIDs) > 0 {
		var dbOrders []models.Order
		if err := r.db.WithContext(ctx).Where("id IN ?", uncachedIDs).Find(&dbOrders).Error; err != nil {
			return nil, fmt.Errorf("failed to batch get orders: %w", err)
		}

		for _, dbOrder := range dbOrders {
			order, err := r.convertToInternalOrder(&dbOrder)
			if err != nil {
				r.logger.Error("Failed to convert order", zap.Error(err))
				continue
			}
			result[dbOrder.ID] = order

			// Cache the order if cache is available
			if r.cache != nil {
				if orderData, err := json.Marshal(dbOrder); err == nil {
					cacheKey := fmt.Sprintf("order:%s", dbOrder.ID.String())
					r.cache.Set(ctx, cacheKey, orderData, 30*time.Second)
				}
			}
		}
	}

	return result, nil
}

// BatchGetUserOrdersOptimized retrieves orders for multiple users efficiently
func (r *GormRepository) BatchGetUserOrdersOptimized(ctx context.Context, userIDs []uuid.UUID, statuses []string, limit int) (map[uuid.UUID][]*model.Order, error) {
	if len(userIDs) == 0 {
		return make(map[uuid.UUID][]*model.Order), nil
	}

	start := time.Now()
	defer func() {
		r.logger.Debug("BatchGetUserOrdersOptimized completed",
			zap.Int("user_count", len(userIDs)),
			zap.Duration("duration", time.Since(start)))
	}()

	// Build optimized query with proper indexing hints
	query := r.db.WithContext(ctx).
		Where("user_id IN ?", userIDs)

	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}

	// Add ordering and limit
	var dbOrders []models.Order
	if err := query.Order("user_id, created_at DESC").
		Limit(limit).
		Find(&dbOrders).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get user orders: %w", err)
	}

	// Group orders by user ID
	result := make(map[uuid.UUID][]*model.Order)
	for _, dbOrder := range dbOrders {
		order, err := r.convertToInternalOrder(&dbOrder)
		if err != nil {
			r.logger.Error("Failed to convert order", zap.Error(err))
			continue
		}

		userID := dbOrder.UserID
		if result[userID] == nil {
			result[userID] = make([]*model.Order, 0)
		}
		result[userID] = append(result[userID], order)
	}

	return result, nil
}

// BatchCreateOrdersOptimized creates multiple orders efficiently
func (r *GormRepository) BatchCreateOrdersOptimized(ctx context.Context, orders []*model.Order) error {
	if len(orders) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		r.logger.Debug("BatchCreateOrdersOptimized completed",
			zap.Int("order_count", len(orders)),
			zap.Duration("duration", time.Since(start)))
	}()

	// Convert to database models
	dbOrders := make([]models.Order, len(orders))
	for i, order := range orders {
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

		// Handle optional fields
		if !order.StopPrice.IsZero() {
			stopPrice := order.StopPrice.InexactFloat64()
			dbOrder.StopPrice = &stopPrice
		}
		if !order.AvgPrice.IsZero() {
			avgPrice := order.AvgPrice.InexactFloat64()
			dbOrder.AveragePrice = &avgPrice
		}
		if order.ExpireAt != nil {
			dbOrder.ExpiresAt = order.ExpireAt
		}
		if order.OCOGroupID != nil {
			dbOrder.OCOGroupID = order.OCOGroupID
		}
		if order.ParentOrderID != nil {
			dbOrder.ParentOrderID = order.ParentOrderID
		}

		dbOrders[i] = *dbOrder
	}

	// Batch insert with optimal batch size
	batchSize := r.batchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	if err := r.db.WithContext(ctx).CreateInBatches(dbOrders, batchSize).Error; err != nil {
		return fmt.Errorf("failed to batch create orders: %w", err)
	}

	return nil
}

// BatchUpdateOrderStatusOptimized updates multiple order statuses efficiently using CASE statements
func (r *GormRepository) BatchUpdateOrderStatusOptimized(ctx context.Context, updates map[uuid.UUID]string) error {
	if len(updates) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		r.logger.Debug("BatchUpdateOrderStatusOptimized completed",
			zap.Int("update_count", len(updates)),
			zap.Duration("duration", time.Since(start)))
	}()

	// Build CASE statement for efficient bulk update
	orderIDs := make([]uuid.UUID, 0, len(updates))
	for orderID := range updates {
		orderIDs = append(orderIDs, orderID)
	}

	// Use CASE-based bulk update for better performance
	caseStmt := "CASE id"
	args := []interface{}{
		time.Now(),
	}
	argIndex := 1

	for orderID, status := range updates {
		caseStmt += fmt.Sprintf(" WHEN $%d THEN $%d", argIndex, argIndex+1)
		args = append(args, orderID, status)
		argIndex += 2
	}
	caseStmt += " ELSE status END"

	updateQuery := fmt.Sprintf("UPDATE orders SET status = %s, updated_at = $%d WHERE id IN (?)", caseStmt, argIndex)
	args = append(args, orderIDs)

	if err := r.db.WithContext(ctx).Exec(updateQuery, args...).Error; err != nil {
		return fmt.Errorf("failed to batch update order statuses: %w", err)
	}

	// Invalidate cache if available
	if r.cache != nil {
		for orderID := range updates {
			cacheKey := fmt.Sprintf("order:%s", orderID.String())
			r.cache.Del(ctx, cacheKey)
		}
	}

	return nil
}

// BatchGetOpenOrdersByUsers retrieves open orders for multiple users (resolves N+1 in bookkeeper service)
func (r *GormRepository) BatchGetOpenOrdersByUsers(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]*model.Order, error) {
	openStatuses := []string{model.OrderStatusOpen, model.OrderStatusPartiallyFilled, model.OrderStatusPendingTrigger}
	return r.BatchGetUserOrdersOptimized(ctx, userIDs, openStatuses, 1000)
}

// GetOrdersByIDsWithCache retrieves orders with fallback to individual queries if batch fails
func (r *GormRepository) GetOrdersByIDsWithCache(ctx context.Context, orderIDs []uuid.UUID) ([]*model.Order, error) {
	if !r.enableBatchOps {
		// Fallback to individual queries
		orders := make([]*model.Order, 0, len(orderIDs))
		for _, orderID := range orderIDs {
			order, err := r.GetOrderByID(ctx, orderID)
			if err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			if order != nil {
				orders = append(orders, order)
			}
		}
		return orders, nil
	}

	// Use batch operation
	orderMap, err := r.BatchGetOrdersByIDs(ctx, orderIDs)
	if err != nil {
		return nil, err
	}

	orders := make([]*model.Order, 0, len(orderMap))
	for _, order := range orderMap {
		orders = append(orders, order)
	}

	return orders, nil
}

// Additional helper methods for improved performance

// InvalidateOrderCache removes an order from cache
func (r *GormRepository) InvalidateOrderCache(ctx context.Context, orderID uuid.UUID) {
	if r.cache != nil {
		cacheKey := fmt.Sprintf("order:%s", orderID.String())
		r.cache.Del(ctx, cacheKey)
	}
}

// WarmupOrderCache preloads frequently accessed orders into cache
func (r *GormRepository) WarmupOrderCache(ctx context.Context, pairs []string) error {
	if r.cache == nil {
		return nil // No cache available
	}

	for _, pair := range pairs {
		orders, err := r.GetOpenOrdersByPair(ctx, pair)
		if err != nil {
			r.logger.Warn("Failed to warmup cache for pair", zap.String("pair", pair), zap.Error(err))
			continue
		}

		// Cache recent orders
		for _, order := range orders {
			if time.Since(order.CreatedAt) < 5*time.Minute {
				if orderData, err := json.Marshal(order); err == nil {
					cacheKey := fmt.Sprintf("order:%s", order.ID.String())
					r.cache.Set(ctx, cacheKey, orderData, 30*time.Second)
				}
			}
		}
	}

	return nil
}
