# Pincex - Production-Ready Crypto Exchange Platform

Pincex is a high-performance, production-ready cryptocurrency exchange platform built with Go. It features a unified monorepo structure with standardized components and APIs.

## Features

- High-performance matching engine (3,000-7,000 matches/second)
- Support for unlimited trading pairs (scalable to 5,000+ currencies)
- User identity management with 2FA and KYC integration
- Comprehensive account and transaction management
- Fiat payment processing with multiple provider integrations
- Real-time market data feeds
- RESTful API and WebSocket support
- PostgreSQL for data storage and Redis for caching

## Architecture

The platform follows a standard Go monorepo structure:

```text
pincex_unified/
├── cmd/                  # Command-line applications
│   └── pincex/           # Main application entry point
├── internal/             # Private application code
│   ├── bookkeeper/       # Account and transaction management
│   ├── config/           # Configuration management
│   ├── database/         # Database connections
│   ├── fiat/             # Fiat payment processing
│   ├── identities/       # User identity management
│   ├── marketfeeds/      # Market data processing
│   ├── server/           # HTTP server and API implementation
│   └── trading/          # Trading engine and order book
├── pkg/                  # Public libraries
│   ├── logger/           # Logging utilities
│   └── models/           # Shared data models
├── scripts/              # Utility scripts
├── .github/workflows/    # CI/CD configuration
├── .env.example          # Example environment variables
├── Dockerfile            # Container definition
├── go.mod                # Go module definition
└── README.md             # Project documentation
```

## Getting Started

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 14 or higher
- Redis 6 or higher

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/Aidin1998/pincex.git
   cd pincex
   ```

2. Install dependencies:
   ```bash
   go mod tidy
   ```

3. Set up environment variables:
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

4. Build the application:
   ```bash
   go build -o pincex ./cmd/pincex
   ```

5. Run the application:
   ```bash
   ./pincex
   ```

## API Documentation

The platform provides a comprehensive RESTful API for all operations:

### Authentication

- `POST /api/v1/identities/register` - Register a new user
- `POST /api/v1/identities/login` - Login a user
- `POST /api/v1/identities/logout` - Logout a user
- `POST /api/v1/identities/refresh` - Refresh authentication token
- `POST /api/v1/identities/2fa/enable` - Enable 2FA
- `POST /api/v1/identities/2fa/verify` - Verify 2FA token
- `POST /api/v1/identities/2fa/disable` - Disable 2FA

### User Management

- `GET /api/v1/identities/me` - Get current user
- `PUT /api/v1/identities/me` - Update current user
- `POST /api/v1/identities/kyc/submit` - Submit KYC documents
- `GET /api/v1/identities/kyc/status` - Get KYC status

### Account Management

- `GET /api/v1/accounts` - Get all accounts
- `GET /api/v1/accounts/:currency` - Get account by currency
- `GET /api/v1/accounts/:currency/transactions` - Get account transactions

### Trading

- `GET /api/v1/trading/pairs` - Get all trading pairs
- `GET /api/v1/trading/pairs/:symbol` - Get trading pair by symbol
- `POST /api/v1/trading/orders` - Place an order
- `GET /api/v1/trading/orders` - Get all orders
- `GET /api/v1/trading/orders/:id` - Get order by ID
- `DELETE /api/v1/trading/orders/:id` - Cancel an order
- `GET /api/v1/trading/orderbook/:symbol` - Get order book for a symbol

### Market Data

- `GET /api/v1/market/prices` - Get all market prices
- `GET /api/v1/market/prices/:symbol` - Get market price by symbol
- `GET /api/v1/market/candles/:symbol` - Get candles for a symbol

### Fiat Operations

- `POST /api/v1/fiat/deposit` - Initiate a fiat deposit
- `POST /api/v1/fiat/withdraw` - Initiate a fiat withdrawal
- `GET /api/v1/fiat/deposits` - Get all fiat deposits
- `GET /api/v1/fiat/withdrawals` - Get all fiat withdrawals

### Admin Operations

- `POST /api/v1/admin/trading/pairs` - Create a trading pair
- `PUT /api/v1/admin/trading/pairs/:symbol` - Update a trading pair
- `GET /api/v1/admin/users` - Get all users
- `GET /api/v1/admin/users/:id` - Get user by ID
- `PUT /api/v1/admin/users/:id/kyc` - Update user KYC status

## Deployment

### Docker

Build and run the Docker container:

```bash
docker build -t pincex .
docker run -p 8080:8080 pincex
```

### Kubernetes

Apply the Kubernetes configuration:

```bash
kubectl apply -f k8s/
```

## Development

### Running Tests

```bash
go test ./...
```

### Database Migrations

```bash
./scripts/migrate.sh
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
