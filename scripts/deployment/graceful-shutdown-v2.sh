#!/bin/bash

# PinCEX Unified Exchange - Advanced Graceful Shutdown and Connection Draining
# Comprehensive shutdown orchestration with trading position safety

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${SCRIPT_DIR}/../logs"
SHUTDOWN_LOG="${LOG_DIR}/graceful-shutdown-$(date +%Y%m%d-%H%M%S).log"

# Application configuration
APP_NAME="pincex-unified-exchange"
NAMESPACE="pincex-production"
HEALTH_ENDPOINT="http://localhost:8080/health"
ADMIN_ENDPOINT="http://localhost:8080/admin"
METRICS_ENDPOINT="http://localhost:8080/metrics"

# Timing configuration
PRE_SHUTDOWN_DELAY=5
CONNECTION_DRAIN_TIMEOUT=60
POSITION_SETTLEMENT_TIMEOUT=120
FINAL_SHUTDOWN_TIMEOUT=30
HEALTH_CHECK_INTERVAL=2

# Trading specific configuration
TRADING_ENGINE_ENDPOINT="http://localhost:8080/trading"
ORDER_BOOK_ENDPOINT="http://localhost:8080/orderbook"
SETTLEMENT_ENDPOINT="http://localhost:8080/settlement"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Logging function
log() {
    local level=$1
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo -e "${timestamp} [${level}] ${message}" | tee -a "${SHUTDOWN_LOG}"
}

log_info() { log "INFO" "$@"; }
log_warn() { log "WARN" "$@"; }
log_error() { log "ERROR" "$@"; }
log_success() { log "SUCCESS" "$@"; }
log_debug() { log "DEBUG" "$@"; }

# Setup logging
setup_logging() {
    mkdir -p "${LOG_DIR}"
    log_info "Graceful shutdown process initiated"
    log_info "Process ID: $$"
    log_info "Shutdown log: ${SHUTDOWN_LOG}"
}

# Get current connection metrics
get_connection_metrics() {
    local connections=0
    local active_orders=0
    local pending_settlements=0
    
    # Get active HTTP connections
    if command -v ss >/dev/null; then
        connections=$(ss -ant | grep -c ":8080.*ESTABLISHED" || echo "0")
    elif command -v netstat >/dev/null; then
        connections=$(netstat -ant | grep -c ":8080.*ESTABLISHED" || echo "0")
    fi
    
    # Get active orders from metrics endpoint
    if curl -sf "${METRICS_ENDPOINT}" >/dev/null 2>&1; then
        active_orders=$(curl -s "${METRICS_ENDPOINT}" | grep "active_orders" | awk '{print $2}' || echo "0")
        pending_settlements=$(curl -s "${METRICS_ENDPOINT}" | grep "pending_settlements" | awk '{print $2}' || echo "0")
    fi
    
    echo "${connections}:${active_orders}:${pending_settlements}"
}

# Display connection status
display_connection_status() {
    local metrics=$(get_connection_metrics)
    local connections=$(echo "$metrics" | cut -d: -f1)
    local active_orders=$(echo "$metrics" | cut -d: -f2)
    local pending_settlements=$(echo "$metrics" | cut -d: -f3)
    
    log_info "Connection Status - Connections: ${connections}, Active Orders: ${active_orders}, Pending Settlements: ${pending_settlements}"
}

# Check if application is ready for shutdown
check_shutdown_readiness() {
    log_info "Checking application readiness for shutdown"
    
    # Check if health endpoint is responding
    if ! curl -sf "${HEALTH_ENDPOINT}/ready" >/dev/null 2>&1; then
        log_warn "Application readiness check failed"
        return 1
    fi
    
    # Check for critical trading operations
    if curl -sf "${TRADING_ENGINE_ENDPOINT}/status" >/dev/null 2>&1; then
        local trading_status=$(curl -s "${TRADING_ENGINE_ENDPOINT}/status" | jq -r '.status' 2>/dev/null || echo "unknown")
        if [[ "$trading_status" == "maintenance" ]]; then
            log_warn "Trading engine is in maintenance mode"
            return 1
        fi
    fi
    
    log_success "Application is ready for graceful shutdown"
    return 0
}

