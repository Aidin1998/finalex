-- Migration: 001_create_audit_tables.down.sql
-- Description: Drop audit logging tables and related objects

-- Drop views
DROP VIEW IF EXISTS audit_security_incidents;
DROP VIEW IF EXISTS audit_compliance_report;
DROP VIEW IF EXISTS audit_events_analysis;

-- Drop triggers
DROP TRIGGER IF EXISTS update_audit_config_updated_at ON audit_config;
DROP TRIGGER IF EXISTS update_audit_summary_updated_at ON audit_summary;

-- Drop functions
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP FUNCTION IF EXISTS cleanup_audit_logs();
DROP FUNCTION IF EXISTS maintain_audit_partitions();
DROP FUNCTION IF EXISTS create_monthly_partition(text, date);

-- Drop tables (this will also drop partitions)
DROP TABLE IF EXISTS audit_integrity_log;
DROP TABLE IF EXISTS audit_config;
DROP TABLE IF EXISTS audit_summary;
DROP TABLE IF EXISTS audit_events; -- This will drop all partitions as well

-- Note: Extensions are not dropped as they might be used by other parts of the system
-- DROP EXTENSION IF EXISTS "pgcrypto";
-- DROP EXTENSION IF EXISTS "uuid-ossp";
