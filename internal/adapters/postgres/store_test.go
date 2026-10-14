package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestPostgresSerializesOverlappingBookings(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL concurrency check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := Migrate(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	professionalID := fmt.Sprintf("concurrency-test-%d", time.Now().UnixNano())
	jobIDs := []string{professionalID + "-a", professionalID + "-b"}
	if _, err := store.pool.Exec(ctx, `INSERT INTO accounts (id, issuer, subject) VALUES ($1, 'test', $1)`, professionalID); err != nil {
		t.Fatal(err)
	}
	defer store.pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, professionalID)
	if _, err := store.pool.Exec(ctx, `INSERT INTO professional_profiles
		(id, account_id, display_name, category, service_name, service_area, currency, price_minor, callout_minor, duration_minutes, materials_policy, status)
		VALUES ($1, $1, 'Test professional', 'Plumbing', 'Test repair', 'Amsterdam', 'USD', 10000, 2000, 30, 'Included', 'active')`, professionalID); err != nil {
		t.Fatal(err)
	}
	appointment := demoAppointment()
	appointment.SlotID = professionalID + "-slot"
	if _, err := store.pool.Exec(ctx, `INSERT INTO professional_availability_slots (id, professional_account_id, starts_at, ends_at, timezone)
		VALUES ($1, $2, $3, $4, 'UTC')`, appointment.SlotID, professionalID, mustParseUTC(appointment.StartUTC), mustParseUTC(appointment.EndUTC)); err != nil {
		t.Fatal(err)
	}
	defer store.pool.Exec(context.Background(), `DELETE FROM booking_reservations WHERE job_id = ANY($1)`, jobIDs)

	start := make(chan struct{})
	ready := sync.WaitGroup{}
	ready.Add(2)
	results := make(chan error, 2)
	for _, jobID := range jobIDs {
		go func(id string) {
			tx, beginErr := store.pool.Begin(ctx)
			if beginErr != nil {
				results <- beginErr
				ready.Done()
				return
			}
			ready.Done()
			<-start
			job := workflow.Job{ID: id, ProfessionalAccountID: professionalID, Appointment: appointment}
			bookingErr := reserveBooking(ctx, tx, job)
			if bookingErr != nil {
				_ = tx.Rollback(ctx)
				results <- bookingErr
				return
			}
			results <- tx.Commit(ctx)
		}(jobID)
	}
	ready.Wait()
	close(start)
	successes := 0
	for range 2 {
		if resultErr := <-results; resultErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("overlapping booking successes = %d, want 1", successes)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM booking_reservations WHERE professional_id = $1 AND status = 'booked'`, professionalID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active reservations = %d, want 1", count)
	}
}

func TestPostgresRiverRecordsQueuedNotificationWithoutClaimingDelivery(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the River notification check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := Migrate(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accountID := fmt.Sprintf("acct-test-%d", time.Now().UnixNano())
	if _, err := store.pool.Exec(ctx, `INSERT INTO accounts (id, issuer, subject) VALUES ($1, 'test', $1)`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO account_memberships (account_id, role, status) VALUES ($1, 'customer', 'active')`, accountID); err != nil {
		t.Fatal(err)
	}
	defer store.pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	state, err := store.CreateWorkspace(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	jobID := fmt.Sprintf("job-test-%d", time.Now().UnixNano())
	state.Job = workflow.Job{ID: jobID, Title: "Test request", Category: "Plumbing", ScopeVersion: 1, EvidenceSubmitted: true, Evidence: []workflow.Evidence{{Label: "Evidence", Kind: "photo", AssetID: "asset_test"}}, Timeline: []workflow.Event{}}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json = $2 WHERE id = $1`, state.WorkspaceID, stateBytes); err != nil {
		t.Fatal(err)
	}
	var nextRevision int64
	if err := store.pool.QueryRow(ctx, `SELECT revision + 1 FROM account_workspaces WHERE id = $1`, state.WorkspaceID).Scan(&nextRevision); err != nil {
		t.Fatal(err)
	}
	eventKey := fmt.Sprintf("%s:%d:request-details", jobID, nextRevision)
	if _, err := store.ApplyWorkspace(ctx, accountID, "customer", state.WorkspaceID, workflow.Action{Action: "request-details", Actor: "customer"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err = store.pool.QueryRow(ctx, `
			SELECT status FROM notification_outbox
			WHERE job_id = $1 AND event_key = $2`, jobID, eventKey).Scan(&status)
		if err == nil && status == "adapter_unconfigured" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("River did not record the notification as adapter_unconfigured; last query error: %v", err)
}

func demoAppointment() *workflow.Appointment {
	return &workflow.Appointment{SlotID: "concurrency-test-slot", StartsAt: "Test slot", Timezone: "UTC", StartUTC: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), EndUTC: time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339)}
}