# Stop accepting new connections
stop_accepting_connections() {
    log_info "Stopping acceptance of new connections"
    
    # Signal application to stop accepting new requests
    if curl -sf "${ADMIN_ENDPOINT}/shutdown/prepare" -X POST >/dev/null 2>&1; then
        log_success "Application notified to stop accepting new connections"
    else
        log_warn "Failed to notify application via API, using signal method"
        # Send USR1 signal to indicate preparation for shutdown
        if [[ -n "${MAIN_PID:-}" ]]; then
            kill -USR1 "${MAIN_PID}" 2>/dev/null || true
        fi
    fi
    
    # Update readiness probe to fail
    if curl -sf "${ADMIN_ENDPOINT}/readiness/disable" -X POST >/dev/null 2>&1; then
        log_success "Readiness probe disabled"
    else
        log_warn "Could not disable readiness probe via API"
    fi
    
    # Remove from load balancer
    remove_from_load_balancer
    
    sleep "${PRE_SHUTDOWN_DELAY}"
}

# Remove from load balancer
remove_from_load_balancer() {
    log_info "Removing instance from load balancer"
    
    # For Kubernetes environments
    if [[ -n "${POD_NAME:-}" ]] && [[ -n "${POD_NAMESPACE:-}" ]]; then
        # Label pod as not ready
        kubectl label pod "${POD_NAME}" -n "${POD_NAMESPACE}" app.kubernetes.io/ready=false --overwrite 2>/dev/null || true
    fi
    
    # For AWS ALB/NLB
    if [[ -n "${AWS_INSTANCE_ID:-}" ]] && command -v aws >/dev/null; then
        log_info "Deregistering from AWS load balancer"
        # This would require the target group ARN
        # aws elbv2 deregister-targets --target-group-arn "$TARGET_GROUP_ARN" --targets Id="$AWS_INSTANCE_ID"
    fi
    
    # For NGINX upstream
    if [[ -f "/etc/nginx/conf.d/upstream.conf" ]]; then
        log_info "Removing from NGINX upstream"
        # This would require NGINX Plus or custom logic
    fi
    
    log_success "Instance removal from load balancer initiated"
}

# Wait for active connections to drain
wait_for_connection_drain() {
    log_info "Waiting for active connections to drain (timeout: ${CONNECTION_DRAIN_TIMEOUT}s)"
    
    local start_time=$(date +%s)
    local timeout_time=$((start_time + CONNECTION_DRAIN_TIMEOUT))
    
    while [[ $(date +%s) -lt $timeout_time ]]; do
        local metrics=$(get_connection_metrics)
        local connections=$(echo "$metrics" | cut -d: -f1)
        
        display_connection_status
        
        if [[ "$connections" -eq 0 ]]; then
            log_success "All connections have been drained"
            return 0
        fi
        
        sleep "${HEALTH_CHECK_INTERVAL}"
    done
    
    # Check final connection count
    local final_metrics=$(get_connection_metrics)
    local final_connections=$(echo "$final_metrics" | cut -d: -f1)
    
    if [[ "$final_connections" -gt 0 ]]; then
        log_warn "Connection drain timeout reached with ${final_connections} remaining connections"
        return 1
    else
        log_success "All connections drained successfully"
        return 0
    fi
}

