package ledger

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnbalancedTransaction = errors.New("transaction does not balance to zero")
	ErrInvalidAmount         = errors.New("invalid amount")
	ErrInvalidCurrency       = errors.New("invalid currency")
	ErrAccountNotFound       = errors.New("account not found")
	ErrTransactionNotFound   = errors.New("transaction not found")
)

type AccountType string

const (
	AccountTypeAsset     AccountType = "ASSET"
	AccountTypeLiability AccountType = "LIABILITY"
	AccountTypeRevenue   AccountType = "REVENUE"
	AccountTypeExpense   AccountType = "EXPENSE"
)

type Account struct {
	ID        string
	Name      string
	Type      AccountType
	Currency  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Transaction struct {
	ID          string
	Description string
	CreatedAt   time.Time
}

type LedgerEntry struct {
	ID            string
	TransactionID string
	EntryIndex    int
	AccountID     string
	Amount        int64
	Currency      string
	CreatedAt     time.Time
}

type PostTransactionRequest struct {
	TransactionID string
	Description   string
	Entries       []EntryRequest
}

type EntryRequest struct {
	AccountID string
	Amount    int64
	Currency  string
}

func (req PostTransactionRequest) IsBalanced() bool {
	var sum int64
	for _, entry := range req.Entries {
		sum += entry.Amount
	}
	return sum == 0
}

func (req PostTransactionRequest) Validate() error {
	if req.TransactionID == "" {
		return errors.New("transaction ID is required")
	}
	if req.Description == "" {
		return errors.New("description is required")
	}
	if len(req.Entries) < 2 {
		return errors.New("at least two entries required for double-entry")
	}
	if !req.IsBalanced() {
		return ErrUnbalancedTransaction
	}
	for _, entry := range req.Entries {
		if entry.AccountID == "" {
			return errors.New("account ID is required")
		}
		if entry.Amount == 0 {
			return ErrInvalidAmount
		}
		if entry.Currency == "" {
			return ErrInvalidCurrency
		}
	}
	return nil
}

type Repository interface {
	CreateAccount(ctx context.Context, account *Account) error
	GetAccount(ctx context.Context, id string) (*Account, error)
	ListAccounts(ctx context.Context) ([]*Account, error)
	PostTransaction(ctx context.Context, req PostTransactionRequest) error
	GetTransaction(ctx context.Context, id string) (*Transaction, error)
	GetEntriesByTransaction(ctx context.Context, transactionID string) ([]*LedgerEntry, error)
	GetEntriesByAccount(ctx context.Context, accountID string, limit int) ([]*LedgerEntry, error)
}

type Service interface {
	PostTransaction(ctx context.Context, req PostTransactionRequest) error
	GetTransaction(ctx context.Context, id string) (*Transaction, []*LedgerEntry, error)
	GetAccountBalance(ctx context.Context, accountID string) (int64, error)
}
