#!/bin/bash

# PinCEX Unified Exchange - Advanced ML Model Deployment with Shadow Testing
# Zero-downtime model updates with comprehensive A/B testing and rollback capabilities

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SCRIPT_DIR}/../configs"
LOG_DIR="${SCRIPT_DIR}/../logs"
MODEL_DIR="${SCRIPT_DIR}/../models"
DEPLOYMENT_LOG="${LOG_DIR}/model-deployment-$(date +%Y%m%d-%H%M%S).log"

# Model configuration
MODEL_REGISTRY="s3://pincex-ml-models"
MODEL_CACHE_DIR="/opt/pincex/models"
SHADOW_MODEL_DIR="/opt/pincex/shadow-models"
BACKUP_MODEL_DIR="/opt/pincex/backup-models"

# Application endpoints
ML_SERVICE_ENDPOINT="http://localhost:8080/ml"
ADMIN_ENDPOINT="http://localhost:8080/admin"
METRICS_ENDPOINT="http://localhost:8080/metrics"
SHADOW_ENDPOINT="http://localhost:8080/shadow"

# Testing configuration
SHADOW_TRAFFIC_PERCENTAGE=10
CANARY_TRAFFIC_PERCENTAGE=5
AB_TEST_DURATION=3600  # 1 hour
PERFORMANCE_THRESHOLD=0.95
ACCURACY_THRESHOLD=0.92
LATENCY_THRESHOLD_MS=100

# Model types
SUPPORTED_MODELS=(
    "price-prediction"
    "risk-assessment"
    "fraud-detection"
    "market-analysis"
    "orderbook-prediction"
    "volatility-forecasting"
)

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
    echo -e "${timestamp} [${level}] ${message}" | tee -a "${DEPLOYMENT_LOG}"
}

log_info() { log "INFO" "$@"; }
log_warn() { log "WARN" "$@"; }
log_error() { log "ERROR" "$@"; }
log_success() { log "SUCCESS" "$@"; }
log_debug() { log "DEBUG" "$@"; }

# Setup logging and directories
setup_environment() {
    mkdir -p "${LOG_DIR}" "${MODEL_CACHE_DIR}" "${SHADOW_MODEL_DIR}" "${BACKUP_MODEL_DIR}"
    log_info "ML Model deployment environment initialized"
    log_info "Deployment log: ${DEPLOYMENT_LOG}"
}

# Download model from registry
download_model() {
    local model_name=$1
    local model_version=$2
    local target_dir=$3
    
    log_info "Downloading model: ${model_name} version ${model_version}"
    
    local model_path="${MODEL_REGISTRY}/${model_name}/${model_version}"
    local local_path="${target_dir}/${model_name}-${model_version}"
    
    # Create model directory
    mkdir -p "${local_path}"
    
    # Download model files
    if command -v aws >/dev/null; then
        if aws s3 sync "${model_path}" "${local_path}" --quiet; then
            log_success "Model downloaded successfully to ${local_path}"
            
            # Verify model integrity
            if verify_model_integrity "${local_path}"; then
                echo "${local_path}"
                return 0
            else
                log_error "Model integrity verification failed"
                return 1
            fi
        else
            log_error "Failed to download model from S3"
            return 1
        fi
    else
        log_error "AWS CLI not available for model download"
        return 1
    fi
}

# Verify model integrity
verify_model_integrity() {
    local model_path=$1
    
    log_info "Verifying model integrity: ${model_path}"
    
    # Check for required files
    local required_files=("model.pkl" "metadata.json" "requirements.txt")
    for file in "${required_files[@]}"; do
        if [[ ! -f "${model_path}/${file}" ]]; then
            log_error "Required model file missing: ${file}"
            return 1
        fi
    done
    
    # Verify checksums if available
    if [[ -f "${model_path}/checksums.sha256" ]]; then
        if ! (cd "${model_path}" && sha256sum -c checksums.sha256 --quiet); then
            log_error "Model checksum verification failed"
            return 1
        fi
    fi
    
    # Validate metadata
    if ! python3 -c "
import json
import sys
try:
    with open('${model_path}/metadata.json') as f:
        metadata = json.load(f)
    required_fields = ['name', 'version', 'type', 'training_date', 'accuracy']
    for field in required_fields:
        if field not in metadata:
            print(f'Missing required metadata field: {field}')
            sys.exit(1)
    print('Metadata validation passed')
except Exception as e:
    print(f'Metadata validation failed: {e}')
    sys.exit(1)
"; then
        log_error "Model metadata validation failed"
        return 1
    fi
    
    log_success "Model integrity verification passed"
    return 0
}

