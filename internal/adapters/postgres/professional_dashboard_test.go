package postgres

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestProfessionalDashboardUsesAllJobsAndVerifiedPayments(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	owner, other, customer := accountFixture(t, ctx, store), accountFixture(t, ctx, store), accountFixture(t, ctx, store)
	seq := 0
	add := func(pro, currency, status string, amount int64, approved, paid, cancelled bool, confirmation string) {
		t.Helper()
		seq++
		id := fmt.Sprintf("%s-stats-%d", customer, seq)
		state := workflow.NewState()
		state.Cancelled = cancelled
		state.Job = workflow.Job{ID: id, ProfessionalAccountID: pro, ScopeVersion: 1, CompletionApproved: approved, Paid: paid, Quote: workflow.Quote{Currency: currency, TotalMinor: amount}}
		if status == "captured-refund-pending" {
			status = "captured"
			state.Job.Dispute = &workflow.DisputeCase{Status: "refund_pending"}
		}
		if confirmation != "" {
			state.Job.Appointment = &workflow.Appointment{StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)}
			state.Job.AppointmentConfirmation = &workflow.AppointmentConfirmation{Status: confirmation, DueAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
		}
		raw, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `INSERT INTO account_workspaces(id,account_id,state_json) VALUES ($1,$2,$3)`, id, customer, raw); err != nil {
			t.Fatal(err)
		}
		if status != "" {
			if _, err = store.pool.Exec(ctx, `INSERT INTO paypal_payment_operations(workspace_id,job_id,scope_version,amount_minor,currency,status,capture_id,order_request_id,authorize_request_id,capture_request_id,void_request_id) VALUES ($1,$1,1,$2,$3,$4,$5,'test-order','test-auth','test-capture','test-void')`, id, amount, currency, status, id+"-capture"); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(owner, "EUR", "captured", 25000, true, true, false, "")
	add(owner, "USD", "captured", 5000, true, true, false, "")
	add(owner, "EUR", "refunded", 1000, true, true, false, "")
	add(owner, "EUR", "partially_refunded", 3000, true, true, false, "")
	add(owner, "EUR", "captured-refund-pending", 4000, true, true, false, "")
	add(owner, "EUR", "authorized", 6000, true, false, false, "")
	add(owner, "EUR", "", 7000, true, true, false, "") // A paid flag alone cannot establish earnings.
	add(other, "EUR", "captured", 999999, true, true, false, "")
	add(owner, "EUR", "", 2000, false, false, true, "confirmed")
	add(owner, "EUR", "", 2000, false, false, false, "pending")
	for range 105 {
		add(owner, "EUR", "", 2000, false, false, false, "confirmed")
	}
	stats, err := store.ProfessionalDashboard(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if stats.UpcomingVisits != 105 || stats.CompletedJobs != 7 || stats.AwaitingConfirmation != 1 {
		t.Fatalf("counts=%+v", stats)
	}
	if len(stats.Balances) != 2 || stats.Balances[0] != (workflow.ProfessionalBalance{Currency: "EUR", ReceivedMinor: 25000, AwaitingPaymentMinor: 6000}) || stats.Balances[1] != (workflow.ProfessionalBalance{Currency: "USD", ReceivedMinor: 5000}) {
		t.Fatalf("balances=%+v", stats.Balances)
	}
	empty, err := store.ProfessionalDashboard(ctx, "missing-professional")
	if err != nil || len(empty.Balances) != 0 || empty.UpcomingVisits != 0 {
		t.Fatalf("empty overview=%+v, %v", empty, err)
	}
	if _, err = store.ProfessionalDashboard(ctx, ""); err == nil {
		t.Fatal("empty account accepted")
	}
}
