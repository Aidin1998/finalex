# Redis Key Schema Documentation

## Overview

This document describes the Redis key patterns and data structures used in the ultra-high concurrency database layer for the Accounts module. The schema is designed to support 100,000+ requests per second with hot/warm/cold data tiering.

## Key Naming Conventions

### General Patterns
```
<module>:<entity>:<identifier>[:<attribute>][:<qualifier>]

Examples:
- account:balance:uuid:btc
- account:lock:uuid:eth  
- reservation:uuid
- transaction:uuid
```

### Prefixes by Module
- `account:` - Account-related data
- `reservation:` - Balance reservations
- transaction:` - Transaction records
- `audit:` - Audit trail entries
- `partition:` - Partitioning metadata
- `stats:` - Statistical data
- `cache:` - General cache entries
- `lock:` - Distributed locks
- `session:` - User sessions
- `rate_limit:` - Rate limiting data

## Account Module Key Patterns

### Account Balance Keys
```redis
# Primary balance key (hot data)
account:balance:{user_id}:{currency}
Type: Hash
TTL: 5 minutes (hot tier)
Fields:
  - available: decimal (available balance)
  - reserved: decimal (reserved balance)
  - total: decimal (total balance)
  - version: integer (optimistic locking)
  - updated_at: timestamp
  - tier: string (hot/warm/cold)

Example:
account:balance:550e8400-e29b-41d4-a716-446655440000:btc
{
  "available": "1.50000000",
  "reserved": "0.25000000", 
  "total": "1.75000000",
  "version": 42,
  "updated_at": "2025-01-28T10:30:00Z",
  "tier": "hot"
}
```

### Account Lock Keys
```redis
# Distributed locking for atomic operations
account:lock:{user_id}:{currency}
Type: String
TTL: 30 seconds
Value: lock_id (UUID)

Example:
account:lock:550e8400-e29b-41d4-a716-446655440000:btc
Value: "lock_abc123-def456-789ghi"
```

### Account Version Keys
```redis
# Optimistic concurrency control
account:version:{user_id}:{currency}
Type: Integer
TTL: 30 minutes (warm tier)
Value: version_number

Example:
account:version:550e8400-e29b-41d4-a716-446655440000:btc
Value: 42
```

### Account Metadata Keys
```redis
# Extended account information
account:meta:{user_id}:{currency}
Type: Hash
TTL: 30 minutes (warm tier)
Fields:
  - created_at: timestamp
  - last_activity: timestamp
  - transaction_count: integer
  - risk_level: string
  - flags: list

Example:
account:meta:550e8400-e29b-41d4-a716-446655440000:btc
{
  "created_at": "2025-01-01T00:00:00Z",
  "last_activity": "2025-01-28T10:30:00Z",
  "transaction_count": 156,
  "risk_level": "low",
  "flags": ["verified", "active"]
}
```

## Reservation Keys

### Individual Reservations
```redis
# Specific reservation data
reservation:{reservation_id}
Type: Hash
TTL: 2 hours (warm tier)
Fields:
  - user_id: UUID
  - currency: string
  - amount: decimal
  - status: string (active/completed/cancelled)
  - expires_at: timestamp
  - reference_id: string
  - created_at: timestamp

Example:
reservation:res_abc123-def456-789ghi
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "currency": "btc",
  "amount": "0.25000000",
  "status": "active",
  "expires_at": "2025-01-28T12:00:00Z",
  "reference_id": "order_xyz789",
  "created_at": "2025-01-28T10:00:00Z"
}
```

### User Reservation Index
```redis
# Index of reservations by user
reservation:user:{user_id}:{currency}
Type: Sorted Set
TTL: 2 hours (warm tier)
Score: expiration_timestamp
Value: reservation_id

Example:
reservation:user:550e8400-e29b-41d4-a716-446655440000:btc
1738065600 res_abc123-def456-789ghi
1738069200 res_def456-789ghi-abc123
```

## Transaction Keys

### Individual Transactions
```redis
# Transaction journal entries
transaction:{transaction_id}
Type: Hash
TTL: 24 hours (cold tier)
Fields:
  - user_id: UUID
  - currency: string
  - amount: decimal
  - type: string (credit/debit)
  - reference_id: string
  - status: string
  - created_at: timestamp
  - processed_at: timestamp

