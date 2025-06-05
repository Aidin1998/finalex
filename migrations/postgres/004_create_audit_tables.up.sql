-- Ultra-high concurrency ledger transactions table
-- Supports audit trail and transaction history with time-based partitioning

CREATE TABLE ledger_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL,
    transaction_type VARCHAR(50) NOT NULL,
    amount DECIMAL(20,8) NOT NULL,
    balance_before DECIMAL(20,8) NOT NULL,
    balance_after DECIMAL(20,8) NOT NULL,
    reference_id UUID,
    reference_type VARCHAR(100),
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by UUID,
    
    -- Constraints
    CONSTRAINT ledger_transactions_amount_not_zero CHECK (amount != 0),
    CONSTRAINT ledger_transactions_type_valid CHECK (
        transaction_type IN (
            'deposit', 'withdrawal', 'transfer_in', 'transfer_out',
            'trade_buy', 'trade_sell', 'fee', 'refund', 'adjustment',
            'reservation', 'release', 'freeze', 'unfreeze'
        )
    )
) PARTITION BY RANGE (created_at);

-- Create monthly partitions for the current and next 12 months
CREATE TABLE ledger_transactions_2024_12 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2024-12-01') TO ('2025-01-01');
CREATE TABLE ledger_transactions_2025_01 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
CREATE TABLE ledger_transactions_2025_02 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
CREATE TABLE ledger_transactions_2025_03 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');
CREATE TABLE ledger_transactions_2025_04 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');
CREATE TABLE ledger_transactions_2025_05 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');
CREATE TABLE ledger_transactions_2025_06 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
CREATE TABLE ledger_transactions_2025_07 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');
CREATE TABLE ledger_transactions_2025_08 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');
CREATE TABLE ledger_transactions_2025_09 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');
CREATE TABLE ledger_transactions_2025_10 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE ledger_transactions_2025_11 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE ledger_transactions_2025_12 PARTITION OF ledger_transactions 
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- Hot path indexes for account transaction history
CREATE INDEX CONCURRENTLY idx_ledger_transactions_account_created ON ledger_transactions (account_id, created_at DESC);

-- Transaction lookup by reference
CREATE INDEX CONCURRENTLY idx_ledger_transactions_reference ON ledger_transactions (reference_id, reference_type);

-- Transaction type analysis
CREATE INDEX CONCURRENTLY idx_ledger_transactions_type_created ON ledger_transactions (transaction_type, created_at);

-- Audit and compliance queries
CREATE INDEX CONCURRENTLY idx_ledger_transactions_created_amount ON ledger_transactions (created_at, amount) WHERE ABS(amount) > 1000;

-- JSONB metadata search
CREATE INDEX CONCURRENTLY idx_ledger_transactions_metadata_gin ON ledger_transactions USING GIN (metadata);

-- Balance snapshots table for point-in-time balance verification
CREATE TABLE balance_snapshots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL,
    balance DECIMAL(20,8) NOT NULL,
    available_balance DECIMAL(20,8) NOT NULL,
    pending_balance DECIMAL(20,8) NOT NULL,
    snapshot_at TIMESTAMP WITH TIME ZONE NOT NULL,
    snapshot_type VARCHAR(50) NOT NULL DEFAULT 'scheduled',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT balance_snapshots_balance_non_negative CHECK (balance >= 0),
    CONSTRAINT balance_snapshots_type_valid CHECK (
        snapshot_type IN ('scheduled', 'manual', 'reconciliation', 'audit')
    )
) PARTITION BY RANGE (snapshot_at);

-- Create quarterly partitions for balance snapshots
CREATE TABLE balance_snapshots_2024_q4 PARTITION OF balance_snapshots 
    FOR VALUES FROM ('2024-10-01') TO ('2025-01-01');
CREATE TABLE balance_snapshots_2025_q1 PARTITION OF balance_snapshots 
    FOR VALUES FROM ('2025-01-01') TO ('2025-04-01');
CREATE TABLE balance_snapshots_2025_q2 PARTITION OF balance_snapshots 
    FOR VALUES FROM ('2025-04-01') TO ('2025-07-01');
CREATE TABLE balance_snapshots_2025_q3 PARTITION OF balance_snapshots 
    FOR VALUES FROM ('2025-07-01') TO ('2025-10-01');
CREATE TABLE balance_snapshots_2025_q4 PARTITION OF balance_snapshots 
    FOR VALUES FROM ('2025-10-01') TO ('2026-01-01');

-- Indexes for balance snapshots
CREATE INDEX CONCURRENTLY idx_balance_snapshots_account_snapshot ON balance_snapshots (account_id, snapshot_at DESC);
CREATE INDEX CONCURRENTLY idx_balance_snapshots_type_created ON balance_snapshots (snapshot_type, created_at);

-- Transaction journal for double-entry bookkeeping
CREATE TABLE transaction_journal (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id UUID NOT NULL,
    account_id UUID NOT NULL,
    debit_amount DECIMAL(20,8) DEFAULT 0,
    credit_amount DECIMAL(20,8) DEFAULT 0,
    currency VARCHAR(10) NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT transaction_journal_debit_credit_exclusive CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR 
        (credit_amount > 0 AND debit_amount = 0)
    ),
    CONSTRAINT transaction_journal_amount_positive CHECK (
        debit_amount >= 0 AND credit_amount >= 0
    )
) PARTITION BY RANGE (created_at);

