package payments

import (
	"testing"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from PaymentState
		to   PaymentState
		want bool
	}{
		{"CREATED to AUTHORIZED", StateCreated, StateAuthorized, true},
		{"CREATED to FAILED", StateCreated, StateFailed, true},
		{"AUTHORIZED to CAPTURED", StateAuthorized, StateCaptured, true},
		{"AUTHORIZED to FAILED", StateAuthorized, StateFailed, true},
		{"CAPTURED to REFUNDED", StateCaptured, StateRefunded, true},
		{"CREATED to CAPTURED", StateCreated, StateCaptured, false},
		{"CAPTURED to CREATED", StateCaptured, StateCreated, false},
		{"REFUNDED to CAPTURED", StateRefunded, StateCaptured, false},
		{"FAILED to AUTHORIZED", StateFailed, StateAuthorized, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("CanTransition(%v, %v) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    PaymentState
		to      PaymentState
		wantErr bool
	}{
		{"valid CREATED to AUTHORIZED", StateCreated, StateAuthorized, false},
		{"valid AUTHORIZED to CAPTURED", StateAuthorized, StateCaptured, false},
		{"invalid CREATED to CAPTURED", StateCreated, StateCaptured, true},
		{"invalid CAPTURED to CREATED", StateCaptured, StateCreated, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != ErrInvalidTransition {
				t.Errorf("ValidateTransition() error = %v, want %v", err, ErrInvalidTransition)
			}
		})
	}
}
