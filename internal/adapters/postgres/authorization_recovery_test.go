package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/riverqueue/river"
)

func bookingAuthorizationFixture(t *testing.T, ctx context.Context, store *Store, status string) (string, workflow.State, paypal.IntentRecord) {
	t.Helper()
	customer, _, state := bookedCancellationFixture(t, ctx, store)
	state.Job.PaymentAuthorizationTiming = workflow.PaymentAuthorizationAtBooking
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	record := paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion, AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Status: status, OrderRequestID: "order-request", AuthRequestID: "auth-request", CaptureRequestID: "capture-request", VoidRequestID: "void-request"}
	if created, err := store.CreatePaymentIntent(ctx, record); err != nil || !created {
		t.Fatalf("payment operation: %v, %v", created, err)
	}
	return customer, state, record
}

func TestTimingDeferralRejectsUnresolvedDurableAuthorization(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, status := range []string{"creating", "order_uncertain", "authorizing", "authorization_uncertain", "authorized", "voiding", "void_uncertain"} {
		t.Run(status, func(t *testing.T) {
			customer, state, _ := bookingAuthorizationFixture(t, ctx, store, status)
			if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "defer-payment-authorization"}); err == nil {
				t.Fatalf("timing changed with durable %s payment operation", status)
			}
			loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
			if err != nil || loaded.Job.PaymentAuthorizationTiming != workflow.PaymentAuthorizationAtBooking {
				t.Fatalf("payment timing changed: %+v, %v", loaded.Job, err)
			}
		})
	}
}

func TestCreatedOrderCanDeferButCannotAuthorizeUntilCompletion(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, state, record := bookingAuthorizationFixture(t, ctx, store, "created")
	deferred, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "defer-payment-authorization"})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.Job.PaymentAuthorizationTiming != workflow.PaymentAuthorizationAfterCompletion {
		t.Fatalf("timing = %q", deferred.Job.PaymentAuthorizationTiming)
	}
	stored, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || stored.Status != "deferred" {
		t.Fatalf("abandoned order = %+v, %v", stored, err)
	}
	if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "created", "authorizing"); err != nil || claimed {
		t.Fatalf("stale order authorized after deferral: %v, %v", claimed, err)
	}
	record.Status = "creating"
	record.OrderRequestID = "replacement-order-request"
	if created, err := store.RetryPaymentIntent(ctx, record); err == nil || created {
		t.Fatalf("deferred payment restarted before completion: %v, %v", created, err)
	}
	deferred.Job.WorkStarted = true
	deferred.Job.CompletionApproved = true
	deferred.Job.PaymentApprovedVersion = deferred.Job.ScopeVersion
	raw, err := json.Marshal(deferred)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	if created, err := store.RetryPaymentIntent(ctx, record); err != nil || !created {
		t.Fatalf("deferred payment could not restart at completion: %v, %v", created, err)
	}
}

func TestPaymentCreationAndAuthorizationRejectStaleBookingState(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, operation := range []string{"create", "authorize"} {
		t.Run(operation, func(t *testing.T) {
			customer, state, record := bookingAuthorizationFixture(t, ctx, store, "created")
			if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "defer-payment-authorization"}); err != nil {
				t.Fatal(err)
			}
			if operation == "create" {
				// A stale checkout retained its old in-memory state while the customer
				// selected the after-completion timing in another request.
				record.Status = "creating"
				if created, err := store.CreatePaymentIntent(ctx, record); err == nil || created {
					t.Fatalf("stale checkout created an intent: %v, %v", created, err)
				}
			} else {
				// The status predicate alone is insufficient if an earlier request had
				// already prepared authorization before the timing switch committed.
				if _, err := store.pool.Exec(ctx, `UPDATE paypal_payment_operations SET status='created' WHERE job_id=$1`, state.Job.ID); err != nil {
					t.Fatal(err)
				}
				if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "created", "authorizing"); err == nil || claimed {
					t.Fatalf("stale checkout authorized: %v, %v", claimed, err)
				}
			}
		})
	}
}

