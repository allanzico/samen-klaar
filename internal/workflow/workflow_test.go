package workflow

import (
	"slices"
	"testing"
)

func apply(state *State, action, actor, value string) error {
	return Apply(state, Action{Action: action, Actor: actor, Value: value})
}

func TestQuoteAndAppointmentRequireCustomerApproval(t *testing.T) {
	state := NewDemoState("quote-ready")
	if err := apply(&state, "book-appointment", "customer", "slot-tue"); err == nil {
		t.Fatal("booking succeeded before quote approval")
	}
	if err := apply(&state, "approve-quote", "professional", ""); err == nil {
		t.Fatal("professional approved the customer's quote")
	}
	if err := apply(&state, "approve-quote", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "book-appointment", "customer", "not-a-slot"); err == nil {
		t.Fatal("booking accepted an unseeded appointment")
	}
	if err := apply(&state, "book-appointment", "customer", "slot-tue"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowFixturesKeepJobIdentitySeparate(t *testing.T) {
	scenarios := []string{"missing-docs", "quote-ready", "ready-review", "change-review", "carpentry", "electrical"}
	seen := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		id := NewDemoState(scenario).Job.ID
		if slices.Contains(seen, id) {
			t.Fatalf("scenario %q reused job identity %q", scenario, id)
		}
		seen = append(seen, id)
	}
}

func TestClarificationMustBeAnsweredBeforeQuoteApproval(t *testing.T) {
	state := NewDemoState("quote-ready")
	state.Job.NeedsClarification = true
	state.Job.ClarificationQuestion = "When does the sink leak?"
	if err := apply(&state, "approve-quote", "customer", ""); err == nil {
		t.Fatal("quote approved before clarification")
	}
	if err := apply(&state, "answer-clarification", "customer", "Only when the tap is running."); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "approve-quote", "customer", ""); err != nil {
		t.Fatal(err)
	}
}

func TestMissingEvidenceBlocksCompletionAndPayment(t *testing.T) {
	state := NewDemoState("missing-docs")
	if err := apply(&state, "approve-completion", "customer", ""); err == nil {
		t.Fatal("completion approved without requested detail")
	}
	if err := apply(&state, "approve-payment", "customer", ""); err == nil {
		t.Fatal("payment approved before completion")
	}
	if err := apply(&state, "request-details", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "request-details", "customer", ""); err == nil {
		t.Fatal("duplicate documentation request was accepted")
	}
	if err := apply(&state, "add-evidence", "professional", "Checked the connection after running water for two minutes."); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "approve-completion", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "confirm-payment", "customer", ""); err == nil {
		t.Fatal("payment confirmed without amount approval")
	}
	if err := apply(&state, "approve-payment", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "approve-payment", "customer", ""); err == nil {
		t.Fatal("duplicate approval for the same scope was accepted")
	}
	if err := apply(&state, "confirm-payment", "professional", ""); err == nil {
		t.Fatal("professional confirmed customer's payment")
	}
	if err := Apply(&state, Action{Action: "confirm-payment", Actor: "customer", PayPalCaptured: true, PaymentCaptureID: "capture_test_123"}); err != nil {
		t.Fatal(err)
	}
}

func TestScopeChangeVersionsPriceAndResetsApprovals(t *testing.T) {
	state := NewDemoState("ready-review")
	state.Job.CompletionApproved = true
	state.Job.PaymentApprovedVersion = 1
	if err := Apply(&state, Action{Action: "propose-change", Actor: "professional", Value: "Replace the worn valve", ChangeAmountMinor: 4800}); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "approve-payment", "customer", ""); err == nil {
		t.Fatal("payment accepted while scope change was pending")
	}
	if err := apply(&state, "approve-change", "customer", ""); err != nil {
		t.Fatal(err)
	}
	job := state.Job
	if job.ScopeVersion != 2 {
		t.Fatalf("scope version = %d, want 2", job.ScopeVersion)
	}
	if job.Quote.TotalMinor != 19500 {
		t.Fatalf("quote total = %d, want 19500", job.Quote.TotalMinor)
	}
	if len(job.ScopeHistory) != 2 || job.ScopeHistory[0].TotalMinor != 14700 {
		t.Fatalf("scope history did not retain v1: %#v", job.ScopeHistory)
	}
	if job.CompletionApproved || job.PaymentApprovedVersion != 0 {
		t.Fatal("prior completion/payment approval survived the scope change")
	}
	if err := apply(&state, "decline-change", "customer", ""); err == nil {
		t.Fatal("approved change was subsequently declined")
	}
}

func TestDeclinedScopeChangeLeavesVersionAndPriceUnchanged(t *testing.T) {
	state := NewDemoState("ready-review")
	if err := Apply(&state, Action{Action: "propose-change", Actor: "professional", Value: "Replace the worn valve", ChangeAmountMinor: 4800}); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "decline-change", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if state.Job.ScopeVersion != 1 || state.Job.Quote.TotalMinor != 14700 {
		t.Fatal("declined change altered the approved scope")
	}
}
