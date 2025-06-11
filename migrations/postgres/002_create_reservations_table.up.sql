-- Migration: Create reservations table for fund reservations and locks
CREATE TABLE IF NOT EXISTS reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    currency VARCHAR(10) NOT NULL,
    amount DECIMAL(36,18) NOT NULL,
    type VARCHAR(20) NOT NULL,
    reference_id VARCHAR(100),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMP WITH TIME ZONE,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT reservations_amount_positive CHECK (amount > 0),
    CONSTRAINT reservations_type_check CHECK (type IN ('order', 'withdrawal', 'transfer', 'fee', 'margin')),
    CONSTRAINT reservations_status_check CHECK (status IN ('active', 'released', 'expired', 'consumed'))
);
CREATE INDEX idx_reservations_user_currency ON reservations (user_id, currency);
CREATE INDEX idx_reservations_user_status ON reservations (user_id, status);
CREATE INDEX idx_reservations_reference_id ON reservations (reference_id);
CREATE INDEX idx_reservations_status_expires ON reservations (status, expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_reservations_created_at ON reservations (created_at);
CREATE INDEX idx_reservations_type_status ON reservations (type, status);
CREATE INDEX idx_reservations_active ON reservations (user_id, currency, amount) WHERE status = 'active';
