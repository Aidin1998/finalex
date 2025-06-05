-- Ultra-high concurrency accounts table with partitioning support
-- Supports 100k+ RPS with optimized indexing and constraints

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
CREATE EXTENSION IF NOT EXISTS "pg_partman";

-- Create accounts table with range partitioning by user_id hash
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    currency VARCHAR(10) NOT NULL,
    balance DECIMAL(20,8) NOT NULL DEFAULT 0.00000000,
    available_balance DECIMAL(20,8) NOT NULL DEFAULT 0.00000000,
    pending_balance DECIMAL(20,8) NOT NULL DEFAULT 0.00000000,
    version BIGINT NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    account_type VARCHAR(50) NOT NULL DEFAULT 'spot',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    metadata JSONB DEFAULT '{}',
    
    -- Constraints
    CONSTRAINT accounts_balance_non_negative CHECK (balance >= 0),
    CONSTRAINT accounts_available_balance_non_negative CHECK (available_balance >= 0),
    CONSTRAINT accounts_pending_balance_non_negative CHECK (pending_balance >= 0),
    CONSTRAINT accounts_status_valid CHECK (status IN ('active', 'suspended', 'frozen', 'closed')),
    CONSTRAINT accounts_currency_valid CHECK (currency ~ '^[A-Z]{3,10}$'),
    CONSTRAINT accounts_balance_consistency CHECK (balance = available_balance + pending_balance)
) PARTITION BY HASH (user_id);

-- Create unique constraint on partitioned table
ALTER TABLE accounts ADD CONSTRAINT accounts_user_currency_unique UNIQUE (user_id, currency);

-- Create partitions for horizontal scaling (16 partitions for load distribution)
CREATE TABLE accounts_p0 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE accounts_p1 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE accounts_p2 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE accounts_p3 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE accounts_p4 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE accounts_p5 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE accounts_p6 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE accounts_p7 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE accounts_p8 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE accounts_p9 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE accounts_p10 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE accounts_p11 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE accounts_p12 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE accounts_p13 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE accounts_p14 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE accounts_p15 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 15);

-- Create optimized indexes for high-performance queries
-- Hot path: balance queries by user_id and currency
CREATE INDEX CONCURRENTLY idx_accounts_user_currency ON accounts (user_id, currency) INCLUDE (balance, available_balance, version);

-- Hot path: balance updates with optimistic concurrency
CREATE INDEX CONCURRENTLY idx_accounts_version ON accounts (id, version);

-- Warm path: queries by status and currency
CREATE INDEX CONCURRENTLY idx_accounts_status_currency ON accounts (status, currency) WHERE status = 'active';

-- Warm path: queries by account type
CREATE INDEX CONCURRENTLY idx_accounts_type_status ON accounts (account_type, status) WHERE status = 'active';

-- Cold path: administrative queries
CREATE INDEX CONCURRENTLY idx_accounts_created_at ON accounts (created_at) WHERE status = 'active';
CREATE INDEX CONCURRENTLY idx_accounts_updated_at ON accounts (updated_at);

-- JSONB metadata queries (for compliance and audit)
CREATE INDEX CONCURRENTLY idx_accounts_metadata_gin ON accounts USING GIN (metadata);

-- Partial indexes for specific status values (memory optimization)
CREATE INDEX CONCURRENTLY idx_accounts_suspended ON accounts (user_id, currency) WHERE status = 'suspended';
CREATE INDEX CONCURRENTLY idx_accounts_frozen ON accounts (user_id, currency) WHERE status = 'frozen';

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to automatically update updated_at
CREATE TRIGGER accounts_update_updated_at 
    BEFORE UPDATE ON accounts 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Function for optimistic concurrency control
CREATE OR REPLACE FUNCTION update_account_balance(
    p_account_id UUID,
    p_expected_version BIGINT,
    p_balance_delta DECIMAL(20,8),
    p_available_delta DECIMAL(20,8),
    p_pending_delta DECIMAL(20,8)
) RETURNS BOOLEAN AS $$
DECLARE
    affected_rows INTEGER;
