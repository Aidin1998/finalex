# Backend-Frontend Integration Guide

This guide demonstrates how to connect the Finalex backend APIs to your frontend application with complete automation and no undocumented endpoints.

## Overview

The Finalex platform provides comprehensive API coverage through:
- **REST API**: Full CRUD operations for all platform features
- **WebSocket API**: Real-time data streaming and updates
- **OpenAPI Documentation**: Complete API specifications
- **Auto-generated Client Libraries**: Type-safe API clients

## Integration Architecture

```
Frontend Application
├── API Client Layer (Auto-generated)
├── State Management (Redux/Zustand)
├── Real-time Layer (WebSocket)
├── Authentication Handler
└── Error Handling Middleware
```

## 1. Setting Up API Client

### Option A: Auto-generated TypeScript Client

Generate a TypeScript client from the OpenAPI specification:

```bash
# Install OpenAPI Generator
npm install -g @openapitools/openapi-generator-cli

# Generate TypeScript client
openapi-generator-cli generate \
  -i http://localhost:8080/docs/doc.json \
  -g typescript-axios \
  -o ./src/api/generated \
  --additional-properties=supportsES6=true,npmName=finalex-api-client
```

### Option B: Manual API Client Setup

```typescript
// src/api/client.ts
import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';

export interface ApiConfig {
  baseURL: string;
  timeout?: number;
  headers?: Record<string, string>;
}

export class FinalexApiClient {
  private client: AxiosInstance;
  private token: string | null = null;

  constructor(config: ApiConfig) {
    this.client = axios.create({
      baseURL: config.baseURL,
      timeout: config.timeout || 10000,
      headers: {
        'Content-Type': 'application/json',
        ...config.headers,
      },
    });

    this.setupInterceptors();
  }

  private setupInterceptors() {
    // Request interceptor for authentication
    this.client.interceptors.request.use(
      (config) => {
        if (this.token) {
          config.headers.Authorization = `Bearer ${this.token}`;
        }
        return config;
      },
      (error) => Promise.reject(error)
    );

    // Response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      (error) => {
        if (error.response?.status === 401) {
          this.handleUnauthorized();
        }
        return Promise.reject(error);
      }
    );
  }

  setToken(token: string) {
    this.token = token;
  }

  clearToken() {
    this.token = null;
  }

  private handleUnauthorized() {
    this.clearToken();
    // Redirect to login or refresh token
    window.location.href = '/login';
  }

  // Generic request method
  async request<T>(config: AxiosRequestConfig): Promise<T> {
    const response = await this.client.request<T>(config);
    return response.data;
  }
}

// Create singleton instance
export const apiClient = new FinalexApiClient({
  baseURL: process.env.REACT_APP_API_URL || 'http://localhost:8080',
});
```

## 2. Authentication Integration

### Login Flow
```typescript
// src/api/auth.ts
export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  user: {
    id: string;
    email: string;
    username: string;
  };
}

export class AuthApi {
  constructor(private client: FinalexApiClient) {}

  async login(credentials: LoginRequest): Promise<LoginResponse> {
    return this.client.request<LoginResponse>({
      method: 'POST',
      url: '/api/v1/auth/login',
      data: credentials,
    });
  }

  async register(userData: RegisterRequest): Promise<RegisterResponse> {
    return this.client.request<RegisterResponse>({
      method: 'POST',
      url: '/api/v1/auth/register',
      data: userData,
    });
  }

  async refreshToken(refreshToken: string): Promise<LoginResponse> {
    return this.client.request<LoginResponse>({
      method: 'POST',
      url: '/api/v1/auth/refresh',
      data: { refresh_token: refreshToken },
    });
  }

  async logout(): Promise<void> {
    return this.client.request<void>({
      method: 'POST',
      url: '/api/v1/auth/logout',
    });
  }
}

export const authApi = new AuthApi(apiClient);
```

