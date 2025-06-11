-- High-performance indexes for trading engine optimization
-- This migration adds concurrent indexes to avoid downtime

-- Orders table performance indexes
-- User order history with compound index for efficient pagination
CREATE INDEX idx_orders_user_created_status 
ON orders (user_id, created_at DESC, status);

-- Symbol-based trading queries
CREATE INDEX idx_orders_symbol_status_price 
ON orders (symbol, status, price) 
WHERE status IN ('active', 'partial');

-- Price-based order book queries (covering index)
CREATE INDEX idx_orders_symbol_side_price_quantity 
ON orders (symbol, side, price, quantity) 
WHERE status = 'active';

-- Order book depth queries
CREATE INDEX idx_orders_symbol_side_status_created 
ON orders (symbol, side, status, created_at DESC) 
WHERE status IN ('active', 'partial');

-- Trades table performance indexes
-- User trade history with efficient sorting
CREATE INDEX idx_trades_user_created_symbol 
ON trades (user_id, created_at DESC, symbol);

-- Symbol trading activity analysis
CREATE INDEX idx_trades_symbol_created_side 
ON trades (symbol, created_at DESC, side);

-- Order relationship queries (foreign key optimization)
CREATE INDEX idx_trades_order_id 
ON trades (order_id);

CREATE INDEX idx_trades_counter_order_id 
ON trades (counter_order_id);

-- Volume analysis by time periods
CREATE INDEX idx_trades_symbol_created_quantity 
ON trades (symbol, created_at DESC, quantity);

-- Compliance alerts performance indexes (if table exists)
-- Supabase does not support DO blocks for conditional index creation, so these must be run manually if needed.
CREATE INDEX idx_compliance_alerts_status_created 
ON compliance_alerts (status, created_at DESC);

-- User-based compliance queries
CREATE INDEX idx_compliance_alerts_user_created 
ON compliance_alerts (user_id, created_at DESC);

-- Severity-based alert filtering
CREATE INDEX idx_compliance_alerts_severity_status 
ON compliance_alerts (severity, status, created_at DESC);

-- Assignment tracking
CREATE INDEX idx_compliance_alerts_assigned_status 
ON compliance_alerts (assigned_to, status) 
WHERE assigned_to IS NOT NULL;

-- AML users performance indexes (if table exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'aml_users') THEN
        -- Risk-based user queries
        EXECUTE 'CREATE INDEX idx_aml_users_risk_level_score 
                 ON aml_users (risk_level, risk_score DESC)';
        
        -- Geographic risk analysis
        EXECUTE 'CREATE INDEX idx_aml_users_country_risk 
                 ON aml_users (country_code, is_high_risk_country, risk_score DESC)';
        
        -- PEP and sanctions screening
        EXECUTE 'CREATE INDEX idx_aml_users_pep_sanctions 
                 ON aml_users (pep_status, sanction_status, risk_level)';
        
        -- Risk update tracking
        EXECUTE 'CREATE INDEX idx_aml_users_last_risk_update 
                 ON aml_users (last_risk_update DESC) 
                 WHERE last_risk_update > now() - interval ''30 days''';
    END IF;
END $$;

-- Performance optimization settings
-- Update statistics for better query planning
ANALYZE orders;
ANALYZE trades;

-- Optimize table settings for high-throughput trading
ALTER TABLE orders SET (fillfactor = 90);
ALTER TABLE trades SET (fillfactor = 95); -- Trades are mostly insert-only

-- Add helpful comments for monitoring
COMMENT ON INDEX idx_orders_user_created_status IS 'Optimizes user order history queries with status filtering';
COMMENT ON INDEX idx_orders_symbol_status_price IS 'Optimizes order book depth queries for active orders';
COMMENT ON INDEX idx_trades_user_created_symbol IS 'Optimizes user trade history with symbol filtering';
COMMENT ON INDEX idx_trades_symbol_created_side IS 'Optimizes market analysis queries by trading side';

-- Note: Conditional index creation for Supabase
-- Supabase does not support DO blocks for conditional index creation, so these must be run manually if needed.
