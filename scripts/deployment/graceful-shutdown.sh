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

error_exit() {
    log_error "$1"
    exit 1
}

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
    
    sleep "${PRE_SHUTDOWN_DELAY}"
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
        return 1
    else
        log_success "All settlements completed successfully"
        return 0
    fi
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

# Drain WebSocket connections gracefully
drain_websocket_connections() {
    local pod_name="$1"
    local start_time=$(date +%s)
    
    log "Starting WebSocket connection draining for $pod_name"
    
    # Send close notifications to WebSocket clients
    kubectl exec -n "$NAMESPACE" "$pod_name" -- curl -X POST http://localhost:8080/admin/websocket/graceful-close || {
        log "WARNING: Failed to initiate WebSocket graceful close for $pod_name"
    }
    
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        
        if [[ $elapsed_time -gt $WEBSOCKET_DRAIN_TIMEOUT ]]; then
            log "WebSocket drain timeout reached for $pod_name"
            break
        fi
        
        local ws_connections=$(get_websocket_connections "$pod_name")
        
        if [[ "$ws_connections" -eq 0 ]]; then
            log "All WebSocket connections drained from $pod_name"
            break
        fi
        
        log "WebSocket connections remaining on $pod_name: $ws_connections"
        sleep 10
    done
}

# Drain HTTP connections gracefully
drain_http_connections() {
    local pod_name="$1"
    local start_time=$(date +%s)
    
    log "Starting HTTP connection draining for $pod_name"
    
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        
        if [[ $elapsed_time -gt $DRAIN_TIMEOUT ]]; then
            log "HTTP drain timeout reached for $pod_name"
            break
        fi
        
        local connections=$(get_active_connections "$pod_name")
        
        if [[ "$connections" -le 2 ]]; then  # Allow for health check connections
            log "HTTP connections successfully drained from $pod_name"
            break
        fi
        
        log "HTTP connections remaining on $pod_name: $connections"
        sleep 5
    done
}

# Wait for trading sessions to complete
wait_trading_sessions() {
    local pod_name="$1"
    local start_time=$(date +%s)
    local max_wait=1800  # 30 minutes for trading sessions
    
    log "Waiting for active trading sessions to complete on $pod_name"
    
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        
        if [[ $elapsed_time -gt $max_wait ]]; then
            log "Trading session wait timeout reached for $pod_name"
            break
        fi
        
        local sessions=$(get_active_trading_sessions "$pod_name")
        
        if [[ "$sessions" -eq 0 ]]; then
            log "All trading sessions completed on $pod_name"
            break
        fi
        
        log "Active trading sessions on $pod_name: $sessions"
        sleep 30
    done
}

# Perform graceful shutdown of a single pod
graceful_shutdown_pod() {
    local pod_name="$1"
    
    log "Starting graceful shutdown process for pod: $pod_name"
    
    # Phase 1: Prepare for shutdown
    if ! prepare_pod_shutdown "$pod_name"; then
        log "Failed to prepare $pod_name for shutdown"
        return 1
    fi
    
    # Phase 2: Wait for trading sessions to complete
    wait_trading_sessions "$pod_name"
    
    # Phase 3: Drain WebSocket connections
    drain_websocket_connections "$pod_name"
    
    # Phase 4: Drain HTTP connections
    drain_http_connections "$pod_name"
    
    # Phase 5: Final verification
    local final_connections=$(get_active_connections "$pod_name")
    local final_ws=$(get_websocket_connections "$pod_name")
    local final_sessions=$(get_active_trading_sessions "$pod_name")
    
    log "Final connection state for $pod_name:"
    log "  HTTP connections: $final_connections"
    log "  WebSocket connections: $final_ws"
    log "  Trading sessions: $final_sessions"
    
    # Phase 6: Initiate shutdown
    log "Initiating shutdown signal for $pod_name"
    kubectl exec -n "$NAMESPACE" "$pod_name" -- curl -X POST http://localhost:8080/admin/shutdown/execute || {
        log "WARNING: Failed to execute shutdown for $pod_name"
    }
    
    log "Graceful shutdown process completed for pod: $pod_name"
    return 0
}

