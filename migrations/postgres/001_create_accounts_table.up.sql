-- Migration: Create user accounts table for Finalex (Supabase compatible)
-- This table stores user account information for authentication and profile management

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    username VARCHAR(50) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(50),
    last_name VARCHAR(50),
    phone VARCHAR(32),
    date_of_birth DATE,
    country VARCHAR(2),
    status VARCHAR(32) NOT NULL DEFAULT 'active', -- active, suspended, pending_verification, banned, locked
    kyc_level INTEGER DEFAULT 0,
    tier VARCHAR(16) NOT NULL DEFAULT 'basic', -- basic, premium, vip
    mfa_enabled BOOLEAN NOT NULL DEFAULT false,
    email_verified BOOLEAN NOT NULL DEFAULT false,
    phone_verified BOOLEAN NOT NULL DEFAULT false,
    last_login TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Optional: JSONB for preferences, roles, and extra profile data
    preferences JSONB DEFAULT '{}',
    roles JSONB DEFAULT '[]',
    -- Constraints
    CONSTRAINT accounts_status_check CHECK (status IN ('active', 'suspended', 'pending_verification', 'banned', 'locked')),
    CONSTRAINT accounts_tier_check CHECK (tier IN ('basic', 'premium', 'vip'))
);

-- Indexes for fast lookup
CREATE INDEX idx_accounts_email ON accounts (email);
CREATE INDEX idx_accounts_username ON accounts (username);
CREATE INDEX idx_accounts_status ON accounts (status);
CREATE INDEX idx_accounts_kyc_level ON accounts (kyc_level);
CREATE INDEX idx_accounts_created_at ON accounts (created_at);