-- Create monthly partitions for transaction journal
CREATE TABLE transaction_journal_2024_12 PARTITION OF transaction_journal 
    FOR VALUES FROM ('2024-12-01') TO ('2025-01-01');
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

-- Indexes for transaction journal
CREATE INDEX CONCURRENTLY idx_transaction_journal_transaction_id ON transaction_journal (transaction_id);
CREATE INDEX CONCURRENTLY idx_transaction_journal_account_created ON transaction_journal (account_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_transaction_journal_currency_created ON transaction_journal (currency, created_at);

-- Audit log table for compliance and monitoring
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    old_values JSONB,
    new_values JSONB,
    changed_fields TEXT[],
    user_id UUID,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT audit_log_action_valid CHECK (
        action IN ('create', 'update', 'delete', 'freeze', 'unfreeze', 'suspend', 'activate')
    )
) PARTITION BY RANGE (created_at);

-- Create weekly partitions for audit log (high volume)
CREATE TABLE audit_log_2024_w49 PARTITION OF audit_log 
    FOR VALUES FROM ('2024-12-02') TO ('2024-12-09');
CREATE TABLE audit_log_2024_w50 PARTITION OF audit_log 
    FOR VALUES FROM ('2024-12-09') TO ('2024-12-16');
CREATE TABLE audit_log_2024_w51 PARTITION OF audit_log 
    FOR VALUES FROM ('2024-12-16') TO ('2024-12-23');
CREATE TABLE audit_log_2024_w52 PARTITION OF audit_log 
    FOR VALUES FROM ('2024-12-23') TO ('2024-12-30');
CREATE TABLE audit_log_2025_w01 PARTITION OF audit_log 
    FOR VALUES FROM ('2024-12-30') TO ('2025-01-06');

-- Continue creating partitions for the next 12 weeks
CREATE TABLE audit_log_2025_w02 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-01-06') TO ('2025-01-13');
CREATE TABLE audit_log_2025_w03 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-01-13') TO ('2025-01-20');
CREATE TABLE audit_log_2025_w04 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-01-20') TO ('2025-01-27');
CREATE TABLE audit_log_2025_w05 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-01-27') TO ('2025-02-03');
CREATE TABLE audit_log_2025_w06 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-02-03') TO ('2025-02-10');
CREATE TABLE audit_log_2025_w07 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-02-10') TO ('2025-02-17');
CREATE TABLE audit_log_2025_w08 PARTITION OF audit_log 
    FOR VALUES FROM ('2025-02-17') TO ('2025-02-24');

-- Indexes for audit log
CREATE INDEX CONCURRENTLY idx_audit_log_entity ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_audit_log_user ON audit_log (user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_audit_log_action ON audit_log (action, created_at DESC);
CREATE INDEX CONCURRENTLY idx_audit_log_ip ON audit_log (ip_address, created_at) WHERE ip_address IS NOT NULL;

-- JSONB indexes for audit log
CREATE INDEX CONCURRENTLY idx_audit_log_old_values_gin ON audit_log USING GIN (old_values);
CREATE INDEX CONCURRENTLY idx_audit_log_new_values_gin ON audit_log USING GIN (new_values);

-- Function to create new partitions automatically
CREATE OR REPLACE FUNCTION create_monthly_partition(
    table_name TEXT,
    start_date DATE
) RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    end_date DATE;
BEGIN
    end_date := start_date + INTERVAL '1 month';
    partition_name := table_name || '_' || to_char(start_date, 'YYYY_MM');
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        partition_name, table_name, start_date, end_date
    );
END;
$$ LANGUAGE plpgsql;

-- Function to create weekly partitions for audit log
CREATE OR REPLACE FUNCTION create_weekly_partition(
    table_name TEXT,
    start_date DATE
) RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    end_date DATE;
    week_num TEXT;
BEGIN
    end_date := start_date + INTERVAL '1 week';
    week_num := to_char(start_date, 'YYYY_"w"WW');
    partition_name := table_name || '_' || week_num;
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        partition_name, table_name, start_date, end_date
    );
END;
$$ LANGUAGE plpgsql;

-- Comments for documentation
COMMENT ON TABLE ledger_transactions IS 'Transaction history with time-based partitioning for audit trail';
COMMENT ON TABLE balance_snapshots IS 'Point-in-time balance snapshots for reconciliation and compliance';
COMMENT ON TABLE transaction_journal IS 'Double-entry bookkeeping journal for financial integrity';
COMMENT ON TABLE audit_log IS 'Comprehensive audit trail for all account operations';

-- Grant permissions
GRANT SELECT, INSERT ON ledger_transactions TO accounts_app;
GRANT SELECT, INSERT ON balance_snapshots TO accounts_app;
GRANT SELECT, INSERT ON transaction_journal TO accounts_app;
GRANT SELECT, INSERT ON audit_log TO accounts_app;
GRANT EXECUTE ON FUNCTION create_monthly_partition(TEXT, DATE) TO accounts_app;
GRANT EXECUTE ON FUNCTION create_weekly_partition(TEXT, DATE) TO accounts_app;
