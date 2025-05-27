# Fiat Gateway Service Specification

## Functional Requirements

### REST Endpoints

- **POST /fiat/deposit**
  - Initiate a fiat deposit via Stripe
  - Request: `{ amount, currency, user_id, payment_method }`
  - Response: `{ deposit_id, status, stripe_session_url }`

- **POST /fiat/exchange**
  - Convert fiat to USDT or vice versa
  - Request: `{ from_currency, to_currency, amount, user_id }`
  - Response: `{ exchange_id, status, rate, fee, amount_converted }`

- **GET /fiat/rates**
  - Get current fiat↔USDT rates (aggregated)
  - Response: `{ rates: [{ pair, rate, source }] }`

### Data Models

#### fiat_wallets
- `id`: UUID
- `user_id`: UUID
- `currency`: string
- `balance`: decimal
- `created_at`, `updated_at`: timestamp

#### exchange_logs
- `id`: UUID
- `user_id`: UUID
- `from_currency`, `to_currency`: string
- `amount`, `amount_converted`: decimal
- `rate`: decimal
- `fee`: decimal
- `status`: string
- `created_at`: timestamp

#### fee_configs
- `id`: UUID
- `user_group`: string
- `tier_min`, `tier_max`: decimal
- `fee_percent`: decimal
- `active`: bool
- `created_at`, `updated_at`: timestamp

### Sequence Diagrams

#### Deposit Flow
```
User → API: POST /fiat/deposit
API → Stripe: Create payment session
Stripe → API: Session URL
API → User: Return session URL
User → Stripe: Complete payment
Stripe → API: Webhook (payment succeeded)
API → Store: Update fiat_wallets
API → Audit Log: Record deposit
```

#### Exchange Flow
```
User → API: POST /fiat/exchange
API → Rates: Fetch rate
API → Store: Check fiat_wallets balance
API → Exchange: Calculate fee, perform conversion
API → Store: Update fiat_wallets, log exchange
API → Audit Log: Record exchange
API → User: Return result
```

## Non-Functional Requirements

- **Performance**: ≥1,000 requests/sec, <100 ms P95 latency
- **Security**: PCI-DSS compliance for payment data
- **Idempotency**: All POST endpoints must be idempotent (idempotency key header)
- **Audit Logging**: All deposit and exchange actions must be logged for compliance
- **Availability**: 99.9% uptime
- **Scalability**: Horizontal scaling supported
- **Monitoring**: Metrics for latency, error rate, throughput

---
This spec covers the core requirements for the fiat-gateway service, including endpoints, data models, flows, and operational guarantees.
