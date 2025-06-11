-- Migration: Create compliance tables for AML/KYC monitoring
-- Database: PostgreSQL
CREATE TABLE IF NOT EXISTS compliance_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id VARCHAR(255) NOT NULL,
    type VARCHAR(100) NOT NULL,
    severity VARCHAR(50) NOT NULL DEFAULT 'medium',
    message TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'open',
    assigned_to VARCHAR(255),
    notes TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS aml_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID UNIQUE NOT NULL,
    risk_level VARCHAR(50) NOT NULL DEFAULT 'LOW',
    risk_score DECIMAL(5,2) NOT NULL DEFAULT 0.00,
    kyc_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    is_blacklisted BOOLEAN NOT NULL DEFAULT false,
    is_whitelisted BOOLEAN NOT NULL DEFAULT false,
    last_risk_update TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    country_code VARCHAR(3),
    is_high_risk_country BOOLEAN NOT NULL DEFAULT false,
    pep_status BOOLEAN NOT NULL DEFAULT false,
    sanction_status BOOLEAN NOT NULL DEFAULT false,
    customer_type VARCHAR(50) DEFAULT 'Individual',
    business_type VARCHAR(100),
    risk_factors JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_compliance_alerts_user_id ON compliance_alerts (user_id);
CREATE INDEX idx_compliance_alerts_status ON compliance_alerts (status);
CREATE INDEX idx_compliance_alerts_created_at ON compliance_alerts (created_at DESC);
CREATE INDEX idx_aml_users_user_id ON aml_users (user_id);
CREATE INDEX idx_aml_users_risk_level ON aml_users (risk_level);
CREATE INDEX idx_aml_users_risk_score ON aml_users (risk_score DESC);
