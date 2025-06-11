-- Migration: Create transaction journal table for audit trail and transaction history
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
    CONSTRAINT transaction_journal_status_check CHECK (status IN ('pending', 'completed', 'failed', 'cancelled'))
);
CREATE INDEX idx_transaction_journal_user_created ON transaction_journal (user_id, created_at DESC);
CREATE INDEX idx_transaction_journal_status ON transaction_journal (status);
CREATE INDEX idx_transaction_journal_currency ON transaction_journal (currency, type, created_at DESC);
CREATE INDEX idx_transaction_journal_reference ON transaction_journal (reference_id) WHERE reference_id IS NOT NULL;
CREATE INDEX idx_transaction_journal_metadata ON transaction_journal USING GIN (metadata);
