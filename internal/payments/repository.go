package payments

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type postgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, intent *PaymentIntent) error {
	query := `
		INSERT INTO payment_intents (
			id, merchant_id, amount, currency, state, version, 
			idempotency_key, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query,
		intent.ID,
		intent.MerchantID,
		intent.Amount,
		intent.Currency,
		intent.State,
		0,
		intent.IdempotencyKey,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to create payment intent: %w", err)
	}
	intent.Version = 0
	intent.CreatedAt = now
	intent.UpdatedAt = now
	return nil
}

func (r *postgresRepository) Get(ctx context.Context, id string) (*PaymentIntent, error) {
	query := `
		SELECT id, merchant_id, amount, currency, state, version,
			   idempotency_key, selected_provider, provider_payment_id,
			   created_at, updated_at
		FROM payment_intents
		WHERE id = $1
	`
	intent := &PaymentIntent{}
	var idempotencyKey, selectedProvider, providerPaymentID sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&intent.ID,
		&intent.MerchantID,
		&intent.Amount,
		&intent.Currency,
		&intent.State,
		&intent.Version,
		&idempotencyKey,
		&selectedProvider,
		&providerPaymentID,
		&intent.CreatedAt,
		&intent.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrIntentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment intent: %w", err)
	}

	if idempotencyKey.Valid {
		intent.IdempotencyKey = idempotencyKey.String
	}
	if selectedProvider.Valid {
		intent.SelectedProvider = selectedProvider.String
	}
	if providerPaymentID.Valid {
		intent.ProviderPaymentID = providerPaymentID.String
	}

	return intent, nil
}

func (r *postgresRepository) GetByIdempotencyKey(ctx context.Context, merchantID, key string) (*PaymentIntent, error) {
	query := `
		SELECT id, merchant_id, amount, currency, state, version,
			   idempotency_key, selected_provider, provider_payment_id,
			   created_at, updated_at
		FROM payment_intents
		WHERE merchant_id = $1 AND idempotency_key = $2
	`
	intent := &PaymentIntent{}
	var idempotencyKey, selectedProvider, providerPaymentID sql.NullString

	err := r.db.QueryRowContext(ctx, query, merchantID, key).Scan(
		&intent.ID,
		&intent.MerchantID,
		&intent.Amount,
		&intent.Currency,
		&intent.State,
		&intent.Version,
		&idempotencyKey,
		&selectedProvider,
		&providerPaymentID,
		&intent.CreatedAt,
		&intent.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrIntentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment intent by idempotency key: %w", err)
	}

	if idempotencyKey.Valid {
		intent.IdempotencyKey = idempotencyKey.String
	}
	if selectedProvider.Valid {
		intent.SelectedProvider = selectedProvider.String
	}
	if providerPaymentID.Valid {
		intent.ProviderPaymentID = providerPaymentID.String
	}

	return intent, nil
}

func (r *postgresRepository) UpdateState(ctx context.Context, id string, state PaymentState, expectedVersion int64) error {
	query := `
		UPDATE payment_intents
		SET state = $1, version = version + 1, updated_at = $2
		WHERE id = $3 AND version = $4
	`
	result, err := r.db.ExecContext(ctx, query, state, time.Now(), id, expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to update payment intent state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrVersionMismatch
	}

	return nil
}

func (r *postgresRepository) UpdateStateWithProvider(ctx context.Context, id string, state PaymentState, provider, providerPaymentID string, expectedVersion int64) error {
	query := `
		UPDATE payment_intents
		SET state = $1, version = version + 1, updated_at = $2,
		    selected_provider = $3, provider_payment_id = $4
		WHERE id = $5 AND version = $6
	`
	result, err := r.db.ExecContext(ctx, query, state, time.Now(), provider, providerPaymentID, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to update payment intent state with provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrVersionMismatch
	}

	return nil
}

func (r *postgresRepository) List(ctx context.Context, merchantID string, limit int) ([]*PaymentIntent, error) {
	query := `
		SELECT id, merchant_id, amount, currency, state, version,
			   idempotency_key, selected_provider, provider_payment_id,
			   created_at, updated_at
		FROM payment_intents
		WHERE merchant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, merchantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list payment intents: %w", err)
	}
	defer rows.Close()

	var intents []*PaymentIntent
	for rows.Next() {
		intent := &PaymentIntent{}
		var idempotencyKey, selectedProvider, providerPaymentID sql.NullString

		err := rows.Scan(
			&intent.ID,
			&intent.MerchantID,
			&intent.Amount,
			&intent.Currency,
			&intent.State,
			&intent.Version,
			&idempotencyKey,
			&selectedProvider,
			&providerPaymentID,
			&intent.CreatedAt,
			&intent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment intent: %w", err)
		}

		if idempotencyKey.Valid {
			intent.IdempotencyKey = idempotencyKey.String
		}
		if selectedProvider.Valid {
			intent.SelectedProvider = selectedProvider.String
		}
		if providerPaymentID.Valid {
			intent.ProviderPaymentID = providerPaymentID.String
		}

		intents = append(intents, intent)
	}

	return intents, nil
}
