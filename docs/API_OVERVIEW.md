# API and Interface Documentation Overview

This document provides a summary of all major APIs (REST, gRPC) and Go interfaces in the PinCEX Unified codebase. For full endpoint details, see `docs/API_DOCUMENTATION.md` and `docs/api.md`.

---

## REST API Endpoints (Summary)

See: `README.md`, `docs/API_DOCUMENTATION.md`, `docs/api.md`, and `internal/server/server.go` for full details.

| Path | Method | Description | Handler |
|------|--------|-------------|---------|
| /api/v1/identities/register | POST | Register a new user | handleRegister |
| /api/v1/identities/login | POST | Login a user | handleLogin |
| /api/v1/identities/logout | POST | Logout a user | handleLogout |
| /api/v1/accounts | GET | Get all accounts | handleGetAccounts |
| /api/v1/trading/orders | POST | Place an order | handlePlaceOrder |
| /api/v1/market/prices | GET | Get all market prices | handleGetMarketPrices |
| /api/v1/fiat/deposit | POST | Initiate fiat deposit | handleFiatDeposit |
| ... | ... | ... | ... |

See the full list in the referenced markdown files.

---

## gRPC API (Market Data)

See: `api/marketdata/marketdata_grpc.pb.go`, `internal/marketdata/distribution/transport/grpc_server.go`

- **Service:** MarketData
  - **Subscribe** (stream): Subscribe to real-time market data updates

---

## Major Go Interfaces (Internal)

| Interface | File | Description |
|-----------|------|-------------|
| PatternDetector | internal/compliance/aml/detection/common/pattern_framework.go | AML pattern detection plugin interface |
| DetectionActivity | internal/compliance/aml/detection/common/pattern_framework.go | Activity abstraction for detection |
| TradingEngine | internal/manipulation/detector.go | Trading engine enforcement interface |
| KeyManager | internal/wallet/provider.go | Key management abstraction |
| CustodyProvider | internal/wallet/provider.go | Custody provider abstraction |
| XAResource | internal/transaction/xa_manager.go | Distributed transaction resource interface |
| RecoveryStrategy | internal/transaction/recovery_manager.go | Recovery strategy for transaction manager |
| MetricsObserver | internal/transaction/performance_metrics.go | Observer for metrics changes |
| AlertSubscriber | internal/transaction/monitoring.go | Alert notification interface |
| ConfigWatcher | internal/transaction/config_manager.go | Config change notification interface |
| PubSubBackend | internal/marketdata/pubsub.go | Market data pub/sub backend interface |
| ... | ... | ... |

---

## API Handler Structs

- `TransactionAPI` (internal/transaction/api.go): HTTP endpoints for distributed transaction management
- `MetricsAPI` (internal/analytics/metrics/api.go): HTTP endpoints for business metrics and compliance
- `Server` (internal/server/server.go): Main HTTP server and router
- `GRPCServer` (internal/marketdata/distribution/transport/grpc_server.go): gRPC server for market data
- ...

---

## Notes
- All exported interfaces and handler structs should have GoDoc comments.
- For full endpoint details, see the OpenAPI/Swagger docs and markdown files.
- For gRPC, see the generated `.pb.go` files and proto definitions.

---

_Last updated: 2025-06-03_
