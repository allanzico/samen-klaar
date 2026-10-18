package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/riverqueue/river"
)

type lifecycleSender struct {
	messages []mailadapter.Message
	fail     bool
}

func (s *lifecycleSender) Send(_ context.Context, message mailadapter.Message) error {
	s.messages = append(s.messages, message)
	if s.fail {
		return errors.New("fixture transient failure")
	}
	return nil
}

func enqueueReminderFixture(t *testing.T, ctx context.Context, store *Store, state workflow.State, event string) NotificationArgs {
	t.Helper()
	persistLifecycleState(t, ctx, store, state)
	var args NotificationArgs
	for _, wait := range workflow.WaitingStates(state) {
		if wait.EventType == event {
			args = NotificationArgs{JobID: state.Job.ID, EventKey: wait.EventKey(state.Job), EventType: event, RecipientRole: wait.ResponsibleRole}
		}
	}
	if args.EventType == "" {
		t.Fatalf("fixture has no %s waiting state", event)
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := enqueueNotificationTx(ctx, tx, nil, args); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return args
}

func TestActionableReminderRechecksDecisionScopeDisputeAndCancellationBeforeRetry(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	for _, change := range []string{"complete", "scope", "dispute", "cancel", "replace-submission"} {
		t.Run(change, func(t *testing.T) {
			customer, _, state := bookedCancellationFixture(t, ctx, store)
			if _, err := store.pool.Exec(ctx, `UPDATE accounts SET email='recipient@example.test' WHERE id=$1`, customer); err != nil {
				t.Fatal(err)
			}
			state.Job.EvidenceSubmitted = true
			state.Job.DetailAdded = true
			state.Job.EvidenceSubmittedAt = time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339)
			args := enqueueReminderFixture(t, ctx, store, state, "completion-review-reminder")
			sender := &lifecycleSender{fail: true}
			worker := NotificationWorker{pool: store.pool, sender: sender, appBaseURL: "https://example.test"}
			if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err == nil {
				t.Fatal("fixture did not exercise failed delivery")
			}
			switch change {
			case "complete":
				state.Job.CompletionApproved = true
			case "scope":
				state.Job.ScopeVersion++
			case "dispute":
				state.Job.Dispute = &workflow.DisputeCase{Status: "open"}
			case "cancel":
				state.Cancelled = true
			case "replace-submission":
				state.Job.EvidenceSubmittedAt = time.Now().UTC().Format(time.RFC3339)
			}
			persistLifecycleState(t, ctx, store, state)
			sender.fail = false
			if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err != nil {
				t.Fatal(err)
			}
			if len(sender.messages) != 1 {
				t.Fatalf("obsolete retry sent another email: %d", len(sender.messages))
			}
			var status string
			if err := store.pool.QueryRow(ctx, `SELECT status FROM notification_outbox WHERE job_id=$1 AND event_key=$2`, args.JobID, args.EventKey).Scan(&status); err != nil || status != "obsolete" {
				t.Fatalf("obsolete status=%s, %v", status, err)
			}
			inbox, err := store.ListNotifications(ctx, customer, "customer")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range inbox {
				if item.EventType == args.EventType {
					found = true
				}
			}
			if !found {
				t.Fatal("suppression erased historical in-app notification")
			}
		})
	}
}

func TestAppointmentReminderDoesNotAnnounceReplacedVisit(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	customer, _, state := bookedCancellationFixture(t, ctx, store)
	if _, err := store.pool.Exec(ctx, `UPDATE accounts SET email='recipient@example.test' WHERE id=$1`, customer); err != nil {
		t.Fatal(err)
	}
	state.Job.AppointmentConfirmation.Status = "confirmed"
	start := time.Now().UTC().Add(12 * time.Hour).Truncate(time.Minute)
	state.Job.Appointment.StartUTC = start.Format(time.RFC3339)
	state.Job.Appointment.EndUTC = start.Add(time.Hour).Format(time.RFC3339)
	persistLifecycleState(t, ctx, store, state)
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	var args NotificationArgs
	args.JobID = state.Job.ID
	args.EventType = "appointment-reminder"
	args.RecipientRole = "customer"
	if err := store.pool.QueryRow(ctx, `SELECT event_key FROM notification_outbox WHERE job_id=$1 AND event_type=$2 AND recipient_role='customer'`, args.JobID, args.EventType).Scan(&args.EventKey); err != nil {
		t.Fatal(err)
	}
	state.Job.Appointment.StartUTC = start.Add(2 * time.Hour).Format(time.RFC3339)
	state.Job.Appointment.EndUTC = start.Add(3 * time.Hour).Format(time.RFC3339)
	persistLifecycleState(t, ctx, store, state)
	sender := &lifecycleSender{}
	worker := NotificationWorker{pool: store.pool, sender: sender}
	if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 0 {
		t.Fatal("old appointment email was delivered after rescheduling")
	}
}

func TestNotificationLanguageUsesRecipientPreferenceAndKeepsHistoricalEvents(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	customer, professional, state := bookedCancellationFixture(t, ctx, store)
	if err := store.SetAccountLocale(ctx, customer, "nl"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountLocale(ctx, professional, "en"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE accounts SET email='recipient@example.test' WHERE id=ANY($1)`, []string{customer, professional}); err != nil {
		t.Fatal(err)
	}
	state.Cancelled = true
	persistLifecycleState(t, ctx, store, state)
	sender := &lifecycleSender{}
	worker := NotificationWorker{pool: store.pool, sender: sender}
	for _, role := range []string{"customer", "professional"} {
		args := NotificationArgs{JobID: state.Job.ID, EventKey: state.Job.ID + ":history:" + role, EventType: "send-message", RecipientRole: role}
		tx, err := store.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := enqueueNotificationTx(ctx, tx, nil, args); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.messages) != 2 || sender.messages[0].Subject != "Nieuw bericht over je aanvraag" || !strings.Contains(sender.messages[0].Body, "Open je aanvraag:") || sender.messages[1].Subject != "New message about your service request" {
		t.Fatalf("recipient languages/history = %+v", sender.messages)
	}
	user, err := store.UpsertIdentity(ctx, "account-test", "locale-retained-"+customer, "", "Locale test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountLocale(ctx, user.ID, "nl"); err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertIdentity(ctx, "account-test", "locale-retained-"+customer, "", "New name")
	if err != nil || updated.Locale != "nl" {
		t.Fatalf("sign-in overwrote language preference: %+v, %v", updated, err)
	}
	if err := store.SetAccountLocale(ctx, user.ID, "fr"); err == nil {
		t.Fatal("unsupported locale was accepted")
	}
	store.pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, user.ID)
}
