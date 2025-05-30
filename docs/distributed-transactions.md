# Distributed Transaction Management System

## Overview

This document describes the comprehensive distributed transaction management system implemented for the PinCEX cryptocurrency exchange platform. The system provides enterprise-grade ACID guarantees across microservices using the XA protocol and two-phase commit.

## Architecture

### Core Components

1. **XA Transaction Manager** (`xa_manager.go`)
   - Implements the XA protocol for distributed transaction coordination
   - Manages two-phase commit (2PC) protocol
   - Handles transaction state tracking and recovery
   - Provides timeout management and heuristic outcome handling

2. **XA Resource Adapters**
   - **Bookkeeper XA Resource** (`bookkeeper_xa.go`) - Account balance management
   - **Settlement XA Resource** (`settlement_xa.go`) - Trade settlement operations
   - **Trading XA Resource** (`trading_xa.go`) - Order placement and matching
   - **Fiat XA Resource** (`fiat_xa.go`) - Fiat currency operations
   - **Wallet XA Resource** (`wallet_xa.go`) - Cryptocurrency wallet operations

3. **Distributed Lock Manager** (`distributed_locks.go`)
   - Prevents resource conflicts during concurrent operations
   - Implements heartbeat mechanism for lock management
   - Provides saga pattern support for complex workflows

4. **Workflow Orchestrator** (`workflows.go`)
   - Coordinates complex multi-service transactions
   - Implements common exchange workflows (trade execution, deposits, withdrawals)
   - Provides compensation logic for failed transactions

5. **Recovery Manager** (`recovery_manager.go`, `recovery_strategies.go`)
   - Handles transaction recovery after system failures
   - Implements multiple recovery strategies
   - Provides circuit breaker and exponential backoff mechanisms

6. **Monitoring and Alerting** (`monitoring.go`)
   - Real-time transaction monitoring
   - Threshold-based alerting
   - Email and Slack notifications
   - Comprehensive event tracking

7. **Configuration Management** (`config_manager.go`)
   - Dynamic configuration updates
   - File-based configuration (JSON/YAML)
   - Configuration change tracking and validation

8. **Performance Metrics** (`performance_metrics.go`)
   - Real-time performance monitoring
   - Historical metrics storage
   - Transaction tracing and observability

9. **Testing Framework** (`testing_framework.go`)
   - Chaos engineering capabilities
   - Load testing with realistic scenarios
   - Circuit breaker testing

## Installation and Setup

### Prerequisites

- Go 1.19+
- PostgreSQL 13+
- Redis 6+

### Configuration

1. **Copy configuration files:**
```bash
cp configs/transaction-manager.yaml /etc/transaction-manager/config.yaml
# OR
cp configs/transaction-manager.json /etc/transaction-manager/config.json
```

2. **Set environment variables:**
```bash
export TRANSACTION_CONFIG_PATH="/etc/transaction-manager/config.yaml"
export LOG_LEVEL="info"
```

3. **Database migration:**
The system automatically migrates required tables on startup.

### Integration

The distributed transaction system is automatically integrated into the main application. It initializes during startup and provides both programmatic and HTTP APIs.

## API Usage

### HTTP Endpoints

#### Transaction Execution
```bash
# Execute a distributed transaction
POST /api/v1/transactions/execute
{
  "operations": [
    {
      "service": "bookkeeper",
      "operation": "lock_funds",
      "parameters": {
        "user_id": "123",
        "currency": "BTC",
        "amount": 0.5
      }
    },
    {
      "service": "trading",
      "operation": "place_order",
      "parameters": {
        "user_id": "123",
        "symbol": "BTC/USD",
        "side": "sell",
        "amount": 0.5,
        "price": 50000
      }
    }
  ],
  "timeout_seconds": 300
}
```

#### Workflow Execution
```bash
# Execute a complex workflow
POST /api/v1/transactions/execute-workflow
{
  "workflow_type": "trade_execution",
  "parameters": {
    "user_id": "123",
    "symbol": "BTC/USD",
    "side": "buy",
    "amount": 1.0,
    "price": 45000
  },
  "timeout_seconds": 600
}
```

#### Monitoring
```bash
# Get health status
GET /api/v1/transactions/health

# Get transaction metrics
GET /api/v1/transactions/metrics

# Get performance metrics
GET /api/v1/transactions/performance

# Get active alerts
GET /api/v1/transactions/alerts
```

#### Recovery
```bash
# Trigger manual recovery
POST /api/v1/transactions/recovery/trigger
{
  "recovery_type": "global"
}

# Get recovery status
GET /api/v1/transactions/recovery/status
```

### Programmatic API

