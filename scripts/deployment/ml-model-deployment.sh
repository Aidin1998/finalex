#!/bin/bash
# Dynamic ML Model Updates and Shadow Testing for PinCEX
# Enables zero-downtime ML model deployments with A/B testing and performance validation

set -euo pipefail

# Configuration
NAMESPACE="pincex-prod"
MODEL_REGISTRY="pincex-model-registry.azurecr.io"
SHADOW_TESTING_DURATION=3600  # 1 hour
A_B_TEST_DURATION=7200        # 2 hours
LOG_DIR="/var/log/pincex-ml-deployment"
METRICS_ENDPOINT="http://prometheus:9090"

# Create log directory
mkdir -p "$LOG_DIR"

# Logging
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_DIR/ml-deployment.log"
}

error_exit() {
    log "ERROR: $1"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    for tool in kubectl helm jq curl; do
        if ! command -v "$tool" &> /dev/null; then
            error_exit "$tool is not installed"
        fi
    done
    
    # Check if MLflow is available
    if ! kubectl get service mlflow-tracking -n "$NAMESPACE" &> /dev/null; then
        error_exit "MLflow tracking service not found"
    fi
    
    log "Prerequisites check passed"
}

# Deploy shadow model for testing
deploy_shadow_model() {
    local model_name="$1"
    local model_version="$2"
    local model_image="$MODEL_REGISTRY/$model_name:$model_version"
    
    log "Deploying shadow model: $model_name version $model_version"
    
    # Create shadow deployment
    cat > "/tmp/shadow-model-$model_name.yaml" << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${model_name}-shadow
  namespace: $NAMESPACE
  labels:
    app: ${model_name}-shadow
    model: $model_name
    version: $model_version
    deployment-type: shadow
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ${model_name}-shadow
  template:
    metadata:
      labels:
        app: ${model_name}-shadow
        model: $model_name
        version: $model_version
        deployment-type: shadow
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - name: model-server
        image: $model_image
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: MODEL_NAME
          value: "$model_name"
        - name: MODEL_VERSION
          value: "$model_version"
        - name: DEPLOYMENT_TYPE
          value: "shadow"
        - name: SHADOW_MODE
          value: "true"
        - name: LOG_PREDICTIONS
          value: "true"
        - name: MLFLOW_TRACKING_URI
          value: "http://mlflow-tracking:5000"
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
            nvidia.com/gpu: "0.5"
          limits:
            memory: "2Gi"
            cpu: "1000m"
            nvidia.com/gpu: "1"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 60
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        volumeMounts:
        - name: model-cache
          mountPath: /models
        - name: shadow-logs
          mountPath: /logs
      volumes:
      - name: model-cache
        emptyDir:
          sizeLimit: 10Gi
      - name: shadow-logs
        persistentVolumeClaim:
          claimName: shadow-logs-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: ${model_name}-shadow-service
  namespace: $NAMESPACE
  labels:
    app: ${model_name}-shadow
spec:
  ports:
  - port: 80
    targetPort: 8080
    name: http
  - port: 9090
    targetPort: 9090
    name: metrics
  selector:
    app: ${model_name}-shadow
EOF
    
    # Apply shadow deployment
    kubectl apply -f "/tmp/shadow-model-$model_name.yaml"
    
    # Wait for shadow deployment to be ready
    kubectl wait --for=condition=available deployment/${model_name}-shadow -n "$NAMESPACE" --timeout=300s
    
    log "Shadow model deployment completed: $model_name"
}

# Configure traffic splitting for A/B testing
setup_traffic_splitting() {
    local model_name="$1"
    local production_weight="${2:-90}"
    local shadow_weight="${3:-10}"
    
    log "Setting up traffic splitting: production($production_weight%) shadow($shadow_weight%)"
    
    # Create Istio VirtualService for traffic splitting
    cat > "/tmp/traffic-split-$model_name.yaml" << EOF
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: ${model_name}-traffic-split
  namespace: $NAMESPACE
spec:
  hosts:
  - ${model_name}-service
  http:
  - match:
    - headers:
        x-shadow-test:
          exact: "true"
    route:
    - destination:
        host: ${model_name}-shadow-service
      weight: 100
  - route:
    - destination:
        host: ${model_name}-service
      weight: $production_weight
    - destination:
        host: ${model_name}-shadow-service
      weight: $shadow_weight
    fault:
      delay:
        percentage:
          value: 0.1
        fixedDelay: 100ms
---
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: ${model_name}-destination-rule
  namespace: $NAMESPACE
spec:
  host: ${model_name}-service
  trafficPolicy:
    circuitBreaker:
      consecutiveErrors: 5
      interval: 30s
      baseEjectionTime: 30s
EOF
    
    kubectl apply -f "/tmp/traffic-split-$model_name.yaml"
    
    log "Traffic splitting configured for $model_name"
}

