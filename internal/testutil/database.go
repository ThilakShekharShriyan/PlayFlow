package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

type TestDB struct {
	DB     *sql.DB
	DBName string
}

func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	baseURL := "postgres://payflow:payflow@localhost:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", baseURL)
	if err != nil {
		t.Fatalf("Failed to connect to postgres: %v", err)
	}

	dbName := fmt.Sprintf("test_payflow_%d", time.Now().UnixNano())
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	testURL := fmt.Sprintf("postgres://payflow:payflow@localhost:5432/%s?sslmode=disable", dbName)
	testDB, err := sql.Open("postgres", testURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	testDB.SetMaxOpenConns(10)
	testDB.SetMaxIdleConns(5)
	testDB.SetConnMaxLifetime(5 * time.Minute)

	return &TestDB{
		DB:     testDB,
		DBName: dbName,
	}
}

func (tdb *TestDB) Close(t *testing.T) {
	t.Helper()

	dbName := tdb.DBName
	tdb.DB.Close()

	baseURL := "postgres://payflow:payflow@localhost:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", baseURL)
	if err != nil {
		t.Logf("Failed to connect to postgres for cleanup: %v", err)
		return
	}
	defer db.Close()

	_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	if err != nil {
		t.Logf("Failed to drop test database: %v", err)
	}
}

func (tdb *TestDB) ApplyMigrations(t *testing.T) {
	t.Helper()

	migrations := []string{
		`CREATE TABLE accounts (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL CHECK (type IN ('ASSET', 'LIABILITY', 'REVENUE', 'EXPENSE')),
			currency VARCHAR(3) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE transactions (
			id VARCHAR(255) PRIMARY KEY,
			description TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE ledger_entries (
			id VARCHAR(255) PRIMARY KEY,
			transaction_id VARCHAR(255) NOT NULL REFERENCES transactions(id),
			entry_index INT NOT NULL,
			account_id VARCHAR(255) NOT NULL REFERENCES accounts(id),
			amount BIGINT NOT NULL,
			currency VARCHAR(3) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			UNIQUE (transaction_id, entry_index)
		)`,
		`CREATE INDEX idx_ledger_entries_transaction_id ON ledger_entries(transaction_id)`,
		`CREATE INDEX idx_ledger_entries_account_id ON ledger_entries(account_id)`,
		`INSERT INTO accounts (id, name, type, currency) VALUES
			('acc_customer_cash', 'Customer Cash', 'ASSET', 'USD'),
			('acc_merchant_receivable', 'Merchant Receivable', 'ASSET', 'USD'),
			('acc_platform_fee', 'Platform Fee', 'REVENUE', 'USD'),
			('acc_merchant_payable', 'Merchant Payable', 'LIABILITY', 'USD')`,
		`CREATE TABLE payment_intents (
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
		)`,
		`CREATE INDEX idx_payment_intents_merchant_id ON payment_intents(merchant_id)`,
		`CREATE INDEX idx_payment_intents_state ON payment_intents(state)`,
		`CREATE TABLE idempotency_records (
			id VARCHAR(255) PRIMARY KEY,
			merchant_id VARCHAR(255) NOT NULL,
			idempotency_key VARCHAR(255) NOT NULL,
			request_hash VARCHAR(64) NOT NULL,
			response_body TEXT,
			status_code INT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMP NOT NULL,
			UNIQUE (merchant_id, idempotency_key)
		)`,
		`CREATE TABLE outbox_events (
			id VARCHAR(255) PRIMARY KEY,
			aggregate_id VARCHAR(255) NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			published_at TIMESTAMP
		)`,
		`CREATE TABLE inbox_events (
			event_id VARCHAR(255) PRIMARY KEY,
			processed_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
	}

	ctx := context.Background()
	for i, migration := range migrations {
		_, err := tdb.DB.ExecContext(ctx, migration)
		if err != nil {
			t.Fatalf("Failed to apply migration %d: %v\nSQL: %s", i, err, migration)
		}
	}
}

func (tdb *TestDB) Truncate(t *testing.T, tables ...string) {
	t.Helper()

	ctx := context.Background()
	for _, table := range tables {
		_, err := tdb.DB.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Fatalf("Failed to truncate table %s: %v", table, err)
		}
	}
}
