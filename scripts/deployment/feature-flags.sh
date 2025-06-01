#!/bin/bash
# Feature Flags and Dynamic Configuration System for PinCEX
# Enables runtime configuration changes without service restarts

set -euo pipefail

# Configuration
NAMESPACE="pincex-prod"
CONSUL_ENDPOINT="http://consul:8500"
CONFIG_BACKUP_DIR="/var/backups/pincex-config"
LOG_FILE="/var/log/pincex-feature-flags.log"

# Create backup directory
mkdir -p "$CONFIG_BACKUP_DIR"

# Logging
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

error_exit() {
    log "ERROR: $1"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    for tool in kubectl jq curl; do
        if ! command -v "$tool" &> /dev/null; then
            error_exit "$tool is not installed"
        fi
    done
    
    log "Prerequisites check passed"
}

# Deploy Consul for configuration management
deploy_consul() {
    log "Deploying Consul cluster for configuration management"
    
    cat > "/tmp/consul-cluster.yaml" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: consul-config
  namespace: pincex-prod
data:
  consul.json: |
    {
      "datacenter": "pincex-prod",
      "data_dir": "/consul/data",
      "log_level": "INFO",
      "server": true,
      "bootstrap_expect": 3,
      "bind_addr": "0.0.0.0",
      "client_addr": "0.0.0.0",
      "retry_join": [
        "consul-0.consul.pincex-prod.svc.cluster.local",
        "consul-1.consul.pincex-prod.svc.cluster.local",
        "consul-2.consul.pincex-prod.svc.cluster.local"
      ],
      "ui_config": {
        "enabled": true
      },
      "connect": {
        "enabled": true
      },
      "acl": {
        "enabled": true,
        "default_policy": "deny",
        "enable_token_persistence": true
      }
    }
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: consul
  namespace: pincex-prod
spec:
  serviceName: consul
  replicas: 3
  selector:
    matchLabels:
      app: consul
  template:
    metadata:
      labels:
        app: consul
    spec:
      containers:
      - name: consul
        image: consul:1.16.1
        ports:
        - containerPort: 8500
          name: ui-port
        - containerPort: 8400
          name: alt-port
        - containerPort: 53
          name: udp-port
        - containerPort: 8443
          name: https-port
        - containerPort: 8080
          name: http-port
        - containerPort: 8301
          name: serflan
        - containerPort: 8302
          name: serfwan
        - containerPort: 8600
          name: consuldns
        - containerPort: 8300
          name: server
        env:
        - name: CONSUL_BIND_INTERFACE
          value: eth0
        - name: CONSUL_CLIENT_INTERFACE
          value: eth0
        command:
        - consul
        - agent
        - -config-file=/consul/config/consul.json
        volumeMounts:
        - name: config
          mountPath: /consul/config
        - name: data
          mountPath: /consul/data
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      volumes:
      - name: config
        configMap:
          name: consul-config
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: consul
  namespace: pincex-prod
  labels:
    app: consul
spec:
  clusterIP: None
  ports:
  - port: 8500
    targetPort: 8500
    name: ui-port
  - port: 8400
    targetPort: 8400
    name: alt-port
  - port: 53
    targetPort: 53
    name: udp-port
  - port: 8443
    targetPort: 8443
    name: https-port
  - port: 8080
    targetPort: 8080
    name: http-port
  - port: 8301
    targetPort: 8301
    name: serflan-tcp
    protocol: TCP
  - port: 8301
    targetPort: 8301
    name: serflan-udp
    protocol: UDP
  - port: 8302
    targetPort: 8302
    name: serfwan-tcp
    protocol: TCP
  - port: 8302
    targetPort: 8302
    name: serfwan-udp
    protocol: UDP
  - port: 8300
    targetPort: 8300
    name: server
  - port: 8600
    targetPort: 8600
    name: consuldns-tcp
    protocol: TCP
  - port: 8600
    targetPort: 8600
    name: consuldns-udp
    protocol: UDP
  selector:
    app: consul
---
apiVersion: v1
kind: Service
metadata:
  name: consul-ui
  namespace: pincex-prod
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 8500
  selector:
    app: consul
EOF
    
    kubectl apply -f "/tmp/consul-cluster.yaml"
    
    # Wait for Consul to be ready
    kubectl wait --for=condition=ready pod -l app=consul -n "$NAMESPACE" --timeout=300s
    
    log "Consul cluster deployed successfully"
}

