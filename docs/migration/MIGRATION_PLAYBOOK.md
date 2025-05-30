# Two-Phase Commit Migration Playbook

## Overview

This comprehensive playbook provides step-by-step operational guidance for executing zero-downtime order book migrations using the two-phase commit protocol. This document is designed for operations teams, SREs, and technical leads responsible for migration execution.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Pre-Migration Preparation](#pre-migration-preparation)
3. [Migration Execution](#migration-execution)
4. [Monitoring and Safety](#monitoring-and-safety)
5. [Emergency Procedures](#emergency-procedures)
6. [Post-Migration Validation](#post-migration-validation)
7. [Troubleshooting Guide](#troubleshooting-guide)
8. [Success Criteria](#success-criteria)

## Prerequisites

### System Requirements

- **Database**: PostgreSQL cluster with >= 16GB RAM, replication enabled
- **Cache**: Redis cluster with persistence, >= 8GB memory
- **Monitoring**: Prometheus, Grafana, AlertManager configured
- **Load Balancer**: Configured with health checks and circuit breakers

### Team Preparation

- [ ] Primary operator identified and trained
- [ ] Secondary operator available for backup
- [ ] DevOps engineer on standby
- [ ] Database administrator available
- [ ] Communication channels established (Slack, PagerDuty)

### Infrastructure Checklist

```bash
# Verify system health
curl -f http://trading-service:8080/health
curl -f http://trading-service:8080/metrics

# Check database connections
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT 1;"

# Verify Redis cluster
redis-cli -h $REDIS_HOST ping

# Check monitoring systems
curl -f http://prometheus:9090/-/healthy
curl -f http://grafana:3000/api/health
```

## Pre-Migration Preparation

### 1. Configuration Review

```bash
# Review migration configuration
curl -s http://trading-service:8080/admin/migration/config | jq '.'

# Verify participant registration
curl -s http://trading-service:8080/admin/migration/participants | jq '.'

# Check safety settings
curl -s http://trading-service:8080/admin/migration/safety/config | jq '.'
```

### 2. Baseline Capture

```bash
# Capture performance baseline
./scripts/capture_baseline.sh --duration=10m --pairs=BTCUSDT,ETHUSDT

# Create database checkpoint
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT pg_create_restore_point('pre_migration_$(date +%Y%m%d_%H%M%S)');"

# Backup current order book state
curl -X POST http://trading-service:8080/admin/backup/create \
  -H "Content-Type: application/json" \
  -d '{"backup_type": "full", "include_orders": true}'
```

### 3. Safety Mechanisms Verification

```bash
# Test circuit breakers
curl -X POST http://trading-service:8080/admin/circuit-breaker/test

# Verify automatic rollback configuration
curl -s http://trading-service:8080/admin/migration/safety/rollback-config | jq '.'

# Test alert channels
./scripts/test_alerts.sh --channel=slack --channel=pagerduty
```

## Migration Execution

### Phase 1: Migration Initiation

```bash
#!/bin/bash
# migration_start.sh

PAIR="${1:-BTCUSDT}"
TARGET_PERCENTAGE="${2:-100}"
MIGRATION_ID=$(uuidgen)

echo "=== Starting Migration for $PAIR ==="
echo "Migration ID: $MIGRATION_ID"
echo "Target Percentage: $TARGET_PERCENTAGE%"

# Step 1: Create migration request
curl -X POST http://trading-service:8080/api/v1/migrations \
  -H "Content-Type: application/json" \
  -d "{
    \"id\": \"$MIGRATION_ID\",
    \"pair\": \"$PAIR\",
    \"requested_by\": \"$(whoami)\",
    \"config\": {
      \"target_implementation\": \"new_orderbook_v2\",
      \"migration_percentage\": $TARGET_PERCENTAGE,
      \"prepare_timeout\": \"5m\",
      \"commit_timeout\": \"10m\",
      \"abort_timeout\": \"3m\",
      \"overall_timeout\": \"30m\",
      \"enable_rollback\": true,
      \"enable_safety_checks\": true,
      \"participant_ids\": [\"orderbook\", \"persistence\", \"marketdata\", \"engine\"]
    }
  }"

echo "Migration initiated. Monitor at: http://dashboard:3000/migration/$MIGRATION_ID"
```

### Phase 2: Real-time Monitoring

```bash
#!/bin/bash
# monitor_migration.sh

MIGRATION_ID="$1"

if [ -z "$MIGRATION_ID" ]; then
  echo "Usage: $0 <migration_id>"
  exit 1
fi

echo "=== Monitoring Migration: $MIGRATION_ID ==="

while true; do
  # Get migration status
  STATUS=$(curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID | jq -r '.status')
  PHASE=$(curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID | jq -r '.phase')
  PROGRESS=$(curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID | jq -r '.progress')
  
  echo "$(date): Status=$STATUS, Phase=$PHASE, Progress=$PROGRESS%"
  
  # Check if completed
  if [[ "$STATUS" == "completed" || "$STATUS" == "failed" || "$STATUS" == "aborted" ]]; then
    echo "Migration finished with status: $STATUS"
    break
  fi
  
  # Check for warnings
  HEALTH=$(curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/health | jq -r '.is_healthy')
  if [ "$HEALTH" != "true" ]; then
    echo "WARNING: Migration health check failed!"
    curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/health | jq '.'
  fi
  
  sleep 10
done
```

### Phase 3: Participant Coordination

```bash
# Check participant status during migration
check_participants() {
  local migration_id="$1"
  
  echo "=== Participant Status ==="
  curl -s http://trading-service:8080/api/v1/migrations/$migration_id/participants | jq '.[] | {
    id: .id,
    type: .type,
    vote: .vote,
    is_healthy: .is_healthy,
    last_heartbeat: .last_heartbeat
  }'
}

# Monitor prepare phase votes
monitor_prepare_phase() {
  local migration_id="$1"
  
  echo "=== Monitoring Prepare Phase ==="
  while true; do
    PHASE=$(curl -s http://trading-service:8080/api/v1/migrations/$migration_id | jq -r '.phase')
    
    if [ "$PHASE" = "prepare" ]; then
      VOTES=$(curl -s http://trading-service:8080/api/v1/migrations/$migration_id/votes | jq '.')
      echo "Votes Summary: $VOTES"
    elif [ "$PHASE" != "preparing" ]; then
      echo "Prepare phase completed. Current phase: $PHASE"
      break
    fi
    
    sleep 5
  done
}
```

## Monitoring and Safety

### Key Metrics to Monitor

```bash
# Performance metrics
watch -n 5 'curl -s http://trading-service:8080/metrics | grep -E "(latency_p95|throughput|error_rate)"'

# System resources
watch -n 10 'kubectl top pods -l app=trading-service'

# Database performance
watch -n 15 'psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "
SELECT 
  schemaname,
  tablename,
  n_tup_ins + n_tup_upd + n_tup_del as total_writes,
  n_tup_ins,
  n_tup_upd,
  n_tup_del
FROM pg_stat_user_tables 
WHERE schemaname = '\''public'\'' 
ORDER BY total_writes DESC 
LIMIT 10;"'
```

### Safety Thresholds

| Metric | Threshold | Action |
|--------|-----------|---------|
| Error Rate | > 1% for 5 min | Automatic rollback |
| Latency P95 | > 50ms for 3 min | Automatic rollback |
| Memory Usage | > 90% for 2 min | Automatic rollback |
| Circuit Breaker | > 3 opens in 10 min | Automatic rollback |
| Database Locks | > 100 waiting | Manual investigation |
| Participant Health | Any unhealthy | Migration pause |

### Automated Alerts

```yaml
# Prometheus alert rules
groups:
  - name: migration.rules
    rules:
      - alert: MigrationHighErrorRate
        expr: migration_error_rate > 0.01
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Migration error rate exceeded threshold"
          
      - alert: MigrationHighLatency
        expr: migration_latency_p95 > 0.05
        for: 3m
        labels:
          severity: critical
        annotations:
          summary: "Migration latency exceeded threshold"
          
      - alert: MigrationParticipantUnhealthy
        expr: migration_participant_healthy == 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Migration participant health check failed"
```

## Emergency Procedures

### Emergency Stop

```bash
#!/bin/bash
# emergency_stop.sh

MIGRATION_ID="$1"

echo "=== EMERGENCY STOP PROCEDURE ==="
echo "WARNING: This will immediately abort migration $MIGRATION_ID"
echo "Press Ctrl+C within 10 seconds to cancel..."
sleep 10

echo "1. Aborting migration..."
curl -X POST http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/abort \
  -H "Content-Type: application/json" \
  -d '{"reason": "Emergency stop requested", "force": true}'

echo "2. Activating circuit breakers..."
curl -X POST http://trading-service:8080/admin/circuit-breaker/activate-all

echo "3. Capturing system state..."
kubectl describe pods > emergency_state_$(date +%Y%m%d_%H%M%S).log
kubectl logs deployment/trading-service --tail=1000 > emergency_logs_$(date +%Y%m%d_%H%M%S).log

echo "4. Notifying team..."
./scripts/notify_incident.sh --severity=critical --message="Emergency migration stop executed"

echo "=== EMERGENCY STOP COMPLETED ==="
```

### Rollback Procedure

```bash
#!/bin/bash
# rollback_migration.sh

MIGRATION_ID="$1"

echo "=== ROLLBACK PROCEDURE ==="

# Step 1: Initiate rollback
echo "1. Initiating rollback..."
curl -X POST http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/rollback \
  -H "Content-Type: application/json" \
  -d '{"reason": "Manual rollback requested"}'

# Step 2: Monitor rollback progress
echo "2. Monitoring rollback progress..."
while true; do
  STATUS=$(curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID | jq -r '.status')
  
  if [ "$STATUS" = "rolled_back" ]; then
    echo "Rollback completed successfully"
    break
  elif [ "$STATUS" = "rollback_failed" ]; then
    echo "ERROR: Rollback failed!"
    exit 1
  fi
  
  echo "Rollback in progress... Status: $STATUS"
  sleep 5
done

# Step 3: Verify system state
echo "3. Verifying system state..."
./scripts/verify_system_health.sh

echo "=== ROLLBACK COMPLETED ==="
```

### Data Recovery

```bash
#!/bin/bash
# data_recovery.sh

echo "=== DATA RECOVERY PROCEDURE ==="

# Step 1: Stop all migrations
echo "1. Stopping all active migrations..."
curl -X POST http://trading-service:8080/admin/migration/stop-all

# Step 2: Restore from backup
echo "2. Restoring from latest backup..."
BACKUP_ID=$(curl -s http://trading-service:8080/admin/backup/list | jq -r '.[0].id')
curl -X POST http://trading-service:8080/admin/backup/restore \
  -H "Content-Type: application/json" \
  -d "{\"backup_id\": \"$BACKUP_ID\"}"

# Step 3: Verify data integrity
echo "3. Verifying data integrity..."
curl -X POST http://trading-service:8080/admin/data/verify-integrity

echo "=== DATA RECOVERY COMPLETED ==="
```

## Post-Migration Validation

### Validation Checklist

```bash
#!/bin/bash
# post_migration_validation.sh

MIGRATION_ID="$1"

echo "=== POST-MIGRATION VALIDATION ==="

# 1. Verify migration completion
echo "1. Checking migration status..."
STATUS=$(curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID | jq -r '.status')
if [ "$STATUS" != "completed" ]; then
  echo "ERROR: Migration not completed. Status: $STATUS"
  exit 1
fi

# 2. Performance validation
echo "2. Validating performance metrics..."
./scripts/performance_test.sh --duration=5m --compare-baseline

# 3. Data integrity check
echo "3. Checking data integrity..."
curl -X POST http://trading-service:8080/admin/data/integrity-check | jq '.'

# 4. Order book validation
echo "4. Validating order book state..."
curl -s http://trading-service:8080/api/v1/orderbook/BTCUSDT | jq '.bids | length, .asks | length'

# 5. System health check
echo "5. Comprehensive health check..."
./scripts/health_check_comprehensive.sh

echo "=== VALIDATION COMPLETED ==="
```

### Performance Comparison

```bash
# Compare pre and post migration performance
compare_performance() {
  local migration_id="$1"
  
  echo "=== Performance Comparison ==="
  
  # Get baseline metrics
  BASELINE=$(curl -s http://trading-service:8080/api/v1/migrations/$migration_id/baseline)
  
  # Get current metrics
  CURRENT=$(curl -s http://trading-service:8080/metrics | ./scripts/parse_metrics.sh)
  
  echo "Latency P95:"
  echo "  Baseline: $(echo $BASELINE | jq -r '.latency_p95')"
  echo "  Current:  $(echo $CURRENT | jq -r '.latency_p95')"
  
  echo "Throughput:"
  echo "  Baseline: $(echo $BASELINE | jq -r '.throughput')"
  echo "  Current:  $(echo $CURRENT | jq -r '.throughput')"
  
  echo "Error Rate:"
  echo "  Baseline: $(echo $BASELINE | jq -r '.error_rate')"
  echo "  Current:  $(echo $CURRENT | jq -r '.error_rate')"
}
```

## Troubleshooting Guide

### Common Issues

#### 1. Prepare Phase Timeout

**Symptoms:**
- Migration stuck in "preparing" phase
- Some participants not responding

**Resolution:**
```bash
# Check participant health
curl -s http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/participants | jq '.[] | select(.is_healthy == false)'

# Restart unhealthy participants
kubectl delete pod -l app=trading-service,component=participant

# Retry prepare phase
curl -X POST http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/retry-prepare
```

#### 2. High Error Rate During Migration

**Symptoms:**
- Error rate exceeding 1%
- Client connection failures

**Resolution:**
```bash
# Immediate mitigation
curl -X POST http://trading-service:8080/api/v1/migrations/$MIGRATION_ID/pause

# Investigate errors
kubectl logs deployment/trading-service | grep ERROR | tail -100

# Check database locks
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "
SELECT blocked_locks.pid AS blocked_pid,
       blocked_activity.usename AS blocked_user,
       blocking_locks.pid AS blocking_pid,
       blocking_activity.usename AS blocking_user,
       blocked_activity.query AS blocked_statement,
       blocking_activity.query AS current_statement_in_blocking_process
FROM pg_catalog.pg_locks blocked_locks
JOIN pg_catalog.pg_stat_activity blocked_activity ON blocked_activity.pid = blocked_locks.pid
JOIN pg_catalog.pg_locks blocking_locks 
    ON blocking_locks.locktype = blocked_locks.locktype
    AND blocking_locks.database IS NOT DISTINCT FROM blocked_locks.database
    AND blocking_locks.relation IS NOT DISTINCT FROM blocked_locks.relation
    AND blocking_locks.page IS NOT DISTINCT FROM blocked_locks.page
    AND blocking_locks.tuple IS NOT DISTINCT FROM blocked_locks.tuple
    AND blocking_locks.virtualxid IS NOT DISTINCT FROM blocked_locks.virtualxid
    AND blocking_locks.transactionid IS NOT DISTINCT FROM blocked_locks.transactionid
    AND blocking_locks.classid IS NOT DISTINCT FROM blocked_locks.classid
    AND blocking_locks.objid IS NOT DISTINCT FROM blocked_locks.objid
    AND blocking_locks.objsubid IS NOT DISTINCT FROM blocked_locks.objsubid
    AND blocking_locks.pid != blocked_locks.pid
JOIN pg_catalog.pg_stat_activity blocking_activity ON blocking_activity.pid = blocking_locks.pid
WHERE NOT blocked_locks.granted;
"
```

#### 3. Memory Leak During Migration

**Symptoms:**
- Continuously increasing memory usage
- OOM kills of trading service pods

**Resolution:**
```bash
# Immediate scaling
kubectl scale deployment/trading-service --replicas=6

# Memory analysis
kubectl exec -it deployment/trading-service -- /app/pprof-heap-dump.sh

# Gradual rollback if necessary
./scripts/gradual_rollback.sh --migration-id=$MIGRATION_ID --step-size=25
```

#### 4. Database Connection Pool Exhaustion

**Symptoms:**
- "too many connections" errors
- High database connection count

**Resolution:**
```bash
# Check connection count
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT count(*) FROM pg_stat_activity;"

# Restart connection pooler
kubectl restart deployment/pgbouncer

# Temporarily increase pool size
kubectl patch configmap pgbouncer-config --patch '{"data":{"pgbouncer.ini":"[databases]\npincex = host=postgres port=5432 dbname=pincex\n[pgbouncer]\nlisten_port = 5432\nmax_client_conn = 200\ndefault_pool_size = 50"}}'
```

### Escalation Procedures

#### Level 1: Operator Response (0-15 minutes)
- Monitor migration progress
- Check basic health metrics
- Execute standard procedures

#### Level 2: Technical Lead (15-30 minutes)
- Analyze detailed logs and metrics
- Make rollback decisions
- Coordinate with team

#### Level 3: Engineering Manager (30+ minutes)
- Customer communication
- Post-incident planning
- System architecture decisions

### Contact Information

```yaml
contacts:
  primary_operator:
    name: "Operations Team"
    slack: "#trading-ops"
    phone: "+1-xxx-xxx-xxxx"
    
  technical_lead:
    name: "Technical Lead"
    slack: "@tech-lead"
    phone: "+1-xxx-xxx-xxxx"
    
  dba:
    name: "Database Administrator"
    slack: "@dba-oncall"
    phone: "+1-xxx-xxx-xxxx"
    
  engineering_manager:
    name: "Engineering Manager"
    slack: "@eng-manager"
    phone: "+1-xxx-xxx-xxxx"
```

## Success Criteria

### Technical Criteria

- [ ] Migration completed successfully (status = "completed")
- [ ] All participants voted "yes" and committed successfully
- [ ] Zero data loss (verified through integrity checks)
- [ ] Performance within acceptable bounds:
  - Latency P95 < 15ms (< 10ms preferred)
  - Throughput >= baseline
  - Error rate < 0.01%
- [ ] Downtime < 100ms (zero downtime preferred)
- [ ] All safety mechanisms functioned correctly
- [ ] Post-migration validation passed

### Operational Criteria

- [ ] All monitoring and alerting functional
- [ ] No manual intervention required during migration
- [ ] Rollback capability verified and working
- [ ] Team communication effective
- [ ] Documentation updated with lessons learned
- [ ] Backup and recovery procedures tested

### Business Criteria

- [ ] No customer-impacting incidents
- [ ] Trading operations continued uninterrupted
- [ ] Market data integrity maintained
- [ ] Compliance requirements met
- [ ] SLA commitments honored

## Lessons Learned Template

After each migration, document lessons learned:

```markdown
# Migration Post-Mortem: [MIGRATION_ID]

## Summary
- **Date**: [DATE]
- **Pair**: [TRADING_PAIR]
- **Duration**: [DURATION]
- **Outcome**: [SUCCESS/FAILURE]

## What Went Well
- [List successful aspects]

## What Could Be Improved
- [List areas for improvement]

## Action Items
- [ ] [Action item 1]
- [ ] [Action item 2]

## Updated Procedures
- [Document any procedure updates]
```

---

**Document Version**: 1.0  
**Last Updated**: [Current Date]  
**Next Review**: [Date + 3 months]  
**Owner**: Trading Platform Operations Team
