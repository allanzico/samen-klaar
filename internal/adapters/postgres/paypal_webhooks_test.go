package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestCaptureWebhookIsDeduplicatedAndReconcilesPaymentState(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	jobID := "webhook-" + rand.Text()
	state.Job = workflow.Job{
		ID: jobID, Title: "Sink repair", ScopeVersion: 1, CompletionApproved: true,
		PaymentApprovedVersion: 1, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 1234, DurationMinutes: 60},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, stateJSON); err != nil {
		t.Fatal(err)
	}
	orderID := "order-" + rand.Text()
	created, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{
		WorkspaceID: state.WorkspaceID, JobID: jobID, ScopeVersion: 1, AmountMinor: 1234, Currency: "EUR",
		OrderID: orderID, Status: "capturing", OrderRequestID: "order-request", AuthRequestID: "authorize-request",
		CaptureRequestID: "capture-request", VoidRequestID: "void-request",
	})
	if err != nil || !created {
		t.Fatalf("create payment operation = %v, %v", created, err)
	}
	event := paypal.WebhookEvent{ID: "event-" + rand.Text(), Type: "PAYMENT.CAPTURE.COMPLETED", ResourceID: "capture-1", CaptureID: "capture-1", OrderID: orderID, Status: "COMPLETED", Currency: "EUR", AmountMinor: 1234}
	if err := store.EnqueuePayPalWebhook(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueuePayPalWebhook(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := store.processPayPalWebhook(ctx, event.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Job.Paid || loaded.Job.PaymentTransactionID != "capture-1" {
		t.Fatalf("capture webhook did not mark payment paid: %+v", loaded.Job)
	}
	var operationStatus, captureID, eventStatus string
	if err := store.pool.QueryRow(ctx, `SELECT status,capture_id FROM paypal_payment_operations WHERE job_id=$1`, jobID).Scan(&operationStatus, &captureID); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "captured" || captureID != "capture-1" {
		t.Fatalf("payment operation = %s, %s", operationStatus, captureID)
	}
	if err := store.pool.QueryRow(ctx, `SELECT status FROM paypal_webhook_events WHERE event_id=$1`, event.ID).Scan(&eventStatus); err != nil {
		t.Fatal(err)
	}
	if eventStatus != "processed" {
		t.Fatalf("webhook event status = %s", eventStatus)
	}
	beforeRepeat, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{
		Action: "confirm-payment", Actor: "customer", PayPalCaptured: true, PaymentCaptureID: "capture-1",
	}); err != nil {
		t.Fatalf("browser return after completed webhook was not idempotent: %v", err)
	}
	afterRepeat, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || afterRepeat.Job.Timeline[len(afterRepeat.Job.Timeline)-1].Text != beforeRepeat.Job.Timeline[len(beforeRepeat.Job.Timeline)-1].Text {
		t.Fatalf("idempotent browser return changed payment timeline: before=%+v after=%+v err=%v", beforeRepeat.Job.Timeline, afterRepeat.Job.Timeline, err)
	}
	var eventCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM paypal_webhook_events WHERE event_id=$1`, event.ID).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("duplicate webhook rows=%d, query error=%v", eventCount, err)
	}
	var queuedNotifications int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND event_type='confirm-payment'`, jobID).Scan(&queuedNotifications); err != nil || queuedNotifications != 2 {
		t.Fatalf("capture notifications=%d, query error=%v", queuedNotifications, err)
	}
}

