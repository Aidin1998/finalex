# Operational Runbook - Adaptive Order Book System

## Overview

This runbook provides step-by-step operational procedures for the Adaptive Order Book system, covering day-to-day operations, incident response, and maintenance tasks.

## Table of Contents

1. [System Architecture](#system-architecture)
2. [Daily Operations](#daily-operations)
3. [Incident Response](#incident-response)
4. [Maintenance Procedures](#maintenance-procedures)
5. [Performance Monitoring](#performance-monitoring)
6. [Emergency Procedures](#emergency-procedures)
7. [Health Checks](#health-checks)
8. [Troubleshooting Guide](#troubleshooting-guide)

## System Architecture

### Component Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Load Balancer │    │   API Gateway   │    │   Trading UI    │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │
                   ┌─────────────▼───────────────┐
                   │    Adaptive Trading        │
                   │       Service              │
                   └─────────────┬───────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
          ▼                      ▼                      ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   PostgreSQL    │    │   Redis Cache   │    │   Monitoring    │
│   (Trades/Orders│    │   (Sessions)    │    │   (Metrics)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Key Components

- **Adaptive Trading Service**: Core trading engine with migration capabilities
- **Order Book Manager**: Handles both old and new order book implementations
- **Performance Monitor**: Real-time performance tracking and alerting
- **Migration Controller**: Manages gradual rollout of new order book
- **Circuit Breaker**: Automatic fallback mechanism for reliability

## Daily Operations

### Morning Checklist (Start of Trading Day)

```bash
#!/bin/bash
# daily_startup_checklist.sh

echo "=== Daily Startup Checklist ==="

# 1. Check service health
echo "1. Checking service health..."
curl -f http://trading-service:8080/health || exit 1

# 2. Verify database connectivity
echo "2. Checking database connectivity..."
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT 1;" || exit 1

# 3. Check Redis connectivity
echo "3. Checking Redis connectivity..."
redis-cli -h $REDIS_HOST ping || exit 1

# 4. Verify migration states
echo "4. Checking migration states..."
curl -s http://trading-service:8080/admin/migration/status | jq '.'

# 5. Check overnight performance
echo "5. Reviewing overnight performance..."
curl -s http://trading-service:8080/admin/metrics/summary | jq '.last_24h'

# 6. Verify monitoring systems
echo "6. Checking monitoring systems..."
curl -f http://prometheus:9090/-/healthy || exit 1
curl -f http://grafana:3000/api/health || exit 1

# 7. Check for any circuit breaker activations
echo "7. Checking circuit breakers..."
curl -s http://trading-service:8080/admin/circuit-breaker/status | \
    jq '.[] | select(.open == true)'

echo "=== Startup checklist completed ==="
```

### End of Day Checklist

```bash
#!/bin/bash
# daily_shutdown_checklist.sh

echo "=== End of Day Checklist ==="

# 1. Generate daily performance report
echo "1. Generating daily performance report..."
curl -s http://trading-service:8080/admin/reports/daily > daily_report_$(date +%Y%m%d).json

# 2. Check for any unresolved issues
echo "2. Checking for unresolved issues..."
curl -s http://trading-service:8080/admin/alerts/active | jq '.'

# 3. Backup critical metrics
echo "3. Backing up metrics..."
curl -s http://prometheus:9090/api/v1/query_range \
    -G --data-urlencode 'query=trading_orders_total' \
    --data-urlencode "start=$(date -d '1 day ago' +%s)" \
    --data-urlencode "end=$(date +%s)" \
    --data-urlencode 'step=300' > metrics_backup_$(date +%Y%m%d).json

# 4. Verify log rotation
echo "4. Checking log files..."
ls -la /var/log/trading/

# 5. Check resource usage trends
echo "5. Resource usage summary..."
curl -s http://trading-service:8080/admin/system/resources | jq '.'

echo "=== End of day checklist completed ==="
```

## Incident Response

### Severity Levels

#### Level 1 (Critical) - 15 minute response time
- Complete trading outage
- Data corruption
- Security breach
- Multiple circuit breakers open

#### Level 2 (High) - 30 minute response time
- Significant performance degradation (>50% slower)
- Single pair trading issues
- Circuit breaker activation
- Memory leak detection

#### Level 3 (Medium) - 2 hour response time
- Minor performance degradation
- Non-critical feature issues
- Monitoring alert false positives

### Incident Response Procedures

#### Critical Incident Response
```bash
#!/bin/bash
# critical_incident_response.sh

echo "=== CRITICAL INCIDENT RESPONSE ==="

# 1. Immediate assessment
echo "1. Assessing system status..."
./scripts/health_check_comprehensive.sh

# 2. Stop all migrations immediately
echo "2. Emergency migration stop..."
curl -X POST http://trading-service:8080/admin/migration/emergency-stop

# 3. Enable circuit breakers for protection
echo "3. Activating protective measures..."
curl -X POST http://trading-service:8080/admin/circuit-breaker/activate-all

# 4. Capture system state
echo "4. Capturing system state for analysis..."
kubectl describe pods > incident_state_$(date +%Y%m%d_%H%M%S).log
kubectl logs deployment/trading-service --tail=1000 > incident_logs_$(date +%Y%m%d_%H%M%S).log

# 5. Notify stakeholders
echo "5. Sending notifications..."
./scripts/notify_incident.sh --severity=critical --status=active

echo "=== Initial response completed - Begin investigation ==="
```

### Common Incident Scenarios

#### Scenario 1: High Latency Alert
```bash
# Investigation steps
echo "Investigating high latency incident..."

# Check current latency metrics
curl -s http://trading-service:8080/metrics | grep latency_p95

# Check CPU and memory usage
kubectl top pods -l app=trading-service

# Check for slow database queries
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "
SELECT query, mean_time, calls 
FROM pg_stat_statements 
ORDER BY mean_time DESC 
LIMIT 10;"

# Check GC metrics
curl -s http://trading-service:8080/debug/pprof/heap > heap_$(date +%H%M%S).prof

# Temporary mitigation: Reduce migration percentage
curl -X POST http://trading-service:8080/admin/migration/percentage \
    -d '{"pair": "all", "percentage": 0}'
```

#### Scenario 2: Circuit Breaker Activation
```bash
# Investigation steps
echo "Investigating circuit breaker activation..."

# Check which pairs are affected
curl -s http://trading-service:8080/admin/circuit-breaker/status | \
    jq '.[] | select(.open == true)'

# Check error rates and types
curl -s http://trading-service:8080/admin/errors/summary | jq '.'

# Review recent error logs
kubectl logs deployment/trading-service --tail=500 | grep ERROR

# Check for deadlock patterns
grep -i deadlock /var/log/trading/application.log

# Reset circuit breaker after investigation
curl -X POST http://trading-service:8080/admin/circuit-breaker/reset/BTCUSDT
```

#### Scenario 3: Memory Leak Detection
```bash
# Investigation steps
echo "Investigating memory leak..."

# Capture heap profile
curl -s http://trading-service:8080/debug/pprof/heap > heap_leak_$(date +%H%M%S).prof

# Check goroutine count
curl -s http://trading-service:8080/debug/pprof/goroutine?debug=1 | head -20

# Monitor memory growth rate
for i in {1..10}; do
    kubectl top pods -l app=trading-service
    sleep 30
done

# Emergency mitigation: Restart affected pods
kubectl rollout restart deployment/trading-service
```

## Maintenance Procedures

### Planned Maintenance Window

```bash
#!/bin/bash
# planned_maintenance.sh

echo "=== Starting Planned Maintenance ==="

# 1. Pre-maintenance health check
echo "1. Pre-maintenance health check..."
./scripts/health_check_comprehensive.sh

# 2. Reduce traffic gradually
echo "2. Reducing traffic..."
kubectl scale deployment/trading-service --replicas=2

# 3. Stop all active migrations
echo "3. Stopping migrations..."
curl -X POST http://trading-service:8080/admin/migration/stop-all

# 4. Wait for in-flight orders to complete
echo "4. Waiting for order completion..."
sleep 30

# 5. Enable maintenance mode
echo "5. Enabling maintenance mode..."
curl -X POST http://trading-service:8080/admin/maintenance/enable

# 6. Perform maintenance tasks
echo "6. Performing maintenance..."
# Add specific maintenance tasks here

# 7. Disable maintenance mode
echo "7. Disabling maintenance mode..."
curl -X POST http://trading-service:8080/admin/maintenance/disable

# 8. Scale back up
echo "8. Scaling back up..."
kubectl scale deployment/trading-service --replicas=4

# 9. Post-maintenance health check
echo "9. Post-maintenance health check..."
./scripts/health_check_comprehensive.sh

echo "=== Maintenance completed ==="
```

### Database Maintenance

```sql
-- Weekly database maintenance
-- Run during low-traffic periods

-- 1. Update table statistics
ANALYZE;

-- 2. Vacuum tables
VACUUM (ANALYZE, VERBOSE) orders;
VACUUM (ANALYZE, VERBOSE) trades;
VACUUM (ANALYZE, VERBOSE) order_book_snapshots;

-- 3. Reindex if needed
REINDEX INDEX CONCURRENTLY idx_orders_timestamp;

-- 4. Check for bloat
SELECT schemaname, tablename, 
       pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size,
       n_dead_tup, n_live_tup
FROM pg_stat_user_tables 
WHERE n_dead_tup > 1000
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;

-- 5. Archive old data (older than 90 days)
DELETE FROM order_book_snapshots 
WHERE created_at < NOW() - INTERVAL '90 days';
```

## Performance Monitoring

### Key Metrics Dashboard

#### Real-Time Monitoring
```bash
#!/bin/bash
# realtime_monitoring.sh

while true; do
    clear
    echo "=== Real-Time Trading System Monitor ==="
    echo "Timestamp: $(date)"
    echo
    
    # Service health
    echo "Service Health:"
    curl -s http://trading-service:8080/health | jq '.status'
    echo
    
    # Current latency
    echo "Latency Metrics (P95):"
    curl -s http://trading-service:8080/metrics | grep trading_latency_p95 | tail -5
    echo
    
    # Throughput
    echo "Throughput (last 1 minute):"
    curl -s http://trading-service:8080/admin/metrics/throughput | jq '.last_minute'
    echo
    
    # Active migrations
    echo "Active Migrations:"
    curl -s http://trading-service:8080/admin/migration/status | \
        jq '.[] | select(.current_percentage > 0) | {pair: .pair, percentage: .current_percentage}'
    echo
    
    # Circuit breaker status
    echo "Circuit Breakers:"
    curl -s http://trading-service:8080/admin/circuit-breaker/status | \
        jq '.[] | select(.open == true) | {pair: .pair, reason: .reason}'
    echo
    
    # Resource usage
    echo "Resource Usage:"
    kubectl top pods -l app=trading-service | tail -n +2
    echo
    
    sleep 10
done
```

### Performance Baselines

#### Daily Performance Report
```bash
#!/bin/bash
# generate_performance_report.sh

DATE=$(date +%Y-%m-%d)
REPORT_FILE="performance_report_$DATE.json"

echo "Generating performance report for $DATE..."

# Collect metrics from the last 24 hours
curl -s "http://prometheus:9090/api/v1/query_range" \
    -G --data-urlencode 'query=avg(trading_latency_p95)' \
    --data-urlencode "start=$(date -d '1 day ago' +%s)" \
    --data-urlencode "end=$(date +%s)" \
    --data-urlencode 'step=300' > latency_$DATE.json

curl -s "http://prometheus:9090/api/v1/query_range" \
    -G --data-urlencode 'query=sum(rate(trading_orders_total[5m]))' \
    --data-urlencode "start=$(date -d '1 day ago' +%s)" \
    --data-urlencode "end=$(date +%s)" \
    --data-urlencode 'step=300' > throughput_$DATE.json

# Generate summary report
cat > $REPORT_FILE << EOF
{
  "date": "$DATE",
  "summary": {
    "avg_latency_p95": $(curl -s http://trading-service:8080/admin/metrics/summary | jq '.avg_latency_p95'),
    "total_orders": $(curl -s http://trading-service:8080/admin/metrics/summary | jq '.total_orders_24h'),
    "total_trades": $(curl -s http://trading-service:8080/admin/metrics/summary | jq '.total_trades_24h'),
    "error_rate": $(curl -s http://trading-service:8080/admin/metrics/summary | jq '.error_rate_24h'),
    "peak_throughput": $(curl -s http://trading-service:8080/admin/metrics/summary | jq '.peak_throughput_24h')
  },
  "migration_summary": $(curl -s http://trading-service:8080/admin/migration/summary),
  "circuit_breaker_activations": $(curl -s http://trading-service:8080/admin/circuit-breaker/history | jq '.last_24h | length')
}
EOF

echo "Performance report generated: $REPORT_FILE"
```

## Emergency Procedures

### Emergency Stop Procedure

```bash
#!/bin/bash
# emergency_stop.sh

echo "=== EMERGENCY STOP PROCEDURE ==="
echo "WARNING: This will halt all trading operations"
echo "Press Ctrl+C within 10 seconds to abort..."
sleep 10

# 1. Halt all new order processing
echo "1. Halting new order processing..."
curl -X POST http://trading-service:8080/admin/trading/halt

# 2. Complete in-flight orders
echo "2. Completing in-flight orders..."
sleep 5

# 3. Stop all migrations
echo "3. Stopping all migrations..."
curl -X POST http://trading-service:8080/admin/migration/emergency-stop

# 4. Activate all circuit breakers
echo "4. Activating protective circuit breakers..."
curl -X POST http://trading-service:8080/admin/circuit-breaker/activate-all

# 5. Scale down to minimum replicas
echo "5. Scaling down service..."
kubectl scale deployment/trading-service --replicas=1

# 6. Notify stakeholders
echo "6. Sending emergency notifications..."
./scripts/notify_incident.sh --severity=critical --status=emergency_stop

echo "=== EMERGENCY STOP COMPLETED ==="
echo "To resume operations, run: ./emergency_resume.sh"
```

### Emergency Resume Procedure

```bash
#!/bin/bash
# emergency_resume.sh

echo "=== EMERGENCY RESUME PROCEDURE ==="

# 1. Comprehensive health check
echo "1. Performing comprehensive health check..."
./scripts/health_check_comprehensive.sh || exit 1

# 2. Reset circuit breakers
echo "2. Resetting circuit breakers..."
curl -X POST http://trading-service:8080/admin/circuit-breaker/reset-all

# 3. Scale up service
echo "3. Scaling up service..."
kubectl scale deployment/trading-service --replicas=4

# 4. Wait for pods to be ready
echo "4. Waiting for pods to be ready..."
kubectl wait --for=condition=Ready pod -l app=trading-service --timeout=300s

# 5. Resume trading
echo "5. Resuming trading operations..."
curl -X POST http://trading-service:8080/admin/trading/resume

# 6. Verify operations
echo "6. Verifying operations..."
sleep 10
curl -s http://trading-service:8080/health | jq '.'

# 7. Notify stakeholders
echo "7. Sending resume notifications..."
./scripts/notify_incident.sh --severity=info --status=resumed

echo "=== EMERGENCY RESUME COMPLETED ==="
```

## Health Checks

### Comprehensive Health Check

```bash
#!/bin/bash
# health_check_comprehensive.sh

echo "=== Comprehensive Health Check ==="

OVERALL_STATUS="HEALTHY"

# 1. Service health endpoint
echo "1. Checking service health endpoint..."
if ! curl -f http://trading-service:8080/health; then
    echo "❌ Service health check failed"
    OVERALL_STATUS="UNHEALTHY"
else
    echo "✅ Service health check passed"
fi

# 2. Database connectivity
echo "2. Checking database connectivity..."
if ! psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT 1;" > /dev/null 2>&1; then
    echo "❌ Database connectivity failed"
    OVERALL_STATUS="UNHEALTHY"
else
    echo "✅ Database connectivity passed"
fi

# 3. Redis connectivity
echo "3. Checking Redis connectivity..."
if ! redis-cli -h $REDIS_HOST ping > /dev/null 2>&1; then
    echo "❌ Redis connectivity failed"
    OVERALL_STATUS="UNHEALTHY"
else
    echo "✅ Redis connectivity passed"
fi

# 4. Check for circuit breaker issues
echo "4. Checking circuit breakers..."
OPEN_BREAKERS=$(curl -s http://trading-service:8080/admin/circuit-breaker/status | jq '[.[] | select(.open == true)] | length')
if [ "$OPEN_BREAKERS" -gt 0 ]; then
    echo "⚠️  $OPEN_BREAKERS circuit breakers are open"
    OVERALL_STATUS="DEGRADED"
else
    echo "✅ All circuit breakers are closed"
fi

# 5. Check latency
echo "5. Checking latency metrics..."
LATENCY=$(curl -s http://trading-service:8080/admin/metrics/current | jq '.latency_p95_ms')
if (( $(echo "$LATENCY > 20" | bc -l) )); then
    echo "⚠️  High latency detected: ${LATENCY}ms"
    OVERALL_STATUS="DEGRADED"
else
    echo "✅ Latency within normal range: ${LATENCY}ms"
fi

# 6. Check error rate
echo "6. Checking error rate..."
ERROR_RATE=$(curl -s http://trading-service:8080/admin/metrics/current | jq '.error_rate')
if (( $(echo "$ERROR_RATE > 0.01" | bc -l) )); then
    echo "⚠️  High error rate detected: ${ERROR_RATE}"
    OVERALL_STATUS="DEGRADED"
else
    echo "✅ Error rate within normal range: ${ERROR_RATE}"
fi

# 7. Check resource usage
echo "7. Checking resource usage..."
MEMORY_USAGE=$(kubectl top pods -l app=trading-service --no-headers | awk '{sum+=$3} END {print sum}' | sed 's/Mi//')
if [ "$MEMORY_USAGE" -gt 6000 ]; then
    echo "⚠️  High memory usage detected: ${MEMORY_USAGE}Mi"
    OVERALL_STATUS="DEGRADED"
else
    echo "✅ Memory usage within normal range: ${MEMORY_USAGE}Mi"
fi

echo
echo "=== Overall Status: $OVERALL_STATUS ==="

case $OVERALL_STATUS in
    "HEALTHY")
        exit 0
        ;;
    "DEGRADED")
        exit 1
        ;;
    "UNHEALTHY")
        exit 2
        ;;
esac
```

## Troubleshooting Guide

### Common Issues and Solutions

#### Issue 1: Slow Order Processing

**Symptoms:**
- Latency P95 > 20ms
- Increasing order queue depth
- User complaints about slow execution

**Investigation:**
```bash
# Check CPU usage
kubectl top nodes
kubectl top pods

# Check database performance
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "
SELECT query, mean_time, calls, total_time
FROM pg_stat_statements 
WHERE mean_time > 10
ORDER BY mean_time DESC;"

# Check for lock contention
curl -s http://trading-service:8080/debug/pprof/mutex > mutex.prof
go tool pprof mutex.prof
```

**Solutions:**
1. Scale up pods: `kubectl scale deployment/trading-service --replicas=6`
2. Optimize database queries
3. Increase connection pool size
4. Temporarily reduce migration percentage

#### Issue 2: Memory Leak

**Symptoms:**
- Steadily increasing memory usage
- OOM kills in Kubernetes
- Performance degradation over time

**Investigation:**
```bash
# Capture heap profile
curl -s http://trading-service:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Check goroutine count
curl -s http://trading-service:8080/debug/pprof/goroutine?debug=1

# Monitor memory growth
watch 'kubectl top pods -l app=trading-service'
```

**Solutions:**
1. Restart affected pods: `kubectl rollout restart deployment/trading-service`
2. Adjust GOGC environment variable
3. Review code for goroutine leaks
4. Implement memory limits in Kubernetes

#### Issue 3: Database Connection Issues

**Symptoms:**
- Connection timeouts
- "Too many connections" errors
- Database unavailability

**Investigation:**
```bash
# Check connection count
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "
SELECT count(*) as connections, state 
FROM pg_stat_activity 
GROUP BY state;"

# Check for long-running queries
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "
SELECT pid, now() - pg_stat_activity.query_start AS duration, query 
FROM pg_stat_activity 
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes';"
```

**Solutions:**
1. Increase max_connections in PostgreSQL
2. Optimize connection pooling
3. Kill long-running queries
4. Scale database resources

### Escalation Procedures

#### When to Escalate

**Immediate Escalation (Level 3):**
- Complete service outage
- Data corruption detected
- Security incident
- Multiple circuit breakers open

**Escalate within 30 minutes (Level 2):**
- Performance degradation > 50%
- Single trading pair failure
- Memory leak causing restarts

**Escalate within 2 hours (Level 1):**
- Minor performance issues
- Non-critical feature problems
- Monitoring alerts

#### Escalation Contacts

```bash
#!/bin/bash
# escalate_incident.sh

SEVERITY=$1
DESCRIPTION=$2

case $SEVERITY in
    "critical")
        # Page on-call engineer immediately
        curl -X POST "https://api.pagerduty.com/incidents" \
            -H "Authorization: Token $PAGERDUTY_TOKEN" \
            -d "{\"incident\":{\"type\":\"incident\",\"title\":\"Critical Trading Issue\",\"service\":{\"id\":\"$SERVICE_ID\"},\"urgency\":\"high\"}}"
        
        # Send SMS to team leads
        aws sns publish --topic-arn $CRITICAL_SNS_TOPIC --message "$DESCRIPTION"
        ;;
    "high")
        # Create incident ticket
        curl -X POST "https://company.atlassian.net/rest/api/2/issue" \
            -H "Authorization: Bearer $JIRA_TOKEN" \
            -d "{\"fields\":{\"project\":{\"key\":\"TRADING\"},\"summary\":\"Trading Performance Issue\",\"issuetype\":{\"name\":\"Incident\"}}}"
        ;;
    "medium")
        # Send email notification
        echo "$DESCRIPTION" | mail -s "Trading System Alert" trading-team@company.com
        ;;
esac
```

## Contact Information

### Team Contacts
- **Trading Engineering**: trading-eng@company.com
- **DevOps Team**: devops@company.com
- **Database Team**: dba@company.com
- **Security Team**: security@company.com

### Emergency Contacts
- **Primary On-Call**: +1-555-TRADING (24/7)
- **Engineering Manager**: +1-555-ENG-MGR
- **VP Engineering**: +1-555-VP-ENG

### External Vendors
- **Cloud Provider Support**: [Provider Support Portal]
- **Database Vendor**: [Database Support Portal]
- **Monitoring Vendor**: [Monitoring Support Portal]
