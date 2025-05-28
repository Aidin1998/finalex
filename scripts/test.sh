#!/bin/bash

# Test script for Pincex

set -e

echo "Running tests for Pincex..."

# Run unit tests
echo "Running unit tests..."
go test -v ./...

# Run integration tests if database is available
if [ ! -z "$PINCEX_DATABASE_DSN" ]; then
    echo "Running integration tests..."
    go test -v -tags=integration ./...
else
    echo "Skipping integration tests (PINCEX_DATABASE_DSN not set)"
fi

# Run benchmarks
echo "Running benchmarks..."
go test -v -bench=. -benchmem ./internal/trading/...

echo "All tests completed successfully!"
