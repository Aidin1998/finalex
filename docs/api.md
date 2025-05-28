# Pincex Crypto Exchange API Documentation

## Overview

This document provides comprehensive documentation for the Pincex Crypto Exchange API. The API allows users to access market data, manage accounts, place trades, and perform various other operations related to cryptocurrency trading.

## Base URL

All API endpoints are relative to the base URL:

```
https://api.pincex.com/api/v1
```

## Authentication

Most API endpoints require authentication. To authenticate, include an `Authorization` header with a Bearer token:

```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

You can obtain an access token by calling the `/auth/login` endpoint.

## Rate Limiting

API requests are rate-limited to protect the system from abuse. The current limits are:

- Public endpoints: 100 requests per minute
- Private endpoints: 60 requests per minute
- Trading endpoints: 20 requests per minute

Rate limit information is included in the response headers:

- `X-RateLimit-Limit`: The maximum number of requests allowed per minute
- `X-RateLimit-Remaining`: The number of requests remaining in the current window
- `X-RateLimit-Reset`: The time at which the current rate limit window resets (Unix timestamp)

## Error Handling

The API uses standard HTTP status codes to indicate the success or failure of a request. In case of an error, the response body will contain a JSON object with an `error` field describing the error:

```json
{
  "error": "Invalid credentials"
}
```

Common error codes:

- `400 Bad Request`: The request was malformed or contained invalid parameters
- `401 Unauthorized`: Authentication is required or failed
- `403 Forbidden`: The authenticated user does not have permission to access the requested resource
- `404 Not Found`: The requested resource was not found
- `429 Too Many Requests`: The rate limit has been exceeded
- `500 Internal Server Error`: An unexpected error occurred on the server

## Endpoints

### Public Endpoints

#### Health Check

```
GET /health
```

Returns the current status of the API.

**Response:**

```json
{
  "status": "ok",
  "time": "2025-05-27T07:00:00Z"
}
```

#### Market Data

##### Get All Market Prices

```
GET /market/prices
```

Returns the current prices for all trading pairs.

**Response:**

```json
{
  "prices": [
    {
      "symbol": "BTC/USD",
      "price": 75000.0,
      "change_24h": 2.5,
      "volume_24h": 1000000.0,
      "high_24h": 76000.0,
      "low_24h": 74000.0,
      "updated_at": "2025-05-27T07:00:00Z"
    },
    {
      "symbol": "ETH/USD",
      "price": 4500.0,
      "change_24h": 1.8,
      "volume_24h": 500000.0,
      "high_24h": 4600.0,
      "low_24h": 4400.0,
      "updated_at": "2025-05-27T07:00:00Z"
    }
  ]
}
```

##### Get Market Price

```
GET /market/price/:symbol
```

Returns the current price for a specific trading pair.

**Parameters:**

- `symbol`: The trading pair symbol (e.g., `BTC/USD`)

**Response:**

```json
{
  "symbol": "BTC/USD",
  "price": 75000.0,
  "change_24h": 2.5,
  "volume_24h": 1000000.0,
  "high_24h": 76000.0,
  "low_24h": 74000.0,
  "updated_at": "2025-05-27T07:00:00Z"
}
```

##### Get Market Summary

```
GET /market/summary/:symbol
```

Returns a summary of market data for a specific trading pair.

**Parameters:**

- `symbol`: The trading pair symbol (e.g., `BTC/USD`)

**Response:**

```json
{
  "symbol": "BTC/USD",
  "price": 75000.0,
  "change_24h": 2.5,
  "volume_24h": 1000000.0,
  "high_24h": 76000.0,
  "low_24h": 74000.0,
  "updated_at": "2025-05-27T07:00:00Z"
}
```

##### Get Market History

```
GET /market/history/:symbol
```

Returns historical market data for a specific trading pair.

**Parameters:**

- `symbol`: The trading pair symbol (e.g., `BTC/USD`)

**Query Parameters:**

- `interval`: The time interval for the data points (default: `1h`). Possible values: `1m`, `5m`, `15m`, `1h`, `4h`, `1d`
- `limit`: The maximum number of data points to return (default: `100`)

**Response:**

```json
{
  "symbol": "BTC/USD",
  "interval": "1h",
  "history": [
    {
      "timestamp": "2025-05-27T06:00:00Z",
      "open": 74800.0,
      "high": 75200.0,
      "low": 74600.0,
      "close": 75000.0,
      "volume": 10000.0
    },
    {
      "timestamp": "2025-05-27T05:00:00Z",
      "open": 74600.0,
      "high": 75000.0,
      "low": 74500.0,
      "close": 74800.0,
      "volume": 9500.0
    }
  ]
}
```

##### Get Order Book

```
GET /market/orderbook/:symbol
```

Returns the current order book for a specific trading pair.

**Parameters:**

- `symbol`: The trading pair symbol (e.g., `BTC/USD`)

**Query Parameters:**

- `depth`: The number of price levels to return on each side (default: `10`)

**Response:**

```json
{
  "symbol": "BTC/USD",
  "bids": [
    [74950.0, 1.5],
    [74900.0, 2.0],
    [74850.0, 2.5]
  ],
  "asks": [
    [75050.0, 1.2],
    [75100.0, 1.8],
    [75150.0, 2.2]
  ],
  "timestamp": "2025-05-27T07:00:00Z"
}
```

Each bid and ask is represented as an array of [price, size].

##### Get Recent Trades

```
GET /market/trades/:symbol
```

Returns recent trades for a specific trading pair.

**Parameters:**

- `symbol`: The trading pair symbol (e.g., `BTC/USD`)

**Query Parameters:**

- `limit`: The maximum number of trades to return (default: `50`)

**Response:**

```json
{
  "symbol": "BTC/USD",
  "trades": [
    {
      "id": "t1621234567890",
      "price": 75000.0,
      "size": 0.5,
      "side": "buy",
      "timestamp": "2025-05-27T06:59:50Z"
    },
    {
      "id": "t1621234567891",
      "price": 74950.0,
      "size": 0.3,
      "side": "sell",
      "timestamp": "2025-05-27T06:59:40Z"
    }
  ]
}
```

##### Get Trading Pairs

```
GET /market/pairs
```

Returns all available trading pairs.

**Response:**

```json
{
  "pairs": [
    {
      "symbol": "BTC/USD",
      "base_asset": "BTC",
      "quote_asset": "USD",
      "min_quantity": 0.0001,
      "max_quantity": 1000.0,
      "price_precision": 2,
      "quantity_precision": 6,
      "status": "active"
    },
    {
      "symbol": "ETH/USD",
      "base_asset": "ETH",
      "quote_asset": "USD",
      "min_quantity": 0.001,
      "max_quantity": 1000.0,
      "price_precision": 2,
      "quantity_precision": 6,
      "status": "active"
    }
  ]
}
```

#### Authentication

##### Register

```
POST /auth/register
```

Registers a new user.

**Request:**

```json
{
  "email": "user@example.com",
  "username": "user123",
  "password": "securepassword",
  "first_name": "John",
  "last_name": "Doe"
}
```

**Response:**

```json
{
  "user": {
    "id": "00000000-0000-0000-0000-000000000001",
    "email": "user@example.com",
    "username": "user123",
    "first_name": "John",
    "last_name": "Doe"
  },
  "token": "example_token"
}
```

##### Login

```
POST /auth/login
```

Authenticates a user and returns an access token.

**Request:**

```json
{
  "login": "user@example.com",
  "password": "securepassword"
}
```

**Response:**

If 2FA is not enabled:

```json
{
  "user": {
    "id": "00000000-0000-0000-0000-000000000001",
    "email": "user@example.com",
    "username": "user123",
    "first_name": "John",
    "last_name": "Doe"
  },
  "token": "example_token"
}
```

If 2FA is enabled:

```json
{
  "requires_2fa": true,
  "user_id": "00000000-0000-0000-0000-000000000001"
}
```

##### Refresh Token

```
POST /auth/refresh
```

Refreshes an access token.

**Request:**

```json
{
  "refresh_token": "example_refresh_token"
}
```

**Response:**

```json
{
  "token": "new_example_token",
  "refresh_token": "new_example_refresh_token"
}
```

##### Logout

```
POST /auth/logout
```

Logs out a user by invalidating their access token.

**Response:**

```json
{
  "message": "Logged out successfully"
}
```

#### WebSocket

```
GET /ws
```

Establishes a WebSocket connection for real-time market data.

**WebSocket Messages:**

To subscribe to a channel:

```json
{
  "method": "subscribe",
  "params": {
    "channel": "ticker",
    "symbol": "BTC/USD"
  }
}
```

To unsubscribe from a channel:

```json
{
  "method": "unsubscribe",
  "params": {
    "channel": "ticker",
    "symbol": "BTC/USD"
  }
}
```

**WebSocket Responses:**

Subscription confirmation:

```json
{
  "event": "subscribed",
  "channel": "ticker",
  "symbol": "BTC/USD"
}
```

Ticker update:

```json
{
  "channel": "ticker",
  "symbol": "BTC/USD",
  "data": {
    "symbol": "BTC/USD",
    "price": 75000.0,
    "bid_price": 74950.0,
    "ask_price": 75050.0,
    "volume_24h": 1000000.0,
    "high_24h": 76000.0,
    "low_24h": 74000.0,
    "change_24h": 2.5,
    "timestamp": "2025-05-27T07:00:00Z"
  }
}
```

### Private Endpoints

#### User

##### Get User Profile

```
GET /user/profile
```

Returns the authenticated user's profile.

**Response:**

```json
{
  "user": {
    "id": "00000000-0000-0000-0000-000000000001",
    "email": "user@example.com",
    "username": "user123",
    "first_name": "John",
    "last_name": "Doe",
    "kyc_status": "approved",
    "two_fa_enabled": true
  }
}
```

##### Update User Profile

```
PUT /user/profile
```

Updates the authenticated user's profile.

**Request:**

```json
{
  "first_name": "John",
  "last_name": "Smith"
}
```

**Response:**

```json
{
  "user": {
    "id": "00000000-0000-0000-0000-000000000001",
    "email": "user@example.com",
    "username": "user123",
    "first_name": "John",
    "last_name": "Smith",
    "kyc_status": "approved",
    "two_fa_enabled": true
  }
}
```

##### Change Password

```
PUT /user/password
```

Changes the authenticated user's password.

**Request:**

```json
{
  "current_password": "securepassword",
  "new_password": "newsecurepassword"
}
```

**Response:**

```json
{
  "message": "Password changed successfully"
}
```

##### Enable 2FA

```
POST /user/2fa/enable
```

Enables two-factor authentication for the authenticated user.

**Response:**

```json
{
  "secret": "example_2fa_secret",
  "qr_code_url": "example_qr_code_url"
}
```

##### Verify 2FA

```
POST /user/2fa/verify
```

Verifies a two-factor authentication token and enables 2FA for the authenticated user.

**Request:**

```json
{
  "secret": "example_2fa_secret",
  "token": "123456"
}
```

**Response:**

```json
{
  "message": "2FA enabled successfully"
}
```

##### Disable 2FA

```
POST /user/2fa/disable
```

Disables two-factor authentication for the authenticated user.

**Request:**

```json
{
  "password": "securepassword"
}
```

**Response:**

```json
{
  "message": "2FA disabled successfully"
}
```

##### Get KYC Status

```
GET /user/kyc/status
```

Returns the authenticated user's KYC status.

**Response:**

```json
{
  "status": "approved"
}
```

Possible status values: `pending`, `approved`, `rejected`

##### Submit KYC Document

```
POST /user/kyc/document
```

Submits a KYC document for verification.

**Request:**

```json
{
  "type": "passport",
  "file_path": "/path/to/document.jpg"
}
```

**Response:**

```json
{
  "document": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "type": "passport",
    "status": "pending",
    "created_at": "2025-05-27T07:00:00Z"
  }
}
```

##### Get KYC Documents

```
GET /user/kyc/documents
```

Returns the authenticated user's KYC documents.

**Response:**

```json
{
  "documents": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "type": "passport",
      "status": "approved",
      "created_at": "2025-05-27T07:00:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "type": "proof_of_address",
      "status": "pending",
      "created_at": "2025-05-27T07:00:00Z"
    }
  ]
}
```

##### Get API Keys

```
GET /user/api-keys
```

Returns the authenticated user's API keys.

**Response:**

```json
{
  "api_keys": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "name": "Trading Bot",
      "permissions": "trade",
      "ip_whitelist": "192.168.1.1",
      "created_at": "2025-05-27T07:00:00Z"
    }
  ]
}
```

##### Create API Key

```
POST /user/api-keys
```

Creates a new API key for the authenticated user.

**Request:**

```json
{
  "name": "Trading Bot",
  "permissions": "trade",
  "ip_whitelist": "192.168.1.1"
}
```

**Response:**

```json
{
  "api_key": {
    "id": "00000000-0000-0000-0000-000000000001",
    "name": "Trading Bot",
    "permissions": "trade",
    "ip_whitelist": "192.168.1.1",
    "created_at": "2025-05-27T07:00:00Z"
  },
  "secret": "example_api_key_secret"
}
```

The `secret` is only returned once and should be stored securely.

##### Delete API Key

```
DELETE /user/api-keys/:id
```

Deletes an API key.

**Parameters:**

- `id`: The API key ID

**Response:**

```json
{
  "message": "API key deleted successfully"
}
```

#### Account

##### Get Accounts

```
GET /account
```

Returns all accounts for the authenticated user.

**Response:**

```json
{
  "accounts": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "BTC",
      "balance": 1.5,
      "available": 1.2,
      "locked": 0.3,
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T07:00:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "USD",
      "balance": 10000.0,
      "available": 8000.0,
      "locked": 2000.0,
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T07:00:00Z"
    }
  ]
}
```

##### Get Account

```
GET /account/:currency
```

Returns a specific account for the authenticated user.

**Parameters:**

- `currency`: The account currency (e.g., `BTC`)

**Response:**

```json
{
  "account": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "currency": "BTC",
    "balance": 1.5,
    "available": 1.2,
    "locked": 0.3,
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  }
}
```

##### Get Transactions

```
GET /account/transactions
```

Returns transactions for the authenticated user.

**Query Parameters:**

- `limit`: The maximum number of transactions to return (default: `20`)
- `offset`: The number of transactions to skip (default: `0`)

**Response:**

```json
{
  "transactions": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "type": "credit",
      "amount": 1.0,
      "currency": "BTC",
      "status": "completed",
      "reference": "deposit",
      "description": "Bitcoin deposit",
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T07:00:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "type": "debit",
      "amount": 0.5,
      "currency": "BTC",
      "status": "completed",
      "reference": "withdrawal",
      "description": "Bitcoin withdrawal",
      "created_at": "2025-05-27T06:00:00Z",
      "updated_at": "2025-05-27T06:00:00Z"
    }
  ],
  "pagination": {
    "total": 10,
    "limit": 20,
    "offset": 0
  }
}
```

##### Get Transaction

```
GET /account/transaction/:id
```

Returns a specific transaction for the authenticated user.

**Parameters:**

- `id`: The transaction ID

**Response:**

```json
{
  "transaction": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "type": "credit",
    "amount": 1.0,
    "currency": "BTC",
    "status": "completed",
    "reference": "deposit",
    "description": "Bitcoin deposit",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  },
  "entries": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "transaction_id": "00000000-0000-0000-0000-000000000001",
      "account_id": "00000000-0000-0000-0000-000000000001",
      "type": "credit",
      "amount": 1.0,
      "currency": "BTC",
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T07:00:00Z"
    }
  ]
}
```

#### Trading

##### Place Order

```
POST /trade/order
```

Places a new order.

**Request:**

```json
{
  "symbol": "BTC/USD",
  "side": "buy",
  "type": "limit",
  "price": 75000.0,
  "quantity": 0.1,
  "time_in_force": "GTC"
}
```

**Response:**

```json
{
  "order": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "symbol": "BTC/USD",
    "side": "buy",
    "type": "limit",
    "price": 75000.0,
    "quantity": 0.1,
    "filled_quantity": 0.0,
    "status": "open",
    "time_in_force": "GTC",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  }
}
```

##### Get Orders

```
GET /trade/orders
```

Returns orders for the authenticated user.

**Query Parameters:**

- `symbol`: Filter by trading pair symbol (e.g., `BTC/USD`)
- `status`: Filter by order status (e.g., `open`, `closed`, `canceled`)
- `limit`: The maximum number of orders to return (default: `20`)
- `offset`: The number of orders to skip (default: `0`)

**Response:**

```json
{
  "orders": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "symbol": "BTC/USD",
      "side": "buy",
      "type": "limit",
      "price": 75000.0,
      "quantity": 0.1,
      "filled_quantity": 0.0,
      "status": "open",
      "time_in_force": "GTC",
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T07:00:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "symbol": "ETH/USD",
      "side": "sell",
      "type": "limit",
      "price": 4500.0,
      "quantity": 1.0,
      "filled_quantity": 0.5,
      "status": "partially_filled",
      "time_in_force": "GTC",
      "created_at": "2025-05-27T06:00:00Z",
      "updated_at": "2025-05-27T06:30:00Z"
    }
  ],
  "pagination": {
    "total": 10,
    "limit": 20,
    "offset": 0
  }
}
```

##### Get Order

```
GET /trade/order/:id
```

Returns a specific order for the authenticated user.

**Parameters:**

- `id`: The order ID

**Response:**

```json
{
  "order": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "symbol": "BTC/USD",
    "side": "buy",
    "type": "limit",
    "price": 75000.0,
    "quantity": 0.1,
    "filled_quantity": 0.0,
    "status": "open",
    "time_in_force": "GTC",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  },
  "trades": []
}
```

##### Cancel Order

```
DELETE /trade/order/:id
```

Cancels a specific order for the authenticated user.

**Parameters:**

- `id`: The order ID

**Response:**

```json
{
  "order": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "symbol": "BTC/USD",
    "side": "buy",
    "type": "limit",
    "price": 75000.0,
    "quantity": 0.1,
    "filled_quantity": 0.0,
    "status": "canceled",
    "time_in_force": "GTC",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:01:00Z"
  }
}
```

#### Fiat

##### Initiate Deposit

```
POST /fiat/deposit
```

Initiates a fiat deposit.

**Request:**

```json
{
  "amount": 1000.0,
  "currency": "USD",
  "provider": "stripe"
}
```

**Response:**

```json
{
  "deposit": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "currency": "USD",
    "amount": 1000.0,
    "status": "pending",
    "tx_hash": "payment_id",
    "network": "stripe",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  },
  "payment_id": "payment_id",
  "payment_url": "https://example.com/pay/payment_id"
}
```

##### Get Deposits

```
GET /fiat/deposits
```

Returns fiat deposits for the authenticated user.

**Query Parameters:**

- `limit`: The maximum number of deposits to return (default: `20`)
- `offset`: The number of deposits to skip (default: `0`)

**Response:**

```json
{
  "deposits": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "USD",
      "amount": 1000.0,
      "status": "completed",
      "tx_hash": "payment_id",
      "network": "stripe",
      "transaction_id": "00000000-0000-0000-0000-000000000001",
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T07:10:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "EUR",
      "amount": 500.0,
      "status": "pending",
      "tx_hash": "payment_id_2",
      "network": "stripe",
      "created_at": "2025-05-27T06:00:00Z",
      "updated_at": "2025-05-27T06:00:00Z"
    }
  ],
  "pagination": {
    "total": 5,
    "limit": 20,
    "offset": 0
  }
}
```

##### Get Deposit

```
GET /fiat/deposit/:id
```

Returns a specific fiat deposit for the authenticated user.

**Parameters:**

- `id`: The deposit ID

**Response:**

```json
{
  "deposit": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "currency": "USD",
    "amount": 1000.0,
    "status": "completed",
    "tx_hash": "payment_id",
    "network": "stripe",
    "transaction_id": "00000000-0000-0000-0000-000000000001",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:10:00Z"
  }
}
```

##### Initiate Withdrawal

```
POST /fiat/withdraw
```

Initiates a fiat withdrawal.

**Request:**

```json
{
  "amount": 500.0,
  "currency": "USD",
  "bank_details": {
    "account_number": "1234567890",
    "routing_number": "123456789",
    "account_name": "John Doe",
    "bank_name": "Example Bank"
  }
}
```

**Response:**

```json
{
  "withdrawal": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "currency": "USD",
    "amount": 500.0,
    "fee": 1.0,
    "status": "pending",
    "transaction_id": "00000000-0000-0000-0000-000000000001",
    "address": "Bank account: 1234567890",
    "network": "bank_transfer",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  }
}
```

##### Get Withdrawals

```
GET /fiat/withdrawals
```

Returns fiat withdrawals for the authenticated user.

**Query Parameters:**

- `limit`: The maximum number of withdrawals to return (default: `20`)
- `offset`: The number of withdrawals to skip (default: `0`)

**Response:**

```json
{
  "withdrawals": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "USD",
      "amount": 500.0,
      "fee": 1.0,
      "status": "completed",
      "transaction_id": "00000000-0000-0000-0000-000000000001",
      "address": "Bank account: 1234567890",
      "network": "bank_transfer",
      "tx_hash": "transfer_id",
      "created_at": "2025-05-27T07:00:00Z",
      "updated_at": "2025-05-27T08:00:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "EUR",
      "amount": 300.0,
      "fee": 0.9,
      "status": "pending",
      "transaction_id": "00000000-0000-0000-0000-000000000002",
      "address": "Bank account: 0987654321",
      "network": "bank_transfer",
      "created_at": "2025-05-27T06:00:00Z",
      "updated_at": "2025-05-27T06:00:00Z"
    }
  ],
  "pagination": {
    "total": 5,
    "limit": 20,
    "offset": 0
  }
}
```

##### Get Withdrawal

```
GET /fiat/withdrawal/:id
```

Returns a specific fiat withdrawal for the authenticated user.

**Parameters:**

- `id`: The withdrawal ID

**Response:**

```json
{
  "withdrawal": {
    "id": "00000000-0000-0000-0000-000000000001",
    "user_id": "00000000-0000-0000-0000-000000000001",
    "currency": "USD",
    "amount": 500.0,
    "fee": 1.0,
    "status": "completed",
    "transaction_id": "00000000-0000-0000-0000-000000000001",
    "address": "Bank account: 1234567890",
    "network": "bank_transfer",
    "tx_hash": "transfer_id",
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T08:00:00Z"
  }
}
```

### Admin Endpoints

#### Users

##### Get Users

```
GET /admin/users
```

Returns all users.

**Response:**

```json
{
  "users": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "email": "user1@example.com",
      "username": "user1",
      "first_name": "User",
      "last_name": "One",
      "kyc_status": "pending",
      "two_fa_enabled": false
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "email": "user2@example.com",
      "username": "user2",
      "first_name": "User",
      "last_name": "Two",
      "kyc_status": "approved",
      "two_fa_enabled": true
    }
  ]
}
```

##### Get User

```
GET /admin/user/:id
```

Returns a specific user.

**Parameters:**

- `id`: The user ID

**Response:**

```json
{
  "user": {
    "id": "00000000-0000-0000-0000-000000000001",
    "email": "user1@example.com",
    "username": "user1",
    "first_name": "User",
    "last_name": "One",
    "kyc_status": "pending",
    "two_fa_enabled": false,
    "created_at": "2025-05-27T07:00:00Z",
    "updated_at": "2025-05-27T07:00:00Z"
  }
}
```

##### Update User KYC

```
PUT /admin/user/:id/kyc
```

Updates a user's KYC status.

**Parameters:**

- `id`: The user ID

**Request:**

```json
{
  "status": "approved"
}
```

**Response:**

```json
{
  "message": "KYC status updated successfully"
}
```

#### Trading Pairs

##### Get Trading Pairs

```
GET /admin/pairs
```

Returns all trading pairs.

**Response:**

```json
{
  "pairs": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "symbol": "BTC/USD",
      "base_asset": "BTC",
      "quote_asset": "USD",
      "min_quantity": 0.0001,
      "max_quantity": 1000.0,
      "price_precision": 2,
      "quantity_precision": 6,
      "status": "active"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "symbol": "ETH/USD",
      "base_asset": "ETH",
      "quote_asset": "USD",
      "min_quantity": 0.001,
      "max_quantity": 1000.0,
      "price_precision": 2,
      "quantity_precision": 6,
      "status": "active"
    }
  ]
}
```

##### Create Trading Pair

```
POST /admin/pair
```

Creates a new trading pair.

**Request:**

```json
{
  "symbol": "LTC/USD",
  "base_asset": "LTC",
  "quote_asset": "USD",
  "min_quantity": 0.01,
  "max_quantity": 1000.0,
  "price_precision": 2,
  "quantity_precision": 6,
  "status": "active"
}
```

**Response:**

```json
{
  "message": "Trading pair created successfully",
  "pair": {
    "id": "00000000-0000-0000-0000-000000000003",
    "symbol": "LTC/USD",
    "base_asset": "LTC",
    "quote_asset": "USD",
    "min_quantity": 0.01,
    "max_quantity": 1000.0,
    "price_precision": 2,
    "quantity_precision": 6,
    "status": "active"
  }
}
```

##### Update Trading Pair

```
PUT /admin/pair/:symbol
```

Updates a trading pair.

**Parameters:**

- `symbol`: The trading pair symbol (e.g., `BTC/USD`)

**Request:**

```json
{
  "min_quantity": 0.001,
  "max_quantity": 2000.0,
  "price_precision": 2,
  "quantity_precision": 6,
  "status": "active"
}
```

**Response:**

```json
{
  "message": "Trading pair updated successfully"
}
```

#### Withdrawals

##### Get Withdrawals

```
GET /admin/withdrawals
```

Returns all withdrawals.

**Query Parameters:**

- `status`: Filter by withdrawal status (e.g., `pending`, `approved`, `processing`, `completed`, `failed`)

**Response:**

```json
{
  "withdrawals": [
    {
      "id": "00000000-0000-0000-0000-000000000001",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "currency": "USD",
      "amount": 100.0,
      "fee": 1.0,
      "status": "pending",
      "transaction_id": "00000000-0000-0000-0000-000000000001",
      "address": "Bank account: 1234567890",
      "network": "bank_transfer",
      "created_at": "2025-05-27T07:00:00Z"
    },
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "user_id": "00000000-0000-0000-0000-000000000002",
      "currency": "EUR",
      "amount": 200.0,
      "fee": 2.0,
      "status": "pending",
      "transaction_id": "00000000-0000-0000-0000-000000000002",
      "address": "Bank account: 0987654321",
      "network": "bank_transfer",
      "created_at": "2025-05-27T06:00:00Z"
    }
  ]
}
```

##### Approve Withdrawal

```
PUT /admin/withdrawal/:id/approve
```

Approves a withdrawal.

**Parameters:**

- `id`: The withdrawal ID

**Response:**

```json
{
  "message": "Withdrawal approved successfully"
}
```

##### Reject Withdrawal

```
PUT /admin/withdrawal/:id/reject
```

Rejects a withdrawal.

**Parameters:**

- `id`: The withdrawal ID

**Request:**

```json
{
  "reason": "Insufficient funds"
}
```

**Response:**

```json
{
  "message": "Withdrawal rejected successfully"
}
```

##### Complete Withdrawal

```
PUT /admin/withdrawal/:id/complete
```

Marks a withdrawal as completed.

**Parameters:**

- `id`: The withdrawal ID

**Request:**

```json
{
  "tx_hash": "transfer_id"
}
```

**Response:**

```json
{
  "message": "Withdrawal completed successfully"
}
```

## Data Models

### User

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "email": "user@example.com",
  "username": "user123",
  "first_name": "John",
  "last_name": "Doe",
  "kyc_status": "approved",
  "two_fa_enabled": true,
  "two_fa_secret": "example_2fa_secret",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### Account

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "currency": "BTC",
  "balance": 1.5,
  "available": 1.2,
  "locked": 0.3,
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### Transaction

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "type": "credit",
  "amount": 1.0,
  "currency": "BTC",
  "status": "completed",
  "reference": "deposit",
  "description": "Bitcoin deposit",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### TransactionEntry

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "transaction_id": "00000000-0000-0000-0000-000000000001",
  "account_id": "00000000-0000-0000-0000-000000000001",
  "type": "credit",
  "amount": 1.0,
  "currency": "BTC",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### Order

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "symbol": "BTC/USD",
  "side": "buy",
  "type": "limit",
  "price": 75000.0,
  "quantity": 0.1,
  "filled_quantity": 0.0,
  "status": "open",
  "time_in_force": "GTC",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### Trade

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "order_id": "00000000-0000-0000-0000-000000000001",
  "counter_order_id": "00000000-0000-0000-0000-000000000002",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "counter_user_id": "00000000-0000-0000-0000-000000000002",
  "symbol": "BTC/USD",
  "side": "buy",
  "price": 75000.0,
  "quantity": 0.1,
  "fee": 0.0001,
  "fee_currency": "BTC",
  "created_at": "2025-05-27T07:00:00Z"
}
```