### React Authentication Hook
```typescript
// src/hooks/useAuth.ts
import { useState, useEffect, useCallback } from 'react';
import { authApi } from '../api/auth';
import { apiClient } from '../api/client';

export interface User {
  id: string;
  email: string;
  username: string;
}

export const useAuth = () => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Load user from localStorage on mount
  useEffect(() => {
    const token = localStorage.getItem('access_token');
    const userData = localStorage.getItem('user');
    
    if (token && userData) {
      apiClient.setToken(token);
      setUser(JSON.parse(userData));
    }
    setLoading(false);
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    try {
      setLoading(true);
      setError(null);
      
      const response = await authApi.login({ email, password });
      
      // Store tokens and user data
      localStorage.setItem('access_token', response.access_token);
      localStorage.setItem('refresh_token', response.refresh_token);
      localStorage.setItem('user', JSON.stringify(response.user));
      
      // Set token in API client
      apiClient.setToken(response.access_token);
      setUser(response.user);
      
      return response;
    } catch (err: any) {
      setError(err.response?.data?.message || 'Login failed');
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } catch (err) {
      console.error('Logout error:', err);
    } finally {
      // Clear local storage and state
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
      localStorage.removeItem('user');
      apiClient.clearToken();
      setUser(null);
    }
  }, []);

  const refreshToken = useCallback(async () => {
    const refreshToken = localStorage.getItem('refresh_token');
    if (!refreshToken) {
      logout();
      return;
    }

    try {
      const response = await authApi.refreshToken(refreshToken);
      localStorage.setItem('access_token', response.access_token);
      apiClient.setToken(response.access_token);
      return response;
    } catch (err) {
      logout();
      throw err;
    }
  }, [logout]);

  return {
    user,
    loading,
    error,
    login,
    logout,
    refreshToken,
    isAuthenticated: !!user,
  };
};
```

## 3. Trading Integration

### Trading API Client
```typescript
// src/api/trading.ts
export interface TradingPair {
  symbol: string;
  base_currency: string;
  quote_currency: string;
  min_order_size: string;
  max_order_size: string;
  price_precision: number;
  quantity_precision: number;
  status: 'active' | 'inactive';
}

export interface OrderRequest {
  symbol: string;
  side: 'buy' | 'sell';
  type: 'market' | 'limit' | 'stop_limit';
  quantity: string;
  price?: string;
  stop_price?: string;
  time_in_force?: 'GTC' | 'IOC' | 'FOK';
}

export interface Order {
  id: string;
  symbol: string;
  side: 'buy' | 'sell';
  type: string;
  status: string;
  quantity: string;
  price: string;
  filled_quantity: string;
  remaining_quantity: string;
  created_at: string;
  updated_at: string;
}

export class TradingApi {
  constructor(private client: FinalexApiClient) {}

  async getTradingPairs(): Promise<TradingPair[]> {
    const response = await this.client.request<{ data: TradingPair[] }>({
      method: 'GET',
      url: '/api/v1/trading/pairs',
    });
    return response.data;
  }

  async placeOrder(order: OrderRequest): Promise<Order> {
    return this.client.request<Order>({
      method: 'POST',
      url: '/api/v1/trading/orders',
      data: order,
    });
  }

  async getOrders(symbol?: string): Promise<Order[]> {
    const response = await this.client.request<{ data: Order[] }>({
      method: 'GET',
      url: '/api/v1/trading/orders',
      params: symbol ? { symbol } : undefined,
    });
    return response.data;
  }

  async cancelOrder(orderId: string): Promise<void> {
    return this.client.request<void>({
      method: 'DELETE',
      url: `/api/v1/trading/orders/${orderId}`,
    });
  }

  async getOrderBook(symbol: string): Promise<OrderBook> {
    return this.client.request<OrderBook>({
      method: 'GET',
      url: '/api/v1/market/orderbook',
      params: { symbol },
    });
  }
}

export const tradingApi = new TradingApi(apiClient);
```

### React Trading Hook
```typescript
// src/hooks/useTrading.ts
import { useState, useCallback } from 'react';
import { tradingApi, OrderRequest, Order } from '../api/trading';

export const useTrading = () => {
  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const placeOrder = useCallback(async (orderData: OrderRequest) => {
    try {
      setLoading(true);
      setError(null);
      
      const order = await tradingApi.placeOrder(orderData);
      setOrders(prev => [order, ...prev]);
      
      return order;
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to place order');
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const cancelOrder = useCallback(async (orderId: string) => {
    try {
      setLoading(true);
      await tradingApi.cancelOrder(orderId);
      
      setOrders(prev => 
        prev.map(order => 
          order.id === orderId 
            ? { ...order, status: 'cancelled' }
            : order
        )
      );
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to cancel order');
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const loadOrders = useCallback(async (symbol?: string) => {
    try {
      setLoading(true);
      const ordersData = await tradingApi.getOrders(symbol);
      setOrders(ordersData);
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to load orders');
    } finally {
      setLoading(false);
    }
  }, []);

  return {
    orders,
    loading,
    error,
    placeOrder,
    cancelOrder,
    loadOrders,
  };
};
```

## 4. WebSocket Integration

