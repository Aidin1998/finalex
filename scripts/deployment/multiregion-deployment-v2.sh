#!/bin/bash

# PinCEX Unified Exchange - Multi-Region Zero-Downtime Deployment Automation
# Advanced deployment orchestration across multiple regions with failover capabilities

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SCRIPT_DIR}/../configs"
LOG_DIR="${SCRIPT_DIR}/../logs"
DEPLOYMENT_LOG="${LOG_DIR}/multiregion-deployment-$(date +%Y%m%d-%H%M%S).log"

# Regions configuration
REGIONS=(
    "us-east-1:primary"
    "us-west-2:secondary" 
    "eu-west-1:secondary"
    "ap-southeast-1:secondary"
)

# Application configuration
APP_NAME="pincex-unified-exchange"
NAMESPACE="pincex-production"
IMAGE_TAG="${IMAGE_TAG:-latest}"
CLUSTER_PREFIX="pincex-cluster"

# Health check configuration
HEALTH_CHECK_TIMEOUT=300
HEALTH_CHECK_INTERVAL=10
MAX_DEPLOYMENT_TIME=1800

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    local level=$1
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo -e "${timestamp} [${level}] ${message}" | tee -a "${DEPLOYMENT_LOG}"
}

log_info() { log "INFO" "$@"; }
log_warn() { log "WARN" "$@"; }
log_error() { log "ERROR" "$@"; }
log_success() { log "SUCCESS" "$@"; }

# Setup logging directory
setup_logging() {
    mkdir -p "${LOG_DIR}"
    log_info "Multi-region deployment started"
    log_info "Deployment log: ${DEPLOYMENT_LOG}"
}

# Health check function
check_application_health() {
    local region=$1
    local cluster_name="${CLUSTER_PREFIX}-${region}"
    local endpoint
    
    log_info "Checking application health in region: ${region}"
    
    # Get service endpoint
    endpoint=$(kubectl --context="${cluster_name}" get service "${APP_NAME}-service" \
        -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "")
    
    if [[ -z "$endpoint" ]]; then
        log_warn "Service endpoint not available in ${region}"
        return 1
    fi
    
    local start_time=$(date +%s)
    local timeout_time=$((start_time + HEALTH_CHECK_TIMEOUT))
    
    while [[ $(date +%s) -lt $timeout_time ]]; do
        if curl -sf "http://${endpoint}/health" >/dev/null 2>&1; then
            log_success "Application healthy in region: ${region}"
            return 0
        fi
        
        log_info "Waiting for application to become healthy in ${region}..."
        sleep "${HEALTH_CHECK_INTERVAL}"
    done
    
    log_error "Health check timeout in region: ${region}"
    return 1
}

# Traffic routing functions
enable_traffic_to_region() {
    local region=$1
    local cluster_name="${CLUSTER_PREFIX}-${region}"
    
    log_info "Enabling traffic to region: ${region}"
    
    # Update ingress controller to route traffic
    kubectl --context="${cluster_name}" patch ingress "${APP_NAME}-ingress" \
        -n "${NAMESPACE}" --type='merge' \
        -p='{"metadata":{"annotations":{"nginx.ingress.kubernetes.io/weight":"100"}}}'
    
    # Update DNS weights if using Route53
    if command -v aws >/dev/null; then
        aws route53 change-resource-record-sets \
            --hosted-zone-id "${HOSTED_ZONE_ID}" \
            --change-batch file://<(cat <<EOF
{
    "Changes": [{
        "Action": "UPSERT",
        "ResourceRecordSet": {
            "Name": "${APP_NAME}.${DOMAIN_NAME}",
            "Type": "A",
            "SetIdentifier": "${region}",
            "Weight": 100,
            "AliasTarget": {
                "DNSName": "$(kubectl --context="${cluster_name}" get ingress "${APP_NAME}-ingress" -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')",
                "EvaluateTargetHealth": true
            }
        }
    }]
}
EOF
        )
    fi
    
    log_success "Traffic enabled for region: ${region}"
}

