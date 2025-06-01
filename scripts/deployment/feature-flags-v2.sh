#!/bin/bash

# PinCEX Unified Exchange - Advanced Feature Flags and Dynamic Configuration Management
# Comprehensive feature flag system with A/B testing and gradual rollouts

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SCRIPT_DIR}/../configs"
LOG_DIR="${SCRIPT_DIR}/../logs"
FEATURE_LOG="${LOG_DIR}/feature-flags-$(date +%Y%m%d-%H%M%S).log"

# Application configuration
APP_NAME="pincex-unified-exchange"
NAMESPACE="pincex-production"
CONFIG_MAP_NAME="pincex-feature-flags"
REDIS_CONFIG_KEY="pincex:feature-flags"

# Feature flag endpoints
ADMIN_ENDPOINT="http://localhost:8080/admin"
FEATURE_ENDPOINT="http://localhost:8080/features"
METRICS_ENDPOINT="http://localhost:8080/metrics"

# Deployment configuration
ROLLOUT_STRATEGIES=("immediate" "canary" "gradual" "blue-green")
ENVIRONMENTS=("development" "staging" "production")

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
    echo -e "${timestamp} [${level}] ${message}" | tee -a "${FEATURE_LOG}"
}

log_info() { log "INFO" "$@"; }
log_warn() { log "WARN" "$@"; }
log_error() { log "ERROR" "$@"; }
log_success() { log "SUCCESS" "$@"; }

# Setup logging
setup_logging() {
    mkdir -p "${LOG_DIR}"
    log_info "Feature flags management system initialized"
    log_info "Feature log: ${FEATURE_LOG}"
}

