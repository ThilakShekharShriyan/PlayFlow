package ledger

import (
	"testing"
)

func TestPostTransactionRequest_IsBalanced(t *testing.T) {
	tests := []struct {
		name    string
		entries []EntryRequest
		want    bool
	}{
		{
			name: "balanced transaction",
			entries: []EntryRequest{
				{AccountID: "acc_1", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_2", Amount: 9700, Currency: "USD"},
				{AccountID: "acc_3", Amount: 300, Currency: "USD"},
			},
			want: true,
		},
		{
			name: "unbalanced transaction",
			entries: []EntryRequest{
				{AccountID: "acc_1", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_2", Amount: 5000, Currency: "USD"},
			},
			want: false,
		},
		{
			name: "simple balanced",
			entries: []EntryRequest{
				{AccountID: "acc_1", Amount: 100, Currency: "USD"},
				{AccountID: "acc_2", Amount: -100, Currency: "USD"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := PostTransactionRequest{
				TransactionID: "txn_123",
				Description:   "Test transaction",
				Entries:       tt.entries,
			}
			if got := req.IsBalanced(); got != tt.want {
				t.Errorf("IsBalanced() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostTransactionRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     PostTransactionRequest
		wantErr bool
	}{
		{
			name: "valid transaction",
			req: PostTransactionRequest{
				TransactionID: "txn_123",
				Description:   "Test",
				Entries: []EntryRequest{
					{AccountID: "acc_1", Amount: 100, Currency: "USD"},
					{AccountID: "acc_2", Amount: -100, Currency: "USD"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing transaction ID",
			req: PostTransactionRequest{
				Description: "Test",
				Entries: []EntryRequest{
					{AccountID: "acc_1", Amount: 100, Currency: "USD"},
					{AccountID: "acc_2", Amount: -100, Currency: "USD"},
				},
			},
			wantErr: true,
		},
		{
			name: "unbalanced",
			req: PostTransactionRequest{
				TransactionID: "txn_123",
				Description:   "Test",
				Entries: []EntryRequest{
					{AccountID: "acc_1", Amount: 100, Currency: "USD"},
					{AccountID: "acc_2", Amount: -50, Currency: "USD"},
				},
			},
			wantErr: true,
		},
		{
			name: "insufficient entries",
			req: PostTransactionRequest{
				TransactionID: "txn_123",
				Description:   "Test",
				Entries: []EntryRequest{
					{AccountID: "acc_1", Amount: 0, Currency: "USD"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
