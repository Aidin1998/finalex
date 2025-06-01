# Advanced Monitoring Dashboards and Alerts

## Overview
This document describes the advanced monitoring and alerting setup for PinCEX Unified Exchange, covering all critical system components. It includes real-time dashboards, automated alerting, and recommended response actions.

---

## 1. Metrics Tracked

### Matching Engine
- `pincex_orders_processed_total` (by side): Total orders processed
- `pincex_order_processing_latency_seconds`: Order processing latency (histogram)
- `trading_order_processing_latency_p95`: P95 latency (Prometheus query)
- `trading_errors_total`: Error count
- `conversion_rate`, `fill_rate`, `avg_slippage`, `spread`, `market_impact`: Business metrics

### Database
- `pincex_db_open_connections`, `pincex_db_idle_connections`, `pincex_db_in_use_connections`: Connection pool
- `db_query_duration_seconds`: Query latency (histogram)
- `db_cache_hit_rate`: Cache hit rate
- `db_replication_lag_seconds`: Replication lag

### WebSocket
- `ws_active_connections`: Active connections
- `ws_shard_utilization`: Shard usage
- `ws_message_throughput`: Message throughput

### AML/Compliance
- `aml_queue_length`: Queue length
- `aml_retry_rate`: Retry rate
- `aml_circuit_breaker_open`: Circuit breaker status

### API
- `pincex_http_requests_total`: HTTP requests (by path, method, status)
- `pincex_http_request_duration_seconds`: Request latency (histogram)
- `api_5xx_responses_total`: 5xx error count

### Resource Utilization
- `node_cpu_utilization`, `node_memory_utilization`, `node_disk_utilization`: Node exporter

---

## 2. Grafana Dashboards

- **Trading Performance Dashboard**: Order rates, latency, error rates, migration progress
- **System Health Dashboard**: CPU, memory, GC, goroutines
- **Database Dashboard**: Query latency, connection pool, cache hit rate, replication lag
- **WebSocket Dashboard**: Connections, shards, throughput
- **AML/Compliance Dashboard**: Queue, retries, circuit breaker
- **API Dashboard**: Endpoint health, error rates, latency

Example dashboard JSON: `internal/analytics/metrics/dashboard_grafana.json`

---

## 3. Prometheus Alerting Rules

- **High Matching Engine Latency**: `trading_order_processing_latency_p95 > 0.001` (1ms)
- **High DB Latency**: `db_query_duration_seconds_p95 > 0.01` (10ms)
- **High Resource Usage**: `node_cpu_utilization > 0.8`, `node_memory_utilization > 0.8`, `node_disk_utilization > 0.8`
- **High Error Rate**: `rate(api_5xx_responses_total[5m]) > 0.01`
- **AML Circuit Breaker**: `aml_circuit_breaker_open == 1`
- **Anomaly Detection**: Use `predict_linear()` or `increase()` for historical deviation

Example alert YAML:
```yaml
groups:
  - name: pincex_critical
    rules:
      - alert: HighMatchingEngineLatency
        expr: trading_order_processing_latency_p95 > 0.001
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Matching engine latency >1ms"
      - alert: HighDBLatency
        expr: db_query_duration_seconds_p95 > 0.01
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Database query latency >10ms"
      - alert: HighCPU
        expr: node_cpu_utilization > 0.8
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "CPU usage >80%"
      - alert: HighErrorRate
        expr: rate(api_5xx_responses_total[5m]) > 0.01
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "API 5xx error rate >1%"
      - alert: AMLCircuitBreaker
        expr: aml_circuit_breaker_open == 1
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "AML circuit breaker open"
```

---

## 4. Alert Response Actions

- **Latency/Resource Alerts**: Check recent deployments, resource usage, and logs. Scale up if needed.
- **Error Rate Alerts**: Review logs, recent code changes, and database health. Roll back if necessary.
- **AML Circuit Breaker**: Investigate AML queue, retry rates, and upstream data sources.
- **Anomaly Alerts**: Compare with historical trends, check for DDoS or abnormal user activity.

---

## 5. Deployment & Operations

- Prometheus scrapes `/metrics` endpoints on all services.
- Dashboards are imported into Grafana (`internal/analytics/metrics/dashboard_grafana.json`).
- Alert rules are loaded into Prometheus Alertmanager.
- All metrics and dashboards are documented here and in subsystem deployment guides.

---

## 6. References
- See `internal/database/DEPLOYMENT_GUIDE.md` and `docs/adaptive_orderbook/DEPLOYMENT_GUIDE.md` for subsystem-specific monitoring.
- See `test/PERFORMANCE_REPORT.md` for load test metrics and targets.
