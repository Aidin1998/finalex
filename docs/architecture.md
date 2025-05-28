# Architecture Overview and Inventory

This document summarizes Phase 1: Inventory & Dependency Graph for the `pincex_unified` monorepo.

## Services and Their Packages

- `internal/identities` (pkg: `github.com/Aidin1998/pincex/internal/identities`)
  - Core user management: registration, authentication, 2FA, KYC, API keys (currently stubbed)
  - Dependencies: `database`, `logger`, `models`

- `internal/bookkeeper` (pkg: `github.com/Aidin1998/pincex/internal/bookkeeper`)
  - Account & transaction bookkeeping
  - Dependencies: `database`, `models`

- `internal/fiat` (pkg: `github.com/Aidin1998/pincex/internal/fiat`)
  - Fiat deposit/withdrawal logic
  - Dependencies: `bookkeeper`, `database`, `models`

- `internal/marketfeeds` (pkg: `github.com/Aidin1998/pincex/internal/marketfeeds`)
  - Market data: prices, summaries, candles
  - Dependencies: external feed clients, `models`

- `internal/trading` (pkg: `github.com/Aidin1998/pincex/internal/trading`)
  - Order placement, cancellation, matching via `engine` & `orderbook`
  - Dependencies: `bookkeeper`, `models`, `trading/engine`, `trading/orderbook`

- `internal/server` (pkg: `github.com/Aidin1998/pincex/internal/server`)
  - HTTP server setup and routing wrappers
  - Dependencies: `gin`, `otel`, `logger`

- `cmd/pincex` (executable)
  - Orchestrates all services and starts HTTP server
  - Dependencies: `internal/*`, `pkg/logger`, `pkg/models`

- `api/server.go` (temporary monolithic HTTP handlers)
  - Current HTTP handlers, many stubbed out, to be refactored into `internal/server`

- `pkg/logger` (pkg)
  - Zap logger wrapper

- `pkg/models` (pkg)
  - Shared domain models across services

## Shared Utilities and Third-Party Dependencies

- Database drivers: `gorm.io/gorm`, `gorm.io/driver/postgres`
- Redis: `github.com/redis/go-redis/v9`
- Logging: `go.uber.org/zap`
- HTTP router & middleware: `github.com/gin-gonic/gin`, `gin-contrib/zap`, `otelgin`
- JWT: `github.com/golang-jwt/jwt/v5`
- Config: `github.com/spf13/viper`, `github.com/joho/godotenv`
- UUID: `github.com/google/uuid`

## Next Steps

1. **Phase 2 (Module & Versioning)**: Consolidate to one root `go.mod`, remove nested modules, pin versions, run `go mod tidy`.  
2. **Phase 3 (Directory Layout)**: Move code into `cmd/`, `internal/`, `pkg/`, remove legacy `api/server.go`.  
3. **Phase 4 (Interfaces & DI)**: Define clear service interfaces in each package and wire via constructors in `cmd/*`.  
4. **Phase 5 (Error Handling & Logging)**: Standardize error wrapping and structured logging across packages.  
5. **Phase 6 (Testing)**: Add unit tests, integration tests, E2E tests, and CI config.  
6. **Phase 7 (Docs & Diagrams)**: Finalize developer docs, update README, architecture diagrams.
