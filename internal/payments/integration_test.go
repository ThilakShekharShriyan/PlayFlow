//go:build integration
// +build integration

package payments_test

import (
	"context"
	"testing"

	"github.com/thilakshekharshriyan/playflow/internal/payments"
	"github.com/thilakshekharshriyan/playflow/internal/testutil"
)

func TestPaymentFlow_EndToEnd(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := payments.NewPostgresRepository(testDB.DB)
	svc := payments.NewService(repo)
	ctx := context.Background()

	t.Run("Complete Happy Path: Create -> Authorize -> Capture", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			MerchantID:     "merchant_123",
			Amount:         10000,
			Currency:       "USD",
			IdempotencyKey: "idem_create_1",
		}

		intent, err := svc.CreateIntent(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create intent: %v", err)
		}

		if intent.State != payments.StateCreated {
			t.Errorf("Expected state CREATED, got %v", intent.State)
		}
		if intent.Amount != 10000 {
			t.Errorf("Expected amount 10000, got %d", intent.Amount)
		}
		if intent.Version != 0 {
			t.Errorf("Expected version 0, got %d", intent.Version)
		}

		authReq := payments.AuthorizeRequest{
			IntentID:       intent.ID,
			IdempotencyKey: "idem_auth_1",
		}
		authorized, err := svc.AuthorizeIntent(ctx, authReq)
		if err != nil {
			t.Fatalf("Failed to authorize intent: %v", err)
		}

		if authorized.State != payments.StateAuthorized {
			t.Errorf("Expected state AUTHORIZED, got %v", authorized.State)
		}
		if authorized.Version != 1 {
			t.Errorf("Expected version 1, got %d", authorized.Version)
		}
		if authorized.SelectedProvider == "" {
			t.Error("Expected provider to be set")
		}
		if authorized.ProviderPaymentID == "" {
			t.Error("Expected provider payment ID to be set")
		}

		captureReq := payments.CaptureRequest{
			IntentID:       intent.ID,
			Amount:         10000,
			IdempotencyKey: "idem_capture_1",
		}
		captured, err := svc.CaptureIntent(ctx, captureReq)
		if err != nil {
			t.Fatalf("Failed to capture intent: %v", err)
		}

		if captured.State != payments.StateCaptured {
			t.Errorf("Expected state CAPTURED, got %v", captured.State)
		}
		if captured.Version != 2 {
			t.Errorf("Expected version 2, got %d", captured.Version)
		}
	})

	t.Run("Refund Flow: Captured -> Refunded", func(t *testing.T) {
		testDB.Truncate(t, "payment_intents")

		createReq := payments.CreateIntentRequest{
			MerchantID:     "merchant_456",
			Amount:         5000,
			Currency:       "USD",
			IdempotencyKey: "idem_create_2",
		}
		intent, _ := svc.CreateIntent(ctx, createReq)

		authReq := payments.AuthorizeRequest{
			IntentID:       intent.ID,
			IdempotencyKey: "idem_auth_2",
		}
		svc.AuthorizeIntent(ctx, authReq)

		captureReq := payments.CaptureRequest{
			IntentID:       intent.ID,
			Amount:         5000,
			IdempotencyKey: "idem_capture_2",
		}
		svc.CaptureIntent(ctx, captureReq)

		refundReq := payments.RefundRequest{
			IntentID:       intent.ID,
			Amount:         5000,
			Reason:         "customer_request",
			IdempotencyKey: "idem_refund_1",
		}
		refunded, err := svc.RefundIntent(ctx, refundReq)
		if err != nil {
			t.Fatalf("Failed to refund intent: %v", err)
		}

		if refunded.State != payments.StateRefunded {
			t.Errorf("Expected state REFUNDED, got %v", refunded.State)
		}
		if refunded.Version != 3 {
			t.Errorf("Expected version 3, got %d", refunded.Version)
		}
	})

	t.Run("Invalid Transitions Are Rejected", func(t *testing.T) {
		testDB.Truncate(t, "payment_intents")

		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_789",
			Amount:     1000,
			Currency:   "USD",
		}
		intent, _ := svc.CreateIntent(ctx, createReq)

		captureReq := payments.CaptureRequest{
			IntentID: intent.ID,
			Amount:   1000,
		}
		_, err := svc.CaptureIntent(ctx, captureReq)
		if err == nil {
			t.Error("Expected error when capturing from CREATED state")
		}
		if err != payments.ErrInvalidTransition {
			t.Errorf("Expected ErrInvalidTransition, got %v", err)
		}

		authReq := payments.AuthorizeRequest{
			IntentID: intent.ID,
		}
		authorized, _ := svc.AuthorizeIntent(ctx, authReq)

		refundReq := payments.RefundRequest{
			IntentID: authorized.ID,
			Amount:   1000,
			Reason:   "test",
		}
		_, err = svc.RefundIntent(ctx, refundReq)
		if err == nil {
			t.Error("Expected error when refunding from AUTHORIZED state")
		}
		if err != payments.ErrInvalidTransition {
			t.Errorf("Expected ErrInvalidTransition, got %v", err)
		}
	})

	t.Run("Partial Capture", func(t *testing.T) {
		testDB.Truncate(t, "payment_intents")

		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_partial",
			Amount:     10000,
			Currency:   "USD",
		}
		intent, _ := svc.CreateIntent(ctx, createReq)

		authReq := payments.AuthorizeRequest{
			IntentID: intent.ID,
		}
		svc.AuthorizeIntent(ctx, authReq)

		captureReq := payments.CaptureRequest{
			IntentID: intent.ID,
			Amount:   7500,
		}
		captured, err := svc.CaptureIntent(ctx, captureReq)
		if err != nil {
			t.Fatalf("Failed to partially capture: %v", err)
		}

		if captured.State != payments.StateCaptured {
			t.Errorf("Expected state CAPTURED, got %v", captured.State)
		}
	})

	t.Run("Cannot Capture More Than Authorized", func(t *testing.T) {
		testDB.Truncate(t, "payment_intents")

		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_overcapture",
			Amount:     5000,
			Currency:   "USD",
		}
		intent, _ := svc.CreateIntent(ctx, createReq)

		authReq := payments.AuthorizeRequest{
			IntentID: intent.ID,
		}
		svc.AuthorizeIntent(ctx, authReq)

		captureReq := payments.CaptureRequest{
			IntentID: intent.ID,
			Amount:   6000,
		}
		_, err := svc.CaptureIntent(ctx, captureReq)
		if err == nil {
			t.Error("Expected error when capturing more than authorized amount")
		}
	})
}

