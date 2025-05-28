-- CockroachDB orders schema: optimized for high-throughput, time-series partitioning
CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    symbol STRING NOT NULL,
    side STRING NOT NULL,
    price DECIMAL NOT NULL,
    quantity DECIMAL NOT NULL,
    status STRING NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at) (
    PARTITION p_2025_05 VALUES FROM ('2025-05-01') TO ('2025-06-01'),
    PARTITION p_2025_06 VALUES FROM ('2025-06-01') TO ('2025-07-01')
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_orders_user ON orders (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_symbol ON orders (symbol, status);
