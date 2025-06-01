#!/bin/bash
# Multi-Region Deployment Automation for PinCEX
# Orchestrates deployments across multiple regions with data synchronization

set -euo pipefail

# Configuration
REGIONS=("us-east-1" "eu-west-1" "ap-southeast-1")
NAMESPACE="pincex-prod"
DEPLOYMENT_NAME="pincex-core-api"
FEDERATION_CONTEXT="pincex-federation"
CONFIG_DIR="/etc/pincex/deployment"
LOG_DIR="/var/log/pincex-multiregion"

# Create log directory
mkdir -p "$LOG_DIR"

# Logging
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_DIR/multiregion-deployment.log"
}

error_exit() {
    log "ERROR: $1"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    for tool in kubectl helm terraform; do
        if ! command -v "$tool" &> /dev/null; then
            error_exit "$tool is not installed"
        fi
    done
    
    log "Prerequisites check passed"
}

# Setup federation cluster
setup_federation() {
    log "Setting up Kubernetes federation..."
    
    # Create federation namespace
    kubectl create namespace kube-federation-system --dry-run=client -o yaml | kubectl apply -f -
    
    # Install KubeFed
    helm repo add kubefed-charts https://raw.githubusercontent.com/kubernetes-sigs/kubefed/master/charts
    helm repo update
    
    helm upgrade --install kubefed kubefed-charts/kubefed \
        --namespace kube-federation-system \
        --create-namespace \
        --set-string controllermanager.replicaCount=3
    
    # Wait for KubeFed to be ready
    kubectl wait --for=condition=Available deployment/kubefed-controller-manager -n kube-federation-system --timeout=300s
    
    log "Federation setup completed"
}

# Join clusters to federation
join_clusters() {
    log "Joining regional clusters to federation..."
    
    for region in "${REGIONS[@]}"; do
        local cluster_name="pincex-$region"
        local context_name="$cluster_name"
        
        log "Joining cluster: $cluster_name"
        
        # Join cluster to federation
        kubefedctl join "$cluster_name" \
            --cluster-context "$context_name" \
            --host-cluster-context "$FEDERATION_CONTEXT" \
            --kubefed-namespace kube-federation-system
        
        # Verify cluster join
        if kubectl get kubefedclusters "$cluster_name" -n kube-federation-system; then
            log "Successfully joined cluster: $cluster_name"
        else
            error_exit "Failed to join cluster: $cluster_name"
        fi
    done
    
    log "All clusters joined to federation"
}

# Create federated deployment
create_federated_deployment() {
    log "Creating federated deployment configuration..."
    
    cat > "$CONFIG_DIR/federated-deployment.yaml" << 'EOF'
apiVersion: types.kubefed.io/v1beta1
kind: FederatedDeployment
metadata:
  name: pincex-core-api
  namespace: pincex-prod
spec:
  template:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: pincex-core-api
      labels:
        app: pincex-core-api
        deployment: federated
    spec:
      replicas: 3  # Base replica count per region
      strategy:
        type: RollingUpdate
        rollingUpdate:
          maxUnavailable: 1
          maxSurge: 1
      selector:
        matchLabels:
          app: pincex-core-api
      template:
        metadata:
          labels:
            app: pincex-core-api
        spec:
          containers:
          - name: pincex-core
            image: pincex/core-api:v2.0.0
            ports:
            - containerPort: 8080
            env:
            - name: REGION
              valueFrom:
                fieldRef:
                  fieldPath: metadata.annotations['region']
            - name: DEPLOYMENT_MODE
              value: "federated"
            resources:
              requests:
                memory: "512Mi"
                cpu: "500m"
              limits:
                memory: "1Gi"
                cpu: "1000m"
            livenessProbe:
              httpGet:
                path: /health/live
                port: 8080
              initialDelaySeconds: 30
              periodSeconds: 10
            readinessProbe:
              httpGet:
                path: /health/ready
                port: 8080
              initialDelaySeconds: 15
              periodSeconds: 5
  placement:
    clusters:
    - name: pincex-us-east-1
    - name: pincex-eu-west-1
    - name: pincex-ap-southeast-1
  overrides:
  - clusterName: pincex-us-east-1
    clusterOverrides:
    - path: "/spec/replicas"
      value: 5  # Higher replica count for primary region
    - path: "/spec/template/metadata/annotations"
      value:
        region: "us-east-1"
        primary: "true"
  - clusterName: pincex-eu-west-1
    clusterOverrides:
    - path: "/spec/replicas"
      value: 3
    - path: "/spec/template/metadata/annotations"
      value:
        region: "eu-west-1"
        primary: "false"
  - clusterName: pincex-ap-southeast-1
    clusterOverrides:
    - path: "/spec/replicas"
      value: 3
    - path: "/spec/template/metadata/annotations"
      value:
        region: "ap-southeast-1"
        primary: "false"
EOF
    
    # Apply federated deployment
    kubectl apply -f "$CONFIG_DIR/federated-deployment.yaml"
    
    log "Federated deployment created"
}

