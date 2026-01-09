//go:build integration
// +build integration

package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thilakshekharshriyan/playflow/internal/ledger"
	"github.com/thilakshekharshriyan/playflow/internal/platform"
	"github.com/thilakshekharshriyan/playflow/internal/testutil"
)

func TestLedger_EndToEnd(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := ledger.NewPostgresRepository(testDB.DB)
	svc := ledger.NewService(repo)
	ctx := context.Background()

	t.Run("Post Balanced Transaction", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Customer payment of $100",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 9700, Currency: "USD"},
				{AccountID: "acc_platform_fee", Amount: 300, Currency: "USD"},
			},
		}

		err := svc.PostTransaction(ctx, txnReq)
		if err != nil {
			t.Fatalf("Failed to post transaction: %v", err)
		}

		txn, entries, err := svc.GetTransaction(ctx, txnReq.TransactionID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}

		if txn.Description != "Customer payment of $100" {
			t.Errorf("Expected description 'Customer payment of $100', got %s", txn.Description)
		}

		if len(entries) != 3 {
			t.Fatalf("Expected 3 entries, got %d", len(entries))
		}

		var sum int64
		for _, entry := range entries {
			sum += entry.Amount
		}
		if sum != 0 {
			t.Errorf("Transaction not balanced: sum = %d", sum)
		}
	})

	t.Run("Reject Unbalanced Transaction", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Unbalanced transaction",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 5000, Currency: "USD"},
			},
		}

		err := svc.PostTransaction(ctx, txnReq)
		if err == nil {
			t.Error("Expected error for unbalanced transaction")
		}
		if !errors.Is(err, ledger.ErrUnbalancedTransaction) {
			t.Errorf("Expected ErrUnbalancedTransaction, got %v", err)
		}
	})

	t.Run("Calculate Account Balance", func(t *testing.T) {
		testDB.Truncate(t, "ledger_entries", "transactions")

		txn1 := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Payment 1",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 10000, Currency: "USD"},
			},
		}
		svc.PostTransaction(ctx, txn1)

		txn2 := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Payment 2",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -5000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 5000, Currency: "USD"},
			},
		}
		svc.PostTransaction(ctx, txn2)

		balance, err := svc.GetAccountBalance(ctx, "acc_merchant_receivable")
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}

		if balance != 15000 {
			t.Errorf("Expected balance 15000, got %d", balance)
		}

		customerBalance, err := svc.GetAccountBalance(ctx, "acc_customer_cash")
		if err != nil {
			t.Fatalf("Failed to get customer balance: %v", err)
		}

		if customerBalance != -15000 {
			t.Errorf("Expected customer balance -15000, got %d", customerBalance)
		}
	})

	t.Run("Multiple Transactions Maintain Invariant", func(t *testing.T) {
		testDB.Truncate(t, "ledger_entries", "transactions")

		for i := 0; i < 10; i++ {
			txnReq := ledger.PostTransactionRequest{
				TransactionID: platform.GenerateID("txn"),
				Description:   "Test transaction",
				Entries: []ledger.EntryRequest{
					{AccountID: "acc_customer_cash", Amount: -1000, Currency: "USD"},
					{AccountID: "acc_merchant_receivable", Amount: 970, Currency: "USD"},
					{AccountID: "acc_platform_fee", Amount: 30, Currency: "USD"},
				},
			}
			err := svc.PostTransaction(ctx, txnReq)
			if err != nil {
				t.Fatalf("Transaction %d failed: %v", i, err)
			}
		}

		merchantBalance, _ := svc.GetAccountBalance(ctx, "acc_merchant_receivable")
		feeBalance, _ := svc.GetAccountBalance(ctx, "acc_platform_fee")
		customerBalance, _ := svc.GetAccountBalance(ctx, "acc_customer_cash")

		totalBalance := merchantBalance + feeBalance + customerBalance
		if totalBalance != 0 {
			t.Errorf("Total balance should be 0, got %d", totalBalance)
		}

		if merchantBalance != 9700 {
			t.Errorf("Expected merchant balance 9700, got %d", merchantBalance)
		}
		if feeBalance != 300 {
			t.Errorf("Expected fee balance 300, got %d", feeBalance)
		}
		if customerBalance != -10000 {
			t.Errorf("Expected customer balance -10000, got %d", customerBalance)
		}
	})

	t.Run("Immutability - Transactions Cannot Be Modified", func(t *testing.T) {
		testDB.Truncate(t, "ledger_entries", "transactions")

		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Original transaction",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -1000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 1000, Currency: "USD"},
			},
		}
		svc.PostTransaction(ctx, txnReq)

		_, originalEntries, _ := svc.GetTransaction(ctx, txnReq.TransactionID)

		txnReq.Description = "Modified description"
		err := svc.PostTransaction(ctx, txnReq)
		if err == nil {
			t.Error("Expected error when posting duplicate transaction ID")
		}

		_, currentEntries, _ := svc.GetTransaction(ctx, txnReq.TransactionID)

		if len(originalEntries) != len(currentEntries) {
			t.Error("Transaction was modified")
		}
	})
}