# Create feature flag configuration
create_feature_config() {
    local config_file="${CONFIG_DIR}/feature-flags-config.yaml"
    
    log_info "Creating comprehensive feature flag configuration"
    
    cat > "${config_file}" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: pincex-feature-flags
  namespace: pincex-production
  labels:
    app: pincex-exchange
    component: feature-flags
data:
  feature-flags.json: |
    {
      "features": {
        "new_order_engine": {
          "enabled": false,
          "description": "New high-performance order matching engine",
          "rollout_strategy": "canary",
          "rollout_percentage": 0,
          "target_percentage": 100,
          "environments": ["development", "staging", "production"],
          "user_segments": ["beta_users", "high_volume_traders"],
          "created_at": "2025-06-01T00:00:00Z",
          "updated_at": "2025-06-01T00:00:00Z",
          "metadata": {
            "owner": "trading-team",
            "jira_ticket": "TRADE-1234",
            "rollback_threshold": 0.05
          }
        },
        "advanced_risk_management": {
          "enabled": true,
          "description": "Enhanced risk management with ML-based detection",
          "rollout_strategy": "gradual",
          "rollout_percentage": 50,
          "target_percentage": 100,
          "environments": ["staging", "production"],
          "user_segments": ["institutional_traders"],
          "created_at": "2025-05-15T00:00:00Z",
          "updated_at": "2025-06-01T00:00:00Z",
          "metadata": {
            "owner": "risk-team",
            "jira_ticket": "RISK-5678",
            "rollback_threshold": 0.02
          }
        },
        "websocket_v2": {
          "enabled": true,
          "description": "WebSocket API v2 with improved performance",
          "rollout_strategy": "blue-green",
          "rollout_percentage": 100,
          "target_percentage": 100,
          "environments": ["production"],
          "user_segments": ["all"],
          "created_at": "2025-04-01T00:00:00Z",
          "updated_at": "2025-05-20T00:00:00Z",
          "metadata": {
            "owner": "api-team",
            "jira_ticket": "API-9999",
            "rollback_threshold": 0.01
          }
        },
        "ml_price_prediction": {
          "enabled": false,
          "description": "ML-based price prediction for trading recommendations",
          "rollout_strategy": "canary",
          "rollout_percentage": 0,
          "target_percentage": 25,
          "environments": ["development"],
          "user_segments": ["beta_users"],
          "created_at": "2025-06-01T00:00:00Z",
          "updated_at": "2025-06-01T00:00:00Z",
          "metadata": {
            "owner": "ml-team",
            "jira_ticket": "ML-3456",
            "rollback_threshold": 0.10
          }
        },
        "enhanced_kyc": {
          "enabled": true,
          "description": "Enhanced KYC with document verification",
          "rollout_strategy": "immediate",
          "rollout_percentage": 100,
          "target_percentage": 100,
          "environments": ["production"],
          "user_segments": ["all"],
          "created_at": "2025-03-01T00:00:00Z",
          "updated_at": "2025-03-15T00:00:00Z",
          "metadata": {
            "owner": "compliance-team",
            "jira_ticket": "KYC-7890",
            "rollback_threshold": 0.001
          }
        }
      },
      "global_settings": {
        "feature_flag_refresh_interval": 30,
        "metrics_collection_enabled": true,
        "audit_logging_enabled": true,
        "emergency_rollback_enabled": true,
        "a_b_testing_enabled": true
      },
      "user_segments": {
        "beta_users": {
          "description": "Users who opted into beta features",
          "criteria": {
            "user_type": "beta",
            "account_age_days": 30
          }
        },
        "high_volume_traders": {
          "description": "Users with high trading volume",
          "criteria": {
            "monthly_volume_usd": 100000,
            "trade_count_monthly": 1000
          }
        },
        "institutional_traders": {
          "description": "Institutional trading accounts",
          "criteria": {
            "account_type": "institutional",
            "kyc_level": "enhanced"
          }
        },
        "all": {
          "description": "All users",
          "criteria": {}
        }
      }
    }
  
  redis-config.lua: |
    -- Redis Lua script for atomic feature flag operations
    local function get_feature_flag(feature_name, user_id, user_segment)
        local flags_key = "pincex:feature-flags"
        local user_key = "pincex:user-segments:" .. user_id
        
        -- Get feature configuration
        local feature_config = redis.call('HGET', flags_key, feature_name)
        if not feature_config then
            return false
        end
        
        local config = cjson.decode(feature_config)
        
        -- Check if feature is globally enabled
        if not config.enabled then
            return false
        end
        
        -- Check environment
        local current_env = redis.call('GET', 'pincex:environment')
        if current_env and config.environments then
            local env_allowed = false
            for _, env in ipairs(config.environments) do
                if env == current_env then
                    env_allowed = true
                    break
                end
            end
            if not env_allowed then
                return false
            end
        end
        
        -- Check user segment
        if config.user_segments and #config.user_segments > 0 then
            local user_segment = redis.call('GET', user_key)
            if not user_segment then
                return false
            end
            
            local segment_allowed = false
            for _, segment in ipairs(config.user_segments) do
                if segment == user_segment or segment == "all" then
                    segment_allowed = true
                    break
                end
            end
            if not segment_allowed then
                return false
            end
        end
        
        -- Check rollout percentage
        if config.rollout_percentage < 100 then
            local user_hash = redis.call('GET', 'pincex:user-hash:' .. user_id)
            if not user_hash then
                -- Generate consistent hash for user
                user_hash = tonumber(string.sub(redis.sha1hex(user_id), 1, 8), 16) % 100
                redis.call('SET', 'pincex:user-hash:' .. user_id, user_hash, 'EX', 86400)
            end
            
            if tonumber(user_hash) >= config.rollout_percentage then
                return false
            end
        end
        
        return true
    end

EOF
    
    log_success "Feature flag configuration created: ${config_file}"
}

# Deploy feature flag configuration
deploy_feature_config() {
    local config_file="${CONFIG_DIR}/feature-flags-config.yaml"
    
    log_info "Deploying feature flag configuration to Kubernetes"
    
    if [[ ! -f "$config_file" ]]; then
        log_error "Configuration file not found: $config_file"
        return 1
    fi
    
    # Apply configuration to Kubernetes
    kubectl apply -f "$config_file" -n "$NAMESPACE"
    
    # Verify deployment
    if kubectl get configmap "$CONFIG_MAP_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
        log_success "Feature flag ConfigMap deployed successfully"
    else
        log_error "Failed to deploy feature flag ConfigMap"
        return 1
    fi
    
    # Update Redis with feature flags
    update_redis_feature_flags
    
    log_success "Feature flag configuration deployed successfully"
}

