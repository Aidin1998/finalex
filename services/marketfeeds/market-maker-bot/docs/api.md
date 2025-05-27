# Market Maker Bot API Documentation

## Overview
This document describes the REST/WebSocket APIs for the Market Maker Bot, including authentication, endpoints, and example requests/responses.

## Authentication
- API keys are required for all endpoints.
- Keys must be securely stored and transmitted via HTTPS.

## REST Endpoints

### LLM Prediction
- **POST** `/api/llm/predict`
  - Request: `{ "input": "<string>" }`
  - Response: `{ "output": "<string>" }`

### Order Management
- **POST** `/api/order`
  - Place a new order.
- **GET** `/api/order/{id}`
  - Get order status.
- **DELETE** `/api/order/{id}`
  - Cancel order.

### Monitoring
- **GET** `/api/metrics`
  - Prometheus metrics endpoint.

## WebSocket Streams
- **/ws/market**: Real-time market data.
- **/ws/orders**: Order updates and fills.

## Error Handling
- Standard JSON error responses: `{ "error": "<message>" }`

## OpenAPI/Swagger
- See `openapi.yaml` for full schema (to be generated).

## Security
- All endpoints require API key authentication.
- Rate limiting and IP whitelisting enforced.

---
