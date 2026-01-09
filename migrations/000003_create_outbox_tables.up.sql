-- Create outbox_events table
CREATE TABLE outbox_events (
    id VARCHAR(255) PRIMARY KEY,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP
);

-- Create inbox_events table for deduplication
CREATE TABLE inbox_events (
    event_id VARCHAR(255) PRIMARY KEY,
    processed_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for outbox_events
CREATE INDEX idx_outbox_events_unpublished ON outbox_events(created_at) WHERE published_at IS NULL;
CREATE INDEX idx_outbox_events_aggregate_id ON outbox_events(aggregate_id);

-- Indexes for inbox_events
CREATE INDEX idx_inbox_events_processed_at ON inbox_events(processed_at);
