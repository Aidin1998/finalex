# UserAuth Module Testing Guide

This document outlines the comprehensive test suite for the userauth module. These tests ensure that the user authentication system works perfectly under various scenarios, including real-world pressure conditions.

## Test Categories

The test suite is organized into the following categories:

1. **Basic Registration Tests** (`userauth_registration_test.go`)
   - Tests basic user registration functionality
   - Verifies duplicate user detection

2. **Registration Validation Tests** (`userauth_registration_validation_test.go`)
   - Tests password policy enforcement
   - Tests compliance checks and country restrictions
   - Verifies required field validation

3. **Two-Factor Authentication Tests** (`userauth_2fa_test.go`)
   - Tests TOTP secret generation
   - Tests verification of TOTP codes
   - Tests backup code generation and usage
   - Tests enabling/disabling MFA

4. **Authentication Flow Tests** (`userauth_authentication_test.go`)
   - Tests user authentication with email/password
   - Tests token validation and refresh
   - Tests session management

5. **Stress Tests** (`userauth_stress_test.go`)
   - Tests registration under high concurrency
   - Tests authentication under high load
   - Measures performance metrics

6. **Integration Tests** (`userauth_integration_test.go`)
   - Tests end-to-end flows
   - Tests interactions between different components
   - Validates complex user journeys

## Running the Tests

### Dependencies

Make sure you have the required dependencies:

```bash
go get github.com/stretchr/testify
```

### Basic Tests

To run the basic userauth tests:

```bash
go test -tags userauth ./test
```

### With Coverage

To run tests with coverage reporting:

```bash
go test -tags userauth -coverprofile=coverage.out ./test
go tool cover -html=coverage.out
```

### Stress Tests

To run the stress tests (which may take longer):

```bash
go test -tags "userauth stress" ./test -timeout 5m
```

### Integration Tests

To run the integration tests:

```bash
go test -tags "userauth integration" ./test
```

## Test Data

The tests use in-memory SQLite databases, which are populated with test data as needed. This ensures that tests are isolated and don't interfere with each other.

## Mocked Dependencies

Several services are mocked to enable focused testing:

- `mockEncryptionService`: For encrypting PII data
- `mockComplianceService`: For compliance checks
- `mockAuditService`: For audit logging
- `mockPasswordPolicyEngine`: For password validation
- `mockKYCIntegrationService`: For KYC processes
- `mockNotificationService`: For sending emails and SMS
- `mock2FAService`: For two-factor authentication

## Best Practices

When adding new features to the userauth module, follow these testing best practices:

1. Add unit tests for new functions or methods
2. Update integration tests to cover new user flows
3. Add stress tests for performance-critical paths
4. Ensure that tests cover both success and failure scenarios
5. Check edge cases and security constraints

## Test Coverage Goals

- Line coverage: >80%
- Branch coverage: >75%
- Function coverage: >90%

## Security Testing Notes

The test suite includes various security scenarios:

- Password policy enforcement
- Rate limiting under high concurrency
- Session invalidation
- Token expiration and refresh
- Compliance checks

## Future Enhancements

Future test enhancements planned:

- Penetration testing scenarios
- More extensive compliance testing
- Performance benchmarks against industry standards
- User impersonation detection tests
- Token revocation tests