# Monitor shadow testing metrics
monitor_shadow_testing() {
    local model_name="$1"
    local duration="$2"
    local start_time=$(date +%s)
    
    log "Starting shadow testing monitoring for $model_name (duration: ${duration}s)"
    
    # Create monitoring dashboard
    cat > "/tmp/shadow-metrics-$model_name.json" << 'EOF'
{
  "dashboard": {
    "title": "Shadow Model Testing",
    "panels": [
      {
        "title": "Prediction Accuracy",
        "type": "stat",
        "targets": [
          {
            "expr": "accuracy_score{model=\"{{ model_name }}\",deployment_type=\"shadow\"}"
          }
        ]
      },
      {
        "title": "Response Time",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(prediction_duration_seconds_bucket{model=\"{{ model_name }}\"}[5m]))"
          }
        ]
      },
      {
        "title": "Error Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(prediction_errors_total{model=\"{{ model_name }}\"}[5m])"
          }
        ]
      }
    ]
  }
}
EOF
    
    # Monitor metrics during testing period
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        
        if [[ $elapsed_time -gt $duration ]]; then
            log "Shadow testing monitoring completed"
            break
        fi
        
        # Collect metrics
        local shadow_accuracy=$(query_prometheus "accuracy_score{model=\"$model_name\",deployment_type=\"shadow\"}")
        local production_accuracy=$(query_prometheus "accuracy_score{model=\"$model_name\",deployment_type=\"production\"}")
        local shadow_latency=$(query_prometheus "histogram_quantile(0.95, rate(prediction_duration_seconds_bucket{model=\"$model_name\",deployment_type=\"shadow\"}[5m]))")
        local production_latency=$(query_prometheus "histogram_quantile(0.95, rate(prediction_duration_seconds_bucket{model=\"$model_name\",deployment_type=\"production\"}[5m]))")
        
        log "Shadow Model Metrics:"
        log "  Accuracy: shadow=${shadow_accuracy:-N/A}, production=${production_accuracy:-N/A}"
        log "  P95 Latency: shadow=${shadow_latency:-N/A}s, production=${production_latency:-N/A}s"
        
        # Check for critical issues
        if [[ -n "$shadow_accuracy" && -n "$production_accuracy" ]]; then
            local accuracy_diff=$(echo "$shadow_accuracy - $production_accuracy" | bc 2>/dev/null || echo "0")
            if (( $(echo "$accuracy_diff < -0.05" | bc -l) )); then
                log "WARNING: Shadow model accuracy significantly lower than production"
            fi
        fi
        
        sleep 60  # Check every minute
    done
}

# Query Prometheus metrics
query_prometheus() {
    local query="$1"
    local result=$(curl -s "$METRICS_ENDPOINT/api/v1/query" \
        --data-urlencode "query=$query" | \
        jq -r '.data.result[0].value[1]' 2>/dev/null)
    
    if [[ "$result" != "null" && -n "$result" ]]; then
        echo "$result"
    fi
}

# Analyze shadow testing results
analyze_shadow_results() {
    local model_name="$1"
    
    log "Analyzing shadow testing results for $model_name"
    
    # Generate comprehensive analysis report
    local shadow_accuracy=$(query_prometheus "avg_over_time(accuracy_score{model=\"$model_name\",deployment_type=\"shadow\"}[1h])")
    local production_accuracy=$(query_prometheus "avg_over_time(accuracy_score{model=\"$model_name\",deployment_type=\"production\"}[1h])")
    local shadow_latency=$(query_prometheus "avg_over_time(histogram_quantile(0.95, rate(prediction_duration_seconds_bucket{model=\"$model_name\",deployment_type=\"shadow\"}[5m]))[1h])")
    local production_latency=$(query_prometheus "avg_over_time(histogram_quantile(0.95, rate(prediction_duration_seconds_bucket{model=\"$model_name\",deployment_type=\"production\"}[5m]))[1h])")
    local shadow_errors=$(query_prometheus "increase(prediction_errors_total{model=\"$model_name\",deployment_type=\"shadow\"}[1h])")
    local production_errors=$(query_prometheus "increase(prediction_errors_total{model=\"$model_name\",deployment_type=\"production\"}[1h])")
    
    # Create analysis report
    cat > "$LOG_DIR/shadow-analysis-$model_name.txt" << EOF
Shadow Testing Analysis Report
==============================
Model: $model_name
Analysis Time: $(date)
Testing Duration: 1 hour

Performance Metrics:
-------------------
Accuracy:
  Shadow Model: ${shadow_accuracy:-N/A}
  Production Model: ${production_accuracy:-N/A}
  Difference: $(echo "${shadow_accuracy:-0} - ${production_accuracy:-0}" | bc 2>/dev/null || echo "N/A")

Latency (P95):
  Shadow Model: ${shadow_latency:-N/A}s
  Production Model: ${production_latency:-N/A}s
  Difference: $(echo "${shadow_latency:-0} - ${production_latency:-0}" | bc 2>/dev/null || echo "N/A")s

Error Count (1h):
  Shadow Model: ${shadow_errors:-N/A}
  Production Model: ${production_errors:-N/A}

Recommendation:
--------------
EOF
    
    # Generate recommendation
    local recommendation="PROCEED"
    local accuracy_diff=$(echo "${shadow_accuracy:-0} - ${production_accuracy:-0}" | bc 2>/dev/null || echo "0")
    local latency_diff=$(echo "${shadow_latency:-0} - ${production_latency:-0}" | bc 2>/dev/null || echo "0")
    
    if (( $(echo "$accuracy_diff < -0.03" | bc -l 2>/dev/null || echo "0") )); then
        recommendation="REJECT - Accuracy degradation"
    elif (( $(echo "$latency_diff > 0.1" | bc -l 2>/dev/null || echo "0") )); then
        recommendation="REJECT - Latency increase"
    elif [[ "${shadow_errors:-0}" -gt "${production_errors:-0}" ]]; then
        recommendation="REVIEW - Higher error rate"
    fi
    
    echo "$recommendation" >> "$LOG_DIR/shadow-analysis-$model_name.txt"
    
    log "Shadow testing analysis completed: $recommendation"
    echo "$recommendation"
}

