package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnhancedRepository wraps the standard repository with caching and optimization
type EnhancedRepository struct {
	db        *ReadWriteDB
	optimizer *QueryOptimizer
	cache     *redis.Client
	logger    *zap.Logger
	
	// Cache TTL configurations
	orderCacheTTL     time.Duration
	tradeCacheTTL     time.Duration
	userOrdersTTL     time.Duration
	pairOrdersTTL     time.Duration
}

// EnhancedRepositoryConfig holds configuration for the enhanced repository
type EnhancedRepositoryConfig struct {
	OrderCacheTTL     time.Duration `yaml:"order_cache_ttl" json:"order_cache_ttl"`
	TradeCacheTTL     time.Duration `yaml:"trade_cache_ttl" json:"trade_cache_ttl"`
	UserOrdersTTL     time.Duration `yaml:"user_orders_ttl" json:"user_orders_ttl"`
	PairOrdersTTL     time.Duration `yaml:"pair_orders_ttl" json:"pair_orders_ttl"`
	EnableQueryCache  bool          `yaml:"enable_query_cache" json:"enable_query_cache"`
	EnableIndexHints  bool          `yaml:"enable_index_hints" json:"enable_index_hints"`
}

// DefaultEnhancedRepositoryConfig returns default configuration
func DefaultEnhancedRepositoryConfig() *EnhancedRepositoryConfig {
	return &EnhancedRepositoryConfig{
		OrderCacheTTL:     30 * time.Second,  // Hot data for active orders
		TradeCacheTTL:     2 * time.Minute,   // Trade data can be cached longer
		UserOrdersTTL:     10 * time.Second,  // User-specific queries
		PairOrdersTTL:     5 * time.Second,   // Market data changes rapidly
		EnableQueryCache:  true,
		EnableIndexHints:  true,
	}
}

// NewEnhancedRepository creates a new enhanced repository with caching
func NewEnhancedRepository(
	db *ReadWriteDB,
	optimizer *QueryOptimizer,
	cache *redis.Client,
	logger *zap.Logger,
	config *EnhancedRepositoryConfig,
) *EnhancedRepository {
	if config == nil {
		config = DefaultEnhancedRepositoryConfig()
	}
	
	return &EnhancedRepository{
		db:            db,
		optimizer:     optimizer,
		cache:         cache,
		logger:        logger,
		orderCacheTTL: config.OrderCacheTTL,
		tradeCacheTTL: config.TradeCacheTTL,
		userOrdersTTL: config.UserOrdersTTL,
		pairOrdersTTL: config.PairOrdersTTL,
	}
}

// Cache key generators
func (r *EnhancedRepository) orderCacheKey(orderID uuid.UUID) string {
	return fmt.Sprintf("order:%s", orderID.String())
}

func (r *EnhancedRepository) userOrdersCacheKey(userID uuid.UUID, status string) string {
	return fmt.Sprintf("user_orders:%s:%s", userID.String(), status)
}

func (r *EnhancedRepository) pairOrdersCacheKey(pair, status string) string {
	return fmt.Sprintf("pair_orders:%s:%s", pair, status)
}

func (r *EnhancedRepository) tradeCacheKey(tradeID uuid.UUID) string {
	return fmt.Sprintf("trade:%s", tradeID.String())
}

// GetOrderByIDCached retrieves an order with caching
func (r *EnhancedRepository) GetOrderByIDCached(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	// Try cache first
	cacheKey := r.orderCacheKey(orderID)
	cached, err := r.cache.Get(ctx, cacheKey).Result()
	if err == nil {
		var order model.Order
		if err := json.Unmarshal([]byte(cached), &order); err == nil {
			r.logger.Debug("Order cache hit", zap.String("order_id", orderID.String()))
			return &order, nil
		}
	}

	// Cache miss - query database with read replica
	query := `
		SELECT id, user_id, symbol, side, type, price, quantity, filled_quantity, 
		       status, time_in_force, stop_price, average_price, display_quantity,
		       trailing_offset, expires_at, oco_group_id, parent_order_id, 
		       created_at, updated_at
		FROM orders 
		WHERE id = $1
	`
	
	start := time.Now()
	var order model.Order
	err = r.db.Reader().WithContext(ctx).Raw(query, orderID).Scan(&order).Error
	
	// Log slow queries
	duration := time.Since(start)
	if duration > 50*time.Millisecond {
		r.logger.Warn("Slow order query", 
			zap.String("order_id", orderID.String()),
			zap.Duration("duration", duration))
	}
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Cache the result
	if orderData, err := json.Marshal(order); err == nil {
		r.cache.Set(ctx, cacheKey, orderData, r.orderCacheTTL)
	}

	return &order, nil
}

