# PinCEX Operational Runbooks

## Table of Contents
1. [System Health Monitoring](#system-health-monitoring)
2. [Trading Engine Operations](#trading-engine-operations)
3. [Database Operations](#database-operations)
4. [Authentication & Security](#authentication--security)
5. [Market Data Operations](#market-data-operations)
6. [Incident Response](#incident-response)
7. [Performance Troubleshooting](#performance-troubleshooting)
8. [Emergency Procedures](#emergency-procedures)
9. [Operational Runbooks and Testing Guide](#operational-runbooks-and-testing-guide)

---

## System Health Monitoring

### Daily Health Checks

#### Morning Checklist (Every Trading Day)
```powershell
# Check system status
kubectl get pods -n pincex-production
kubectl get services -n pincex-production

# Verify critical services
$services = @("trading-engine", "market-data", "auth-service", "settlement")
foreach ($service in $services) {
    kubectl logs -n pincex-production deployment/$service --tail=50
}

# Check database connectivity
kubectl exec -n pincex-production deployment/trading-engine -- /bin/sh -c "
    pg_isready -h postgres-primary -p 5432 -U trading_user
"

# Verify Redis cluster
kubectl exec -n pincex-production deployment/redis -- redis-cli cluster info
```

#### Key Metrics to Monitor
- **Order Processing Latency**: < 10ms P95
- **Database Connection Pool**: < 80% utilization
- **Memory Usage**: < 85% per pod
- **CPU Usage**: < 70% sustained
- **WebSocket Connections**: Active connection count
- **Error Rates**: < 0.1% for critical endpoints

### Alert Response Procedures

#### High Latency Alert (P95 > 50ms)
```powershell
# 1. Check current load
kubectl top pods -n pincex-production

# 2. Analyze slow queries
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT query, mean_exec_time, calls, total_exec_time 
FROM pg_stat_statements 
ORDER BY mean_exec_time DESC 
LIMIT 10;
"

# 3. Check for blocking locks
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT blocked_locks.pid AS blocked_pid,
       blocked_activity.usename AS blocked_user,
       blocking_locks.pid AS blocking_pid,
       blocking_activity.usename AS blocking_user,
       blocked_activity.query AS blocked_statement,
       blocking_activity.query AS current_statement_in_blocking_process
FROM pg_catalog.pg_locks blocked_locks
JOIN pg_catalog.pg_stat_activity blocked_activity ON blocked_activity.pid = blocked_locks.pid
JOIN pg_catalog.pg_locks blocking_locks ON blocking_locks.locktype = blocked_locks.locktype
JOIN pg_catalog.pg_stat_activity blocking_activity ON blocking_activity.pid = blocking_locks.pid
WHERE NOT blocked_locks.granted;
"

# 4. Scale if necessary
kubectl scale deployment trading-engine --replicas=6 -n pincex-production
```

#### Memory Pressure Alert (> 90% usage)
```powershell
# 1. Identify memory-heavy pods
kubectl top pods -n pincex-production --sort-by=memory

# 2. Check for memory leaks
kubectl exec -n pincex-production deployment/trading-engine -- go tool pprof -top http://localhost:6060/debug/pprof/heap

# 3. Force garbage collection (Go services)
kubectl exec -n pincex-production deployment/trading-engine -- curl -X POST http://localhost:6060/debug/pprof/gc

# 4. Restart pod if necessary
kubectl delete pod -n pincex-production -l app=trading-engine --force
```

---

## Trading Engine Operations

### Order Processing Issues

#### Stuck Orders Investigation
```powershell
# Check order queue depth
kubectl exec -n pincex-production deployment/trading-engine -- curl http://localhost:8080/admin/metrics | Select-String "order_queue"

# View recent order processing logs
kubectl logs -n pincex-production deployment/trading-engine --since=5m | Select-String "ERROR\|WARN"

# Check database for stuck orders
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT id, symbol, side, quantity, status, created_at, updated_at
FROM orders 
WHERE status IN ('PENDING', 'PARTIAL') 
AND created_at < NOW() - INTERVAL '1 minute'
ORDER BY created_at DESC
LIMIT 20;
"
```

#### Order Book Inconsistency
```powershell
# Compare order book state across instances
$instances = kubectl get pods -n pincex-production -l app=trading-engine -o jsonpath='{.items[*].metadata.name}'
foreach ($instance in $instances.Split(' ')) {
    Write-Host "Checking $instance..."
    kubectl exec -n pincex-production $instance -- curl -s http://localhost:8080/admin/orderbook/BTCUSD | ConvertFrom-Json
}

# Force order book rebuild
kubectl exec -n pincex-production deployment/trading-engine -- curl -X POST http://localhost:8080/admin/orderbook/rebuild/BTCUSD

# Verify order book integrity
kubectl exec -n pincex-production deployment/trading-engine -- curl http://localhost:8080/admin/orderbook/validate/BTCUSD
```

### Market Making Operations

#### Enable/Disable Market Making
```powershell
# Disable market making for maintenance
kubectl exec -n pincex-production deployment/market-maker -- curl -X POST http://localhost:8080/admin/disable

# Enable market making
kubectl exec -n pincex-production deployment/market-maker -- curl -X POST http://localhost:8080/admin/enable

# Check market maker status
kubectl exec -n pincex-production deployment/market-maker -- curl http://localhost:8080/admin/status
```

#### Spread Adjustment
```powershell
# Adjust spreads during high volatility
kubectl exec -n pincex-production deployment/market-maker -- curl -X POST http://localhost:8080/admin/spreads -H "Content-Type: application/json" -d '{
  "BTCUSD": {"min_spread": 0.02, "max_spread": 0.05},
  "ETHUSD": {"min_spread": 0.02, "max_spread": 0.05}
}'
```

---

## Database Operations

### Performance Monitoring

#### Connection Pool Monitoring
```powershell
# Check connection pool status
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    state,
    count(*) as connections
FROM pg_stat_activity 
WHERE datname = 'pincex'
GROUP BY state;
"

# Check long-running queries
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    pid,
    now() - pg_stat_activity.query_start AS duration,
    query,
    state
FROM pg_stat_activity
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes'
ORDER BY duration DESC;
"
```

#### Index Usage Analysis
```powershell
# Check unused indexes
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
WHERE idx_tup_read = 0
ORDER BY schemaname, tablename;
"

# Check index efficiency
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_tup_read,
    idx_tup_fetch,
    idx_tup_read / NULLIF(idx_tup_fetch, 0) as efficiency
FROM pg_stat_user_indexes
WHERE idx_tup_fetch > 0
ORDER BY efficiency DESC;
"
```

### Backup and Recovery

#### Manual Backup
```powershell
# Create manual backup
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
kubectl exec -n pincex-production deployment/postgres-primary -- pg_dump -U admin pincex | Out-File -FilePath "backup_pincex_$timestamp.sql"

# Verify backup integrity
kubectl exec -n pincex-production deployment/postgres-primary -- pg_dump -U admin pincex --schema-only | Out-File -FilePath "schema_verification_$timestamp.sql"
```

#### Point-in-Time Recovery
```powershell
# Stop application services
kubectl scale deployment --all --replicas=0 -n pincex-production

# Restore from backup point
$restore_time = "2025-06-02 10:30:00"
kubectl exec -n pincex-production deployment/postgres-primary -- pg_basebackup -R -D /var/lib/postgresql/recovery -U replication_user

# Start services in maintenance mode
kubectl scale deployment trading-engine --replicas=1 -n pincex-production
kubectl patch deployment trading-engine -n pincex-production -p '{"spec":{"template":{"spec":{"containers":[{"name":"trading-engine","env":[{"name":"MAINTENANCE_MODE","value":"true"}]}]}}}}'
```

---

## Authentication & Security

### User Account Issues

#### Account Lockout Resolution
```powershell
# Check locked accounts
kubectl exec -n pincex-production deployment/auth-service -- curl http://localhost:8080/admin/accounts/locked

# Unlock specific account
$user_id = "12345"
kubectl exec -n pincex-production deployment/auth-service -- curl -X POST http://localhost:8080/admin/accounts/$user_id/unlock

# Check failed login attempts
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT user_id, ip_address, attempt_time, reason
FROM failed_login_attempts
WHERE attempt_time > NOW() - INTERVAL '1 hour'
ORDER BY attempt_time DESC
LIMIT 50;
"
```

#### Suspicious Activity Investigation
```powershell
# Check multiple IP logins
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    user_id,
    COUNT(DISTINCT ip_address) as unique_ips,
    array_agg(DISTINCT ip_address) as ip_addresses
FROM user_sessions
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY user_id
HAVING COUNT(DISTINCT ip_address) > 3
ORDER BY unique_ips DESC;
"

# Check high-frequency trading activity
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    user_id,
    COUNT(*) as order_count,
    SUM(quantity * price) as total_volume
FROM orders
WHERE created_at > NOW() - INTERVAL '5 minutes'
GROUP BY user_id
HAVING COUNT(*) > 100
ORDER BY order_count DESC;
"
```

### API Key Management

#### Revoke Compromised API Key
```powershell
$api_key = "ak_compromised_key_example"
kubectl exec -n pincex-production deployment/auth-service -- curl -X DELETE http://localhost:8080/admin/api-keys/$api_key

# Check usage of revoked key
kubectl logs -n pincex-production deployment/auth-service --since=1h | Select-String $api_key
```

---

## Market Data Operations

### Feed Health Monitoring

#### Check External Feed Status
```powershell
# Check feed latency
kubectl exec -n pincex-production deployment/market-data -- curl http://localhost:8080/admin/feeds/status

# Restart stuck feed
$feed_name = "binance"
kubectl exec -n pincex-production deployment/market-data -- curl -X POST http://localhost:8080/admin/feeds/$feed_name/restart

# Check message queue depth
kubectl exec -n pincex-production deployment/redis -- redis-cli llen market_data_queue
```

#### Price Feed Validation
```powershell
# Compare prices across feeds
kubectl exec -n pincex-production deployment/market-data -- curl http://localhost:8080/admin/prices/compare/BTCUSD

# Check for stale prices
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    symbol,
    price,
    timestamp,
    NOW() - timestamp as age
FROM latest_prices
WHERE NOW() - timestamp > INTERVAL '30 seconds'
ORDER BY age DESC;
"
```

### WebSocket Operations

#### Connection Management
```powershell
# Check active connections
kubectl exec -n pincex-production deployment/websocket -- curl http://localhost:8080/admin/connections/count

# Check connection distribution
kubectl exec -n pincex-production deployment/websocket -- curl http://localhost:8080/admin/connections/distribution

# Force disconnect idle connections
kubectl exec -n pincex-production deployment/websocket -- curl -X POST http://localhost:8080/admin/connections/cleanup
```

---

## Incident Response

### Severity Levels

#### Severity 1 (Critical - Trading Halt)
**Response Time**: Immediate (< 5 minutes)
**Escalation**: CTO, Trading Director, Head of Engineering

```powershell
# Immediate actions
kubectl get pods -n pincex-production --field-selector=status.phase!=Running

# Enable maintenance mode
kubectl patch configmap app-config -n pincex-production --patch='{"data":{"MAINTENANCE_MODE":"true"}}'

# Stop new order intake
kubectl scale deployment trading-engine --replicas=0 -n pincex-production

# Notify stakeholders
# Send emergency notification via configured channels
```

#### Severity 2 (High - Degraded Performance)
**Response Time**: < 15 minutes
**Escalation**: Engineering Manager, DevOps Lead

```powershell
# Scale critical services
kubectl scale deployment trading-engine --replicas=6 -n pincex-production
kubectl scale deployment market-data --replicas=4 -n pincex-production

# Enable circuit breakers
kubectl exec -n pincex-production deployment/trading-engine -- curl -X POST http://localhost:8080/admin/circuit-breaker/enable
```

#### Severity 3 (Medium - Minor Issues)
**Response Time**: < 1 hour
**Escalation**: On-call Engineer

```powershell
# Standard troubleshooting
kubectl logs -n pincex-production deployment/affected-service --tail=100

# Check resource usage
kubectl top pods -n pincex-production
```

### Communication Templates

#### Trading Halt Notification
```
URGENT: PinCEX Trading Halt

Trading has been temporarily halted due to [ISSUE_DESCRIPTION].
ETR: [ESTIMATED_TIME]
Impact: [AFFECTED_SERVICES]
Status Page: https://status.pincex.com

Updates will be provided every 15 minutes.
```

#### Resolution Notification
```
RESOLVED: PinCEX Trading Resumed

Issue: [ISSUE_DESCRIPTION]
Duration: [DOWNTIME_DURATION]
Root Cause: [ROOT_CAUSE]
Actions Taken: [REMEDIATION_STEPS]

Full post-mortem will be available within 24 hours.
```

---

## Performance Troubleshooting

### Latency Analysis

#### Order Processing Latency
```powershell
# Check processing pipeline latency
kubectl exec -n pincex-production deployment/trading-engine -- curl http://localhost:8080/admin/metrics | Select-String "latency"

# Analyze by order type
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    order_type,
    AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) as avg_processing_time,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))) as p95_processing_time
FROM orders
WHERE created_at > NOW() - INTERVAL '1 hour'
AND status IN ('FILLED', 'CANCELLED')
GROUP BY order_type;
"
```

#### Database Query Performance
```powershell
# Identify slow queries
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    query,
    calls,
    total_exec_time,
    mean_exec_time,
    max_exec_time
FROM pg_stat_statements
WHERE mean_exec_time > 100  -- queries taking more than 100ms on average
ORDER BY mean_exec_time DESC
LIMIT 10;
"

# Check for missing indexes
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    schemaname,
    tablename,
    seq_scan,
    seq_tup_read,
    idx_scan,
    idx_tup_fetch,
    seq_tup_read / seq_scan as avg_seq_read
FROM pg_stat_user_tables
WHERE seq_scan > 0
ORDER BY seq_tup_read DESC
LIMIT 10;
"
```

### Memory Analysis

#### Garbage Collection Monitoring
```powershell
# Check GC metrics for Go services
kubectl exec -n pincex-production deployment/trading-engine -- curl http://localhost:6060/debug/pprof/gc

# Force GC if needed
kubectl exec -n pincex-production deployment/trading-engine -- curl -X POST http://localhost:6060/debug/pprof/gc

# Check heap profile
kubectl exec -n pincex-production deployment/trading-engine -- go tool pprof -top http://localhost:6060/debug/pprof/heap
```

---

## Emergency Procedures

### Complete System Shutdown

#### Graceful Shutdown Sequence
```powershell
# 1. Stop accepting new orders
kubectl patch configmap app-config -n pincex-production --patch='{"data":{"ACCEPT_NEW_ORDERS":"false"}}'

# 2. Wait for order queue to drain
do {
    $queue_depth = kubectl exec -n pincex-production deployment/trading-engine -- curl -s http://localhost:8080/admin/queue/depth
    Write-Host "Queue depth: $queue_depth"
    Start-Sleep 5
} while ($queue_depth -gt 0)

# 3. Stop trading services
kubectl scale deployment trading-engine --replicas=0 -n pincex-production
kubectl scale deployment market-maker --replicas=0 -n pincex-production

# 4. Stop market data services
kubectl scale deployment market-data --replicas=0 -n pincex-production
kubectl scale deployment websocket --replicas=0 -n pincex-production

# 5. Stop supporting services
kubectl scale deployment auth-service --replicas=0 -n pincex-production
kubectl scale deployment settlement --replicas=0 -n pincex-production
```

#### Emergency Restart
```powershell
# 1. Start core infrastructure
kubectl scale deployment postgres-primary --replicas=1 -n pincex-production
kubectl scale deployment redis --replicas=3 -n pincex-production

# 2. Wait for database readiness
do {
    $db_ready = kubectl exec -n pincex-production deployment/postgres-primary -- pg_isready -h localhost -p 5432
    Start-Sleep 5
} while ($db_ready -notmatch "accepting connections")

# 3. Start auth service
kubectl scale deployment auth-service --replicas=2 -n pincex-production

# 4. Start trading engine
kubectl scale deployment trading-engine --replicas=3 -n pincex-production

# 5. Start market data
kubectl scale deployment market-data --replicas=2 -n pincex-production

# 6. Enable order acceptance
kubectl patch configmap app-config -n pincex-production --patch='{"data":{"ACCEPT_NEW_ORDERS":"true"}}'
```

### Data Corruption Recovery

#### Order Data Corruption
```powershell
# 1. Identify corruption scope
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    COUNT(*) as total_orders,
    COUNT(CASE WHEN quantity <= 0 THEN 1 END) as invalid_quantity,
    COUNT(CASE WHEN price <= 0 THEN 1 END) as invalid_price,
    COUNT(CASE WHEN status NOT IN ('PENDING', 'PARTIAL', 'FILLED', 'CANCELLED') THEN 1 END) as invalid_status
FROM orders
WHERE created_at > NOW() - INTERVAL '1 hour';
"

# 2. Restore from backup if necessary
$backup_time = "2025-06-02 10:00:00"
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
BEGIN;
DELETE FROM orders WHERE created_at > '$backup_time';
-- Restore from backup...
COMMIT;
"
```

### Contact Information

#### Emergency Contacts
- **Primary On-Call**: +1-555-0123 (engineer@pincex.com)
- **Secondary On-Call**: +1-555-0124 (backup@pincex.com)
- **Engineering Manager**: +1-555-0125 (mgr@pincex.com)
- **CTO**: +1-555-0126 (cto@pincex.com)

#### External Vendors
- **Cloud Provider Support**: +1-800-CLOUD-1
- **Database Support**: +1-800-POSTGRES
- **Monitoring Vendor**: +1-800-MONITOR

---

## Appendix

### Useful Commands Reference

#### Kubernetes Quick Commands
```powershell
# Get all pod statuses
kubectl get pods -n pincex-production -o wide

# Describe failing pod
kubectl describe pod <pod-name> -n pincex-production

# Get pod logs
kubectl logs <pod-name> -n pincex-production --tail=100

# Execute command in pod
kubectl exec -n pincex-production <pod-name> -- <command>

# Port forward for debugging
kubectl port-forward -n pincex-production deployment/trading-engine 8080:8080
```

#### Database Quick Commands
```powershell
# Connect to database
kubectl exec -it -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex

# Check replication status
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "SELECT * FROM pg_stat_replication;"

# Check table sizes
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -c "
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
"
```

### Log Locations
- **Application Logs**: Available via `kubectl logs`
- **Audit Logs**: `/var/log/pincex/audit.log` (in containers)
- **Security Logs**: `/var/log/pincex/security.log` (in containers)
- **Performance Logs**: Available via Prometheus metrics endpoint

---

## Operational Runbooks and Testing Guide

### Unit Testing
- Ensure unit tests are run on every PR and push.
- Locations: `internal/auth/`, `internal/database/`, `internal/trading/`.
- Coverage targets: >90% for critical modules.
- How to run:
  ```powershell
  go test -coverprofile=coverage.out ./internal/auth/... ./internal/database/... ./internal/trading/...
  ```

### Integration Testing
- Covers end-to-end flows across services.
- Locations: `test/integration/`.
- Services required: database, Redis, Kafka, internal services.
- How to run:
  ```powershell
  docker-compose up -d
  go test -v ./test/integration/...
  ```
- Common scenarios:
  - Order placement to settlement
  - Wallet debit/credit consistency
  - KYC workflow

### Contract Testing
- Ensures API contract stability across versions.
- Locations: `test/contracts/`.
- Example:
  ```powershell
  go test -v ./test/contracts/...
  ```

### Performance Regression Tests
- Baseline tests for throughput and latency.
- Locations: `internal/trading/`, `internal/wallet/`, `internal/bookkeeper/`.
- How to run:
  ```powershell
  go test -bench=. -benchmem ./internal/trading/ ./internal/wallet/ ./internal/bookkeeper/
  ```
- Review `perf.log` for benchmarks.

### Chaos Engineering
- Introduce failure scenarios to validate resilience.
- Script: `scripts/chaos_monkey.sh`.
- How to run:
  ```powershell
  bash scripts/chaos_monkey.sh
  ```
- Observations:
  - Service restarts
  - Network partition
  - Database failover

---
> Note: Adding these tests increases CI execution time. Consider running heavy performance and chaos tests on schedule rather than every PR.

*This runbook should be updated regularly and tested during maintenance windows.*
