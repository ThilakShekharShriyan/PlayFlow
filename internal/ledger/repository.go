package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type postgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) CreateAccount(ctx context.Context, account *Account) error {
	query := `
		INSERT INTO accounts (id, name, type, currency, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query,
		account.ID,
		account.Name,
		account.Type,
		account.Currency,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}
	account.CreatedAt = now
	account.UpdatedAt = now
	return nil
}

func (r *postgresRepository) GetAccount(ctx context.Context, id string) (*Account, error) {
	query := `
		SELECT id, name, type, currency, created_at, updated_at
		FROM accounts
		WHERE id = $1
	`
	account := &Account{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&account.ID,
		&account.Name,
		&account.Type,
		&account.Currency,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	return account, nil
}

func (r *postgresRepository) ListAccounts(ctx context.Context) ([]*Account, error) {
	query := `
		SELECT id, name, type, currency, created_at, updated_at
		FROM accounts
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		account := &Account{}
		err := rows.Scan(
			&account.ID,
			&account.Name,
			&account.Type,
			&account.Currency,
			&account.CreatedAt,
			&account.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (r *postgresRepository) PostTransaction(ctx context.Context, req PostTransactionRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	insertTxQuery := `
		INSERT INTO transactions (id, description, created_at)
		VALUES ($1, $2, $3)
	`
	_, err = tx.ExecContext(ctx, insertTxQuery, req.TransactionID, req.Description, now)
	if err != nil {
		return fmt.Errorf("failed to insert transaction: %w", err)
	}

	insertEntryQuery := `
		INSERT INTO ledger_entries (id, transaction_id, entry_index, account_id, amount, currency, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	for i, entry := range req.Entries {
		entryID := uuid.New().String()
		_, err = tx.ExecContext(ctx, insertEntryQuery,
			entryID,
			req.TransactionID,
			i,
			entry.AccountID,
			entry.Amount,
			entry.Currency,
			now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert ledger entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *postgresRepository) GetTransaction(ctx context.Context, id string) (*Transaction, error) {
	query := `
		SELECT id, description, created_at
		FROM transactions
		WHERE id = $1
	`
	txn := &Transaction{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&txn.ID,
		&txn.Description,
		&txn.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrTransactionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	return txn, nil
}

func (r *postgresRepository) GetEntriesByTransaction(ctx context.Context, transactionID string) ([]*LedgerEntry, error) {
	query := `
		SELECT id, transaction_id, entry_index, account_id, amount, currency, created_at
		FROM ledger_entries
		WHERE transaction_id = $1
		ORDER BY entry_index
	`
	rows, err := r.db.QueryContext(ctx, query, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entries: %w", err)
	}
	defer rows.Close()

	var entries []*LedgerEntry
	for rows.Next() {
		entry := &LedgerEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.TransactionID,
			&entry.EntryIndex,
			&entry.AccountID,
			&entry.Amount,
			&entry.Currency,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (r *postgresRepository) GetEntriesByAccount(ctx context.Context, accountID string, limit int) ([]*LedgerEntry, error) {
	query := `
		SELECT id, transaction_id, entry_index, account_id, amount, currency, created_at
		FROM ledger_entries
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get entries: %w", err)
	}
	defer rows.Close()

	var entries []*LedgerEntry
	for rows.Next() {
		entry := &LedgerEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.TransactionID,
			&entry.EntryIndex,
			&entry.AccountID,
			&entry.Amount,
			&entry.Currency,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