# Initialize feature flags
initialize_feature_flags() {
    log "Initializing feature flags in Consul"
    
    # Wait for Consul to be fully operational
    sleep 30
    
    # Create feature flags configuration
    local consul_pod=$(kubectl get pods -n "$NAMESPACE" -l app=consul -o jsonpath='{.items[0].metadata.name}')
    
    # Trading engine feature flags
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/features/high_frequency_trading "true"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/features/margin_trading "true"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/features/futures_trading "false"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/features/options_trading "false"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/features/staking "true"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/features/lending "false"
    
    # Performance tuning flags
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/performance/order_batch_size "100"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/performance/max_concurrent_orders "10000"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/performance/cache_ttl "300"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/performance/rate_limit_requests_per_second "1000"
    
    # Risk management settings
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/risk/max_position_size "1000000"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/risk/leverage_limit "10"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/risk/circuit_breaker_threshold "0.05"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/risk/auto_liquidation "true"
    
    # Market data settings
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/market_data/websocket_rate_limit "100"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/market_data/depth_levels "20"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/market_data/update_frequency "100"
    
    # Security settings
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/security/require_2fa "true"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/security/session_timeout "3600"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/security/max_login_attempts "5"
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put config/pincex/security/ip_whitelist_enabled "false"
    
    log "Feature flags initialized successfully"
}

# Create configuration watcher service
create_config_watcher() {
    log "Creating configuration watcher service"
    
    cat > "/tmp/config-watcher.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: config-watcher
  namespace: pincex-prod
  labels:
    app: config-watcher
spec:
  replicas: 2
  selector:
    matchLabels:
      app: config-watcher
  template:
    metadata:
      labels:
        app: config-watcher
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      containers:
      - name: config-watcher
        image: pincex/config-watcher:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: CONSUL_URL
          value: "http://consul:8500"
        - name: CONFIG_PREFIX
          value: "config/pincex"
        - name: RELOAD_ENDPOINT_BASE
          value: "http://pincex-core-api-service/admin/config/reload"
        - name: WATCH_INTERVAL
          value: "5"
        - name: LOG_LEVEL
          value: "INFO"
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 5
        volumeMounts:
        - name: config-backup
          mountPath: /backups
      volumes:
      - name: config-backup
        persistentVolumeClaim:
          claimName: config-backup-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: config-watcher
  namespace: pincex-prod
  labels:
    app: config-watcher
spec:
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  - port: 9090
    targetPort: 9090
    name: metrics
  selector:
    app: config-watcher
EOF
    
    kubectl apply -f "/tmp/config-watcher.yaml"
    
    log "Configuration watcher service created"
}

# Create feature flag management API
create_feature_flag_api() {
    log "Creating feature flag management API"
    
    cat > "/tmp/feature-flag-api.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: feature-flag-api
  namespace: pincex-prod
  labels:
    app: feature-flag-api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: feature-flag-api
  template:
    metadata:
      labels:
        app: feature-flag-api
    spec:
      containers:
      - name: feature-flag-api
        image: pincex/feature-flag-api:v1.0.0
        ports:
        - containerPort: 8080
        env:
        - name: CONSUL_URL
          value: "http://consul:8500"
        - name: CONFIG_PREFIX
          value: "config/pincex"
        - name: AUTH_ENABLED
          value: "true"
        - name: AUDIT_LOG_ENABLED
          value: "true"
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: feature-flag-api
  namespace: pincex-prod
  labels:
    app: feature-flag-api
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: feature-flag-api
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: feature-flag-api-ingress
  namespace: pincex-prod
  annotations:
    kubernetes.io/ingress.class: nginx
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/auth-type: basic
    nginx.ingress.kubernetes.io/auth-secret: feature-flag-auth
spec:
  tls:
  - hosts:
    - admin.pincex.com
    secretName: pincex-tls
  rules:
  - host: admin.pincex.com
    http:
      paths:
      - path: /feature-flags
        pathType: Prefix
        backend:
          service:
            name: feature-flag-api
            port:
              number: 80
EOF
    
    kubectl apply -f "/tmp/feature-flag-api.yaml"
    
    log "Feature flag management API created"
}