Example:
transaction:tx_abc123-def456-789ghi
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "currency": "btc",
  "amount": "0.10000000",
  "type": "credit",
  "reference_id": "deposit_xyz789",
  "status": "completed",
  "created_at": "2025-01-28T10:00:00Z",
  "processed_at": "2025-01-28T10:00:15Z"
}
```

### User Transaction Index
```redis
# User transaction history
transaction:user:{user_id}
Type: Sorted Set
TTL: 24 hours (cold tier)
Score: created_timestamp
Value: transaction_id

Example:
transaction:user:550e8400-e29b-41d4-a716-446655440000
1738062000 tx_abc123-def456-789ghi
1738062060 tx_def456-789ghi-abc123
```

## Balance Snapshot Keys

### Daily Snapshots
```redis
# Daily balance snapshots for reconciliation
snapshot:balance:{user_id}:{currency}:{date}
Type: Hash
TTL: 30 days (cold tier)
Fields:
  - opening_balance: decimal
  - closing_balance: decimal
  - total_credits: decimal
  - total_debits: decimal
  - transaction_count: integer
  - snapshot_at: timestamp

Example:
snapshot:balance:550e8400-e29b-41d4-a716-446655440000:btc:2025-01-28
{
  "opening_balance": "1.50000000",
  "closing_balance": "1.75000000",
  "total_credits": "0.35000000",
  "total_debits": "0.10000000",
  "transaction_count": 8,
  "snapshot_at": "2025-01-28T23:59:59Z"
}
```

## Audit and Compliance Keys

### Audit Logs
```redis
# Audit trail entries
audit:{table}:{record_id}
Type: List
TTL: 7 years (cold tier - regulatory requirement)
Value: JSON audit entries (FIFO)

Example:
audit:accounts:550e8400-e29b-41d4-a716-446655440000
[
  {
    "action": "balance_update",
    "old_value": "1.50000000",
    "new_value": "1.75000000",
    "user_id": "admin_uuid",
    "timestamp": "2025-01-28T10:30:00Z",
    "reason": "deposit_processed"
  }
]
```

## Partitioning and Sharding Keys

### User Partition Mapping
```redis
# Maps users to database partitions
partition:user:{user_id}
Type: String
TTL: 24 hours (warm tier)
Value: partition_name

Example:
partition:user:550e8400-e29b-41d4-a716-446655440000
Value: "p03"
```

### Partition Metadata
```redis
# Partition health and statistics
partition:meta:{partition_name}
Type: Hash
TTL: 1 hour (warm tier)
Fields:
  - status: string (active/readonly/migrating)
  - record_count: integer
  - size_bytes: integer
  - last_accessed: timestamp
  - shard_id: string

Example:
partition:meta:p03
{
  "status": "active",
  "record_count": 150000,
  "size_bytes": 75000000,
  "last_accessed": "2025-01-28T10:30:00Z",
  "shard_id": "shard_01"
}
```

## Statistical and Analytics Keys

### Currency Statistics
```redis
# Aggregate statistics by currency
stats:currency:{currency}
Type: Hash
TTL: 1 hour (warm tier)
Fields:
  - total_supply: decimal
  - active_accounts: integer
  - total_volume_24h: decimal
  - avg_balance: decimal
  - updated_at: timestamp

Example:
stats:currency:btc
{
  "total_supply": "1234.56789000",
  "active_accounts": 5420,
  "total_volume_24h": "89.12345678",
  "avg_balance": "0.22756789",
  "updated_at": "2025-01-28T10:30:00Z"
}
```

### Real-time Metrics
```redis
# Performance and operational metrics
metrics:accounts:{metric_name}:{timestamp}
Type: String/Hash
TTL: 1 hour
Value: metric_value

Examples:
metrics:accounts:balance_queries:1738062000 -> "1247"
metrics:accounts:balance_updates:1738062000 -> "423"
metrics:accounts:cache_hit_rate:1738062000 -> "0.94"
```

## Cache Management Keys

### Cache Warming
```redis
# Cache warming job status
cache:warming:{job_id}
Type: Hash
TTL: 1 hour
Fields:
  - status: string (running/completed/failed)
  - started_at: timestamp
  - completed_at: timestamp
  - records_processed: integer
  - errors: integer