// GetOpenOrdersByUserOptimized retrieves open orders for a user with optimization
func (r *EnhancedRepository) GetOpenOrdersByUserOptimized(ctx context.Context, userID uuid.UUID, limit int) ([]*model.Order, error) {
	// Try cache first
	cacheKey := r.userOrdersCacheKey(userID, "open")
	cached, err := r.cache.Get(ctx, cacheKey).Result()
	if err == nil {
		var orders []*model.Order
		if err := json.Unmarshal([]byte(cached), &orders); err == nil {
			r.logger.Debug("User orders cache hit", zap.String("user_id", userID.String()))
			return orders, nil
		}
	}

	// Optimized query with index hints and prepared statement
	query := `
		SELECT /*+ INDEX(orders_user_status_created_idx) */ 
		       id, user_id, symbol, side, type, price, quantity, filled_quantity, 
		       status, time_in_force, created_at, updated_at
		FROM orders 
		WHERE user_id = $1 
		  AND status IN ('open', 'partially_filled', 'pending_trigger')
		ORDER BY created_at DESC
		LIMIT $2
	`
	
	start := time.Now()
	var orders []*model.Order
	err = r.db.Reader().WithContext(ctx).Raw(query, userID, limit).Scan(&orders).Error
	
	// Performance monitoring
	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		r.logger.Warn("Slow user orders query", 
			zap.String("user_id", userID.String()),
			zap.Duration("duration", duration))
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get user orders: %w", err)
	}

	// Cache the result
	if ordersData, err := json.Marshal(orders); err == nil {
		r.cache.Set(ctx, cacheKey, ordersData, r.userOrdersTTL)
	}

	return orders, nil
}

// GetOpenOrdersByPairOptimized retrieves open orders for a trading pair with optimization
func (r *EnhancedRepository) GetOpenOrdersByPairOptimized(ctx context.Context, pair string, side string, limit int) ([]*model.Order, error) {
	// Try cache first
	cacheKey := r.pairOrdersCacheKey(pair, fmt.Sprintf("open_%s", side))
	cached, err := r.cache.Get(ctx, cacheKey).Result()
	if err == nil {
		var orders []*model.Order
		if err := json.Unmarshal([]byte(cached), &orders); err == nil {
			r.logger.Debug("Pair orders cache hit", zap.String("pair", pair), zap.String("side", side))
			return orders, nil
		}
	}

	// Optimized query for order book reconstruction
	var query string
	var args []interface{}
	
	if side == "" {
		// Get all sides
		query = `
			SELECT /*+ INDEX(orders_symbol_side_price_idx) */ 
			       id, user_id, symbol, side, type, price, quantity, filled_quantity, 
			       status, created_at
			FROM orders 
			WHERE symbol = $1 
			  AND status IN ('open', 'partially_filled')
			ORDER BY 
			  CASE WHEN side = 'buy' THEN price END DESC,
			  CASE WHEN side = 'sell' THEN price END ASC,
			  created_at ASC
			LIMIT $2
		`
		args = []interface{}{pair, limit}
	} else {
		// Get specific side with optimized ordering
		orderBy := "price ASC"
		if side == "buy" {
			orderBy = "price DESC"
		}
		
		query = fmt.Sprintf(`
			SELECT /*+ INDEX(orders_symbol_side_price_idx) */ 
			       id, user_id, symbol, side, type, price, quantity, filled_quantity, 
			       status, created_at
			FROM orders 
			WHERE symbol = $1 
			  AND side = $2
			  AND status IN ('open', 'partially_filled')
			ORDER BY %s, created_at ASC
			LIMIT $3
		`, orderBy)
		args = []interface{}{pair, side, limit}
	}
	
	start := time.Now()
	var orders []*model.Order
	err = r.db.Reader().WithContext(ctx).Raw(query, args...).Scan(&orders).Error
	
	// Performance monitoring
	duration := time.Since(start)
	if duration > 50*time.Millisecond {
		r.logger.Warn("Slow pair orders query", 
			zap.String("pair", pair),
			zap.String("side", side),
			zap.Duration("duration", duration))
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get pair orders: %w", err)
	}

	// Cache the result with shorter TTL for market data
	if ordersData, err := json.Marshal(orders); err == nil {
		r.cache.Set(ctx, cacheKey, ordersData, r.pairOrdersTTL)
	}

	return orders, nil
}