func TestMatchingCapturedOperationRepairsUnpaidWorkspace(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, _, state := bookedCancellationFixture(t, ctx, store)
	state.Job.CompletionApproved = true
	state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, data); err != nil {
		t.Fatal(err)
	}
	orderID, captureID := "order-"+rand.Text(), "capture-"+rand.Text()
	created, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion,
		AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, OrderID: orderID, CaptureID: captureID,
		Status: "captured", OrderRequestID: "order-request", AuthRequestID: "auth-request", CaptureRequestID: "capture-request", VoidRequestID: "void-request"})
	if err != nil || !created {
		t.Fatalf("payment operation: %v, %v", created, err)
	}
	for i := 0; i < 2; i++ {
		event := paypal.WebhookEvent{ID: "event-" + rand.Text(), Type: "PAYMENT.CAPTURE.COMPLETED", CaptureID: captureID, OrderID: orderID,
			Status: "COMPLETED", Currency: state.Job.Quote.Currency, AmountMinor: state.Job.Quote.TotalMinor}
		if err := store.EnqueuePayPalWebhook(ctx, event); err != nil {
			t.Fatal(err)
		}
		if err := store.processPayPalWebhook(ctx, event.ID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if _, err := store.pool.Exec(ctx, `UPDATE booking_reservations SET status='booked' WHERE job_id=$1`, state.Job.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || !loaded.Job.Paid || loaded.Job.PaymentTransactionID != captureID {
		t.Fatalf("workspace not reconciled: %+v, %v", loaded.Job, err)
	}
	var reservation string
	if err := store.pool.QueryRow(ctx, `SELECT status FROM booking_reservations WHERE job_id=$1`, state.Job.ID).Scan(&reservation); err != nil || reservation != "completed" {
		t.Fatalf("reservation=%q, %v", reservation, err)
	}
	var events int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE job_id=$1 AND action='capture-completed'`, state.Job.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("capture audit events=%d, %v", events, err)
	}
}

func TestPartialRefundWebhookDoesNotClaimFullRefund(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	jobID := "partial-refund-" + rand.Text()
	state.Job = workflow.Job{
		ID: jobID, Title: "Sink repair", ScopeVersion: 1, Paid: true, PaymentTransactionID: "capture-partial",
		Quote: workflow.Quote{Currency: "EUR", TotalMinor: 1234, DurationMinutes: 60},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, stateJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{
		WorkspaceID: state.WorkspaceID, JobID: jobID, ScopeVersion: 1, AmountMinor: 1234, Currency: "EUR",
		CaptureID: "capture-partial", Status: "captured", OrderRequestID: "order", AuthRequestID: "authorize",
		CaptureRequestID: "capture", VoidRequestID: "void",
	}); err != nil {
		t.Fatal(err)
	}
	partial := paypal.WebhookEvent{ID: "partial-event-" + rand.Text(), Type: "PAYMENT.CAPTURE.REFUNDED", CaptureID: "capture-partial", Status: "PARTIALLY_REFUNDED"}
	insertWebhookFixture(t, ctx, store, partial)
	if err := store.processPayPalWebhook(ctx, partial.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Job.Paid || loaded.Job.PaymentRefunded || loaded.Job.Dispute == nil || loaded.Job.Dispute.Status != "partial_refund_review" {
		t.Fatalf("partial refund was reported as fully refunded: %+v", loaded.Job)
	}
	if _, err := store.BeginWorkRefund(ctx, "operator", state.WorkspaceID, "Review current PayPal refund status"); err == nil {
		t.Fatal("partial refund case allowed a second automated full-refund attempt")
	}
	full := paypal.WebhookEvent{ID: "full-event-" + rand.Text(), Type: "PAYMENT.CAPTURE.REFUNDED", CaptureID: "capture-partial", Status: "REFUNDED"}
	insertWebhookFixture(t, ctx, store, full)
	if err := store.processPayPalWebhook(ctx, full.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || !loaded.Job.PaymentRefunded || loaded.Job.Dispute == nil || loaded.Job.Dispute.Status != "refunded" {
		t.Fatalf("full refund event not reconciled: job=%+v err=%v", loaded.Job, err)
	}
}

func insertWebhookFixture(t *testing.T, ctx context.Context, store *Store, event paypal.WebhookEvent) {
	t.Helper()
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO paypal_webhook_events (event_id,event_type,event_json,status) VALUES ($1,$2,$3,'queued')`, event.ID, event.Type, eventJSON); err != nil {
		t.Fatal(err)
	}
}

func TestRefundOperationRoundTripsRequestIdentity(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	jobID := fmt.Sprintf("refund-record-%s", rand.Text())
	state.Job = workflow.Job{ID: jobID, ScopeVersion: 1, Quote: workflow.Quote{TotalMinor: 1000, Currency: "EUR"}}
	persistLifecycleState(t, ctx, store, state)
	intent := paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: jobID, ScopeVersion: 1, AmountMinor: 1000, Currency: "EUR", Status: "captured", CaptureID: "capture-refund", OrderRequestID: "order-1", AuthRequestID: "auth-1", CaptureRequestID: "capture-1", VoidRequestID: "void-1", RefundRequestID: "refund-request-stable"}
	if _, err := store.CreatePaymentIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	intent.Status = "refunded"
	intent.RefundID = "refund-1"
	if err := store.UpdatePaymentIntent(ctx, intent, "captured"); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := store.GetPaymentIntent(ctx, jobID, 1)
	if err != nil || !found || loaded.Status != "refunded" || loaded.RefundID != "refund-1" || loaded.RefundRequestID != "refund-request-stable" {
		t.Fatalf("loaded payment intent = %+v, found=%v, err=%v", loaded, found, err)
	}
}
