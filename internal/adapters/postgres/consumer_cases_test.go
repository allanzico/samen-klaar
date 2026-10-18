package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func consumerPolicyTestStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("run make check-flow for policy persistence checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "policy_test_" + rand.Text()
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = quoted
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{pool: pool}
	t.Cleanup(func() {
		store.Close()
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+quoted+` CASCADE`)
		admin.Close()
	})
	if err := migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return ctx, store
}

func TestLateRescheduleReviewPreservesThenAtomicallyMovesVisit(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, initiator := range []string{"customer", "professional"} {
		for _, decision := range []string{"approve-reschedule", "keep-visit"} {
			t.Run(initiator+"/"+decision, func(t *testing.T) {
				customer, professional, state := bookedCancellationFixture(t, ctx, store, 23*time.Hour)
				if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "confirm-appointment"}); err != nil {
					t.Fatal(err)
				}
				oldSlot := state.Job.Appointment.SlotID
				start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
				proposed := workflow.Appointment{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(time.Hour).Format(time.RFC3339), Timezone: "UTC"}
				actor, other, otherRole := customer, professional, "professional"
				if initiator == "professional" {
					actor, other, otherRole = professional, customer, "customer"
				}
				state, err := store.ApplyWorkspace(ctx, actor, initiator, state.WorkspaceID, workflow.Action{Actor: initiator, Action: "propose-date", ProposedAppointment: &proposed, Value: "Please move the visit"})
				if err != nil {
					t.Fatal(err)
				}
				state, err = store.ApplyWorkspace(ctx, other, otherRole, state.WorkspaceID, workflow.Action{Actor: otherRole, Action: "accept-date", ProposalID: state.Job.DateProposal.ID})
				if err != nil {
					t.Fatal(err)
				}
				if state.Job.Appointment.SlotID != oldSlot || state.CancellationReview == nil || state.CancellationReview.Kind != "late-reschedule" {
					t.Fatalf("old reservation changed before review: %+v", state)
				}
				cases, err := store.ListWorkExceptions(ctx)
				if err != nil || !hasOperatorCase(cases, "late-reschedule", state.WorkspaceID) {
					t.Fatalf("late reschedule missing from operator queue: %v", err)
				}
				operator := accountFixture(t, ctx, store)
				if err := store.ResolveWorkCase(ctx, operator, state.WorkspaceID, "late-reschedule", decision, "Operator reviewed the requested visit change"); err != nil {
					t.Fatal(err)
				}
				state, err = store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
				if err != nil {
					t.Fatal(err)
				}
				if decision == "approve-reschedule" {
					if state.Job.Appointment.StartUTC != proposed.StartUTC || state.Job.Appointment.SlotID == oldSlot || state.Job.DateProposal.Status != "accepted" {
						t.Fatalf("approved change missing: %+v", state.Job)
					}
				} else if state.Job.Appointment.SlotID != oldSlot || state.Job.DateProposal.Status != "declined" {
					t.Fatalf("declined change altered visit: %+v", state.Job)
				}
			})
		}
	}
}

func TestWithdrawalReviewCancelsPreworkAndRefersPaidWork(t *testing.T) {
	ctx, store := accountTestStore(t)
	for _, active := range []bool{false, true} {
		customer, _, state := bookedCancellationFixture(t, ctx, store)
		if active {
			state.Job.WorkStarted, state.Job.Paid = true, true
			raw, _ := json.Marshal(state)
			if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
				t.Fatal(err)
			}
		}
		state, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "request-withdrawal", Value: "Please review withdrawing my request"})
		if err != nil {
			t.Fatal(err)
		}
		if state.Cancelled || state.CancellationReview.Kind != "withdrawal" {
			t.Fatal("withdrawal was decided automatically")
		}
		operator := accountFixture(t, ctx, store)
		if active {
			if err := store.ResolveWorkCase(ctx, operator, state.WorkspaceID, "withdrawal", "approve-withdrawal", "Operator reviewed this withdrawal request"); err == nil {
				t.Fatal("paid work automatically cancelled")
			}
		}
		decision := "approve-withdrawal"
		if active {
			decision = "review-work"
		}
		if err := store.ResolveWorkCase(ctx, operator, state.WorkspaceID, "withdrawal", decision, "Operator reviewed this withdrawal request"); err != nil {
			t.Fatal(err)
		}
		state, err = store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if active {
			if state.Cancelled || !state.Job.Paid || state.Job.PaymentRefunded || state.Job.Dispute == nil || state.Job.Dispute.Status != "open" {
				t.Fatalf("paid withdrawal did not enter human issue review: %+v", state)
			}
		} else if !state.Cancelled || state.CancellationReview.Status != "approved" {
			t.Fatalf("prework withdrawal decision missing: %+v", state)
		}
	}
}

func TestReviewedConsumerPolicySnapshotsAndAuditableAcknowledgment(t *testing.T) {
	ctx, store := consumerPolicyTestStore(t)
	operator := accountFixture(t, ctx, store)
	if _, err := store.pool.Exec(ctx, `INSERT INTO account_memberships(account_id,role,status) VALUES($1,'operator','active')`, operator); err != nil {
		t.Fatal(err)
	}
	policies, err := store.CurrentConsumerPolicies(ctx)
	if err != nil || len(policies) != 0 {
		t.Fatalf("default policies were invented: %+v %v", policies, err)
	}
	policy := workflow.ConsumerPolicy{Kind: "early_start", Version: "reviewed-v1", Locale: "en", Title: "Fixture policy", Body: "Fixture-only policy text supplied for approval tests.", RequiresAcknowledgment: true, ReviewerID: operator, ReviewRationale: "Reviewed externally for this fixture", ReviewReference: "fixture-decision"}
	outsider := accountFixture(t, ctx, store)
	if err := store.PublishConsumerPolicy(ctx, outsider, policy); err == nil {
		t.Fatal("non-operator published policy")
	}
	if err := store.PublishConsumerPolicy(ctx, operator, policy); err != nil {
		t.Fatal(err)
	}
	customer, professional, state := bookedCancellationFixture(t, ctx, store)
	if len(state.Job.PolicyRequirements) != 1 || state.Job.PolicyRequirements[0].Version != "reviewed-v1" {
		t.Fatalf("booking did not snapshot reviewed policy: %+v", state.Job.PolicyRequirements)
	}
	if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "confirm-appointment"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "start-job"}); err == nil {
		t.Fatal("work started without the required acknowledgment")
	}
	policy.Version = "reviewed-v2"
	if err := store.PublishConsumerPolicy(ctx, operator, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "acknowledge-policy", PolicyKind: "early_start", PolicyVersion: "reviewed-v2"}); err == nil {
		t.Fatal("new policy changed an existing booking's agreement")
	}
	state, err = store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "acknowledge-policy", PolicyKind: "early_start", PolicyVersion: "reviewed-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Job.PolicyAcknowledgments) != 1 || state.Job.PolicyAcknowledgments[0].AcknowledgedAt == "" {
		t.Fatal("acknowledgment was not recorded")
	}
	if _, err := store.ApplyWorkspace(ctx, outsider, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "acknowledge-policy", PolicyKind: "early_start", PolicyVersion: "reviewed-v1"}); err == nil {
		t.Fatal("unrelated account acknowledged another customer's policy")
	}
	if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "start-job"}); err != nil {
		t.Fatal(err)
	}
	policy.Body = "Changed text must never replace a reviewed published version."
	if err := store.PublishConsumerPolicy(ctx, operator, policy); err == nil {
		t.Fatal("published version was overwritten")
	}
}
