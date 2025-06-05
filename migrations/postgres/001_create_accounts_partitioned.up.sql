-- Create partitioned accounts table for ultra-high concurrency
-- This script creates the main accounts table with partitioning support

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- Create accounts table with hash partitioning
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    currency VARCHAR(10) NOT NULL,
    balance DECIMAL(36,18) NOT NULL DEFAULT 0,
    available DECIMAL(36,18) NOT NULL DEFAULT 0,
    locked DECIMAL(36,18) NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    account_type VARCHAR(20) NOT NULL DEFAULT 'spot',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    last_balance_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by UUID,
    updated_by UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT accounts_balance_non_negative CHECK (balance >= 0),
    CONSTRAINT accounts_available_non_negative CHECK (available >= 0),
    CONSTRAINT accounts_locked_non_negative CHECK (locked >= 0),
    CONSTRAINT accounts_balance_consistency CHECK (balance = available + locked),
    CONSTRAINT accounts_user_currency_unique UNIQUE (user_id, currency),
    CONSTRAINT accounts_account_type_check CHECK (account_type IN ('spot', 'margin', 'futures')),
    CONSTRAINT accounts_status_check CHECK (status IN ('active', 'suspended', 'closed'))
) PARTITION BY HASH (user_id);

-- Create 16 hash partitions for horizontal scaling
CREATE TABLE accounts_p00 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE accounts_p01 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE accounts_p02 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE accounts_p03 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE accounts_p04 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE accounts_p05 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE accounts_p06 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE accounts_p07 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE accounts_p08 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE accounts_p09 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE accounts_p10 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE accounts_p11 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE accounts_p12 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE accounts_p13 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE accounts_p14 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE accounts_p15 PARTITION OF accounts FOR VALUES WITH (MODULUS 16, REMAINDER 15);

-- Create composite indexes on partitions for optimal query performance
-- These indexes are optimized for the most common query patterns

-- Primary lookup index (user_id, currency) - most common query
CREATE INDEX CONCURRENTLY idx_accounts_p00_user_currency ON accounts_p00 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p01_user_currency ON accounts_p01 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p02_user_currency ON accounts_p02 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p03_user_currency ON accounts_p03 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p04_user_currency ON accounts_p04 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p05_user_currency ON accounts_p05 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p06_user_currency ON accounts_p06 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p07_user_currency ON accounts_p07 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p08_user_currency ON accounts_p08 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p09_user_currency ON accounts_p09 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p10_user_currency ON accounts_p10 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p11_user_currency ON accounts_p11 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p12_user_currency ON accounts_p12 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p13_user_currency ON accounts_p13 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p14_user_currency ON accounts_p14 (user_id, currency);
CREATE INDEX CONCURRENTLY idx_accounts_p15_user_currency ON accounts_p15 (user_id, currency);

-- User lookup index for getting all accounts for a user
CREATE INDEX CONCURRENTLY idx_accounts_p00_user_id ON accounts_p00 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p01_user_id ON accounts_p01 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p02_user_id ON accounts_p02 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p03_user_id ON accounts_p03 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p04_user_id ON accounts_p04 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p05_user_id ON accounts_p05 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p06_user_id ON accounts_p06 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p07_user_id ON accounts_p07 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p08_user_id ON accounts_p08 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p09_user_id ON accounts_p09 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p10_user_id ON accounts_p10 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p11_user_id ON accounts_p11 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p12_user_id ON accounts_p12 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p13_user_id ON accounts_p13 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p14_user_id ON accounts_p14 (user_id);
CREATE INDEX CONCURRENTLY idx_accounts_p15_user_id ON accounts_p15 (user_id);

-- Currency aggregation index for reporting
CREATE INDEX CONCURRENTLY idx_accounts_p00_currency_status ON accounts_p00 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p01_currency_status ON accounts_p01 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p02_currency_status ON accounts_p02 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p03_currency_status ON accounts_p03 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p04_currency_status ON accounts_p04 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p05_currency_status ON accounts_p05 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p06_currency_status ON accounts_p06 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p07_currency_status ON accounts_p07 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p08_currency_status ON accounts_p08 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p09_currency_status ON accounts_p09 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p10_currency_status ON accounts_p10 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p11_currency_status ON accounts_p11 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p12_currency_status ON accounts_p12 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p13_currency_status ON accounts_p13 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p14_currency_status ON accounts_p14 (currency, status) INCLUDE (balance, available, locked);
CREATE INDEX CONCURRENTLY idx_accounts_p15_currency_status ON accounts_p15 (currency, status) INCLUDE (balance, available, locked);

