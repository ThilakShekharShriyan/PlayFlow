# PayFlow

A production-grade payment orchestration platform built with Go, designed for reliability, correctness, and observability.

## Overview

PayFlow is a payment processing system that handles the complete lifecycle of payment operations—from authorization to capture, refunds, and reconciliation. Built with a **ledger-first** architecture, every financial operation is immutable, auditable, and deterministic.

## Core Principles

- **Ledger-First**: Double-entry bookkeeping as the source of financial truth
- **Idempotency**: Every operation can be safely retried without side effects
- **Event-Driven**: All state changes publish events for decoupled processing
- **PSP Agnostic**: Normalized abstraction over multiple payment service providers
- **Failure-Aware**: Circuit breakers, retries, and graceful degradation built-in

## Architecture Highlights

### Components

1. **Payment Intent Service** - State machine for payment lifecycle (CREATED → AUTHORIZED → CAPTURED → FAILED → REFUNDED)
2. **Ledger Service** - Immutable double-entry accounting with invariant enforcement
3. **Orchestrator** - Intelligent PSP routing with capability-aware selection, retries, and circuit breakers
4. **PSP Connector Layer** - Normalized adapters for Stripe, Adyen, and other providers
5. **Webhook Ingestion** - Deduplication and order-independent event processing
6. **Outbox Pattern** - Transactional event publishing with exactly-once guarantees
7. **Billing Service** - Event-sourced subscriptions and deterministic invoice generation
8. **Collections & Dunning** - Smart retry logic for failed payment collection

### Guarantees

- **Exactly-once state transitions** - Optimistic locking prevents concurrent updates
- **At-least-once event delivery** - With idempotent consumers
- **Deterministic replays** - Events can be replayed for audits and recovery
- **Zero hidden coupling** - PSP behavior never affects internal correctness

## Tech Stack

- **Language**: Go 1.22+
- **Database**: PostgreSQL 15+ (ACID transactions, serializable isolation)
- **Cache**: Redis (idempotency keys, rate limits)
- **Message Bus**: Kafka/Redpanda (event streaming)
- **Observability**: Prometheus, OpenTelemetry, structured logging (Zap/Zerolog)
- **Infrastructure**: Docker, Docker Compose

## Getting Started

### Prerequisites

- Go 1.22 or higher
- Docker and Docker Compose
- Make

### Quick Start

```bash
# Clone the repository
git clone <repository-url>
cd PlayFlow

# Start local infrastructure (Postgres, Redis, Redpanda)
make infra-up

# Run database migrations
make migrate-up

# Run tests
make test

# Start the API server
make run-api

# Start the worker (outbox publisher)
make run-worker
```

### Local Development

```bash
# Run all tests
make test

# Run tests with coverage
make test-cover

# Run linter
make lint

# Format code
make fmt

# Start all services
make up

# Stop all services
make down

# View logs
make logs
```

## Project Structure

```
payflow/
├── cmd/
│   ├── api/          # REST API server entry point
│   └── worker/       # Background worker for outbox processing
├── internal/
│   ├── payments/     # Payment intent state machine
│   ├── ledger/       # Double-entry ledger service
│   ├── outbox/       # Transactional outbox pattern
│   ├── psp/          # PSP connector layer
│   └── platform/     # Database, config, observability
├── pkg/
│   ├── idempotency/  # Idempotent request handling
│   ├── state/        # State machine framework
│   ├── retry/        # Retry with exponential backoff
│   └── circuitbreaker/ # Circuit breaker implementation
├── migrations/       # Database migration files
├── docker/           # Dockerfiles and configs
└── docs/             # Additional documentation
```

## API Examples

### Create Payment Intent

```bash
curl -X POST http://localhost:8080/v1/payment_intents \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: unique-key-123" \
  -d '{
    "merchant_id": "merchant_abc",
    "amount": 10000,
    "currency": "USD"
  }'
```

### Authorize Payment

```bash
curl -X POST http://localhost:8080/v1/payment_intents/{id}/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: auth-key-456"
```

### Capture Payment

```bash
curl -X POST http://localhost:8080/v1/payment_intents/{id}/capture \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: capture-key-789" \
  -d '{
    "amount": 10000
  }'
```

