# Trading Module Migration Guide

## Overview

This guide outlines the transition from legacy trading components to the new optimized implementation.

## Deprecated Components

The following components have been deprecated and should not be used in new code:

### Deprecated Files
These files have been renamed with the `.legacy` extension and should not be imported:

- `orderqueue/badger_queue.go.legacy`: BadgerDB-based implementation replaced by in-memory queue
- `orderqueue/enhanced_queue.go.legacy`: Complex queue with unnecessary features
- `orderqueue/snapshot_store.go.legacy`: Snapshot mechanism not needed with new architecture
- `orderqueue/failover.go.legacy`: Replaced by simpler, more robust approach
- `coordination/service.go`: Coordination logic moved to main trading service
- `coordination/trade_settlement_coordinator.go`: Replaced by direct settlement API
- `engine/adaptive_engine.go`: Now integrated with main engine
- `integration/service.go`: Functions moved to main trading service
- `middleware/rate_limiter.go`: Replaced by more efficient implementation
- `migration/participants/engine_participant.go`: Migration now handled directly
- `migration/orchestrator.go`: Migration now handled directly
- `interface.go`: Contains `LegacyService` interface - use `TradingService` instead

### Replacement Architecture

- The core trading service is now defined by the `TradingService` interface in `service.go`
- The adaptive trading capabilities are defined by the `AdaptiveTradingService` interface in `adaptive_service.go`
- Order queuing now uses the efficient `InMemoryQueue` implementation in `orderqueue/inmemory_queue.go`
- Settlement processing uses the implementation in the `settlement` package

## Migration Path

1. Use `TradingService` interface for standard trading operations
2. Use `AdaptiveTradingService` for advanced order routing and management
3. Use `orderqueue.QueueProvider` interface for queue operations
4. Update any import paths to point to the new implementations

## Implementation Notes

- Thread-safety has been improved across all components
- Memory management is more efficient
- Performance is significantly better under high load
- Error handling is more consistent

## Compatibility

Type aliases have been provided in various files to maintain backward compatibility:

- `trading/settlement_processor.go`: Provides aliases to `settlement` package types
- `trading/engine_types.go`: Provides aliases to `engine` package types

## Metrics and Monitoring

All new components expose standard metrics for monitoring:

- Queue depth and processing rates
- Order matching latency
- Settlement processing time
- Error rates and types

## Support

Please direct any migration questions to the Trading Systems team.
