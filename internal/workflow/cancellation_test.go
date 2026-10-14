package workflow

import (
	"testing"
	"time"
)

func TestWithdrawalsRequireProposerAndCurrentID(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", Quote: Quote{TotalMinor: 10000}, PriceProposal: &PriceProposal{ID: "offer", Actor: "customer", Status: "pending"}}
	for _, a := range []Action{{Action: "withdraw-price", Actor: "professional", ProposalID: "offer"}, {Action: "withdraw-price", Actor: "customer", ProposalID: "stale"}} {
		if err := Apply(&state, a); err == nil {
			t.Fatal("unauthorized or stale offer withdrawn")
		}
	}
	if err := Apply(&state, Action{Action: "withdraw-price", Actor: "customer", ProposalID: "offer"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.PriceProposal.Status != "withdrawn" || state.Job.Quote.TotalMinor != 10000 {
		t.Fatal("withdrawal changed price")
	}
	state.Job.QuoteApproved = true
	state.Job.Appointment = &Appointment{SlotID: "booked"}
	state.Job.DateProposal = &DateProposal{ID: "date", Actor: "professional", Status: "pending"}
	if err := Apply(&state, Action{Action: "withdraw-date", Actor: "customer", ProposalID: "date"}); err == nil {
		t.Fatal("other party withdrew visit")
	}
	if err := Apply(&state, Action{Action: "withdraw-date", Actor: "professional", ProposalID: "date"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.Appointment.SlotID != "booked" {
		t.Fatal("withdrawal released existing booking")
	}
}
func TestCancellationPreservesHistoryAndBlocksFurtherCommitments(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", Description: "Sink repair", Appointment: &Appointment{SlotID: "booked", StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)}, QuoteApproved: true, DateProposal: &DateProposal{ID: "date", Actor: "customer", Status: "pending"}}
	state.Intake = &Intake{Messages: []Message{{Role: "customer", Text: "Please repair the sink"}}}
	if err := Apply(&state, Action{Action: "cancel-request", Actor: "customer", Value: "No longer needed"}); err != nil {
		t.Fatal(err)
	}
	if !state.Cancelled || state.Job.DateProposal.Status != "withdrawn" || state.Job.Appointment == nil || len(state.Intake.Messages) != 1 {
		t.Fatal("cancellation lost history or left a pending proposal")
	}
	for _, action := range []string{"approve-quote", "approve-and-book", "book-appointment", "start-job", "confirm-payment", "revise-intake"} {
		if err := Apply(&state, Action{Action: action, Actor: "customer"}); err == nil {
			t.Fatalf("cancelled request accepted %s", action)
		}
	}
	if err := Apply(&state, Action{Action: "send-message", Actor: "customer", Value: "Thank you for understanding."}); err != nil {
		t.Fatal(err)
	}
	code, _ := RequestStatus(state, "customer")
	if code != "cancelled" {
		t.Fatal("cancelled status not shown")
	}
}

func TestLateCancellationWaitsForOperatorAndKeepsVisitBooked(t *testing.T) {
	state := NewState()
	appointment := &Appointment{SlotID: "booked", StartUTC: time.Now().Add(23 * time.Hour).UTC().Format(time.RFC3339)}
	state.Job = Job{ID: "job", Appointment: appointment, QuoteApproved: true}
	if err := Apply(&state, Action{Action: "cancel-request", Actor: "customer", Value: "Plans changed"}); err != nil {
		t.Fatal(err)
	}
	if state.Cancelled || state.CancellationReview == nil || state.CancellationReview.Status != "pending" || state.Job.Appointment.SlotID != "booked" {
		t.Fatal("late cancellation did not preserve the visit for operator review")
	}
	if err := Apply(&state, Action{Action: "start-job", Actor: "professional"}); err == nil {
		t.Fatal("professional advanced a visit while its cancellation was under review")
	}
	code, _ := RequestStatus(state, "customer")
	if code != "cancellation_review" {
		t.Fatalf("late cancellation status = %q", code)
	}
}

func TestCancellationAtLeast24HoursBeforeVisitIsImmediate(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", Appointment: &Appointment{SlotID: "booked", StartUTC: time.Now().Add(25 * time.Hour).UTC().Format(time.RFC3339)}, QuoteApproved: true}
	if err := Apply(&state, Action{Action: "cancel-request", Actor: "customer"}); err != nil {
		t.Fatal(err)
	}
	if !state.Cancelled || state.CancellationReview != nil {
		t.Fatal("early cancellation should complete without operator review")
	}
}

func TestProfessionalCancellationLetsCustomerReuseRequestForMatching(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Description: "Repair a leaking kitchen sink", Category: "Plumbing", Location: &Location{Label: "Amsterdam West", Source: "manual"}, Matches: []Match{}}
	state.Job = Job{ID: "assigned-job", Title: "Sink repair", QuoteApproved: true, Appointment: &Appointment{SlotID: "reserved"}}
	if err := Apply(&state, Action{Action: "cancel-request", Actor: "professional"}); err != nil {
		t.Fatal(err)
	}
	if !state.Cancelled || state.CancelledBy != "professional" {
		t.Fatal("professional cancellation was not recorded")
	}
	if err := Apply(&state, Action{Action: "return-to-matches", Actor: "customer", Matches: []Match{{ID: "replacement", Name: "A professional", ServiceName: "Sink repair"}}}); err != nil {
		t.Fatalf("customer could not resume matching after professional cancellation: %v", err)
	}
	if state.Cancelled || state.Job.ID != "" || state.Intake.Step != "matches" || len(state.Intake.Matches) != 1 {
		t.Fatalf("request was not returned to matching: %+v", state)
	}
}

func TestCustomerCancellationCannotResumeMatching(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Description: "Repair a leaking kitchen sink"}
	state.Job = Job{ID: "assigned-job"}
	if err := Apply(&state, Action{Action: "cancel-request", Actor: "customer"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Action: "return-to-matches", Actor: "customer"}); err == nil {
		t.Fatal("customer-cancelled request unexpectedly reopened")
	}
}
func TestCancellationRejectsWorkAndPaymentStates(t *testing.T) {
	for _, job := range []Job{{WorkStarted: true}, {EvidenceSubmitted: true}, {CompletionApproved: true}, {Paid: true}, {PaymentApprovedVersion: 1}} {
		state := NewState()
		state.Job = job
		if err := Apply(&state, Action{Action: "cancel-request", Actor: "customer"}); err == nil || state.Cancelled {
			t.Fatal("active work or payment cancellation allowed")
		}
	}
}
func TestSummaryNamesTheActualNextActor(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", Title: "Sink repair", Professional: "Noor", Customer: "Sam", QuoteApproved: true, DateProposal: &DateProposal{Actor: "customer", Status: "pending"}}
	customer := SummarizeWorkspace(state, "customer", time.Time{})
	if customer.Status != "waiting_date" || customer.StatusLabel != "Waiting for Noor" {
		t.Fatalf("wrong status: %+v", customer)
	}
	professional := SummarizeWorkspace(state, "professional", time.Time{})
	if professional.Status != "review_date" {
		t.Fatal("professional did not receive review state")
	}
}
