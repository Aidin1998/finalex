-- Migration: 001_create_audit_tables.up.sql
-- Description: Create comprehensive audit logging tables with enterprise-grade features

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create audit events table with partitioning support
CREATE TABLE audit_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id VARCHAR(255) UNIQUE NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    user_id VARCHAR(255),
    session_id VARCHAR(255),
    action_type VARCHAR(100) NOT NULL,
    resource VARCHAR(255),
    resource_id VARCHAR(255),
    ip_address INET,
    user_agent TEXT,
    request_data JSONB,
    response_data JSONB,
    status_code INTEGER,
    duration_ms INTEGER,
    risk_score INTEGER DEFAULT 0,
    business_impact VARCHAR(50) DEFAULT 'low',
    compliance_flags TEXT[],
    forensic_data JSONB,
    hash VARCHAR(64) NOT NULL,
    previous_hash VARCHAR(64),
    integrity_verified BOOLEAN DEFAULT true,
    encrypted_fields JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (timestamp);

-- Create indexes for optimal query performance
CREATE INDEX idx_audit_events_timestamp ON audit_events (timestamp);
CREATE INDEX idx_audit_events_user_id ON audit_events (user_id);
CREATE INDEX idx_audit_events_action_type ON audit_events (action_type);
CREATE INDEX idx_audit_events_resource ON audit_events (resource);
CREATE INDEX idx_audit_events_ip_address ON audit_events (ip_address);
CREATE INDEX idx_audit_events_risk_score ON audit_events (risk_score);
CREATE INDEX idx_audit_events_compliance ON audit_events USING GIN (compliance_flags);
CREATE INDEX idx_audit_events_forensic ON audit_events USING GIN (forensic_data);
CREATE INDEX idx_audit_events_hash ON audit_events (hash);
CREATE INDEX idx_audit_events_event_id ON audit_events (event_id);

-- Create compound indexes for common query patterns
CREATE INDEX idx_audit_events_user_timestamp ON audit_events (user_id, timestamp);
CREATE INDEX idx_audit_events_action_timestamp ON audit_events (action_type, timestamp);
CREATE INDEX idx_audit_events_resource_timestamp ON audit_events (resource, timestamp);
CREATE INDEX idx_audit_events_risk_timestamp ON audit_events (risk_score, timestamp);

-- Create monthly partitions for the current year and next year
-- This should be automated in production
CREATE TABLE audit_events_2024_12 PARTITION OF audit_events
FOR VALUES FROM ('2024-12-01') TO ('2025-01-01');

CREATE TABLE audit_events_2025_01 PARTITION OF audit_events
FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE audit_events_2025_02 PARTITION OF audit_events
FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

CREATE TABLE audit_events_2025_03 PARTITION OF audit_events
FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');

CREATE TABLE audit_events_2025_04 PARTITION OF audit_events
FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');

CREATE TABLE audit_events_2025_05 PARTITION OF audit_events
FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');

CREATE TABLE audit_events_2025_06 PARTITION OF audit_events
FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');

CREATE TABLE audit_events_2025_07 PARTITION OF audit_events
FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');

CREATE TABLE audit_events_2025_08 PARTITION OF audit_events
FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');

CREATE TABLE audit_events_2025_09 PARTITION OF audit_events
FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');

CREATE TABLE audit_events_2025_10 PARTITION OF audit_events
FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