disable_traffic_to_region() {
    local region=$1
    local cluster_name="${CLUSTER_PREFIX}-${region}"
    
    log_info "Disabling traffic to region: ${region}"
    
    # Update ingress controller to stop routing traffic
    kubectl --context="${cluster_name}" patch ingress "${APP_NAME}-ingress" \
        -n "${NAMESPACE}" --type='merge' \
        -p='{"metadata":{"annotations":{"nginx.ingress.kubernetes.io/weight":"0"}}}'
    
    # Update DNS weights
    if command -v aws >/dev/null; then
        aws route53 change-resource-record-sets \
            --hosted-zone-id "${HOSTED_ZONE_ID}" \
            --change-batch file://<(cat <<EOF
{
    "Changes": [{
        "Action": "UPSERT",
        "ResourceRecordSet": {
            "Name": "${APP_NAME}.${DOMAIN_NAME}",
            "Type": "A",
            "SetIdentifier": "${region}",
            "Weight": 0,
            "AliasTarget": {
                "DNSName": "$(kubectl --context="${cluster_name}" get ingress "${APP_NAME}-ingress" -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')",
                "EvaluateTargetHealth": true
            }
        }
    }]
}
EOF
        )
    fi
    
    log_success "Traffic disabled for region: ${region}"
}

# Wait for connection draining
wait_for_connection_draining() {
    local region=$1
    local drain_time=60
    
    log_info "Waiting for connection draining in region: ${region} (${drain_time}s)"
    
    local countdown=$drain_time
    while [[ $countdown -gt 0 ]]; do
        echo -ne "\rDraining connections... ${countdown}s remaining"
        sleep 1
        ((countdown--))
    done
    echo ""
    
    log_success "Connection draining completed for region: ${region}"
}

# Deploy to single region
deploy_to_region() {
    local region=$1
    local region_type=$2
    local cluster_name="${CLUSTER_PREFIX}-${region}"
    
    log_info "Starting deployment to region: ${region} (${region_type})"
    
    # Verify kubectl context exists
    if ! kubectl config get-contexts "${cluster_name}" >/dev/null 2>&1; then
        log_error "Kubectl context '${cluster_name}' not found"
        return 1
    fi
    
    # Disable traffic to this region during deployment
    if [[ "${region_type}" != "primary" ]]; then
        disable_traffic_to_region "${region}"
        wait_for_connection_draining "${region}"
    fi
    
    # Apply deployment
    log_info "Applying Kubernetes manifests to ${region}"
    
    # Set image tag in deployment
    envsubst < "${CONFIG_DIR}/zero-downtime-deployment.yaml" | \
        kubectl --context="${cluster_name}" apply -n "${NAMESPACE}" -f -
    
    # Wait for rollout to complete
    log_info "Waiting for rollout to complete in ${region}"
    if ! kubectl --context="${cluster_name}" rollout status deployment/"${APP_NAME}" \
        -n "${NAMESPACE}" --timeout="${MAX_DEPLOYMENT_TIME}s"; then
        log_error "Deployment rollout failed in region: ${region}"
        return 1
    fi
    
    # Health check
    if ! check_application_health "${region}"; then
        log_error "Health check failed in region: ${region}"
        return 1
    fi
    
    # Re-enable traffic
    enable_traffic_to_region "${region}"
    
    log_success "Deployment completed successfully in region: ${region}"
    return 0
}

# Rollback function
rollback_region() {
    local region=$1
    local cluster_name="${CLUSTER_PREFIX}-${region}"
    
    log_warn "Rolling back deployment in region: ${region}"
    
    # Disable traffic
    disable_traffic_to_region "${region}"
    
    # Rollback deployment
    kubectl --context="${cluster_name}" rollout undo deployment/"${APP_NAME}" \
        -n "${NAMESPACE}"
    
    # Wait for rollback
    kubectl --context="${cluster_name}" rollout status deployment/"${APP_NAME}" \
        -n "${NAMESPACE}" --timeout=300s
    
    # Health check after rollback
    if check_application_health "${region}"; then
        enable_traffic_to_region "${region}"
        log_success "Rollback completed successfully in region: ${region}"
    else
        log_error "Rollback health check failed in region: ${region}"
    fi
}