### WebSocket Client
```typescript
// src/websocket/client.ts
export interface WebSocketMessage {
  type: string;
  channel: string;
  data: any;
  timestamp: number;
}

export interface SubscriptionCallback {
  (data: any): void;
}

export class FinalexWebSocket {
  private ws: WebSocket | null = null;
  private subscriptions = new Map<string, SubscriptionCallback>();
  private token: string | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;

  constructor(private baseUrl: string) {}

  setToken(token: string) {
    this.token = token;
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      const url = this.token 
        ? `${this.baseUrl}?token=${this.token}`
        : this.baseUrl;

      this.ws = new WebSocket(url);

      this.ws.onopen = () => {
        console.log('WebSocket connected');
        this.reconnectAttempts = 0;
        resolve();
      };

      this.ws.onmessage = (event) => {
        try {
          const message: WebSocketMessage = JSON.parse(event.data);
          this.handleMessage(message);
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };

      this.ws.onclose = () => {
        console.log('WebSocket disconnected');
        this.attemptReconnect();
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        reject(error);
      };
    });
  }

  private handleMessage(message: WebSocketMessage) {
    const key = `${message.channel}_${message.data?.symbol || 'global'}`;
    const callback = this.subscriptions.get(key);
    if (callback) {
      callback(message.data);
    }
  }

  subscribe(channel: string, symbol: string | null, callback: SubscriptionCallback) {
    const key = `${channel}_${symbol || 'global'}`;
    this.subscriptions.set(key, callback);

    if (this.ws?.readyState === WebSocket.OPEN) {
      const message = {
        method: 'subscribe',
        params: {
          channel,
          ...(symbol && { symbol }),
        },
      };
      this.ws.send(JSON.stringify(message));
    }
  }

  unsubscribe(channel: string, symbol: string | null) {
    const key = `${channel}_${symbol || 'global'}`;
    this.subscriptions.delete(key);

    if (this.ws?.readyState === WebSocket.OPEN) {
      const message = {
        method: 'unsubscribe',
        params: {
          channel,
          ...(symbol && { symbol }),
        },
      };
      this.ws.send(JSON.stringify(message));
    }
  }

  private attemptReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnection attempts reached');
      return;
    }

    this.reconnectAttempts++;
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);

    setTimeout(() => {
      console.log(`Attempting to reconnect (${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
      this.connect().catch(() => {
        // Reconnection failed, attemptReconnect will be called again by onclose
      });
    }, delay);
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.subscriptions.clear();
  }
}

// Create singleton instance
export const webSocketClient = new FinalexWebSocket(
  process.env.REACT_APP_WS_URL || 'ws://localhost:8081'
);
```

### React WebSocket Hook
```typescript
// src/hooks/useWebSocket.ts
import { useEffect, useCallback, useRef } from 'react';
import { webSocketClient, SubscriptionCallback } from '../websocket/client';

export const useWebSocket = () => {
  const isConnected = useRef(false);

  useEffect(() => {
    const connect = async () => {
      try {
        await webSocketClient.connect();
        isConnected.current = true;
      } catch (error) {
        console.error('Failed to connect to WebSocket:', error);
      }
    };

    connect();

    return () => {
      webSocketClient.disconnect();
      isConnected.current = false;
    };
  }, []);

  const subscribe = useCallback((
    channel: string, 
    symbol: string | null, 
    callback: SubscriptionCallback
  ) => {
    if (isConnected.current) {
      webSocketClient.subscribe(channel, symbol, callback);
    }
  }, []);

  const unsubscribe = useCallback((channel: string, symbol: string | null) => {
    webSocketClient.unsubscribe(channel, symbol);
  }, []);

  return {
    subscribe,
    unsubscribe,
    isConnected: isConnected.current,
  };
};
```

## 5. State Management Integration

### Redux Setup with RTK Query
```typescript
// src/store/api.ts
import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react';

export const finalexApi = createApi({
  reducerPath: 'finalexApi',
  baseQuery: fetchBaseQuery({
    baseUrl: '/api/v1',
    prepareHeaders: (headers, { getState }) => {
      const token = localStorage.getItem('access_token');
      if (token) {
        headers.set('authorization', `Bearer ${token}`);
      }
      return headers;
    },
  }),
  tagTypes: ['User', 'Order', 'Balance', 'TradingPair'],
  endpoints: (builder) => ({
    // Authentication
    login: builder.mutation({
      query: (credentials) => ({
        url: 'auth/login',
        method: 'POST',
        body: credentials,
      }),
      invalidatesTags: ['User'],
    }),

    // Trading
    getTradingPairs: builder.query({
      query: () => 'trading/pairs',
      providesTags: ['TradingPair'],
    }),

    placeOrder: builder.mutation({
      query: (order) => ({
        url: 'trading/orders',
        method: 'POST',
        body: order,
      }),
      invalidatesTags: ['Order', 'Balance'],
    }),

    getOrders: builder.query({
      query: (symbol) => ({
        url: 'trading/orders',
        params: symbol ? { symbol } : undefined,
      }),
      providesTags: ['Order'],
    }),

    // Wallet
    getBalances: builder.query({
      query: () => 'wallet/balances',
      providesTags: ['Balance'],
    }),
  }),
});

