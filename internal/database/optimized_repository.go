package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// OptimizedRepository provides N+1 query resolution and batch operations
type OptimizedRepository struct {
	db        *ReadWriteDB
	cache     *redis.Client
	logger    *zap.Logger
	optimizer *QueryOptimizer

	// Cache configurations
	orderCacheTTL    time.Duration
	tradeCacheTTL    time.Duration
	userOrdersTTL    time.Duration
	accountCacheTTL  time.Duration
	batchSize        int
	enableQueryHints bool
	enableDebugLogs  bool
}

// OptimizedRepositoryConfig holds configuration for the optimized repository
type OptimizedRepositoryConfig struct {
	OrderCacheTTL    time.Duration `yaml:"order_cache_ttl" json:"order_cache_ttl"`
	TradeCacheTTL    time.Duration `yaml:"trade_cache_ttl" json:"trade_cache_ttl"`
	UserOrdersTTL    time.Duration `yaml:"user_orders_ttl" json:"user_orders_ttl"`
	AccountCacheTTL  time.Duration `yaml:"account_cache_ttl" json:"account_cache_ttl"`
	BatchSize        int           `yaml:"batch_size" json:"batch_size"`
	EnableQueryHints bool          `yaml:"enable_query_hints" json:"enable_query_hints"`
	EnableDebugLogs  bool          `yaml:"enable_debug_logs" json:"enable_debug_logs"`
}

// DefaultOptimizedRepositoryConfig returns default configuration
func DefaultOptimizedRepositoryConfig() *OptimizedRepositoryConfig {
	return &OptimizedRepositoryConfig{
		OrderCacheTTL:    30 * time.Second,
		TradeCacheTTL:    2 * time.Minute,
		UserOrdersTTL:    15 * time.Second,
		AccountCacheTTL:  1 * time.Minute,
		BatchSize:        100,
		EnableQueryHints: true,
		EnableDebugLogs:  false,
	}
}

// NewOptimizedRepository creates a new optimized repository
func NewOptimizedRepository(
	db *ReadWriteDB,
	cache *redis.Client,
	logger *zap.Logger,
	optimizer *QueryOptimizer,
	config *OptimizedRepositoryConfig,
) *OptimizedRepository {
	if config == nil {
		config = DefaultOptimizedRepositoryConfig()
	}

	return &OptimizedRepository{
		db:               db,
		cache:            cache,
		logger:           logger,
		optimizer:        optimizer,
		orderCacheTTL:    config.OrderCacheTTL,
		tradeCacheTTL:    config.TradeCacheTTL,
		userOrdersTTL:    config.UserOrdersTTL,
		accountCacheTTL:  config.AccountCacheTTL,
		batchSize:        config.BatchSize,
		enableQueryHints: config.EnableQueryHints,
		enableDebugLogs:  config.EnableDebugLogs,
	}
}

// BatchGetOrdersByIDs resolves N+1 queries for order retrieval
func (r *OptimizedRepository) BatchGetOrdersByIDs(ctx context.Context, orderIDs []uuid.UUID) (map[uuid.UUID]*model.Order, error) {
	if len(orderIDs) == 0 {
		return make(map[uuid.UUID]*model.Order), nil
	}

	start := time.Now()
	defer func() {
		if r.enableDebugLogs {
			r.logger.Debug("BatchGetOrdersByIDs completed",
				zap.Int("order_count", len(orderIDs)),
				zap.Duration("duration", time.Since(start)))
		}
	}()

	// Check cache first for all orders
	cachedOrders := make(map[uuid.UUID]*model.Order)
	uncachedIDs := make([]uuid.UUID, 0, len(orderIDs))

	for _, orderID := range orderIDs {
		cacheKey := fmt.Sprintf("order:%s", orderID.String())
		cached, err := r.cache.Get(ctx, cacheKey).Result()
		if err == nil {
			var order model.Order
			if err := json.Unmarshal([]byte(cached), &order); err == nil {
				cachedOrders[orderID] = &order
				continue
			}
		}
		uncachedIDs = append(uncachedIDs, orderID)
	}

	// Batch query for uncached orders
	result := cachedOrders
	if len(uncachedIDs) > 0 {
		// Build optimized batch query with index hints
		placeholders := make([]string, len(uncachedIDs))
		args := make([]interface{}, len(uncachedIDs))
		for i, id := range uncachedIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}

		query := fmt.Sprintf(`
			SELECT /*+ INDEX(orders_pkey) */ 
			       id, user_id, symbol, side, type, price, quantity, filled_quantity, 
			       status, time_in_force, stop_price, average_price, display_quantity,
			       trailing_offset, expires_at, oco_group_id, parent_order_id, 
			       created_at, updated_at
			FROM orders 
			WHERE id IN (%s)
		`, strings.Join(placeholders, ","))

		var dbOrders []models.Order
		if err := r.db.Read().WithContext(ctx).Raw(query, args...).Scan(&dbOrders).Error; err != nil {
			return nil, fmt.Errorf("failed to batch get orders: %w", err)
		}

		// Convert and cache results
		for _, dbOrder := range dbOrders {
			order, err := r.convertToInternalOrder(&dbOrder)
			if err != nil {
				r.logger.Error("Failed to convert order", zap.Error(err))
				continue
			}
			result[dbOrder.ID] = order

			// Cache the order
			if orderData, err := json.Marshal(order); err == nil {
				cacheKey := fmt.Sprintf("order:%s", dbOrder.ID.String())
				r.cache.Set(ctx, cacheKey, orderData, r.orderCacheTTL)
			}
		}
	}

	r.logger.Debug("Batch order retrieval completed",
		zap.Int("total_requested", len(orderIDs)),
		zap.Int("cache_hits", len(cachedOrders)),
		zap.Int("db_queries", len(uncachedIDs)),
		zap.Duration("duration", time.Since(start)))

	return result, nil
}

