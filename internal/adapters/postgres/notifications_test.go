package postgres

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestNotificationDeliveryHealthCountsActionableStatuses(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := store.NotificationDeliveryHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "notification-health-test-"
	if _, err := store.pool.Exec(ctx, `DELETE FROM notification_outbox WHERE job_id LIKE $1`, prefix+"%"); err != nil {
		t.Fatal(err)
	}
	defer store.pool.Exec(context.Background(), `DELETE FROM notification_outbox WHERE job_id LIKE $1`, prefix+"%")
	statuses := []string{"failed", "adapter_unconfigured", "sending", "queued", "sent"}
	now := time.Now().UTC()
	for _, status := range statuses {
		updatedAt := now
		if status == "sending" {
			updatedAt = now.Add(-16 * time.Minute)
		}
		key := prefix + status
		if _, err := store.pool.Exec(ctx, `INSERT INTO notification_outbox (job_id,event_key,event_type,recipient_role,status,updated_at)
			VALUES ($1,$2,'confirm-payment','customer',$3,$4)`, key, key, status, updatedAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO notification_outbox (job_id,event_key,event_type,recipient_role,status,updated_at)
        SELECT $1,$1||':'||s,'send-message','customer',s,now()-interval '20 minutes'
        FROM unnest(ARRAY['failed','adapter_unconfigured','sending']) s`, prefix+"routine"); err != nil {
		t.Fatal(err)
	}
	health, err := store.NotificationDeliveryHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := workflow.NotificationDeliveryHealth{Failed: before.Failed + 1, Unconfigured: before.Unconfigured + 1, Stalled: before.Stalled + 1}
	if health != want {
		t.Fatalf("delivery health = %+v, want %+v", health, want)
	}
}

func TestNotificationEnqueueSkipsDeletedProfessionalRecipient(t *testing.T) {
	ctx, store := accountTestStore(t)
	customer := accountFixture(t, ctx, store)
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	jobID := "deleted-recipient-" + rand.Text()
	state.Job = workflow.Job{ID: jobID, ProfessionalAccountID: "acct_deleted_professional"}
	persistLifecycleState(t, ctx, store, state)
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := enqueueNotificationTx(ctx, tx, nil, NotificationArgs{
		JobID: jobID, EventKey: jobID + ":event", EventType: "book-appointment", RecipientRole: string(identity.Professional),
	}); err != nil {
		t.Fatalf("historical missing recipient blocked the event: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var inboxCount, outboxCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM in_app_notifications WHERE job_id=$1`, jobID).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1`, jobID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if inboxCount != 0 || outboxCount != 1 {
		t.Fatalf("missing recipient created inbox/outbox rows: %d/%d", inboxCount, outboxCount)
	}
}
