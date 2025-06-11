-- Down migration for compliance tables
DROP TABLE IF EXISTS compliance_alerts CASCADE;
DROP TABLE IF EXISTS aml_users CASCADE;
