package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestLifecycleSchedulerQueuesRemindersOnceAndEscalatesStalledWork(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	professional := accountFixture(t, ctx, store)
	if _, err := store.EnsureMembership(ctx, professional, identity.Professional); err != nil {
		t.Fatal(err)
	}
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	jobID := "lifecycle-" + rand.Text()
	state.Job = workflow.Job{
		ID: jobID, Title: "Sink repair", ProfessionalAccountID: professional, ScopeVersion: 1,
		Appointment:             &workflow.Appointment{SlotID: "slot-reminder", StartUTC: now.Add(12 * time.Hour).Format(time.RFC3339), EndUTC: now.Add(13 * time.Hour).Format(time.RFC3339)},
		AppointmentConfirmation: &workflow.AppointmentConfirmation{Status: "confirmed"},
		Quote:                   workflow.Quote{Currency: "EUR", TotalMinor: 10000},
	}
	persistLifecycleState(t, ctx, store, state)
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxCount(t, ctx, store, jobID, "appointment-reminder", 2)

	state.Job.WorkStarted = true
	state.Job.WorkStartedAt = now.Add(-26 * time.Hour).Format(time.RFC3339)
	state.Job.Appointment.StartUTC = now.Add(-26 * time.Hour).Format(time.RFC3339)
	state.Job.Appointment.EndUTC = now.Add(-25 * time.Hour).Format(time.RFC3339)
	persistLifecycleState(t, ctx, store, state)
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxCount(t, ctx, store, jobID, "evidence-overdue", 1)
	if cases, err := store.ListWorkExceptions(ctx); err != nil {
		t.Fatal(err)
	} else if !hasOperatorCase(cases, "stalled-job", state.WorkspaceID) {
		t.Fatal("overdue evidence did not appear in the operator escalation queue")
	}

	state.Job.EvidenceSubmitted = true
	state.Job.EvidenceSubmittedAt = now.Add(-25 * time.Hour).Format(time.RFC3339)
	state.Job.DetailAdded = true
	state.Job.CompletionApproved = false
	persistLifecycleState(t, ctx, store, state)
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxCount(t, ctx, store, jobID, "completion-review-reminder", 1)

	state.Job.CompletionApproved = true
	state.Job.CompletionApprovedAt = now.Add(-25 * time.Hour).Format(time.RFC3339)
	state.Job.PaymentApprovedVersion = 0
	persistLifecycleState(t, ctx, store, state)
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxCount(t, ctx, store, jobID, "payment-approval-reminder", 1)
}

func TestLifecycleSchedulerAdvancesPastFirstHundredJobs(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	start := time.Now().UTC().Add(12 * time.Hour).Format(time.RFC3339)
	end := time.Now().UTC().Add(13 * time.Hour).Format(time.RFC3339)
	jobPrefix := "batch-" + rand.Text()
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM river_job WHERE kind='notification' AND args->>'jobId' LIKE $1`, jobPrefix+"%")
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM notification_outbox WHERE job_id LIKE $1`, jobPrefix+"%")
	})
	for i := 0; i < 101; i++ {
		state := workflow.NewState()
		state.Job = workflow.Job{ID: jobPrefix + "-" + string(rune(1000+i)), ScopeVersion: 1,
			Appointment:             &workflow.Appointment{SlotID: jobPrefix + "-slot-" + string(rune(1000+i)), StartUTC: start, EndUTC: end},
			AppointmentConfirmation: &workflow.AppointmentConfirmation{Status: "confirmed"}}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `INSERT INTO account_workspaces(id,account_id,state_json) VALUES ($1,$2,$3)`, jobPrefix+"-workspace-"+string(rune(1000+i)), customer, data); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := store.scheduleLifecycleNotifications(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var workers sync.WaitGroup
	workerErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); workerErrors <- store.scheduleLifecycleNotifications(ctx) }()
	}
	workers.Wait()
	close(workerErrors)
	for err := range workerErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id LIKE $1 AND event_type='appointment-reminder'`, jobPrefix+"%").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 202 {
		t.Fatalf("queued %d of 202 reminders", count)
	}
}

func persistLifecycleState(t *testing.T, ctx context.Context, store *Store, state workflow.State) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,updated_at=now() WHERE id=$1`, state.WorkspaceID, data); err != nil {
		t.Fatal(err)
	}
}

func assertOutboxCount(t *testing.T, ctx context.Context, store *Store, jobID, eventType string, expected int) {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND event_type=$2`, jobID, eventType).Scan(&count); err != nil || count != expected {
		t.Fatalf("outbox %s count=%d, err=%v; want %d", eventType, count, err, expected)
	}
}

func hasOperatorCase(cases []workflow.OperatorCase, kind, workspaceID string) bool {
	for _, item := range cases {
		if item.Kind == kind && item.WorkspaceID == workspaceID {
			return true
		}
	}
	return false
}
