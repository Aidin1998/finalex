# Database Migration and Scaling Operations Guide

## Overview

This guide provides comprehensive instructions for migrating, scaling, and maintaining the ultra-high concurrency database layer for the Accounts module. It covers both PostgreSQL and Redis operations for production environments supporting 100,000+ requests per second.

## Table of Contents
1. [Pre-Migration Planning](#pre-migration-planning)
2. [PostgreSQL Migration Procedures](#postgresql-migration-procedures)
3. [Redis Migration and Scaling](#redis-migration-and-scaling)
4. [Partition Management](#partition-management)
5. [Zero-Downtime Migrations](#zero-downtime-migrations)
6. [Data Validation and Testing](#data-validation-and-testing)
7. [Rollback Procedures](#rollback-procedures)
8. [Monitoring and Alerting](#monitoring-and-alerting)
9. [Troubleshooting](#troubleshooting)

## Pre-Migration Planning

### Migration Checklist
```yaml
# Pre-migration checklist
- [ ] Review current system performance metrics
- [ ] Identify peak traffic patterns and maintenance windows
- [ ] Prepare rollback scripts and procedures
- [ ] Set up monitoring and alerting for migration
- [ ] Test migration procedures in staging environment
- [ ] Coordinate with operations and development teams
- [ ] Prepare communication plan for stakeholders
- [ ] Schedule post-migration validation tests
```

### Environment Preparation
```bash
# 1. Backup current databases
pg_dump -h $POSTGRES_HOST -U $POSTGRES_USER -d accounts_prod > accounts_backup_$(date +%Y%m%d_%H%M%S).sql

# 2. Create Redis backup
redis-cli --rdb accounts_redis_backup_$(date +%Y%m%d_%H%M%S).rdb

# 3. Verify backup integrity
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d test_restore_db < accounts_backup_$(date +%Y%m%d_%H%M%S).sql
```

### Performance Baseline
```bash
# Establish performance baseline
go test -tags=benchmark -bench=BenchmarkBalanceOperations -count=3 -benchmem
go test -tags=benchmark -bench=BenchmarkConcurrentOperations -count=3 -benchmem

# Monitor key metrics
psql -c "SELECT * FROM pg_stat_database WHERE datname = 'accounts_prod';"
redis-cli info stats
```

## PostgreSQL Migration Procedures

### Schema Migrations

#### Running Migrations
```bash
# 1. Check current migration status
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d accounts_prod -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 5;"

# 2. Run migrations in transaction
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d accounts_prod << EOF
BEGIN;
\i migrations/postgres/003_create_accounts_table.up.sql
\i migrations/postgres/004_create_audit_tables.up.sql
COMMIT;
EOF

# 3. Verify migration success
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d accounts_prod -c "
SELECT schemaname, tablename, indexname 
FROM pg_indexes 
WHERE tablename LIKE '%accounts%' 
ORDER BY tablename, indexname;
"
```

#### Partition Creation
```sql
-- Create new monthly partition
CREATE TABLE accounts_p2025_02 PARTITION OF accounts 
FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

-- Create indexes on new partition
CREATE INDEX CONCURRENTLY idx_accounts_p2025_02_user_currency 
ON accounts_p2025_02 (user_id, currency);

CREATE INDEX CONCURRENTLY idx_accounts_p2025_02_created 
ON accounts_p2025_02 (created_at);

-- Verify partition
SELECT 
    schemaname, 
    tablename, 
    attname, 
    n_distinct, 
    correlation 
FROM pg_stats 
WHERE tablename = 'accounts_p2025_02';
```

#### Index Management
```sql
-- Create indexes concurrently to avoid blocking
CREATE INDEX CONCURRENTLY idx_accounts_optimized_balance 
ON accounts (user_id, currency, updated_at) 
WHERE status = 'active';

-- Monitor index creation progress
SELECT 
    pid,
    now() - pg_stat_activity.query_start AS duration,
    query 
FROM pg_stat_activity 
WHERE query LIKE '%CREATE INDEX%';

-- Drop old indexes after verification
DROP INDEX CONCURRENTLY idx_accounts_old_index;
```

### Connection Pool Scaling
```go
// Update connection pool configuration
func UpdateConnectionPool(config *DatabaseConfig) error {
    // Scale connection pool based on load
    newMaxConns := calculateOptimalConnections()
    
    // Gradually increase connections
    for i := config.PostgreSQL.MaxConnections; i < newMaxConns; i += 10 {
        config.PostgreSQL.MaxConnections = i
        time.Sleep(30 * time.Second) // Allow system to adapt
        
        // Monitor connection usage
        if err := validateConnectionUsage(); err != nil {
            return fmt.Errorf("connection pool scaling failed: %w", err)
        }
    }
    
    return nil
}

func calculateOptimalConnections() int {
    // Formula: (CPU cores * 2) + effective_spindle_count + number_of_read_replicas
    cpuCores := runtime.NumCPU()
    readReplicas := 3
    return (cpuCores * 2) + 1 + readReplicas
}
```

## Redis Migration and Scaling

### Cluster Scaling

#### Adding Nodes
```bash
# 1. Start new Redis nodes
redis-server /etc/redis/redis-7001.conf &
redis-server /etc/redis/redis-7002.conf &

# 2. Add nodes to cluster
redis-cli --cluster add-node 127.0.0.1:7001 127.0.0.1:6379
redis-cli --cluster add-node 127.0.0.1:7002 127.0.0.1:6379 --cluster-slave

# 3. Rebalance slots
redis-cli --cluster rebalance 127.0.0.1:6379 --cluster-use-empty-masters

# 4. Verify cluster status
redis-cli cluster nodes
redis-cli cluster info
```

#### Slot Migration
```bash
# 1. Check current slot distribution
redis-cli cluster nodes | grep master

# 2. Migrate slots to new node
redis-cli --cluster reshard 127.0.0.1:6379 \
  --cluster-from <source-node-id> \
  --cluster-to <target-node-id> \
  --cluster-slots 1000 \
  --cluster-yes

# 3. Monitor migration progress
redis-cli cluster nodes | grep migrating
```

### Data Migration

#### Hot Migration Script
```go
// Hot migration between Redis instances
func MigrateRedisData(source, target *redis.Client, pattern string) error {
    ctx := context.Background()
    
    // Use SCAN to iterate through keys
    iter := source.Scan(ctx, 0, pattern, 1000).Iterator()
    
    pipeline := target.Pipeline()
    batchSize := 0
    
    for iter.Next(ctx) {
        key := iter.Val()
        
        // Get TTL
        ttl := source.TTL(ctx, key).Val()
        
        // Dump key
        data := source.Dump(ctx, key).Val()
        
        // Restore to target
        pipeline.RestoreReplace(ctx, key, ttl, data)
        
        batchSize++
        if batchSize >= 100 {
            if _, err := pipeline.Exec(ctx); err != nil {
                return fmt.Errorf("pipeline execution failed: %w", err)
            }
            pipeline = target.Pipeline()
            batchSize = 0
            
            // Rate limiting to avoid overwhelming the system
            time.Sleep(10 * time.Millisecond)
        }
    }
    
    // Execute remaining batch
    if batchSize > 0 {
        _, err := pipeline.Exec(ctx)
        return err
    }
    
    return iter.Err()
}
```

#### Memory Optimization During Migration
```redis
# 1. Enable lazy freeing
CONFIG SET lazyfree-lazy-eviction yes
CONFIG SET lazyfree-lazy-expire yes
CONFIG SET lazyfree-lazy-server-del yes

# 2. Adjust memory policy temporarily
CONFIG SET maxmemory-policy allkeys-lru

# 3. Monitor memory usage
INFO memory
MEMORY USAGE <key>
```

## Partition Management

### Automatic Partition Creation
```go
// Automated partition creation job
func CreateMonthlyPartitions(db *gorm.DB) error {
    nextMonth := time.Now().AddDate(0, 1, 0)
    partitionName := fmt.Sprintf("accounts_p%s", nextMonth.Format("2006_01"))
    
    // Check if partition already exists
    var count int64
    err := db.Raw(`
        SELECT COUNT(*) 
        FROM information_schema.tables 
        WHERE table_name = ?
    `, partitionName).Scan(&count).Error
    
    if err != nil {
        return err
    }
    
    if count > 0 {
        return nil // Partition already exists
    }
    
    // Create partition
    createSQL := fmt.Sprintf(`
        CREATE TABLE %s PARTITION OF accounts 
        FOR VALUES FROM ('%s') TO ('%s');
        
        CREATE INDEX CONCURRENTLY idx_%s_user_currency 
        ON %s (user_id, currency);
        
        CREATE INDEX CONCURRENTLY idx_%s_created 
        ON %s (created_at);
    `, 
        partitionName,
        nextMonth.Format("2006-01-02"),
        nextMonth.AddDate(0, 1, 0).Format("2006-01-02"),
        partitionName, partitionName,
        partitionName, partitionName,
    )
    
    return db.Exec(createSQL).Error
}
```

### Partition Pruning
```sql
-- Drop old partitions (older than 2 years)
DO $$
DECLARE
    partition_name text;
    cutoff_date date := CURRENT_DATE - INTERVAL '2 years';
BEGIN
    FOR partition_name IN 
        SELECT tablename 
        FROM pg_tables 
        WHERE tablename LIKE 'accounts_p%' 
        AND tablename < 'accounts_p' || to_char(cutoff_date, 'YYYY_MM')
    LOOP
        -- Backup partition data first
        EXECUTE format('CREATE TABLE %s_backup AS SELECT * FROM %s', partition_name, partition_name);
        
        -- Drop partition
        EXECUTE format('DROP TABLE %s', partition_name);
        
        RAISE NOTICE 'Dropped partition: %', partition_name;
    END LOOP;
END $$;
```

## Zero-Downtime Migrations

### Blue-Green Deployment Strategy
```yaml
# Blue-Green deployment configuration
blue_environment:
  database:
    host: "postgres-blue.internal"
    port: 5432
    
  redis:
    cluster: ["redis-blue-1:6379", "redis-blue-2:6379", "redis-blue-3:6379"]
    
green_environment:
  database:
    host: "postgres-green.internal" 
    port: 5432
    
  redis:
    cluster: ["redis-green-1:6379", "redis-green-2:6379", "redis-green-3:6379"]
```

### Read Replica Promotion
```bash
# 1. Stop writes to primary
psql -h $PRIMARY_HOST -c "SELECT pg_promote();"

# 2. Wait for replica to catch up
psql -h $REPLICA_HOST -c "SELECT pg_last_wal_receive_lsn() = pg_last_wal_replay_lsn();"

# 3. Promote replica to primary
psql -h $REPLICA_HOST -c "SELECT pg_promote();"

# 4. Update application configuration
kubectl patch configmap app-config --patch '{"data":{"database_host":"new-primary-host"}}'

# 5. Restart application pods
kubectl rollout restart deployment accounts-service
```

### Rolling Migration Process
```go
func RollingMigration(ctx context.Context, config *MigrationConfig) error {
    // 1. Create new schema version
    if err := createNewSchemaVersion(ctx, config); err != nil {
        return fmt.Errorf("failed to create new schema: %w", err)
    }
    
    // 2. Start dual-write to both schemas
    if err := enableDualWrite(ctx, config); err != nil {
        return fmt.Errorf("failed to enable dual write: %w", err)
    }
    
    // 3. Migrate existing data in batches
    if err := migrateDataInBatches(ctx, config); err != nil {
        return fmt.Errorf("data migration failed: %w", err)
    }
    
    // 4. Validate data consistency
    if err := validateDataConsistency(ctx, config); err != nil {
        return fmt.Errorf("data validation failed: %w", err)
    }
    
    // 5. Switch reads to new schema
    if err := switchReadsToNewSchema(ctx, config); err != nil {
        return fmt.Errorf("failed to switch reads: %w", err)
    }
    
    // 6. Stop dual-write, use new schema only
    if err := disableDualWrite(ctx, config); err != nil {
        return fmt.Errorf("failed to disable dual write: %w", err)
    }
    
    // 7. Drop old schema
    if err := dropOldSchema(ctx, config); err != nil {
        return fmt.Errorf("failed to drop old schema: %w", err)
    }
    
    return nil
}
```

## Data Validation and Testing

### Consistency Checks
```sql
-- Account balance consistency check
WITH account_totals AS (
    SELECT 
        user_id,
        currency,
        SUM(CASE WHEN type = 'credit' THEN amount ELSE -amount END) as calculated_balance
    FROM transaction_journal 
    WHERE status = 'completed'
    GROUP BY user_id, currency
),
account_balances AS (
    SELECT 
        user_id,
        currency,
        available + reserved as current_balance
    FROM accounts
)
SELECT 
    ab.user_id,
    ab.currency,
    ab.current_balance,
    at.calculated_balance,
    ab.current_balance - at.calculated_balance as difference
FROM account_balances ab
FULL OUTER JOIN account_totals at 
    ON ab.user_id = at.user_id 
    AND ab.currency = at.currency
WHERE ABS(ab.current_balance - at.calculated_balance) > 0.00000001;
```

### Load Testing During Migration
```bash
# Run load test during migration
k6 run --vus 1000 --duration 10m test/k6/migration-load-test.js

# Monitor system metrics
prometheus_query 'rate(accounts_balance_operations_total[5m])'
prometheus_query 'histogram_quantile(0.95, accounts_operation_duration_seconds_bucket)'
```

### Automated Validation Scripts
```go
func ValidateMigration(ctx context.Context, db *gorm.DB, redis *redis.Client) error {
    // 1. Validate record counts
    if err := validateRecordCounts(ctx, db); err != nil {
        return fmt.Errorf("record count validation failed: %w", err)
    }
    
    // 2. Validate data integrity
    if err := validateDataIntegrity(ctx, db); err != nil {
        return fmt.Errorf("data integrity validation failed: %w", err)
    }
    
    // 3. Validate cache consistency
    if err := validateCacheConsistency(ctx, db, redis); err != nil {
        return fmt.Errorf("cache consistency validation failed: %w", err)
    }
    
    // 4. Validate performance
    if err := validatePerformance(ctx, db, redis); err != nil {
        return fmt.Errorf("performance validation failed: %w", err)
    }
    
    return nil
}
```

## Rollback Procedures

### Database Rollback
```bash
# 1. Stop application traffic
kubectl scale deployment accounts-service --replicas=0

# 2. Restore from backup
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d accounts_prod < accounts_backup_$(date +%Y%m%d_%H%M%S).sql

# 3. Restore Redis data
redis-cli --rdb accounts_redis_backup_$(date +%Y%m%d_%H%M%S).rdb

# 4. Revert schema changes
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d accounts_prod << EOF
BEGIN;
\i migrations/postgres/004_create_audit_tables.down.sql
\i migrations/postgres/003_create_accounts_table.down.sql
COMMIT;
EOF

# 5. Restart application
kubectl scale deployment accounts-service --replicas=3
```

### Point-in-Time Recovery
```bash
# PostgreSQL point-in-time recovery
pg_ctl stop -D /var/lib/postgresql/data
cp -R /backup/base_backup/* /var/lib/postgresql/data/
echo "restore_command = 'cp /backup/wal_archive/%f %p'" >> /var/lib/postgresql/data/recovery.conf
echo "recovery_target_time = '2025-01-28 10:30:00'" >> /var/lib/postgresql/data/recovery.conf
pg_ctl start -D /var/lib/postgresql/data
```

### Automated Rollback Triggers
```go
func MonitorMigrationHealth(ctx context.Context, config *MigrationConfig) error {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            // Check error rates
            errorRate := getErrorRate(ctx)
            if errorRate > config.MaxErrorRate {
                return triggerRollback(ctx, "high error rate detected")
            }
            
            // Check response times
            responseTime := getAverageResponseTime(ctx)
            if responseTime > config.MaxResponseTime {
                return triggerRollback(ctx, "high response time detected")
            }
            
            // Check data consistency
            if !isDataConsistent(ctx) {
                return triggerRollback(ctx, "data inconsistency detected")
            }
        }
    }
}
```

## Monitoring and Alerting

### Migration Metrics
```yaml
# Prometheus alerts for migrations
groups:
  - name: migration.alerts
    rules:
      - alert: MigrationInProgress
        expr: migration_status == 1
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Database migration in progress"
          
      - alert: MigrationSlowProgress
        expr: rate(migration_records_processed[5m]) < 1000
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Migration progress is slower than expected"
          
      - alert: MigrationHighErrorRate
        expr: rate(migration_errors_total[5m]) > 10
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "High error rate during migration"
```

### Real-time Monitoring Dashboard
```json
{
  "dashboard": {
    "title": "Database Migration Dashboard",
    "panels": [
      {
        "title": "Migration Progress",
        "type": "stat",
        "targets": [
          {
            "expr": "migration_progress_percentage",
            "legendFormat": "Progress %"
          }
        ]
      },
      {
        "title": "Records Processed Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(migration_records_processed[5m])",
            "legendFormat": "Records/sec"
          }
        ]
      },
      {
        "title": "Error Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(migration_errors_total[5m])",
            "legendFormat": "Errors/sec"
          }
        ]
      }
    ]
  }
}
```

## Troubleshooting

### Common Migration Issues

#### Performance Degradation
```sql
-- Check for blocking queries
SELECT 
    pid,
    now() - pg_stat_activity.query_start AS duration,
    query,
    state,
    waiting
FROM pg_stat_activity 
WHERE state = 'active'
ORDER BY duration DESC;

-- Check lock contention
SELECT 
    l.locktype,
    l.database,
    l.relation,
    l.page,
    l.tuple,
    l.transactionid,
    l.classid,
    l.objid,
    l.objsubid,
    l.pid,
    a.query
FROM pg_locks l
JOIN pg_stat_activity a ON l.pid = a.pid
WHERE NOT l.granted;
```

#### Memory Issues
```bash
# Monitor Redis memory usage
redis-cli info memory

# Check for memory leaks in PostgreSQL
psql -c "SELECT * FROM pg_stat_activity WHERE state = 'active' ORDER BY backend_start;"

# Monitor system memory
free -h
vmstat 1 10
```

#### Replication Lag
```sql
-- Check replication lag
SELECT 
    client_addr,
    client_hostname,
    state,
    sent_lsn,
    write_lsn,
    flush_lsn,
    replay_lsn,
    write_lag,
    flush_lag,
    replay_lag
FROM pg_stat_replication;
```

### Recovery Procedures

#### Failed Migration Recovery
```bash
#!/bin/bash
# Migration recovery script

echo "Starting migration recovery..."

# 1. Stop all writes
kubectl patch deployment accounts-service -p '{"spec":{"replicas":0}}'

# 2. Identify the failure point
LAST_SUCCESSFUL_MIGRATION=$(psql -t -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;")
echo "Last successful migration: $LAST_SUCCESSFUL_MIGRATION"

# 3. Rollback to last known good state
psql -f migrations/rollback_to_${LAST_SUCCESSFUL_MIGRATION}.sql

# 4. Verify data integrity
psql -f scripts/verify_data_integrity.sql

# 5. Clear Redis cache
redis-cli flushdb

# 6. Restart services
kubectl patch deployment accounts-service -p '{"spec":{"replicas":3}}'

echo "Recovery completed. Monitor system closely."
```

#### Data Corruption Recovery
```sql
-- Identify corrupted data
SELECT 
    user_id,
    currency,
    available,
    reserved,
    total,
    version
FROM accounts 
WHERE available < 0 
   OR reserved < 0 
   OR total != (available + reserved);

-- Restore from transaction log
WITH corrected_balances AS (
    SELECT 
        user_id,
        currency,
        SUM(CASE WHEN type = 'credit' THEN amount ELSE -amount END) as correct_available,
        0 as correct_reserved
    FROM transaction_journal 
    WHERE status = 'completed'
    GROUP BY user_id, currency
)
UPDATE accounts 
SET 
    available = cb.correct_available,
    reserved = cb.correct_reserved,
    total = cb.correct_available + cb.correct_reserved,
    version = version + 1,
    updated_at = NOW()
FROM corrected_balances cb
WHERE accounts.user_id = cb.user_id 
  AND accounts.currency = cb.currency;
```

## Best Practices

### Migration Planning
1. **Always test in staging first**
2. **Plan for rollback scenarios**
3. **Monitor system metrics during migration**
4. **Use feature flags for gradual rollout**
5. **Coordinate with all stakeholders**

### Performance Optimization
1. **Use connection pooling effectively**
2. **Monitor and optimize query patterns**
3. **Implement proper indexing strategies**
4. **Use read replicas for read-heavy operations**
5. **Cache frequently accessed data**

### Safety Measures
1. **Implement circuit breakers**
2. **Use rate limiting during migrations**
3. **Monitor error rates and latency**
4. **Have automated rollback triggers**
5. **Maintain detailed audit logs**

This migration guide provides a comprehensive framework for safely scaling and migrating the ultra-high concurrency database layer while maintaining system reliability and performance.