// CreateOrderOptimized creates an order with cache invalidation
func (r *EnhancedRepository) CreateOrderOptimized(ctx context.Context, order *model.Order) error {
	// Use write database for creation
	query := `
		INSERT INTO orders (
			id, user_id, symbol, side, type, price, quantity, filled_quantity,
			status, time_in_force, stop_price, average_price, display_quantity,
			trailing_offset, expires_at, oco_group_id, parent_order_id,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`
	
	start := time.Now()
	err := r.db.Writer().WithContext(ctx).Exec(query,
		order.ID, order.UserID, order.Pair, order.Side, order.Type,
		order.Price, order.Quantity, order.FilledQuantity, order.Status,
		order.TimeInForce, order.StopPrice, order.AvgPrice, order.DisplayQuantity,
		order.TrailingOffset, order.ExpireAt, order.OCOGroupID, order.ParentOrderID,
		order.CreatedAt, order.UpdatedAt,
	).Error
	
	duration := time.Since(start)
	if err != nil {
		r.logger.Error("Failed to create order", 
			zap.Error(err), 
			zap.String("order_id", order.ID.String()),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Invalidate related caches
	r.invalidateOrderCaches(ctx, order)
	
	r.logger.Debug("Order created successfully", 
		zap.String("order_id", order.ID.String()),
		zap.Duration("duration", duration))
	
	return nil
}

// UpdateOrderStatusOptimized updates order status with cache invalidation
func (r *EnhancedRepository) UpdateOrderStatusOptimized(ctx context.Context, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	query := `
		UPDATE orders 
		SET status = $2, filled_quantity = $3, average_price = $4, updated_at = $5
		WHERE id = $1
	`
	
	start := time.Now()
	result := r.db.Writer().WithContext(ctx).Exec(query, orderID, status, filledQty, avgPrice, time.Now())
	
	duration := time.Since(start)
	if result.Error != nil {
		r.logger.Error("Failed to update order status",
			zap.Error(result.Error),
			zap.String("order_id", orderID.String()),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to update order status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrOrderNotFound
	}

	// Invalidate order cache
	r.cache.Del(ctx, r.orderCacheKey(orderID))
	
	r.logger.Debug("Order status updated successfully",
		zap.String("order_id", orderID.String()),
		zap.String("status", status),
		zap.Duration("duration", duration))
	
	return nil
}

// BatchCreateTradesOptimized creates multiple trades in a single batch operation
func (r *EnhancedRepository) BatchCreateTradesOptimized(ctx context.Context, trades []*model.Trade) error {
	if len(trades) == 0 {
		return nil
	}

	// Build batch insert query
	values := make([]string, len(trades))
	args := make([]interface{}, 0, len(trades)*7)
	
	for i, trade := range trades {
		values[i] = fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", 
			i*7+1, i*7+2, i*7+3, i*7+4, i*7+5, i*7+6, i*7+7)
		args = append(args, trade.ID, trade.OrderID, trade.Pair, 
			trade.Price, trade.Quantity, trade.Side, trade.CreatedAt)
	}
	
	query := fmt.Sprintf(`
		INSERT INTO trades (id, order_id, symbol, price, quantity, side, created_at) 
		VALUES %s
	`, strings.Join(values, ","))
	
	start := time.Now()
	err := r.db.Writer().WithContext(ctx).Exec(query, args...).Error
	
	duration := time.Since(start)
	if err != nil {
		r.logger.Error("Failed to batch create trades",
			zap.Error(err),
			zap.Int("count", len(trades)),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to batch create trades: %w", err)
	}

	r.logger.Debug("Trades batch created successfully",
		zap.Int("count", len(trades)),
		zap.Duration("duration", duration))
	
	return nil
}

// GetTradeHistoryOptimized retrieves trade history with optimization
func (r *EnhancedRepository) GetTradeHistoryOptimized(ctx context.Context, userID uuid.UUID, pair string, limit int, offset int) ([]*model.Trade, error) {
	// Build optimized query with conditional where clauses
	whereConditions := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1
	
	if userID != uuid.Nil {
		whereConditions = append(whereConditions, fmt.Sprintf("t.order_id IN (SELECT id FROM orders WHERE user_id = $%d)", argIndex))
		args = append(args, userID)
		argIndex++
	}
	
	if pair != "" {
		whereConditions = append(whereConditions, fmt.Sprintf("t.symbol = $%d", argIndex))
		args = append(args, pair)
		argIndex++
	}
	
	query := fmt.Sprintf(`
		SELECT /*+ INDEX(trades_symbol_created_idx) */ 
		       t.id, t.order_id, t.symbol, t.price, t.quantity, t.side, t.is_maker, t.created_at
		FROM trades t
		WHERE %s
		ORDER BY t.created_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(whereConditions, " AND "), argIndex, argIndex+1)
	
	args = append(args, limit, offset)
	
	start := time.Now()
	var trades []*model.Trade
	err := r.db.Reader().WithContext(ctx).Raw(query, args...).Scan(&trades).Error
	
	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		r.logger.Warn("Slow trade history query",
			zap.String("user_id", userID.String()),
			zap.String("pair", pair),
			zap.Duration("duration", duration))
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get trade history: %w", err)
	}

	return trades, nil
}

// invalidateOrderCaches removes cached data related to an order
func (r *EnhancedRepository) invalidateOrderCaches(ctx context.Context, order *model.Order) {
	// Invalidate specific order cache
	r.cache.Del(ctx, r.orderCacheKey(order.ID))
	
	// Invalidate user orders cache
	r.cache.Del(ctx, r.userOrdersCacheKey(order.UserID, "open"))
	
	// Invalidate pair orders cache
	r.cache.Del(ctx, r.pairOrdersCacheKey(order.Pair, fmt.Sprintf("open_%s", order.Side)))
	r.cache.Del(ctx, r.pairOrdersCacheKey(order.Pair, "open_"))
}

// ClearUserCache clears all cached data for a specific user
func (r *EnhancedRepository) ClearUserCache(ctx context.Context, userID uuid.UUID) error {
	pattern := fmt.Sprintf("user_orders:%s:*", userID.String())
	keys, err := r.cache.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	
	if len(keys) > 0 {
		return r.cache.Del(ctx, keys...).Err()
	}
	
	return nil
}

// ClearPairCache clears all cached data for a specific trading pair
func (r *EnhancedRepository) ClearPairCache(ctx context.Context, pair string) error {
	pattern := fmt.Sprintf("pair_orders:%s:*", pair)
	keys, err := r.cache.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	
	if len(keys) > 0 {
		return r.cache.Del(ctx, keys...).Err()
	}
	
	return nil
}

// GetCacheStats returns cache statistics
func (r *EnhancedRepository) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	info := r.cache.Info(ctx, "stats").Val()
	
	// Parse basic stats (this is simplified - real implementation would parse the full INFO output)
	stats := map[string]interface{}{
		"hits":   0,
		"misses": 0,
		"keys":   0,
	}
	
	// Get key count for our prefixes
	orderKeys, _ := r.cache.Keys(ctx, "order:*").Result()
	userOrderKeys, _ := r.cache.Keys(ctx, "user_orders:*").Result()
	pairOrderKeys, _ := r.cache.Keys(ctx, "pair_orders:*").Result()
	
	stats["order_keys"] = len(orderKeys)
	stats["user_order_keys"] = len(userOrderKeys)
	stats["pair_order_keys"] = len(pairOrderKeys)
	stats["total_keys"] = len(orderKeys) + len(userOrderKeys) + len(pairOrderKeys)
	stats["info"] = info
	
	return stats, nil
}

// Custom errors
var (
	ErrOrderNotFound = fmt.Errorf("order not found")
	ErrTradeNotFound = fmt.Errorf("trade not found")
)
