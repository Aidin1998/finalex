#!/usr/bin/env bash
# Generate client SDKs from OpenAPI spec
# Requires openapi-generator-cli installed
SPEC_PATH="docs/openapi.yaml"
SDK_OUTPUT_DIR="docs/sdk"

echo "Generating Go SDK..."
openapi-generator-cli generate -i "$SPEC_PATH" -g go -o "$SDK_OUTPUT_DIR/go"

echo "Generating JavaScript SDK..."
openapi-generator-cli generate -i "$SPEC_PATH" -g javascript -o "$SDK_OUTPUT_DIR/javascript"

echo "Generating Python SDK..."
openapi-generator-cli generate -i "$SPEC_PATH" -g python -o "$SDK_OUTPUT_DIR/python"

echo "SDK generation complete."