// BatchGetUserOrdersOptimized retrieves orders for multiple users efficiently
func (r *OptimizedRepository) BatchGetUserOrdersOptimized(ctx context.Context, userIDs []uuid.UUID, statuses []string, limit int) (map[uuid.UUID][]*model.Order, error) {
	if len(userIDs) == 0 {
		return make(map[uuid.UUID][]*model.Order), nil
	}

	start := time.Now()
	result := make(map[uuid.UUID][]*model.Order)

	// Build optimized query with covering index
	userPlaceholders := make([]string, len(userIDs))
	statusPlaceholders := make([]string, len(statuses))
	args := make([]interface{}, 0, len(userIDs)+len(statuses)+1)

	for i, userID := range userIDs {
		userPlaceholders[i] = fmt.Sprintf("$%d", len(args)+1)
		args = append(args, userID)
	}

	for i, status := range statuses {
		statusPlaceholders[i] = fmt.Sprintf("$%d", len(args)+1)
		args = append(args, status)
	}

	limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT /*+ INDEX(orders_user_status_created_idx) */ 
		       id, user_id, symbol, side, type, price, quantity, filled_quantity, 
		       status, time_in_force, created_at, updated_at
		FROM orders 
		WHERE user_id IN (%s) 
		  AND status IN (%s)
		ORDER BY user_id, created_at DESC
		LIMIT %s
	`, strings.Join(userPlaceholders, ","), strings.Join(statusPlaceholders, ","), limitPlaceholder)

	var dbOrders []models.Order
	if err := r.db.Read().WithContext(ctx).Raw(query, args...).Scan(&dbOrders).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get user orders: %w", err)
	}

	// Group orders by user
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

	r.logger.Debug("Batch user orders retrieval completed",
		zap.Int("user_count", len(userIDs)),
		zap.Int("total_orders", len(dbOrders)),
		zap.Duration("duration", time.Since(start)))

	return result, nil
}

// BatchGetTradesByOrderIDs resolves N+1 queries for trade retrieval
func (r *OptimizedRepository) BatchGetTradesByOrderIDs(ctx context.Context, orderIDs []uuid.UUID) (map[uuid.UUID][]*model.Trade, error) {
	if len(orderIDs) == 0 {
		return make(map[uuid.UUID][]*model.Trade), nil
	}

	start := time.Now()

	// Build batch query for trades
	placeholders := make([]string, len(orderIDs))
	args := make([]interface{}, len(orderIDs))
	for i, id := range orderIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT /*+ INDEX(trades_order_id_idx) */ 
		       id, order_id, symbol, price, quantity, side, is_maker, created_at
		FROM trades 
		WHERE order_id IN (%s)
		ORDER BY order_id, created_at DESC
	`, strings.Join(placeholders, ","))

	var dbTrades []models.Trade
	if err := r.db.Read().WithContext(ctx).Raw(query, args...).Scan(&dbTrades).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get trades: %w", err)
	}

	// Group trades by order ID
	result := make(map[uuid.UUID][]*model.Trade)
	for _, dbTrade := range dbTrades {
		trade := &model.Trade{
			ID:        dbTrade.ID,
			OrderID:   dbTrade.OrderID,
			Pair:      dbTrade.Symbol,
			Price:     decimal.NewFromFloat(dbTrade.Price),
			Quantity:  decimal.NewFromFloat(dbTrade.Quantity),
			Side:      dbTrade.Side,
			Maker:     dbTrade.IsMaker,
			CreatedAt: dbTrade.CreatedAt,
		}

		orderID := dbTrade.OrderID
		if result[orderID] == nil {
			result[orderID] = make([]*model.Trade, 0)
		}
		result[orderID] = append(result[orderID], trade)
	}

	r.logger.Debug("Batch trade retrieval completed",
		zap.Int("order_count", len(orderIDs)),
		zap.Int("total_trades", len(dbTrades)),
		zap.Duration("duration", time.Since(start)))

	return result, nil
}

