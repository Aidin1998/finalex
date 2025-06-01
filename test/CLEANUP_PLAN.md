# Test Suite Cleanup Plan

## Issues Identified:
1. **Type Redeclarations**: Multiple files define same types (TestSuite, TestCase, TestResult, TestConfig)
2. **Function Redeclarations**: Same test functions defined in multiple files
3. **Missing Dependencies**: Undefined imports (detection, analytics)
4. **Structural Issues**: Tests need reorganization and consolidation

## Resolution Strategy:

### Phase 1: Remove Duplicates and Conflicts
1. Consolidate common types into shared test utilities
2. Remove or rename duplicate test functions
3. Fix missing imports and dependencies

### Phase 2: Organize Test Structure
1. Create organized subdirectories by test category
2. Move tests to appropriate locations
3. Create comprehensive mock infrastructure

### Phase 3: Enhance and Expand
1. Add missing test categories
2. Implement performance verification
3. Create Market Maker tests
4. Add comprehensive reporting

## Files Requiring Immediate Action:

### Conflicting Types (Need Consolidation):
- `test_suite.go` - TestSuite, TestCase, TestResult
- `aml_test_framework.go` - TestSuite, TestCase, TestResult, TestConfig  
- `orderbook_migration_test.go` - TestConfig, TestResult
- `risk_management_test.go` - RiskManagementTestSuite, TestRiskManagementSystem
- `risk_management_integration_test.go` - RiskManagementTestSuite, TestRiskManagementSystem

### Missing Dependencies:
- `aml_test_framework.go` - undefined: detection, analytics

### Files with Duplicate Functions:
- `risk_management_test.go` / `risk_management_integration_test.go` - Multiple function conflicts

## Next Steps:
1. Create consolidated test utilities
2. Remove conflicts and duplicates
3. Fix import issues
4. Test basic compilation
5. Proceed with enhancement and expansion
