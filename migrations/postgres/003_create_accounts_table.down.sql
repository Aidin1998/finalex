-- Rollback script for accounts table
-- This will drop all partitions, indexes, functions, and the main table

-- Drop views first (dependencies)
DROP VIEW IF EXISTS account_performance;
DROP VIEW IF EXISTS account_statistics;

-- Drop functions
DROP FUNCTION IF EXISTS release_balance(UUID, DECIMAL, BIGINT);
DROP FUNCTION IF EXISTS reserve_balance(UUID, DECIMAL, BIGINT);
DROP FUNCTION IF EXISTS update_account_balance(UUID, BIGINT, DECIMAL, DECIMAL, DECIMAL);
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop trigger
DROP TRIGGER IF EXISTS accounts_update_updated_at ON accounts;

-- Drop partitioned table (this will automatically drop all partitions)
DROP TABLE IF EXISTS accounts CASCADE;

-- Revoke permissions
REVOKE ALL ON accounts FROM accounts_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM accounts_app;
