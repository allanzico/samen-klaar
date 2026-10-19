package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

// workspaceFixture inserts an empty request without the open-request limit, for tests that
// need many requests on one account.
func workspaceFixture(ctx context.Context, s *Store, accountID string) (workflow.State, error) {
	state := workflow.NewState()
	state.WorkspaceID = "fixture-" + rand.Text()
	state.ParentRequestID = state.WorkspaceID
	raw, err := json.Marshal(state)
	if err != nil {
		return workflow.State{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO account_workspaces (id, account_id, state_json) VALUES ($1, $2, $3)`, state.WorkspaceID, accountID, raw)
	return state, err
}

// unapprovedJobFixture gives a customer a request with an assigned professional whose plan
// is still waiting for approval.
func unapprovedJobFixture(t *testing.T, ctx context.Context, s *Store) (customer, professional string, state workflow.State) {
	t.Helper()
	customer, professional = accountFixture(t, ctx, s), accountFixture(t, ctx, s)
	if _, err := s.EnsureMembership(ctx, professional, identity.Professional); err != nil {
		t.Fatal(err)
	}
	state, err := s.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	state.Intake = &workflow.Intake{Step: "matches", Description: "Repair the sink", Messages: []workflow.Message{{Role: "customer", Text: "Repair the sink"}}}
	state.Job = workflow.Job{ID: "removal-" + rand.Text(), ProfessionalAccountID: professional, Professional: "Removal professional", Customer: "Sam", Title: "Repair visit", Description: "Repair the sink", ScopeVersion: 1, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 10000, DurationMinutes: 60, Lines: []workflow.QuoteLine{{Label: "Visit", AmountMinor: 10000}}}, Timeline: []workflow.Event{}}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	return customer, professional, state
}

func TestOpenRequestLimitIsEnforcedOnCreate(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	ids := []string{}
	for range workflow.MaxOpenRequests {
		state, err := store.CreateWorkspace(ctx, customer)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, state.WorkspaceID)
	}
	if _, err := store.CreateWorkspace(ctx, customer); !errors.Is(err, workflow.ErrOpenRequestLimit) {
		t.Fatalf("fourth open request: %v", err)
	}
	if count, err := store.OpenRequestCount(ctx, customer); err != nil || count != workflow.MaxOpenRequests {
		t.Fatalf("open count = %d, %v", count, err)
	}
	// Cancelling one frees its place; the cancelled request stays deletable.
	if _, err := store.ApplyWorkspace(ctx, customer, "customer", ids[0], workflow.Action{Actor: "customer", Action: "cancel-request"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(ctx, customer); err != nil {
		t.Fatalf("cancelling should free a place: %v", err)
	}
	if _, err := store.CreateWorkspace(ctx, customer); !errors.Is(err, workflow.ErrOpenRequestLimit) {
		t.Fatalf("limit again: %v", err)
	}
	if status, err := store.RemoveRequest(ctx, customer, "customer", ids[1]); err != nil || status != "deleted" {
		t.Fatalf("delete draft: %q %v", status, err)
	}
	if _, err := store.CreateWorkspace(ctx, customer); err != nil {
		t.Fatalf("deleting should free a place: %v", err)
	}
	items, err := store.ListWorkspaces(ctx, customer, "customer")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if !item.CanDelete {
			t.Fatalf("unapproved request %s is not marked deletable", item.ID)
		}
	}
}

func TestCustomerDeletesUnapprovedRequestAndKeepsAudit(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, professional, state := unapprovedJobFixture(t, ctx, store)
	if _, err := store.RemoveRequest(ctx, professional, "customer", state.WorkspaceID); err == nil {
		t.Fatal("another account deleted the customer's request")
	}
	if status, err := store.RemoveRequest(ctx, customer, "customer", state.WorkspaceID); err != nil || status != "deleted" {
		t.Fatalf("delete: %q %v", status, err)
	}
	var exists bool
	if err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_workspaces WHERE id=$1)`, state.WorkspaceID).Scan(&exists); err != nil || exists {
		t.Fatalf("workspace still exists: %v %v", exists, err)
	}
	var audits int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE workspace_id=$1 AND job_id=$2 AND action='delete-request' AND actor_account_id=$3`, state.WorkspaceID, state.Job.ID, customer).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("delete audit events = %d, %v", audits, err)
	}
	if jobs, _ := store.ListWorkspaces(ctx, professional, "professional"); len(jobs) != 0 {
		t.Fatalf("deleted request still listed for the professional: %+v", jobs)
	}
}

func TestApprovedAndBookedRequestsCannotBeDeleted(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, professional, booked := bookedCancellationFixture(t, ctx, store)
	if _, err := store.RemoveRequest(ctx, customer, "customer", booked.WorkspaceID); !errors.Is(err, workflow.ErrRequestNotDeletable) {
		t.Fatalf("customer delete booked request: %v", err)
	}
	if _, err := store.RemoveRequest(ctx, professional, "professional", booked.WorkspaceID); !errors.Is(err, workflow.ErrRequestNotRemovable) {
		t.Fatalf("professional remove booked request: %v", err)
	}
	// Cancelling an approved booking keeps the record, and it still cannot be deleted.
	if _, err := store.ApplyWorkspace(ctx, customer, "customer", booked.WorkspaceID, workflow.Action{Actor: "customer", Action: "cancel-request"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoveRequest(ctx, customer, "customer", booked.WorkspaceID); !errors.Is(err, workflow.ErrRequestNotDeletable) {
		t.Fatalf("customer delete cancelled booking: %v", err)
	}
	var reservations int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM booking_reservations WHERE job_id=$1`, booked.Job.ID).Scan(&reservations); err != nil || reservations != 1 {
		t.Fatalf("booking record = %d, %v", reservations, err)
	}
}

