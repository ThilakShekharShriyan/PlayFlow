//go:build integration
// +build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/thilakshekharshriyan/playflow/internal/ledger"
	"github.com/thilakshekharshriyan/playflow/internal/payments"
	"github.com/thilakshekharshriyan/playflow/internal/platform"
	"github.com/thilakshekharshriyan/playflow/internal/testutil"
)

func TestPaymentAndLedger_FullIntegration(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	paymentRepo := payments.NewPostgresRepository(testDB.DB)
	paymentSvc := payments.NewService(paymentRepo)

	ledgerRepo := ledger.NewPostgresRepository(testDB.DB)
	ledgerSvc := ledger.NewService(ledgerRepo)

	ctx := context.Background()

	t.Run("Complete Payment Flow With Ledger Postings", func(t *testing.T) {
		testDB.Truncate(t, "payment_intents", "ledger_entries", "transactions")

		createReq := payments.CreateIntentRequest{
			MerchantID:     "merchant_full_flow",
			Amount:         10000,
			Currency:       "USD",
			IdempotencyKey: "idem_full_flow",
		}
		intent, err := paymentSvc.CreateIntent(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create intent: %v", err)
		}

		authReq := payments.AuthorizeRequest{
			IntentID: intent.ID,
		}
		authorized, err := paymentSvc.AuthorizeIntent(ctx, authReq)
		if err != nil {
			t.Fatalf("Failed to authorize: %v", err)
		}

		authTxn := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Authorization hold for payment " + authorized.ID,
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_customer_cash", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_payable", Amount: 10000, Currency: "USD"},
			},
		}
		err = ledgerSvc.PostTransaction(ctx, authTxn)
		if err != nil {
			t.Fatalf("Failed to post auth ledger entry: %v", err)
		}

		captureReq := payments.CaptureRequest{
			IntentID: intent.ID,
			Amount:   10000,
		}
		captured, err := paymentSvc.CaptureIntent(ctx, captureReq)
		if err != nil {
			t.Fatalf("Failed to capture: %v", err)
		}

		captureTxn := ledger.PostTransactionRequest{
			TransactionID: platform.GenerateID("txn"),
			Description:   "Capture payment " + captured.ID,
			Entries: []ledger.EntryRequest{
				{AccountID: "acc_merchant_payable", Amount: -10000, Currency: "USD"},
				{AccountID: "acc_merchant_receivable", Amount: 9700, Currency: "USD"},
				{AccountID: "acc_platform_fee", Amount: 300, Currency: "USD"},
			},
		}
		err = ledgerSvc.PostTransaction(ctx, captureTxn)
		if err != nil {
			t.Fatalf("Failed to post capture ledger entry: %v", err)
		}

		merchantBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_merchant_receivable")
		if merchantBalance != 9700 {
			t.Errorf("Expected merchant balance 9700, got %d", merchantBalance)
		}

		feeBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_platform_fee")
		if feeBalance != 300 {
			t.Errorf("Expected fee balance 300, got %d", feeBalance)
		}

		customerBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_customer_cash")
		if customerBalance != -10000 {
			t.Errorf("Expected customer balance -10000, got %d", customerBalance)
		}

		payableBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_merchant_payable")
		if payableBalance != 0 {
			t.Errorf("Expected payable balance 0 after capture, got %d", payableBalance)
		}

		if captured.State != payments.StateCaptured {
			t.Errorf("Expected payment state CAPTURED, got %v", captured.State)
		}
	})

	t.Run("Concurrent Payments Maintain Ledger Consistency", func(t *testing.T) {
		testDB.Truncate(t, "payment_intents", "ledger_entries", "transactions")

		numPayments := 10
		done := make(chan error, numPayments)

		for i := 0; i < numPayments; i++ {
			go func() {
				createReq := payments.CreateIntentRequest{
					MerchantID: "merchant_concurrent",
					Amount:     1000,
					Currency:   "USD",
				}
				intent, err := paymentSvc.CreateIntent(ctx, createReq)
				if err != nil {
					done <- err
					return
				}

				authReq := payments.AuthorizeRequest{IntentID: intent.ID}
				_, err = paymentSvc.AuthorizeIntent(ctx, authReq)
				if err != nil {
					done <- err
					return
				}

				authTxn := ledger.PostTransactionRequest{
					TransactionID: platform.GenerateID("txn"),
					Description:   "Concurrent auth",
					Entries: []ledger.EntryRequest{
						{AccountID: "acc_customer_cash", Amount: -1000, Currency: "USD"},
						{AccountID: "acc_merchant_payable", Amount: 1000, Currency: "USD"},
					},
				}
				err = ledgerSvc.PostTransaction(ctx, authTxn)
				if err != nil {
					done <- err
					return
				}

				captureReq := payments.CaptureRequest{IntentID: intent.ID, Amount: 1000}
				_, err = paymentSvc.CaptureIntent(ctx, captureReq)
				if err != nil {
					done <- err
					return
				}

				captureTxn := ledger.PostTransactionRequest{
					TransactionID: platform.GenerateID("txn"),
					Description:   "Concurrent capture",
					Entries: []ledger.EntryRequest{
						{AccountID: "acc_merchant_payable", Amount: -1000, Currency: "USD"},
						{AccountID: "acc_merchant_receivable", Amount: 970, Currency: "USD"},
						{AccountID: "acc_platform_fee", Amount: 30, Currency: "USD"},
					},
				}
				err = ledgerSvc.PostTransaction(ctx, captureTxn)
				done <- err
			}()
		}

		for i := 0; i < numPayments; i++ {
			err := <-done
			if err != nil {
				t.Errorf("Payment %d failed: %v", i, err)
			}
		}

		merchantBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_merchant_receivable")
		feeBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_platform_fee")
		customerBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_customer_cash")
		payableBalance, _ := ledgerSvc.GetAccountBalance(ctx, "acc_merchant_payable")

		totalBalance := merchantBalance + feeBalance + customerBalance + payableBalance
		if totalBalance != 0 {
			t.Errorf("Total system balance should be 0 after concurrent operations, got %d", totalBalance)
		}

		expectedMerchant := int64(numPayments * 970)
		if merchantBalance != expectedMerchant {
			t.Errorf("Expected merchant balance %d, got %d", expectedMerchant, merchantBalance)
		}

		expectedFee := int64(numPayments * 30)
		if feeBalance != expectedFee {
			t.Errorf("Expected fee balance %d, got %d", expectedFee, feeBalance)
		}
	})
}