# Update Redis with feature flags
update_redis_feature_flags() {
    log_info "Updating Redis with feature flag configuration"
    
    local feature_config=$(kubectl get configmap "$CONFIG_MAP_NAME" -n "$NAMESPACE" -o jsonpath='{.data.feature-flags\.json}')
    
    # Update Redis (assumes Redis is accessible)
    if command -v redis-cli >/dev/null; then
        echo "$feature_config" | redis-cli -x SET "$REDIS_CONFIG_KEY"
        log_success "Redis updated with feature flags"
    else
        log_warn "redis-cli not available, skipping Redis update"
    fi
}

# List all feature flags
list_feature_flags() {
    log_info "Listing all feature flags"
    
    local feature_config=$(kubectl get configmap "$CONFIG_MAP_NAME" -n "$NAMESPACE" -o jsonpath='{.data.feature-flags\.json}' 2>/dev/null)
    
    if [[ -z "$feature_config" ]]; then
        log_error "No feature flag configuration found"
        return 1
    fi
    
    echo "$feature_config" | jq -r '
        .features | to_entries[] |
        "\(.key): \(.value.enabled) (\(.value.rollout_percentage)%) - \(.value.description)"
    ' | while read -r line; do
        log_info "$line"
    done
}

# Enable feature flag
enable_feature() {
    local feature_name="$1"
    local rollout_percentage="${2:-100}"
    local environment="${3:-production}"
    
    log_info "Enabling feature flag: $feature_name ($rollout_percentage% in $environment)"
    
    # Update via API if available
    if curl -sf "$FEATURE_ENDPOINT/enable" >/dev/null 2>&1; then
        local response=$(curl -s -X POST "$FEATURE_ENDPOINT/enable" \
            -H "Content-Type: application/json" \
            -d "{
                \"feature_name\": \"$feature_name\",
                \"rollout_percentage\": $rollout_percentage,
                \"environment\": \"$environment\"
            }")
        
        if echo "$response" | jq -e '.success' >/dev/null 2>&1; then
            log_success "Feature flag enabled via API: $feature_name"
        else
            log_error "Failed to enable feature flag via API: $feature_name"
            return 1
        fi
    else
        # Update ConfigMap directly
        update_feature_in_configmap "$feature_name" "enabled" "true"
        update_feature_in_configmap "$feature_name" "rollout_percentage" "$rollout_percentage"
    fi
    
    # Trigger configuration reload
    trigger_config_reload
    
    log_success "Feature flag enabled: $feature_name"
}

# Disable feature flag
disable_feature() {
    local feature_name="$1"
    local environment="${2:-production}"
    
    log_info "Disabling feature flag: $feature_name in $environment"
    
    # Update via API if available
    if curl -sf "$FEATURE_ENDPOINT/disable" >/dev/null 2>&1; then
        local response=$(curl -s -X POST "$FEATURE_ENDPOINT/disable" \
            -H "Content-Type: application/json" \
            -d "{
                \"feature_name\": \"$feature_name\",
                \"environment\": \"$environment\"
            }")
        
        if echo "$response" | jq -e '.success' >/dev/null 2>&1; then
            log_success "Feature flag disabled via API: $feature_name"
        else
            log_error "Failed to disable feature flag via API: $feature_name"
            return 1
        fi
    else
        # Update ConfigMap directly
        update_feature_in_configmap "$feature_name" "enabled" "false"
        update_feature_in_configmap "$feature_name" "rollout_percentage" "0"
    fi
    
    # Trigger configuration reload
    trigger_config_reload
    
    log_success "Feature flag disabled: $feature_name"
}

# Update feature in ConfigMap
update_feature_in_configmap() {
    local feature_name="$1"
    local field="$2"
    local value="$3"
    
    # Get current config
    local current_config=$(kubectl get configmap "$CONFIG_MAP_NAME" -n "$NAMESPACE" -o jsonpath='{.data.feature-flags\.json}')
    
    # Update the specific field
    local updated_config=$(echo "$current_config" | jq \
        --arg feature "$feature_name" \
        --arg field "$field" \
        --arg value "$value" \
        '.features[$feature][$field] = ($value | if . == "true" then true elif . == "false" then false else tonumber? // . end)')
    
    # Create temporary file with updated config
    local temp_file=$(mktemp)
    cat > "$temp_file" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $CONFIG_MAP_NAME
  namespace: $NAMESPACE
data:
  feature-flags.json: |
$(echo "$updated_config" | sed 's/^/    /')
EOF
    
    # Apply updated configuration
    kubectl apply -f "$temp_file"
    rm -f "$temp_file"
    
    log_info "Updated $feature_name.$field = $value in ConfigMap"
}

