#!/bin/bash
# =============================
# Migration Health Check Script
# =============================
# Comprehensive health check for migration system components

set -euo pipefail

# Configuration
TRADING_SERVICE_URL="${TRADING_SERVICE_URL:-http://trading-service:8080}"
DB_HOST="${DB_HOST:-postgres}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-pincex}"
REDIS_HOST="${REDIS_HOST:-redis}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Health check functions
check_trading_service() {
    log_info "Checking trading service health..."
    
    if curl -f -s "$TRADING_SERVICE_URL/health" > /dev/null; then
        log_info "✓ Trading service is healthy"
        return 0
    else
        log_error "✗ Trading service health check failed"
        return 1
    fi
}

check_database() {
    log_info "Checking database connectivity..."
    
    if psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" > /dev/null 2>&1; then
        log_info "✓ Database connectivity OK"
        
        # Check database performance
        local active_connections=$(psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT count(*) FROM pg_stat_activity;")
        local max_connections=$(psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -t -c "SHOW max_connections;")
        
        local connection_usage=$((active_connections * 100 / max_connections))
        
        if [ $connection_usage -lt 80 ]; then
            log_info "✓ Database connection usage: ${connection_usage}%"
        else
            log_warn "⚠ High database connection usage: ${connection_usage}%"
        fi
        
        return 0
    else
        log_error "✗ Database connectivity failed"
        return 1
    fi
}

check_redis() {
    log_info "Checking Redis connectivity..."
    
    if redis-cli -h "$REDIS_HOST" ping > /dev/null 2>&1; then
        log_info "✓ Redis connectivity OK"
        
        # Check Redis memory usage
        local memory_info=$(redis-cli -h "$REDIS_HOST" info memory | grep used_memory_human | cut -d: -f2 | tr -d '\r')
        log_info "✓ Redis memory usage: $memory_info"
        
        return 0
    else
        log_error "✗ Redis connectivity failed"
        return 1
    fi
}

check_migration_components() {
    log_info "Checking migration component status..."
    
    # Check coordinator
    if curl -f -s "$TRADING_SERVICE_URL/admin/migration/coordinator/health" > /dev/null; then
        log_info "✓ Migration coordinator is healthy"
    else
        log_error "✗ Migration coordinator health check failed"
        return 1
    fi
    
    # Check participants
    local participants=$(curl -s "$TRADING_SERVICE_URL/admin/migration/participants" | jq -r '.[].id' 2>/dev/null || echo "")
    
    if [ -n "$participants" ]; then
        log_info "✓ Migration participants registered:"
        echo "$participants" | while read -r participant; do
            log_info "  - $participant"
        done
    else
        log_warn "⚠ No migration participants found"
    fi
    
    # Check safety manager
    if curl -f -s "$TRADING_SERVICE_URL/admin/migration/safety/health" > /dev/null; then
        log_info "✓ Safety manager is healthy"
    else
        log_error "✗ Safety manager health check failed"
        return 1
    fi
    
    return 0
}

check_system_resources() {
    log_info "Checking system resources..."
    
    # Check if running in Kubernetes
    if command -v kubectl > /dev/null && kubectl cluster-info > /dev/null 2>&1; then
        # Kubernetes environment
        local pods=$(kubectl get pods -l app=trading-service --no-headers 2>/dev/null | wc -l)
        local ready_pods=$(kubectl get pods -l app=trading-service --no-headers 2>/dev/null | grep -c Running || echo 0)
        
        log_info "✓ Trading service pods: $ready_pods/$pods ready"
        
        # Check resource usage
        kubectl top pods -l app=trading-service 2>/dev/null | tail -n +2 | while read -r line; do
            local pod_name=$(echo "$line" | awk '{print $1}')
            local cpu=$(echo "$line" | awk '{print $2}')
            local memory=$(echo "$line" | awk '{print $3}')
            log_info "  - $pod_name: CPU=$cpu, Memory=$memory"
        done
    else
        # Non-Kubernetes environment
        local load_avg=$(uptime | awk -F'load average:' '{print $2}' | xargs)
        local memory_usage=$(free | grep Mem | awk '{printf("%.1f%%", $3/$2 * 100.0)}')
        local disk_usage=$(df / | tail -1 | awk '{print $5}')
        
        log_info "✓ System load: $load_avg"
        log_info "✓ Memory usage: $memory_usage"
        log_info "✓ Disk usage: $disk_usage"
    fi
    
    return 0
}

check_monitoring() {
    log_info "Checking monitoring systems..."
    
    # Check Prometheus
    if curl -f -s "http://prometheus:9090/-/healthy" > /dev/null 2>&1; then
        log_info "✓ Prometheus is healthy"
    else
        log_warn "⚠ Prometheus health check failed"
    fi
    
    # Check Grafana
    if curl -f -s "http://grafana:3000/api/health" > /dev/null 2>&1; then
        log_info "✓ Grafana is healthy"
    else
        log_warn "⚠ Grafana health check failed"
    fi
    
    return 0
}

check_circuit_breakers() {
    log_info "Checking circuit breaker status..."
    
    local breakers=$(curl -s "$TRADING_SERVICE_URL/admin/circuit-breaker/status" 2>/dev/null | jq -r '.[].name' 2>/dev/null || echo "")
    
    if [ -n "$breakers" ]; then
        echo "$breakers" | while read -r breaker; do
            local status=$(curl -s "$TRADING_SERVICE_URL/admin/circuit-breaker/status" | jq -r ".[] | select(.name==\"$breaker\") | .state")
            if [ "$status" = "closed" ]; then
                log_info "✓ Circuit breaker '$breaker': $status"
            else
                log_warn "⚠ Circuit breaker '$breaker': $status"
            fi
        done
    else
        log_warn "⚠ No circuit breakers found"
    fi
    
    return 0
}

# Main execution
main() {
    echo "========================================"
    echo "Migration System Health Check"
    echo "========================================"
    echo "Started at: $(date)"
    echo ""
    
    local exit_code=0
    
    # Run all health checks
    check_trading_service || exit_code=1
    echo ""
    
    check_database || exit_code=1
    echo ""
    
    check_redis || exit_code=1
    echo ""
    
    check_migration_components || exit_code=1
    echo ""
    
    check_system_resources || exit_code=1
    echo ""
    
    check_monitoring || exit_code=1
    echo ""
    
    check_circuit_breakers || exit_code=1
    echo ""
    
    # Summary
    echo "========================================"
    if [ $exit_code -eq 0 ]; then
        log_info "Overall system health: HEALTHY"
    else
        log_error "Overall system health: DEGRADED"
        echo ""
        log_error "Please address the issues above before proceeding with migration."
    fi
    echo "Completed at: $(date)"
    echo "========================================"
    
    exit $exit_code
}

# Run main function
main "$@"
