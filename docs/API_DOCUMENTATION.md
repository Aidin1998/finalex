# PinCEX Unified API - Comprehensive Documentation

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Rate Limiting](#rate-limiting)
4. [Error Handling](#error-handling)
5. [API Endpoints](#api-endpoints)
6. [WebSocket API](#websocket-api)
7. [Response Formats](#response-formats)
8. [SDKs and Libraries](#sdks-and-libraries)
9. [Examples](#examples)
10. [Changelog](#changelog)

## Overview

The PinCEX Unified API provides comprehensive access to all exchange functionality including trading, account management, market data, and administrative operations. The API follows RESTful principles and provides real-time data through WebSocket connections.

### Base URLs

| Environment | Base URL | Description |
|-------------|----------|-------------|
| Production | `https://api.pincex.com/api/v1` | Live trading environment |
| Sandbox | `https://sandbox-api.pincex.com/api/v1` | Testing environment |

### API Principles

- **RESTful Design**: Standard HTTP methods and status codes
- **JSON Format**: All requests and responses use JSON
- **Idempotency**: Safe operations are idempotent
- **Versioning**: API version in URL path
- **Pagination**: Cursor-based pagination for large datasets

## Authentication

### JWT Bearer Token Authentication

All private endpoints require authentication using JWT bearer tokens.

```http
Authorization: Bearer <your_jwt_token>
```

### Obtaining Tokens

#### Login Endpoint
```http
POST /identities/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "secure_password",
  "mfa_code": "123456"
}
```

**Response:**
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 3600,
  "token_type": "Bearer"
}
```

#### Token Refresh
```http
POST /identities/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### API Key Authentication (Optional)

For server-to-server integration, API keys can be used:

```http
X-API-Key: your_api_key_here
```

## Rate Limiting

API endpoints are protected by intelligent rate limiting to ensure system stability and fair usage.

### Rate Limit Headers

All responses include rate limiting information:

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
X-RateLimit-Tier: premium
```

### Rate Limit Tiers

| Tier | Public Endpoints | Private Endpoints | Trading Endpoints |
|------|------------------|-------------------|-------------------|
| Basic | 100/min | 60/min | 20/min |
| Premium | 300/min | 180/min | 60/min |
| Professional | 1000/min | 600/min | 200/min |
| Institutional | 5000/min | 3000/min | 1000/min |

### Rate Limit Responses

When rate limit is exceeded:

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests. Please try again later.",
  "retry_after": 60,
  "details": {
    "limit": 100,
    "window": 60,
    "current_usage": 101
  }
}
```

## Error Handling

### Standard Error Response Format

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": {
    "field": "Additional error details",
    "trace_id": "req_1234567890abcdef"
  },
  "timestamp": "2025-01-15T10:30:00Z"
}
```

### HTTP Status Codes

| Code | Description | Common Causes |
|------|-------------|---------------|
| 200 | OK | Successful request |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid request parameters |
| 401 | Unauthorized | Missing or invalid authentication |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource not found |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server-side error |
| 503 | Service Unavailable | System maintenance |

### Common Error Codes

| Error Code | Description |
|------------|-------------|
| `invalid_credentials` | Invalid email/password combination |
| `insufficient_balance` | Not enough funds for operation |
| `invalid_order_type` | Unsupported order type |
| `market_closed` | Trading pair is not available |
| `kyc_required` | KYC verification required |
| `mfa_required` | Multi-factor authentication required |
| `order_not_found` | Order ID does not exist |
| `invalid_symbol` | Trading pair not supported |

## API Endpoints

### Health & System Status

#### Health Check
```http
GET /health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-01-15T10:30:00Z",
  "version": "1.0.0",
  "uptime": 86400,
  "services": {
    "database": "healthy",
    "redis": "healthy",
    "trading_engine": "healthy"
  }
}
```

#### System Status
```http
GET /status
```

**Response:**
```json
{
  "trading_status": "open",
  "maintenance_mode": false,
  "supported_currencies": ["BTC", "ETH", "USD", "EUR"],
  "trading_pairs": 150,
  "last_update": "2025-01-15T10:30:00Z"
}
```

### Authentication Endpoints

#### Register New User
```http
POST /identities/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!",
  "first_name": "John",
  "last_name": "Doe",
  "country": "US",
  "accept_terms": true
}
```

**Response:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "status": "pending_verification",
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### Enable Two-Factor Authentication
```http
POST /identities/2fa/enable
Authorization: Bearer <token>
Content-Type: application/json

{
  "method": "totp"
}
```

**Response:**
```json
{
  "qr_code": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...",
  "secret": "JBSWY3DPEHPK3PXP",
  "backup_codes": [
    "123456-789012",
    "345678-901234"
  ]
}
```

### Account Management

#### Get Account Information
```http
GET /accounts
Authorization: Bearer <token>
```

**Response:**
```json
{
  "accounts": [
    {
      "currency": "USD",
      "balance": "10000.50",
      "available": "9500.50",
      "held": "500.00",
      "last_updated": "2025-01-15T10:30:00Z"
    },
    {
      "currency": "BTC",
      "balance": "0.12345678",
      "available": "0.12345678",
      "held": "0.00000000",
      "last_updated": "2025-01-15T10:30:00Z"
    }
  ]
}
```

#### Get Account Transactions
```http
GET /accounts/{currency}/transactions?limit=50&cursor=abc123
Authorization: Bearer <token>
```

**Response:**
```json
{
  "transactions": [
    {
      "id": "txn_1234567890",
      "type": "trade",
      "amount": "-100.00",
      "currency": "USD",
      "description": "Trade execution for order ord_987654321",
      "timestamp": "2025-01-15T10:30:00Z",
      "balance_after": "9900.50"
    }
  ],
  "pagination": {
    "next_cursor": "def456",
    "has_more": true
  }
}
```

### Trading Endpoints

#### Get Trading Pairs
```http
GET /trading/pairs
```

**Response:**
```json
{
  "trading_pairs": [
    {
      "symbol": "BTC/USD",
      "base_currency": "BTC",
      "quote_currency": "USD",
      "status": "trading",
      "min_order_size": "0.00001000",
      "max_order_size": "1000.00000000",
      "price_precision": 2,
      "quantity_precision": 8,
      "maker_fee": "0.001",
      "taker_fee": "0.002"
    }
  ]
}
```

#### Place Order
```http
POST /trading/orders
Authorization: Bearer <token>
Content-Type: application/json

{
  "symbol": "BTC/USD",
  "side": "buy",
  "type": "limit",
  "quantity": "0.01000000",
  "price": "50000.00",
  "time_in_force": "GTC",
  "client_order_id": "my_order_123"
}
```

**Response:**
```json
{
  "order_id": "ord_1234567890abcdef",
  "client_order_id": "my_order_123",
  "symbol": "BTC/USD",
  "side": "buy",
  "type": "limit",
  "quantity": "0.01000000",
  "price": "50000.00",
  "status": "open",
  "filled_quantity": "0.00000000",
  "remaining_quantity": "0.01000000",
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-15T10:30:00Z"
}
```

#### Get Order Status
```http
GET /trading/orders/{order_id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "order_id": "ord_1234567890abcdef",
  "client_order_id": "my_order_123",
  "symbol": "BTC/USD",
  "side": "buy",
  "type": "limit",
  "quantity": "0.01000000",
  "price": "50000.00",
  "status": "partially_filled",
  "filled_quantity": "0.00500000",
  "remaining_quantity": "0.00500000",
  "average_price": "49995.50",
  "total_fees": "0.25",
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-15T10:35:00Z",
  "trades": [
    {
      "trade_id": "trd_abcdef123456",
      "quantity": "0.00500000",
      "price": "49995.50",
      "fee": "0.25",
      "timestamp": "2025-01-15T10:35:00Z"
    }
  ]
}
```

#### Cancel Order
```http
DELETE /trading/orders/{order_id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "order_id": "ord_1234567890abcdef",
  "status": "cancelled",
  "cancelled_at": "2025-01-15T10:40:00Z"
}
```

#### Get Order Book
```http
GET /trading/orderbook/{symbol}?depth=100
```

**Response:**
```json
{
  "symbol": "BTC/USD",
  "timestamp": "2025-01-15T10:30:00Z",
  "sequence": 123456789,
  "bids": [
    ["49995.50", "0.12345678"],
    ["49990.00", "0.25000000"]
  ],
  "asks": [
    ["50000.00", "0.10000000"],
    ["50005.50", "0.15000000"]
  ]
}
```

### Market Data Endpoints

#### Get Current Prices
```http
GET /market/prices?symbols=BTC/USD,ETH/USD
```

**Response:**
```json
{
  "prices": {
    "BTC/USD": {
      "price": "50000.00",
      "change_24h": "2.5",
      "change_24h_percent": "5.25",
      "volume_24h": "1000000.00",
      "high_24h": "51000.00",
      "low_24h": "48500.00",
      "last_updated": "2025-01-15T10:30:00Z"
    },
    "ETH/USD": {
      "price": "3500.00",
      "change_24h": "150.00",
      "change_24h_percent": "4.48",
      "volume_24h": "500000.00",
      "high_24h": "3600.00",
      "low_24h": "3200.00",
      "last_updated": "2025-01-15T10:30:00Z"
    }
  }
}
```

#### Get Candlestick Data
```http
GET /market/candles/{symbol}?interval=1h&start=1640995200&end=1641081600
```

**Response:**
```json
{
  "symbol": "BTC/USD",
  "interval": "1h",
  "candles": [
    {
      "timestamp": 1640995200,
      "open": "49500.00",
      "high": "50200.00",
      "low": "49300.00",
      "close": "50000.00",
      "volume": "125.50000000"
    }
  ]
}
```

### Fiat Operations

#### Initiate Deposit
```http
POST /fiat/deposit
Authorization: Bearer <token>
Content-Type: application/json

{
  "amount": "1000.00",
  "currency": "USD",
  "payment_method": "bank_transfer",
  "bank_account_id": "bank_123456"
}
```

**Response:**
```json
{
  "deposit_id": "dep_1234567890abcdef",
  "amount": "1000.00",
  "currency": "USD",
  "payment_method": "bank_transfer",
  "status": "pending",
  "estimated_completion": "2025-01-17T10:30:00Z",
  "instructions": {
    "bank_name": "Example Bank",
    "account_number": "1234567890",
    "routing_number": "021000021",
    "reference": "DEP1234567890"
  },
  "created_at": "2025-01-15T10:30:00Z"
}
```

#### Request Withdrawal
```http
POST /fiat/withdraw
Authorization: Bearer <token>
Content-Type: application/json

{
  "amount": "500.00",
  "currency": "USD",
  "payment_method": "bank_transfer",
  "bank_account_id": "bank_123456",
  "mfa_code": "123456"
}
```

**Response:**
```json
{
  "withdrawal_id": "wdr_1234567890abcdef",
  "amount": "500.00",
  "currency": "USD",
  "fee": "25.00",
  "net_amount": "475.00",
  "payment_method": "bank_transfer",
  "status": "pending_approval",
  "estimated_completion": "2025-01-17T10:30:00Z",
  "created_at": "2025-01-15T10:30:00Z"
}
```

### Admin Endpoints

#### Create Trading Pair (Admin Only)
```http
POST /admin/trading/pairs
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "base_currency": "DOGE",
  "quote_currency": "USD",
  "min_order_size": "1.00000000",
  "max_order_size": "1000000.00000000",
  "price_precision": 6,
  "quantity_precision": 8,
  "maker_fee": "0.001",
  "taker_fee": "0.002",
  "status": "trading"
}
```

#### Risk Management Endpoints
```http
GET /admin/risk/limits
POST /admin/risk/limits
PUT /admin/risk/limits/{type}/{id}
DELETE /admin/risk/limits/{type}/{id}

GET /admin/risk/compliance/alerts
PUT /admin/risk/compliance/alerts/{alert_id}
POST /admin/risk/compliance/rules
```

## WebSocket API

### Overview
The PinCEX WebSocket API provides a persistent, bi-directional channel for real-time market data and private user events. It follows JSON message structures and supports both public (market data) and private (account-specific) channels.

**Base Endpoint**
```
wss://api.pincex.com/ws/marketdata
```

### Connection Lifecycle
- **Connect**: Open a WebSocket connection to the base endpoint.
- **Heartbeat**: The server sends periodic `ping` messages. Clients must respond with a `pong` to keep the connection alive:
  ```json
  { "type": "pong" }
  ```
- **Reconnect**: On disconnect or error, implement exponential backoff (e.g., 1s, 2s, 5s) and resume subscriptions.
- **Rate Limits**: Limit subscription changes to 10 messages per 30 seconds per connection.

### Authentication (Private Channels)
After connecting, authenticate to access private channels:
```json
{ "type": "auth", "token": "<JWT_TOKEN>" }
```
**Responses**:
- Success:
  ```json
  { "type": "auth_response", "success": true }
  ```
- Failure:
  ```json
  { "type": "auth_response", "success": false, "error": "Invalid token" }
  ```

### Subscription Management
**Subscribe** to one or more channels:
```json
{
  "type": "subscribe",
  "channels": [
    { "name": "ticker",    "symbols": ["BTC/USD", "ETH/USD"] },
    { "name": "orderbook", "symbols": ["BTC/USD"], "depth": 50 },
    { "name": "trades",    "symbols": ["BTC/USD"] }
  ]
}
```
**Unsubscribe**:
```json
{
  "type": "unsubscribe",
  "channels": [ { "name": "ticker", "symbols": ["BTC/USD"] } ]
}
```
**Acknowledge**:
```json
{ "type": "subscribe_response", "success": true }
```

### Message Formats
#### Heartbeat
- **Server Ping**:
  ```json
  { "type": "ping" }
  ```
- **Client Pong**:
  ```json
  { "type": "pong" }
  ```

#### Public Events
- **Ticker Update**:
  ```json
  {
    "type": "ticker",
    "symbol": "BTC/USD",
    "data": {
      "price":      "50000.00",
      "change_24h": "2.5",
      "volume_24h": "1000000.00",
      "timestamp":  "2025-01-15T10:30:00Z"
    }
  }
  ```

- **Order Book Snapshot**:
  ```json
  {
    "type":   "orderbook_snapshot",
    "symbol": "BTC/USD",
    "data": {
      "bids": [["49995.50","0.123"]],
      "asks": [["50005.00","0.456"]],
      "timestamp": "2025-01-15T10:30:00Z"
    }
  }
  ```

- **Order Book Update**:
  ```json
  {
    "type":   "orderbook_update",
    "symbol": "BTC/USD",
    "data": {
      "changes": [["buy","49995.50","0.123"]],
      "timestamp": "2025-01-15T10:31:00Z"
    }
  }
  ```

- **Trade Update**:
  ```json
  {
    "type":   "trade",
    "symbol": "BTC/USD",
    "data": {
      "trade_id": "trd_abcdef123456",
      "price":    "50000.00",
      "quantity": "0.01000000",
      "side":     "buy",
      "timestamp":"2025-01-15T10:30:00Z"
    }
  }
  ```

#### Private Events (Require Auth)
- **Order Update**:
  ```json
  {
    "type": "order_update",
    "data": {
      "order_id":          "ord_1234567890abcdef",
      "status":            "filled",
      "filled_quantity":   "0.01000000",
      "remaining_quantity":"0.00000000",
      "timestamp":         "2025-01-15T10:30:00Z"
    }
  }
  ```

- **Balance Update**:
  ```json
  {
    "type": "balance_update",
    "data": [
      { "currency": "BTC", "available": "0.5", "reserved": "0.1" }
    ],
    "timestamp": "2025-01-15T10:30:00Z"
  }
  ```

#### Error Messages
```json
{ "type": "error", "code": 4001, "message": "Subscription limit exceeded" }
```

### Client Integration Examples
#### JavaScript (Node.js)
```javascript
const WebSocket = require('ws');
const ws = new WebSocket('wss://api.pincex.com/ws/marketdata');
ws.on('open', () => {
  ws.send(JSON.stringify({ type: 'auth', token: 'YOUR_TOKEN' }));
  ws.send(JSON.stringify({ type: 'subscribe', channels: [{ name: 'ticker', symbols: ['BTC/USD'] }] }));
});
ws.on('message', (msg) => console.log(JSON.parse(msg)));
ws.on('ping', () => ws.pong());
```

#### Python
```python
import asyncio, websockets, json
async def run():
    uri = 'wss://api.pincex.com/ws/marketdata'
    async with websockets.connect(uri) as ws:
        await ws.send(json.dumps({"type":"auth","token":"YOUR_TOKEN"}))
        await ws.send(json.dumps({"type":"subscribe","channels":[{"name":"ticker","symbols":["BTC/USD"]}]}))
        async for message in ws:
            print(json.loads(message))
asyncio.run(run())
```

### Troubleshooting & Best Practices
- Verify network/firewall allows WebSocket traffic.
- Ensure JWT token is valid and not expired.
- Respect subscription rate limits to avoid errors.
- Implement automatic reconnection and re-subscription on disconnect.

## Response Formats

### Pagination
Large result sets use cursor-based pagination:

```json
{
  "data": [...],
  "pagination": {
    "next_cursor": "eyJpZCI6MTIzNDU2Nzg5MH0=",
    "prev_cursor": "eyJpZCI6MTIzNDU2NzgwMH0=",
    "has_more": true,
    "limit": 50
  }
}
```

### Timestamps
All timestamps are in ISO 8601 format with UTC timezone:
```
2025-01-15T10:30:00Z
```

### Decimal Numbers
Financial amounts are returned as strings to preserve precision:
```json
{
  "balance": "123.45678900",
  "price": "50000.00"
}
```

## SDKs and Libraries

### Official SDKs

| Language | Repository | Documentation |
|----------|------------|---------------|
| JavaScript/TypeScript | [@pincex/api-client](https://github.com/pincex/js-api-client) | [Docs](https://docs.pincex.com/sdk/js) |
| Python | [pincex-python](https://github.com/pincex/python-api-client) | [Docs](https://docs.pincex.com/sdk/python) |
| Go | [pincex-go](https://github.com/pincex/go-api-client) | [Docs](https://docs.pincex.com/sdk/go) |
| Java | [pincex-java](https://github.com/pincex/java-api-client) | [Docs](https://docs.pincex.com/sdk/java) |

### Quick Start Examples

#### JavaScript/Node.js
```javascript
import { PinCEXClient } from '@pincex/api-client';

const client = new PinCEXClient({
  apiKey: 'your_api_key',
  environment: 'production' // or 'sandbox'
});

// Get account balances
const balances = await client.accounts.getBalances();

// Place a limit order
const order = await client.trading.placeOrder({
  symbol: 'BTC/USD',
  side: 'buy',
  type: 'limit',
  quantity: '0.01',
  price: '50000.00'
});
```

#### Python
```python
from pincex import PinCEXClient

client = PinCEXClient(
    api_key='your_api_key',
    environment='production'  # or 'sandbox'
)

# Get account balances
balances = client.accounts.get_balances()

# Place a limit order
order = client.trading.place_order(
    symbol='BTC/USD',
    side='buy',
    type='limit',
    quantity='0.01',
    price='50000.00'
)
```

## Examples

### Complete Trading Workflow

1. **Authentication**
```bash
curl -X POST https://api.pincex.com/api/v1/identities/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader@example.com",
    "password": "secure_password",
    "mfa_code": "123456"
  }'
```

2. **Check Account Balance**
```bash
curl -X GET https://api.pincex.com/api/v1/accounts \
  -H "Authorization: Bearer <your_token>"
```

3. **Get Market Data**
```bash
curl -X GET "https://api.pincex.com/api/v1/market/prices?symbols=BTC/USD"
```

4. **Place Order**
```bash
curl -X POST https://api.pincex.com/api/v1/trading/orders \
  -H "Authorization: Bearer <your_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTC/USD",
    "side": "buy",
    "type": "limit",
    "quantity": "0.01",
    "price": "50000.00"
  }'
