# Ultra-High Concurrency Database Layer - Operational Guide

## Overview

This operational guide provides comprehensive instructions for deploying, monitoring, and maintaining the ultra-high concurrency database layer for the Accounts module in production environments. The system is designed to handle 100,000+ requests per second with zero data loss guarantees.

## Table of Contents
1. [Production Deployment](#production-deployment)
2. [System Monitoring](#system-monitoring)
3. [Performance Tuning](#performance-tuning)
4. [Disaster Recovery](#disaster-recovery)
5. [Maintenance Procedures](#maintenance-procedures)
6. [Security Operations](#security-operations)
7. [Capacity Planning](#capacity-planning)
8. [Incident Response](#incident-response)
9. [Health Checks](#health-checks)
10. [Operational Runbooks](#operational-runbooks)

## Production Deployment

### Infrastructure Requirements

#### Minimum Hardware Specifications
```yaml
# Production environment specifications
database_servers:
  postgresql_primary:
    cpu: "16 cores (3.2GHz+)"
    memory: "64GB RAM"
    storage: "2TB NVMe SSD (RAID 10)"
    network: "10Gbps"
    
  postgresql_replicas:
    count: 3
    cpu: "8 cores (3.2GHz+)"
    memory: "32GB RAM" 
    storage: "1TB NVMe SSD"
    network: "10Gbps"

cache_servers:
  redis_cluster:
    nodes: 6
    cpu: "8 cores (3.2GHz+)"
    memory: "32GB RAM"
    storage: "512GB NVMe SSD"
    network: "10Gbps"

application_servers:
  accounts_service:
    instances: 6
    cpu: "8 cores (3.2GHz+)"
    memory: "16GB RAM"
    storage: "100GB SSD"
    network: "10Gbps"
```

#### Network Configuration
```yaml
# Network topology
network:
  vpc_cidr: "10.0.0.0/16"
  
  subnets:
    public:
      - "10.0.1.0/24"  # Load balancers
      - "10.0.2.0/24"  # NAT gateways
      
    private:
      - "10.0.10.0/24" # Application servers
      - "10.0.11.0/24" # Database servers
      - "10.0.12.0/24" # Cache servers
      
    data:
      - "10.0.20.0/24" # Database replication
      - "10.0.21.0/24" # Backup storage

  security_groups:
    database:
      ingress:
        - port: 5432
          source: "10.0.10.0/24"  # App servers only
        - port: 5432
          source: "10.0.20.0/24"  # Replication traffic
          
    cache:
      ingress:
        - port: 6379
          source: "10.0.10.0/24"  # App servers only
        - port: 16379
          source: "10.0.12.0/24"  # Cluster bus
```

### Deployment Process

#### 1. Database Deployment
```bash
#!/bin/bash
# Database deployment script

# Deploy PostgreSQL primary
kubectl apply -f k8s/postgresql-primary.yaml

# Wait for primary to be ready
kubectl wait --for=condition=ready pod -l app=postgresql-primary --timeout=300s

# Deploy read replicas
kubectl apply -f k8s/postgresql-replicas.yaml

# Verify replication status
kubectl exec postgresql-primary-0 -- psql -c "SELECT * FROM pg_stat_replication;"
```

#### 2. Redis Cluster Deployment
```bash
#!/bin/bash
# Redis cluster deployment script

# Deploy Redis nodes
kubectl apply -f k8s/redis-cluster.yaml

# Wait for all nodes to be ready
kubectl wait --for=condition=ready pod -l app=redis-cluster --timeout=300s

# Initialize cluster
kubectl exec redis-cluster-0 -- redis-cli --cluster create \
  redis-cluster-0.redis-cluster:6379 \
  redis-cluster-1.redis-cluster:6379 \
  redis-cluster-2.redis-cluster:6379 \
  redis-cluster-3.redis-cluster:6379 \
  redis-cluster-4.redis-cluster:6379 \
  redis-cluster-5.redis-cluster:6379 \
  --cluster-replicas 1 --cluster-yes

# Verify cluster status
kubectl exec redis-cluster-0 -- redis-cli cluster info
```

#### 3. Application Deployment
```yaml
# k8s/accounts-service.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: accounts-service
  namespace: production
spec:
  replicas: 6
  selector:
    matchLabels:
      app: accounts-service
  template:
    metadata:
      labels:
        app: accounts-service
    spec:
      containers:
      - name: accounts-service
        image: pincex/accounts-service:v1.0.0
        ports:
        - containerPort: 8080
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: database-credentials
              key: url
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: redis-credentials
              key: url
        resources:
          requests:
            cpu: "2"
            memory: "4Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

## System Monitoring

### Prometheus Metrics Configuration
```yaml
# prometheus-config.yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "accounts_alerts.yml"

scrape_configs:
  - job_name: 'accounts-service'
    static_configs:
      - targets: ['accounts-service:8080']
    metrics_path: /metrics
    scrape_interval: 5s
    
  - job_name: 'postgresql'
    static_configs:
      - targets: ['postgres-exporter:9187']
    scrape_interval: 15s
    
  - job_name: 'redis'
    static_configs:
      - targets: ['redis-exporter:9121']
    scrape_interval: 15s

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093
```

### Critical Alerts Configuration
```yaml
# accounts_alerts.yml
groups:
- name: accounts.critical
  rules:
  - alert: HighErrorRate
    expr: rate(accounts_operation_errors_total[5m]) > 10
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "High error rate in accounts service"
      description: "Error rate is {{ $value }} errors/sec"
      
  - alert: HighLatency
    expr: histogram_quantile(0.95, accounts_operation_duration_seconds_bucket) > 0.5
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High latency in accounts operations"
      description: "95th percentile latency is {{ $value }}s"
      
  - alert: DatabaseConnectionPoolExhausted
    expr: accounts_database_connections_active / accounts_database_connections_max > 0.9
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Database connection pool nearly exhausted"
      description: "{{ $value | humanizePercentage }} of connections in use"
      
  - alert: RedisMemoryHigh
    expr: redis_memory_used_bytes / redis_memory_max_bytes > 0.9
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Redis memory usage high"
      description: "Redis memory usage is {{ $value | humanizePercentage }}"
      
  - alert: ReplicationLag
    expr: pg_replication_lag_seconds > 30
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "PostgreSQL replication lag high"
      description: "Replication lag is {{ $value }}s"
```

### Grafana Dashboard Configuration
```json
{
  "dashboard": {
    "id": null,
    "title": "Accounts Service - Ultra High Concurrency",
    "tags": ["accounts", "production"],
    "timezone": "UTC",
    "panels": [
      {
        "id": 1,
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(accounts_operation_total[5m])",
            "legendFormat": "{{operation}} - {{status}}"
          }
        ],
        "yAxes": [
          {
            "label": "Requests/sec",
            "min": 0
          }
        ]
      },
      {
        "id": 2,
        "title": "Response Time Percentiles",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.50, accounts_operation_duration_seconds_bucket)",
            "legendFormat": "P50"
          },
          {
            "expr": "histogram_quantile(0.95, accounts_operation_duration_seconds_bucket)",
            "legendFormat": "P95"
          },
          {
            "expr": "histogram_quantile(0.99, accounts_operation_duration_seconds_bucket)",
            "legendFormat": "P99"
          }
        ]
      },
      {
        "id": 3,
        "title": "Database Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(pg_stat_database_tup_fetched[5m])",
            "legendFormat": "Rows Fetched/sec"
          },
          {
            "expr": "rate(pg_stat_database_tup_inserted[5m])",
            "legendFormat": "Rows Inserted/sec"
          },
          {
            "expr": "rate(pg_stat_database_tup_updated[5m])",
            "legendFormat": "Rows Updated/sec"
          }
        ]
      }
    ]
  }
}
```

## Performance Tuning

### PostgreSQL Optimization
```sql
-- postgresql.conf optimizations for ultra-high concurrency

-- Memory configuration
shared_buffers = '16GB'                    -- 25% of total RAM
effective_cache_size = '48GB'              -- 75% of total RAM
work_mem = '64MB'                          -- Per-connection work memory
maintenance_work_mem = '2GB'               -- Maintenance operations
wal_buffers = '64MB'                       -- WAL buffer size

-- Connection and concurrency
max_connections = 1000                     -- Maximum connections
max_worker_processes = 16                  -- Number of worker processes
max_parallel_workers = 16                  -- Maximum parallel workers
max_parallel_workers_per_gather = 4       -- Workers per gather node

-- WAL and checkpoints
wal_level = 'replica'                      -- WAL level for replication
checkpoint_completion_target = 0.9         -- Checkpoint completion target
max_wal_size = '4GB'                       -- Maximum WAL size
min_wal_size = '1GB'                       -- Minimum WAL size
wal_compression = on                       -- Enable WAL compression

-- Query optimization
random_page_cost = 1.1                     -- SSD random page cost
effective_io_concurrency = 300             -- Expected concurrent I/O operations
default_statistics_target = 1000           -- Statistics target

-- Logging for monitoring
log_min_duration_statement = 1000          -- Log slow queries (1s+)
log_checkpoints = on                       -- Log checkpoint activity
log_connections = on                       -- Log connections
log_disconnections = on                    -- Log disconnections
log_temp_files = 0                         -- Log temp files
```

### Redis Configuration Optimization
```conf
# redis.conf optimizations for ultra-high concurrency

# Memory management
maxmemory 28gb                             # Leave 4GB for OS
maxmemory-policy allkeys-lru               # Eviction policy
maxmemory-samples 10                       # LRU sample size

# Network optimization
tcp-backlog 65535                          # TCP listen backlog
tcp-keepalive 60                           # TCP keepalive
timeout 300                                # Client timeout

# Performance tuning
hz 10                                      # Background task frequency
rdb-save-incremental-fsync yes             # Incremental fsync
aof-rewrite-incremental-fsync yes          # AOF incremental fsync

# Client output buffer limits
client-output-buffer-limit normal 0 0 0
client-output-buffer-limit replica 256mb 64mb 60
client-output-buffer-limit pubsub 32mb 8mb 60

# Lazy freeing for better performance
lazyfree-lazy-eviction yes
lazyfree-lazy-expire yes
lazyfree-lazy-server-del yes
replica-lazy-flush yes
```

### Application Tuning
```go
// Connection pool optimization
func OptimizeConnectionPools(config *DatabaseConfig) {
    // PostgreSQL connection pool
    config.PostgreSQL.MaxOpenConns = 200      // Per instance
    config.PostgreSQL.MaxIdleConns = 50       // Idle connections
    config.PostgreSQL.ConnMaxLifetime = 1 * time.Hour
    config.PostgreSQL.ConnMaxIdleTime = 10 * time.Minute
    
    // Redis connection pool
    config.Redis.PoolSize = 100               // Per instance
    config.Redis.MinIdleConns = 10            // Minimum idle
    config.Redis.PoolTimeout = 5 * time.Second
    config.Redis.IdleTimeout = 5 * time.Minute
}

// Batch operation optimization
func OptimizeBatchOperations() {
    // Use prepared statements
    PreparedStatements.BalanceUpdate = db.Prepare(`
        UPDATE accounts 
        SET available = $1, reserved = $2, total = $3, version = version + 1 
        WHERE user_id = $4 AND currency = $5 AND version = $6
    `)
    
    // Redis pipeline optimization
    PipelineSize = 100                         // Batch size for Redis operations
    PipelineTimeout = 5 * time.Millisecond     // Pipeline timeout
}
```

## Disaster Recovery

### Backup Strategy
```bash
#!/bin/bash
# Automated backup script

BACKUP_DIR="/backup/$(date +%Y%m%d)"
mkdir -p $BACKUP_DIR

# PostgreSQL backup
pg_dump -h $POSTGRES_PRIMARY -U $POSTGRES_USER -d accounts_prod \
  --compress=9 --format=custom \
  --file=$BACKUP_DIR/accounts_$(date +%Y%m%d_%H%M%S).dump

# WAL archiving
rsync -av $POSTGRES_DATA_DIR/pg_wal/ $BACKUP_DIR/wal_archive/

# Redis backup
redis-cli --rdb $BACKUP_DIR/redis_$(date +%Y%m%d_%H%M%S).rdb

# Upload to S3
aws s3 sync $BACKUP_DIR s3://pincex-backups/accounts/$(date +%Y%m%d)/

# Cleanup old local backups (keep 7 days)
find /backup -type d -mtime +7 -exec rm -rf {} \;
```

### Recovery Procedures
```bash
#!/bin/bash
# Disaster recovery script

echo "Starting disaster recovery..."

# 1. Stop all services
kubectl scale deployment accounts-service --replicas=0

# 2. Restore PostgreSQL from backup
BACKUP_FILE="accounts_20250128_100000.dump"
pg_restore -h $POSTGRES_PRIMARY -U $POSTGRES_USER -d accounts_prod \
  --clean --if-exists --verbose $BACKUP_FILE

# 3. Restore Redis from backup
redis-cli --rdb redis_20250128_100000.rdb

# 4. Verify data integrity
psql -h $POSTGRES_PRIMARY -c "SELECT COUNT(*) FROM accounts;"
redis-cli dbsize

# 5. Restart services
kubectl scale deployment accounts-service --replicas=6

echo "Disaster recovery completed"
```

### RTO/RPO Targets
```yaml
# Recovery objectives
recovery_objectives:
  rto: "4 hours"           # Recovery Time Objective
  rpo: "15 minutes"        # Recovery Point Objective
  
backup_frequency:
  full_backup: "daily"
  incremental: "hourly"
  wal_archive: "continuous"
  
testing:
  frequency: "monthly"
  last_test: "2025-01-15"
  next_test: "2025-02-15"
```

## Maintenance Procedures

### Regular Maintenance Tasks
```bash
#!/bin/bash
# Weekly maintenance script

echo "Starting weekly maintenance..."

# 1. Database maintenance
psql -c "VACUUM ANALYZE;"
psql -c "REINDEX DATABASE accounts_prod;"

# 2. Update statistics
psql -c "ANALYZE;"

# 3. Clean up old partitions
psql -f scripts/cleanup_old_partitions.sql

# 4. Redis maintenance
redis-cli bgrewriteaof
redis-cli bgsave

# 5. Clear old logs
find /var/log -name "*.log" -mtime +30 -delete

echo "Weekly maintenance completed"
```

### Index Maintenance
```sql
-- Monitor index usage
SELECT 
    indexrelname,
    idx_tup_read,
    idx_tup_fetch,
    idx_scan
FROM pg_stat_user_indexes 
ORDER BY idx_scan DESC;

-- Rebuild unused indexes
REINDEX INDEX CONCURRENTLY idx_accounts_user_currency;

-- Add missing indexes based on query patterns
CREATE INDEX CONCURRENTLY idx_accounts_recent_activity 
ON accounts (updated_at) 
WHERE updated_at > NOW() - INTERVAL '7 days';
```

### Capacity Management
```sql
-- Monitor table sizes
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables 
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;

-- Monitor partition sizes
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables 
WHERE tablename LIKE 'accounts_p%'
ORDER BY tablename;
```

## Security Operations

### Access Control Management
```bash
#!/bin/bash
# User access management

# Create application user with limited permissions
psql -c "CREATE USER accounts_app WITH PASSWORD 'secure_password';"
psql -c "GRANT CONNECT ON DATABASE accounts_prod TO accounts_app;"
psql -c "GRANT USAGE ON SCHEMA public TO accounts_app;"
psql -c "GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO accounts_app;"

# Create read-only user for analytics
psql -c "CREATE USER analytics_read WITH PASSWORD 'read_password';"
psql -c "GRANT CONNECT ON DATABASE accounts_prod TO analytics_read;"
psql -c "GRANT USAGE ON SCHEMA public TO analytics_read;"
psql -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO analytics_read;"
```

### Security Monitoring
```yaml
# Security alerts
groups:
- name: security.alerts
  rules:
  - alert: SuspiciousLoginActivity
    expr: rate(accounts_login_failures_total[5m]) > 10
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High login failure rate detected"
      
  - alert: UnauthorizedDatabaseAccess
    expr: pg_stat_database_numbackends{datname="accounts_prod"} > 50
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Unusual number of database connections"
      
  - alert: SuspiciousRedisActivity
    expr: rate(redis_commands_total{cmd!~"get|set|exists|ttl"}[5m]) > 100
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "Unusual Redis command pattern detected"
```

### Audit Logging
```go
// Audit logging implementation
func LogSecurityEvent(ctx context.Context, event SecurityEvent) {
    auditLog := AuditLog{
        Timestamp:   time.Now(),
        UserID:      getUserID(ctx),
        Action:      event.Action,
        Resource:    event.Resource,
        IPAddress:   getClientIP(ctx),
        UserAgent:   getUserAgent(ctx),
        Success:     event.Success,
        Details:     event.Details,
    }
    
    // Log to secure audit trail
    secureLogger.Info("security_event", 
        zap.Any("audit", auditLog),
        zap.String("trace_id", getTraceID(ctx)),
    )
    
    // Send to SIEM system
    siemClient.SendEvent(auditLog)
}
```

## Capacity Planning

### Growth Projections
```yaml
# Capacity planning metrics
current_metrics:
  daily_transactions: 10_000_000
  peak_rps: 85_000
  database_size: "500GB"
  redis_memory: "20GB"
  active_users: 50_000

growth_projections:
  6_months:
    daily_transactions: 25_000_000
    peak_rps: 150_000
    database_size: "1.2TB"
    redis_memory: "40GB"
    active_users: 100_000
    
  12_months:
    daily_transactions: 50_000_000
    peak_rps: 250_000
    database_size: "2.5TB"
    redis_memory: "80GB"
    active_users: 200_000
```

### Scaling Triggers
```yaml
# Automated scaling triggers
scaling_rules:
  database:
    cpu_threshold: 70
    memory_threshold: 80
    connection_threshold: 80
    action: "add_read_replica"
    
  redis:
    memory_threshold: 85
    cpu_threshold: 75
    action: "add_cluster_node"
    
  application:
    cpu_threshold: 70
    memory_threshold: 80
    response_time_threshold: "500ms"
    action: "scale_horizontally"
```

## Health Checks

### Application Health Checks
```go
// Health check endpoint implementation
func HealthCheck(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    
    health := HealthStatus{
        Timestamp: time.Now(),
        Status:    "healthy",
        Checks:    make(map[string]CheckResult),
    }
    
    // Database connectivity check
    if err := checkDatabase(ctx); err != nil {
        health.Checks["database"] = CheckResult{
            Status: "unhealthy",
            Error:  err.Error(),
        }
        health.Status = "unhealthy"
    } else {
        health.Checks["database"] = CheckResult{Status: "healthy"}
    }
    
    // Redis connectivity check
    if err := checkRedis(ctx); err != nil {
        health.Checks["redis"] = CheckResult{
            Status: "unhealthy",
            Error:  err.Error(),
        }
        health.Status = "unhealthy"
    } else {
        health.Checks["redis"] = CheckResult{Status: "healthy"}
    }
    
    // Performance check
    if latency := checkPerformance(ctx); latency > 100*time.Millisecond {
        health.Checks["performance"] = CheckResult{
            Status: "degraded",
            Details: fmt.Sprintf("High latency: %v", latency),
        }
        if health.Status == "healthy" {
            health.Status = "degraded"
        }
    } else {
        health.Checks["performance"] = CheckResult{Status: "healthy"}
    }
    
    // Set HTTP status code
    if health.Status == "unhealthy" {
        w.WriteHeader(http.StatusServiceUnavailable)
    } else if health.Status == "degraded" {
        w.WriteHeader(http.StatusOK) // Still accepting traffic
    } else {
        w.WriteHeader(http.StatusOK)
    }
    
    json.NewEncoder(w).Encode(health)
}
```

### Deep Health Checks
```go
// Deep health check for detailed system status
func DeepHealthCheck(ctx context.Context) (*DetailedHealth, error) {
    health := &DetailedHealth{
        Timestamp: time.Now(),
        Database:  checkDatabaseHealth(ctx),
        Redis:     checkRedisHealth(ctx),
        Cache:     checkCacheHealth(ctx),
        Metrics:   collectHealthMetrics(ctx),
    }
    
    return health, nil
}

func checkDatabaseHealth(ctx context.Context) DatabaseHealth {
    // Check connection pool
    stats := db.Stats()
    
    // Check replication lag
    var lag float64
    db.QueryRow("SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))").Scan(&lag)
    
    return DatabaseHealth{
        ConnectionsActive: stats.OpenConnections,
        ConnectionsMax:    stats.MaxOpenConnections,
        ReplicationLag:    time.Duration(lag) * time.Second,
        SlowQueries:       getSlowQueryCount(ctx),
    }
}
```

## Operational Runbooks

### High CPU Usage
```bash
#!/bin/bash
# High CPU usage runbook

echo "Investigating high CPU usage..."

# 1. Identify top CPU consuming processes
top -p $(pgrep postgres) -n 1

# 2. Check for long-running queries
psql -c "
SELECT 
    pid,
    now() - pg_stat_activity.query_start AS duration,
    query 
FROM pg_stat_activity 
WHERE state = 'active' 
ORDER BY duration DESC 
LIMIT 10;
"

# 3. Check for locks
psql -c "
SELECT 
    blocked_locks.pid AS blocked_pid,
    blocked_activity.usename AS blocked_user,
    blocking_locks.pid AS blocking_pid,
    blocking_activity.usename AS blocking_user,
    blocked_activity.query AS blocked_statement,
    blocking_activity.query AS current_statement_in_blocking_process
FROM pg_catalog.pg_locks blocked_locks
JOIN pg_catalog.pg_stat_activity blocked_activity ON blocked_activity.pid = blocked_locks.pid
JOIN pg_catalog.pg_locks blocking_locks 
    ON blocking_locks.locktype = blocked_locks.locktype
    AND blocking_locks.DATABASE IS NOT DISTINCT FROM blocked_locks.DATABASE
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
WHERE NOT blocked_locks.GRANTED;
"

# 4. Scale if necessary
if [ "$CPU_USAGE" -gt 80 ]; then
    kubectl scale deployment accounts-service --replicas=$((CURRENT_REPLICAS + 2))
fi
```

### Memory Issues
```bash
#!/bin/bash
# Memory issues runbook

echo "Investigating memory issues..."

# 1. Check overall memory usage
free -h

# 2. Check PostgreSQL memory usage
psql -c "
SELECT 
    setting,
    unit,
    context
FROM pg_settings 
WHERE name IN ('shared_buffers', 'work_mem', 'maintenance_work_mem', 'effective_cache_size');
"

# 3. Check Redis memory usage
redis-cli info memory

# 4. Identify memory-heavy queries
psql -c "
SELECT 
    query,
    mean_time,
    calls,
    total_time,
    mean_time * calls as total_cpu_time
FROM pg_stat_statements 
ORDER BY mean_time DESC 
LIMIT 10;
"
```

### Database Connection Pool Exhaustion
```bash
#!/bin/bash
# Connection pool exhaustion runbook

echo "Investigating connection pool exhaustion..."

# 1. Check current connections
psql -c "
SELECT 
    datname,
    usename,
    client_addr,
    state,
    COUNT(*)
FROM pg_stat_activity 
GROUP BY datname, usename, client_addr, state
ORDER BY COUNT(*) DESC;
"

# 2. Kill idle connections
psql -c "
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity 
WHERE state = 'idle' 
AND now() - state_change > interval '5 minutes';
"

# 3. Increase connection pool temporarily
kubectl patch deployment accounts-service -p '{"spec":{"template":{"spec":{"containers":[{"name":"accounts-service","env":[{"name":"MAX_DB_CONNECTIONS","value":"300"}]}]}}}}'
```

This operational guide provides comprehensive procedures for maintaining the ultra-high concurrency database layer in production, ensuring system reliability, performance, and operational excellence.