# Get current model information
get_current_model_info() {
    local model_type=$1
    
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/info" >/dev/null 2>&1; then
        curl -s "${ML_SERVICE_ENDPOINT}/models/${model_type}/info"
    else
        echo "{\"version\": \"unknown\", \"loaded_at\": \"unknown\"}"
    fi
}

# Load model into shadow environment
load_shadow_model() {
    local model_type=$1
    local model_path=$2
    
    log_info "Loading model into shadow environment: ${model_type}"
    
    # Copy model to shadow directory
    local shadow_path="${SHADOW_MODEL_DIR}/${model_type}"
    rm -rf "${shadow_path}"
    cp -r "${model_path}" "${shadow_path}"
    
    # Load model via API
    if curl -sf "${SHADOW_ENDPOINT}/models/${model_type}/load" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"model_path\": \"${shadow_path}\"}" >/dev/null 2>&1; then
        
        log_success "Model loaded into shadow environment"
        return 0
    else
        log_error "Failed to load model into shadow environment"
        return 1
    fi
}

# Start shadow testing
start_shadow_testing() {
    local model_type=$1
    local traffic_percentage=$2
    
    log_info "Starting shadow testing for ${model_type} with ${traffic_percentage}% traffic"
    
    # Configure shadow traffic routing
    if curl -sf "${SHADOW_ENDPOINT}/traffic/configure" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{
            \"model_type\": \"${model_type}\", 
            \"percentage\": ${traffic_percentage},
            \"enabled\": true
        }" >/dev/null 2>&1; then
        
        log_success "Shadow testing configured successfully"
        return 0
    else
        log_error "Failed to configure shadow testing"
        return 1
    fi
}

# Monitor shadow testing metrics
monitor_shadow_testing() {
    local model_type=$1
    local duration=$2
    
    log_info "Monitoring shadow testing for ${model_type} (duration: ${duration}s)"
    
    local start_time=$(date +%s)
    local end_time=$((start_time + duration))
    local check_interval=60
    
    local metrics_file="${LOG_DIR}/shadow-metrics-${model_type}-$(date +%Y%m%d-%H%M%S).json"
    
    while [[ $(date +%s) -lt $end_time ]]; do
        # Collect metrics
        if curl -sf "${SHADOW_ENDPOINT}/metrics/${model_type}" >/dev/null 2>&1; then
            local metrics=$(curl -s "${SHADOW_ENDPOINT}/metrics/${model_type}")
            echo "${metrics}" >> "${metrics_file}"
            
            # Parse key metrics
            local accuracy=$(echo "${metrics}" | jq -r '.accuracy // 0')
            local latency=$(echo "${metrics}" | jq -r '.avg_latency_ms // 0')
            local error_rate=$(echo "${metrics}" | jq -r '.error_rate // 0')
            
            log_info "Shadow metrics - Accuracy: ${accuracy}, Latency: ${latency}ms, Error Rate: ${error_rate}"
            
            # Check if metrics meet thresholds
            if (( $(echo "${accuracy} < ${ACCURACY_THRESHOLD}" | bc -l) )); then
                log_warn "Shadow model accuracy below threshold: ${accuracy} < ${ACCURACY_THRESHOLD}"
            fi
            
            if (( $(echo "${latency} > ${LATENCY_THRESHOLD_MS}" | bc -l) )); then
                log_warn "Shadow model latency above threshold: ${latency}ms > ${LATENCY_THRESHOLD_MS}ms"
            fi
        else
            log_warn "Failed to collect shadow testing metrics"
        fi
        
        sleep $check_interval
    done
    
    log_success "Shadow testing monitoring completed. Metrics saved to: ${metrics_file}"
    echo "${metrics_file}"
}

