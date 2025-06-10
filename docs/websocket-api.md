# WebSocket API Documentation

The Finalex platform provides real-time WebSocket APIs for live market data, trading updates, and account notifications.

## WebSocket Endpoints

### Base URL
- **Production**: `wss://ws.finalex.io`
- **Staging**: `wss://staging-ws.finalex.io`  
- **Development**: `ws://localhost:8081`

## Authentication

WebSocket connections can be authenticated using:
- **JWT Token**: Pass as query parameter `?token=<jwt_token>`
- **API Key**: Pass as query parameter `?api_key=<api_key>`

```javascript
// Example connection with JWT
const ws = new WebSocket('wss://ws.finalex.io?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...');

// Example connection with API Key
const ws = new WebSocket('wss://ws.finalex.io?api_key=pk_live_abc123def456');
```

## Message Format

All WebSocket messages follow this format:

```json
{
  "id": "unique_message_id",
  "type": "message_type",
  "channel": "channel_name",
  "data": {},
  "timestamp": 1672531200000
}
```

## Subscription Model

### Subscribe to Channels
```json
{
  "id": "sub_001",
  "method": "subscribe",
  "params": {
    "channel": "ticker",
    "symbol": "BTC/USDT"
  }
}
```

### Unsubscribe from Channels
```json
{
  "id": "unsub_001", 
  "method": "unsubscribe",
  "params": {
    "channel": "ticker",
    "symbol": "BTC/USDT"
  }
}
```

## Available Channels

### 1. Ticker Channel
Real-time price updates for trading pairs.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "ticker",
    "symbol": "BTC/USDT"
  }
}
```

**Message Format:**
```json
{
  "type": "ticker",
  "channel": "ticker",
  "data": {
    "symbol": "BTC/USDT",
    "price": "45678.90",
    "change_24h": "1234.56",
    "change_percent_24h": "2.78",
    "volume_24h": "1234567.89",
    "high_24h": "46000.00",
    "low_24h": "44500.00",
    "bid": "45678.50",
    "ask": "45679.00",
    "timestamp": 1672531200000
  }
}
```

### 2. Order Book Channel
Real-time order book updates.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "orderbook",
    "symbol": "BTC/USDT",
    "depth": 20
  }
}
```

**Snapshot Message:**
```json
{
  "type": "orderbook_snapshot",
  "channel": "orderbook",
  "data": {
    "symbol": "BTC/USDT",
    "bids": [
      ["45678.50", "1.25000000"],
      ["45678.00", "2.50000000"]
    ],
    "asks": [
      ["45679.00", "0.75000000"],
      ["45679.50", "1.80000000"]
    ],
    "timestamp": 1672531200000,
    "sequence": 123456789
  }
}
```

**Update Message:**
```json
{
  "type": "orderbook_update",
  "channel": "orderbook", 
  "data": {
    "symbol": "BTC/USDT",
    "side": "buy",
    "price": "45678.50",
    "quantity": "1.25000000",
    "action": "update",
    "timestamp": 1672531200000,
    "sequence": 123456790
  }
}
```

### 3. Trade Channel
Real-time trade executions.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "trades",
    "symbol": "BTC/USDT"
  }
}
```

**Message Format:**
```json
{
  "type": "trade",
  "channel": "trades",
  "data": {
    "symbol": "BTC/USDT",
    "trade_id": "987654321",
    "price": "45678.90",
    "quantity": "0.50000000",
    "side": "buy",
    "timestamp": 1672531200000
  }
}
```

### 4. Kline/Candlestick Channel
Real-time candlestick data.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "kline",
    "symbol": "BTC/USDT",
    "interval": "1m"
  }
}
```

**Intervals:** `1m`, `3m`, `5m`, `15m`, `30m`, `1h`, `2h`, `4h`, `6h`, `8h`, `12h`, `1d`, `3d`, `1w`, `1M`

**Message Format:**
```json
{
  "type": "kline",
  "channel": "kline",
  "data": {
    "symbol": "BTC/USDT",
    "interval": "1m", 
    "open_time": 1672531200000,
    "close_time": 1672531259999,
    "open": "45600.00",
    "high": "45700.00",
    "low": "45550.00",
    "close": "45678.90",
    "volume": "123.45678900",
    "is_closed": false
  }
}
```

## Private Channels

These channels require authentication and provide user-specific data.

### 5. Account Channel
Account balance and position updates.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "account"
  }
}
```

**Message Format:**
```json
{
  "type": "account_update",
  "channel": "account",
  "data": {
    "balances": [
      {
        "currency": "BTC",
        "available": "1.25000000",
        "locked": "0.50000000",
        "total": "1.75000000"
      },
      {
        "currency": "USDT", 
        "available": "10000.00000000",
        "locked": "5000.00000000",
        "total": "15000.00000000"
      }
    ],
    "timestamp": 1672531200000
  }
}
```

### 6. Orders Channel
Real-time order status updates.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "orders"
  }
}
```

**Message Format:**
```json
{
  "type": "order_update",
  "channel": "orders",
  "data": {
    "order_id": "123e4567-e89b-12d3-a456-426614174000",
    "symbol": "BTC/USDT",
    "side": "buy",
    "type": "limit",
    "status": "filled",
    "quantity": "1.00000000",
    "price": "45678.90",
    "filled_quantity": "1.00000000",
    "remaining_quantity": "0.00000000",
    "average_price": "45678.90",
    "fees": "0.00100000",
    "fee_currency": "BTC",
    "created_at": 1672531100000,
    "updated_at": 1672531200000
  }
}
```

### 7. Trades Channel (User)
User's trade executions.

