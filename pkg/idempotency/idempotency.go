package idempotency

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrAlreadyProcessed = errors.New("request already processed")
)

type Record struct {
	ID             string
	MerchantID     string
	IdempotencyKey string
	RequestHash    string
	ResponseBody   string
	StatusCode     int
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type Store interface {
	Get(ctx context.Context, merchantID, key string) (*Record, error)
	Store(ctx context.Context, record *Record) error
}

type postgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) Store {
	return &postgresStore{db: db}
}

func (s *postgresStore) Get(ctx context.Context, merchantID, key string) (*Record, error) {
	query := `
		SELECT id, merchant_id, idempotency_key, request_hash, response_body, 
		       status_code, created_at, expires_at
		FROM idempotency_records
		WHERE merchant_id = $1 AND idempotency_key = $2 AND expires_at > NOW()
	`
	record := &Record{}
	var responseBody sql.NullString

	err := s.db.QueryRowContext(ctx, query, merchantID, key).Scan(
		&record.ID,
		&record.MerchantID,
		&record.IdempotencyKey,
		&record.RequestHash,
		&responseBody,
		&record.StatusCode,
		&record.CreatedAt,
		&record.ExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get idempotency record: %w", err)
	}

	if responseBody.Valid {
		record.ResponseBody = responseBody.String
	}

	return record, nil
}

func (s *postgresStore) Store(ctx context.Context, record *Record) error {
	query := `
		INSERT INTO idempotency_records (
			id, merchant_id, idempotency_key, request_hash,
			response_body, status_code, created_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (merchant_id, idempotency_key) DO NOTHING
	`
	_, err := s.db.ExecContext(ctx, query,
		record.ID,
		record.MerchantID,
		record.IdempotencyKey,
		record.RequestHash,
		record.ResponseBody,
		record.StatusCode,
		record.CreatedAt,
		record.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to store idempotency record: %w", err)
	}

	return nil
}

func HashRequest(body interface{}) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

type Middleware struct {
	store Store
	ttl   time.Duration
}

func NewMiddleware(store Store, ttl time.Duration) *Middleware {
	return &Middleware{
		store: store,
		ttl:   ttl,
	}
}

func (m *Middleware) CheckIdempotency(ctx context.Context, merchantID, key, requestHash string) (*Record, error) {
	if key == "" {
		return nil, nil
	}

	record, err := m.store.Get(ctx, merchantID, key)
	if err != nil {
		return nil, err
	}

	if record != nil {
		if record.RequestHash != requestHash {
			return nil, fmt.Errorf("idempotency key reused with different request body")
		}
		return record, ErrAlreadyProcessed
	}

	return nil, nil
}

func (m *Middleware) SaveResponse(ctx context.Context, merchantID, key, requestHash, responseBody string, statusCode int) error {
	if key == "" {
		return nil
	}

	record := &Record{
		ID:             fmt.Sprintf("idem_%d", time.Now().UnixNano()),
		MerchantID:     merchantID,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		ResponseBody:   responseBody,
		StatusCode:     statusCode,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(m.ttl),
	}

	return m.store.Store(ctx, record)
}
