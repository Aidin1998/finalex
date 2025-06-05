-- Rollback script for audit and transaction tables

-- Drop functions
DROP FUNCTION IF EXISTS create_weekly_partition(TEXT, DATE);
DROP FUNCTION IF EXISTS create_monthly_partition(TEXT, DATE);

-- Drop partitioned tables (this will automatically drop all partitions)
DROP TABLE IF EXISTS audit_log CASCADE;
DROP TABLE IF EXISTS transaction_journal CASCADE;
DROP TABLE IF EXISTS balance_snapshots CASCADE;
DROP TABLE IF EXISTS ledger_transactions CASCADE;

-- Revoke permissions
REVOKE ALL ON ledger_transactions FROM accounts_app;
REVOKE ALL ON balance_snapshots FROM accounts_app;
REVOKE ALL ON transaction_journal FROM accounts_app;
REVOKE ALL ON audit_log FROM accounts_app;
