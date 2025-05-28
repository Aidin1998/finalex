# Phase 2: Module & Versioning

## Goals
- Consolidate all Go code under a single module at the repository root.
- Remove nested `go.mod` files and convert their code to internal or pkg packages.
- Pin third-party dependencies and run `go mod tidy`.

## Steps
1. Update root `go.mod`:
   - Change module path to `github.com/Aidin1998/pincex_unified` (or your chosen path).
   - Remove any `replace` directives pointing to nested modules.
2. Remove or merge nested `go.mod` files:
   - Delete `repo/common/go.mod` and `services/*/go.mod` if all code is now part of the root module.
3. Update import paths across the codebase:
   - Find any imports that reference `github.com/Aidin1998/pincex/...` and update to the new module path.
   - Ensure no import uses a file-relative module.
4. Run dependency management:
   ```powershell
   cd c:\Orbit CEX\pincex_unified
   go mod tidy
   ```
5. Commit `go.mod` and `go.sum` changes.

## Verification
- Verify `go list ./...` completes without errors.
- Confirm no unexpected module warnings or missing packages.
