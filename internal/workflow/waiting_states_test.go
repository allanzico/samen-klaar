package workflow

import (
	"testing"
	"time"
)

func TestWaitingStateMatrixHasDeterministicDeadlineAndResponsibility(t *testing.T) {
	const started = "2026-10-10T10:00:00Z"
	now, _ := time.Parse(time.RFC3339, "2026-10-11T10:00:00Z")
	tests := []struct {
		event, role, action string
		setup               func(*Job)
	}{
		{"missed-visit-reminder", "professional", "review-missed-visit", func(j *Job) {
			j.Appointment = &Appointment{StartUTC: "2026-10-10T09:00:00Z", EndUTC: started}
			j.AppointmentConfirmation = &AppointmentConfirmation{Status: "confirmed"}
		}},
		{"price-proposal-reminder", "professional", "decide-price", func(j *Job) {
			j.PriceProposal = &PriceProposal{ID: "price", Status: "pending", Actor: "customer", ScopeVersion: 1, CreatedAt: started}
		}},
		{"date-proposal-reminder", "customer", "decide-date", func(j *Job) {
			j.DateProposal = &DateProposal{ID: "date", Status: "pending", Actor: "professional", CreatedAt: started}
		}},
		{"scope-change-reminder", "customer", "decide-scope-change", func(j *Job) { j.Change = &ScopeChange{Status: "pending", ProposedFrom: 1, CreatedAt: started} }},
		{"evidence-overdue", "professional", "submit-evidence", func(j *Job) { j.WorkStarted = true; j.WorkStartedAt = started }},
		{"completion-note-overdue", "professional", "add-evidence", func(j *Job) { j.EvidenceSubmitted = true; j.EvidenceSubmittedAt = started }},
		{"completion-review-reminder", "customer", "approve-completion", func(j *Job) { j.EvidenceSubmitted = true; j.EvidenceSubmittedAt = started; j.DetailAdded = true }},
		{"payment-approval-reminder", "customer", "approve-payment", func(j *Job) { j.CompletionApproved = true; j.CompletionApprovedAt = started }},
		{"checkout-reminder", "customer", "continue-payment", func(j *Job) { j.CompletionApproved = true; j.PaymentApprovedVersion = 1; j.PaymentApprovedAt = started }},
	}
	for _, tc := range tests {
		t.Run(tc.event, func(t *testing.T) {
			state := NewState()
			state.Job = Job{ID: "job", ScopeVersion: 1}
			tc.setup(&state.Job)
			waits := WaitingStates(state)
			if len(waits) != 1 {
				t.Fatalf("waits = %+v", waits)
			}
			wait := waits[0]
			if wait.EventType != tc.event || wait.ResponsibleRole != tc.role || wait.ResolutionAction != tc.action || wait.Deadline != "2026-10-11T10:00:00Z" || wait.Detail == "" {
				t.Fatalf("policy = %+v", wait)
			}
			if wait.Due(now.Add(-time.Second)) || !wait.Due(now) {
				t.Fatalf("deadline boundary is not deterministic: %+v", wait)
			}
		})
	}
}

func TestWaitingStatesStopForDecisionCancellationAndDispute(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 1, CompletionApproved: true, CompletionApprovedAt: "2026-10-10T10:00:00Z"}
	for _, stop := range []string{"cancelled", "paid", "dispute", "cancellation-review"} {
		t.Run(stop, func(t *testing.T) {
			current := state
			switch stop {
			case "cancelled":
				current.Cancelled = true
			case "paid":
				current.Job.Paid = true
			case "dispute":
				current.Job.Dispute = &DisputeCase{Status: "open"}
			case "cancellation-review":
				current.CancellationReview = &CancellationRequest{Status: "pending"}
			}
			if got := WaitingStates(current); len(got) != 0 {
				t.Fatalf("obsolete waits remain: %+v", got)
			}
		})
	}
	state.Job.PaymentApprovedVersion = 1
	if got := WaitingStates(state); len(got) != 0 {
		t.Fatalf("payment approval wait survived its decision: %+v", got)
	}
}

func TestWaitingStateReminderIdentityChangesWithScopeOrProposal(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 1, DateProposal: &DateProposal{ID: "first", Status: "pending", Actor: "professional", CreatedAt: "2026-10-10T10:00:00Z"}}
	first := WaitingStates(state)[0].EventKey(state.Job)
	state.Job.DateProposal.ID = "replacement"
	if next := WaitingStates(state)[0].EventKey(state.Job); next == first {
		t.Fatal("replacement proposal retained old reminder identity")
	}
	state.Job.DateProposal.ID = "first"
	state.Job.ScopeVersion++
	if next := WaitingStates(state)[0].EventKey(state.Job); next == first {
		t.Fatal("replacement scope retained old reminder identity")
	}
}
