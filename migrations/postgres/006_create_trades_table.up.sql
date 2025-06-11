-- Migration: Create trades table
CREATE TABLE IF NOT EXISTS trades (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    counter_order_id UUID NOT NULL,
    user_id UUID NOT NULL,
    counter_user_id UUID NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side VARCHAR(10) NOT NULL,
    price NUMERIC(30,8) NOT NULL,
    quantity NUMERIC(30,8) NOT NULL,
    fee NUMERIC(30,8) NOT NULL,
    fee_currency VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trades_user_created ON trades (user_id, created_at DESC);
CREATE INDEX idx_trades_symbol_side ON trades (symbol, side);