# Rolling graceful shutdown for deployment
rolling_graceful_shutdown() {
    log "Starting rolling graceful shutdown for deployment: $DEPLOYMENT_NAME"
    
    # Get all pods in the deployment
    local pods=($(kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" -o jsonpath='{.items[*].metadata.name}'))
    
    if [[ ${#pods[@]} -eq 0 ]]; then
        error_exit "No pods found for deployment: $DEPLOYMENT_NAME"
    fi
    
    log "Found ${#pods[@]} pods to shutdown gracefully: ${pods[*]}"
    
    # Shutdown pods one by one
    for pod in "${pods[@]}"; do
        # Check if pod is ready before shutting down
        local pod_ready=$(kubectl get pod "$pod" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
        
        if [[ "$pod_ready" == "True" ]]; then
            graceful_shutdown_pod "$pod"
            
            # Wait before proceeding to next pod
            sleep 60
        else
            log "Skipping pod $pod (not ready): $pod_ready"
        fi
    done
    
    log "Rolling graceful shutdown completed for deployment: $DEPLOYMENT_NAME"
}

# Create graceful shutdown ConfigMap
create_shutdown_config() {
    cat > /tmp/graceful-shutdown-config.yaml << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: graceful-shutdown-config
  namespace: pincex-prod
data:
  shutdown.sh: |
    #!/bin/bash
    # Graceful shutdown script for PinCEX pods
    
    set -euo pipefail
    
    log() {
        echo "$(date '+%Y-%m-%d %H:%M:%S') - SHUTDOWN: $1"
    }
    
    log "Received shutdown signal, starting graceful shutdown..."
    
    # Stop health check endpoint
    curl -X POST http://localhost:8080/admin/health/disable || true
    
    # Stop accepting new connections
    curl -X POST http://localhost:8080/admin/server/stop-accept || true
    
    # Notify WebSocket clients of impending shutdown
    curl -X POST http://localhost:8080/admin/websocket/shutdown-notify || true
    
    # Wait for connections to drain (up to 45 seconds)
    for i in {1..45}; do
        connections=$(curl -s http://localhost:8080/admin/connections/count || echo "0")
        if [[ "$connections" -le 1 ]]; then
            log "Connections drained successfully"
            break
        fi
        log "Waiting for $connections connections to drain... ($i/45)"
        sleep 1
    done
    
    # Gracefully close database connections
    curl -X POST http://localhost:8080/admin/database/close || true
    
    # Final cleanup
    curl -X POST http://localhost:8080/admin/cleanup || true
    
    log "Graceful shutdown completed, exiting..."
    exit 0
  
  preStop.sh: |
    #!/bin/bash
    # Pre-stop hook script
    
    echo "$(date '+%Y-%m-%d %H:%M:%S') - Pre-stop hook triggered"
    
    # Execute graceful shutdown
    /scripts/shutdown.sh
    
    # Additional wait to ensure cleanup
    sleep 10
EOF
    
    kubectl apply -f /tmp/graceful-shutdown-config.yaml
    log "Graceful shutdown ConfigMap created"
}

# Update deployment with graceful shutdown configuration
update_deployment_config() {
    log "Updating deployment with graceful shutdown configuration..."
    
    # Create patch for deployment
    cat > /tmp/deployment-patch.yaml << 'EOF'
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 90
      containers:
      - name: pincex-core
        lifecycle:
          preStop:
            exec:
              command:
              - /bin/bash
              - /scripts/preStop.sh
        volumeMounts:
        - name: shutdown-scripts
          mountPath: /scripts
      volumes:
      - name: shutdown-scripts
        configMap:
          name: graceful-shutdown-config
          defaultMode: 0755
EOF
    
    # Apply patch to deployment
    kubectl patch deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" --patch-file /tmp/deployment-patch.yaml
    
    log "Deployment updated with graceful shutdown configuration"
}

# Monitor graceful shutdown
monitor_shutdown() {
    local timeout="$1"
    local start_time=$(date +%s)
    
    log "Monitoring graceful shutdown process (timeout: ${timeout}s)"
    
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        
        if [[ $elapsed_time -gt $timeout ]]; then
            log "Shutdown monitoring timeout reached"
            break
        fi
        
        # Check if any pods are still terminating
        local terminating_pods=$(kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" --field-selector=status.phase=Terminating --no-headers 2>/dev/null | wc -l)
        
        if [[ "$terminating_pods" -eq 0 ]]; then
            log "All pods have shut down gracefully"
            break
        fi
        
        log "Pods still terminating: $terminating_pods"
        sleep 10
    done
}

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
            log_warn "EMERGENCY SHUTDOWN INITIATED"
            stop_accepting_connections
            curl -sf "${TRADING_ENGINE_ENDPOINT}/emergency-stop" -X POST >/dev/null 2>&1 || true
            curl -sf "${SETTLEMENT_ENDPOINT}/force-settle" -X POST >/dev/null 2>&1 || true
            if [[ -n "${MAIN_PID:-}" ]]; then
                kill -KILL "${MAIN_PID}" 2>/dev/null || true
            fi
            log_warn "Emergency shutdown completed"
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

# Signal handlers
trap 'log_info "Graceful shutdown interrupted"; exit 1' INT
trap 'log_info "Graceful shutdown terminated"; exit 1' TERM
trap cleanup_resources EXIT

# Execute main function with all arguments
main "$@"
