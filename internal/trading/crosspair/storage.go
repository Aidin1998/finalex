package crosspair

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"
)

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
	db *sqlx.DB
}

// NewPostgreSQLStorage creates a new PostgreSQL storage instance
func NewPostgreSQLStorage(db *sqlx.DB) Storage {
	return &PostgreSQLStorage{db: db}
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
	return err
}

func (s *PostgreSQLStorage) GetTradesByOrder(ctx context.Context, orderID uuid.UUID) ([]*CrossPairTrade, error) {
	query := `
		SELECT id, order_id, user_id, from_asset, to_asset,
			   quantity, executed_rate, total_fee_usd, slippage, execution_time_ms, created_at
		FROM crosspair_trades 
		WHERE order_id = $1 
		ORDER BY created_at ASC`

	trades := []*CrossPairTrade{}
	err := s.db.SelectContext(ctx, &trades, query, orderID)
	return trades, err
}

func (s *PostgreSQLStorage) GetTradesByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*CrossPairTrade, error) {
	query := `
		SELECT id, order_id, user_id, from_asset, to_asset,
			   quantity, executed_rate, total_fee_usd, slippage, execution_time_ms, created_at
		FROM crosspair_trades 
		WHERE user_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3`

	trades := []*CrossPairTrade{}
	err := s.db.SelectContext(ctx, &trades, query, userID, limit, offset)
	return trades, err
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

type OrderStats struct {
	TotalOrders     int64   `json:"total_orders"`
	CompletedOrders int64   `json:"completed_orders"`
	CancelledOrders int64   `json:"cancelled_orders"`
	TotalVolume     float64 `json:"total_volume"`
	TotalFees       float64 `json:"total_fees"`
}

type VolumeStats struct {
	TotalVolume   float64            `json:"total_volume"`
	VolumeByPair  map[string]float64 `json:"volume_by_pair"`
	TradeCount    int64              `json:"trade_count"`
	UniqueTraders int64              `json:"unique_traders"`
}

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