func TestCancellationReleaseSurvivesWorkerFailureAndRestart(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, state, _ := bookingAuthorizationFixture(t, ctx, store, "authorized")
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "cancel-request"})
	if err != nil || !cancelled.Cancelled || cancelled.Job.PaymentAuthorizedVersion != 0 {
		t.Fatalf("cancellation = %+v, %v", cancelled, err)
	}
	record, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || record.Status != "release_pending" {
		t.Fatalf("release was not durably reserved: %+v, %v", record, err)
	}
	// The cancelled workspace must stop even an already approved capture from
	// acquiring the payment while its durable release is waiting for a worker.
	if claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "authorized", "capturing"); err != nil || claimed {
		t.Fatalf("cancelled authorization captured: %v, %v", claimed, err)
	}
	first, err := Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	first.SetAuthorizationReleaser(&releaseFixture{store: first, calls: make(chan paypal.IntentRecord, 20), fail: true})
	firstCalls := first.releaser.(*releaseFixture).calls
	select {
	case attempt := <-firstCalls:
		if attempt.JobID != state.Job.ID || attempt.Status != "authorized" || attempt.OrderRequestID != "order-request" || attempt.AuthRequestID != "auth-request" || attempt.VoidRequestID != "void-request" {
			t.Fatalf("release lost attempt identity: %+v", attempt)
		}
	case <-ctx.Done():
		t.Fatal("release never reached the provider after restart")
	}
	first.Close()
	second, err := Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.SetAuthorizationReleaser(&releaseFixture{store: second, calls: make(chan paypal.IntentRecord, 20)})
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		record, found, err = store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
		if err != nil {
			t.Fatal(err)
		}
		if found && record.Status == "voided" {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("durable release was not retried: %+v", record)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

type releaseFixture struct {
	store *Store
	calls chan paypal.IntentRecord
	fail  bool
}

func (f *releaseFixture) ReleaseAuthorization(ctx context.Context, attempt paypal.IntentRecord) error {
	select {
	case f.calls <- attempt:
	default:
	}
	if f.fail {
		return errors.New("temporary provider failure")
	}
	current, found, err := f.store.GetPaymentIntent(ctx, attempt.JobID, attempt.ScopeVersion)
	if err != nil || !found {
		return err
	}
	current.Status = "voided"
	return f.store.UpdatePaymentIntent(ctx, current, "release_pending", "voiding", "void_uncertain")
}

func TestExpiryDeclineAndOperatorCancellationQueueExactAttempt(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"expiry", "decline", "operator"} {
		t.Run(path, func(t *testing.T) {
			customer, state, record := bookingAuthorizationFixture(t, ctx, store, "authorized")
			state.Job.PaymentAuthorizedVersion = state.Job.ScopeVersion
			if path == "expiry" {
				state.Job.AppointmentConfirmation.DueAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
			}
			if path == "operator" {
				state.CancellationReview = &workflow.CancellationRequest{Status: "pending", RequestedBy: "customer"}
			}
			raw, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
				t.Fatal(err)
			}
			switch path {
			case "expiry":
				err = store.expirePendingAppointmentConfirmations(ctx)
			case "decline":
				_, err = store.ApplyWorkspace(ctx, state.Job.ProfessionalAccountID, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "decline-appointment"})
			case "operator":
				err = store.ResolveWorkCase(ctx, accountFixture(t, ctx, store), state.WorkspaceID, "late-cancellation", "approve-cancellation", "Customer and professional agreed to cancel")
			}
			if err != nil {
				t.Fatal(err)
			}
			var argsJSON []byte
			if err := store.pool.QueryRow(ctx, `SELECT args FROM river_job WHERE kind='paypal_authorization_release' AND args #>> '{attempt,JobID}'=$1 ORDER BY id DESC LIMIT 1`, state.Job.ID).Scan(&argsJSON); err != nil {
				t.Fatal(err)
			}
			var args AuthorizationReleaseArgs
			if err := json.Unmarshal(argsJSON, &args); err != nil {
				t.Fatal(err)
			}
			if args.Attempt != record {
				t.Fatalf("release queued a different payment attempt: %+v", args.Attempt)
			}
			loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Job.PaymentAuthorizedVersion != 0 {
				t.Fatal("released visit retained its authorization marker")
			}
		})
	}
}

