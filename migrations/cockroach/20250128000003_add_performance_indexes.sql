-- CockroachDB performance indexes for trading engine optimization
-- This migration adds concurrent indexes to avoid downtime

-- Orders table performance indexes
-- User order history with compound index for efficient pagination
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_user_created_status 
ON orders (user_id, created_at DESC, status);

-- Symbol-based trading queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_symbol_status_price 
ON orders (symbol, status, price) 
STORING (quantity, side)
WHERE status IN ('active', 'partial');

-- Price-based order book queries (covering index)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_symbol_side_price_quantity 
ON orders (symbol, side, price, quantity) 
WHERE status = 'active';

-- Order book depth queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_symbol_side_status_created 
ON orders (symbol, side, status, created_at DESC) 
WHERE status IN ('active', 'partial');

-- Trades table performance indexes
-- User trade history with efficient sorting
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_trades_user_created_symbol 
ON trades (user_id, created_at DESC, symbol)
STORING (price, quantity, side);

-- Symbol trading activity analysis
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_trades_symbol_created_side 
ON trades (symbol, created_at DESC, side)
STORING (price, quantity);

-- Order relationship queries (foreign key optimization)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_trades_order_id 
ON trades (order_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_trades_counter_order_id 
ON trades (counter_order_id);

-- Volume analysis by time periods
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_trades_symbol_created_quantity 
ON trades (symbol, created_at DESC, quantity)
STORING (price, side);

-- Compliance alerts performance indexes (if table exists)
-- Note: CockroachDB doesn't support IF EXISTS for tables in DO blocks
-- These will be created only if the tables exist

-- Status-based compliance monitoring
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_compliance_alerts_status_created 
ON compliance_alerts (status, created_at DESC);

-- User-based compliance queries  
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_compliance_alerts_user_created 
ON compliance_alerts (user_id, created_at DESC);

-- Severity-based alert filtering
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_compliance_alerts_severity_status 
ON compliance_alerts (severity, status, created_at DESC);

-- Assignment tracking
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_compliance_alerts_assigned_status 
ON compliance_alerts (assigned_to, status) 
WHERE assigned_to IS NOT NULL;

-- AML users performance indexes
-- Risk-based user queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_aml_users_risk_level_score 
ON aml_users (risk_level, risk_score DESC);

-- Geographic risk analysis
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_aml_users_country_risk 
ON aml_users (country_code, is_high_risk_country, risk_score DESC);

-- PEP and sanctions screening
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_aml_users_pep_sanctions 
ON aml_users (pep_status, sanction_status, risk_level);

-- Risk update tracking
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_aml_users_last_risk_update 
ON aml_users (last_risk_update DESC) 
WHERE last_risk_update > now() - interval '30 days';

-- Performance optimization settings
-- Update statistics for better query planning
ANALYZE orders;
ANALYZE trades;

-- Add helpful comments for monitoring
COMMENT ON INDEX idx_orders_user_created_status IS 'Optimizes user order history queries with status filtering';
COMMENT ON INDEX idx_orders_symbol_status_price IS 'Optimizes order book depth queries for active orders';
COMMENT ON INDEX idx_trades_user_created_symbol IS 'Optimizes user trade history with symbol filtering';
COMMENT ON INDEX idx_trades_symbol_created_side IS 'Optimizes market analysis queries by trading side';
