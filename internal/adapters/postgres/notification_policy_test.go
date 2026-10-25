package postgres

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
)

func TestNotificationsKeepRoutineEventsInAppAndEmailImportantEventsOnce(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	customer, professional, state := bookedCancellationFixture(t, ctx, store)
	if _, err := store.pool.Exec(ctx, `UPDATE accounts SET email='recipient@example.test' WHERE id=ANY($1)`, []string{customer, professional}); err != nil {
		t.Fatal(err)
	}
	sender := &lifecycleSender{}
	worker := NotificationWorker{pool: store.pool, sender: sender}
	for _, role := range []string{"customer", "professional"} {
		for _, event := range []string{
			"send-message", "agent-question", "coordination-question", "coordination-summary",
			"propose-price", "accept-price", "propose-date", "approve-quote", "approve-change",
			"appointment-reminder", "completion-review-reminder", "checkout-reminder", "evidence-overdue",
			"submit-review", "respond-to-review", "future-unknown-event",
			"book-appointment", "confirm-appointment", "accept-date", "cancel-request", "submit-evidence",
			"confirm-payment", "refund-completed", "issue-reported", "approve-late-reschedule",
		} {
			args := NotificationArgs{JobID: state.Job.ID, EventKey: state.Job.ID + ":policy:" + role + ":" + event, EventType: event, RecipientRole: role}
			tx, err := store.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			for range 2 {
				if err := enqueueNotificationTx(ctx, tx, store.river, args); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var inbox, outbox, queued int
			if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM in_app_notifications WHERE job_id=$1 AND event_key=$2`, args.JobID, args.EventKey).Scan(&inbox); err != nil {
				t.Fatal(err)
			}
			if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND event_key=$2`, args.JobID, args.EventKey).Scan(&outbox); err != nil {
				t.Fatal(err)
			}
			if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='notification' AND args->>'eventKey'=$1`, args.EventKey).Scan(&queued); err != nil {
				t.Fatal(err)
			}
			important := event == "book-appointment" || event == "confirm-appointment" || event == "accept-date" || event == "cancel-request" || event == "submit-evidence" || event == "confirm-payment" || event == "refund-completed" || event == "issue-reported" || event == "approve-late-reschedule"
			want := 0
			if important {
				want = 1
			}
			if inbox != 1 || outbox != want || queued != want {
				t.Fatalf("%s/%s inbox/outbox/jobs=%d/%d/%d, want 1/%d/%d", role, event, inbox, outbox, queued, want, want)
			}
			before := len(sender.messages)
			for range 2 {
				if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err != nil {
					t.Fatal(err)
				}
			}
			if len(sender.messages)-before != want {
				t.Fatalf("%s/%s sent %d emails, want %d", role, event, len(sender.messages)-before, want)
			}
		}
	}
}

func TestRecoveryRetiresRoutineBacklogEvenWithoutSMTP(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, state := bookedCancellationFixture(t, ctx, store)
	jobID := state.Job.ID
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM notification_outbox WHERE job_id=$1`, jobID)
	})
	if _, err := store.pool.Exec(ctx, `INSERT INTO notification_outbox(job_id,event_key,event_type,recipient_role,status)
		SELECT $1,$1||':legacy:'||g::text,'send-message','customer',
		(ARRAY['queued','failed','adapter_unconfigured','sending'])[1+(g%4)] FROM generate_series(1,250) g`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO notification_outbox(job_id,event_key,event_type,recipient_role,status) VALUES
		($1,$1||':sent','send-message','customer','sent'),
		($1,$1||':important','confirm-payment','customer','adapter_unconfigured')`, jobID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if n, err := requeueNotificationOutbox(ctx, store); err != nil || n != 0 {
			t.Fatalf("recovery=%d, %v", n, err)
		}
	}
	var retired, sent, important int
	if err := store.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE status='obsolete' AND last_error='in_app_only'),
		count(*) FILTER (WHERE status='sent'),
		count(*) FILTER (WHERE event_type='confirm-payment' AND status='adapter_unconfigured')
		FROM notification_outbox WHERE job_id=$1`, jobID).Scan(&retired, &sent, &important); err != nil {
		t.Fatal(err)
	}
	if retired != 250 || sent != 1 || important != 1 {
		t.Fatalf("retired/sent/important=%d/%d/%d", retired, sent, important)
	}
}

func TestImportantNotificationCanRetryAfterDeliveryFailure(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	customer, _, state := bookedCancellationFixture(t, ctx, store)
	if _, err := store.pool.Exec(ctx, `UPDATE accounts SET email='recipient@example.test' WHERE id=$1`, customer); err != nil {
		t.Fatal(err)
	}
	args := NotificationArgs{JobID: state.Job.ID, EventKey: state.Job.ID + ":payment-retry", EventType: "confirm-payment", RecipientRole: "customer"}
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
	sender := &lifecycleSender{fail: true}
	worker := NotificationWorker{pool: store.pool, sender: sender}
	if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err == nil {
		t.Fatal("delivery failure was not reported")
	}
	var status string
	var attempts int
	if err := store.pool.QueryRow(ctx, `SELECT status,attempts FROM notification_outbox WHERE job_id=$1 AND event_key=$2`, args.JobID, args.EventKey).Scan(&status, &attempts); err != nil || status != "failed" || attempts != 1 {
		t.Fatalf("failed delivery=%s/%d, %v", status, attempts, err)
	}
	sender.fail = false
	for range 2 {
		if err := worker.Work(ctx, &river.Job[NotificationArgs]{Args: args}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.pool.QueryRow(ctx, `SELECT status,attempts FROM notification_outbox WHERE job_id=$1 AND event_key=$2`, args.JobID, args.EventKey).Scan(&status, &attempts); err != nil || status != "sent" || attempts != 2 || len(sender.messages) != 2 {
		t.Fatalf("retried delivery=%s/%d, sends=%d, %v", status, attempts, len(sender.messages), err)
	}
}