# Pre-deployment validation
validate_prerequisites() {
    log_info "Validating deployment prerequisites"
    
    # Check required tools
    local required_tools=("kubectl" "envsubst" "curl")
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" >/dev/null; then
            log_error "Required tool not found: $tool"
            return 1
        fi
    done
    
    # Check kubectl contexts
    for region_config in "${REGIONS[@]}"; do
        local region="${region_config%:*}"
        local cluster_name="${CLUSTER_PREFIX}-${region}"
        
        if ! kubectl config get-contexts "${cluster_name}" >/dev/null 2>&1; then
            log_error "Kubectl context not found: ${cluster_name}"
            return 1
        fi
        
        # Test cluster connectivity
        if ! kubectl --context="${cluster_name}" cluster-info >/dev/null 2>&1; then
            log_error "Cannot connect to cluster: ${cluster_name}"
            return 1
        fi
    done
    
    # Check if namespace exists
    for region_config in "${REGIONS[@]}"; do
        local region="${region_config%:*}"
        local cluster_name="${CLUSTER_PREFIX}-${region}"
        
        if ! kubectl --context="${cluster_name}" get namespace "${NAMESPACE}" >/dev/null 2>&1; then
            log_info "Creating namespace ${NAMESPACE} in ${region}"
            kubectl --context="${cluster_name}" create namespace "${NAMESPACE}"
        fi
    done
    
    log_success "Prerequisites validation completed"
}

# Main deployment orchestration
main_deployment() {
    local failed_regions=()
    
    log_info "Starting multi-region deployment orchestration"
    
    # Phase 1: Deploy to secondary regions first
    log_info "Phase 1: Deploying to secondary regions"
    for region_config in "${REGIONS[@]}"; do
        local region="${region_config%:*}"
        local region_type="${region_config#*:}"
        
        if [[ "${region_type}" == "secondary" ]]; then
            if ! deploy_to_region "${region}" "${region_type}"; then
                failed_regions+=("${region}")
                log_error "Deployment failed in secondary region: ${region}"
            fi
        fi
    done
    
    # Check if any secondary deployments failed
    if [[ ${#failed_regions[@]} -gt 0 ]]; then
        log_error "Secondary region deployments failed: ${failed_regions[*]}"
        log_info "Consider fixing issues before proceeding to primary region"
        return 1
    fi
    
    # Phase 2: Deploy to primary region
    log_info "Phase 2: Deploying to primary region"
    for region_config in "${REGIONS[@]}"; do
        local region="${region_config%:*}"
        local region_type="${region_config#*:}"
        
        if [[ "${region_type}" == "primary" ]]; then
            if ! deploy_to_region "${region}" "${region_type}"; then
                log_error "Primary region deployment failed: ${region}"
                log_info "Initiating emergency rollback procedures"
                
                # Rollback primary region
                rollback_region "${region}"
                
                return 1
            fi
        fi
    done
    
    log_success "Multi-region deployment completed successfully"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up temporary files"
    # Add any cleanup logic here
}

# Signal handlers
trap cleanup EXIT
trap 'log_error "Deployment interrupted"; exit 1' INT TERM

# Main execution
main() {
    local action="${1:-deploy}"
    
    setup_logging
    
    case "$action" in
        "deploy")
            validate_prerequisites
            main_deployment
            ;;
        "rollback")
            local region="${2:-all}"
            if [[ "$region" == "all" ]]; then
                for region_config in "${REGIONS[@]}"; do
                    rollback_region "${region_config%:*}"
                done
            else
                rollback_region "$region"
            fi
            ;;
        "health-check")
            local region="${2:-all}"
            if [[ "$region" == "all" ]]; then
                for region_config in "${REGIONS[@]}"; do
                    check_application_health "${region_config%:*}"
                done
            else
                check_application_health "$region"
            fi
            ;;
        *)
            echo "Usage: $0 {deploy|rollback [region]|health-check [region]}"
            echo "Example: $0 deploy"
            echo "Example: $0 rollback us-east-1"
            echo "Example: $0 health-check all"
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"
