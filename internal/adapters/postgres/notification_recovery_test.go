package postgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	mailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type notificationSenderFixture struct{}

func (notificationSenderFixture) Send(context.Context, mailadapter.Message) error { return nil }

func TestConfiguredDeliveryDrainsMoreThanOneRecoveryBatch(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for outbox recovery test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	id := "recovery-" + time.Now().UTC().Format("20060102150405.000000000")
	if _, err := directory.pool.Exec(ctx, `INSERT INTO accounts(id,issuer,subject,email) VALUES ($1,'test',$1,'recovery@example.test')`, id); err != nil {
		t.Fatal(err)
	}
	defer directory.pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=$1`, id)
	state := workflow.NewState()
	state.Job.ID = id + "-job"
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := directory.pool.Exec(ctx, `INSERT INTO account_workspaces(id,account_id,state_json) VALUES ($1,$2,$3)`, id+"-workspace", id, data); err != nil {
		t.Fatal(err)
	}
	if _, err := directory.pool.Exec(ctx, `INSERT INTO notification_outbox(job_id,event_key,event_type,recipient_role,status)
		SELECT $1,$1||':'||g::text,'confirm-payment','customer','adapter_unconfigured' FROM generate_series(1,250) g`, state.Job.ID); err != nil {
		t.Fatal(err)
	}
	defer directory.pool.Exec(context.Background(), `DELETE FROM notification_outbox WHERE job_id=$1`, state.Job.ID)
	defer directory.pool.Exec(context.Background(), `DELETE FROM river_job WHERE kind='notification' AND args->>'jobId'=$1`, state.Job.ID)
	worker, err := OpenWithNotifications(ctx, url, notificationSenderFixture{}, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var sent int
		if err := directory.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND status='sent'`, state.Job.ID).Scan(&sent); err != nil {
			t.Fatal(err)
		}
		if sent == 250 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	var sent int
	_ = directory.pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND status='sent'`, state.Job.ID).Scan(&sent)
	t.Fatalf("sent %d of 250 eligible notifications", sent)
}