```go
import "github.com/Aidin1998/pincex_unified/internal/transaction"

// Initialize transaction manager suite
suite, err := transaction.GetTransactionManagerSuite(
    db, logger, bookkeeperSvc, settlementEngine, configPath,
)

// Execute distributed transaction
operations := []transaction.TransactionOperation{
    {
        Service: "bookkeeper",
        Operation: "transfer_funds",
        Parameters: map[string]interface{}{
            "from_user_id": "user1",
            "to_user_id": "user2",
            "currency": "USD",
            "amount": 100.0,
            "description": "Trade settlement",
        },
    },
}

result, err := suite.ExecuteDistributedTransaction(
    ctx, operations, 5*time.Minute,
)
```

## Workflows

### Pre-built Workflows

1. **Trade Execution Workflow**
   - Validates order parameters
   - Locks user funds
   - Places order in matching engine
   - Executes settlement
   - Updates account balances

2. **Fiat Deposit Workflow**
   - Validates bank transfer
   - Checks compliance requirements
   - Credits user account
   - Triggers notifications

3. **Crypto Withdrawal Workflow**
   - Validates withdrawal request
   - Checks security requirements
   - Locks funds
   - Broadcasts transaction
   - Updates balances

### Custom Workflow Example

```go
func CustomTradeWorkflow(ctx context.Context, params map[string]interface{}) error {
    // Start distributed transaction
    txn, err := xaManager.Start(ctx, 10*time.Minute)
    if err != nil {
        return err
    }
    
    // Add transaction to context
    ctx = transaction.WithXATransaction(ctx, txn)
    
    // Enlist required resources
    xaManager.Enlist(txn, bookkeeperXA)
    xaManager.Enlist(txn, tradingXA)
    xaManager.Enlist(txn, settlementXA)
    
    // Execute business logic
    if err := validateOrder(ctx, params); err != nil {
        xaManager.Abort(ctx, txn)
        return err
    }
    
    if err := lockFunds(ctx, params); err != nil {
        xaManager.Abort(ctx, txn)
        return err
    }
    
    if err := placeOrder(ctx, params); err != nil {
        xaManager.Abort(ctx, txn)
        return err
    }
    
    // Commit transaction
    return xaManager.Commit(ctx, txn)
}
```

## Configuration

### XA Manager Configuration
```yaml
xa_manager:
  transaction_timeout: 5m        # Maximum transaction duration
  prepare_timeout: 30s          # Prepare phase timeout
  commit_timeout: 60s           # Commit phase timeout
  rollback_timeout: 30s         # Rollback phase timeout
  max_concurrent_txns: 1000     # Maximum concurrent transactions
  recovery_interval: 30s        # Recovery check interval
```

### Recovery Configuration
```yaml
recovery:
  enabled: true
  max_recovery_attempts: 3
  recovery_timeout: 2m
  retry_backoff_initial: 1s
  retry_backoff_max: 30s
  parallel_recovery: true
  circuit_breaker_enabled: true
```

### Monitoring Configuration
```yaml
monitoring:
  enabled: true
  metrics_interval: 10s
  alert_threshold_error_rate: 0.05
  email_notifications:
    enabled: true
    smtp_host: "smtp.company.com"
    to_addresses: ["ops@company.com"]
  slack_notifications:
    enabled: true
    webhook_url: "https://hooks.slack.com/..."
```

## Testing

### Chaos Engineering

```bash
# Run network partition test
POST /api/v1/transactions/test/chaos
{
  "test_type": "network_partition",
  "duration_seconds": 60,
  "intensity": 3
}

# Run database failure test
POST /api/v1/transactions/test/chaos
{
  "test_type": "database_failure",
  "duration_seconds": 30,
  "intensity": 5
}
```

### Load Testing

```bash
# Run load test
POST /api/v1/transactions/test/load
{
  "concurrency": 100,
  "total_transactions": 10000,
  "test_type": "mixed_workload"
}
```

### Unit Testing

```go
func TestDistributedTransaction(t *testing.T) {
    // Setup test environment
    suite := setupTestSuite(t)
    defer suite.Cleanup()
    
    // Create test scenario
    scenario := &TestScenario{
        Name: "Simple Transfer",
        Operations: []TransactionOperation{
            {
                Service: "bookkeeper",
                Operation: "transfer_funds",
                Parameters: map[string]interface{}{
                    "from_user_id": "user1",
                    "to_user_id": "user2",
                    "currency": "USD",
                    "amount": 100.0,
                },
            },
        },
        ExpectedResult: "success",
    }
    
    // Execute test
    result := suite.ExecuteScenario(context.Background(), scenario)
    assert.Equal(t, "success", result.Status)
}
```

