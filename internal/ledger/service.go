package ledger

import (
	"context"
	"fmt"
)

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) PostTransaction(ctx context.Context, req PostTransactionRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("invalid transaction: %w", err)
	}

	return s.repo.PostTransaction(ctx, req)
}

func (s *service) GetTransaction(ctx context.Context, id string) (*Transaction, []*LedgerEntry, error) {
	txn, err := s.repo.GetTransaction(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	entries, err := s.repo.GetEntriesByTransaction(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	return txn, entries, nil
}

func (s *service) GetAccountBalance(ctx context.Context, accountID string) (int64, error) {
	entries, err := s.repo.GetEntriesByAccount(ctx, accountID, 1000000)
	if err != nil {
		return 0, err
	}

	var balance int64
	for _, entry := range entries {
		balance += entry.Amount
	}

	return balance, nil
}