# Gradual rollout
gradual_rollout() {
    local feature_name="$1"
    local start_percentage="${2:-0}"
    local end_percentage="${3:-100}"
    local step_size="${4:-10}"
    local step_interval="${5:-300}" # 5 minutes
    
    log_info "Starting gradual rollout for $feature_name: $start_percentage% -> $end_percentage% (step: $step_size%, interval: ${step_interval}s)"
    
    local current_percentage="$start_percentage"
    
    while [[ $current_percentage -lt $end_percentage ]]; do
        # Calculate next percentage
        local next_percentage=$((current_percentage + step_size))
        if [[ $next_percentage -gt $end_percentage ]]; then
            next_percentage="$end_percentage"
        fi
        
        log_info "Rolling out $feature_name to $next_percentage%"
        
        # Update rollout percentage
        enable_feature "$feature_name" "$next_percentage"
        
        # Wait before next step
        if [[ $next_percentage -lt $end_percentage ]]; then
            log_info "Waiting ${step_interval}s before next rollout step..."
            
            # Monitor metrics during wait
            monitor_feature_metrics "$feature_name" "$step_interval"
            
            # Check for rollback conditions
            if check_rollback_conditions "$feature_name"; then
                log_warn "Rollback conditions met, stopping gradual rollout"
                disable_feature "$feature_name"
                return 1
            fi
        fi
        
        current_percentage="$next_percentage"
    done
    
    log_success "Gradual rollout completed for $feature_name"
}

# Monitor feature metrics
monitor_feature_metrics() {
    local feature_name="$1"
    local duration="$2"
    
    log_info "Monitoring metrics for $feature_name (${duration}s)"
    
    local start_time=$(date +%s)
    local end_time=$((start_time + duration))
    
    while [[ $(date +%s) -lt $end_time ]]; do
        if curl -sf "$METRICS_ENDPOINT/features/$feature_name" >/dev/null 2>&1; then
            local metrics=$(curl -s "$METRICS_ENDPOINT/features/$feature_name")
            local error_rate=$(echo "$metrics" | jq -r '.error_rate // 0')
            local usage_count=$(echo "$metrics" | jq -r '.usage_count // 0')
            
            log_info "Feature metrics - Error rate: $error_rate, Usage: $usage_count"
        fi
        
        sleep 30
    done
}

# Check rollback conditions
check_rollback_conditions() {
    local feature_name="$1"
    
    if curl -sf "$METRICS_ENDPOINT/features/$feature_name" >/dev/null 2>&1; then
        local metrics=$(curl -s "$METRICS_ENDPOINT/features/$feature_name")
        local error_rate=$(echo "$metrics" | jq -r '.error_rate // 0')
        local threshold=$(echo "$metrics" | jq -r '.rollback_threshold // 0.05')
        
        if (( $(echo "$error_rate > $threshold" | bc -l) )); then
            log_warn "Error rate ($error_rate) exceeds threshold ($threshold) for $feature_name"
            return 0
        fi
    fi
    
    return 1
}

# Trigger configuration reload
trigger_config_reload() {
    log_info "Triggering configuration reload"
    
    # Reload via API
    if curl -sf "$ADMIN_ENDPOINT/config/reload" >/dev/null 2>&1; then
        curl -s -X POST "$ADMIN_ENDPOINT/config/reload" >/dev/null
        log_success "Configuration reload triggered via API"
    else
        # Send SIGHUP to application pods
        local pods=($(kubectl get pods -n "$NAMESPACE" -l app="$APP_NAME" -o jsonpath='{.items[*].metadata.name}'))
        
        for pod in "${pods[@]}"; do
            kubectl exec -n "$NAMESPACE" "$pod" -- kill -HUP 1 2>/dev/null || true
        done
        
        log_success "Configuration reload triggered via SIGHUP"
    fi
}

