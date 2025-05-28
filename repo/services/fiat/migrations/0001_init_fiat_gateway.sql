-- 0001_init_fiat_gateway.sql
CREATE TABLE IF NOT EXISTS fiat_wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    currency VARCHAR(10) NOT NULL,
    balance NUMERIC(20,8) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS exchange_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    from_currency VARCHAR(10) NOT NULL,
    to_currency VARCHAR(10) NOT NULL,
    amount NUMERIC(20,8) NOT NULL,
    amount_converted NUMERIC(20,8) NOT NULL,
    rate NUMERIC(20,8) NOT NULL,
    fee NUMERIC(20,8) NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fee_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_group VARCHAR(32) NOT NULL,
    tier_min NUMERIC(20,8) NOT NULL,
    tier_max NUMERIC(20,8) NOT NULL,
    fee_percent NUMERIC(5,4) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fiat_wallets_user_id ON fiat_wallets(user_id);
CREATE INDEX IF NOT EXISTS idx_exchange_logs_user_id ON exchange_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_fee_configs_user_group ON fee_configs(user_group);
