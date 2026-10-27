package postgres

import (
	"encoding/json"
	"testing"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestPlatformFeeSnapshotSurvivesPersistenceAndClaims(t *testing.T) {
	ctx, store := accountTestStore(t)
	_, professional, state := bookedCancellationFixture(t, ctx, store)
	state.Job.PlatformFeeBasisPoints = 500
	state.Job.Quote.TotalMinor = 10500
	state.Job.CompletionApproved = true
	state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
	raw, _ := json.Marshal(state)
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	record := paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion, AmountMinor: 10500, Currency: state.Job.Quote.Currency, Status: "authorized", AuthorizationID: "auth", OrderID: "fee-order", OrderRequestID: "order", AuthRequestID: "auth", CaptureRequestID: "capture", VoidRequestID: "void"}
	if created, err := store.CreatePaymentIntent(ctx, record); err == nil || created {
		t.Fatal("payment with missing fee accepted")
	}
	record.PlatformFeeMinor = 525
	if created, err := store.CreatePaymentIntent(ctx, record); err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	loaded, found, err := store.GetPaymentIntent(ctx, record.JobID, record.ScopeVersion)
	if err != nil || !found || loaded.PlatformFeeMinor != 525 {
		t.Fatalf("fee not persisted: %+v %v", loaded, err)
	}
	forged := loaded
	forged.PlatformFeeMinor++
	if err := store.UpdatePaymentIntent(ctx, forged, "authorized"); err == nil {
		t.Fatal("fee could be changed during retry")
	}
	if claimed, err := store.ClaimPaymentIntentAttempt(ctx, forged, "authorized", "capturing"); err != nil || claimed {
		t.Fatal("forged fee snapshot claimed original attempt")
	}
	if claimed, err := store.ClaimPaymentIntentAttempt(ctx, loaded, "authorized", "capturing"); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	awaiting, err := store.ProfessionalDashboard(ctx, professional)
	if err != nil || len(awaiting.Balances) != 1 || awaiting.Balances[0].AwaitingPaymentMinor != 9975 {
		t.Fatalf("awaiting proceeds: %+v %v", awaiting, err)
	}
	event := paypal.WebhookEvent{Type: "PAYMENT.CAPTURE.COMPLETED", OrderID: "fee-order", CaptureID: "fee-capture", AmountMinor: 10500, Currency: record.Currency, Status: "COMPLETED"}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := store.applyCaptureCompleted(ctx, tx, event)
	tx.Rollback(ctx)
	if err != nil || accepted {
		t.Fatal("webhook without fee settlement was trusted")
	}
	event.PlatformFeeMinor = 525
	event.PlatformFeeReported = true
	tx, err = store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err = store.applyCaptureCompleted(ctx, tx, event)
	if err != nil || !accepted {
		tx.Rollback(ctx)
		t.Fatalf("verified settlement rejected: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	stats, err := store.ProfessionalDashboard(ctx, professional)
	if err != nil || len(stats.Balances) != 1 || stats.Balances[0].ReceivedMinor != 9975 || stats.Balances[0].PlatformFeesMinor != 525 {
		t.Fatalf("net earnings mismatch: %+v %v", stats, err)
	}
	// The old schema default is zero; no historical deduction is invented.
	if fee, err := workflow.PlatformFeeAmount(10500, 0); err != nil || fee != 0 {
		t.Fatal("legacy agreement charged")
	}
}

func TestFeePermissionUpgradePreservesExistingPayoutAccess(t *testing.T) {
	ctx, store := accountTestStore(t)
	_, professional, _ := bookedCancellationFixture(t, ctx, store)
	if _, err := store.BeginPayPalSellerReferral(ctx, professional, "tracking-upgrade", "original-request"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePayPalSellerStatus(ctx, professional, "seller", "verified", true, true, []string{"payments"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePayPalSellerReferral(ctx, professional, "tracking-upgrade", "https://api-m.sandbox.paypal.com/v2/customer/partner-referrals/upgrade"); err != nil {
		t.Fatal(err)
	}
	seller, found, err := store.GetPayPalSeller(ctx, professional)
	if err != nil || !found || seller.Status != "verified" || seller.MerchantID != "seller" || !seller.PaymentsReceivable || !seller.PrimaryEmailConfirmed {
		t.Fatalf("upgrade revoked old access: %+v %v", seller, err)
	}
}