-- Temporal indexes for time-based queries
CREATE INDEX CONCURRENTLY idx_accounts_p00_updated_at ON accounts_p00 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p01_updated_at ON accounts_p01 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p02_updated_at ON accounts_p02 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p03_updated_at ON accounts_p03 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p04_updated_at ON accounts_p04 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p05_updated_at ON accounts_p05 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p06_updated_at ON accounts_p06 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p07_updated_at ON accounts_p07 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p08_updated_at ON accounts_p08 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p09_updated_at ON accounts_p09 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p10_updated_at ON accounts_p10 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p11_updated_at ON accounts_p11 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p12_updated_at ON accounts_p12 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p13_updated_at ON accounts_p13 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p14_updated_at ON accounts_p14 (updated_at);
CREATE INDEX CONCURRENTLY idx_accounts_p15_updated_at ON accounts_p15 (updated_at);

-- Create trigger function for updating updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for each partition
CREATE TRIGGER update_accounts_p00_updated_at BEFORE UPDATE ON accounts_p00 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p01_updated_at BEFORE UPDATE ON accounts_p01 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p02_updated_at BEFORE UPDATE ON accounts_p02 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p03_updated_at BEFORE UPDATE ON accounts_p03 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p04_updated_at BEFORE UPDATE ON accounts_p04 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p05_updated_at BEFORE UPDATE ON accounts_p05 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p06_updated_at BEFORE UPDATE ON accounts_p06 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p07_updated_at BEFORE UPDATE ON accounts_p07 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p08_updated_at BEFORE UPDATE ON accounts_p08 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p09_updated_at BEFORE UPDATE ON accounts_p09 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p10_updated_at BEFORE UPDATE ON accounts_p10 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p11_updated_at BEFORE UPDATE ON accounts_p11 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p12_updated_at BEFORE UPDATE ON accounts_p12 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p13_updated_at BEFORE UPDATE ON accounts_p13 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p14_updated_at BEFORE UPDATE ON accounts_p14 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_accounts_p15_updated_at BEFORE UPDATE ON accounts_p15 FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create function for optimistic concurrency control
CREATE OR REPLACE FUNCTION update_account_with_version_check()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.version != NEW.version - 1 THEN
        RAISE EXCEPTION 'Optimistic lock failed: expected version %, got %', OLD.version, NEW.version - 1;
    END IF;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add version check triggers to all partitions
CREATE TRIGGER check_accounts_p00_version BEFORE UPDATE ON accounts_p00 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p01_version BEFORE UPDATE ON accounts_p01 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p02_version BEFORE UPDATE ON accounts_p02 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p03_version BEFORE UPDATE ON accounts_p03 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p04_version BEFORE UPDATE ON accounts_p04 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p05_version BEFORE UPDATE ON accounts_p05 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p06_version BEFORE UPDATE ON accounts_p06 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p07_version BEFORE UPDATE ON accounts_p07 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p08_version BEFORE UPDATE ON accounts_p08 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p09_version BEFORE UPDATE ON accounts_p09 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p10_version BEFORE UPDATE ON accounts_p10 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p11_version BEFORE UPDATE ON accounts_p11 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p12_version BEFORE UPDATE ON accounts_p12 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p13_version BEFORE UPDATE ON accounts_p13 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p14_version BEFORE UPDATE ON accounts_p14 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();
CREATE TRIGGER check_accounts_p15_version BEFORE UPDATE ON accounts_p15 FOR EACH ROW EXECUTE FUNCTION update_account_with_version_check();

-- Analyze tables for optimal query planning
ANALYZE accounts_p00;
ANALYZE accounts_p01;
ANALYZE accounts_p02;
ANALYZE accounts_p03;
ANALYZE accounts_p04;
ANALYZE accounts_p05;
ANALYZE accounts_p06;
ANALYZE accounts_p07;
ANALYZE accounts_p08;
ANALYZE accounts_p09;
ANALYZE accounts_p10;
ANALYZE accounts_p11;
ANALYZE accounts_p12;
ANALYZE accounts_p13;
ANALYZE accounts_p14;
ANALYZE accounts_p15;
