# Developer Guide

## Onboarding

1. Clone the repository:
   ```powershell
   git clone https://github.com/Aidin1998/pincex_unified.git
   cd pincex_unified
   ```
2. Copy `.env.example` to `.env` and set your environment variables:
   ```env
   PORT=8080
   DB_URL=postgres://user:pass@localhost:5432/pincex
   REDIS_URL=redis://localhost:6379/0
   LOG_LEVEL=debug
   ```
3. Install dependencies:
   ```powershell
   go mod download
   ```
4. Start local services:
   ```powershell
   docker-compose up -d db redis
   ```
5. Run tests:
   ```powershell
   go test ./...  
   ```
6. Build and run:
   ```powershell
   go build -o pincex.exe ./cmd/pincex
   .\pincex.exe
   ```

## Code Conventions

- Use dependency injection via constructors:
  ```go
  NewServer(logger, identitiesSvc, bookkeeperSvc, fiatSvc, marketFeedsSvc, tradingSvc)
  ```
- Centralize error mapping in `errorMapper.mapError` and use `writeError` for HTTP handlers.
- Structured logging with `go.uber.org/zap`.
- Follow standard layout: `cmd/`, `internal/`, `pkg/`.

## Configuration

All configuration is managed by Viper in `internal/config/config.go`. Retrieve values:

```go
cfg := config.Load()
port := cfg.GetString("PORT")
```

## Logging

Use the `pkg/logger` helper to create a global logger:

```go
log := logger.NewLogger(cfg.LogLevel)
```
