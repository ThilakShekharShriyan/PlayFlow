# PayFlow Testing Guide

This document describes the comprehensive testing strategy for PayFlow.

## Test Types

### 1. Unit Tests
Fast, isolated tests that don't require external dependencies.

**Run unit tests:**
```bash
make test-unit
# or
go test -v -short ./...
```

**Coverage:**
- State machine transitions
- Validation logic
- Business rules
- Utility functions

### 2. Integration Tests  
Tests that verify components work together correctly with real database.

**Prerequisites:**
- PostgreSQL running on `localhost:5432`
- User: `payflow` / Password: `payflow`
- Superuser privileges to create/drop test databases

**Run integration tests:**
```bash
# Start infrastructure first
make infra-up

# Run all integration tests
make test-integration

# Run specific integration test suites
go test -v -tags=integration ./internal/payments -run Integration
go test -v -tags=integration ./internal/ledger -run Integration
go test -v -tags=integration ./internal -run Integration
```

### 3. End-to-End Tests
Complete workflow tests combining multiple services.

**Covered scenarios:**
- Full payment lifecycle (Create → Authorize → Capture)
- Refund flows
- Ledger consistency across operations
- Concurrent payment processing

## Test Coverage

### Payment Intent Tests (`internal/payments/integration_test.go`)

✅ **Happy Path Scenarios:**
- Create → Authorize → Capture flow
- Captured → Refunded flow
- Partial capture support

✅ **State Machine Validation:**
- Invalid transitions rejected
- State progression enforced
- Cannot skip states (e.g., CREATED → CAPTURED fails)

✅ **Idempotency:**
- Duplicate requests return same result
- Same key + different merchant = different intents
- Idempotency keys scoped per merchant

✅ **Concurrency:**
- Optimistic locking prevents race conditions
- Only one concurrent operation succeeds
- Version mismatch errors for conflicts

✅ **Validation:**
- Zero/negative amounts rejected
- Missing merchant ID rejected
- Missing currency rejected
- Cannot capture more than authorized

### Ledger Tests (`internal/ledger/integration_test.go`)

✅ **Double-Entry Invariants:**
- All transactions balance to zero
- Unbalanced transactions rejected
- Multiple transactions maintain invariant

✅ **Immutability:**
- Ledger entries cannot be modified
- Append-only guarantee enforced
- Duplicate transaction IDs rejected

✅ **Account Balancing:**
- Debit/credit calculations correct
- Multi-transaction balances accurate
- Negative balances supported (liabilities)

✅ **Complex Scenarios:**
- Authorization holds
- Capture with fees
- Refund reversals
- Multi-step workflows

✅ **Validation:**
- Minimum 2 entries required
- Zero amount entries rejected
- Missing currency rejected
- Missing account ID rejected

### Integration Tests (`internal/integration_test.go`)

✅ **Payment + Ledger Integration:**
- Payment state changes trigger ledger postings
- Ledger balances reflect payment operations
- Failed payments don't affect ledger
- System-wide balance always zero

✅ **Complete Workflows:**
- Authorization → Ledger hold
- Capture → Ledger settlement with fees
- Refund → Ledger reversal
- Multi-payment aggregation

✅ **Concurrent Operations:**
- 10+ concurrent payments processed correctly
- Ledger consistency maintained under load
- No duplicate ledger entries
- Final balances match expected totals

## Running Tests

### Quick Test (Unit Only)
```bash
make test-quick
```
**Duration:** ~1 second  
**No database required**

### Standard Test (Unit + Integration)
```bash
make infra-up          # Start PostgreSQL
make test-integration  # Run integration tests
```
**Duration:** ~10-30 seconds  
**Requires:** PostgreSQL running

### Full Test Suite
```bash
make infra-up
make test-all
```
**Duration:** ~30-60 seconds  
**Includes:** All unit and integration tests

### With Coverage Report
```bash
make infra-up
make test-cover
open coverage.html
```

### Continuous Testing (Watch Mode)
```bash
make watch-test  # Requires 'entr' tool
```

## Test Database Management

Tests use a **disposable database strategy**:

1. Each test creates a unique database: `test_payflow_<timestamp>`
2. Migrations applied automatically
3. Database dropped after test completion
4. No manual cleanup required

**Parallel test execution supported** - each test gets its own database.

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_USER: payflow
          POSTGRES_PASSWORD: payflow
          POSTGRES_DB: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
    
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      
      - name: Run tests
        run: make test-all
        env:
          DATABASE_URL: postgres://payflow:payflow@localhost:5432/postgres?sslmode=disable