# Update application deployment with config injection
update_app_deployment() {
    log "Updating application deployment with dynamic configuration support"
    
    cat > "/tmp/app-config-patch.yaml" << 'EOF'
spec:
  template:
    spec:
      containers:
      - name: pincex-core
        env:
        - name: CONSUL_URL
          value: "http://consul:8500"
        - name: CONFIG_PREFIX
          value: "config/pincex"
        - name: CONFIG_RELOAD_ENABLED
          value: "true"
        - name: CONFIG_RELOAD_INTERVAL
          value: "30"
        volumeMounts:
        - name: config-cache
          mountPath: /app/config
        - name: consul-template-config
          mountPath: /consul-template
      - name: consul-template
        image: hashicorp/consul-template:0.32.0
        command:
        - consul-template
        - -config=/consul-template/config.hcl
        - -once=false
        - -wait=5s:30s
        env:
        - name: CONSUL_URL
          value: "http://consul:8500"
        volumeMounts:
        - name: config-cache
          mountPath: /app/config
        - name: consul-template-config
          mountPath: /consul-template
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
      volumes:
      - name: config-cache
        emptyDir: {}
      - name: consul-template-config
        configMap:
          name: consul-template-config
EOF
    
    # Create consul-template configuration
    cat > "/tmp/consul-template-config.yaml" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: consul-template-config
  namespace: pincex-prod
data:
  config.hcl: |
    consul {
      address = "consul:8500"
    }
    
    template {
      source      = "/consul-template/features.json.tpl"
      destination = "/app/config/features.json"
      perms       = 0644
      command     = "curl -X POST http://localhost:8080/admin/config/reload || true"
    }
    
    template {
      source      = "/consul-template/performance.json.tpl"
      destination = "/app/config/performance.json"
      perms       = 0644
      command     = "curl -X POST http://localhost:8080/admin/config/reload || true"
    }
    
    template {
      source      = "/consul-template/risk.json.tpl"
      destination = "/app/config/risk.json"
      perms       = 0644
      command     = "curl -X POST http://localhost:8080/admin/config/reload || true"
    }
  
  features.json.tpl: |
    {
      "high_frequency_trading": {{ key "config/pincex/features/high_frequency_trading" }},
      "margin_trading": {{ key "config/pincex/features/margin_trading" }},
      "futures_trading": {{ key "config/pincex/features/futures_trading" }},
      "options_trading": {{ key "config/pincex/features/options_trading" }},
      "staking": {{ key "config/pincex/features/staking" }},
      "lending": {{ key "config/pincex/features/lending" }}
    }
  
  performance.json.tpl: |
    {
      "order_batch_size": {{ key "config/pincex/performance/order_batch_size" }},
      "max_concurrent_orders": {{ key "config/pincex/performance/max_concurrent_orders" }},
      "cache_ttl": {{ key "config/pincex/performance/cache_ttl" }},
      "rate_limit_requests_per_second": {{ key "config/pincex/performance/rate_limit_requests_per_second" }}
    }
  
  risk.json.tpl: |
    {
      "max_position_size": {{ key "config/pincex/risk/max_position_size" }},
      "leverage_limit": {{ key "config/pincex/risk/leverage_limit" }},
      "circuit_breaker_threshold": {{ key "config/pincex/risk/circuit_breaker_threshold" }},
      "auto_liquidation": {{ key "config/pincex/risk/auto_liquidation" }}
    }
EOF
    
    kubectl apply -f "/tmp/consul-template-config.yaml"
    kubectl patch deployment pincex-core-api -n "$NAMESPACE" --patch-file "/tmp/app-config-patch.yaml"
    
    log "Application deployment updated with dynamic configuration support"
}

