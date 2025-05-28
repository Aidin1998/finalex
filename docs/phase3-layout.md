# Phase 3: Directory Layout Refactoring

## Goals
- Adopt standard Go project layout: `cmd/`, `internal/`, `pkg/`, `docs/`, and remove legacy folders.
- Place each executable in its own `cmd/<name>`.
- Ensure all application-specific code lives under `internal/`.
- Shared libraries and models under `pkg/`.
- Remove the old `api/` folder now that `internal/server` is in place.

## Steps

1. **Remove old `api/` folder**
   - Delete `api/server.go` and entire `api/` directory.
   - Update documentation in `docs/api.md` to point to `internal/server`.

2. **Verify `cmd/pincex` executable**
   - In `cmd/pincex/main.go`, ensure you use `internal/server.NewServer(...)` to wire up handlers.
   - Ensure `cmd/pincex` contains only its `main.go`.

3. **Ensure `internal/` contains only application code**
   - `internal/bookkeeper/`, `internal/fiat/`, `internal/identities/`, `internal/marketfeeds/`, `internal/trading/`, `internal/config/`, `internal/database/`, `internal/server/`.
   - No code should import from paths outside `internal/` or `pkg/`.

4. **Verify `pkg/` contents**
   - `pkg/models` for domain types.
   - `pkg/logger` for logging utilities.
   - Delete any other `pkg/` folders not in use.

5. **Run tests and builds**
   - `go test ./...` should pass.
   - `go build ./cmd/pincex` should produce the executable.

6. **Update root README**
   - Describe project structure, how to build and run the server.

After these steps, the repo will be fully reorganized for Phase 4 (interfaces & DI).