```

## Test Scenarios Covered

### 1. Payment Flow Tests

| Scenario | Test | Assertion |
|----------|------|-----------|
| Create intent | `TestPaymentFlow_EndToEnd` | State = CREATED, Version = 0 |
| Authorize payment | `TestPaymentFlow_EndToEnd` | State = AUTHORIZED, Version = 1 |
| Capture payment | `TestPaymentFlow_EndToEnd` | State = CAPTURED, Version = 2 |
| Refund payment | `TestPaymentFlow_EndToEnd` | State = REFUNDED, Version = 3 |
| Invalid transition | `TestPaymentFlow_EndToEnd` | ErrInvalidTransition |
| Partial capture | `TestPaymentFlow_EndToEnd` | Amount < Intent.Amount succeeds |
| Over-capture | `TestPaymentFlow_EndToEnd` | Amount > Intent.Amount fails |

### 2. Idempotency Tests

| Scenario | Test | Assertion |
|----------|------|-----------|
| Duplicate create | `TestPaymentFlow_Idempotency` | Same intent ID returned |
| Same key, different merchant | `TestPaymentFlow_Idempotency` | Different intent IDs |

### 3. Concurrency Tests

| Scenario | Test | Assertion |
|----------|------|-----------|
| 5 concurrent authorizes | `TestPaymentFlow_ConcurrentOperations` | 1 success, 4 version mismatches |
| Final state | `TestPaymentFlow_ConcurrentOperations` | State = AUTHORIZED, Version = 1 |

### 4. Ledger Tests

| Scenario | Test | Assertion |
|----------|------|-----------|
| Post balanced txn | `TestLedger_EndToEnd` | Success, sum(entries) = 0 |
| Post unbalanced txn | `TestLedger_EndToEnd` | ErrUnbalancedTransaction |
| Account balance | `TestLedger_EndToEnd` | Correct debit/credit calculation |
| Multi-transaction | `TestLedger_EndToEnd` | System balance = 0 |
| Immutability | `TestLedger_EndToEnd` | Duplicate transaction ID fails |
| Authorization hold | `TestLedger_ComplexScenarios` | Funds reserved in payable |
| Capture with fee | `TestLedger_ComplexScenarios` | Fee deducted correctly |
| Refund | `TestLedger_ComplexScenarios` | Funds returned to customer |

### 5. Integration Tests

| Scenario | Test | Assertion |
|----------|------|-----------|
| Full flow with ledger | `TestPaymentAndLedger_FullIntegration` | Payment + ledger consistent |
| Refund with reversal | `TestPaymentAndLedger_FullIntegration` | Balances return to zero |
| Multiple payments | `TestPaymentAndLedger_FullIntegration` | Aggregate balances correct |
| Failed payment | `TestPaymentAndLedger_FullIntegration` | Ledger unaffected |
| 10 concurrent payments | `TestConcurrentPaymentsWithLedger` | System balance = 0 |

## Debugging Failed Tests

### View Test Output
```bash
go test -v -tags=integration ./internal/payments
```

### Run Single Test
```bash
go test -v -tags=integration ./internal/payments -run TestPaymentFlow_EndToEnd
```

### Enable SQL Logging
Set environment variable:
```bash
export PQLOG=1
go test -v -tags=integration ./internal/payments
```

### Check PostgreSQL
```bash
psql postgres://payflow:payflow@localhost:5432/postgres

-- List test databases
\l test_payflow_*

-- Connect to test database
\c test_payflow_1234567890

-- Inspect tables
\dt
SELECT * FROM payment_intents;
SELECT * FROM ledger_entries;
```

## Performance Benchmarks

Run benchmarks:
```bash
go test -bench=. -benchmem ./...
```

Expected performance (indicative):
- Create payment intent: ~1ms
- Authorize payment: ~2ms
- Post ledger transaction: ~3ms
- Account balance query: <1ms

## Next Steps

1. **Add Property-Based Tests** - Use `gopter` or `rapid` for generative testing
2. **Chaos Testing** - Inject network failures, database timeouts
3. **Load Testing** - Use `k6` or `vegeta` for stress testing
4. **E2E API Tests** - Test REST API endpoints with real HTTP
5. **Observability Tests** - Verify metrics, logs, traces are emitted

## Test Maintenance

- **Keep tests independent** - Each test should create its own data
- **Use table-driven tests** - Easy to add new scenarios
- **Test failures, not just success** - Validate error cases
- **Keep tests fast** - Unit tests < 10ms, integration tests < 100ms
- **Document assumptions** - Add comments for complex scenarios

---

**All tests passing = System is production-ready! ✅**