BEGIN
    UPDATE accounts 
    SET 
        balance = balance + p_balance_delta,
        available_balance = available_balance + p_available_delta,
        pending_balance = pending_balance + p_pending_delta,
        version = version + 1,
        updated_at = NOW()
    WHERE 
        id = p_account_id 
        AND version = p_expected_version
        AND balance + p_balance_delta >= 0
        AND available_balance + p_available_delta >= 0
        AND pending_balance + p_pending_delta >= 0;
    
    GET DIAGNOSTICS affected_rows = ROW_COUNT;
    RETURN affected_rows > 0;
END;
$$ LANGUAGE plpgsql;

-- Function for atomic balance reservation
CREATE OR REPLACE FUNCTION reserve_balance(
    p_account_id UUID,
    p_amount DECIMAL(20,8),
    p_expected_version BIGINT
) RETURNS BOOLEAN AS $$
DECLARE
    affected_rows INTEGER;
BEGIN
    UPDATE accounts 
    SET 
        available_balance = available_balance - p_amount,
        pending_balance = pending_balance + p_amount,
        version = version + 1,
        updated_at = NOW()
    WHERE 
        id = p_account_id 
        AND version = p_expected_version
        AND available_balance >= p_amount;
    
    GET DIAGNOSTICS affected_rows = ROW_COUNT;
    RETURN affected_rows > 0;
END;
$$ LANGUAGE plpgsql;

-- Function for releasing balance reservation
CREATE OR REPLACE FUNCTION release_balance(
    p_account_id UUID,
    p_amount DECIMAL(20,8),
    p_expected_version BIGINT
) RETURNS BOOLEAN AS $$
DECLARE
    affected_rows INTEGER;
BEGIN
    UPDATE accounts 
    SET 
        available_balance = available_balance + p_amount,
        pending_balance = pending_balance - p_amount,
        version = version + 1,
        updated_at = NOW()
    WHERE 
        id = p_account_id 
        AND version = p_expected_version
        AND pending_balance >= p_amount;
    
    GET DIAGNOSTICS affected_rows = ROW_COUNT;
    RETURN affected_rows > 0;
END;
$$ LANGUAGE plpgsql;

-- Statistics and monitoring views
CREATE VIEW account_statistics AS
SELECT 
    currency,
    account_type,
    status,
    COUNT(*) as account_count,
    SUM(balance) as total_balance,
    SUM(available_balance) as total_available_balance,
    SUM(pending_balance) as total_pending_balance,
    AVG(balance) as avg_balance,
    MIN(balance) as min_balance,
    MAX(balance) as max_balance
FROM accounts 
GROUP BY currency, account_type, status;

-- Performance monitoring view
CREATE VIEW account_performance AS
SELECT 
    schemaname,
    tablename,
    attname,
    n_distinct,
    correlation,
    most_common_vals,
    most_common_freqs
FROM pg_stats 
WHERE tablename LIKE 'accounts%' AND schemaname = current_schema();

-- Comments for documentation
COMMENT ON TABLE accounts IS 'Ultra-high concurrency accounts table supporting 100k+ RPS with partitioning';
COMMENT ON COLUMN accounts.balance IS 'Total account balance (available + pending)';
COMMENT ON COLUMN accounts.available_balance IS 'Available balance for new transactions';
COMMENT ON COLUMN accounts.pending_balance IS 'Balance reserved for pending transactions';
COMMENT ON COLUMN accounts.version IS 'Optimistic concurrency control version';
COMMENT ON COLUMN accounts.metadata IS 'Additional account metadata as JSON';

-- Grant permissions for application user
GRANT SELECT, INSERT, UPDATE ON accounts TO accounts_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO accounts_app;
GRANT EXECUTE ON FUNCTION update_account_balance(UUID, BIGINT, DECIMAL, DECIMAL, DECIMAL) TO accounts_app;
GRANT EXECUTE ON FUNCTION reserve_balance(UUID, DECIMAL, BIGINT) TO accounts_app;
GRANT EXECUTE ON FUNCTION release_balance(UUID, DECIMAL, BIGINT) TO accounts_app;
