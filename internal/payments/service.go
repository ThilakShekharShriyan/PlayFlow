package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/thilakshekharshriyan/playflow/internal/platform"
)

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreateIntent(ctx context.Context, req CreateIntentRequest) (*PaymentIntent, error) {
	if req.MerchantID == "" {
		return nil, fmt.Errorf("merchant ID is required")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if req.Currency == "" {
		return nil, fmt.Errorf("currency is required")
	}

	if req.IdempotencyKey != "" {
		existing, err := s.repo.GetByIdempotencyKey(ctx, req.MerchantID, req.IdempotencyKey)
		if err != nil && err != ErrIntentNotFound {
			return nil, fmt.Errorf("failed to check idempotency: %w", err)
		}
		if existing != nil {
			return existing, nil
		}
	}

	intent := &PaymentIntent{
		ID:             platform.GenerateID("pi"),
		MerchantID:     req.MerchantID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		State:          StateCreated,
		IdempotencyKey: req.IdempotencyKey,
	}

	if err := s.repo.Create(ctx, intent); err != nil {
		return nil, fmt.Errorf("failed to create intent: %w", err)
	}

	return intent, nil
}

func (s *service) GetIntent(ctx context.Context, id string) (*PaymentIntent, error) {
	return s.repo.Get(ctx, id)
}

func (s *service) AuthorizeIntent(ctx context.Context, req AuthorizeRequest) (*PaymentIntent, error) {
	intent, err := s.repo.Get(ctx, req.IntentID)
	if err != nil {
		return nil, err
	}

	if err := ValidateTransition(intent.State, StateAuthorized); err != nil {
		return nil, err
	}

	providerPaymentID := fmt.Sprintf("psp_%s", uuid.New().String())

	if err := s.repo.UpdateStateWithProvider(ctx, intent.ID, StateAuthorized, "mock_provider", providerPaymentID, intent.Version); err != nil {
		return nil, fmt.Errorf("failed to authorize intent: %w", err)
	}

	return s.repo.Get(ctx, intent.ID)
}

func (s *service) CaptureIntent(ctx context.Context, req CaptureRequest) (*PaymentIntent, error) {
	intent, err := s.repo.Get(ctx, req.IntentID)
	if err != nil {
		return nil, err
	}

	if err := ValidateTransition(intent.State, StateCaptured); err != nil {
		return nil, err
	}

	if req.Amount > intent.Amount {
		return nil, fmt.Errorf("capture amount cannot exceed intent amount")
	}

	if err := s.repo.UpdateState(ctx, intent.ID, StateCaptured, intent.Version); err != nil {
		return nil, fmt.Errorf("failed to capture intent: %w", err)
	}

	return s.repo.Get(ctx, intent.ID)
}

func (s *service) RefundIntent(ctx context.Context, req RefundRequest) (*PaymentIntent, error) {
	intent, err := s.repo.Get(ctx, req.IntentID)
	if err != nil {
		return nil, err
	}

	if err := ValidateTransition(intent.State, StateRefunded); err != nil {
		return nil, err
	}

	if req.Amount > intent.Amount {
		return nil, fmt.Errorf("refund amount cannot exceed intent amount")
	}

	if err := s.repo.UpdateState(ctx, intent.ID, StateRefunded, intent.Version); err != nil {
		return nil, fmt.Errorf("failed to refund intent: %w", err)
	}

	return s.repo.Get(ctx, intent.ID)
}
