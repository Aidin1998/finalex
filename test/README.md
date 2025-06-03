# Test Suite Structure

All test helpers, mocks, and test utilities have been moved to this folder. Each test file is tagged and grouped by package for better error handling and result checking.

- All files with `+build testhelpers` are helpers/utilities for tests.
- All integration, e2e, and unit tests are in this folder or its subfolders.
- To run all tests: `go test ./test/...`