## Monitoring and Observability

### Metrics

The system provides comprehensive metrics:

- **Transaction Metrics**: Success/failure rates, latency percentiles
- **Resource Metrics**: Lock contention, resource utilization
- **Recovery Metrics**: Recovery attempts, success rates
- **Performance Metrics**: Throughput, queue depths

### Alerts

Automatic alerts are generated for:

- High error rates (>5%)
- High timeout rates (>2%)
- High latency (P99 > 5s)
- Recovery failures
- Resource deadlocks

### Logging

Structured logging with correlation IDs:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "message": "Transaction committed",
  "transaction_id": "123e4567-e89b-12d3-a456-426614174000",
  "duration_ms": 150,
  "resources": ["bookkeeper", "trading"],
  "operation_count": 3
}
```

## Troubleshooting

### Common Issues

1. **Transaction Timeouts**
   - Check resource availability
   - Increase timeout configuration
   - Verify network connectivity

2. **Deadlocks**
   - Review lock acquisition order
   - Enable deadlock detection
   - Adjust lock timeouts

3. **Recovery Failures**
   - Check recovery configuration
   - Verify database connectivity
   - Review recovery logs

### Debugging

Enable debug logging:
```yaml
logging:
  level: "debug"
  include_payload: true
```

Check transaction status:
```bash
GET /api/v1/transactions/status/{transaction_id}
```

Review active locks:
```bash
GET /api/v1/transactions/locks
```

## Performance Tuning

### Database Optimization

1. **Connection Pooling**
```yaml
performance:
  connection_pool_size: 50
  max_idle_connections: 10
```

2. **Batch Processing**
```yaml
performance:
  batch_size: 100
  worker_count: 10
```

### Resource Configuration

1. **Timeouts**
- Adjust based on expected operation duration
- Consider network latency
- Account for database performance

2. **Concurrency**
- Set appropriate connection limits
- Configure worker pools
- Monitor resource utilization

## Security Considerations

### Authentication and Authorization

```yaml
security:
  enable_authentication: true
  enable_authorization: true
  api_key_required: true
```

### Audit Logging

```yaml
security:
  audit_logging:
    enabled: true
    log_level: "info"
    include_payload: false  # Avoid logging sensitive data
```

### Rate Limiting

```yaml
security:
  rate_limiting:
    enabled: true
    requests_per_minute: 60
    burst_size: 10
```

## Production Deployment

### Prerequisites

1. **High Availability Database**
   - PostgreSQL cluster with replication
   - Connection pooling (PgBouncer)
   - Regular backups

2. **Redis Cluster**
   - Multi-node Redis setup
   - Persistence enabled
   - Monitoring configured

3. **Load Balancer**
   - Distribute transaction load
   - Health check endpoints
   - Session affinity if needed

### Deployment Steps

1. **Deploy Configuration**
```bash
kubectl create configmap transaction-config \
  --from-file=configs/transaction-manager.yaml
```

2. **Deploy Application**
```bash
kubectl apply -f deployments/transaction-manager.yaml
```

3. **Verify Deployment**
```bash
curl http://transaction-manager/api/v1/transactions/health
```

### Monitoring Setup

1. **Prometheus Metrics**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: transaction-metrics
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"
```

2. **Grafana Dashboard**
- Import provided dashboard template
- Configure alerting rules
- Set up notification channels

## Maintenance

### Regular Tasks

1. **Configuration Updates**
   - Review and update timeouts
   - Adjust concurrency limits
   - Update alert thresholds

2. **Performance Review**
   - Analyze transaction metrics
   - Optimize slow operations
   - Review resource utilization

3. **Recovery Testing**
   - Test recovery procedures
   - Validate backup strategies
   - Update documentation

### Backup and Recovery

1. **Database Backups**
   - Regular automated backups
   - Test restore procedures
   - Monitor backup integrity

2. **Configuration Backups**
   - Version control configuration
   - Document configuration changes
   - Maintain rollback procedures

## Support and Maintenance

### Log Analysis

Monitor for patterns:
- Recurring transaction failures
- Resource contention
- Performance degradation

### Health Checks

Regular health check endpoints:
- `/api/v1/transactions/health` - Overall system health
- `/api/v1/transactions/metrics` - Performance metrics
- `/api/v1/transactions/recovery/status` - Recovery status

### Escalation Procedures

1. **Level 1**: Check monitoring dashboards
2. **Level 2**: Review transaction logs
3. **Level 3**: Engage development team
4. **Level 4**: Implement emergency procedures

For additional support, refer to the system logs and monitoring dashboards for detailed transaction information.
