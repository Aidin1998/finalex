# Adaptive Order Book Deployment Guide

## Overview

This document provides comprehensive deployment guidance for the Adaptive Order Book system, including migration strategies, monitoring setup, and operational procedures.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Environment Setup](#environment-setup)
3. [Configuration Management](#configuration-management)
4. [Deployment Strategies](#deployment-strategies)
5. [Migration Procedures](#migration-procedures)
6. [Monitoring and Alerting](#monitoring-and-alerting)
7. [Rollback Procedures](#rollback-procedures)
8. [Troubleshooting](#troubleshooting)

## Prerequisites

### System Requirements

- **Go Version**: 1.19 or higher
- **Memory**: Minimum 8GB RAM (16GB recommended for production)
- **CPU**: 4+ cores (8+ cores recommended for high-volume trading)
- **Disk**: SSD storage with 100GB+ available space
- **Network**: Low-latency network infrastructure (< 1ms for critical components)

### Dependencies

- PostgreSQL 13+ (for trade history and audit logs)
- Redis 6+ (for caching and session management)
- Prometheus + Grafana (for monitoring)
- ELK Stack (for centralized logging)

### Security Requirements

- TLS 1.3 for all external communications
- Network segmentation with firewall rules
- Service-to-service authentication
- Audit logging enabled

## Environment Setup

### Development Environment

```bash
# Clone the repository
git clone https://github.com/your-org/pincex_unified.git
cd pincex_unified

# Install dependencies
go mod download
go mod verify

# Run tests
make test-adaptive

# Build the application
make build-adaptive
```

### Production Environment Variables

```bash
# Database Configuration
export DB_HOST=your-db-host
export DB_PORT=5432
export DB_NAME=pincex_trading
export DB_USER=trading_service
export DB_PASSWORD=secure_password

# Redis Configuration
export REDIS_HOST=your-redis-host
export REDIS_PORT=6379
export REDIS_PASSWORD=redis_password

# Monitoring Configuration
export PROMETHEUS_ENDPOINT=http://prometheus:9090
export GRAFANA_ENDPOINT=http://grafana:3000

# Trading Configuration
export TRADING_ENV=production
export LOG_LEVEL=info
export METRICS_COLLECTION_INTERVAL=10s
export PERFORMANCE_REPORT_INTERVAL=5m
```

## Configuration Management

### Configuration Presets

The system provides five predefined configuration presets:

#### 1. Conservative (Recommended for Initial Deployment)
```go
config := engine.NewConfigurationPresets().ConservativeConfig()
```
- **Initial Migration**: 5%
- **Max Migration**: 50%
- **Step Size**: 5%
- **Step Interval**: 10 minutes
- **Auto-Migration**: Disabled

#### 2. Production (Standard Production Configuration)
```go
config := engine.NewConfigurationPresets().ProductionConfig()
```
- **Initial Migration**: 10%
- **Max Migration**: 100%
- **Step Size**: 10%
- **Step Interval**: 5 minutes
- **Auto-Migration**: Enabled with conservative thresholds

#### 3. Aggressive (High-Confidence Deployments)
```go
config := engine.NewConfigurationPresets().AggressiveConfig()
```
- **Initial Migration**: 20%
- **Max Migration**: 100%
- **Step Size**: 20%
- **Step Interval**: 3 minutes
- **Auto-Migration**: Enabled with relaxed thresholds

### Pair-Specific Configuration

```go
config := engine.NewConfigBuilder().
    EnableAdaptiveOrderBooks(true).
    WithMigrationConfig(defaultConfig).
    WithPairSpecificConfig("BTCUSDT", btcSpecificConfig).
    WithPairSpecificConfig("ETHUSDT", ethSpecificConfig).
    Build()
```

## Deployment Strategies

### Strategy 1: Blue-Green Deployment (Recommended)

```bash
# Phase 1: Deploy to Green Environment
kubectl apply -f k8s/green/adaptive-trading-service.yaml

# Phase 2: Health Check
kubectl exec -it green-trading-pod -- /app/health-check

# Phase 3: Switch Traffic (using service mesh)
kubectl patch service trading-service -p '{"spec":{"selector":{"version":"green"}}}'

# Phase 4: Monitor for 30 minutes
./scripts/monitor-deployment.sh --duration=30m --environment=green

# Phase 5: Decommission Blue (after validation)
kubectl delete -f k8s/blue/
```

### Strategy 2: Canary Deployment

```bash
# Phase 1: Deploy Canary (10% traffic)
kubectl apply -f k8s/canary/adaptive-trading-canary.yaml

# Phase 2: Gradual Traffic Increase
./scripts/canary-rollout.sh --start=10 --target=100 --step=10 --interval=10m

# Phase 3: Full Rollout
kubectl apply -f k8s/production/adaptive-trading-service.yaml
```

### Strategy 3: Rolling Update

```bash
# For Kubernetes deployments
kubectl set image deployment/trading-service \
    trading-container=your-registry/adaptive-trading:v2.0.0

# Monitor rollout
kubectl rollout status deployment/trading-service --timeout=600s
```

## Migration Procedures

### Pre-Migration Checklist

- [ ] Database backups completed
- [ ] Monitoring systems operational
- [ ] Circuit breakers configured
- [ ] Rollback plan tested
- [ ] Team communication channels established
- [ ] Performance baselines captured

### Migration Phases

#### Phase 1: Preparation
```bash
# 1. Deploy adaptive service without migration
./deploy.sh --config=conservative --enable-migration=false

# 2. Verify health
curl -f http://service:8080/health

# 3. Warm up caches
./scripts/cache-warmup.sh --pairs=BTCUSDT,ETHUSDT
```

#### Phase 2: Initial Migration (5-10%)
```bash
# Start migration for major pairs
curl -X POST http://service:8080/admin/migration/start \
    -H "Content-Type: application/json" \
    -d '{"pair": "BTCUSDT", "target_percentage": 10}'

# Monitor for 30 minutes
./scripts/monitor-migration.sh --pair=BTCUSDT --duration=30m
```

#### Phase 3: Gradual Rollout
```bash
# Increase migration percentage based on performance
for percentage in 25 50 75 100; do
    curl -X POST http://service:8080/admin/migration/percentage \
        -H "Content-Type: application/json" \
        -d "{\"pair\": \"BTCUSDT\", \"percentage\": $percentage}"
    
    sleep 600  # Wait 10 minutes between steps
done
```

#### Phase 4: Multi-Pair Rollout
```bash
# Expand to other trading pairs
for pair in ETHUSDT ADAUSDT DOTUSDT; do
    curl -X POST http://service:8080/admin/migration/start \
        -H "Content-Type: application/json" \
        -d "{\"pair\": \"$pair\", \"target_percentage\": 100}"
    
    sleep 300  # 5 minutes between pairs
done
```

## Monitoring and Alerting

### Key Performance Indicators (KPIs)

#### Latency Metrics
- **Order Processing Latency**: < 10ms (P95)
- **Trade Execution Latency**: < 5ms (P95)
- **Market Data Latency**: < 1ms (P99)

#### Throughput Metrics
- **Orders Per Second**: > 10,000 OPS
- **Trades Per Second**: > 5,000 TPS
- **Market Data Updates**: > 50,000 UPS

#### Reliability Metrics
- **Error Rate**: < 0.01%
- **Circuit Breaker Activations**: < 1 per hour
- **Deadlock Incidents**: 0

### Monitoring Setup

#### Prometheus Configuration
```yaml
# prometheus.yml
global:
  scrape_interval: 10s
  evaluation_interval: 10s

scrape_configs:
  - job_name: 'adaptive-trading'
    static_configs:
      - targets: ['trading-service:8080']
    metrics_path: /metrics
    scrape_interval: 5s
```

#### Grafana Dashboards

**Trading Performance Dashboard**
- Order processing rates
- Latency distribution
- Error rates by pair
- Migration progress

**System Health Dashboard**
- Memory usage
- CPU utilization
- Goroutine count
- GC metrics

#### Alert Rules

```yaml
# alerts.yml
groups:
  - name: adaptive_trading
    rules:
      - alert: HighLatency
        expr: trading_order_processing_latency_p95 > 0.010
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "High order processing latency detected"
          
      - alert: CircuitBreakerOpen
        expr: trading_circuit_breaker_open == 1
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Circuit breaker opened for {{ $labels.pair }}"
          
      - alert: HighErrorRate
        expr: rate(trading_errors_total[5m]) > 0.001
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
```

### Real-Time Monitoring Commands

```bash
# Monitor migration status
watch -n 5 'curl -s http://service:8080/admin/migration/status | jq'

# Check performance metrics
curl -s http://service:8080/metrics | grep trading_latency

# View recent logs
kubectl logs -f deployment/trading-service --tail=100

# Monitor resource usage
kubectl top pods -l app=trading-service
```

## Rollback Procedures

### Automatic Rollback Triggers

The system automatically triggers rollbacks when:
- Error rate exceeds 1% for 5 consecutive minutes
- Latency P95 exceeds 50ms for 3 consecutive minutes
- Circuit breaker opens more than 3 times in 10 minutes
- Memory usage exceeds 90% for 2 minutes

### Manual Rollback

#### Emergency Rollback (< 30 seconds)
```bash
# Immediately stop all migrations
curl -X POST http://service:8080/admin/migration/emergency-stop

# Verify rollback
curl -s http://service:8080/admin/migration/status | jq '.[] | select(.current_percentage > 0)'
```

#### Gradual Rollback
```bash
# Reduce migration percentage gradually
for percentage in 75 50 25 0; do
    curl -X POST http://service:8080/admin/migration/percentage \
        -H "Content-Type: application/json" \
        -d "{\"pair\": \"BTCUSDT\", \"percentage\": $percentage}"
    
    sleep 120  # Wait 2 minutes between steps
done
```

#### Database Rollback
```bash
# If database schema changes are involved
pg_dump -h $DB_HOST -U $DB_USER pincex_trading > backup_pre_migration.sql

# To rollback database (if needed)
psql -h $DB_HOST -U $DB_USER pincex_trading < backup_pre_migration.sql
```

## Troubleshooting

### Common Issues

#### High Memory Usage
```bash
# Check for memory leaks
curl -s http://service:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Restart service if necessary
kubectl rollout restart deployment/trading-service
```

#### Circuit Breaker Issues
```bash
# Reset circuit breaker
curl -X POST http://service:8080/admin/circuit-breaker/reset/BTCUSDT

# Check circuit breaker status
curl -s http://service:8080/admin/circuit-breaker/status
```

#### Performance Degradation
```bash
# Check system resources
kubectl describe node
kubectl top nodes

# Analyze slow queries
curl -s http://service:8080/admin/slow-queries

# Profile CPU usage
curl -s http://service:8080/debug/pprof/profile > profile.prof
go tool pprof profile.prof
```

### Log Analysis

#### Important Log Patterns
```bash
# Migration events
grep "migration" /var/log/trading/application.log

# Error patterns
grep -E "(ERROR|FATAL)" /var/log/trading/application.log | tail -100

# Performance warnings
grep "latency\|throughput" /var/log/trading/application.log
```

#### Log Aggregation (ELK Stack)
```json
{
  "query": {
    "bool": {
      "must": [
        {"match": {"service": "adaptive-trading"}},
        {"range": {"@timestamp": {"gte": "now-1h"}}}
      ]
    }
  }
}
```

### Performance Tuning

#### JVM Tuning (if applicable)
```bash
# Garbage collection tuning
export GOMAXPROCS=8
export GOGC=100
```

#### Database Optimization
```sql
-- Index optimization
CREATE INDEX CONCURRENTLY idx_orders_timestamp_pair 
ON orders (created_at, pair) WHERE status = 'active';

-- Connection pooling
ALTER SYSTEM SET max_connections = 200;
ALTER SYSTEM SET shared_buffers = '4GB';
```

## Post-Deployment Validation

### Automated Tests
```bash
# Run integration tests
make test-integration

# Performance benchmarks
make benchmark-trading

# Load testing
./scripts/load-test.sh --duration=10m --rate=1000rps
```

### Manual Validation
1. **Order Book Integrity**: Verify price-time priority
2. **Trade Matching**: Confirm accurate trade execution
3. **Market Data**: Validate real-time data accuracy
4. **Performance**: Measure latency and throughput
5. **Monitoring**: Verify all metrics are collecting

### Success Criteria
- [ ] All automated tests passing
- [ ] Error rate < 0.01%
- [ ] Latency P95 < 10ms
- [ ] Throughput > 10,000 OPS
- [ ] No circuit breaker activations
- [ ] Zero deadlock incidents
- [ ] Monitoring systems operational
- [ ] Rollback procedures tested

## Support and Escalation

### Contact Information
- **Primary On-Call**: trading-oncall@company.com
- **Engineering Team**: trading-eng@company.com
- **DevOps Team**: devops@company.com

### Escalation Matrix
1. **Level 1**: Service degradation (< 2 hour response)
2. **Level 2**: Service outage (< 30 minute response)
3. **Level 3**: Data integrity issues (< 15 minute response)

### Documentation Links
- [API Documentation](./API_REFERENCE.md)
- [Operational Runbook](./OPERATIONS_RUNBOOK.md)
- [Architecture Overview](./ARCHITECTURE.md)
- [Performance Tuning Guide](./PERFORMANCE_TUNING.md)
