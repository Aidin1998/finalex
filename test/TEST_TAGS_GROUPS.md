# Test Tags & Groups

## Market Data
- websocket_client_test.go: [integration, marketdata, websocket, subscribe, receive, client, server]

## Settlement
- settlement_processor_unit_test.go: [unit, settlement, internal, processor]

## Integration Tests
- websocket_server_test.go: [integration, marketdata, websocket, server]
- grpc_server_test.go: [integration, marketdata, grpc, server]
- adaptive_integration_test.go: [integration, trading, adaptive]
- integration/integration_test.go: [integration, multi-module]
- e2e/batch_operations_e2e_test.go: [e2e, batch, operations]
- integration/batch_operations_integration_test.go: [integration, batch, operations]

## E2E Tests
- e2e_exchange_test.go: [e2e, exchange, workflow]
- e2e_latency_benchmark_test.go: [e2e, latency, benchmark]

## AML/Compliance Tests
- aml_risk_management_comprehensive_test.go: [integration, aml, risk, compliance]
- aml_risk_management_comprehensive_test_fixed.go: [integration, aml, risk, compliance]
- aml/screening/pep_screener_test.go: [unit, aml, screening, pep]
- aml/screening/sanctions_screener_test.go: [unit, aml, screening, sanctions]
- aml/screening/sanctions_updater_test.go: [unit, aml, screening, sanctions, updater]
- aml/screening/fuzzy_matcher_test.go: [unit, aml, screening, fuzzy]

## Validation & Security
- validation_comprehensive_test.go: [integration, validation, security, api]
- security_test.go: [integration, security, attacks]
- security_integration_test.go: [integration, security]

## Performance & Concurrency
- performance_benchmark_test.go: [performance, benchmark]
- matching_engine_benchmark_test.go: [performance, matching, engine]
- marketdata_distribution_benchmark_test.go: [performance, marketdata, distribution]
- batch_operations_benchmark_test.go: [performance, batch, operations]
- write_latency_analysis_test.go: [performance, latency]
- service_concurrency_fixed_test.go: [concurrency, service, funds]
- race_condition_fix_test.go: [concurrency, race, market, orderbook, event]

## Trading/Orderbook
- engine_participant_risk_test.go: [integration, trading, risk]
- strong_consistency_test_suite.go: [integration, trading, consistency]

## API/Server
- api_server_test.go: [integration, api, server]
- api_validation_comprehensive_test.go: [integration, api, validation]
- grpc_client_test.go: [integration, grpc, client]

## Miscellaneous
- internal_bookkeeper_service_test.go: [integration, bookkeeper]
- internal_fiat_service_test.go: [integration, fiat]
- internal_marketdata_sample_client.go: [sample, marketdata]
- internal_marketdata_testserver.go: [sample, marketdata]
- internal_marketfeeds_service_test.go: [integration, marketfeeds]
- internal_transaction_testing_framework.go: [framework, transaction]

## Unit Tests (New)
- wallet_unit_test.go: [unit, wallet, full, passing]
- scaling_unit_test.go: [unit, scaling]
- consensus_unit_test.go: [unit, consensus]

## Passing Tests
- websocket_client_test.go: [integration, websocket, marketdata, distribution, end-to-end]
- backpressure_test.go: [unit, backpressure, manager, rate-limiter, capability-detector]

## Test Group Coverage
- Market Data: websocket_client_test.go, backpressure_test.go
- Backpressure: backpressure_test.go
- Distribution: websocket_client_test.go

---

This file groups and tags all test files for selective execution and coverage analysis. Update as new tests are added or tags change. All core integration and unit tests for market data and backpressure are passing and robust. Ready for next round of advanced/edge-case and performance tests.
