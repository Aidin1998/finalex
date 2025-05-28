-- CockroachDB trades schema: partitioned by created_at
CREATE TABLE IF NOT EXISTS trades (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    counter_order_id UUID NOT NULL,
    user_id UUID NOT NULL,
    counter_user_id UUID NOT NULL,
    symbol STRING NOT NULL,
    side STRING NOT NULL,
    price DECIMAL NOT NULL,
    quantity DECIMAL NOT NULL,
    fee DECIMAL NOT NULL,
    fee_currency STRING NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at) (
    PARTITION p_2025_05 VALUES FROM ('2025-05-01') TO ('2025-06-01'),
    PARTITION p_2025_06 VALUES FROM ('2025-06-01') TO ('2025-07-01')
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_trades_user ON trades (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades (symbol, side);