// BatchGetAccountsOptimized resolves N+1 queries for account retrieval
func (r *OptimizedRepository) BatchGetAccountsOptimized(ctx context.Context, userIDs []uuid.UUID, currencies []string) (map[uuid.UUID]map[string]*models.Account, error) {
	if len(userIDs) == 0 {
		return make(map[uuid.UUID]map[string]*models.Account), nil
	}

	start := time.Now()

	// Build batch query for accounts
	userPlaceholders := make([]string, len(userIDs))
	currencyPlaceholders := make([]string, len(currencies))
	args := make([]interface{}, 0, len(userIDs)+len(currencies))

	for i, userID := range userIDs {
		userPlaceholders[i] = fmt.Sprintf("$%d", len(args)+1)
		args = append(args, userID)
	}

	currencyClause := ""
	if len(currencies) > 0 {
		for i, currency := range currencies {
			currencyPlaceholders[i] = fmt.Sprintf("$%d", len(args)+1)
			args = append(args, currency)
		}
		currencyClause = fmt.Sprintf(" AND currency IN (%s)", strings.Join(currencyPlaceholders, ","))
	}

	query := fmt.Sprintf(`
		SELECT /*+ INDEX(accounts_user_currency_idx) */ 
		       id, user_id, currency, balance, available, locked, created_at, updated_at
		FROM accounts 
		WHERE user_id IN (%s)%s
		ORDER BY user_id, currency
	`, strings.Join(userPlaceholders, ","), currencyClause)

	var accounts []models.Account
	if err := r.db.Read().WithContext(ctx).Raw(query, args...).Scan(&accounts).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get accounts: %w", err)
	}

	// Group accounts by user and currency
	result := make(map[uuid.UUID]map[string]*models.Account)
	for _, account := range accounts {
		userID := account.UserID
		if result[userID] == nil {
			result[userID] = make(map[string]*models.Account)
		}
		result[userID][account.Currency] = &account
	}

	r.logger.Debug("Batch account retrieval completed",
		zap.Int("user_count", len(userIDs)),
		zap.Int("total_accounts", len(accounts)),
		zap.Duration("duration", time.Since(start)))

	return result, nil
}

