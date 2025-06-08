package crosspair

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Event publishing DTO for cross-pair order/trade events
// (This should be moved to a shared DTO file if reused elsewhere)
type CrossPairOrderEvent struct {
	OrderID        uuid.UUID       `json:"order_id"`
	UserID         uuid.UUID       `json:"user_id"`
	Status         string          `json:"status"`
	Leg1TradeID    uuid.UUID       `json:"leg1_trade_id,omitempty"`
	Leg2TradeID    uuid.UUID       `json:"leg2_trade_id,omitempty"`
	FillAmountLeg1 decimal.Decimal `json:"fill_amount_leg1"`
	FillAmountLeg2 decimal.Decimal `json:"fill_amount_leg2"`
	SyntheticRate  decimal.Decimal `json:"synthetic_rate"`
	Fee            []CrossPairFee  `json:"fee"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	FilledAt       *time.Time      `json:"filled_at,omitempty"`
	CanceledAt     *time.Time      `json:"canceled_at,omitempty"`
	Error          *string         `json:"error,omitempty"`
}

// Storage defines the interface for cross-pair trading persistence
type Storage interface {
	// Order management
	CreateOrder(ctx context.Context, order *CrossPairOrder) error
	GetOrder(ctx context.Context, orderID uuid.UUID) (*CrossPairOrder, error)
	GetOrdersByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairOrder, error)
	UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status OrderStatus) error
	UpdateOrderExecution(ctx context.Context, orderID uuid.UUID, executedQuantity float64, avgPrice float64) error

	// Trade management
	CreateTrade(ctx context.Context, trade *CrossPairTrade) error
	GetTradesByOrder(ctx context.Context, orderID uuid.UUID) ([]*CrossPairTrade, error)
	GetTradesByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error)

	// Route management
	CreateRoute(ctx context.Context, route *CrossPairRoute) error
	GetRoute(ctx context.Context, routeID uuid.UUID) (*CrossPairRoute, error)
	GetRouteByPair(ctx context.Context, baseCurrency, quoteCurrency string) (*CrossPairRoute, error)

	// Analytics
	GetOrderStats(ctx context.Context, userID *uuid.UUID, from, to time.Time) (*OrderStats, error)
	GetVolumeStats(ctx context.Context, from, to time.Time) (*VolumeStats, error)

	// Cleanup
	CleanupExpiredOrders(ctx context.Context, before time.Time) (int64, error)
}

// PostgreSQLStorage implements Storage using PostgreSQL
type PostgreSQLStorage struct {
	db             *sql.DB
	EventPublisher CrossPairEventPublisher // new field
}

// NewPostgreSQLStorage creates a new PostgreSQL storage instance
func NewPostgreSQLStorage(db *sql.DB, publisher CrossPairEventPublisher) Storage {
	return &PostgreSQLStorage{db: db, EventPublisher: publisher}
}

// Order management

func (s *PostgreSQLStorage) CreateOrder(ctx context.Context, order *CrossPairOrder) error {
	query := `
		INSERT INTO crosspair_orders (
			id, user_id, from_asset, to_asset, side, order_type,
			quantity, price, status, estimated_rate, max_slippage,
			created_at, updated_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)`

	_, err := s.db.ExecContext(ctx, query,
		order.ID, order.UserID, order.FromAsset, order.ToAsset,
		order.Side, order.Type, order.Quantity, order.Price, order.Status,
		order.EstimatedRate, order.MaxSlippage,
		order.CreatedAt, order.UpdatedAt, order.ExpiresAt,
	)
	return err
}

func (s *PostgreSQLStorage) GetOrder(ctx context.Context, orderID uuid.UUID) (*CrossPairOrder, error) {
	query := `
		SELECT id, user_id, from_asset, to_asset, side, order_type,
			   quantity, price, executed_quantity, status,
			   estimated_rate, max_slippage, created_at, updated_at, expires_at
		FROM crosspair_orders WHERE id = $1`

	var order CrossPairOrder
	var price sql.NullString
	var expiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, query, orderID).Scan(
		&order.ID, &order.UserID, &order.FromAsset, &order.ToAsset,
		&order.Side, &order.Type, &order.Quantity, &price,
		&order.ExecutedQuantity, &order.Status, &order.EstimatedRate,
		&order.MaxSlippage, &order.CreatedAt, &order.UpdatedAt, &expiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Handle nullable fields
	if price.Valid {
		priceDec, _ := decimal.NewFromString(price.String)
		order.Price = &priceDec
	}
	if expiresAt.Valid {
		order.ExpiresAt = &expiresAt.Time
	}

	return &order, nil
}

func (s *PostgreSQLStorage) GetOrdersByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairOrder, error) {
	query := `
		SELECT id, user_id, from_asset, to_asset, side, order_type,
			   quantity, price, executed_quantity, status,
			   estimated_rate, max_slippage, created_at, updated_at, expires_at
		FROM crosspair_orders 
		WHERE user_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3`

	rows, err := s.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders by user: %w", err)
	}
	defer rows.Close()

	var orders []*CrossPairOrder
	for rows.Next() {
		var order CrossPairOrder
		var price sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(
			&order.ID, &order.UserID, &order.FromAsset, &order.ToAsset,
			&order.Side, &order.Type, &order.Quantity, &price,
			&order.ExecutedQuantity, &order.Status, &order.EstimatedRate,
			&order.MaxSlippage, &order.CreatedAt, &order.UpdatedAt, &expiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}

		// Handle nullable fields
		if price.Valid {
			priceDec, _ := decimal.NewFromString(price.String)
			order.Price = &priceDec
		}
		if expiresAt.Valid {
			order.ExpiresAt = &expiresAt.Time
		}

		orders = append(orders, &order)
	}

	return orders, nil
}

func (s *PostgreSQLStorage) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status OrderStatus) error {
	query := `UPDATE crosspair_orders SET status = $1, updated_at = $2 WHERE id = $3`
	_, err := s.db.ExecContext(ctx, query, status, time.Now(), orderID)
	if err == nil && s.EventPublisher != nil {
		event := CrossPairOrderEvent{
			OrderID:   orderID,
			Status:    string(status),
			UpdatedAt: time.Now(),
		}
		s.EventPublisher.PublishCrossPairOrderEvent(ctx, event)
	}
	return err
}

func (s *PostgreSQLStorage) UpdateOrderExecution(ctx context.Context, orderID uuid.UUID, executedQuantity float64, executedRate float64) error {
	query := `
		UPDATE crosspair_orders 
		SET executed_quantity = $1, executed_rate = $2, updated_at = $3 
		WHERE id = $4`

	rate := decimal.NewFromFloat(executedRate)
	_, err := s.db.ExecContext(ctx, query, executedQuantity, rate, time.Now(), orderID)
	return err
}

// Trade management

func (s *PostgreSQLStorage) CreateTrade(ctx context.Context, trade *CrossPairTrade) error {
	query := `
		INSERT INTO crosspair_trades (
			id, order_id, user_id, from_asset, to_asset,
			quantity, executed_rate, total_fee_usd, slippage, execution_time_ms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := s.db.ExecContext(ctx, query,
		trade.ID, trade.OrderID, trade.UserID, trade.FromAsset, trade.ToAsset,
		trade.Quantity, trade.ExecutedRate, trade.TotalFeeUSD, trade.Slippage,
		trade.ExecutionTimeMs, trade.CreatedAt,
	)
	if err == nil && s.EventPublisher != nil {
		event := CrossPairOrderEvent{
			OrderID:        trade.OrderID,
			UserID:         trade.UserID,
			Status:         "TRADE_EXECUTED",
			Leg1TradeID:    trade.ID, // If available, else leave blank
			SyntheticRate:  trade.ExecutedRate,
			Fee:            trade.Fees,
			FillAmountLeg1: trade.Quantity, // For single-leg, or sum for multi-leg
			CreatedAt:      trade.CreatedAt,
			UpdatedAt:      trade.CreatedAt,
		}
		s.EventPublisher.PublishCrossPairOrderEvent(ctx, event)
	}
	return err
}

func (s *PostgreSQLStorage) GetTradesByOrder(ctx context.Context, orderID uuid.UUID) ([]*CrossPairTrade, error) {
	query := `
		SELECT id, order_id, user_id, from_asset, to_asset,
			   quantity, executed_rate, total_fee_usd, slippage, execution_time_ms, created_at
		FROM crosspair_trades 
		WHERE order_id = $1 
		ORDER BY created_at ASC`

	rows, err := s.db.QueryContext(ctx, query, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []*CrossPairTrade
	for rows.Next() {
		var trade CrossPairTrade
		err := rows.Scan(
			&trade.ID, &trade.OrderID, &trade.UserID, &trade.FromAsset, &trade.ToAsset,
			&trade.Quantity, &trade.ExecutedRate, &trade.TotalFeeUSD, &trade.Slippage, &trade.ExecutionTimeMs, &trade.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		trades = append(trades, &trade)
	}
	return trades, nil
}

func (s *PostgreSQLStorage) GetTradesByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error) {
	query := `
		SELECT id, order_id, user_id, from_asset, to_asset,
			   quantity, executed_rate, total_fee_usd, slippage, execution_time_ms, created_at
		FROM crosspair_trades 
		WHERE user_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3`

	rows, err := s.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []*CrossPairTrade
	for rows.Next() {
		var trade CrossPairTrade
		err := rows.Scan(
			&trade.ID, &trade.OrderID, &trade.UserID, &trade.FromAsset, &trade.ToAsset,
			&trade.Quantity, &trade.ExecutedRate, &trade.TotalFeeUSD, &trade.Slippage, &trade.ExecutionTimeMs, &trade.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		trades = append(trades, &trade)
	}
	return trades, nil
}

// Route management

func (s *PostgreSQLStorage) CreateRoute(ctx context.Context, route *CrossPairRoute) error {
	query := `
		INSERT INTO crosspair_routes (
			base_asset, first_pair, second_pair, first_rate, second_rate,
			synthetic_rate, confidence, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := s.db.ExecContext(ctx, query,
		route.BaseAsset, route.FirstPair, route.SecondPair,
		route.FirstRate, route.SecondRate, route.SyntheticRate,
		route.Confidence, route.UpdatedAt,
	)
	return err
}

func (s *PostgreSQLStorage) GetRoute(ctx context.Context, routeID uuid.UUID) (*CrossPairRoute, error) {
	// Since CrossPairRoute doesn't have an ID field, we can't query by ID
	// This method should be reconsidered or the route structure should be updated
	return nil, fmt.Errorf("GetRoute by ID not supported with current route structure")
}

func (s *PostgreSQLStorage) GetRouteByPair(ctx context.Context, baseAsset, targetAsset string) (*CrossPairRoute, error) {
	query := `
		SELECT base_asset, first_pair, second_pair, first_rate, second_rate,
			   synthetic_rate, confidence, updated_at
		FROM crosspair_routes 
		WHERE base_asset = $1
		ORDER BY confidence DESC, updated_at DESC
		LIMIT 1`

	var route CrossPairRoute
	err := s.db.QueryRowContext(ctx, query, baseAsset).Scan(
		&route.BaseAsset, &route.FirstPair, &route.SecondPair,
		&route.FirstRate, &route.SecondRate, &route.SyntheticRate,
		&route.Confidence, &route.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrRouteNotFound
		}
		return nil, fmt.Errorf("failed to get route by pair: %w", err)
	}

	return &route, nil
}

// Analytics

func (s *PostgreSQLStorage) GetOrderStats(ctx context.Context, userID *uuid.UUID, from, to time.Time) (*OrderStats, error) {
	query := `
		SELECT 
			COUNT(*) as total_orders,
			COUNT(CASE WHEN status = 'COMPLETED' THEN 1 END) as completed_orders,
			COUNT(CASE WHEN status = 'CANCELED' THEN 1 END) as cancelled_orders,
			COALESCE(SUM(executed_quantity * COALESCE(executed_rate, estimated_rate)), 0) as total_volume,
			COALESCE(SUM((
				SELECT SUM(total_fee_usd) FROM crosspair_trades 
				WHERE order_id = crosspair_orders.id
			)), 0) as total_fees
		FROM crosspair_orders 
		WHERE created_at >= $1 AND created_at <= $2`

	args := []interface{}{from, to}
	if userID != nil {
		query += " AND user_id = $3"
		args = append(args, *userID)
	}

	var stats OrderStats
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalOrders, &stats.CompletedOrders, &stats.CancelledOrders,
		&stats.TotalVolume, &stats.TotalFees,
	)
	return &stats, err
}

func (s *PostgreSQLStorage) GetVolumeStats(ctx context.Context, from, to time.Time) (*VolumeStats, error) {
	query := `
		SELECT 
			COALESCE(SUM(quantity * executed_rate), 0) as total_volume,
			COUNT(*) as trade_count,
			COUNT(DISTINCT user_id) as unique_traders
		FROM crosspair_trades 
		WHERE created_at >= $1 AND created_at <= $2`

	var stats VolumeStats
	err := s.db.QueryRowContext(ctx, query, from, to).Scan(
		&stats.TotalVolume, &stats.TradeCount, &stats.UniqueTraders,
	)
	if err != nil {
		return nil, err
	}

	// Get volume by pair
	pairQuery := `
		SELECT 
			CONCAT(t.from_asset, '/', t.to_asset) as pair,
			COALESCE(SUM(t.quantity * t.executed_rate), 0) as volume
		FROM crosspair_trades t
		WHERE t.created_at >= $1 AND t.created_at <= $2
		GROUP BY t.from_asset, t.to_asset`

	rows, err := s.db.QueryContext(ctx, pairQuery, from, to)
	if err != nil {
		return &stats, nil // Return stats without pair breakdown
	}
	defer rows.Close()

	stats.VolumeByPair = make(map[string]float64)
	for rows.Next() {
		var pair string
		var volume float64
		if err := rows.Scan(&pair, &volume); err == nil {
			stats.VolumeByPair[pair] = volume
		}
	}

	return &stats, nil
}

// Cleanup

func (s *PostgreSQLStorage) CleanupExpiredOrders(ctx context.Context, before time.Time) (int64, error) {
	query := `
		DELETE FROM crosspair_orders 
		WHERE status IN ('pending', 'processing') 
		AND expires_at < $1`

	result, err := s.db.ExecContext(ctx, query, before)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	return rowsAffected, err
}

// InMemoryStorage implements Storage using in-memory data structures for testing
type InMemoryStorage struct {
	orders map[uuid.UUID]*CrossPairOrder
	trades map[uuid.UUID]*CrossPairTrade
	routes map[uuid.UUID]*CrossPairRoute
	mutex  sync.RWMutex
}

// NewInMemoryStorage creates a new in-memory storage instance
func NewInMemoryStorage() Storage {
	return &InMemoryStorage{
		orders: make(map[uuid.UUID]*CrossPairOrder),
		trades: make(map[uuid.UUID]*CrossPairTrade),
		routes: make(map[uuid.UUID]*CrossPairRoute),
	}
}

// Order management
func (s *InMemoryStorage) CreateOrder(ctx context.Context, order *CrossPairOrder) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	s.orders[order.ID] = order
	return nil
}

func (s *InMemoryStorage) GetOrder(ctx context.Context, orderID uuid.UUID) (*CrossPairOrder, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	order, exists := s.orders[orderID]
	if !exists {
		return nil, ErrOrderNotFound
	}
	return order, nil
}

func (s *InMemoryStorage) GetOrdersByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairOrder, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var orders []*CrossPairOrder
	count := 0
	skipped := 0

	for _, order := range s.orders {
		if order.UserID == userID {
			if skipped < offset {
				skipped++
				continue
			}
			if count >= limit {
				break
			}
			orders = append(orders, order)
			count++
		}
	}

	return orders, nil
}

func (s *InMemoryStorage) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status OrderStatus) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	order, exists := s.orders[orderID]
	if !exists {
		return ErrOrderNotFound
	}

	order.Status = status
	order.UpdatedAt = time.Now()
	return nil
}

func (s *InMemoryStorage) UpdateOrderExecution(ctx context.Context, orderID uuid.UUID, executedQuantity float64, avgPrice float64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	order, exists := s.orders[orderID]
	if !exists {
		return ErrOrderNotFound
	}

	order.ExecutedQuantity = decimal.NewFromFloat(executedQuantity)
	if avgPrice > 0 {
		price := decimal.NewFromFloat(avgPrice)
		order.ExecutedRate = &price
	}
	order.UpdatedAt = time.Now()
	return nil
}

// Trade management
func (s *InMemoryStorage) CreateTrade(ctx context.Context, trade *CrossPairTrade) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if trade.ID == uuid.Nil {
		trade.ID = uuid.New()
	}
	trade.CreatedAt = time.Now()

	s.trades[trade.ID] = trade
	return nil
}

func (s *InMemoryStorage) GetTradesByOrder(ctx context.Context, orderID uuid.UUID) ([]*CrossPairTrade, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var trades []*CrossPairTrade
	for _, trade := range s.trades {
		if trade.OrderID == orderID {
			trades = append(trades, trade)
		}
	}

	return trades, nil
}

func (s *InMemoryStorage) GetTradesByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var trades []*CrossPairTrade
	count := 0
	skipped := 0

	for _, trade := range s.trades {
		if trade.UserID == userID {
			if skipped < offset {
				skipped++
				continue
			}
			if count >= limit {
				break
			}
			trades = append(trades, trade)
			count++
		}
	}

	return trades, nil
}

// Route management
func (s *InMemoryStorage) CreateRoute(ctx context.Context, route *CrossPairRoute) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if route.ID == uuid.Nil {
		route.ID = uuid.New()
	}
	route.CreatedAt = time.Now()
	route.UpdatedAt = time.Now()

	s.routes[route.ID] = route
	return nil
}

func (s *InMemoryStorage) GetRoute(ctx context.Context, routeID uuid.UUID) (*CrossPairRoute, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	route, exists := s.routes[routeID]
	if !exists {
		return nil, ErrRouteNotFound
	}
	return route, nil
}

func (s *InMemoryStorage) GetRouteByPair(ctx context.Context, baseCurrency, quoteCurrency string) (*CrossPairRoute, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, route := range s.routes {
		if route.FirstPair == baseCurrency+"-"+route.BaseAsset &&
			route.SecondPair == route.BaseAsset+"-"+quoteCurrency {
			return route, nil
		}
	}

	return nil, ErrRouteNotFound
}

// Analytics
func (s *InMemoryStorage) GetOrderStats(ctx context.Context, userID *uuid.UUID, from, to time.Time) (*OrderStats, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := &OrderStats{
		VolumeByPair: make(map[string]float64),
	}

	for _, order := range s.orders {
		if userID != nil && order.UserID != *userID {
			continue
		}
		if order.CreatedAt.Before(from) || order.CreatedAt.After(to) {
			continue
		}

		stats.TotalOrders++

		switch order.Status {
		case CrossPairOrderCompleted:
			stats.CompletedOrders++
		case CrossPairOrderCanceled:
			stats.CancelledOrders++
		}

		if order.ExecutedQuantity.GreaterThan(decimal.Zero) && order.ExecutedRate != nil {
			volume := order.ExecutedQuantity.Mul(*order.ExecutedRate)
			stats.TotalVolume += volume.InexactFloat64()
		}
	}

	return stats, nil
}

func (s *InMemoryStorage) GetVolumeStats(ctx context.Context, from, to time.Time) (*VolumeStats, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := &VolumeStats{
		VolumeByPair: make(map[string]float64),
	}

	uniqueTraders := make(map[uuid.UUID]bool)

	for _, trade := range s.trades {
		if trade.CreatedAt.Before(from) || trade.CreatedAt.After(to) {
			continue
		}

		stats.TradeCount++
		uniqueTraders[trade.UserID] = true

		volume := trade.Quantity.Mul(trade.ExecutedRate)
		stats.TotalVolume += volume.InexactFloat64()

		pair := trade.FromAsset + "/" + trade.ToAsset
		stats.VolumeByPair[pair] += volume.InexactFloat64()
	}

	stats.UniqueTraders = int64(len(uniqueTraders))

	return stats, nil
}

// Cleanup
func (s *InMemoryStorage) CleanupExpiredOrders(ctx context.Context, before time.Time) (int64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var deletedCount int64

	for orderID, order := range s.orders {
		if (order.Status == CrossPairOrderPending || order.Status == CrossPairOrderExecuting) &&
			order.ExpiresAt != nil && order.ExpiresAt.Before(before) {
			delete(s.orders, orderID)
			deletedCount++
		}
	}

	return deletedCount, nil
}

// EventPublisher defines the interface for publishing cross-pair events
// This allows wiring to real infra (WebSocket, EventBus, Compliance, etc.)
type CrossPairEventPublisher interface {
	PublishCrossPairOrderEvent(ctx context.Context, event CrossPairOrderEvent)
}

// Migration SQL for database setup
const CreateTablesSQL = `
-- Cross-pair orders table
CREATE TABLE IF NOT EXISTS crosspair_orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    base_currency VARCHAR(10) NOT NULL,
    quote_currency VARCHAR(10) NOT NULL,
    side VARCHAR(4) NOT NULL CHECK (side IN ('buy', 'sell')),
    order_type VARCHAR(10) NOT NULL CHECK (order_type IN ('market', 'limit')),
    quantity DECIMAL(20,8) NOT NULL CHECK (quantity > 0),
    price DECIMAL(20,8),
    executed_quantity DECIMAL(20,8) DEFAULT 0,
    avg_price DECIMAL(20,8) DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    route_id UUID,
    estimated_legs JSONB,
    estimated_fees JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE
);

-- Cross-pair trades table
CREATE TABLE IF NOT EXISTS crosspair_trades (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES crosspair_orders(id),
    user_id UUID NOT NULL,
    route_id UUID NOT NULL,
    leg1_trade_id UUID NOT NULL,
    leg2_trade_id UUID NOT NULL,
    quantity DECIMAL(20,8) NOT NULL,
    price DECIMAL(20,8) NOT NULL,
    fee DECIMAL(20,8) NOT NULL,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Cross-pair routes table
CREATE TABLE IF NOT EXISTS crosspair_routes (
    id UUID PRIMARY KEY,
    base_currency VARCHAR(10) NOT NULL,
    quote_currency VARCHAR(10) NOT NULL,
    intermediate_currency VARCHAR(10) NOT NULL,
    leg1_pair VARCHAR(20) NOT NULL,
    leg2_pair VARCHAR(20) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_crosspair_orders_user_id ON crosspair_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_crosspair_orders_status ON crosspair_orders(status);
CREATE INDEX IF NOT EXISTS idx_crosspair_orders_created_at ON crosspair_orders(created_at);
CREATE INDEX IF NOT EXISTS idx_crosspair_orders_expires_at ON crosspair_orders(expires_at);
CREATE INDEX IF NOT EXISTS idx_crosspair_orders_pair ON crosspair_orders(base_currency, quote_currency);

CREATE INDEX IF NOT EXISTS idx_crosspair_trades_order_id ON crosspair_trades(order_id);
CREATE INDEX IF NOT EXISTS idx_crosspair_trades_user_id ON crosspair_trades(user_id);
CREATE INDEX IF NOT EXISTS idx_crosspair_trades_executed_at ON crosspair_trades(executed_at);

CREATE INDEX IF NOT EXISTS idx_crosspair_routes_pair ON crosspair_routes(base_currency, quote_currency);
CREATE INDEX IF NOT EXISTS idx_crosspair_routes_active ON crosspair_routes(active);

-- Unique constraint for active routes per pair
CREATE UNIQUE INDEX IF NOT EXISTS idx_crosspair_routes_unique_active 
ON crosspair_routes(base_currency, quote_currency) 
WHERE active = true;
`
