-- Create payment_intents table
CREATE TABLE payment_intents (
    id VARCHAR(255) PRIMARY KEY,
    merchant_id VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    state VARCHAR(50) NOT NULL CHECK (state IN ('CREATED', 'AUTHORIZED', 'CAPTURED', 'FAILED', 'REFUNDED')),
    version BIGINT NOT NULL DEFAULT 0,
    idempotency_key VARCHAR(255),
    selected_provider VARCHAR(100),
    provider_payment_id VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create idempotency_records table
CREATE TABLE idempotency_records (
    id VARCHAR(255) PRIMARY KEY,
    merchant_id VARCHAR(255) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    response_body TEXT,
    status_code INT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    UNIQUE (merchant_id, idempotency_key)
);

-- Indexes for payment_intents
CREATE INDEX idx_payment_intents_merchant_id ON payment_intents(merchant_id);
CREATE INDEX idx_payment_intents_state ON payment_intents(state);
CREATE INDEX idx_payment_intents_created_at ON payment_intents(created_at);
CREATE INDEX idx_payment_intents_idempotency_key ON payment_intents(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_payment_intents_provider_payment_id ON payment_intents(provider_payment_id) WHERE provider_payment_id IS NOT NULL;

-- Indexes for idempotency_records
CREATE INDEX idx_idempotency_records_expires_at ON idempotency_records(expires_at);