# Create configuration management scripts
create_management_scripts() {
    log "Creating configuration management scripts"
    
    # Feature flag toggle script
    cat > "/tmp/toggle-feature.sh" << 'EOF'
#!/bin/bash
# Toggle feature flag script

FEATURE_NAME="$1"
FEATURE_VALUE="$2"
NAMESPACE="pincex-prod"

if [[ -z "$FEATURE_NAME" || -z "$FEATURE_VALUE" ]]; then
    echo "Usage: $0 <feature_name> <true|false>"
    exit 1
fi

# Get Consul pod
CONSUL_POD=$(kubectl get pods -n "$NAMESPACE" -l app=consul -o jsonpath='{.items[0].metadata.name}')

# Update feature flag
kubectl exec -n "$NAMESPACE" "$CONSUL_POD" -- consul kv put "config/pincex/features/$FEATURE_NAME" "$FEATURE_VALUE"

echo "Feature '$FEATURE_NAME' set to '$FEATURE_VALUE'"

# Verify configuration reload
sleep 5
echo "Checking if configuration was reloaded..."
kubectl logs -n "$NAMESPACE" -l app=config-watcher --tail=10
EOF
    
    # Configuration backup script
    cat > "/tmp/backup-config.sh" << 'EOF'
#!/bin/bash
# Configuration backup script

NAMESPACE="pincex-prod"
BACKUP_DIR="/var/backups/pincex-config"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$BACKUP_DIR"

# Get Consul pod
CONSUL_POD=$(kubectl get pods -n "$NAMESPACE" -l app=consul -o jsonpath='{.items[0].metadata.name}')

# Export all configuration
kubectl exec -n "$NAMESPACE" "$CONSUL_POD" -- consul kv export config/pincex/ > "$BACKUP_DIR/config_backup_$TIMESTAMP.json"

echo "Configuration backed up to: $BACKUP_DIR/config_backup_$TIMESTAMP.json"

# Keep only last 30 backups
find "$BACKUP_DIR" -name "config_backup_*.json" -mtime +30 -delete
EOF
    
    # Configuration restore script
    cat > "/tmp/restore-config.sh" << 'EOF'
#!/bin/bash
# Configuration restore script

BACKUP_FILE="$1"
NAMESPACE="pincex-prod"

if [[ -z "$BACKUP_FILE" ]]; then
    echo "Usage: $0 <backup_file>"
    exit 1
fi

if [[ ! -f "$BACKUP_FILE" ]]; then
    echo "Backup file not found: $BACKUP_FILE"
    exit 1
fi

# Get Consul pod
CONSUL_POD=$(kubectl get pods -n "$NAMESPACE" -l app=consul -o jsonpath='{.items[0].metadata.name}')

# Restore configuration
kubectl cp "$BACKUP_FILE" "$NAMESPACE/$CONSUL_POD:/tmp/restore.json"
kubectl exec -n "$NAMESPACE" "$CONSUL_POD" -- consul kv import @/tmp/restore.json

echo "Configuration restored from: $BACKUP_FILE"
EOF
    
    chmod +x /tmp/toggle-feature.sh /tmp/backup-config.sh /tmp/restore-config.sh
    
    # Copy scripts to a permanent location
    kubectl create configmap management-scripts -n "$NAMESPACE" \
        --from-file=toggle-feature.sh=/tmp/toggle-feature.sh \
        --from-file=backup-config.sh=/tmp/backup-config.sh \
        --from-file=restore-config.sh=/tmp/restore-config.sh
    
    log "Configuration management scripts created"
}

# Validate configuration system
validate_config_system() {
    log "Validating dynamic configuration system"
    
    # Check if Consul is running
    if ! kubectl get pods -n "$NAMESPACE" -l app=consul | grep -q Running; then
        error_exit "Consul is not running"
    fi
    
    # Check if config watcher is running
    if ! kubectl get pods -n "$NAMESPACE" -l app=config-watcher | grep -q Running; then
        error_exit "Config watcher is not running"
    fi
    
    # Test configuration change
    local consul_pod=$(kubectl get pods -n "$NAMESPACE" -l app=consul -o jsonpath='{.items[0].metadata.name}')
    local test_value="test_$(date +%s)"
    
    log "Testing configuration change..."
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv put "config/pincex/test/validation" "$test_value"
    
    # Wait and check if change was detected
    sleep 10
    
    local retrieved_value=$(kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv get "config/pincex/test/validation")
    
    if [[ "$retrieved_value" == "$test_value" ]]; then
        log "Configuration system validation successful"
    else
        error_exit "Configuration system validation failed"
    fi
    
    # Cleanup test key
    kubectl exec -n "$NAMESPACE" "$consul_pod" -- consul kv delete "config/pincex/test/validation"
}

# Main execution
main() {
    local action="${1:-deploy}"
    
    check_prerequisites
    
    case "$action" in
        "deploy")
            log "Deploying feature flags and dynamic configuration system"
            deploy_consul
            sleep 60  # Wait for Consul to stabilize
            initialize_feature_flags
            create_config_watcher
            create_feature_flag_api
            update_app_deployment
            create_management_scripts
            validate_config_system
            log "Feature flags and dynamic configuration system deployed successfully"
            ;;
        "toggle")
            local feature_name="$2"
            local feature_value="$3"
            if [[ -z "$feature_name" || -z "$feature_value" ]]; then
                error_exit "Usage: $0 toggle <feature_name> <true|false>"
            fi
            /tmp/toggle-feature.sh "$feature_name" "$feature_value"
            ;;
        "backup")
            /tmp/backup-config.sh
            ;;
        "restore")
            local backup_file="$2"
            if [[ -z "$backup_file" ]]; then
                error_exit "Usage: $0 restore <backup_file>"
            fi
            /tmp/restore-config.sh "$backup_file"
            ;;
        "validate")
            validate_config_system
            ;;
        *)
            error_exit "Unknown action: $action. Use: deploy, toggle, backup, restore, or validate"
            ;;
    esac
}

# Execute main function
main "$@"