func TestDeleteRefusesWhenPaymentRecordExists(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, _, state := unapprovedJobFixture(t, ctx, store)
	// A booking row for an unapproved job should never exist; if one does, it must survive.
	if _, err := store.pool.Exec(ctx, `INSERT INTO booking_reservations (job_id, professional_id, slot_id, starts_at, ends_at, timezone, status) VALUES ($1, 'p', 's', now() + interval '2 days', now() + interval '2 days 1 hour', 'UTC', 'released')`, state.Job.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.pool.Exec(context.Background(), `DELETE FROM booking_reservations WHERE job_id=$1`, state.Job.ID)
	})
	if _, err := store.RemoveRequest(ctx, customer, "customer", state.WorkspaceID); !errors.Is(err, workflow.ErrRequestNotDeletable) {
		t.Fatalf("delete with a booking record: %v", err)
	}
}

func TestProfessionalRemovalDeclinesAndHidesOnlyForThem(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, professional, state := unapprovedJobFixture(t, ctx, store)
	if jobs, _ := store.ListWorkspaces(ctx, professional, "professional"); len(jobs) != 1 || !jobs[0].CanDelete {
		t.Fatalf("professional list before removal: %+v", jobs)
	}
	if status, err := store.RemoveRequest(ctx, professional, "professional", state.WorkspaceID); err != nil || status != "removed" {
		t.Fatalf("remove: %q %v", status, err)
	}
	if jobs, _ := store.ListWorkspaces(ctx, professional, "professional"); len(jobs) != 0 {
		t.Fatalf("removed job still listed: %+v", jobs)
	}
	kept, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Cancelled || kept.CancelledBy != "professional" || kept.Intake == nil || kept.Intake.Description != "Repair the sink" {
		t.Fatalf("customer request should be declined and kept: cancelled=%v by=%q", kept.Cancelled, kept.CancelledBy)
	}
	var notified int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND event_type='cancel-request' AND recipient_role='customer'`, state.Job.ID).Scan(&notified); err != nil || notified != 1 {
		t.Fatalf("customer notifications = %d, %v", notified, err)
	}
	// Removing again is harmless and does not notify twice.
	if _, err := store.RemoveRequest(ctx, professional, "professional", state.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND event_type='cancel-request'`, state.Job.ID).Scan(&notified); err != nil || notified != 1 {
		t.Fatalf("customer notifications after repeat = %d, %v", notified, err)
	}
}

func TestReturnToMatchesRespectsOpenRequestLimit(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer, professional, state := unapprovedJobFixture(t, ctx, store)
	if _, err := store.RemoveRequest(ctx, professional, "professional", state.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	for range workflow.MaxOpenRequests {
		if _, err := store.CreateWorkspace(ctx, customer); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "return-to-matches"}); !errors.Is(err, workflow.ErrOpenRequestLimit) {
		t.Fatalf("reopen past the limit: %v", err)
	}
}

func TestSplitJobsCountTowardsOpenRequestLimit(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	parent, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	plan := func(n int) []workflow.JobPlan {
		plans := []workflow.JobPlan{}
		for i := range n {
			plans = append(plans, workflow.JobPlan{ID: "plan-" + rand.Text()[:8], Description: "Separate job number " + string(rune('A'+i))})
		}
		return plans
	}
	if _, err := store.AddRequestJobs(ctx, customer, parent.WorkspaceID, plan(workflow.MaxOpenRequests), "en"); !errors.Is(err, workflow.ErrOpenRequestLimit) {
		t.Fatalf("split past the limit: %v", err)
	}
	if _, err := store.AddRequestJobs(ctx, customer, parent.WorkspaceID, plan(workflow.MaxOpenRequests-1), "en"); err != nil {
		t.Fatal(err)
	}
	if count, err := store.OpenRequestCount(ctx, customer); err != nil || count != workflow.MaxOpenRequests {
		t.Fatalf("open count = %d, %v", count, err)
	}
}
