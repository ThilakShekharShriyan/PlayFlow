# PayFlow — Design Documentation

## Table of Contents

1. [High-Level Architecture](#high-level-architecture)
2. [Design Principles](#design-principles)
3. [Core Components](#core-components)
4. [Data Flow](#data-flow)
5. [State Machines](#state-machines)
6. [Consistency Guarantees](#consistency-guarantees)
7. [Failure Scenarios](#failure-scenarios)
8. [Scalability](#scalability)

---

## High-Level Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────────┐
│         API Gateway & Auth              │
│  - Idempotency key extraction           │
│  - Correlation ID injection             │
│  - Rate limiting                        │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│      Payment Intent Service             │
│  - State machine (CREATED → CAPTURED)   │
│  - Optimistic locking                   │
│  - Version management                   │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│         Orchestrator                    │
│  - PSP routing & capability matching    │
│  - Circuit breaker per provider         │
│  - Retry with exponential backoff       │
└──────────────┬──────────────────────────┘
               │
        ┌──────┴──────┬──────────┐
        ▼             ▼          ▼
   ┌────────┐   ┌────────┐  ┌────────┐
   │ Stripe │   │ Adyen  │  │  Mock  │
   └────────┘   └────────┘  └────────┘
        │             │          │
        └──────┬──────┴──────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│        Webhook Ingestion                │
│  - Signature validation                 │
│  - Event deduplication (inbox)          │
│  - Order-independent handling           │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│     Transactional Outbox                │
│  - Update state + insert event (atomic) │
│  - Async publisher (worker)             │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│         Event Bus (Kafka)               │
│  - payment.authorized                   │
│  - payment.captured                     │
│  - payment.refunded                     │
└──────────────┬──────────────────────────┘
               │
        ┌──────┴──────┬──────────┐
        ▼             ▼          ▼
   ┌────────┐   ┌────────┐  ┌────────┐
   │ Ledger │   │Billing │  │Analytics│
   └────────┘   └────────┘  └────────┘
```

---

## Design Principles

### 1. Ledger-First Architecture

**Principle**: The ledger is the single source of financial truth. Operational state (payment intents) reconciles to the ledger, not vice versa.

**Implementation**:
- Every financial operation posts to the ledger
- Ledger entries are immutable and append-only
- Double-entry bookkeeping enforces balance invariants
- Operational state (payment intents) is derived, not canonical

**Why**:
- Auditable: Complete financial history
- Recoverable: Can rebuild state from ledger
- Provably correct: Mathematical invariants (Σ debits = Σ credits)

### 2. Idempotency Everywhere

**Principle**: Every externally visible operation must be idempotent—same input produces same output, no matter how many times it's called.

**Implementation**:
- Idempotency keys extracted at API gateway
- Keys stored with request hash and response snapshot
- Duplicate requests return cached response
- Database constraints enforce uniqueness

**Why**:
- Safe retries: Network failures don't cause double-charging
- Client simplicity: Retry without fear
- Deterministic: Same input → same outcome

### 3. Event-Driven Core

**Principle**: State changes publish events. Side effects are handled by event consumers, not inline.

**Implementation**:
- Outbox pattern: State update + event insertion in one transaction
- Async workers publish events to Kafka
- Consumers are idempotent (inbox deduplication)
- Events are versioned and backward-compatible

**Why**:
- Decoupling: Payment flow doesn't block on ledger posting
- Scalability: Event consumers scale independently
- Replayability: Rebuild projections from event stream

### 4. Failure as First-Class Design

**Principle**: Every integration point can fail. Design for it, don't react to it.

**Implementation**:
- Circuit breakers per PSP
- Retries only on retryable errors (not hard declines)
- Timeouts on all external calls
- Fallback to secondary PSPs
- Crash recovery via event replay

**Why**:
- Resilience: System stays up when dependencies fail
- Predictability: Known behavior under failure
- Graceful degradation: Partial functionality better than none

---

## Core Components

### 1. Payment Intent Service

**Responsibilities**:
- Manage payment lifecycle state
- Enforce valid state transitions
- Coordinate with orchestrator for PSP operations

**Data Model**:
```go
type PaymentIntent struct {
    ID                string
    MerchantID        string
    Amount            int64  // Amount in minor units (cents)
    Currency          string
    State             PaymentState
    Version           int64  // Optimistic lock
    IdempotencyKey    string
    SelectedProvider  string
    ProviderPaymentID string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}

type PaymentState string

const (
    StateCreated    PaymentState = "CREATED"
    StateAuthorized PaymentState = "AUTHORIZED"
    StateCaptured   PaymentState = "CAPTURED"
    StateFailed     PaymentState = "FAILED"
    StateRefunded   PaymentState = "REFUNDED"
)
```

**State Transition Rules**:
```
CREATED → AUTHORIZED  (authorize succeeds)
CREATED → FAILED      (authorize fails)
AUTHORIZED → CAPTURED (capture succeeds)
AUTHORIZED → FAILED   (capture fails)
CAPTURED → REFUNDED   (refund succeeds)
```

Invalid transitions are rejected deterministically.

**Optimistic Locking**:
```sql
UPDATE payment_intents
SET state = $1, version = version + 1
WHERE id = $2 AND version = $3
```

If `version` doesn't match, update fails → concurrent modification detected.

### 2. Ledger Service

**Responsibilities**:
- Record all financial movements
- Enforce double-entry balance invariant
- Provide audit trail

**Data Model**:
```go
type Account struct {
    ID          string
    Name        string
    Type        AccountType // ASSET, LIABILITY, REVENUE, EXPENSE
    Currency    string
}

type Transaction struct {
    ID          string
    Description string
    CreatedAt   time.Time
}

type LedgerEntry struct {
    ID            string
    TransactionID string
    AccountID     string
    Amount        int64  // Positive for debit, negative for credit
    Currency      string
    CreatedAt     time.Time
}
```

**Invariant**: For each transaction, `SUM(amount) = 0`.

**Example**: Customer pays $100, merchant receives $97, platform takes $3 fee.

```
Transaction: txn_123
Entries:
  - Account: customer_cash,      Amount: -10000  (credit, money out)
  - Account: merchant_receivable, Amount: +9700   (debit, money in)
  - Account: platform_fee,        Amount: +300    (debit, revenue)
SUM = -10000 + 9700 + 300 = 0 ✓
```

**Posting API**:
```go
func (s *LedgerService) PostTransaction(ctx context.Context, req PostTransactionRequest) error {
    if !req.IsBalanced() {
        return ErrUnbalancedTransaction
    }
    // Begin DB transaction
    // Insert transaction
    // Insert all entries
    // Commit
}
```

### 3. Orchestrator

**Responsibilities**:
- Choose optimal PSP
- Execute retries and fallbacks
- Circuit breaker management

**Routing Logic**:
```go
func (o *Orchestrator) SelectProvider(ctx context.Context, intent *PaymentIntent) (string, error) {
    capabilities := o.requiredCapabilities(intent)
    
    candidates := o.filterByCapabilities(capabilities)
    candidates = o.filterByCircuitState(candidates)
    
    if len(candidates) == 0 {
        return "", ErrNoAvailableProvider
    }
    
    return o.scoreAndSelect(candidates), nil
}
```

**Retry Policy**:
```go
func (o *Orchestrator) AuthorizeWithRetry(ctx context.Context, intent *PaymentIntent) error {
    backoff := NewExponentialBackoff(100*time.Millisecond, 5*time.Second, 3)
    
    for attempt := 0; attempt < maxAttempts; attempt++ {
        resp, err := o.connector.Authorize(ctx, intent)
        
        if err == nil {
            return nil
        }
        
        if !isRetryable(err) {
            return err
        }
        
        time.Sleep(backoff.Next())
    }
    
    return ErrMaxRetriesExceeded
}
```

**Circuit Breaker**:
- **Closed**: Normal operation
- **Open**: Too many failures, reject immediately
- **Half-Open**: Allow one probe request to test recovery

### 4. PSP Connector Layer

**Interface**:
```go
type PSPConnector interface {
    Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResponse, error)
    Capture(ctx context.Context, req CaptureRequest) (CaptureResponse, error)
    Refund(ctx context.Context, req RefundRequest) (RefundResponse, error)
    Void(ctx context.Context, req VoidRequest) (VoidResponse, error)
}

type AuthorizeResponse struct {
    Status            ResponseStatus
    ProviderPaymentID string
    ErrorCode         string
    Retryable         bool
}
```

**Canonical Error Model**:
```go
const (
    ErrorInsufficientFunds ErrorCode = "INSUFFICIENT_FUNDS"
    ErrorSoftDecline       ErrorCode = "SOFT_DECLINE"
    ErrorHardDecline       ErrorCode = "HARD_DECLINE"
    ErrorNetworkError      ErrorCode = "NETWORK_ERROR"
    ErrorInvalidCard       ErrorCode = "INVALID_CARD"
)
```

### 5. Outbox Pattern

**Schema**:
```sql
CREATE TABLE outbox_events (
    id              UUID PRIMARY KEY,
    aggregate_id    VARCHAR NOT NULL,
    event_type      VARCHAR NOT NULL,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMP
);

CREATE INDEX idx_outbox_unpublished ON outbox_events (created_at)
WHERE published_at IS NULL;
```

**Transactional Write**:
```go
func (s *PaymentService) Authorize(ctx context.Context, id string) error {
    tx, _ := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // Update payment intent state
    _, err := tx.Exec(`
        UPDATE payment_intents
        SET state = 'AUTHORIZED', version = version + 1
        WHERE id = $1 AND version = $2
    `, id, expectedVersion)
    
    // Insert outbox event
    _, err = tx.Exec(`
        INSERT INTO outbox_events (id, aggregate_id, event_type, payload)
        VALUES ($1, $2, 'payment.authorized', $3)
    `, uuid.New(), id, payloadJSON)
    
    return tx.Commit()
}
```

**Worker**:
```go
func (w *OutboxWorker) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)
    
    for {
        select {
        case <-ticker.C:
            events := w.fetchUnpublished()
            for _, event := range events {
                w.publish(event)
                w.markPublished(event.ID)
            }
        case <-ctx.Done():
            return
        }
    }
}
```

---

## Data Flow

### Happy Path: Authorize + Capture

1. **Client** sends `POST /v1/payment_intents` with idempotency key
2. **API Gateway** extracts key, injects correlation ID
3. **Payment Service** creates intent in `CREATED` state
4. **Client** sends `POST /v1/payment_intents/{id}/authorize`
5. **Orchestrator** selects PSP (e.g., Stripe)
6. **PSP Connector** calls Stripe API, normalizes response
7. **Payment Service** updates intent to `AUTHORIZED` + inserts outbox event
8. **Outbox Worker** publishes `payment.authorized` to Kafka
9. **Ledger Consumer** posts double-entry ledger transaction
10. **Client** sends `POST /v1/payment_intents/{id}/capture`
11. **Orchestrator** calls PSP to capture funds
12. **Payment Service** updates intent to `CAPTURED` + inserts outbox event
13. **Outbox Worker** publishes `payment.captured` to Kafka
14. **Ledger Consumer** posts settlement ledger entries

### Failure Scenario: PSP Timeout

1. **Client** sends authorize request
2. **Orchestrator** calls PSP
3. **PSP** times out (no response after 5s)
4. **Orchestrator** retries with exponential backoff
5. **PSP** responds on retry (may have authorized on first call)
6. **Payment Service** updates state (idempotent—same result if authorized twice)
7. **Webhook** arrives from PSP (async confirmation)
8. **Webhook Service** deduplicates via inbox pattern
9. **State** reconciles (webhook matches intent state)

### Failure Scenario: Duplicate API Call

1. **Client** sends authorize with idempotency key `key_abc`
2. **API Gateway** checks idempotency store (Redis + Postgres)
3. **Not found** → proceed
4. **Payment Service** authorizes, stores result with key
5. **Network fails** before client receives response
6. **Client retries** with same idempotency key `key_abc`
7. **API Gateway** finds key → returns cached response
8. **No duplicate charge**

---

## State Machines

### Payment Intent State Machine

```
          ┌─────────┐
          │ CREATED │
          └────┬────┘
               │
        ┌──────┴───────┐
        ▼              ▼
   ┌──────────┐   ┌────────┐
   │AUTHORIZED│   │ FAILED │
   └────┬─────┘   └────────┘
        │
        ▼
   ┌─────────┐
   │CAPTURED │
   └────┬────┘
        │
        ▼
   ┌─────────┐
   │REFUNDED │
   └─────────┘
```

**Transition Table**:
```go
var allowedTransitions = map[PaymentState][]PaymentState{
    StateCreated:    {StateAuthorized, StateFailed},
    StateAuthorized: {StateCaptured, StateFailed},
    StateCaptured:   {StateRefunded},
    StateFailed:     {},
    StateRefunded:   {},
}

func (s *StateMachine) CanTransition(from, to PaymentState) bool {
    allowed, ok := allowedTransitions[from]
    if !ok {
        return false
    }
    for _, state := range allowed {
        if state == to {
            return true
        }
    }
    return false
}
```

### Circuit Breaker State Machine

```
      ┌────────┐
      │ CLOSED │  (normal operation)
      └───┬────┘
          │
          │ failures > threshold
          ▼
      ┌────────┐
      │  OPEN  │  (reject immediately)
      └───┬────┘
          │
          │ timeout expires
          ▼
    ┌──────────┐
    │HALF-OPEN │  (allow 1 probe)
    └─────┬────┘
          │
     ┌────┴────┐
     ▼         ▼
  success    failure
     │         │
     ▼         ▼
  CLOSED    OPEN
```

---

## Consistency Guarantees

### Exactly-Once State Transitions

**Problem**: Concurrent requests must not cause duplicate state changes.

**Solution**: Optimistic locking with version field.

```sql
UPDATE payment_intents
SET state = 'AUTHORIZED', version = version + 1
WHERE id = $1 AND version = $2
```

Only one concurrent update succeeds. Others get zero rows affected → retry.

### At-Least-Once Event Delivery

**Problem**: Events must not be lost, even if publisher crashes.

**Solution**: Outbox pattern with persistent storage.

1. State update + event insert in same DB transaction
2. Worker polls outbox table
3. Publishes to Kafka
4. Marks as published

If worker crashes, event remains unpublished → retried on restart.

### Idempotent Event Consumers

**Problem**: Events may be delivered multiple times.

**Solution**: Inbox pattern with deduplication.

```sql
CREATE TABLE inbox_events (
    event_id VARCHAR PRIMARY KEY,
    processed_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

Before processing:
```go
_, err := db.Exec("INSERT INTO inbox_events (event_id) VALUES ($1)", event.ID)
if err != nil {
    // Already processed, skip
    return nil
}
// Process event
```

### Deterministic Replays

**Problem**: Replaying events must produce identical results.

**Solution**:
- Event payload contains all context (no lookups)
- Explicit rounding rules for calculations
- Versioned event schemas

---

## Failure Scenarios

| Failure | Detection | Mitigation | Outcome |
|---------|-----------|------------|---------|
| PSP timeout | Context deadline | Retry with backoff | No double charge |
| Duplicate API call | Idempotency key check | Return cached response | Same result |
| Webhook arrives twice | Inbox deduplication | Skip processing | Idempotent |
| Service crash mid-flow | Event replay | Resume from outbox | Exactly-once |
| DB connection lost | Connection pool retry | Transient error | Retry succeeds |
| Circuit breaker open | Health check | Route to fallback PSP | Degraded but functional |
| Event replay | Inbox check | Skip if processed | Deterministic state |

---

## Scalability

### Horizontal Scaling

- **API servers**: Stateless, scale to N instances behind load balancer
- **Workers**: Scale by partitioning outbox table or Kafka consumer groups
- **Ledger consumers**: Partition by transaction ID

### Database Optimization

- **Read replicas**: Route read-only queries to replicas
- **Connection pooling**: pgbouncer in transaction mode
- **Indexes**: On `(merchant_id, created_at)`, `(state, version)`, `(idempotency_key)`
- **Partitioning**: Ledger entries by month

### Caching Strategy

- **Idempotency keys**: Redis with 24-hour TTL
- **PSP capabilities**: Redis with 1-hour TTL
- **Circuit breaker state**: In-memory per instance

### Message Bus Tuning

- **Kafka partitions**: Partition by `payment_intent_id` for ordering
- **Consumer groups**: One per service type (ledger, billing, analytics)
- **Retention**: 7 days for events, 30 days for ledger events

---

## Testing Strategy

### Unit Tests

```go
func TestStateMachine_ValidTransitions(t *testing.T) {
    sm := NewStateMachine()
    
    assert.True(t, sm.CanTransition(StateCreated, StateAuthorized))
    assert.False(t, sm.CanTransition(StateCaptured, StateCreated))
}
```

### Property-Based Tests

```go
func TestLedger_AlwaysBalanced(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        entries := generateBalancedEntries(t)
        
        err := ledger.PostTransaction(ctx, entries)
        assert.NoError(t, err)
        
        sum := sumEntries(entries)
        assert.Equal(t, 0, sum)
    })
}
```

### Integration Tests

```go
func TestPayment_AuthorizeCapture(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    
    svc := NewPaymentService(db)
    
    intent, _ := svc.Create(ctx, CreateRequest{Amount: 1000})
    svc.Authorize(ctx, intent.ID)
    svc.Capture(ctx, intent.ID, 1000)
    
    final, _ := svc.Get(ctx, intent.ID)
    assert.Equal(t, StateCaptured, final.State)
}
```

### Chaos Tests

```go
func TestOrchestrator_PSPFailures(t *testing.T) {
    mockPSP := NewFlakyPSP(failureRate: 0.5)
    orch := NewOrchestrator(mockPSP)
    
    for i := 0; i < 1000; i++ {
        intent := generateIntent()
        err := orch.Authorize(ctx, intent)
        
        // Eventually succeeds or deterministically fails
        if err != nil {
            assert.True(t, isExpectedError(err))
        }
    }
}
```

---

## Observability

### Structured Logging

```go
logger.Info("payment authorized",
    zap.String("correlation_id", corrID),
    zap.String("payment_intent_id", id),
    zap.String("provider", provider),
    zap.Int64("amount", amount),
    zap.Duration("latency", latency),
)
```

### Metrics

```go
paymentAuthorizeLatency.WithLabelValues(provider).Observe(latency.Seconds())
pspErrorRate.WithLabelValues(provider, errorCode).Inc()
circuitBreakerState.WithLabelValues(provider).Set(stateValue)
```

### Tracing

```go
ctx, span := tracer.Start(ctx, "payment.authorize")
defer span.End()

span.SetAttributes(
    attribute.String("payment_intent_id", id),
    attribute.String("provider", provider),
)
```

---

## Security Considerations

- **API keys**: Scoped permissions (`payments:write`, `billing:read`)
- **Secrets**: Stored in Vault, never in config files
- **PCI compliance**: No raw PAN storage, use tokenization
- **Audit logs**: All financial operations logged immutably
- **Rate limiting**: Per merchant, per API key
- **Network**: TLS 1.3 for all external communication

---

**This design ensures correctness, auditability, and resilience under failure—the cornerstones of production payment infrastructure.**