func TestQueuedReleaseDoesNotReachProviderForReplacementAttempt(t *testing.T) {
	ctx, store := accountTestStore(t)
	_, state, old := bookingAuthorizationFixture(t, ctx, store, "voided")
	next := old
	next.Status = "creating"
	next.OrderRequestID = "next-order"
	next.AuthRequestID = "next-authorization"
	next.VoidRequestID = "next-void"
	if retried, err := store.RetryPaymentIntent(ctx, next); err != nil || !retried {
		t.Fatalf("replacement: %v, %v", retried, err)
	}
	provider := &releaseFixture{store: store, calls: make(chan paypal.IntentRecord, 1)}
	store.SetAuthorizationReleaser(provider)
	worker := AuthorizationReleaseWorker{store: store}
	if err := worker.Work(ctx, &river.Job[AuthorizationReleaseArgs]{Args: AuthorizationReleaseArgs{Attempt: old}}); err != nil {
		t.Fatal(err)
	}
	select {
	case attempt := <-provider.calls:
		t.Fatalf("old release reached provider for replacement: %+v", attempt)
	default:
	}
	current, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || current.Status != "creating" || current.OrderRequestID != "next-order" {
		t.Fatalf("replacement changed: %+v, %v", current, err)
	}
}

func TestExactAttemptClaimCannotClaimReplacementWithSameStatus(t *testing.T) {
	ctx, store := accountTestStore(t)
	_, state, old := bookingAuthorizationFixture(t, ctx, store, "voided")
	next := old
	next.Status = "creating"
	next.OrderRequestID = "replacement-order"
	next.AuthRequestID = "replacement-auth"
	next.VoidRequestID = "replacement-void"
	if retried, err := store.RetryPaymentIntent(ctx, next); err != nil || !retried {
		t.Fatalf("replacement: %v, %v", retried, err)
	}
	next.Status = "authorized"
	if err := store.UpdatePaymentIntent(ctx, next, "creating"); err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.ClaimPaymentIntentAttempt(ctx, old, "authorized", "voiding"); err != nil || claimed {
		t.Fatalf("old release claimed replacement authorization: %v, %v", claimed, err)
	}
	old.Status = "voided"
	if err := store.UpdatePaymentIntent(ctx, old, "authorized"); err == nil {
		t.Fatal("old release completion overwrote a replacement authorization")
	}
	current, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || current.Status != "authorized" || current.OrderRequestID != "replacement-order" {
		t.Fatalf("replacement changed: %+v, %v", current, err)
	}
}

func verifiedBookingAuthorizationFixture(t *testing.T, ctx context.Context, store *Store) (string, workflow.State, paypal.IntentRecord) {
	t.Helper()
	customer, state, record := bookingAuthorizationFixture(t, ctx, store, "authorized")
	id := "seller-" + rand.Text()
	if _, err := store.BeginPayPalSellerReferral(ctx, state.Job.ProfessionalAccountID, id, "referral-"+id); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePayPalSellerStatus(ctx, state.Job.ProfessionalAccountID, id, "verified", true, true, []string{"payments"}); err != nil {
		t.Fatal(err)
	}
	record.SellerMerchantID = id
	record.OrderID = "order-" + rand.Text()
	record.AuthorizationID = "auth-" + rand.Text()
	if err := store.UpdatePaymentIntent(ctx, record, "authorized"); err != nil {
		t.Fatal(err)
	}
	return customer, state, record
}

func TestAuthorizationProjectionChecksDurableAttemptBeforeUnlockingVisit(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, invalid := range []string{"unverified-boolean", "request-identity", "authorization-id", "scope", "status", "amount", "seller"} {
		t.Run(invalid, func(t *testing.T) {
			customer, state, record := verifiedBookingAuthorizationFixture(t, ctx, store)
			action := workflow.Action{Actor: "customer", Action: "record-payment-authorization", PayPalAuthorized: true, PaymentAuthorizationID: record.AuthorizationID, PaymentAuthorizationRequestID: record.AuthRequestID, PaymentAuthorizationScopeVersion: record.ScopeVersion}
			switch invalid {
			case "unverified-boolean":
				action.PaymentAuthorizationID = ""
				action.PaymentAuthorizationRequestID = ""
				action.PaymentAuthorizationScopeVersion = 0
			case "request-identity":
				action.PaymentAuthorizationRequestID = "old-attempt"
			case "authorization-id":
				action.PaymentAuthorizationID = "old-authorization"
			case "scope":
				action.PaymentAuthorizationScopeVersion++
			case "status":
				record.Status = "voided"
			case "amount":
				record.AmountMinor++
			case "seller":
				record.SellerMerchantID = "a-different-merchant"
			}
			if err := store.UpdatePaymentIntent(ctx, record, "authorized"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, action); err == nil {
				t.Fatal("invalid authorization unlocked the visit")
			}
			loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
			if err != nil || loaded.Job.PaymentAuthorizedVersion != 0 {
				t.Fatalf("authorization marker changed: %+v, %v", loaded.Job, err)
			}
		})
	}
	customer, state, record := verifiedBookingAuthorizationFixture(t, ctx, store)
	approved, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "record-payment-authorization", PayPalAuthorized: true, PaymentAuthorizationID: record.AuthorizationID, PaymentAuthorizationRequestID: record.AuthRequestID, PaymentAuthorizationScopeVersion: record.ScopeVersion})
	if err != nil || approved.Job.PaymentAuthorizedVersion != state.Job.ScopeVersion {
		t.Fatalf("verified authorization could not project: %+v, %v", approved.Job, err)
	}
	if _, err := store.ApplyWorkspace(ctx, state.Job.ProfessionalAccountID, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "confirm-appointment"}); err != nil {
		t.Fatal(err)
	}
}