# Handle trading-specific shutdown procedures
handle_trading_shutdown() {
    log_info "Initiating trading-specific shutdown procedures"
    
    # Stop order matching
    if curl -sf "${TRADING_ENGINE_ENDPOINT}/matching/stop" -X POST >/dev/null 2>&1; then
        log_success "Order matching stopped"
    else
        log_warn "Failed to stop order matching via API"
    fi
    
    # Pause new order acceptance
    if curl -sf "${ORDER_BOOK_ENDPOINT}/orders/pause" -X POST >/dev/null 2>&1; then
        log_success "New order acceptance paused"
    else
        log_warn "Failed to pause new orders via API"
    fi
    
    # Wait for pending settlements
    wait_for_settlement_completion
    
    # Cancel open orders (if configured)
    if [[ "${CANCEL_OPEN_ORDERS:-false}" == "true" ]]; then
        cancel_open_orders
    fi
    
    # Save order book state
    save_order_book_state
    
    log_success "Trading shutdown procedures completed"
}

# Wait for settlement completion
wait_for_settlement_completion() {
    log_info "Waiting for pending settlements to complete (timeout: ${POSITION_SETTLEMENT_TIMEOUT}s)"
    
    local start_time=$(date +%s)
    local timeout_time=$((start_time + POSITION_SETTLEMENT_TIMEOUT))
    
    while [[ $(date +%s) -lt $timeout_time ]]; do
        local metrics=$(get_connection_metrics)
        local pending_settlements=$(echo "$metrics" | cut -d: -f3)
        
        if [[ "$pending_settlements" -eq 0 ]]; then
            log_success "All settlements completed"
            return 0
        fi
        
        log_info "Waiting for ${pending_settlements} pending settlements..."
        sleep "${HEALTH_CHECK_INTERVAL}"
    done
    
    local final_metrics=$(get_connection_metrics)
    local final_settlements=$(echo "$final_metrics" | cut -d: -f3)
    
    if [[ "$final_settlements" -gt 0 ]]; then
        log_warn "Settlement timeout reached with ${final_settlements} pending settlements"
        # Log the pending settlements for manual review
        log_pending_settlements
        return 1
    else
        log_success "All settlements completed successfully"
        return 0
    fi
}

# Log pending settlements for manual review
log_pending_settlements() {
    log_warn "Logging pending settlements for manual review"
    
    if curl -sf "${SETTLEMENT_ENDPOINT}/pending" >/dev/null 2>&1; then
        local pending_file="${LOG_DIR}/pending-settlements-$(date +%Y%m%d-%H%M%S).json"
        curl -s "${SETTLEMENT_ENDPOINT}/pending" > "${pending_file}"
        log_warn "Pending settlements saved to: ${pending_file}"
    fi
}

# Cancel open orders
cancel_open_orders() {
    log_info "Cancelling all open orders"
    
    if curl -sf "${ORDER_BOOK_ENDPOINT}/orders/cancel-all" -X POST >/dev/null 2>&1; then
        log_success "All open orders cancelled"
    else
        log_error "Failed to cancel open orders"
    fi
}

# Save order book state
save_order_book_state() {
    log_info "Saving order book state"
    
    local state_file="${LOG_DIR}/orderbook-state-$(date +%Y%m%d-%H%M%S).json"
    
    if curl -sf "${ORDER_BOOK_ENDPOINT}/state" >/dev/null 2>&1; then
        curl -s "${ORDER_BOOK_ENDPOINT}/state" > "${state_file}"
        log_success "Order book state saved to: ${state_file}"
    else
        log_warn "Failed to save order book state"
    fi
}

# Perform final shutdown
perform_final_shutdown() {
    log_info "Performing final application shutdown"
    
    # Send shutdown signal to application
    if curl -sf "${ADMIN_ENDPOINT}/shutdown/execute" -X POST >/dev/null 2>&1; then
        log_success "Shutdown signal sent via API"
    else
        log_warn "Failed to send shutdown via API, using signal method"
        if [[ -n "${MAIN_PID:-}" ]]; then
            kill -TERM "${MAIN_PID}" 2>/dev/null || true
        fi
    fi
    
    # Wait for graceful shutdown
    local start_time=$(date +%s)
    local timeout_time=$((start_time + FINAL_SHUTDOWN_TIMEOUT))
    
    while [[ $(date +%s) -lt $timeout_time ]]; do
        if ! curl -sf "${HEALTH_ENDPOINT}" >/dev/null 2>&1; then
            log_success "Application has shut down gracefully"
            return 0
        fi
        
        sleep 1
    done
    
    log_warn "Graceful shutdown timeout reached"
    
    # Force shutdown if necessary
    if [[ -n "${MAIN_PID:-}" ]]; then
        log_warn "Sending SIGKILL to force shutdown"
        kill -KILL "${MAIN_PID}" 2>/dev/null || true
    fi
    
    return 1
}

