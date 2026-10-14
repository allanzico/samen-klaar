package postgres

import (
	"encoding/json"
	"testing"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestCaptureClaimRechecksDisputeAndConflictsWithNewIssue(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, issueFirst := range []bool{true, false} {
		customer, _, state := bookedCancellationFixture(t, ctx, store)
		state.Job.AppointmentConfirmation.Status = "confirmed"
		state.Job.WorkStarted = true
		state.Job.CompletionApproved = true
		state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, data); err != nil {
			t.Fatal(err)
		}
		created, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion,
			AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Status: "authorized", AuthorizationID: "auth-test",
			OrderRequestID: "order-request", AuthRequestID: "auth-request", CaptureRequestID: "capture-request", VoidRequestID: "void-request"})
		if err != nil || !created {
			t.Fatalf("payment operation: %v, %v", created, err)
		}
		issue := workflow.Action{Actor: "customer", Action: "report-issue", Value: "The valve is still leaking"}
		if issueFirst {
			if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, issue); err != nil {
				t.Fatal(err)
			}
			if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "authorized", "capturing"); err == nil || claimed {
				t.Fatalf("capture claimed after dispute: %v, %v", claimed, err)
			}
		} else {
			if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "authorized", "capturing"); err != nil || !claimed {
				t.Fatalf("capture claim: %v, %v", claimed, err)
			}
			if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, issue); err == nil {
				t.Fatal("issue promised a pause after capture started")
			}
		}
	}
}

func TestScopeChangeRejectsUnresolvedPaymentOperations(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, status := range []string{"authorizing", "authorization_uncertain", "capturing", "capture_uncertain", "captured", "void_uncertain", "refunding"} {
		t.Run(status, func(t *testing.T) {
			customer, _, state := bookedCancellationFixture(t, ctx, store)
			state.Job.AppointmentConfirmation.Status = "confirmed"
			state.Job.Change = &workflow.ScopeChange{Status: "pending", Description: "Replace the damaged valve", AmountMinor: 2500, ProposedFrom: state.Job.ScopeVersion}
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, data); err != nil {
				t.Fatal(err)
			}
			created, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion,
				AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Status: status,
				OrderRequestID: "order-request", AuthRequestID: "auth-request", CaptureRequestID: "capture-request", VoidRequestID: "void-request"})
			if err != nil || !created {
				t.Fatalf("payment operation: %v, %v", created, err)
			}
			if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "approve-change"}); err == nil {
				t.Fatal("scope changed with unresolved payment")
			}
			loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
			if err != nil || loaded.Job.ScopeVersion != 1 || loaded.Job.Quote.TotalMinor != 10000 {
				t.Fatalf("scope changed: %+v, %v", loaded.Job.Quote, err)
			}
		})
	}
}

func TestInterruptedRefundClaimCanBeRecoveredAfterLease(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, _, state := bookedCancellationFixture(t, ctx, store)
	_ = customer
	created, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion,
		AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Status: "refunding", RefundRequestID: "stable-refund-id",
		OrderRequestID: "order-request", AuthRequestID: "auth-request", CaptureRequestID: "capture-request", VoidRequestID: "void-request"})
	if err != nil || !created {
		t.Fatalf("payment operation: %v, %v", created, err)
	}
	if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "refunding", "refunding"); err != nil || claimed {
		t.Fatalf("fresh refund claim was stolen: %v, %v", claimed, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE paypal_payment_operations SET updated_at=now()-interval '3 minutes' WHERE job_id=$1`, state.Job.ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "refunding", "refunding"); err != nil || !claimed {
		t.Fatalf("stale refund claim not recovered: %v, %v", claimed, err)
	}
	record, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || record.RefundRequestID != "stable-refund-id" {
		t.Fatalf("refund identity changed: %+v, %v", record, err)
	}
}
