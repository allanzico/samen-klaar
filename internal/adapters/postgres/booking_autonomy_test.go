package postgres

import (
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestAutonomousCaptureClaimRechecksConsentAndAgentPause(t *testing.T) {
	for _, change := range []string{"valid", "revoked", "paused", "inactive", "new-evidence", "unclear"} {
		t.Run(change, func(t *testing.T) {
			ctx, store := accountTestStore(t)
			customer, _, state := bookedCancellationFixture(t, ctx, store)
			state.Job.AppointmentConfirmation.Status = "confirmed"
			state.Job.WorkStarted = true
			state.Job.EvidenceSubmitted = true
			state.Job.DetailAdded = true
			state.Job.CompletionApproved = true
			state.Job.CompletionApprovedByAgent = true
			state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
			state.Job.PaymentApprovedByAgent = true
			state.Job.BookingAutonomy = &workflow.BookingAutonomy{Mode: "automatic", Version: 1, ScopeVersion: state.Job.ScopeVersion, AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, ProfessionalAccountID: state.Job.ProfessionalAccountID, ProfessionalProfileID: state.Job.ProfessionalProfileID, DecidedAt: time.Now().UTC().Format(time.RFC3339)}
			state.Job.Evidence = []workflow.Evidence{{AssetID: "photo", ContentType: "image/png", Kind: "photo"}}
			state.Job.EvidenceReview = &workflow.EvidenceReview{Summary: "The agreed repair is documented.", ReviewedAt: time.Now().UTC().Format(time.RFC3339), Coverage: []workflow.EvidenceFinding{{ScopeItem: "work-1", Coverage: "documented", EvidenceIDs: []string{"photo"}, Observation: "The repaired connection is visible."}}}
			workflow.BindEvidenceReview(state.Job, state.Job.EvidenceReview)
			persistLifecycleState(t, ctx, store, state)
			if ok, err := store.CreatePaymentIntent(ctx, paypal.IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion, AmountMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Status: "authorized", AuthorizationID: "auth-fixture", OrderRequestID: "order", AuthRequestID: "auth", CaptureRequestID: "capture", VoidRequestID: "void"}); err != nil || !ok {
				t.Fatalf("intent=%v, %v", ok, err)
			}
			switch change {
			case "revoked":
				if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
					t.Fatal(err)
				}
			case "paused":
				if _, err := store.SaveAgentMandate(ctx, customer, "customer", workflow.AgentMandate{Enabled: false}); err != nil {
					t.Fatal(err)
				}
			case "inactive":
				if _, err := store.pool.Exec(ctx, `UPDATE account_memberships SET status='disabled' WHERE account_id=$1 AND role='customer'`, customer); err != nil {
					t.Fatal(err)
				}
			case "new-evidence":
				state.Job.Evidence = append(state.Job.Evidence, workflow.Evidence{AssetID: "new", ContentType: "image/png"})
				persistLifecycleState(t, ctx, store, state)
			case "unclear":
				state.Job.EvidenceReview.Coverage[0].Coverage = "unclear"
				persistLifecycleState(t, ctx, store, state)
			}
			claimed, err := store.ClaimPaymentIntent(ctx, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion, "authorized", "capturing")
			if change == "valid" {
				if err != nil || !claimed {
					t.Fatalf("valid release=%v, %v", claimed, err)
				}
				if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err == nil {
					t.Fatal("revocation falsely promised to stop an already claimed payment")
				}
			} else if err == nil || claimed {
				t.Fatalf("capture survived %s: %v, %v", change, claimed, err)
			}
		})
	}
}

func TestBookingAutonomyConsentIsAuditedAndSurvivesReload(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	state.Job = workflow.Job{ID: state.WorkspaceID + "-job", ProfessionalAccountID: "fixture-professional", ProfessionalProfileID: "fixture-service", ScopeVersion: 1, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 12500}}
	persistLifecycleState(t, ctx, store, state)
	state, err = store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "set-booking-autonomy", Value: "automatic|" + workflow.BookingAutonomyKey(state.Job)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || loaded.Job.BookingAutonomy.Mode != "manual" {
		t.Fatalf("consent did not survive reload: %v", err)
	}
	var versions, automatic int
	if err := store.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE mode='automatic' AND actor_account_id=$2 AND amount_minor=12500 AND currency='EUR') FROM booking_autonomy_consents WHERE workspace_id=$1`, state.WorkspaceID, customer).Scan(&versions, &automatic); err != nil || versions != 2 || automatic != 1 {
		t.Fatalf("consent audit=%d/%d, %v", versions, automatic, err)
	}
}
