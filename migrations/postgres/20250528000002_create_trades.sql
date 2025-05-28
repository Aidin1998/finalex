-- PostgreSQL trades table partitioned by month for long-term storage
CREATE TABLE IF NOT EXISTS trades (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    counter_order_id UUID NOT NULL,
    user_id UUID NOT NULL,
    counter_user_id UUID NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side VARCHAR(10) NOT NULL,
    price NUMERIC(30,8) NOT NULL,
    quantity NUMERIC(30,8) NOT NULL,
    fee NUMERIC(30,8) NOT NULL,
    fee_currency VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at);

-- Create partitions for 2025
CREATE TABLE IF NOT EXISTS trades_2025_05 PARTITION OF trades
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');
CREATE TABLE IF NOT EXISTS trades_2025_06 PARTITION OF trades
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_trades_user_created ON trades (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trades_symbol_side ON trades (symbol, side);
