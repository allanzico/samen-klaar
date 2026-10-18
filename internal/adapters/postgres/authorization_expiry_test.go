package postgres

import (
	"encoding/json"
	"testing"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
)

func TestAuthorizationExpiryPersistsAndPreventsCaptureClaim(t *testing.T) {
	ctx, store := accountTestStore(t)
	_, _, state := bookedCancellationFixture(t, ctx, store)
	state.Job.AppointmentConfirmation.Status = "confirmed"
	state.Job.WorkStarted, state.Job.CompletionApproved = true, true
	state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	record := paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion,
		AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Status: "authorized", AuthorizationID: "expired-authorization", AuthorizationExpiresAt: "2000-01-01T00:00:00Z",
		OrderRequestID: "expired-order-request", AuthRequestID: "expired-auth-request", CaptureRequestID: "expired-capture-request", VoidRequestID: "expired-void-request"}
	if created, err := store.CreatePaymentIntent(ctx, record); err != nil || !created {
		t.Fatalf("create authorization=%v err=%v", created, err)
	}
	loaded, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || loaded.AuthorizationExpiresAt != "2000-01-01T00:00:00Z" {
		t.Fatalf("saved expiry=%+v found=%v err=%v", loaded, found, err)
	}
	if claimed, err := store.ClaimPaymentIntentAttempt(ctx, loaded, "authorized", "capturing"); err != nil || claimed {
		t.Fatalf("expired authorization claimed capture=%v err=%v", claimed, err)
	}
	loaded.Status = "expired"
	if err := store.UpdatePaymentIntent(ctx, loaded, "authorized"); err != nil {
		t.Fatal(err)
	}
	loaded.Status, loaded.AuthorizationID, loaded.AuthorizationExpiresAt = "creating", "", ""
	loaded.OrderRequestID, loaded.AuthRequestID, loaded.CaptureRequestID, loaded.VoidRequestID = "fresh-order-request", "fresh-auth-request", "fresh-capture-request", "fresh-void-request"
	if created, err := store.RetryPaymentIntent(ctx, loaded); err != nil || !created {
		t.Fatalf("fresh explicit attempt=%v err=%v", created, err)
	}
	loaded, found, err = store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || loaded.AuthorizationExpiresAt != "" || loaded.AuthRequestID != "fresh-auth-request" {
		t.Fatalf("fresh attempt inherited old expiry or identity: %+v found=%v err=%v", loaded, found, err)
	}
}