func TestPaymentFlow_Idempotency(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := payments.NewPostgresRepository(testDB.DB)
	svc := payments.NewService(repo)
	ctx := context.Background()

	t.Run("Duplicate Create Request Returns Same Intent", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			MerchantID:     "merchant_idem",
			Amount:         10000,
			Currency:       "USD",
			IdempotencyKey: "idem_unique_123",
		}

		intent1, err := svc.CreateIntent(ctx, createReq)
		if err != nil {
			t.Fatalf("First create failed: %v", err)
		}

		intent2, err := svc.CreateIntent(ctx, createReq)
		if err != nil {
			t.Fatalf("Second create failed: %v", err)
		}

		if intent1.ID != intent2.ID {
			t.Errorf("Expected same intent ID, got %s and %s", intent1.ID, intent2.ID)
		}
		if intent1.Version != intent2.Version {
			t.Errorf("Expected same version, got %d and %d", intent1.Version, intent2.Version)
		}
	})

	t.Run("Same Idempotency Key Different Merchant Creates New Intent", func(t *testing.T) {
		createReq1 := payments.CreateIntentRequest{
			MerchantID:     "merchant_A",
			Amount:         10000,
			Currency:       "USD",
			IdempotencyKey: "shared_key_456",
		}
		intent1, _ := svc.CreateIntent(ctx, createReq1)

		createReq2 := payments.CreateIntentRequest{
			MerchantID:     "merchant_B",
			Amount:         10000,
			Currency:       "USD",
			IdempotencyKey: "shared_key_456",
		}
		intent2, _ := svc.CreateIntent(ctx, createReq2)

		if intent1.ID == intent2.ID {
			t.Error("Expected different intents for different merchants")
		}
	})
}

func TestPaymentFlow_ConcurrentOperations(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := payments.NewPostgresRepository(testDB.DB)
	svc := payments.NewService(repo)
	ctx := context.Background()

	t.Run("Concurrent Authorize Attempts - Only One Succeeds", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_concurrent",
			Amount:     10000,
			Currency:   "USD",
		}
		intent, _ := svc.CreateIntent(ctx, createReq)

		results := make(chan error, 5)
		for i := 0; i < 5; i++ {
			go func() {
				authReq := payments.AuthorizeRequest{
					IntentID: intent.ID,
				}
				_, err := svc.AuthorizeIntent(ctx, authReq)
				results <- err
			}()
		}

		successCount := 0
		errorCount := 0
		for i := 0; i < 5; i++ {
			err := <-results
			if err == nil {
				successCount++
			} else {
				// Either version mismatch or invalid transition (already authorized)
				errorCount++
			}
		}

		if successCount != 1 {
			t.Errorf("Expected exactly 1 success, got %d", successCount)
		}
		if errorCount != 4 {
			t.Errorf("Expected 4 errors, got %d", errorCount)
		}

		finalIntent, _ := svc.GetIntent(ctx, intent.ID)
		if finalIntent.State != payments.StateAuthorized {
			t.Errorf("Expected state AUTHORIZED, got %v", finalIntent.State)
		}
		if finalIntent.Version != 1 {
			t.Errorf("Expected version 1, got %d", finalIntent.Version)
		}
	})
}

func TestPaymentFlow_ValidationRules(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close(t)
	testDB.ApplyMigrations(t)

	repo := payments.NewPostgresRepository(testDB.DB)
	svc := payments.NewService(repo)
	ctx := context.Background()

	t.Run("Cannot Create Intent With Zero Amount", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_validation",
			Amount:     0,
			Currency:   "USD",
		}
		_, err := svc.CreateIntent(ctx, createReq)
		if err == nil {
			t.Error("Expected error for zero amount")
		}
	})

	t.Run("Cannot Create Intent With Negative Amount", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_validation",
			Amount:     -1000,
			Currency:   "USD",
		}
		_, err := svc.CreateIntent(ctx, createReq)
		if err == nil {
			t.Error("Expected error for negative amount")
		}
	})

	t.Run("Cannot Create Intent Without Merchant ID", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			Amount:   1000,
			Currency: "USD",
		}
		_, err := svc.CreateIntent(ctx, createReq)
		if err == nil {
			t.Error("Expected error for missing merchant ID")
		}
	})

	t.Run("Cannot Create Intent Without Currency", func(t *testing.T) {
		createReq := payments.CreateIntentRequest{
			MerchantID: "merchant_validation",
			Amount:     1000,
		}
		_, err := svc.CreateIntent(ctx, createReq)
		if err == nil {
			t.Error("Expected error for missing currency")
		}
	})
}