# Analyze shadow testing results
analyze_shadow_results() {
    local model_type=$1
    local metrics_file=$2
    
    log_info "Analyzing shadow testing results for ${model_type}"
    
    # Python script for analysis
    local analysis_result=$(python3 << EOF
import json
import sys
from statistics import mean

metrics = []
try:
    with open('${metrics_file}', 'r') as f:
        for line in f:
            if line.strip():
                metrics.append(json.loads(line))
except Exception as e:
    print(f"Error reading metrics: {e}")
    sys.exit(1)

if not metrics:
    print("No metrics data available")
    sys.exit(1)

# Calculate averages
avg_accuracy = mean([m.get('accuracy', 0) for m in metrics])
avg_latency = mean([m.get('avg_latency_ms', 0) for m in metrics])
avg_error_rate = mean([m.get('error_rate', 0) for m in metrics])

# Performance score
performance_score = avg_accuracy * (1 - avg_error_rate) * min(1, ${LATENCY_THRESHOLD_MS} / max(avg_latency, 1))

result = {
    "avg_accuracy": avg_accuracy,
    "avg_latency": avg_latency,
    "avg_error_rate": avg_error_rate,
    "performance_score": performance_score,
    "meets_accuracy_threshold": avg_accuracy >= ${ACCURACY_THRESHOLD},
    "meets_latency_threshold": avg_latency <= ${LATENCY_THRESHOLD_MS},
    "meets_performance_threshold": performance_score >= ${PERFORMANCE_THRESHOLD}
}

print(json.dumps(result))
EOF
    )
    
    echo "${analysis_result}" | tee "${LOG_DIR}/shadow-analysis-${model_type}.json"
}

# Promote shadow model to production
promote_to_production() {
    local model_type=$1
    local shadow_path="${SHADOW_MODEL_DIR}/${model_type}"
    
    log_info "Promoting shadow model to production: ${model_type}"
    
    # Backup current production model
    backup_current_model "${model_type}"
    
    # Copy shadow model to production
    local production_path="${MODEL_CACHE_DIR}/${model_type}"
    rm -rf "${production_path}"
    cp -r "${shadow_path}" "${production_path}"
    
    # Load new model into production
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/reload" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"model_path\": \"${production_path}\"}" >/dev/null 2>&1; then
        
        log_success "Model promoted to production successfully"
        
        # Update model registry
        update_model_registry "${model_type}" "${production_path}"
        
        return 0
    else
        log_error "Failed to promote model to production"
        return 1
    fi
}

# Backup current production model
backup_current_model() {
    local model_type=$1
    local production_path="${MODEL_CACHE_DIR}/${model_type}"
    local backup_path="${BACKUP_MODEL_DIR}/${model_type}-$(date +%Y%m%d-%H%M%S)"
    
    if [[ -d "${production_path}" ]]; then
        log_info "Backing up current production model: ${model_type}"
        cp -r "${production_path}" "${backup_path}"
        log_success "Model backed up to: ${backup_path}"
    else
        log_warn "No current production model to backup for: ${model_type}"
    fi
}

# Update model registry
update_model_registry() {
    local model_type=$1
    local model_path=$2
    
    log_info "Updating model registry for ${model_type}"
    
    # Read model metadata
    local metadata_file="${model_path}/metadata.json"
    if [[ -f "${metadata_file}" ]]; then
        local version=$(python3 -c "
import json
with open('${metadata_file}') as f:
    print(json.load(f)['version'])
")
        
        # Update registry via API
        if curl -sf "${ADMIN_ENDPOINT}/models/registry/update" \
            -X POST \
            -H "Content-Type: application/json" \
            -d "{
                \"model_type\": \"${model_type}\",
                \"version\": \"${version}\",
                \"path\": \"${model_path}\",
                \"deployed_at\": \"$(date -Iseconds)\"
            }" >/dev/null 2>&1; then
            
            log_success "Model registry updated successfully"
        else
            log_warn "Failed to update model registry"
        fi
    fi
}