```

5. **Monitor Order Status**
```bash
curl -X GET https://api.pincex.com/api/v1/trading/orders/<order_id> \
  -H "Authorization: Bearer <your_token>"
```

### Error Handling Example

```javascript
try {
  const order = await client.trading.placeOrder({
    symbol: 'BTC/USD',
    side: 'buy',
    type: 'limit',
    quantity: '0.01',
    price: '50000.00'
  });
  console.log('Order placed:', order);
} catch (error) {
  if (error.code === 'insufficient_balance') {
    console.log('Not enough funds to place order');
  } else if (error.code === 'rate_limit_exceeded') {
    console.log('Rate limit exceeded, retry after:', error.retryAfter);
  } else {
    console.log('Unexpected error:', error.message);
  }
}
```

## Changelog

### Version 1.0.0 (Current)
- Initial API release
- Trading endpoints for all order types
- Real-time market data via WebSocket
- Account management and KYC integration
- Fiat deposit/withdrawal support
- Admin endpoints for exchange management
- Comprehensive rate limiting and security

### Upcoming Features
- Options trading endpoints
- Margin trading support
- Advanced order types (OCO, trailing stop)
- Portfolio analytics endpoints
- Social trading features

---

## Support

For API support and questions:
- **Documentation**: [https://docs.pincex.com](https://docs.pincex.com)
- **Support Portal**: [https://support.pincex.com](https://support.pincex.com)
- **Developer Discord**: [https://discord.gg/pincex-dev](https://discord.gg/pincex-dev)
- **Email**: api-support@pincex.com

## Terms of Service

By using the PinCEX API, you agree to our [Terms of Service](https://pincex.com/terms) and [Privacy Policy](https://pincex.com/privacy).