# A/B test setup
setup_ab_test() {
    local test_name="$1"
    local feature_a="$2"
    local feature_b="$3"
    local traffic_split="${4:-50}" # percentage for variant A
    
    log_info "Setting up A/B test: $test_name (A: $feature_a, B: $feature_b, split: $traffic_split%)"
    
    # Enable feature A with specified traffic
    enable_feature "$feature_a" "$traffic_split"
    
    # Enable feature B with remaining traffic
    local traffic_b=$((100 - traffic_split))
    enable_feature "$feature_b" "$traffic_b"
    
    log_success "A/B test setup completed: $test_name"
}

# Emergency rollback
emergency_rollback() {
    local feature_name="$1"
    local reason="${2:-Emergency rollback}"
    
    log_warn "EMERGENCY ROLLBACK: $feature_name - $reason"
    
    # Immediately disable the feature
    disable_feature "$feature_name"
    
    # Log emergency action
    local emergency_log="${LOG_DIR}/emergency-rollback-$(date +%Y%m%d-%H%M%S).log"
    cat > "$emergency_log" << EOF
Emergency Rollback Report
========================
Feature: $feature_name
Reason: $reason
Timestamp: $(date)
Operator: $(whoami)
Host: $(hostname)

Actions Taken:
- Feature flag disabled immediately
- Configuration reloaded
- All traffic diverted away from feature

EOF
    
    log_warn "Emergency rollback completed. Report saved: $emergency_log"
}

# Main execution
main() {
    local action="${1:-list}"
    
    setup_logging
    
    case "$action" in
        "init")
            log_info "Initializing feature flag system"
            create_feature_config
            deploy_feature_config
            ;;
        "list")
            list_feature_flags
            ;;
        "enable")
            local feature_name="${2:-}"
            local rollout_percentage="${3:-100}"
            local environment="${4:-production}"
            
            if [[ -z "$feature_name" ]]; then
                log_error "Feature name required"
                exit 1
            fi
            
            enable_feature "$feature_name" "$rollout_percentage" "$environment"
            ;;
        "disable")
            local feature_name="${2:-}"
            local environment="${3:-production}"
            
            if [[ -z "$feature_name" ]]; then
                log_error "Feature name required"
                exit 1
            fi
            
            disable_feature "$feature_name" "$environment"
            ;;
        "rollout")
            local feature_name="${2:-}"
            local start_percentage="${3:-0}"
            local end_percentage="${4:-100}"
            local step_size="${5:-10}"
            local step_interval="${6:-300}"
            
            if [[ -z "$feature_name" ]]; then
                log_error "Feature name required"
                exit 1
            fi
            
            gradual_rollout "$feature_name" "$start_percentage" "$end_percentage" "$step_size" "$step_interval"
            ;;
        "ab-test")
            local test_name="${2:-}"
            local feature_a="${3:-}"
            local feature_b="${4:-}"
            local traffic_split="${5:-50}"
            
            if [[ -z "$test_name" || -z "$feature_a" || -z "$feature_b" ]]; then
                log_error "Test name and both features required"
                exit 1
            fi
            
            setup_ab_test "$test_name" "$feature_a" "$feature_b" "$traffic_split"
            ;;
        "emergency-rollback")
            local feature_name="${2:-}"
            local reason="${3:-Emergency rollback}"
            
            if [[ -z "$feature_name" ]]; then
                log_error "Feature name required"
                exit 1
            fi
            
            emergency_rollback "$feature_name" "$reason"
            ;;
        "reload")
            trigger_config_reload
            ;;
        *)
            echo "Usage: $0 {init|list|enable|disable|rollout|ab-test|emergency-rollback|reload}"
            echo "Examples:"
            echo "  $0 init                                    # Initialize feature flag system"
            echo "  $0 list                                    # List all feature flags"
            echo "  $0 enable new_feature 50 production       # Enable feature at 50% rollout"
            echo "  $0 disable old_feature production         # Disable feature"
            echo "  $0 rollout new_feature 0 100 10 300       # Gradual rollout 0->100% in 10% steps"
            echo "  $0 ab-test test1 feature_a feature_b 50    # A/B test with 50/50 split"
            echo "  $0 emergency-rollback feature \"reason\"    # Emergency rollback"
            echo "  $0 reload                                  # Reload configuration"
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"
