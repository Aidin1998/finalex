# PinCEX Disaster Recovery Procedures

## Table of Contents
1. [Disaster Recovery Overview](#disaster-recovery-overview)
2. [Recovery Time Objectives (RTO)](#recovery-time-objectives-rto)
3. [Backup Procedures](#backup-procedures)
4. [Data Recovery Scenarios](#data-recovery-scenarios)
5. [Infrastructure Recovery](#infrastructure-recovery)
6. [Business Continuity](#business-continuity)
7. [Testing and Validation](#testing-and-validation)
8. [Emergency Contacts](#emergency-contacts)

---

## Disaster Recovery Overview

### Disaster Classification

#### Severity Levels

**Level 1: Critical (Total System Failure)**
- Complete data center failure
- Major security breach with data compromise
- Complete database corruption
- **RTO**: 4 hours
- **RPO**: 15 minutes

**Level 2: Major (Partial System Failure)**
- Single service cluster failure
- Database primary failure with replica availability
- Network infrastructure failure
- **RTO**: 1 hour
- **RPO**: 5 minutes

**Level 3: Minor (Service Degradation)**
- Individual service instance failure
- Non-critical data corruption
- Performance degradation
- **RTO**: 15 minutes
- **RPO**: 1 minute

### Business Impact Assessment

| System Component | Business Impact | Recovery Priority |
|------------------|-----------------|-------------------|
| Trading Engine | Critical - Revenue Loss | 1 |
| User Database | Critical - Regulatory/Legal | 1 |
| Authentication | Critical - Security | 1 |
| Market Data | High - Customer Experience | 2 |
| Settlement | High - Financial Integrity | 2 |
| WebSocket Services | Medium - Real-time Features | 3 |
| Analytics | Low - Reporting | 4 |

---

## Recovery Time Objectives (RTO)

### Service-Level Recovery Targets

| Service | RTO Target | Maximum Downtime | Recovery Steps |
|---------|------------|------------------|----------------|
| Trading Engine | 15 minutes | 30 minutes | Automated failover + manual validation |
| Authentication | 10 minutes | 20 minutes | Database failover + service restart |
| Settlement | 30 minutes | 1 hour | Data validation + manual reconciliation |
| Market Data | 5 minutes | 10 minutes | Feed reconnection + cache rebuild |
| User Database | 20 minutes | 45 minutes | Replica promotion + data validation |
| WebSocket | 5 minutes | 15 minutes | Service restart + connection rebuild |

### Recovery Point Objectives (RPO)

| Data Type | RPO Target | Backup Frequency | Recovery Method |
|-----------|------------|------------------|-----------------|
| Trading Data | 1 minute | Continuous replication | Real-time sync |
| User Accounts | 5 minutes | Every 5 minutes | Incremental backup |
| Financial Records | 30 seconds | Real-time + hourly | Transaction log replay |
| Configuration | 1 hour | Hourly snapshots | Version control restore |
| Audit Logs | 15 minutes | Continuous streaming | Log replay |

---

## Backup Procedures

### Database Backup Strategy

#### Primary Database Backup
```powershell
# Automated daily full backup
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$backupPath = "gs://pincex-backups/postgresql/daily"

# Full backup
kubectl exec -n pincex-production deployment/postgres-primary -- pg_dump -U admin -Fc pincex | gsutil cp - "$backupPath/pincex_full_$timestamp.backup"

# WAL archiving for point-in-time recovery
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -c "SELECT pg_start_backup('daily_backup');"
gsutil -m cp -r /var/lib/postgresql/data/pg_wal gs://pincex-backups/postgresql/wal/
kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -c "SELECT pg_stop_backup();"
```

#### Incremental Backup Script
```powershell
# scripts/incremental-backup.ps1

param(
    [string]$BackupType = "incremental",
    [string]$Retention = "30d"
)

$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$backupBase = "gs://pincex-backups"

Write-Host "Starting $BackupType backup at $timestamp"

# Database incremental backup
if ($BackupType -eq "incremental" -or $BackupType -eq "full") {
    Write-Host "Backing up PostgreSQL database..."
    
    if ($BackupType -eq "full") {
        # Full database backup
        kubectl exec -n pincex-production deployment/postgres-primary -- pg_dump -U admin -Fc --verbose pincex | gsutil cp - "$backupBase/postgresql/full/pincex_full_$timestamp.backup"
    } else {
        # Incremental using WAL files
        kubectl exec -n pincex-production deployment/postgres-primary -- /scripts/backup-wal.sh
        gsutil -m rsync -d -r /tmp/wal-backup "$backupBase/postgresql/wal/$timestamp/"
    }
}

# Redis backup
Write-Host "Backing up Redis data..."
kubectl exec -n pincex-production deployment/redis-0 -- redis-cli BGSAVE
Start-Sleep 30  # Wait for background save to complete
kubectl cp pincex-production/redis-0:/data/dump.rdb "./redis_backup_$timestamp.rdb"
gsutil cp "./redis_backup_$timestamp.rdb" "$backupBase/redis/redis_$timestamp.rdb"
Remove-Item "./redis_backup_$timestamp.rdb"

# Configuration backup
Write-Host "Backing up configurations..."
kubectl get configmaps -n pincex-production -o yaml | gsutil cp - "$backupBase/config/configmaps_$timestamp.yaml"
kubectl get secrets -n pincex-production -o yaml | gsutil cp - "$backupBase/config/secrets_$timestamp.yaml"

# Application state backup
Write-Host "Backing up application state..."
kubectl exec -n pincex-production deployment/trading-engine -- curl -s http://localhost:8080/admin/export/state | gsutil cp - "$backupBase/state/trading_state_$timestamp.json"

# Cleanup old backups based on retention policy
Write-Host "Cleaning up old backups (retention: $Retention)..."
$retentionDate = (Get-Date).AddDays(-[int]$Retention.TrimEnd('d'))
$retentionTimestamp = $retentionDate.ToString("yyyyMMdd")

gsutil -m rm "$backupBase/postgresql/daily/pincex_full_$retentionTimestamp*.backup"
gsutil -m rm "$backupBase/redis/redis_$retentionTimestamp*.rdb"

Write-Host "Backup completed successfully"
```

### Continuous Data Protection

#### Real-time Replication Setup
```yaml
# postgres-replica.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres-replica
  namespace: pincex-production
spec:
  serviceName: postgres-replica
  replicas: 2
  template:
    spec:
      containers:
      - name: postgres
        image: postgres:15
        env:
        - name: POSTGRES_USER
          value: "replication_user"
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-credentials
              key: replication-password
        - name: PGUSER
          value: "replication_user"
        command:
        - postgres
        - -c
        - hot_standby=on
        - -c
        - wal_level=replica
        - -c
        - max_wal_senders=3
        - -c
        - wal_keep_segments=64
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        - name: recovery-conf
          mountPath: /var/lib/postgresql/recovery.conf
          subPath: recovery.conf
      volumes:
      - name: recovery-conf
        configMap:
          name: postgres-recovery-config
```

#### Cross-Region Backup Replication
```powershell
# Setup cross-region backup replication
$primaryRegion = "us-central1"
$drRegion = "us-east1"

# Sync backups to DR region every hour
gsutil -m rsync -d -r "gs://pincex-backups-$primaryRegion/" "gs://pincex-backups-$drRegion/"

# Verify backup integrity
$latestBackup = gsutil ls "gs://pincex-backups-$drRegion/postgresql/daily/" | Sort-Object | Select-Object -Last 1
gsutil cp $latestBackup - | pg_restore --list > backup_contents.txt
Write-Host "Backup verification completed. Contents saved to backup_contents.txt"
```

---

## Data Recovery Scenarios

### Scenario 1: Complete Database Loss

#### Assessment Phase
```powershell
# Assess the extent of data loss
Write-Host "Assessing database status..."

# Check if primary database is accessible
$dbStatus = kubectl exec -n pincex-production deployment/postgres-primary -- pg_isready 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "Primary database is not accessible"
    
    # Check replica status
    $replicaStatus = kubectl exec -n pincex-production deployment/postgres-replica-0 -- pg_isready 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Replica database is accessible - proceeding with replica promotion"
        $recoveryStrategy = "replica_promotion"
    } else {
        Write-Host "All databases are down - proceeding with backup restoration"
        $recoveryStrategy = "backup_restore"
    }
} else {
    Write-Host "Primary database is accessible - checking for data corruption"
    $recoveryStrategy = "corruption_repair"
}
```

#### Recovery Execution
```powershell
# Recovery strategy execution
switch ($recoveryStrategy) {
    "replica_promotion" {
        Write-Host "Promoting replica to primary..."
        
        # Stop all application services
        kubectl scale deployment --all --replicas=0 -n pincex-production
        
        # Promote replica
        kubectl exec -n pincex-production deployment/postgres-replica-0 -- su - postgres -c "pg_promote"
        
        # Update service endpoints
        kubectl patch service postgres-primary -n pincex-production -p '{"spec":{"selector":{"app":"postgres-replica"}}}'
        
        # Restart services
        kubectl scale deployment trading-engine --replicas=3 -n pincex-production
        kubectl scale deployment auth-service --replicas=2 -n pincex-production
        
        Write-Host "Replica promotion completed"
    }
    
    "backup_restore" {
        Write-Host "Restoring from backup..."
        
        # Get latest backup
        $latestBackup = gsutil ls gs://pincex-backups/postgresql/daily/ | Sort-Object | Select-Object -Last 1
        Write-Host "Restoring from: $latestBackup"
        
        # Stop all services
        kubectl scale deployment --all --replicas=0 -n pincex-production
        
        # Restore database
        gsutil cp $latestBackup - | kubectl exec -i -n pincex-production deployment/postgres-primary -- pg_restore -U admin -d pincex --clean --if-exists
        
        # Apply WAL files for point-in-time recovery
        $walFiles = gsutil ls gs://pincex-backups/postgresql/wal/ | Sort-Object
        foreach ($walFile in $walFiles) {
            gsutil cp $walFile - | kubectl exec -i -n pincex-production deployment/postgres-primary -- /scripts/apply-wal.sh
        }
        
        Write-Host "Database restoration completed"
    }
}
```

### Scenario 2: Trading Engine Failure

#### Immediate Response
```powershell
# Trading engine failure recovery
Write-Host "Detecting trading engine failure..."

# Check trading engine health
$engineHealth = kubectl exec -n pincex-production deployment/trading-engine -- curl -f http://localhost:8080/health 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "Trading engine is not responding - initiating recovery"
    
    # Enable maintenance mode
    kubectl patch configmap app-config -n pincex-production --patch='{"data":{"MAINTENANCE_MODE":"true"}}'
    
    # Check for corrupted order state
    $orderCount = kubectl exec -n pincex-production deployment/postgres-primary -- psql -U admin -d pincex -t -c "SELECT COUNT(*) FROM orders WHERE status IN ('PENDING', 'PARTIAL');"
    Write-Host "Found $orderCount pending orders"
    
    # Restart trading engine with state recovery
    kubectl delete pod -n pincex-production -l app=trading-engine
    kubectl wait --for=condition=ready pod -l app=trading-engine -n pincex-production --timeout=300s
    
    # Verify order book reconstruction
    $symbols = @("BTCUSD", "ETHUSD", "LTCUSD")
    foreach ($symbol in $symbols) {
        kubectl exec -n pincex-production deployment/trading-engine -- curl -f "http://localhost:8080/admin/orderbook/validate/$symbol"
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Order book validation failed for $symbol - rebuilding"
            kubectl exec -n pincex-production deployment/trading-engine -- curl -X POST "http://localhost:8080/admin/orderbook/rebuild/$symbol"
        }
    }
    
    # Disable maintenance mode
    kubectl patch configmap app-config -n pincex-production --patch='{"data":{"MAINTENANCE_MODE":"false"}}'
    Write-Host "Trading engine recovery completed"
}
```

### Scenario 3: Market Data Feed Interruption

#### Feed Recovery Process
```powershell
# Market data feed recovery
Write-Host "Checking market data feed status..."

$feeds = @("binance", "coinbase", "kraken")
$failedFeeds = @()

foreach ($feed in $feeds) {
    $feedStatus = kubectl exec -n pincex-production deployment/market-data -- curl -f "http://localhost:8080/admin/feeds/$feed/status" 2>$null
    if ($LASTEXITCODE -ne 0) {
        $failedFeeds += $feed
        Write-Host "Feed $feed is down"
    }
}

if ($failedFeeds.Count -gt 0) {
    Write-Host "Recovering failed feeds: $($failedFeeds -join ', ')"
    
    foreach ($feed in $failedFeeds) {
        # Restart specific feed
        kubectl exec -n pincex-production deployment/market-data -- curl -X POST "http://localhost:8080/admin/feeds/$feed/restart"
        
        # Wait for feed to stabilize
        Start-Sleep 10
        
        # Verify feed recovery
        $attempts = 0
        do {
            $feedStatus = kubectl exec -n pincex-production deployment/market-data -- curl -f "http://localhost:8080/admin/feeds/$feed/status" 2>$null
            $attempts++
            Start-Sleep 5
        } while ($LASTEXITCODE -ne 0 -and $attempts -lt 12)
        
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Feed $feed recovered successfully"
        } else {
            Write-Host "Feed $feed recovery failed - escalating to manual intervention"
        }
    }
    
    # Rebuild price cache from recovered feeds
    kubectl exec -n pincex-production deployment/market-data -- curl -X POST "http://localhost:8080/admin/cache/rebuild"
}
```

---

## Infrastructure Recovery

### Kubernetes Cluster Recovery

#### Cluster Health Assessment
```powershell
# scripts/assess-cluster-health.ps1

Write-Host "Assessing Kubernetes cluster health..."

# Check node status
$unhealthyNodes = kubectl get nodes --no-headers | Where-Object { $_ -notmatch "Ready" }
if ($unhealthyNodes) {
    Write-Host "Unhealthy nodes detected:"
    $unhealthyNodes | ForEach-Object { Write-Host "  $_" }
}

# Check critical system pods
$systemNamespaces = @("kube-system", "kube-public", "istio-system")
foreach ($namespace in $systemNamespaces) {
    $failedPods = kubectl get pods -n $namespace --field-selector=status.phase!=Running --no-headers
    if ($failedPods) {
        Write-Host "Failed pods in $namespace:"
        $failedPods | ForEach-Object { Write-Host "  $_" }
    }
}

# Check persistent volumes
$failedPVs = kubectl get pv --no-headers | Where-Object { $_ -notmatch "Bound" }
if ($failedPVs) {
    Write-Host "Failed persistent volumes:"
    $failedPVs | ForEach-Object { Write-Host "  $_" }
}

# Check critical services
$criticalServices = @("postgres-primary", "redis", "trading-engine")
foreach ($service in $criticalServices) {
    $serviceStatus = kubectl get service $service -n pincex-production -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>$null
    if (-not $serviceStatus) {
        Write-Host "Service $service is not accessible"
    }
}
```

#### Cluster Recovery Procedures
```powershell
# Complete cluster recovery
function Restore-KubernetesCluster {
    param(
        [string]$BackupLocation = "gs://pincex-backups/k8s",
        [string]$ClusterName = "pincex-production"
    )
    
    Write-Host "Starting cluster recovery for $ClusterName"
    
    # Restore cluster configuration
    Write-Host "Restoring cluster configuration..."
    gsutil cp "$BackupLocation/cluster-config.yaml" ./cluster-config.yaml
    kubectl apply -f ./cluster-config.yaml
    
    # Restore namespaces
    Write-Host "Restoring namespaces..."
    gsutil cp "$BackupLocation/namespaces.yaml" ./namespaces.yaml
    kubectl apply -f ./namespaces.yaml
    
    # Restore RBAC
    Write-Host "Restoring RBAC configuration..."
    gsutil cp "$BackupLocation/rbac.yaml" ./rbac.yaml
    kubectl apply -f ./rbac.yaml
    
    # Restore secrets and configmaps
    Write-Host "Restoring secrets and configmaps..."
    gsutil cp "$BackupLocation/secrets.yaml" ./secrets.yaml
    gsutil cp "$BackupLocation/configmaps.yaml" ./configmaps.yaml
    kubectl apply -f ./secrets.yaml
    kubectl apply -f ./configmaps.yaml
    
    # Restore persistent volume claims
    Write-Host "Restoring persistent volume claims..."
    gsutil cp "$BackupLocation/pvcs.yaml" ./pvcs.yaml
    kubectl apply -f ./pvcs.yaml
    
    # Wait for PVCs to be bound
    kubectl wait --for=condition=bound pvc --all -n pincex-production --timeout=300s
    
    # Restore deployments and services
    Write-Host "Restoring applications..."
    gsutil cp "$BackupLocation/deployments.yaml" ./deployments.yaml
    gsutil cp "$BackupLocation/services.yaml" ./services.yaml
    kubectl apply -f ./deployments.yaml
    kubectl apply -f ./services.yaml
    
    # Wait for deployments to be ready
    kubectl wait --for=condition=available deployment --all -n pincex-production --timeout=600s
    
    Write-Host "Cluster recovery completed"
}
```

### Network Infrastructure Recovery

#### Network Connectivity Restoration
```powershell
# Network recovery procedures
function Restore-NetworkInfrastructure {
    Write-Host "Checking network infrastructure..."
    
    # Check external connectivity
    $externalEndpoints = @(
        "https://api.binance.com/api/v3/ping",
        "https://api.pro.coinbase.com/time",
        "https://api.kraken.com/0/public/Time"
    )
    
    foreach ($endpoint in $externalEndpoints) {
        try {
            $response = Invoke-RestMethod -Uri $endpoint -TimeoutSec 10
            Write-Host "✓ Connectivity to $endpoint: OK"
        } catch {
            Write-Host "✗ Connectivity to $endpoint: FAILED"
        }
    }
    
    # Check internal service connectivity
    $internalServices = @(
        "postgres-primary.pincex-production.svc.cluster.local:5432",
        "redis.pincex-production.svc.cluster.local:6379",
        "trading-engine.pincex-production.svc.cluster.local:8080"
    )
    
    foreach ($service in $internalServices) {
        $host, $port = $service.Split(':')
        $tcpTest = Test-NetConnection -ComputerName $host -Port $port -WarningAction SilentlyContinue
        if ($tcpTest.TcpTestSucceeded) {
            Write-Host "✓ Internal connectivity to $service: OK"
        } else {
            Write-Host "✗ Internal connectivity to $service: FAILED"
        }
    }
    
    # Check DNS resolution
    $dnsTests = @(
        "postgres-primary.pincex-production.svc.cluster.local",
        "api.pincex.com",
        "www.google.com"
    )
    
    foreach ($dnsName in $dnsTests) {
        try {
            $resolved = Resolve-DnsName $dnsName -ErrorAction Stop
            Write-Host "✓ DNS resolution for $dnsName: OK"
        } catch {
            Write-Host "✗ DNS resolution for $dnsName: FAILED"
        }
    }
}
```

---

## Business Continuity

### Trading Halt Procedures

#### Controlled Trading Halt
```powershell
# Initiate controlled trading halt
function Start-TradingHalt {
    param(
        [string]$Reason = "System maintenance",
        [int]$EstimatedDurationMinutes = 30
    )
    
    Write-Host "Initiating controlled trading halt..."
    Write-Host "Reason: $Reason"
    Write-Host "Estimated duration: $EstimatedDurationMinutes minutes"
    
    # Stop accepting new orders
    kubectl patch configmap app-config -n pincex-production --patch='{"data":{"ACCEPT_NEW_ORDERS":"false"}}'
    
    # Wait for order queue to drain
    $maxWaitTime = 300  # 5 minutes
    $startTime = Get-Date
    
    do {
        $queueDepth = kubectl exec -n pincex-production deployment/trading-engine -- curl -s http://localhost:8080/admin/queue/depth
        Write-Host "Order queue depth: $queueDepth"
        
        if ($queueDepth -eq 0) {
            break
        }
        
        Start-Sleep 5
        $elapsed = (Get-Date) - $startTime
    } while ($elapsed.TotalSeconds -lt $maxWaitTime)
    
    if ($queueDepth -gt 0) {
        Write-Host "Warning: Order queue did not drain completely. Proceeding with halt."
    }
    
    # Enable maintenance mode
    kubectl patch configmap app-config -n pincex-production --patch='{"data":{"MAINTENANCE_MODE":"true"}}'
    
    # Update status page
    $statusMessage = @{
        status = "maintenance"
        message = $Reason
        estimatedResolution = (Get-Date).AddMinutes($EstimatedDurationMinutes).ToString("yyyy-MM-ddTHH:mm:ssZ")
    } | ConvertTo-Json
    
    # Send notifications
    Send-TradingHaltNotification -Reason $Reason -EstimatedDuration $EstimatedDurationMinutes
    
    Write-Host "Trading halt initiated successfully"
}
```

#### Trading Resumption
```powershell
# Resume trading operations
function Resume-Trading {
    param(
        [switch]$RunHealthChecks = $true
    )
    
    Write-Host "Preparing to resume trading operations..."
    
    if ($RunHealthChecks) {
        Write-Host "Running pre-flight health checks..."
        
        # Database connectivity check
        $dbHealth = kubectl exec -n pincex-production deployment/postgres-primary -- pg_isready
        if ($LASTEXITCODE -ne 0) {
            throw "Database health check failed. Cannot resume trading."
        }
        
        # Trading engine health check
        $engineHealth = kubectl exec -n pincex-production deployment/trading-engine -- curl -f http://localhost:8080/health
        if ($LASTEXITCODE -ne 0) {
            throw "Trading engine health check failed. Cannot resume trading."
        }
        
        # Market data feed check
        $feedHealth = kubectl exec -n pincex-production deployment/market-data -- curl -f http://localhost:8080/health
        if ($LASTEXITCODE -ne 0) {
            throw "Market data health check failed. Cannot resume trading."
        }
        
        Write-Host "✓ All health checks passed"
    }
    
    # Disable maintenance mode
    kubectl patch configmap app-config -n pincex-production --patch='{"data":{"MAINTENANCE_MODE":"false"}}'
    
    # Enable order acceptance
    kubectl patch configmap app-config -n pincex-production --patch='{"data":{"ACCEPT_NEW_ORDERS":"true"}}'
    
    # Verify system is accepting orders
    Start-Sleep 10
    $orderTest = kubectl exec -n pincex-production deployment/trading-engine -- curl -f http://localhost:8080/admin/status
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ Trading operations resumed successfully"
        
        # Send resumption notification
        Send-TradingResumptionNotification
        
        # Update status page
        $statusMessage = @{
            status = "operational"
            message = "All systems operational"
            timestamp = (Get-Date).ToString("yyyy-MM-ddTHH:mm:ssZ")
        } | ConvertTo-Json
        
    } else {
        throw "Failed to resume trading operations. System is not accepting orders."
    }
}
```

### Communication Procedures

#### Stakeholder Notification System
```powershell
# Notification functions
function Send-TradingHaltNotification {
    param(
        [string]$Reason,
        [int]$EstimatedDuration
    )
    
    $message = @"
URGENT: PinCEX Trading Halt

Trading has been temporarily halted due to: $Reason

Estimated Resolution Time: $EstimatedDuration minutes
Current Time: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss UTC')

We will provide updates every 15 minutes until resolution.

Status Page: https://status.pincex.com
Support: support@pincex.com

PinCEX Operations Team
"@

    # Send to multiple channels
    Send-SlackNotification -Channel "#trading-alerts" -Message $message -Priority "high"
    Send-EmailNotification -Recipients @("trading@pincex.com", "management@pincex.com") -Subject "URGENT: Trading Halt" -Body $message
    Update-StatusPage -Status "maintenance" -Message $Reason
    
    # SMS to critical personnel
    $criticalContacts = @("+1-555-0123", "+1-555-0124", "+1-555-0125")
    foreach ($contact in $criticalContacts) {
        Send-SMSNotification -Number $contact -Message "PinCEX: Trading halted - $Reason. Check email for details."
    }
}

function Send-TradingResumptionNotification {
    $message = @"
RESOLVED: PinCEX Trading Resumed

Trading operations have been successfully resumed.

Resolution Time: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss UTC')

All systems are now operational and accepting orders.

Thank you for your patience.

PinCEX Operations Team
"@

    Send-SlackNotification -Channel "#trading-alerts" -Message $message -Priority "resolved"
    Send-EmailNotification -Recipients @("trading@pincex.com", "management@pincex.com") -Subject "RESOLVED: Trading Resumed" -Body $message
    Update-StatusPage -Status "operational" -Message "All systems operational"
}
```

---

## Testing and Validation

### Disaster Recovery Testing Schedule

#### Monthly Tests
- Database failover testing
- Backup restoration validation
- Network failure simulation
- Service recovery procedures

#### Quarterly Tests
- Complete infrastructure recovery
- Cross-region failover
- Business continuity procedures
- Communication protocol testing

#### Annual Tests
- Full disaster recovery simulation
- Regulatory compliance validation
- Third-party vendor coordination
- Documentation review and updates

### Test Execution Framework

#### Automated DR Testing
```powershell
# scripts/dr-test-automation.ps1

param(
    [ValidateSet("database", "infrastructure", "network", "full")]
    [string]$TestType = "database",
    [switch]$DryRun = $false
)

function Test-DatabaseFailover {
    Write-Host "Testing database failover..."
    
    if (-not $DryRun) {
        # Simulate primary database failure
        kubectl patch deployment postgres-primary -n pincex-production -p '{"spec":{"replicas":0}}'
        
        # Wait for replica promotion
        Start-Sleep 30
        
        # Verify replica is serving traffic
        $replicaHealth = kubectl exec -n pincex-production deployment/postgres-replica-0 -- pg_isready
        if ($LASTEXITCODE -eq 0) {
            Write-Host "✓ Database failover successful"
        } else {
            Write-Host "✗ Database failover failed"
        }
        
        # Restore primary
        kubectl patch deployment postgres-primary -n pincex-production -p '{"spec":{"replicas":1}}'
    } else {
        Write-Host "DRY RUN: Would simulate database failover"
    }
}

function Test-InfrastructureRecovery {
    Write-Host "Testing infrastructure recovery..."
    
    if (-not $DryRun) {
        # Scale down non-critical services
        kubectl scale deployment websocket --replicas=0 -n pincex-production
        kubectl scale deployment analytics --replicas=0 -n pincex-production
        
        # Wait and verify core services remain healthy
        Start-Sleep 60
        
        $coreServices = @("trading-engine", "auth-service", "market-data")
        $allHealthy = $true
        
        foreach ($service in $coreServices) {
            $health = kubectl exec -n pincex-production deployment/$service -- curl -f http://localhost:8080/health 2>$null
            if ($LASTEXITCODE -ne 0) {
                Write-Host "✗ Core service $service is unhealthy"
                $allHealthy = $false
            }
        }
        
        if ($allHealthy) {
            Write-Host "✓ Core services remained healthy during infrastructure test"
        }
        
        # Restore all services
        kubectl scale deployment websocket --replicas=2 -n pincex-production
        kubectl scale deployment analytics --replicas=1 -n pincex-production
    } else {
        Write-Host "DRY RUN: Would test infrastructure recovery"
    }
}

# Execute test based on type
switch ($TestType) {
    "database" { Test-DatabaseFailover }
    "infrastructure" { Test-InfrastructureRecovery }
    "network" { Test-NetworkFailure }
    "full" { 
        Test-DatabaseFailover
        Test-InfrastructureRecovery
        Test-NetworkFailure
    }
}
```

### Recovery Validation Checklist

#### Post-Recovery Validation
- [ ] All critical services are running and healthy
- [ ] Database connectivity and integrity verified
- [ ] Order processing functionality confirmed
- [ ] Market data feeds are operational
- [ ] User authentication is working
- [ ] WebSocket connections are stable
- [ ] Monitoring and alerting are functional
- [ ] Audit logs are being generated
- [ ] External integrations are operational
- [ ] Performance metrics are within acceptable ranges

#### Data Integrity Validation
- [ ] Order data consistency check
- [ ] Balance reconciliation completed
- [ ] Trade history verification
- [ ] Audit trail completeness
- [ ] Configuration consistency
- [ ] User account data integrity

---

## Emergency Contacts

### Primary Response Team

**Incident Commander**
- Name: Chief Technology Officer
- Phone: +1-555-0126
- Email: cto@pincex.com
- Backup: Head of Engineering (+1-555-0127)

**Database Administrator**
- Name: Senior DBA
- Phone: +1-555-0128
- Email: dba@pincex.com
- Backup: Infrastructure Engineer (+1-555-0129)

**Security Officer**
- Name: Chief Security Officer
- Phone: +1-555-0130
- Email: security@pincex.com
- Backup: Security Engineer (+1-555-0131)

**Trading Operations**
- Name: Trading Director
- Phone: +1-555-0132
- Email: trading@pincex.com
- Backup: Senior Trader (+1-555-0133)

### External Contacts

**Cloud Provider (Google Cloud)**
- Support: +1-855-836-1615
- Account Manager: +1-555-0140
- Priority Support PIN: [CONFIDENTIAL]

**Database Vendor (PostgreSQL Support)**
- Enterprise Support: +1-855-POSTGRES
- Account: enterprise@pincex.com

**Network Provider**
- NOC: +1-800-NETWORK
- Account Manager: +1-555-0145

**Legal Counsel**
- Primary: +1-555-0150
- Regulatory: +1-555-0151

### Escalation Matrix

| Time from Incident | Action | Notify |
|-------------------|---------|---------|
| 0-15 minutes | Initial response | On-call Engineer, Team Lead |
| 15-30 minutes | Service impact assessment | Engineering Manager, CTO |
| 30-60 minutes | Business impact evaluation | CEO, Trading Director |
| 1+ hours | External communication | PR Team, Regulatory Affairs |
| 4+ hours | Regulatory notification | Legal Counsel, Compliance |

---

## Appendix

### Recovery Time Tracking

#### Key Performance Indicators
- **Mean Time To Detection (MTTD)**: Average time to detect an incident
- **Mean Time To Response (MTTR)**: Average time to begin response
- **Mean Time To Resolution (MTTR)**: Average time to fully resolve
- **Recovery Point Objective (RPO)**: Maximum acceptable data loss
- **Recovery Time Objective (RTO)**: Maximum acceptable downtime

#### SLA Compliance Tracking
```powershell
# Calculate DR SLA compliance
function Calculate-DRCompliance {
    param(
        [datetime]$IncidentStart,
        [datetime]$ServiceRestored,
        [datetime]$FullyResolved,
        [string]$ServiceTier = "Critical"
    )
    
    $detectionTime = ($IncidentStart - $IncidentStart).TotalMinutes  # Assuming immediate detection
    $responseTime = ($ServiceRestored - $IncidentStart).TotalMinutes
    $resolutionTime = ($FullyResolved - $IncidentStart).TotalMinutes
    
    $slaTargets = @{
        "Critical" = @{ RTO = 15; RPO = 1 }
        "High" = @{ RTO = 60; RPO = 5 }
        "Medium" = @{ RTO = 240; RPO = 15 }
    }
    
    $target = $slaTargets[$ServiceTier]
    $rtoCompliance = $responseTime -le $target.RTO
    
    Write-Host "DR Performance Metrics:"
    Write-Host "Service Tier: $ServiceTier"
    Write-Host "Response Time: $([math]::Round($responseTime, 2)) minutes"
    Write-Host "RTO Target: $($target.RTO) minutes"
    Write-Host "RTO Compliance: $(if($rtoCompliance){"✓ PASS"}else{"✗ FAIL"})"
    Write-Host "Resolution Time: $([math]::Round($resolutionTime, 2)) minutes"
    
    return @{
        ResponseTime = $responseTime
        ResolutionTime = $resolutionTime
        RTOCompliance = $rtoCompliance
        ServiceTier = $ServiceTier
    }
}
```

---

*This disaster recovery plan should be reviewed and updated quarterly, and tested according to the defined schedule.*
