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

func bookedCancellationFixture(t *testing.T, ctx context.Context, s *Store, lead ...time.Duration) (string, string, workflow.State) {
	t.Helper()
	customer, professional := accountFixture(t, ctx, s), accountFixture(t, ctx, s)
	if _, err := s.EnsureMembership(ctx, professional, identity.Professional); err != nil {
		t.Fatal(err)
	}
	profile, err := s.SaveProfessionalProfile(ctx, professional, workflow.ProfessionalProfile{DisplayName: "Cancellation professional", Category: "cancel-test", ServiceName: "Repair visit", Currency: "EUR", PriceMinor: 10000, DurationMinutes: 60, MaterialsPolicy: "Parts agreed separately"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE account_id=$1`, professional); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, professional); err != nil {
		t.Fatal(err)
	}
	visitLead := 48 * time.Hour
	if len(lead) > 0 {
		visitLead = lead[0]
	}
	start := time.Now().UTC().Add(visitLead).Truncate(time.Minute)
	slot, err := s.AddProfessionalAvailability(ctx, professional, workflow.Appointment{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(time.Hour).Format(time.RFC3339), Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	state.Intake = &workflow.Intake{Messages: []workflow.Message{{Role: "customer", Text: "Repair the sink"}}}
	state.Job = workflow.Job{ID: "cancel-" + rand.Text(), ProfessionalAccountID: professional, ProfessionalProfileID: profile.ID, Title: "Repair visit", Description: "Repair the sink", ScopeVersion: 1, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 10000, DurationMinutes: 60, Lines: []workflow.QuoteLine{{Label: "Visit", AmountMinor: 10000}}}, Timeline: []workflow.Event{}}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.pool.Exec(context.Background(), `DELETE FROM booking_reservations WHERE job_id=$1`, state.Job.ID)
	})
	state, err = s.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "approve-and-book", Value: slot})
	if err != nil {
		t.Fatal(err)
	}
	return customer, professional, state
}

func TestLateCancellationKeepsReservationUntilApproved(t *testing.T) {
	ctx, s := accountTestStore(t)
	operator := accountFixture(t, ctx, s)
	for _, decision := range []string{"keep-visit", "approve-cancellation"} {
		t.Run(decision, func(t *testing.T) {
			customer, professional, booked := bookedCancellationFixture(t, ctx, s, 23*time.Hour)
			pending, err := s.ApplyWorkspace(ctx, customer, "customer", booked.WorkspaceID, workflow.Action{Actor: "customer", Action: "cancel-request"})
			if err != nil || pending.Cancelled || pending.CancellationReview == nil || pending.CancellationReview.Status != "pending" {
				t.Fatalf("late cancellation did not stay pending: %+v, %v", pending.CancellationReview, err)
			}
			var status string
			if err := s.pool.QueryRow(ctx, `SELECT status FROM booking_reservations WHERE job_id=$1`, booked.Job.ID).Scan(&status); err != nil || status != "booked" {
				t.Fatalf("pending reservation = %q, %v", status, err)
			}
			if slots, err := s.AvailableProfessionalSlots(ctx, professional, ""); err != nil || len(slots) != 0 {
				t.Fatalf("another customer could claim pending visit: %+v, %v", slots, err)
			}
			if err := s.ResolveWorkCase(ctx, operator, booked.WorkspaceID, "late-cancellation", decision, "Operator reviewed this request"); err != nil {
				t.Fatal(err)
			}
			if err := s.pool.QueryRow(ctx, `SELECT status FROM booking_reservations WHERE job_id=$1`, booked.Job.ID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			want := "booked"
			if decision == "approve-cancellation" {
				want = "released"
			}
			if status != want {
				t.Fatalf("reservation after %s = %q, want %q", decision, status, want)
			}
		})
	}
}

func TestCancellationReleasesRealBookingAndRejectsOutsiders(t *testing.T) {
	ctx, s := accountTestStore(t)
	for _, role := range []string{"customer", "professional"} {
		t.Run(role, func(t *testing.T) {
			customer, professional, booked := bookedCancellationFixture(t, ctx, s)
			outsider := accountFixture(t, ctx, s)
			for _, outsiderRole := range []string{"customer", "professional"} {
				if _, err := s.ApplyWorkspace(ctx, outsider, outsiderRole, booked.WorkspaceID, workflow.Action{Actor: outsiderRole, Action: "cancel-request"}); err == nil {
					t.Fatal("outsider cancelled request")
				}
			}
			actor := customer
			if role == "professional" {
				actor = professional
			}
			cancelled, err := s.ApplyWorkspace(ctx, actor, role, booked.WorkspaceID, workflow.Action{Actor: role, Action: "cancel-request", Value: "No longer needed"})
			if err != nil {
				t.Fatal(err)
			}
			if !cancelled.Cancelled || cancelled.CancelledBy != role || cancelled.Job.Appointment == nil || len(cancelled.Intake.Messages) != 1 || len(cancelled.AvailableSlots) != 0 {
				t.Fatal("cancellation lost history or retained actionable slots")
			}
			var reservation, availability string
			if err = s.pool.QueryRow(ctx, `SELECT b.status,a.status FROM booking_reservations b JOIN professional_availability_slots a ON a.id=b.slot_id WHERE b.job_id=$1`, booked.Job.ID).Scan(&reservation, &availability); err != nil {
				t.Fatal(err)
			}
			if reservation != "released" || availability != "available" {
				t.Fatalf("reservation=%s availability=%s", reservation, availability)
			}
			slots, err := s.AvailableProfessionalSlots(ctx, professional, "")
			if err != nil || len(slots) != 1 || slots[0].SlotID != booked.Job.Appointment.SlotID {
				t.Fatal("released visit cannot be booked again")
			}
			loaded, err := s.LoadWorkspace(ctx, professional, "professional", booked.WorkspaceID)
			if err != nil || !loaded.Cancelled {
				t.Fatal("professional does not see cancellation")
			}
			if _, err = s.ApplyWorkspace(ctx, professional, "professional", booked.WorkspaceID, workflow.Action{Actor: "professional", Action: "start-job"}); err == nil {
				t.Fatal("cancelled work could start")
			}
		})
	}
}

func TestCancellationAndStartAreSerialized(t *testing.T) {
	ctx, s := accountTestStore(t)
	customer, professional, booked := bookedCancellationFixture(t, ctx, s)
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for index, action := range []string{"cancel-request", "start-job"} {
		wg.Add(1)
		go func(i int, action string) {
			defer wg.Done()
			<-barrier
			actor, role := customer, "customer"
			if i == 1 {
				actor, role = professional, "professional"
			}
			_, errs[i] = s.ApplyWorkspace(ctx, actor, role, booked.WorkspaceID, workflow.Action{Actor: role, Action: action})
		}(index, action)
	}
	close(barrier)
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("expected one successful transition, got %v", errs)
	}
	loaded, err := s.LoadWorkspace(ctx, customer, "customer", booked.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Cancelled == loaded.Job.WorkStarted {
		t.Fatal("cancellation and active work are inconsistent")
	}
	var status string
	if err = s.pool.QueryRow(ctx, `SELECT status FROM booking_reservations WHERE job_id=$1`, booked.Job.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if loaded.Cancelled && status != "released" || loaded.Job.WorkStarted && status != "booked" {
		t.Fatal("reservation disagrees with request")
	}
}

func TestExpiredProfessionalConfirmationReleasesReservation(t *testing.T) {
	ctx, s := accountTestStore(t)
	customer, professional, booked := bookedCancellationFixture(t, ctx, s)
	booked.Job.AppointmentConfirmation.Status = "pending"
	booked.Job.AppointmentConfirmation.DueAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	raw, err := json.Marshal(booked)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, booked.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.expirePendingAppointmentConfirmations(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadWorkspace(ctx, customer, "customer", booked.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Job.Appointment != nil || loaded.Job.AppointmentConfirmation.Status != "expired" || len(loaded.AvailableSlots) != 1 {
		t.Fatal("expired confirmation did not release the visit and return its availability")
	}
	var reservation, availability string
	if err := s.pool.QueryRow(ctx, `SELECT b.status,a.status FROM booking_reservations b JOIN professional_availability_slots a ON a.id=b.slot_id WHERE b.job_id=$1`, booked.Job.ID).Scan(&reservation, &availability); err != nil {
		t.Fatal(err)
	}
	if reservation != "released" || availability != "available" {
		t.Fatalf("expired booking remains reserved: reservation=%s availability=%s", reservation, availability)
	}
	proState, err := s.LoadWorkspace(ctx, professional, "professional", booked.WorkspaceID)
	if err != nil || proState.Job.AppointmentConfirmation.Status != "expired" {
		t.Fatal("professional does not see the expired appointment decision")
	}
}
