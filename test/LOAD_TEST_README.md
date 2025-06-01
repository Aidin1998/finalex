# PinCEX Load & Performance Test Setup

This document describes how to set up, execute, and collect results for comprehensive load and performance testing of the PinCEX Unified Exchange.

---

## 1. Test Scenarios
- **Sustained Load:** 100,000 TPS for 12 hours
- **Traffic Spike:** 150,000 TPS for 30 minutes

## 2. Test Tools
- **k6**: For HTTP API load (JavaScript-based)
- **Locust**: For custom trading workflows (Python-based)

## 3. Test Scripts
- `test/load/sustained_100k_tps.js` (k6, sustained)
- `test/load/spike_150k_tps.js` (k6, spike)
- `test/load/locust_sustained.py` (Locust, sustained)
- `test/load/locust_spike.py` (Locust, spike)

## 4. Setup Instructions

### 4.1 Install Tools
- **k6:**
  - Windows: `choco install k6`
  - macOS/Linux: `brew install k6`
- **Locust:**
  - `pip install locust`

### 4.2 Prepare Environment
- Use a production-like cluster and database.
- Ensure Prometheus, Grafana, and node_exporter are running for resource monitoring.

### 4.3 Run Tests
- **Sustained Load:**
  - `k6 run test/load/sustained_100k_tps.js`
  - `locust -f test/load/locust_sustained.py --headless -u 100000 -r 1000 --run-time 12h`
- **Traffic Spike:**
  - `k6 run test/load/spike_150k_tps.js`
  - `locust -f test/load/locust_spike.py --headless -u 150000 -r 5000 --run-time 30m`

### 4.4 Collect Results
- Export k6/Locust results to `test/results/`
- Archive Grafana dashboard snapshots to `test/results/grafana/`

## 5. Metrics to Record
- Matching engine latency (avg, p50, p90, p99, max)
- API response time (per endpoint)
- Database read/write TPS and latency
- CPU, RAM, disk I/O, network usage
- Error rates (HTTP 4xx/5xx, matching, DB)

## 6. Reporting
- Summarize results in `test/PERFORMANCE_REPORT.md`
- Attach raw CSV/JSON results in `test/results/`

---

For questions, see the test scripts or contact the performance engineering team.
