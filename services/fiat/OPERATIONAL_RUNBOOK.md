# Fiat Gateway Operational Runbook

## Dashboards

### 1. Prometheus + Grafana
- **Import the following dashboards:**
  - HTTP Latency & Throughput: Visualize `http_requests_total` and `http_request_duration_seconds` by endpoint.
  - Error Rates: Track error codes and alert on spikes.
  - System Health: Uptime, memory, CPU, and `/health` endpoint status.
  - Balances & Transactions: (If metrics exported) plot wallet balances, exchange volume, and deposit/withdrawal rates.

#### Example Prometheus Queries
- `sum(rate(http_requests_total[1m])) by (handler, code)`
- `histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, handler))`

### 2. Custom Metrics (Optional)
- Export business metrics (e.g., `fiat_wallet_balance`, `exchange_volume_total`) via Prometheus if needed for dashboards.

---

## Operational Runbooks

### Routine Operations
- **Deployment:**
  - Use Helm or kubectl to deploy new versions.
  - Monitor `/metrics` and `/health` endpoints post-deploy.
- **Scaling:**
  - HPA auto-scales pods based on CPU; adjust thresholds as needed.
  - For manual scaling: `kubectl scale deployment fiat-gateway --replicas=N`
- **Config Reload:**
  - Publish to Redis channel `fiat:config:reload` to trigger hot-reload.
- **Database Migrations:**
  - Run `make migrate` or use migration tool before new releases.

### Emergency Scenarios
- **High Latency or Errors:**
  - Check Grafana dashboards for spikes in latency or error rates.
  - Inspect logs in ELK/Sentry for stack traces.
  - Roll back to previous deployment if needed.
- **Node Failure:**
  - HPA and K8s will reschedule pods automatically.
  - Check `/health` and pod logs for root cause.
- **Data Inconsistency:**
  - Audit logs and DB state for recent changes.
  - Use backup/restore procedures if needed.
- **Security Incident:**
  - Rotate secrets via K8s or Vault.
  - Review audit logs and restrict access.

---

## References
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
- [Grafana Dashboards](https://grafana.com/grafana/dashboards)
- [Kubernetes Docs](https://kubernetes.io/docs/)
- [Stripe Webhook Security](https://stripe.com/docs/webhooks)

---

For more, see `README.md` and `docs/openapi.yaml`.