# Cleanup function
cleanup_resources() {
    log_info "Cleaning up resources"
    
    # Close database connections
    if curl -sf "${ADMIN_ENDPOINT}/database/close" -X POST >/dev/null 2>&1; then
        log_success "Database connections closed"
    fi
    
    # Close Redis connections
    if curl -sf "${ADMIN_ENDPOINT}/redis/close" -X POST >/dev/null 2>&1; then
        log_success "Redis connections closed"
    fi
    
    # Cleanup temporary files
    find /tmp -name "pincex-*" -type f -mmin +60 -delete 2>/dev/null || true
    
    log_success "Resource cleanup completed"
}

# Main graceful shutdown orchestration
main_shutdown() {
    log_info "Starting comprehensive graceful shutdown process"
    
    # Phase 1: Readiness check
    if ! check_shutdown_readiness; then
        log_error "Application not ready for shutdown"
        return 1
    fi
    
    # Phase 2: Stop accepting new connections
    stop_accepting_connections
    
    # Phase 3: Handle trading-specific procedures
    handle_trading_shutdown
    
    # Phase 4: Wait for connection draining
    if ! wait_for_connection_drain; then
        log_warn "Connection draining completed with warnings"
    fi
    
    # Phase 5: Final shutdown
    if ! perform_final_shutdown; then
        log_error "Final shutdown completed with errors"
        return 1
    fi
    
    # Phase 6: Cleanup
    cleanup_resources
    
    log_success "Graceful shutdown process completed successfully"
    return 0
}

# Emergency shutdown (force)
emergency_shutdown() {
    log_warn "EMERGENCY SHUTDOWN INITIATED"
    
    # Immediate stop of new connections
    stop_accepting_connections
    
    # Force close trading operations
    curl -sf "${TRADING_ENGINE_ENDPOINT}/emergency-stop" -X POST >/dev/null 2>&1 || true
    
    # Quick settlement attempt
    curl -sf "${SETTLEMENT_ENDPOINT}/force-settle" -X POST >/dev/null 2>&1 || true
    
    # Force shutdown
    if [[ -n "${MAIN_PID:-}" ]]; then
        kill -KILL "${MAIN_PID}" 2>/dev/null || true
    fi
    
    log_warn "Emergency shutdown completed"
}

# Signal handlers
trap 'log_info "Graceful shutdown interrupted"; emergency_shutdown; exit 1' INT
trap 'log_info "Graceful shutdown terminated"; emergency_shutdown; exit 1' TERM
trap cleanup_resources EXIT

# Main execution
main() {
    local action="${1:-graceful}"
    
    setup_logging
    
    # Set main PID for signal handling
    export MAIN_PID=$(<"${PIDFILE:-/tmp/pincex.pid}" 2>/dev/null || echo "")
    
    case "$action" in
        "graceful")
            main_shutdown
            ;;
        "emergency")
            emergency_shutdown
            ;;
        "status")
            display_connection_status
            ;;
        "test-drain")
            stop_accepting_connections
            wait_for_connection_drain
            ;;
        *)
            echo "Usage: $0 {graceful|emergency|status|test-drain}"
            echo "  graceful  - Perform full graceful shutdown"
            echo "  emergency - Force immediate shutdown"
            echo "  status    - Display current connection status"
            echo "  test-drain - Test connection draining only"
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"