# Rollback to previous model
rollback_model() {
    local model_type=$1
    
    log_warn "Rolling back model: ${model_type}"
    
    # Find most recent backup
    local backup_path
    backup_path=$(find "${BACKUP_MODEL_DIR}" -name "${model_type}-*" -type d | sort -r | head -1)
    
    if [[ -z "${backup_path}" ]]; then
        log_error "No backup found for model: ${model_type}"
        return 1
    fi
    
    log_info "Rolling back to: ${backup_path}"
    
    # Restore backup to production
    local production_path="${MODEL_CACHE_DIR}/${model_type}"
    rm -rf "${production_path}"
    cp -r "${backup_path}" "${production_path}"
    
    # Reload model
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/reload" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"model_path\": \"${production_path}\"}" >/dev/null 2>&1; then
        
        log_success "Model rollback completed successfully"
        return 0
    else
        log_error "Failed to reload rolled back model"
        return 1
    fi
}

# Canary deployment
deploy_canary() {
    local model_type=$1
    local model_path=$2
    local traffic_percentage=$3
    
    log_info "Starting canary deployment for ${model_type} with ${traffic_percentage}% traffic"
    
    # Load model into canary slot
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/canary/load" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"model_path\": \"${model_path}\"}" >/dev/null 2>&1; then
        
        log_success "Model loaded into canary slot"
        
        # Configure traffic splitting
        if curl -sf "${ML_SERVICE_ENDPOINT}/traffic/split" \
            -X POST \
            -H "Content-Type: application/json" \
            -d "{
                \"model_type\": \"${model_type}\",
                \"canary_percentage\": ${traffic_percentage}
            }" >/dev/null 2>&1; then
            
            log_success "Canary traffic splitting configured"
            return 0
        else
            log_error "Failed to configure canary traffic splitting"
            return 1
        fi
    else
        log_error "Failed to load model into canary slot"
        return 1
    fi
}

# Main deployment orchestration
deploy_model() {
    local model_type=$1
    local model_version=$2
    local deployment_strategy="${3:-shadow}"
    
    log_info "Starting model deployment: ${model_type} version ${model_version} using ${deployment_strategy} strategy"
    
    # Validate model type
    if [[ ! " ${SUPPORTED_MODELS[*]} " =~ " ${model_type} " ]]; then
        log_error "Unsupported model type: ${model_type}"
        return 1
    fi
    
    # Download new model
    local model_path
    if ! model_path=$(download_model "${model_type}" "${model_version}" "${MODEL_CACHE_DIR}"); then
        log_error "Failed to download model"
        return 1
    fi
    
    case "${deployment_strategy}" in
        "shadow")
            deploy_with_shadow_testing "${model_type}" "${model_path}"
            ;;
        "canary")
            deploy_with_canary "${model_type}" "${model_path}"
            ;;
        "direct")
            deploy_direct "${model_type}" "${model_path}"
            ;;
        *)
            log_error "Unknown deployment strategy: ${deployment_strategy}"
            return 1
            ;;
    esac
}

# Deploy with shadow testing
deploy_with_shadow_testing() {
    local model_type=$1
    local model_path=$2
    
    log_info "Deploying ${model_type} with shadow testing"
    
    # Load into shadow environment
    if ! load_shadow_model "${model_type}" "${model_path}"; then
        return 1
    fi
    
    # Start shadow testing
    if ! start_shadow_testing "${model_type}" "${SHADOW_TRAFFIC_PERCENTAGE}"; then
        return 1
    fi
    
    # Monitor shadow testing
    local metrics_file
    metrics_file=$(monitor_shadow_testing "${model_type}" "${AB_TEST_DURATION}")
    
    # Analyze results
    local analysis
    analysis=$(analyze_shadow_results "${model_type}" "${metrics_file}")
    
    # Check if promotion criteria are met
    local meets_criteria
    meets_criteria=$(echo "${analysis}" | jq -r '.meets_performance_threshold and .meets_accuracy_threshold and .meets_latency_threshold')
    
    if [[ "${meets_criteria}" == "true" ]]; then
        log_success "Shadow testing passed all criteria - promoting to production"
        promote_to_production "${model_type}"
    else
        log_warn "Shadow testing failed criteria - deployment aborted"
        echo "${analysis}" | jq '.'
        return 1
    fi
}