func TestLedger_ComplexScenarios(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := ledger.NewPostgresRepository(testDB.DB)
	svc := ledger.NewService(repo)
	ctx := context.Background()

	t.Run("Payment Authorization - Reserve Funds", func(t *testing.T) {
		testDB.Truncate(t, "ledger_entries", "transactions")

		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Authorization hold",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_payable", Amount: 10000, Currency: "USD"},
			},
		}
		err := svc.PostTransaction(ctx, txnReq)
		if err != nil {
			t.Fatalf("Failed to post authorization: %v", err)
		}

		balance, _ := svc.GetAccountBalance(ctx, "acc_merchant_payable")
		if balance != 10000 {
			t.Errorf("Expected payable balance 10000, got %d", balance)
		}
	})

	t.Run("Payment Capture - Transfer Funds", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Capture payment with fee",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_merchant_payable", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 9700, Currency: "USD"},
				{AccountID: "acc_platform_fee", Amount: 300, Currency: "USD"},
			},
		}
		err := svc.PostTransaction(ctx, txnReq)
		if err != nil {
			t.Fatalf("Failed to post capture: %v", err)
		}

		payableBalance, _ := svc.GetAccountBalance(ctx, "acc_merchant_payable")
		receivableBalance, _ := svc.GetAccountBalance(ctx, "acc_merchant_receivable")
		feeBalance, _ := svc.GetAccountBalance(ctx, "acc_platform_fee")

		if payableBalance != 0 {
			t.Errorf("Expected payable balance 0, got %d", payableBalance)
		}
		if receivableBalance != 9700 {
			t.Errorf("Expected receivable balance 9700, got %d", receivableBalance)
		}
		if feeBalance != 300 {
			t.Errorf("Expected fee balance 300, got %d", feeBalance)
		}
	})

	t.Run("Refund Transaction", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Refund to customer",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_merchant_receivable", Amount: -9700, Currency: "USD"},
				{AccountID: "acc_platform_fee", Amount: -300, Currency: "USD"},
				{AccountID: "acc_customer_cash", Amount: 10000, Currency: "USD"},
			},
		}
		err := svc.PostTransaction(ctx, txnReq)
		if err != nil {
			t.Fatalf("Failed to post refund: %v", err)
		}

		customerBalance, _ := svc.GetAccountBalance(ctx, "acc_customer_cash")
		if customerBalance != 0 {
			t.Errorf("Expected customer balance 0 after refund, got %d", customerBalance)
		}
	})
}

func TestLedger_ValidationRules(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := ledger.NewPostgresRepository(testDB.DB)
	svc := ledger.NewService(repo)
	ctx := context.Background()

	t.Run("Reject Transaction With Less Than Two Entries", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Single entry transaction",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -1000, Currency: "USD"},
			},
		}
		err := svc.PostTransaction(ctx, txnReq)
		if err == nil {
			t.Error("Expected error for single-entry transaction")
		}
	})

	t.Run("Reject Transaction With Zero Amount Entry", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Zero amount entry",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: 0, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 0, Currency: "USD"},
			},
		}
		err := svc.PostTransaction(ctx, txnReq)
		if err == nil {
			t.Error("Expected error for zero amount entry")
		}
		if !errors.Is(err, ledger.ErrInvalidAmount) {
			t.Errorf("Expected ErrInvalidAmount, got %v", err)
		}
	})

	t.Run("Reject Transaction Without Currency", func(t *testing.T) {
		txnReq := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Missing currency",
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -1000, Currency: ""},
				{AccountID: "acc_merchant_receivable", Amount: 1000, Currency: ""},
			},
		}
		err := svc.PostTransaction(ctx, txnReq)
		if err == nil {
			t.Error("Expected error for missing currency")
		}
		if !errors.Is(err, ledger.ErrInvalidCurrency) {
			t.Errorf("Expected ErrInvalidCurrency, got %v", err)
		}
	})
}
