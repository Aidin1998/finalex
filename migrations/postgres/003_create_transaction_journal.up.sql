-- Create transaction journal table for audit trail and transaction history
-- This table uses range partitioning by created_at for time-series data

CREATE TABLE IF NOT EXISTS transaction_journal (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    currency VARCHAR(10) NOT NULL,
    type VARCHAR(50) NOT NULL,
    amount DECIMAL(36,18) NOT NULL,
    balance_before DECIMAL(36,18) NOT NULL,
    balance_after DECIMAL(36,18) NOT NULL,
    available_before DECIMAL(36,18) NOT NULL,
    available_after DECIMAL(36,18) NOT NULL,
    locked_before DECIMAL(36,18) NOT NULL,
    locked_after DECIMAL(36,18) NOT NULL,
    reference_id VARCHAR(100),
    description TEXT,
    metadata JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'completed',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT transaction_journal_status_check CHECK (status IN ('pending', 'completed', 'failed', 'cancelled'))
) PARTITION BY RANGE (created_at);

-- Create monthly partitions for the current year and next year
-- This enables efficient time-based queries and data archival

-- 2025 partitions
CREATE TABLE transaction_journal_2025_01 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
CREATE TABLE transaction_journal_2025_02 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
CREATE TABLE transaction_journal_2025_03 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');
CREATE TABLE transaction_journal_2025_04 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');
CREATE TABLE transaction_journal_2025_05 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');
CREATE TABLE transaction_journal_2025_06 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
CREATE TABLE transaction_journal_2025_07 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');
CREATE TABLE transaction_journal_2025_08 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');
CREATE TABLE transaction_journal_2025_09 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');
CREATE TABLE transaction_journal_2025_10 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE transaction_journal_2025_11 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE transaction_journal_2025_12 PARTITION OF transaction_journal
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- 2026 partitions
CREATE TABLE transaction_journal_2026_01 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE transaction_journal_2026_02 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE transaction_journal_2026_03 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE transaction_journal_2026_04 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE transaction_journal_2026_05 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE transaction_journal_2026_06 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE transaction_journal_2026_07 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE transaction_journal_2026_08 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE transaction_journal_2026_09 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE transaction_journal_2026_10 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE transaction_journal_2026_11 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE transaction_journal_2026_12 PARTITION OF transaction_journal
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Create indexes on each partition for optimal performance
-- User-based queries
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_01_user_id ON transaction_journal_2025_01 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_02_user_id ON transaction_journal_2025_02 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_03_user_id ON transaction_journal_2025_03 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_04_user_id ON transaction_journal_2025_04 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_05_user_id ON transaction_journal_2025_05 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_06_user_id ON transaction_journal_2025_06 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_07_user_id ON transaction_journal_2025_07 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_08_user_id ON transaction_journal_2025_08 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_09_user_id ON transaction_journal_2025_09 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_10_user_id ON transaction_journal_2025_10 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_11_user_id ON transaction_journal_2025_11 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_12_user_id ON transaction_journal_2025_12 (user_id, created_at DESC);

CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_01_user_id ON transaction_journal_2026_01 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_02_user_id ON transaction_journal_2026_02 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_03_user_id ON transaction_journal_2026_03 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_04_user_id ON transaction_journal_2026_04 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_05_user_id ON transaction_journal_2026_05 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_06_user_id ON transaction_journal_2026_06 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_07_user_id ON transaction_journal_2026_07 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_08_user_id ON transaction_journal_2026_08 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_09_user_id ON transaction_journal_2026_09 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_10_user_id ON transaction_journal_2026_10 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_11_user_id ON transaction_journal_2026_11 (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_12_user_id ON transaction_journal_2026_12 (user_id, created_at DESC);

-- Currency-based queries for reporting
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_01_currency ON transaction_journal_2025_01 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_02_currency ON transaction_journal_2025_02 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_03_currency ON transaction_journal_2025_03 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_04_currency ON transaction_journal_2025_04 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_05_currency ON transaction_journal_2025_05 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_06_currency ON transaction_journal_2025_06 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_07_currency ON transaction_journal_2025_07 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_08_currency ON transaction_journal_2025_08 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_09_currency ON transaction_journal_2025_09 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_10_currency ON transaction_journal_2025_10 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_11_currency ON transaction_journal_2025_11 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_12_currency ON transaction_journal_2025_12 (currency, type, created_at DESC);

CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_01_currency ON transaction_journal_2026_01 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_02_currency ON transaction_journal_2026_02 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_03_currency ON transaction_journal_2026_03 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_04_currency ON transaction_journal_2026_04 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_05_currency ON transaction_journal_2026_05 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_06_currency ON transaction_journal_2026_06 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_07_currency ON transaction_journal_2026_07 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_08_currency ON transaction_journal_2026_08 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_09_currency ON transaction_journal_2026_09 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_10_currency ON transaction_journal_2026_10 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_11_currency ON transaction_journal_2026_11 (currency, type, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_12_currency ON transaction_journal_2026_12 (currency, type, created_at DESC);

-- Reference ID lookup for transaction tracing
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_01_reference ON transaction_journal_2025_01 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_02_reference ON transaction_journal_2025_02 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_03_reference ON transaction_journal_2025_03 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_04_reference ON transaction_journal_2025_04 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_05_reference ON transaction_journal_2025_05 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_06_reference ON transaction_journal_2025_06 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_07_reference ON transaction_journal_2025_07 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_08_reference ON transaction_journal_2025_08 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_09_reference ON transaction_journal_2025_09 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_10_reference ON transaction_journal_2025_10 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_11_reference ON transaction_journal_2025_11 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_12_reference ON transaction_journal_2025_12 (reference_id) WHERE reference_id IS NOT NULL;

CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_01_reference ON transaction_journal_2026_01 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_02_reference ON transaction_journal_2026_02 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_03_reference ON transaction_journal_2026_03 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_04_reference ON transaction_journal_2026_04 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_05_reference ON transaction_journal_2026_05 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_06_reference ON transaction_journal_2026_06 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_07_reference ON transaction_journal_2026_07 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_08_reference ON transaction_journal_2026_08 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_09_reference ON transaction_journal_2026_09 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_10_reference ON transaction_journal_2026_10 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_11_reference ON transaction_journal_2026_11 (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_12_reference ON transaction_journal_2026_12 (reference_id) WHERE reference_id IS NOT NULL;

-- JSONB metadata index for advanced querying
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_06_metadata ON transaction_journal_2025_06 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_07_metadata ON transaction_journal_2025_07 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_08_metadata ON transaction_journal_2025_08 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_09_metadata ON transaction_journal_2025_09 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_10_metadata ON transaction_journal_2025_10 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_11_metadata ON transaction_journal_2025_11 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2025_12_metadata ON transaction_journal_2025_12 USING GIN (metadata);

CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_01_metadata ON transaction_journal_2026_01 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_02_metadata ON transaction_journal_2026_02 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_03_metadata ON transaction_journal_2026_03 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_04_metadata ON transaction_journal_2026_04 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_05_metadata ON transaction_journal_2026_05 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_06_metadata ON transaction_journal_2026_06 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_07_metadata ON transaction_journal_2026_07 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_08_metadata ON transaction_journal_2026_08 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_09_metadata ON transaction_journal_2026_09 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_10_metadata ON transaction_journal_2026_10 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_11_metadata ON transaction_journal_2026_11 USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_transaction_journal_2026_12_metadata ON transaction_journal_2026_12 USING GIN (metadata);

-- Function to automatically create future partitions
CREATE OR REPLACE FUNCTION create_monthly_transaction_partition(start_date DATE)
RETURNS VOID AS $$
DECLARE
    table_name TEXT;
    end_date DATE;
BEGIN
    table_name := 'transaction_journal_' || to_char(start_date, 'YYYY_MM');
    end_date := start_date + INTERVAL '1 month';
    
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF transaction_journal FOR VALUES FROM (%L) TO (%L)',
                   table_name, start_date, end_date);
    
    -- Create indexes
    EXECUTE format('CREATE INDEX CONCURRENTLY idx_%I_user_id ON %I (user_id, created_at DESC)',
                   table_name, table_name);
    EXECUTE format('CREATE INDEX CONCURRENTLY idx_%I_currency ON %I (currency, type, created_at DESC)',
                   table_name, table_name);
    EXECUTE format('CREATE INDEX CONCURRENTLY idx_%I_reference ON %I (reference_id) WHERE reference_id IS NOT NULL',
                   table_name, table_name);
    EXECUTE format('CREATE INDEX CONCURRENTLY idx_%I_metadata ON %I USING GIN (metadata)',
                   table_name, table_name);
END;
$$ LANGUAGE plpgsql;

-- Create a function to automatically manage partition creation
CREATE OR REPLACE FUNCTION maintain_transaction_partitions()
RETURNS VOID AS $$
DECLARE
    current_month DATE;
    future_month DATE;
BEGIN
    current_month := date_trunc('month', CURRENT_DATE);
    
    -- Create partition for next 3 months if they don't exist
    FOR i IN 1..3 LOOP
        future_month := current_month + (i || ' month')::INTERVAL;
        PERFORM create_monthly_transaction_partition(future_month);
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Create a scheduled job to maintain partitions (requires pg_cron extension)
-- SELECT cron.schedule('maintain-partitions', '0 0 1 * *', 'SELECT maintain_transaction_partitions();');