// BatchCreateOrdersOptimized creates multiple orders in a single transaction
func (r *OptimizedRepository) BatchCreateOrdersOptimized(ctx context.Context, orders []*model.Order) error {
	if len(orders) == 0 {
		return nil
	}

	start := time.Now()

	// Process in batches to avoid overwhelming the database
	for i := 0; i < len(orders); i += r.batchSize {
		end := i + r.batchSize
		if end > len(orders) {
			end = len(orders)
		}

		batch := orders[i:end]
		if err := r.createOrderBatch(ctx, batch); err != nil {
			return fmt.Errorf("failed to create order batch %d-%d: %w", i, end-1, err)
		}
	}

	r.logger.Debug("Batch order creation completed",
		zap.Int("order_count", len(orders)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// createOrderBatch creates a batch of orders in a single transaction
func (r *OptimizedRepository) createOrderBatch(ctx context.Context, orders []*model.Order) error {
	if len(orders) == 0 {
		return nil
	}

	// Build batch insert query
	values := make([]string, len(orders))
	args := make([]interface{}, 0, len(orders)*19)

	for i, order := range orders {
		values[i] = fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i*19+1, i*19+2, i*19+3, i*19+4, i*19+5, i*19+6, i*19+7, i*19+8, i*19+9, i*19+10,
			i*19+11, i*19+12, i*19+13, i*19+14, i*19+15, i*19+16, i*19+17, i*19+18, i*19+19)

		args = append(args,
			order.ID, order.UserID, order.Pair, order.Side, order.Type,
			order.Price.InexactFloat64(), order.Quantity.InexactFloat64(), order.FilledQuantity.InexactFloat64(),
			order.Status, order.TimeInForce,
			r.getFloatPtr(order.StopPrice), r.getFloatPtr(order.AvgPrice), r.getFloatPtr(order.DisplayQuantity),
			r.getFloatPtr(order.TrailingOffset), order.ExpireAt, order.OCOGroupID, order.ParentOrderID,
			order.CreatedAt, order.UpdatedAt)
	}

	query := fmt.Sprintf(`
		INSERT INTO orders (
			id, user_id, symbol, side, type, price, quantity, filled_quantity,
			status, time_in_force, stop_price, average_price, display_quantity,
			trailing_offset, expires_at, oco_group_id, parent_order_id,
			created_at, updated_at
		) VALUES %s
		ON CONFLICT (id) DO NOTHING
	`, strings.Join(values, ","))

	return r.db.Write().WithContext(ctx).Exec(query, args...).Error
}

// BatchUpdateOrderStatusOptimized updates multiple order statuses efficiently
func (r *OptimizedRepository) BatchUpdateOrderStatusOptimized(ctx context.Context, updates []OrderStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	start := time.Now()

	// Build batch update using CASE statements for maximum efficiency
	orderIDs := make([]string, len(updates))
	statusCases := make([]string, len(updates))
	filledQtyCases := make([]string, len(updates))
	avgPriceCases := make([]string, len(updates))
	args := make([]interface{}, 0, len(updates)*4)

	for i, update := range updates {
		argIndex := i*4 + 1
		orderIDs[i] = fmt.Sprintf("$%d", argIndex)
		statusCases[i] = fmt.Sprintf("WHEN id = $%d THEN $%d", argIndex, argIndex+1)
		filledQtyCases[i] = fmt.Sprintf("WHEN id = $%d THEN $%d", argIndex, argIndex+2)
		avgPriceCases[i] = fmt.Sprintf("WHEN id = $%d THEN $%d", argIndex, argIndex+3)

		args = append(args, update.OrderID, update.Status, update.FilledQuantity.InexactFloat64(), update.AvgPrice.InexactFloat64())
	}

	query := fmt.Sprintf(`
		UPDATE orders SET 
			status = CASE %s END,
			filled_quantity = CASE %s END,
			average_price = CASE %s END,
			updated_at = NOW()
		WHERE id IN (%s)
	`, strings.Join(statusCases, " "), strings.Join(filledQtyCases, " "), strings.Join(avgPriceCases, " "), strings.Join(orderIDs, ","))

	if err := r.db.Write().WithContext(ctx).Exec(query, args...).Error; err != nil {
		return fmt.Errorf("failed to batch update order statuses: %w", err)
	}

	// Invalidate caches for updated orders
	go r.invalidateOrderCaches(ctx, updates)

	r.logger.Debug("Batch order status update completed",
		zap.Int("update_count", len(updates)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// OrderStatusUpdate represents an order status update
type OrderStatusUpdate struct {
	OrderID        uuid.UUID
	Status         string
	FilledQuantity decimal.Decimal
	AvgPrice       decimal.Decimal
}

// getFloatPtr safely converts decimal to float pointer
func (r *OptimizedRepository) getFloatPtr(d decimal.Decimal) *float64 {
	if d.IsZero() {
		return nil
	}
	val := d.InexactFloat64()
	return &val
}

// convertToInternalOrder converts database model to internal model
func (r *OptimizedRepository) convertToInternalOrder(dbOrder *models.Order) (*model.Order, error) {
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
		OCOGroupID:     dbOrder.OCOGroupID,
		ParentOrderID:  dbOrder.ParentOrderID,
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

	return order, nil
}

// invalidateOrderCaches removes cached data for updated orders
func (r *OptimizedRepository) invalidateOrderCaches(ctx context.Context, updates []OrderStatusUpdate) {
	pipeline := r.cache.Pipeline()
	for _, update := range updates {
		cacheKey := fmt.Sprintf("order:%s", update.OrderID.String())
		pipeline.Del(ctx, cacheKey)
	}
	pipeline.Exec(ctx)
}