# Deploy with canary
deploy_with_canary() {
    local model_type=$1
    local model_path=$2
    
    log_info "Deploying ${model_type} with canary strategy"
    
    # Start canary deployment
    if ! deploy_canary "${model_type}" "${model_path}" "${CANARY_TRAFFIC_PERCENTAGE}"; then
        return 1
    fi
    
    # Monitor canary metrics
    local metrics_file
    metrics_file=$(monitor_shadow_testing "${model_type}" "${AB_TEST_DURATION}")
    
    # Analyze canary results
    local analysis
    analysis=$(analyze_shadow_results "${model_type}" "${metrics_file}")
    
    # Decide on promotion
    local meets_criteria
    meets_criteria=$(echo "${analysis}" | jq -r '.meets_performance_threshold and .meets_accuracy_threshold and .meets_latency_threshold')
    
    if [[ "${meets_criteria}" == "true" ]]; then
        log_success "Canary testing passed - promoting to 100% traffic"
        promote_canary_to_production "${model_type}"
    else
        log_warn "Canary testing failed - rolling back"
        rollback_canary "${model_type}"
        return 1
    fi
}

# Deploy direct (no testing)
deploy_direct() {
    local model_type=$1
    local model_path=$2
    
    log_warn "Deploying ${model_type} directly to production (no testing)"
    
    backup_current_model "${model_type}"
    
    local production_path="${MODEL_CACHE_DIR}/${model_type}"
    rm -rf "${production_path}"
    cp -r "${model_path}" "${production_path}"
    
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/reload" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"model_path\": \"${production_path}\"}" >/dev/null 2>&1; then
        
        log_success "Direct deployment completed"
        update_model_registry "${model_type}" "${production_path}"
    else
        log_error "Direct deployment failed"
        return 1
    fi
}

# Promote canary to production
promote_canary_to_production() {
    local model_type=$1
    
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/canary/promote" \
        -X POST >/dev/null 2>&1; then
        
        log_success "Canary promoted to production"
    else
        log_error "Failed to promote canary to production"
        return 1
    fi
}

# Rollback canary
rollback_canary() {
    local model_type=$1
    
    if curl -sf "${ML_SERVICE_ENDPOINT}/models/${model_type}/canary/rollback" \
        -X POST >/dev/null 2>&1; then
        
        log_success "Canary rolled back"
    else
        log_error "Failed to rollback canary"
        return 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up temporary model files"
    # Remove temporary files older than 1 day
    find "${MODEL_CACHE_DIR}" "${SHADOW_MODEL_DIR}" -name "tmp-*" -type d -mtime +1 -exec rm -rf {} + 2>/dev/null || true
}

# Signal handlers
trap cleanup EXIT

# Main execution
main() {
    local action="${1:-deploy}"
    
    setup_environment
    
    case "$action" in
        "deploy")
            local model_type="${2:-}"
            local model_version="${3:-}"
            local strategy="${4:-shadow}"
            
            if [[ -z "$model_type" ]] || [[ -z "$model_version" ]]; then
                echo "Usage: $0 deploy <model_type> <model_version> [strategy]"
                echo "Strategies: shadow, canary, direct"
                exit 1
            fi
            
            deploy_model "$model_type" "$model_version" "$strategy"
            ;;
        "rollback")
            local model_type="${2:-}"
            
            if [[ -z "$model_type" ]]; then
                echo "Usage: $0 rollback <model_type>"
                exit 1
            fi
            
            rollback_model "$model_type"
            ;;
        "status")
            local model_type="${2:-all}"
            
            if [[ "$model_type" == "all" ]]; then
                for model in "${SUPPORTED_MODELS[@]}"; do
                    echo "Model: $model"
                    get_current_model_info "$model" | jq '.'
                    echo
                done
            else
                get_current_model_info "$model_type" | jq '.'
            fi
            ;;
        "test")
            local model_type="${2:-}"
            
            if [[ -z "$model_type" ]]; then
                echo "Usage: $0 test <model_type>"
                exit 1
            fi
            
            start_shadow_testing "$model_type" "$SHADOW_TRAFFIC_PERCENTAGE"
            ;;
        *)
            echo "Usage: $0 {deploy|rollback|status|test}"
            echo "  deploy <model_type> <version> [strategy] - Deploy new model"
            echo "  rollback <model_type>                    - Rollback to previous version"
            echo "  status [model_type]                      - Show model status"
            echo "  test <model_type>                        - Start shadow testing"
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"