### Refund Payment

```bash
curl -X POST http://localhost:8080/v1/payment_intents/{id}/refund \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: refund-key-012" \
  -d '{
    "amount": 5000,
    "reason": "customer_request"
  }'
```

## Testing Strategy

### Test Pyramid

1. **Unit Tests** (fast, many)
   - State machine transitions
   - Retry/circuit breaker behavior
   - Ledger balancing rules

2. **Integration Tests** (medium speed)
   - Postgres transactions
   - Idempotency key deduplication
   - Outbox event processing

3. **Scenario/Chaos Tests** (slower)
   - 1k-10k synthetic payments with fault injection
   - Validate invariants (no duplicate ledger entries, no invalid transitions)
   - Deterministic replay after crashes

### Running Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/ledger/...

# Run integration tests (requires Docker)
make test-integration

# Run with race detector
make test-race

# Property-based tests for ledger
go test -run TestLedgerInvariants ./internal/ledger/...
```

### Test Results

✅ **All tests passing!** See [TEST_RESULTS.md](TEST_RESULTS.md) and [E2E_TEST_REPORT.md](E2E_TEST_REPORT.md) for comprehensive test reports.

**Quick Stats:**
- **28 tests** total (14 unit + 14 integration)
- **100% pass rate**
- **~2.5s** total execution time
- **68.7%** payment service coverage
- **63.9%** ledger service coverage
- **100%** critical path coverage

**Key Validations:**
- ✅ Double-entry bookkeeping (all transactions balance)
- ✅ Payment state machine (valid transitions only)
- ✅ Concurrent operations (optimistic locking works)
- ✅ Idempotency (duplicate requests handled)
- ✅ 10 concurrent payments maintain consistency

## Observability

### Metrics (Prometheus)

- `payment_authorize_latency_ms` - Authorization latency histogram
- `psp_error_rate{provider}` - Error rate by PSP provider
- `idempotency_hits_total` - Idempotency key cache hits
- `outbox_lag_seconds` - Event publishing lag
- `circuit_breaker_state{provider}` - Circuit breaker status

### Logging

All logs are structured JSON with:
- Correlation IDs for request tracing
- Operation context (payment_intent_id, merchant_id)
- Error details with stack traces

### Tracing

OpenTelemetry traces span the entire flow:
`API → Orchestrator → PSP → Webhook → Ledger`

## Configuration

Configuration via environment variables:

```bash
# Database
DATABASE_URL=postgres://user:pass@localhost:5432/payflow?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379

# Kafka/Redpanda
KAFKA_BROKERS=localhost:9092

# API Server
API_PORT=8080
API_TIMEOUT=30s

# PSP Configuration
STRIPE_API_KEY=sk_test_...
ADYEN_API_KEY=...

# Observability
LOG_LEVEL=info
PROMETHEUS_PORT=9090
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

## Deployment

### Docker

```bash
# Build all images
make docker-build

# Run with Docker Compose
docker-compose up -d

# Scale workers
docker-compose up -d --scale worker=3
```

### Production Considerations

- Use connection pooling for Postgres (pgbouncer)
- Run multiple API instances behind load balancer
- Deploy workers with sufficient concurrency for outbox processing
- Set up Prometheus scraping and Grafana dashboards
- Configure circuit breaker thresholds per PSP
- Enable audit logging for compliance

## Development Phases

The project is built incrementally:

- **Phase 0**: Repository bootstrap and tooling
- **Phase 1**: Double-entry ledger with invariant tests
- **Phase 2**: Payment intent state machine with idempotency
- **Phase 3**: Orchestrator with PSP mocks and failure injection
- **Phase 4**: Outbox pattern for exactly-once event processing
- **Phase 5**: Billing service with event sourcing
- **Phase 6**: Observability and SLO-based alerting

See [DESIGN.md](docs/DESIGN.md) for detailed architecture documentation.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for your changes
4. Ensure all tests pass (`make test`)
5. Run the linter (`make lint`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## License

MIT License - see LICENSE file for details

## Support

For questions or issues, please open an issue on GitHub.

---

**Built with ❤️ for production-grade payment processing**