# Promote shadow model to production
promote_shadow_model() {
    local model_name="$1"
    
    log "Promoting shadow model to production: $model_name"
    
    # Update production deployment with shadow model image
    local shadow_image=$(kubectl get deployment "${model_name}-shadow" -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}')
    
    log "Updating production deployment with image: $shadow_image"
    
    # Perform rolling update
    kubectl set image deployment/"$model_name" model-server="$shadow_image" -n "$NAMESPACE"
    
    # Wait for rollout to complete
    kubectl rollout status deployment/"$model_name" -n "$NAMESPACE" --timeout=600s
    
    # Remove shadow deployment
    kubectl delete deployment "${model_name}-shadow" -n "$NAMESPACE"
    kubectl delete service "${model_name}-shadow-service" -n "$NAMESPACE"
    
    # Remove traffic splitting
    kubectl delete virtualservice "${model_name}-traffic-split" -n "$NAMESPACE"
    kubectl delete destinationrule "${model_name}-destination-rule" -n "$NAMESPACE"
    
    log "Shadow model promotion completed: $model_name"
}

# Rollback shadow deployment
rollback_shadow_model() {
    local model_name="$1"
    
    log "Rolling back shadow model deployment: $model_name"
    
    # Remove shadow deployment
    kubectl delete deployment "${model_name}-shadow" -n "$NAMESPACE" || true
    kubectl delete service "${model_name}-shadow-service" -n "$NAMESPACE" || true
    
    # Remove traffic splitting
    kubectl delete virtualservice "${model_name}-traffic-split" -n "$NAMESPACE" || true
    kubectl delete destinationrule "${model_name}-destination-rule" -n "$NAMESPACE" || true
    
    log "Shadow model rollback completed: $model_name"
}

# Main execution function
main() {
    local action="$1"
    local model_name="$2"
    local model_version="${3:-latest}"
    
    check_prerequisites
    
    case "$action" in
        "deploy-shadow")
            deploy_shadow_model "$model_name" "$model_version"
            setup_traffic_splitting "$model_name" 95 5
            ;;
        "start-ab-test")
            local production_weight="${3:-70}"
            local shadow_weight="${4:-30}"
            setup_traffic_splitting "$model_name" "$production_weight" "$shadow_weight"
            monitor_shadow_testing "$model_name" "$A_B_TEST_DURATION"
            ;;
        "analyze")
            local recommendation=$(analyze_shadow_results "$model_name")
            echo "Recommendation: $recommendation"
            ;;
        "promote")
            promote_shadow_model "$model_name"
            ;;
        "rollback")
            rollback_shadow_model "$model_name"
            ;;
        "full-pipeline")
            log "Starting full ML model deployment pipeline for $model_name"
            
            # Deploy shadow model
            deploy_shadow_model "$model_name" "$model_version"
            
            # Start with shadow testing (5% traffic)
            setup_traffic_splitting "$model_name" 95 5
            monitor_shadow_testing "$model_name" "$SHADOW_TESTING_DURATION"
            
            # Analyze results
            local recommendation=$(analyze_shadow_results "$model_name")
            
            if [[ "$recommendation" == "PROCEED" ]]; then
                # Increase traffic for A/B testing
                setup_traffic_splitting "$model_name" 70 30
                monitor_shadow_testing "$model_name" "$A_B_TEST_DURATION"
                
                # Final analysis
                recommendation=$(analyze_shadow_results "$model_name")
                
                if [[ "$recommendation" == "PROCEED" ]]; then
                    promote_shadow_model "$model_name"
                    log "ML model deployment pipeline completed successfully"
                else
                    rollback_shadow_model "$model_name"
                    error_exit "A/B testing failed: $recommendation"
                fi
            else
                rollback_shadow_model "$model_name"
                error_exit "Shadow testing failed: $recommendation"
            fi
            ;;
        *)
            error_exit "Unknown action: $action. Use: deploy-shadow, start-ab-test, analyze, promote, rollback, or full-pipeline"
            ;;
    esac
}

# Execute main function
main "$@"