### TradingPair

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "symbol": "BTC/USD",
  "base_asset": "BTC",
  "quote_asset": "USD",
  "min_quantity": 0.0001,
  "max_quantity": 1000.0,
  "price_precision": 2,
  "quantity_precision": 6,
  "status": "active",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### MarketPrice

```json
{
  "symbol": "BTC/USD",
  "price": 75000.0,
  "change_24h": 2.5,
  "volume_24h": 1000000.0,
  "high_24h": 76000.0,
  "low_24h": 74000.0,
  "updated_at": "2025-05-27T07:00:00Z"
}
```

### Deposit

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "currency": "USD",
  "amount": 1000.0,
  "status": "completed",
  "tx_hash": "payment_id",
  "network": "stripe",
  "transaction_id": "00000000-0000-0000-0000-000000000001",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:10:00Z"
}
```

### Withdrawal

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "currency": "USD",
  "amount": 500.0,
  "fee": 1.0,
  "status": "completed",
  "transaction_id": "00000000-0000-0000-0000-000000000001",
  "address": "Bank account: 1234567890",
  "network": "bank_transfer",
  "tx_hash": "transfer_id",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T08:00:00Z"
}
```

### KYCDocument

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "type": "passport",
  "status": "approved",
  "file_path": "/path/to/document.jpg",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:30:00Z"
}
```

### APIKey

```json
{
  "id": "00000000-0000-0000-0000-000000000001",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "name": "Trading Bot",
  "key": "example_api_key",
  "secret_hash": "example_api_key_secret_hash",
  "permissions": "trade",
  "ip_whitelist": "192.168.1.1",
  "created_at": "2025-05-27T07:00:00Z",
  "updated_at": "2025-05-27T07:00:00Z"
}
```

## Conclusion

This documentation provides a comprehensive overview of the Pincex Crypto Exchange API. For any questions or issues, please contact our support team at support@pincex.com.