# Setup cross-region data synchronization
setup_data_sync() {
    log "Setting up cross-region data synchronization..."
    
    # CockroachDB cross-region configuration
    cat > "$CONFIG_DIR/cockroachdb-multiregion.yaml" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: cockroachdb-multiregion-config
  namespace: pincex-prod
data:
  multiregion.sql: |
    -- Enable multi-region capabilities
    ALTER DATABASE pincex_trading SET PRIMARY REGION 'us-east-1';
    ALTER DATABASE pincex_trading ADD REGION 'eu-west-1';
    ALTER DATABASE pincex_trading ADD REGION 'ap-southeast-1';
    
    -- Configure table localities for optimal performance
    ALTER TABLE orders SET LOCALITY GLOBAL;
    ALTER TABLE trades SET LOCALITY REGIONAL BY ROW AS "region";
    ALTER TABLE user_balances SET LOCALITY REGIONAL BY ROW AS "region";
    ALTER TABLE market_data SET LOCALITY GLOBAL;
    
    -- Setup cross-region replication
    ALTER TABLE critical_data SET LOCALITY REGIONAL;
    SET CLUSTER SETTING cluster.organization = 'pincex-crypto-exchange';
---
apiVersion: batch/v1
kind: Job
metadata:
  name: setup-multiregion-db
  namespace: pincex-prod
spec:
  template:
    spec:
      containers:
      - name: cockroach-setup
        image: cockroachdb/cockroach:v23.1.0
        command:
        - /bin/bash
        - -c
        - |
          cockroach sql --host cockroachdb-public --insecure < /config/multiregion.sql
        volumeMounts:
        - name: config
          mountPath: /config
      volumes:
      - name: config
        configMap:
          name: cockroachdb-multiregion-config
      restartPolicy: OnFailure
EOF
    
    kubectl apply -f "$CONFIG_DIR/cockroachdb-multiregion.yaml"
    
    # Redis Cluster cross-region setup
    cat > "$CONFIG_DIR/redis-cluster-sync.yaml" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-sync-config
  namespace: pincex-prod
data:
  sync-script.sh: |
    #!/bin/bash
    # Redis cross-region synchronization
    
    REGIONS=("us-east-1" "eu-west-1" "ap-southeast-1")
    PRIMARY_REGION="us-east-1"
    
    for region in "${REGIONS[@]}"; do
      if [[ "$region" != "$PRIMARY_REGION" ]]; then
        echo "Setting up replication from $PRIMARY_REGION to $region"
        
        # Configure Redis replication
        redis-cli -h redis-$region.pincex-prod.svc.cluster.local \
          REPLICAOF redis-$PRIMARY_REGION.pincex-prod.svc.cluster.local 6379
      fi
    done
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: redis-sync-monitor
  namespace: pincex-prod
spec:
  schedule: "*/5 * * * *"  # Every 5 minutes
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: redis-sync
            image: redis:7-alpine
            command:
            - /bin/sh
            - /config/sync-script.sh
            volumeMounts:
            - name: config
              mountPath: /config
          volumes:
          - name: config
            configMap:
              name: redis-sync-config
              defaultMode: 0755
          restartPolicy: OnFailure
EOF
    
    kubectl apply -f "$CONFIG_DIR/redis-cluster-sync.yaml"
    
    log "Data synchronization setup completed"
}