**Subscribe:**
```json
{
  "method": "subscribe",
  "params": {
    "channel": "user_trades"
  }
}
```

**Message Format:**
```json
{
  "type": "user_trade",
  "channel": "user_trades",
  "data": {
    "trade_id": "trade_123456789",
    "order_id": "123e4567-e89b-12d3-a456-426614174000",
    "symbol": "BTC/USDT",
    "side": "buy",
    "quantity": "0.50000000",
    "price": "45678.90",
    "fee": "0.00050000",
    "fee_currency": "BTC",
    "role": "taker",
    "timestamp": 1672531200000
  }
}
```

## Error Handling

**Error Message Format:**
```json
{
  "type": "error",
  "data": {
    "code": "INVALID_CHANNEL",
    "message": "Invalid channel name provided",
    "details": "Channel 'invalid_channel' does not exist"
  },
  "timestamp": 1672531200000
}
```

**Common Error Codes:**
- `AUTHENTICATION_REQUIRED` - Authentication needed for private channels
- `INVALID_TOKEN` - Invalid or expired JWT token
- `INVALID_CHANNEL` - Channel does not exist
- `INVALID_SYMBOL` - Trading pair not found
- `RATE_LIMIT_EXCEEDED` - Too many subscription requests
- `SUBSCRIPTION_LIMIT` - Maximum subscriptions reached

## Rate Limits

- **Public Channels**: 100 subscriptions per connection
- **Private Channels**: 50 subscriptions per connection  
- **Connection Limit**: 10 connections per IP
- **Message Rate**: 1000 messages per minute per connection

## Connection Management

### Heartbeat/Ping-Pong
The server sends ping frames every 30 seconds. Clients should respond with pong frames.

### Automatic Reconnection
Implement exponential backoff for reconnection attempts:
- Initial delay: 1 second
- Maximum delay: 30 seconds
- Backoff multiplier: 2

### Connection Status
```json
{
  "type": "connection_status",
  "data": {
    "status": "connected",
    "server_time": 1672531200000,
    "rate_limits": {
      "subscriptions_remaining": 95,
      "connections_remaining": 8
    }
  }
}
```

## Code Examples

### JavaScript WebSocket Client
```javascript
class FinalexWebSocket {
  constructor(token) {
    this.token = token;
    this.ws = null;
    this.subscriptions = new Map();
  }

  connect() {
    this.ws = new WebSocket(`wss://ws.finalex.io?token=${this.token}`);
    
    this.ws.onopen = () => {
      console.log('Connected to Finalex WebSocket');
    };

    this.ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      this.handleMessage(message);
    };

    this.ws.onclose = () => {
      console.log('WebSocket connection closed');
      this.reconnect();
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  subscribe(channel, symbol, callback) {
    const id = `sub_${Date.now()}`;
    const message = {
      id,
      method: 'subscribe',
      params: { channel, symbol }
    };
    
    this.subscriptions.set(`${channel}_${symbol}`, callback);
    this.ws.send(JSON.stringify(message));
  }

  handleMessage(message) {
    const key = `${message.channel}_${message.data?.symbol}`;
    const callback = this.subscriptions.get(key);
    if (callback) {
      callback(message.data);
    }
  }

  reconnect() {
    setTimeout(() => {
      this.connect();
    }, 1000);
  }
}

// Usage
const ws = new FinalexWebSocket('your_jwt_token');
ws.connect();

// Subscribe to ticker updates
ws.subscribe('ticker', 'BTC/USDT', (data) => {
  console.log('BTC/USDT Price:', data.price);
});

// Subscribe to order book
ws.subscribe('orderbook', 'BTC/USDT', (data) => {
  console.log('Order Book Update:', data);
});
```

### Python WebSocket Client
```python
import websocket
import json
import threading

class FinalexWebSocket:
    def __init__(self, token):
        self.token = token
        self.ws = None
        self.subscriptions = {}

    def connect(self):
        url = f"wss://ws.finalex.io?token={self.token}"
        self.ws = websocket.WebSocketApp(
            url,
            on_open=self.on_open,
            on_message=self.on_message,
            on_error=self.on_error,
            on_close=self.on_close
        )
        
        # Run in separate thread
        wst = threading.Thread(target=self.ws.run_forever)
        wst.daemon = True
        wst.start()

    def on_open(self, ws):
        print("Connected to Finalex WebSocket")

    def on_message(self, ws, message):
        data = json.loads(message)
        self.handle_message(data)

    def on_error(self, ws, error):
        print(f"WebSocket error: {error}")

    def on_close(self, ws, close_status_code, close_msg):
        print("WebSocket connection closed")

    def subscribe(self, channel, symbol, callback):
        message = {
            "method": "subscribe", 
            "params": {
                "channel": channel,
                "symbol": symbol
            }
        }
        
        key = f"{channel}_{symbol}"
        self.subscriptions[key] = callback
        self.ws.send(json.dumps(message))

    def handle_message(self, message):
        if message.get("type") == "ticker":
            key = f"ticker_{message['data']['symbol']}"
            callback = self.subscriptions.get(key)
            if callback:
                callback(message["data"])

# Usage
def on_ticker_update(data):
    print(f"Price Update: {data['symbol']} = {data['price']}")

ws = FinalexWebSocket("your_jwt_token")
ws.connect()
ws.subscribe("ticker", "BTC/USDT", on_ticker_update)
```

## WebSocket Testing

You can test WebSocket connections using tools like:
- **wscat**: `wscat -c "wss://ws.finalex.io?token=your_token"`
- **Postman**: WebSocket request feature
- **Browser DevTools**: WebSocket inspection
