# API Layer

This directory contains the API layer components for the Finalex platform.

## Structure

- `handlers/` - HTTP request handlers organized by domain
- `middleware/` - API-specific middleware components  
- `routes/` - Route definitions and grouping
- `validators/` - API request/response validation
- `responses/` - Standardized response formatting
- `docs/` - API documentation and schemas

## Design Principles

- **Separation of Concerns**: Each handler focuses on a specific business domain
- **Standardized Responses**: All responses follow RFC 7807 Problem Details standard
- **Comprehensive Validation**: Multi-layer validation with security hardening
- **Error Handling**: Unified error handling with proper logging and tracing
- **Performance**: Optimized for high-throughput trading operations

## Usage

API components are designed to be imported and used by the main application servers in `/cmd`.