CREATE TABLE audit_events_2025_11 PARTITION OF audit_events
FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE audit_events_2025_12 PARTITION OF audit_events
FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- Create audit summary table for analytics and reporting
CREATE TABLE audit_summary (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    user_id VARCHAR(255),
    action_type VARCHAR(100),
    resource VARCHAR(255),
    total_events INTEGER NOT NULL DEFAULT 0,
    failed_events INTEGER NOT NULL DEFAULT 0,
    high_risk_events INTEGER NOT NULL DEFAULT 0,
    avg_risk_score DECIMAL(5,2),
    compliance_violations INTEGER NOT NULL DEFAULT 0,
    unique_ips INTEGER NOT NULL DEFAULT 0,
    suspicious_patterns TEXT[],
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for audit summary
CREATE INDEX idx_audit_summary_period ON audit_summary (period_start, period_end);
CREATE INDEX idx_audit_summary_user_id ON audit_summary (user_id);
CREATE INDEX idx_audit_summary_action_type ON audit_summary (action_type);
CREATE INDEX idx_audit_summary_resource ON audit_summary (resource);

-- Create audit configuration table
CREATE TABLE audit_config (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    key VARCHAR(255) UNIQUE NOT NULL,
    value JSONB NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Insert default audit configuration
INSERT INTO audit_config (key, value, description) VALUES
('retention_policy', '{"default_days": 2555, "compliance_days": 3650, "high_risk_days": 7300}', 'Audit log retention policies in days'),
('batch_config', '{"batch_size": 100, "flush_interval_seconds": 30, "max_queue_size": 10000}', 'Batch processing configuration'),
('encryption_config', '{"algorithm": "AES-256-GCM", "key_rotation_days": 90, "require_encryption": true}', 'Encryption configuration'),
('compliance_modes', '{"SOX": true, "PCI_DSS": true, "GDPR": true, "SOC2": true, "ISO27001": true}', 'Enabled compliance modes'),
('risk_thresholds', '{"low": 30, "medium": 60, "high": 80, "critical": 95}', 'Risk score thresholds'),
('alert_config', '{"real_time_alerts": true, "high_risk_threshold": 80, "pattern_detection": true}', 'Alerting configuration');

-- Create audit integrity log for tracking verification results
CREATE TABLE audit_integrity_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    check_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    total_events INTEGER NOT NULL,
    verified_events INTEGER NOT NULL,
    hash_mismatches INTEGER NOT NULL DEFAULT 0,
    chain_breaks INTEGER NOT NULL DEFAULT 0,
    timestamp_anomalies INTEGER NOT NULL DEFAULT 0,
    encryption_errors INTEGER NOT NULL DEFAULT 0,
    overall_status VARCHAR(20) NOT NULL, -- 'valid', 'compromised', 'warning'
    violations JSONB,
    check_duration_ms INTEGER,
    performed_by VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create index for integrity log
CREATE INDEX idx_audit_integrity_log_timestamp ON audit_integrity_log (check_timestamp);
CREATE INDEX idx_audit_integrity_log_period ON audit_integrity_log (period_start, period_end);
CREATE INDEX idx_audit_integrity_log_status ON audit_integrity_log (overall_status);

-- Create function to automatically create monthly partitions
CREATE OR REPLACE FUNCTION create_monthly_partition(table_name text, start_date date)
RETURNS void AS $$
DECLARE
    partition_name text;
    end_date date;
BEGIN
    partition_name := table_name || '_' || to_char(start_date, 'YYYY_MM');
    end_date := start_date + interval '1 month';
    
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF %I
                   FOR VALUES FROM (%L) TO (%L)',
                   partition_name, table_name, start_date, end_date);
END;
$$ LANGUAGE plpgsql;

-- Create function to automatically maintain partitions
CREATE OR REPLACE FUNCTION maintain_audit_partitions()
RETURNS void AS $$
DECLARE
    current_month date;
    next_month date;
    future_month date;
BEGIN
    current_month := date_trunc('month', CURRENT_DATE);
    next_month := current_month + interval '1 month';
    future_month := current_month + interval '2 months';
    
    -- Create next month's partition if it doesn't exist
    PERFORM create_monthly_partition('audit_events', next_month);
    
    -- Create the month after next's partition if it doesn't exist
    PERFORM create_monthly_partition('audit_events', future_month);
END;
$$ LANGUAGE plpgsql;

-- Create trigger function to update timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS trigger AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create triggers for updating timestamps
CREATE TRIGGER update_audit_summary_updated_at
    BEFORE UPDATE ON audit_summary
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_audit_config_updated_at
    BEFORE UPDATE ON audit_config
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create view for audit event analysis
CREATE VIEW audit_events_analysis AS
SELECT 
    date_trunc('day', timestamp) as event_date,
    action_type,
    resource,
    COUNT(*) as total_events,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as failed_events,
    COUNT(CASE WHEN risk_score >= 80 THEN 1 END) as high_risk_events,
    AVG(risk_score) as avg_risk_score,
    AVG(duration_ms) as avg_duration_ms,
    COUNT(DISTINCT user_id) as unique_users,
    COUNT(DISTINCT ip_address) as unique_ips
FROM audit_events
GROUP BY date_trunc('day', timestamp), action_type, resource;

-- Create view for compliance reporting
CREATE VIEW audit_compliance_report AS
SELECT 
    date_trunc('month', timestamp) as report_month,
    unnest(compliance_flags) as compliance_flag,
    COUNT(*) as flagged_events,
    COUNT(DISTINCT user_id) as affected_users,
    COUNT(CASE WHEN risk_score >= 80 THEN 1 END) as high_risk_events,
    MAX(risk_score) as max_risk_score
FROM audit_events
WHERE compliance_flags IS NOT NULL AND array_length(compliance_flags, 1) > 0
GROUP BY date_trunc('month', timestamp), unnest(compliance_flags);

-- Create view for security incident analysis
CREATE VIEW audit_security_incidents AS
SELECT 
    timestamp,
    user_id,
    session_id,
    action_type,
    resource,
    ip_address,
    risk_score,
    compliance_flags,
    forensic_data,
    (forensic_data->>'suspicious_patterns')::text[] as suspicious_patterns
FROM audit_events
WHERE risk_score >= 80 
   OR array_length(compliance_flags, 1) > 0
   OR forensic_data->>'suspicious_patterns' IS NOT NULL;

-- Create function for audit log cleanup based on retention policy
CREATE OR REPLACE FUNCTION cleanup_audit_logs()
RETURNS void AS $$
DECLARE
    retention_config jsonb;
    default_retention integer;
    compliance_retention integer;
    high_risk_retention integer;
    cutoff_date_default date;
    cutoff_date_compliance date;
    cutoff_date_high_risk date;
BEGIN
    -- Get retention configuration
    SELECT value INTO retention_config 
    FROM audit_config 
    WHERE key = 'retention_policy';
    
    default_retention := (retention_config->>'default_days')::integer;
    compliance_retention := (retention_config->>'compliance_days')::integer;
    high_risk_retention := (retention_config->>'high_risk_days')::integer;
    
    -- Calculate cutoff dates
    cutoff_date_default := CURRENT_DATE - interval '1 day' * default_retention;
    cutoff_date_compliance := CURRENT_DATE - interval '1 day' * compliance_retention;
    cutoff_date_high_risk := CURRENT_DATE - interval '1 day' * high_risk_retention;
    
    -- Delete old audit events based on retention policy
    DELETE FROM audit_events 
    WHERE timestamp < cutoff_date_default
      AND (compliance_flags IS NULL OR array_length(compliance_flags, 1) = 0)
      AND risk_score < 80;
    
    -- Delete old compliance events
    DELETE FROM audit_events 
    WHERE timestamp < cutoff_date_compliance
      AND (compliance_flags IS NOT NULL AND array_length(compliance_flags, 1) > 0)
      AND risk_score < 80;
    
    -- Delete old high-risk events
    DELETE FROM audit_events 
    WHERE timestamp < cutoff_date_high_risk
      AND risk_score >= 80;
    
    -- Clean up old integrity logs (keep for 2 years)
    DELETE FROM audit_integrity_log 
    WHERE check_timestamp < CURRENT_DATE - interval '2 years';
    
    -- Clean up old audit summaries (keep for 5 years)
    DELETE FROM audit_summary 
    WHERE period_end < CURRENT_DATE - interval '5 years';
END;
$$ LANGUAGE plpgsql;

-- Grant permissions for audit operations
-- Note: Adjust these based on your specific user/role setup
-- GRANT SELECT, INSERT ON audit_events TO audit_writer;
-- GRANT SELECT ON audit_events TO audit_reader;
-- GRANT ALL ON audit_summary TO audit_admin;
-- GRANT ALL ON audit_config TO audit_admin;
-- GRANT ALL ON audit_integrity_log TO audit_admin;