export const {
  useLoginMutation,
  useGetTradingPairsQuery,
  usePlaceOrderMutation,
  useGetOrdersQuery,
  useGetBalancesQuery,
} = finalexApi;
```

## 6. React Components Examples

### Trading Component
```typescript
// src/components/Trading/OrderForm.tsx
import React, { useState } from 'react';
import { usePlaceOrderMutation, useGetTradingPairsQuery } from '../../store/api';

export const OrderForm: React.FC = () => {
  const [symbol, setSymbol] = useState('BTC/USDT');
  const [side, setSide] = useState<'buy' | 'sell'>('buy');
  const [type, setType] = useState<'market' | 'limit'>('limit');
  const [quantity, setQuantity] = useState('');
  const [price, setPrice] = useState('');

  const { data: tradingPairs } = useGetTradingPairsQuery();
  const [placeOrder, { isLoading, error }] = usePlaceOrderMutation();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    try {
      await placeOrder({
        symbol,
        side,
        type,
        quantity,
        ...(type === 'limit' && { price }),
      }).unwrap();

      // Reset form
      setQuantity('');
      setPrice('');
    } catch (err) {
      console.error('Failed to place order:', err);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="order-form">
      <div>
        <label>Trading Pair</label>
        <select value={symbol} onChange={(e) => setSymbol(e.target.value)}>
          {tradingPairs?.data?.map((pair) => (
            <option key={pair.symbol} value={pair.symbol}>
              {pair.symbol}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label>Side</label>
        <div>
          <button
            type="button"
            className={side === 'buy' ? 'active' : ''}
            onClick={() => setSide('buy')}
          >
            Buy
          </button>
          <button
            type="button"
            className={side === 'sell' ? 'active' : ''}
            onClick={() => setSide('sell')}
          >
            Sell
          </button>
        </div>
      </div>

      <div>
        <label>Order Type</label>
        <select value={type} onChange={(e) => setType(e.target.value as any)}>
          <option value="market">Market</option>
          <option value="limit">Limit</option>
        </select>
      </div>

      <div>
        <label>Quantity</label>
        <input
          type="number"
          value={quantity}
          onChange={(e) => setQuantity(e.target.value)}
          required
        />
      </div>

      {type === 'limit' && (
        <div>
          <label>Price</label>
          <input
            type="number"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            required
          />
        </div>
      )}

      <button type="submit" disabled={isLoading}>
        {isLoading ? 'Placing Order...' : `Place ${side.toUpperCase()} Order`}
      </button>

      {error && (
        <div className="error">
          {(error as any).data?.message || 'Failed to place order'}
        </div>
      )}
    </form>
  );
};
```

### Real-time Price Component
```typescript
// src/components/Market/PriceDisplay.tsx
import React, { useState, useEffect } from 'react';
import { useWebSocket } from '../../hooks/useWebSocket';

interface PriceData {
  symbol: string;
  price: string;
  change_24h: string;
  change_percent_24h: string;
}

export const PriceDisplay: React.FC<{ symbol: string }> = ({ symbol }) => {
  const [priceData, setPriceData] = useState<PriceData | null>(null);
  const { subscribe, unsubscribe } = useWebSocket();

  useEffect(() => {
    const handlePriceUpdate = (data: PriceData) => {
      setPriceData(data);
    };

    subscribe('ticker', symbol, handlePriceUpdate);

    return () => {
      unsubscribe('ticker', symbol);
    };
  }, [symbol, subscribe, unsubscribe]);

  if (!priceData) {
    return <div>Loading price...</div>;
  }

  const isPositive = parseFloat(priceData.change_24h) >= 0;

  return (
    <div className="price-display">
      <h3>{priceData.symbol}</h3>
      <div className="price">${priceData.price}</div>
      <div className={`change ${isPositive ? 'positive' : 'negative'}`}>
        {isPositive ? '+' : ''}{priceData.change_24h} ({priceData.change_percent_24h}%)
      </div>
    </div>
  );
};
```

## 7. Error Handling Strategy

### Global Error Handler
```typescript
// src/utils/errorHandler.ts
export interface ApiError {
  type: string;
  title: string;
  status: number;
  detail: string;
  instance?: string;
}

export const handleApiError = (error: any) => {
  if (error.response?.data) {
    const apiError: ApiError = error.response.data;
    
    // Handle specific error types
    switch (apiError.type) {
      case 'authentication_required':
        // Redirect to login
        window.location.href = '/login';
        break;
      
      case 'insufficient_balance':
        // Show balance error
        toast.error('Insufficient balance for this operation');
        break;
      
      case 'rate_limit_exceeded':
        // Show rate limit error
        toast.error('Rate limit exceeded. Please try again later.');
        break;
      
      default:
        // Generic error
        toast.error(apiError.detail || 'An error occurred');
    }
  } else {
    // Network or other error
    toast.error('Network error. Please check your connection.');
  }
};
```

## 8. Testing Integration

### API Testing
```typescript
// src/api/__tests__/trading.test.ts
import { rest } from 'msw';
import { setupServer } from 'msw/node';
import { tradingApi } from '../trading';

const server = setupServer(
  rest.get('/api/v1/trading/pairs', (req, res, ctx) => {
    return res(
      ctx.json({
        data: [
          {
            symbol: 'BTC/USDT',
            base_currency: 'BTC',
            quote_currency: 'USDT',
            min_order_size: '0.001',
            max_order_size: '100',
            price_precision: 2,
            quantity_precision: 6,
            status: 'active',
          },
        ],
      })
    );
  }),

  rest.post('/api/v1/trading/orders', (req, res, ctx) => {
    return res(
      ctx.json({
        id: '123e4567-e89b-12d3-a456-426614174000',
        symbol: 'BTC/USDT',
        side: 'buy',
        type: 'limit',
        status: 'open',
        quantity: '1.0',
        price: '45000.00',
        created_at: new Date().toISOString(),
      })
    );
  })
);

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe('Trading API', () => {
  test('should fetch trading pairs', async () => {
    const pairs = await tradingApi.getTradingPairs();
    expect(pairs).toHaveLength(1);
    expect(pairs[0].symbol).toBe('BTC/USDT');
  });

  test('should place order', async () => {
    const order = await tradingApi.placeOrder({
      symbol: 'BTC/USDT',
      side: 'buy',
      type: 'limit',
      quantity: '1.0',
      price: '45000.00',
    });
    
    expect(order.symbol).toBe('BTC/USDT');
    expect(order.status).toBe('open');
  });
});
```

## 9. Deployment Configuration

### Environment Configuration
```typescript
// src/config/env.ts
export const config = {
  API_URL: process.env.REACT_APP_API_URL || 'http://localhost:8080',
  WS_URL: process.env.REACT_APP_WS_URL || 'ws://localhost:8081',
  ENVIRONMENT: process.env.NODE_ENV || 'development',
};

// Environment-specific configurations
export const environments = {
  development: {
    API_URL: 'http://localhost:8080',
    WS_URL: 'ws://localhost:8081',
  },
  staging: {
    API_URL: 'https://staging-api.finalex.io',
    WS_URL: 'wss://staging-ws.finalex.io',
  },
  production: {
    API_URL: 'https://api.finalex.io',
    WS_URL: 'wss://ws.finalex.io',
  },
};
```

## 10. Monitoring and Analytics

### API Performance Monitoring
```typescript
// src/utils/monitoring.ts
export const trackApiCall = (endpoint: string, method: string, duration: number, status: number) => {
  // Send to analytics service
  analytics.track('api_call', {
    endpoint,
    method,
    duration,
    status,
    timestamp: Date.now(),
  });
};

// Axios interceptor for monitoring
apiClient.interceptors.request.use((config) => {
  config.metadata = { startTime: Date.now() };
  return config;
});

apiClient.interceptors.response.use(
  (response) => {
    const duration = Date.now() - response.config.metadata.startTime;
    trackApiCall(response.config.url!, response.config.method!, duration, response.status);
    return response;
  },
  (error) => {
    const duration = Date.now() - error.config.metadata.startTime;
    trackApiCall(error.config.url!, error.config.method!, duration, error.response?.status || 0);
    return Promise.reject(error);
  }
);
```

This comprehensive integration guide ensures that your frontend application can seamlessly connect to all Finalex backend APIs with complete automation, type safety, and real-time capabilities. All endpoints are documented and tested, providing a robust foundation for your cryptocurrency exchange platform.
