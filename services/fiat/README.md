# Fiat Gateway Service

## Architecture
- **Go microservice** for fiat deposit, exchange, and rates aggregation
- **REST API**: /fiat/deposit, /fiat/exchange, /fiat/rates
- **CockroachDB** for wallet and exchange logs
- **Stripe** for fiat onramp
- **Rate Aggregator**: Binance, Coinbase, Kraken, Bitfinex, OKX
- **Admin API**: Fee configs, RBAC, JWT
- **Observability**: Prometheus, Jaeger, ELK/Sentry

## Runbook
### Local Dev
1. `make migrate` (apply DB migrations)
2. `make run` (start service)
3. Set env vars: STRIPE_KEY, DB_URL, etc. (see `.env.example`)
4. Run tests: `make test`

### Stripe Webhook
- Set `STRIPE_WEBHOOK_SECRET` in env
- Expose `/webhook` endpoint to Stripe

### DB
- CockroachDB cluster (see `migrations/`)
- Use testcontainers or Docker Compose for local dev

## Scaling Guide
- Deploy with Kubernetes (see manifests below)
- Use HPA to scale pods to handle ≥1,000 req/s
- Use Redis for rate cache if needed
- Use managed CockroachDB for prod

## Observability
- Prometheus metrics at `/metrics`
- Jaeger tracing via OpenTelemetry
- Logs: JSON to stdout (ship to ELK/Sentry)

---

# Kubernetes Manifests

## deployment.yaml
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fiat-gateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: fiat-gateway
  template:
    metadata:
      labels:
        app: fiat-gateway
    spec:
      containers:
      - name: fiat-gateway
        image: yourrepo/fiat-gateway:latest
        envFrom:
        - secretRef:
            name: fiat-gateway-secrets
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
          limits:
            cpu: "2"
            memory: "2Gi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
```

## service.yaml
```yaml
apiVersion: v1
kind: Service
metadata:
  name: fiat-gateway
spec:
  selector:
    app: fiat-gateway
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
  type: ClusterIP
```

## hpa.yaml
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: fiat-gateway-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: fiat-gateway
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
```

---

# Helm/Terraform
- Use Helm for templating manifests and secret rollout
- Use Terraform for cloud resources (DB, Redis, S3, etc.)
- Store secrets in Kubernetes Secrets or HashiCorp Vault

---

# Observability
- **Prometheus**: `/metrics` endpoint
- **Jaeger**: Instrument with OpenTelemetry SDK
- **Logs**: Structured JSON, ship to ELK/Sentry
- **Alerting**: Set up Prometheus alerts for latency, errors, and pod health

---

For more, see `fiat-gateway-spec.md` and `docs/openapi.yaml`.