Example:
cache:warming:job_abc123
{
  "status": "completed",
  "started_at": "2025-01-28T10:00:00Z",
  "completed_at": "2025-01-28T10:05:30Z",
  "records_processed": 50000,
  "errors": 3
}
```

### Cache Invalidation
```redis
# Cache invalidation patterns
cache:invalidate:{pattern}
Type: Set
TTL: 5 minutes
Value: key_pattern

Example:
cache:invalidate:account:balance:*
cache:invalidate:transaction:user:*
```

## Data Tiering Strategy

### Hot Tier (5 minutes TTL)
- Active account balances
- Current reservations
- Active distributed locks
- Real-time metrics

### Warm Tier (30 minutes - 2 hours TTL)
- Account metadata
- Recent transaction history
- User session data
- Partition metadata

### Cold Tier (24 hours - 30 days TTL)
- Historical transactions
- Daily snapshots
- Audit logs
- Analytics data

## Memory Optimization

### Key Patterns for Compression
```redis
# Use shorter keys for frequently accessed data
bal:{uid}:{cur}     # instead of account:balance:{user_id}:{currency}
res:{rid}           # instead of reservation:{reservation_id}
tx:{tid}            # instead of transaction:{transaction_id}
```

### Hash vs String Trade-offs
- Use Hashes for objects with multiple fields (>3 fields)
- Use Strings for simple key-value pairs
- Use Sorted Sets for time-ordered data
- Use Sets for unique collections

## Monitoring and Alerting

### Key Metrics to Monitor
```redis
# Redis keyspace statistics
INFO keyspace
DBSIZE
MEMORY USAGE <key>
TTL <key>

# Pattern-based monitoring
SCAN 0 MATCH account:balance:* COUNT 1000
SCAN 0 MATCH lock:* COUNT 1000
```

### Alert Thresholds
- Key count growth rate > 10%/hour
- Memory usage > 80%
- Expired key cleanup lag > 5 minutes
- Lock TTL violations

## Security Considerations

### Access Control Lists (ACL)
```redis
# Application user permissions
user accounts_app on >password ~account:* ~reservation:* ~transaction:* +@read +@write +@string +@hash +@list +@set +@stream -@dangerous

# Read-only analytics user
user analytics_read on >password ~stats:* ~metrics:* +@read +info +ping

# Cache management user
user cache_mgr on >password ~cache:* +@read +@write +expire +del
```

### Sensitive Data Handling
- Never store plaintext passwords
- Use hashed user identifiers where possible
- Implement proper key rotation
- Monitor for unauthorized access patterns

## Migration and Scaling

### Key Migration Patterns
```redis
# During partition migrations
MIGRATE host port key destination-db timeout [COPY | REPLACE]
DUMP key
RESTORE key ttl serialized-value [REPLACE]
```

### Scaling Considerations
- Plan for key distribution across cluster nodes
- Monitor hotspot keys and patterns
- Implement consistent hashing for sharding
- Use pipeline operations for bulk operations

## Backup and Recovery

### Backup Key Patterns
```bash
# Export specific patterns
redis-cli --scan --pattern "account:*" | xargs redis-cli DUMP
redis-cli --scan --pattern "reservation:*" | xargs redis-cli DUMP
```

### Recovery Procedures
1. Identify affected key patterns
2. Restore from most recent backup
3. Replay transactions from audit logs
4. Verify data consistency
5. Resume normal operations

## Best Practices

### Key Naming
1. Use consistent naming conventions
2. Include TTL information in documentation
3. Group related keys with common prefixes
4. Avoid special characters in key names
5. Use descriptive but concise names

### Performance
1. Use pipelining for batch operations
2. Implement proper TTL for all keys
3. Monitor key distribution and hotspots
4. Use appropriate data structures
5. Batch expire operations

### Reliability
1. Implement proper error handling
2. Use distributed locks for consistency
3. Monitor key expiration patterns
4. Plan for disaster recovery
5. Regular backup verification

## Troubleshooting

### Common Issues
```redis
# Find memory-heavy keys
MEMORY USAGE key

# Check key distribution
INFO commandstats

# Monitor slow operations
SLOWLOG GET 10

# Check expired key statistics
INFO stats
```

### Debug Commands
```redis
# Key analysis
TYPE key
TTL key
OBJECT ENCODING key
DEBUG OBJECT key

# Pattern analysis  
SCAN 0 MATCH pattern COUNT 1000
```

This Redis key schema provides a comprehensive foundation for ultra-high concurrency operations while maintaining data consistency, performance, and operational excellence.
