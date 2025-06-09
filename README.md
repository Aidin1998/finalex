# Finalex - Cryptocurrency Exchange Platform

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)](.)
[![Go Version](https://img.shields.io/badge/go-1.21+-blue)](https://golang.org)
[![RFC 7807](https://img.shields.io/badge/RFC%207807-compliant-green)](https://tools.ietf.org/html/rfc7807)
[![API Version](https://img.shields.io/badge/API-v1.0-blue)](./api/README.md)

Finalex is a comprehensive, high-performance cryptocurrency exchange platform built with Go, featuring advanced trading engines, comprehensive compliance systems, and enterprise-grade security.

## 🚀 Recent Updates (June 2025)

**Major Legacy Code Cleanup Completed** - We've successfully consolidated and modernized our codebase:
- ✅ **Unified Error Handling**: RFC 7807 compliant error responses across all APIs
- ✅ **Consolidated Validation**: Streamlined validation middleware system
- ✅ **Structured API Layer**: Organized handlers, middleware, and responses
- ✅ **40% Code Reduction**: Eliminated duplications while improving functionality

📖 **[View Full Cleanup Report](LEGACY_CLEANUP_REPORT.md)** | **[See Changelog](CHANGELOG.md)**

## 🏗️ Architecture Overview

### Core Components

```
Finalex/
├── 🌐 api/                    # Unified API layer (NEW)
│   ├── handlers/              # Request handlers
│   ├── middleware/            # API middleware  
│   ├── responses/             # Standardized responses
│   └── routes/                # Route definitions
├── 🖥️ cmd/                     # Application entry points
│   ├── pincex/                # Main exchange application
│   └── run_trading_tests/     # Trading system tests
├── 🛠️ common/                  # Shared utilities
│   ├── apiutil/               # API utilities
│   ├── auth/                  # Authentication
│   ├── cfg/                   # Configuration
│   └── dbutil/                # Database utilities
├── 🏭 internal/               # Private application code
│   ├── accounts/              # Account management
│   ├── compliance/            # Regulatory compliance
│   ├── trading/               # Trading engine
│   ├── userauth/              # User authentication
│   └── wallet/                # Wallet management
└── 📦 pkg/                    # Public packages
    ├── errors/                # Unified error handling
    ├── logger/                # Logging framework
    ├── marketdata/            # Market data client
    ├── metrics/               # Metrics collection
    ├── models/                # Data models
    └── validation/            # Input validation
```

## 🎯 Key Features

### 💱 Trading Engine
- **High-frequency trading** support with microsecond latency
- **Multiple order types**: Market, Limit, Stop-Loss, Take-Profit
- **Advanced matching engine** with price-time priority
- **Real-time market data** streaming via WebSocket
- **Cross-pair arbitrage** detection and execution

### 🛡️ Security & Compliance
- **Multi-factor authentication** (2FA/MFA) required
- **KYC/AML compliance** with automated screening
- **Rate limiting** and DDoS protection
- **End-to-end encryption** for sensitive data
- **Comprehensive audit trails** with trace IDs

### 📊 Market Data & Analytics
- **Real-time price feeds** from multiple sources
- **Advanced charting** with technical indicators
- **Historical data** storage and analysis
- **Market manipulation** detection systems
- **Risk management** with position limits

### 🏦 Wallet & Settlement
- **Multi-currency support** (crypto and fiat)
- **Hot/cold wallet** management
- **Automated settlement** systems
- **Liquidity management** across pairs
- **Transaction batching** for efficiency

## 🚀 Quick Start

### Prerequisites
- **Go 1.21+** installed
- **PostgreSQL 14+** for data storage
- **Redis 7+** for caching and sessions
- **Docker** (optional, for containerized deployment)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/Aidin1998/finalex.git
   cd finalex
   ```

2. **Install dependencies**
   ```bash
   go mod download
   go mod tidy
   ```

3. **Build the application**
   ```bash
   go build ./...
   ```

4. **Run the main application**
   ```bash
   go run cmd/pincex/main.go
   ```

5. **Run the admin API** (separate terminal)
   ```bash
   go run -tags admin cmd/pincex/admin_api_main.go
   ```

### Configuration

The application uses YAML configuration files located in the `configs/` directory:

- **`transaction-manager.yaml`** - Core transaction settings
- **`risk-management.yaml`** - Risk parameters and limits
- **`fee-config.yaml`** - Trading fee structure
- **`aml-us.yaml`** / **`aml-eu.yaml`** - Regional compliance rules

## 📡 API Documentation

### Unified Response Format

All API responses follow our standardized format:

```json
{
  "success": true,
  "data": { ... },
  "message": "Operation completed successfully",
  "timestamp": "2025-06-09T10:30:00Z",
  "trace_id": "req_abc123def456"
}
```

### Error Responses (RFC 7807 Compliant)

```json
{
  "type": "https://api.finalex.io/problems/insufficient-funds",
  "title": "Insufficient Funds",
  "status": 422,
  "detail": "Account balance insufficient for order placement",
  "instance": "/api/v1/trading/orders",
  "trace_id": "req_abc123def456"
}
```

### Key Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/trading/orders` | POST | Place a new order |
| `/api/v1/trading/orders` | GET | List user orders |
| `/api/v1/trading/orders/{id}` | GET | Get order details |
| `/api/v1/trading/orders/{id}` | DELETE | Cancel an order |
| `/api/v1/marketdata/symbols` | GET | List trading pairs |
| `/api/v1/marketdata/ticker/{symbol}` | GET | Get ticker data |
| `/api/v1/wallet/balances` | GET | Get account balances |

📖 **[Complete API Documentation](api/README.md)**

## 🏗️ Development

### Code Organization

#### Unified Error Handling
```go
import "github.com/Aidin1998/finalex/pkg/errors"

// Create business-specific errors
err := errors.NewInsufficientFundsError("Balance too low", "/api/v1/orders")

// Use in API responses
responses.Error(c, err)
```

#### Validation Middleware
```go
import "github.com/Aidin1998/finalex/pkg/validation"

// Apply unified validation with different profiles
router.Use(validation.UnifiedMiddleware(validation.ConfigMedium))
```

#### Standardized API Responses
```go
import "github.com/Aidin1998/finalex/api/responses"

// Success responses
responses.Success(c, orderData, "Order placed successfully")

// Error responses with problem details
responses.Error(c, errors.NewValidationError("Invalid input", path))
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run trading system tests
go run cmd/run_trading_tests/main.go
```

### Building

```bash
# Build all components
go build ./...

# Build specific components
go build -o finalex cmd/pincex/main.go
go build -o admin-api -tags admin cmd/pincex/admin_api_main.go

# Build for production
go build -ldflags="-w -s" -o finalex cmd/pincex/main.go
```

## 🐳 Deployment

### Docker

```bash
# Build production image
docker build -f Dockerfile.production -t finalex:latest .

# Run with docker-compose
docker-compose up -d
```

### Kubernetes

```bash
# Deploy to Kubernetes
kubectl apply -f infra/k8s/deployments/
kubectl apply -f infra/k8s/monitoring/
```

### Terraform (Infrastructure)

```bash
# Deploy infrastructure
cd infra/terraform
terraform init
terraform plan
terraform apply
```

## 🔧 Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_HOST` | PostgreSQL host | `localhost` |
| `DB_PORT` | PostgreSQL port | `5432` |
| `REDIS_HOST` | Redis host | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `API_PORT` | API server port | `8080` |
| `ADMIN_PORT` | Admin API port | `8081` |
| `LOG_LEVEL` | Logging level | `info` |

### Feature Flags

- **`ENABLE_MARKET_MAKING`** - Enable automated market making
- **`ENABLE_CROSS_PAIR_ARBITRAGE`** - Enable arbitrage detection
- **`STRICT_VALIDATION`** - Use strict validation profile
- **`ENABLE_METRICS`** - Enable Prometheus metrics

## 📊 Monitoring & Observability

### Metrics (Prometheus)
- **Trading metrics**: Orders, fills, volume
- **System metrics**: Latency, errors, throughput
- **Business metrics**: Revenue, fees, user activity

### Logging (Structured)
- **Trace ID integration** for request tracking
- **Structured JSON** logging format
- **Log levels**: Debug, Info, Warn, Error
- **Audit trail** for compliance

### Health Checks
- **Liveness probe**: `/health/live`
- **Readiness probe**: `/health/ready`
- **Dependency checks**: Database, Redis, external APIs

## 🛠️ Contributing

### Development Workflow

1. **Fork the repository**
2. **Create feature branch**: `git checkout -b feature/amazing-feature`
3. **Follow code standards**: Use unified error handling and validation
4. **Add tests**: Ensure > 80% coverage
5. **Update documentation**: Keep README and API docs current
6. **Submit pull request**: Include detailed description

### Code Standards

- **Use unified error handling**: `pkg/errors` package only
- **Follow RFC 7807**: For all error responses
- **Use structured logging**: With trace IDs
- **Add comprehensive tests**: Unit and integration
- **Document public APIs**: With examples

### Testing Guidelines

- **Unit tests**: `*_test.go` files alongside source
- **Integration tests**: `test/` directory
- **Benchmark tests**: For performance-critical code
- **Mock external dependencies**: Use interfaces

## 📚 Documentation

- **[API Documentation](api/README.md)** - Complete API reference
- **[Validation Guide](pkg/validation/CONSOLIDATION_GUIDE.md)** - Validation system
- **[Migration Guide](docs/migration-guide.md)** - Update existing code
- **[Operational Guide](docs/operational-guide.md)** - Production operations
- **[Legacy Cleanup Report](LEGACY_CLEANUP_REPORT.md)** - Recent improvements

## 🔐 Security

### Reporting Vulnerabilities

Please report security vulnerabilities to: **security@finalex.io**

### Security Features

- **Rate limiting** on all endpoints
- **Input validation** and sanitization
- **SQL injection** prevention
- **XSS protection** for web interfaces
- **CORS configuration** for API access
- **TLS encryption** for all communications

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Support

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/Aidin1998/finalex/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Aidin1998/finalex/discussions)
- **Email**: support@finalex.io

## 🎯 Roadmap

### Q3 2025
- [ ] **GraphQL API** implementation
- [ ] **Advanced order types** (Iceberg, TWAP)
- [ ] **Mobile API** optimization
- [ ] **Institutional features** (block trading)

### Q4 2025
- [ ] **DeFi integration** protocols
- [ ] **Layer 2** scaling solutions
- [ ] **Advanced analytics** dashboard
- [ ] **Multi-region** deployment

---

**Built with ❤️ by the Finalex Team**

*Last Updated: June 9, 2025*
