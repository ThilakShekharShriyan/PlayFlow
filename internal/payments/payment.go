package payments

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidTransition    = errors.New("invalid state transition")
	ErrInvalidState         = errors.New("invalid state")
	ErrVersionMismatch      = errors.New("version mismatch - concurrent modification detected")
	ErrIntentNotFound       = errors.New("payment intent not found")
	ErrIdempotencyKeyExists = errors.New("idempotency key already exists")
)

type PaymentState string

const (
	StateCreated    PaymentState = "CREATED"
	StateAuthorized PaymentState = "AUTHORIZED"
	StateCaptured   PaymentState = "CAPTURED"
	StateFailed     PaymentState = "FAILED"
	StateRefunded   PaymentState = "REFUNDED"
)

type PaymentIntent struct {
	ID                string
	MerchantID        string
	Amount            int64
	Currency          string
	State             PaymentState
	Version           int64
	IdempotencyKey    string
	SelectedProvider  string
	ProviderPaymentID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateIntentRequest struct {
	MerchantID     string
	Amount         int64
	Currency       string
	IdempotencyKey string
}

type AuthorizeRequest struct {
	IntentID       string
	IdempotencyKey string
}

type CaptureRequest struct {
	IntentID       string
	Amount         int64
	IdempotencyKey string
}

type RefundRequest struct {
	IntentID       string
	Amount         int64
	Reason         string
	IdempotencyKey string
}

type StateTransition struct {
	From PaymentState
	To   PaymentState
}

var allowedTransitions = map[StateTransition]bool{
	{From: StateCreated, To: StateAuthorized}:  true,
	{From: StateCreated, To: StateFailed}:      true,
	{From: StateAuthorized, To: StateCaptured}: true,
	{From: StateAuthorized, To: StateFailed}:   true,
	{From: StateCaptured, To: StateRefunded}:   true,
}

func CanTransition(from, to PaymentState) bool {
	return allowedTransitions[StateTransition{From: from, To: to}]
}

func ValidateTransition(from, to PaymentState) error {
	if !CanTransition(from, to) {
		return ErrInvalidTransition
	}
	return nil
}

type Repository interface {
	Create(ctx context.Context, intent *PaymentIntent) error
	Get(ctx context.Context, id string) (*PaymentIntent, error)
	GetByIdempotencyKey(ctx context.Context, merchantID, key string) (*PaymentIntent, error)
	UpdateState(ctx context.Context, id string, state PaymentState, expectedVersion int64) error
	UpdateStateWithProvider(ctx context.Context, id string, state PaymentState, provider, providerPaymentID string, expectedVersion int64) error
	List(ctx context.Context, merchantID string, limit int) ([]*PaymentIntent, error)
}

type Service interface {
	CreateIntent(ctx context.Context, req CreateIntentRequest) (*PaymentIntent, error)
	GetIntent(ctx context.Context, id string) (*PaymentIntent, error)
	AuthorizeIntent(ctx context.Context, req AuthorizeRequest) (*PaymentIntent, error)
	CaptureIntent(ctx context.Context, req CaptureRequest) (*PaymentIntent, error)
	RefundIntent(ctx context.Context, req RefundRequest) (*PaymentIntent, error)
}
