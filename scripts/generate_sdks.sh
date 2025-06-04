#!/usr/bin/env bash
# Generate client SDKs from Swagger spec
# Requires swagger-codegen-cli installed
SPEC_PATH="docs/swagger.yaml"
SDK_OUTPUT_DIR="docs/sdk"

echo "Generating Go SDK..."
swagger-codegen-cli generate -i "$SPEC_PATH" -l go -o "$SDK_OUTPUT_DIR/go"

echo "Generating JavaScript SDK..."
swagger-codegen-cli generate -i "$SPEC_PATH" -l javascript -o "$SDK_OUTPUT_DIR/javascript"

echo "Generating Python SDK..."
swagger-codegen-cli generate -i "$SPEC_PATH" -l python -o "$SDK_OUTPUT_DIR/python"

echo "SDK generation complete."