func TestEarlyAuthorizationSurvivesBilateralSameScopeReschedule(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, state, record := verifiedBookingAuthorizationFixture(t, ctx, store)
	state.Job.PaymentAuthorizedVersion = state.Job.ScopeVersion
	state.Job.AppointmentConfirmation.Status = "confirmed"
	persistLifecycleState(t, ctx, store, state)
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
	proposed, err := store.ApplyWorkspace(ctx, state.Job.ProfessionalAccountID, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "propose-date", Value: "I can arrive the following day", ProposedAppointment: &workflow.Appointment{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(time.Hour).Format(time.RFC3339), Timezone: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "accept-date", ProposalID: proposed.Job.DateProposal.ID})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Job.PaymentAuthorizedVersion != state.Job.ScopeVersion || accepted.Job.PaymentAuthorizationTiming != workflow.PaymentAuthorizationAtBooking || accepted.Job.Appointment.StartUTC != start.Format(time.RFC3339) {
		t.Fatalf("reschedule lost authorization: %+v", accepted.Job)
	}
	current, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || current.Status != "authorized" || current.AuthRequestID != record.AuthRequestID {
		t.Fatalf("reschedule changed provider authorization: %+v, %v", current, err)
	}
}

func TestCancellationRollsBackWhenDurableReleaseCannotBeQueued(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, state, _ := bookingAuthorizationFixture(t, ctx, store, "authorized")
	unconfigured := &Store{pool: store.pool}
	_, err := unconfigured.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "cancel-request"})
	if err == nil {
		t.Fatal("cancellation committed without a durable release queue")
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || loaded.Cancelled || loaded.Job.Appointment == nil {
		t.Fatalf("failed cancellation changed workspace: %+v, %v", loaded, err)
	}
	record, found, err := store.GetPaymentIntent(ctx, state.Job.ID, state.Job.ScopeVersion)
	if err != nil || !found || record.Status != "authorized" {
		t.Fatalf("failed cancellation changed payment: %+v, %v", record, err)
	}
	slots, err := store.AvailableProfessionalSlots(ctx, state.Job.ProfessionalAccountID, "")
	if err != nil || len(slots) != 0 {
		t.Fatalf("failed cancellation released its reservation: %+v, %v", slots, err)
	}
}

func TestQueuedReleaseIgnoresMissingAndTerminalOperations(t *testing.T) {
	ctx, store := accountTestStore(t)
	provider := &releaseFixture{store: store, calls: make(chan paypal.IntentRecord, 10)}
	store.SetAuthorizationReleaser(provider)
	worker := AuthorizationReleaseWorker{store: store}
	for _, status := range []string{"missing", "voided", "denied", "expired", "deferred", "captured", "refunded", "reversed"} {
		t.Run(status, func(t *testing.T) {
			var attempt paypal.IntentRecord
			if status == "missing" {
				attempt = paypal.IntentRecord{WorkspaceID: "deleted-workspace", JobID: "deleted-job", ScopeVersion: 1}
			} else {
				_, _, attempt = bookingAuthorizationFixture(t, ctx, store, status)
			}
			if err := worker.Work(ctx, &river.Job[AuthorizationReleaseArgs]{Args: AuthorizationReleaseArgs{Attempt: attempt}}); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-provider.calls:
				t.Fatalf("unnecessary release reached provider: %+v", got)
			default:
			}
		})
	}
}