# Setup global load balancer
setup_global_loadbalancer() {
    log "Setting up global load balancer..."
    
    cat > "$CONFIG_DIR/global-loadbalancer.yaml" << 'EOF'
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: pincex-global-routing
  namespace: pincex-prod
spec:
  hosts:
  - api.pincex.com
  gateways:
  - pincex-gateway
  http:
  - match:
    - headers:
        x-region:
          exact: us-east-1
    route:
    - destination:
        host: pincex-core-api-service.us-east-1.local
      weight: 100
  - match:
    - headers:
        x-region:
          exact: eu-west-1
    route:
    - destination:
        host: pincex-core-api-service.eu-west-1.local
      weight: 100
  - match:
    - headers:
        x-region:
          exact: ap-southeast-1
    route:
    - destination:
        host: pincex-core-api-service.ap-southeast-1.local
      weight: 100
  - route:  # Default routing with failover
    - destination:
        host: pincex-core-api-service.us-east-1.local
      weight: 60
    - destination:
        host: pincex-core-api-service.eu-west-1.local
      weight: 25
    - destination:
        host: pincex-core-api-service.ap-southeast-1.local
      weight: 15
    fault:
      delay:
        percentage:
          value: 0.1
        fixedDelay: 1s
---
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: pincex-regional-circuits
  namespace: pincex-prod
spec:
  host: "*.pincex-prod.svc.cluster.local"
  trafficPolicy:
    connectionPool:
      tcp:
        maxConnections: 100
      http:
        http1MaxPendingRequests: 50
        maxRequestsPerConnection: 10
    circuitBreaker:
      consecutiveErrors: 3
      interval: 30s
      baseEjectionTime: 30s
      maxEjectionPercent: 50
    outlierDetection:
      consecutive5xxErrors: 3
      interval: 30s
      baseEjectionTime: 30s
      maxEjectionPercent: 50
EOF
    
    kubectl apply -f "$CONFIG_DIR/global-loadbalancer.yaml"
    
    log "Global load balancer configured"
}

# Validate multi-region deployment
validate_deployment() {
    log "Validating multi-region deployment..."
    
    for region in "${REGIONS[@]}"; do
        local cluster_name="pincex-$region"
        
        log "Checking deployment in region: $region"
        
        # Switch to region context
        kubectl config use-context "$cluster_name"
        
        # Check deployment status
        if kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" &> /dev/null; then
            local ready_replicas=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}')
            local desired_replicas=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}')
            
            if [[ "$ready_replicas" == "$desired_replicas" ]]; then
                log "Region $region: $ready_replicas/$desired_replicas replicas ready ✓"
            else
                error_exit "Region $region: Only $ready_replicas/$desired_replicas replicas ready"
            fi
        else
            error_exit "Deployment not found in region: $region"
        fi
    done
    
    # Switch back to federation context
    kubectl config use-context "$FEDERATION_CONTEXT"
    
    log "Multi-region deployment validation completed successfully"
}

# Main execution
main() {
    log "Starting multi-region deployment automation for PinCEX"
    
    check_prerequisites
    setup_federation
    join_clusters
    create_federated_deployment
    setup_data_sync
    setup_global_loadbalancer
    
    # Wait for deployments to stabilize
    sleep 120
    
    validate_deployment
    
    log "Multi-region deployment automation completed successfully"
    
    # Generate deployment report
    cat > "$LOG_DIR/deployment-report.txt" << EOF
PinCEX Multi-Region Deployment Report
=====================================
Deployment Time: $(date)
Regions: ${REGIONS[*]}
Status: SUCCESS

Regional Status:
EOF
    
    for region in "${REGIONS[@]}"; do
        kubectl config use-context "pincex-$region"
        local ready_replicas=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        echo "- $region: $ready_replicas replicas ready" >> "$LOG_DIR/deployment-report.txt"
    done
    
    log "Deployment report generated: $LOG_DIR/deployment-report.txt"
}

# Execute main function
main "$@"
