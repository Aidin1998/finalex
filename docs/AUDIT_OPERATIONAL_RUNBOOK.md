# PinCEX Audit Logging Operational Runbook

## Table of Contents
1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Daily Operations](#daily-operations)
4. [Incident Response](#incident-response)
5. [Maintenance Procedures](#maintenance-procedures)
6. [Troubleshooting](#troubleshooting)
7. [Compliance Procedures](#compliance-procedures)
8. [Emergency Procedures](#emergency-procedures)

## Overview

The PinCEX audit logging system provides comprehensive tracking of all administrative actions with enterprise-grade security, compliance, and forensic capabilities. This runbook provides operational procedures for managing and maintaining the audit logging infrastructure.

### Key Features
- **Tamper-proof storage** with cryptographic integrity verification
- **Real-time audit logging** for all administrative actions
- **Advanced forensic analysis** with pattern detection
- **Compliance reporting** for regulatory requirements
- **Encrypted sensitive data** protection
- **Automated retention policies** with secure archival

### System Components
- **AuditService**: Core audit logging engine
- **AuditMiddleware**: Automatic request/response capture
- **ForensicEngine**: Advanced analysis and pattern detection
- **EncryptionService**: Sensitive data protection
- **Database**: PostgreSQL with partitioned tables

## System Architecture

### Data Flow
```
API Request → AuditMiddleware → AuditService → Database
                    ↓
              ForensicEngine → Pattern Detection → Alerts
```

### Storage Structure
- **audit_events**: Main audit log table (partitioned by month)
- **audit_summary**: Aggregated statistics for reporting
- **audit_config**: System configuration parameters
- **audit_integrity_log**: Integrity verification results

## Daily Operations

### 1. System Health Monitoring

#### Check Audit Service Status
```bash
# Check if audit service is running
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/api/v1/admin/audit/statistics

# Expected: HTTP 200 with statistics summary
```

#### Monitor Audit Queue
```bash
# Check audit event processing metrics
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/metrics | grep audit_queue

# Key metrics to monitor:
# - audit_queue_size: Current queue depth
# - audit_batch_processing_duration: Processing time
# - audit_events_per_second: Throughput
```

#### Database Health Check
```sql
-- Check partition status
SELECT schemaname, tablename, tableowner, tablespace 
FROM pg_tables 
WHERE tablename LIKE 'audit_events_%' 
ORDER BY tablename;

-- Check recent event counts
SELECT 
    date_trunc('hour', timestamp) as hour,
    COUNT(*) as event_count
FROM audit_events 
WHERE timestamp > NOW() - INTERVAL '24 hours'
GROUP BY hour
ORDER BY hour;
```

### 2. Daily Verification Tasks

#### Integrity Verification
```bash
# Run daily integrity check
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "start_time": "'$(date -d '1 day ago' -Iseconds)'",
       "end_time": "'$(date -Iseconds)'"
     }' \
     http://localhost:8080/api/v1/admin/audit/integrity

# Review results for any violations
```

#### Pattern Detection
```bash
# Check for suspicious patterns in last 24 hours
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "start_time": "'$(date -d '1 day ago' -Iseconds)'",
       "end_time": "'$(date -Iseconds)'"
     }' \
     http://localhost:8080/api/v1/admin/audit/patterns

# Review any detected patterns
```

### 3. Compliance Reporting

#### Generate Daily Compliance Report
```bash
# Export audit logs for compliance review
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     "http://localhost:8080/api/v1/admin/audit/export?format=csv&start_time=$(date -d '1 day ago' -Iseconds)&end_time=$(date -Iseconds)" \
     > daily_audit_$(date +%Y%m%d).csv
```

## Incident Response

### 1. Security Incident Detection

#### High-Risk Activity Alert
When audit logs detect high-risk activities (risk_score >= 80):

1. **Immediate Response**
   ```bash
   # Get timeline for affected user
   curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
          "user_id": "AFFECTED_USER_ID",
          "start_time": "'$(date -d '6 hours ago' -Iseconds)'",
          "end_time": "'$(date -Iseconds)'"
        }' \
        http://localhost:8080/api/v1/admin/audit/timeline
   ```

2. **Pattern Analysis**
   ```bash
   # Check for suspicious patterns
   curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
          "user_id": "AFFECTED_USER_ID",
          "start_time": "'$(date -d '24 hours ago' -Iseconds)'",
          "end_time": "'$(date -Iseconds)'"
        }' \
        http://localhost:8080/api/v1/admin/audit/patterns
   ```

3. **Evidence Collection**
   ```bash
   # Export detailed forensic data
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        "http://localhost:8080/api/v1/admin/audit/export?format=json&user_id=AFFECTED_USER_ID&start_time=$(date -d '24 hours ago' -Iseconds)" \
        > incident_evidence_$(date +%Y%m%d_%H%M%S).json
   ```

### 2. System Compromise Response

#### Integrity Violation Detected
When integrity verification fails:

1. **Immediate Actions**
   - Stop all administrative operations
   - Preserve current state
   - Contact security team

2. **Forensic Analysis**
   ```sql
   -- Check integrity log for details
   SELECT * FROM audit_integrity_log 
   WHERE overall_status != 'valid' 
   ORDER BY check_timestamp DESC 
   LIMIT 10;
   
   -- Check for tampering attempts
   SELECT * FROM audit_events 
   WHERE integrity_verified = false 
   ORDER BY timestamp DESC;
   ```

3. **Recovery Procedures**
   - Restore from last known good backup
   - Re-run integrity verification
   - Document incident in security log

## Maintenance Procedures

### 1. Database Maintenance

#### Monthly Partition Creation
```sql
-- Create next month's partition
SELECT maintain_audit_partitions();

-- Verify partition creation
SELECT schemaname, tablename 
FROM pg_tables 
WHERE tablename LIKE 'audit_events_%' 
AND tablename LIKE '%'||to_char(NOW() + INTERVAL '1 month', 'YYYY_MM');
```

#### Quarterly Cleanup
```sql
-- Run retention policy cleanup
SELECT cleanup_audit_logs();

-- Verify cleanup results
SELECT 
    date_trunc('month', timestamp) as month,
    COUNT(*) as event_count
FROM audit_events 
GROUP BY month
ORDER BY month;
```

### 2. Performance Optimization

#### Index Maintenance
```sql
-- Reindex audit tables monthly
REINDEX TABLE audit_events;
REINDEX TABLE audit_summary;
REINDEX TABLE audit_integrity_log;

-- Analyze tables for query optimization
ANALYZE audit_events;
ANALYZE audit_summary;
```

#### Statistics Update
```sql
-- Update table statistics
UPDATE audit_summary SET updated_at = NOW() 
WHERE period_start >= date_trunc('month', NOW() - INTERVAL '1 month');
```

### 3. Configuration Management

#### Update Audit Configuration
```bash
# View current configuration
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/api/v1/admin/audit/config

# Update retention policy
curl -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "retention_policy": {
         "default_days": 2555,
         "compliance_days": 3650,
         "high_risk_days": 7300
       }
     }' \
     http://localhost:8080/api/v1/admin/audit/config/retention_policy
```

## Troubleshooting

### 1. Common Issues

#### Audit Queue Backlog
**Symptoms**: High queue depth, slow response times
**Solution**:
```bash
# Check queue status
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/metrics | grep audit_queue_size

# Increase batch size temporarily
curl -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "batch_config": {
         "batch_size": 200,
         "flush_interval_seconds": 15
       }
     }' \
     http://localhost:8080/api/v1/admin/audit/config/batch_config
```

#### Database Connection Issues
**Symptoms**: Audit events not being stored
**Solution**:
```bash
# Check database connectivity
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT COUNT(*) FROM audit_events WHERE timestamp > NOW() - INTERVAL '1 hour';"

# Restart audit service if needed
sudo systemctl restart pincex-audit
```

#### High Memory Usage
**Symptoms**: Memory consumption increasing
**Solution**:
```bash
# Check batch buffer size
echo "Current batch buffer size: $(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/api/v1/admin/audit/statistics | jq .batch_buffer_size)"

# Reduce batch size
curl -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "batch_config": {
         "batch_size": 50,
         "flush_interval_seconds": 10
       }
     }' \
     http://localhost:8080/api/v1/admin/audit/config/batch_config
```

### 2. Log Analysis

#### Search for Specific Events
```bash
# Search by user ID
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "user_id": "USER_ID",
       "start_time": "'$(date -d '1 week ago' -Iseconds)'",
       "limit": 100
     }' \
     http://localhost:8080/api/v1/admin/audit/search

# Search by action type
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "action_type": "user_management",
       "start_time": "'$(date -d '1 day ago' -Iseconds)'",
       "limit": 50
     }' \
     http://localhost:8080/api/v1/admin/audit/search
```

## Compliance Procedures

### 1. SOX Compliance

#### Quarterly SOX Report
```bash
# Generate SOX compliance report
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "start_time": "'$(date -d '3 months ago' -Iseconds)'",
       "end_time": "'$(date -Iseconds)'",
       "compliance": ["SOX"]
     }' \
     http://localhost:8080/api/v1/admin/audit/search | \
     jq '.events[] | select(.compliance_flags[] | contains("SOX"))' > sox_report_$(date +%Y%m%d).json
```

### 2. PCI DSS Compliance

#### Monthly PCI Report
```bash
# Export PCI-related events
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     "http://localhost:8080/api/v1/admin/audit/export?format=csv&compliance=PCI_DSS&start_time=$(date -d '1 month ago' -Iseconds)" \
     > pci_audit_$(date +%Y%m).csv
```

### 3. GDPR Compliance

#### Data Subject Request
```bash
# Get all audit events for a specific user (GDPR Article 15)
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "user_id": "DATA_SUBJECT_ID",
       "start_time": "2020-01-01T00:00:00Z",
       "limit": 10000
     }' \
     http://localhost:8080/api/v1/admin/audit/search > gdpr_subject_data.json
```

## Emergency Procedures

### 1. System Emergency

#### Enable Emergency Mode
```bash
# Enable emergency audit mode (increased logging)
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "emergency_mode": true,
       "detail_level": "forensic",
       "real_time_alerts": true
     }' \
     http://localhost:8080/api/v1/admin/audit/config/emergency
```

#### Disable Audit Logging (Emergency Only)
```bash
# WARNING: Only in extreme emergency
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "audit_enabled": false,
       "reason": "EMERGENCY_REASON",
       "authorized_by": "ADMIN_NAME"
     }' \
     http://localhost:8080/api/v1/admin/audit/config/disable
```

### 2. Data Breach Response

#### Immediate Evidence Preservation
```bash
# Create point-in-time backup
pg_dump -h $DB_HOST -U $DB_USER -d $DB_NAME -t audit_events -f audit_backup_$(date +%Y%m%d_%H%M%S).sql

# Export all recent high-risk events
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     "http://localhost:8080/api/v1/admin/audit/export?format=json&min_risk_score=60&start_time=$(date -d '7 days ago' -Iseconds)" \
     > breach_evidence_$(date +%Y%m%d_%H%M%S).json
```

## Contact Information

### Escalation Matrix
- **Level 1**: Operations Team (24/7 monitoring)
- **Level 2**: Security Team (security incidents)
- **Level 3**: Engineering Team (system issues)
- **Level 4**: Compliance Team (regulatory issues)

### Emergency Contacts
- **Operations**: ops@pincex.com
- **Security**: security@pincex.com
- **Engineering**: engineering@pincex.com
- **Compliance**: compliance@pincex.com

### External Contacts
- **Regulatory Authority**: [Contact Information]
- **Legal Counsel**: [Contact Information]
- **External Auditor**: [Contact Information]

---

**Document Version**: 1.0  
**Last Updated**: June 2, 2025  
**Next Review**: September 2, 2025  
**Owner**: Security Team  
**Approver**: CISO
