#!/bin/bash
# Automated Rollback Script for PinCEX Zero-Downtime Deployments
# This script monitors deployment health and automatically rolls back on failures

set -euo pipefail

# Configuration
NAMESPACE="pincex-prod"
DEPLOYMENT_NAME="pincex-core-api"
HEALTH_CHECK_URL="http://pincex-core-api-service/health/ready"
MAX_WAIT_TIME=600  # 10 minutes
CHECK_INTERVAL=10  # 10 seconds
FAILURE_THRESHOLD=3
LOG_FILE="/var/log/pincex-deployment.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Error handling
error_exit() {
    log "ERROR: $1"
    exit 1
}

# Check if kubectl is available and configured
check_prerequisites() {
    if ! command -v kubectl &> /dev/null; then
        error_exit "kubectl is not installed or not in PATH"
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        error_exit "kubectl is not configured properly"
    fi
    
    log "Prerequisites check passed"
}

# Get current deployment revision
get_current_revision() {
    kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}'
}

# Get previous revision for rollback
get_previous_revision() {
    local current_revision=$(get_current_revision)
    echo $((current_revision - 1))
}

# Check deployment status
check_deployment_status() {
    local status=$(kubectl rollout status deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout=60s 2>&1 || echo "failed")
    echo "$status"
}

# Perform health check
health_check() {
    local response_code=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_CHECK_URL" || echo "000")
    
    if [[ "$response_code" == "200" ]]; then
        return 0
    else
        return 1
    fi
}

# Advanced health verification
advanced_health_check() {
    log "Performing advanced health checks..."
    
    # Check pod readiness
    local ready_pods=$(kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}' | grep -o "True" | wc -l)
    local total_pods=$(kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" --no-headers | wc -l)
    
    log "Ready pods: $ready_pods/$total_pods"
    
    if [[ "$ready_pods" -eq 0 ]]; then
        return 1
    fi
    
    # Check application health endpoint
    if ! health_check; then
        return 1
    fi
    
    # Check critical metrics
    local error_rate=$(kubectl exec -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" -- curl -s http://localhost:9090/metrics | grep 'http_requests_total{status="5xx"}' | awk '{print $2}' | head -1 || echo "0")
    
    if [[ "${error_rate:-0}" -gt 10 ]]; then
        log "WARNING: High error rate detected: $error_rate"
        return 1
    fi
    
    return 0
}

# Monitor deployment progress
monitor_deployment() {
    local start_time=$(date +%s)
    local failure_count=0
    
    log "Starting deployment monitoring..."
    
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        
        if [[ $elapsed_time -gt $MAX_WAIT_TIME ]]; then
            log "Deployment monitoring timeout reached"
            return 1
        fi
        
        # Check deployment rollout status
        local rollout_status=$(check_deployment_status)
        
        if echo "$rollout_status" | grep -q "successfully rolled out"; then
            log "Deployment rollout completed successfully"
            break
        elif echo "$rollout_status" | grep -q "failed\|error"; then
            log "Deployment rollout failed: $rollout_status"
            return 1
        fi
        
        # Perform health checks
        if ! advanced_health_check; then
            failure_count=$((failure_count + 1))
            log "Health check failed (attempt $failure_count/$FAILURE_THRESHOLD)"
            
            if [[ $failure_count -ge $FAILURE_THRESHOLD ]]; then
                log "Health check failure threshold reached"
                return 1
            fi
        else
            failure_count=0  # Reset failure count on success
            log "Health check passed"
        fi
        
        sleep $CHECK_INTERVAL
    done
    
    # Final comprehensive health check
    log "Performing final health verification..."
    sleep 30  # Allow time for stabilization
    
    if ! advanced_health_check; then
        log "Final health check failed"
        return 1
    fi
    
    log "Deployment monitoring completed successfully"
    return 0
}

# Perform automatic rollback
perform_rollback() {
    local previous_revision=$(get_previous_revision)
    
    log "Initiating automatic rollback to revision $previous_revision..."
    
    # Perform rollback
    if kubectl rollout undo deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --to-revision="$previous_revision"; then
        log "Rollback command executed successfully"
    else
        error_exit "Failed to execute rollback command"
    fi
    
    # Wait for rollback to complete
    log "Waiting for rollback to complete..."
    
    if kubectl rollout status deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout=300s; then
        log "Rollback completed successfully"
    else
        error_exit "Rollback failed to complete within timeout"
    fi
    
    # Verify rollback health
    sleep 30
    if advanced_health_check; then
        log "Rollback health verification passed"
        return 0
    else
        error_exit "Rollback health verification failed"
    fi
}

# Send notification
send_notification() {
    local status="$1"
    local message="$2"
    
    # Slack notification
    if [[ -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        curl -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"PinCEX Deployment $status: $message\"}" \
            "$SLACK_WEBHOOK_URL" || log "Failed to send Slack notification"
    fi
    
    # Email notification (if configured)
    if [[ -n "${EMAIL_RECIPIENTS:-}" ]]; then
        echo "$message" | mail -s "PinCEX Deployment $status" "$EMAIL_RECIPIENTS" || log "Failed to send email notification"
    fi
    
    log "Notification sent: $status - $message"
}

# Main execution
main() {
    log "Starting PinCEX zero-downtime deployment monitor"
    
    check_prerequisites
    
    local current_revision=$(get_current_revision)
    log "Current deployment revision: $current_revision"
    
    if monitor_deployment; then
        log "Deployment completed successfully"
        send_notification "SUCCESS" "Deployment to revision $current_revision completed successfully"
        exit 0
    else
        log "Deployment failed, initiating automatic rollback"
        send_notification "FAILED" "Deployment to revision $current_revision failed, initiating rollback"
        
        if perform_rollback; then
            log "Automatic rollback completed successfully"
            send_notification "ROLLBACK_SUCCESS" "Automatic rollback completed successfully"
            exit 1  # Exit with error to indicate original deployment failed
        else
            error_exit "Automatic rollback failed - manual intervention required"
        fi
    fi
}

# Execute main function
main "$@"
