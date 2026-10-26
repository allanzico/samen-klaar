package workflow

import (
	"encoding/json"
	"testing"
)

func projectedFields(t *testing.T, state State) map[string]any {
	t.Helper()
	data, err := json.Marshal(ConversationForRole(state, "customer"))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCustomerSeesPersonalPriceReviewWithoutPrivateNegotiationLimits(t *testing.T) {
	s := NewState()
	s.Intake = &Intake{Step: "plan_review", Messages: []Message{{Audience: "professional", Text: "Private price limit"}}}
	s.Job.PriceProposal = &PriceProposal{ID: "offer", Actor: "customer", Status: "pending", AmountMinor: 7500}
	s.ProDialog = &AgentDialog{Stage: "confirm", Awaiting: true, Pending: "accept-price", PendingID: "offer"}
	view := projectedFields(t, s)
	proposal := view["job"].(map[string]any)["priceProposal"].(map[string]any)
	if proposal["professionalReviewRequired"] != true {
		t.Fatal("customer cannot see that the professional must answer personally")
	}
	if view["proDialog"] != nil || len(ConversationForRole(s, "customer").Intake.Messages) != 0 {
		t.Fatal("private professional conversation was exposed")
	}
	// Reload, replacement proposals and settled proposals must not reuse an old wait reason.
	for _, change := range []func(*State){
		func(s *State) { s.Job.PriceProposal.Status = "accepted" },
		func(s *State) { s.ProDialog.PendingID = "previous" },
		func(s *State) { s.Job.PriceProposal.Actor = "professional" },
	} {
		copied := s
		proposal := *s.Job.PriceProposal
		copied.Job.PriceProposal = &proposal
		dialog := *s.ProDialog
		copied.ProDialog = &dialog
		change(&copied)
		p := projectedFields(t, copied)["job"].(map[string]any)["priceProposal"].(map[string]any)
		if p["professionalReviewRequired"] == true {
			t.Fatal("stale professional wait status")
		}
	}
	s.Intake = nil
	if ConversationForRole(s, "customer").ProDialog != nil || s.ProDialog == nil {
		t.Fatal("a legacy request must hide the private dialog without changing stored state")
	}
}

func TestCompletionAutomationProjectionExplainsCurrentEvidenceOutcome(t *testing.T) {
	s := autonomousFixture(t)
	s.Job.EvidenceReview = nil
	if got := projectedFields(t, s)["completionAutomation"]; got != "checking" {
		t.Fatalf("pending check shown as %v", got)
	}
	s = autonomousFixture(t)
	s.Job.EvidenceReview.Coverage[0].Coverage = "unclear"
	if got := projectedFields(t, s)["completionAutomation"]; got != "needs_review" {
		t.Fatalf("unclear evidence shown as %v", got)
	}
	// A new completion record requires a new check, not the previous manual fallback.
	s.Job.Evidence[0].AssetID = "new-photo"
	if got := projectedFields(t, s)["completionAutomation"]; got != "checking" {
		t.Fatalf("new evidence shown as %v", got)
	}
	s = autonomousFixture(t)
	s.Job.CompletionApproved = true
	if got := projectedFields(t, s)["completionAutomation"]; got != "releasing" {
		t.Fatalf("approved documented evidence shown as %v", got)
	}
	s.Job.BookingAutonomy.Mode = "manual"
	s.CompletionAutomation = "releasing" // A saved display value is never authoritative.
	if got := projectedFields(t, s)["completionAutomation"]; got != nil {
		t.Fatalf("manual review shown as %v", got)
	}
}

func TestAssignedMessagesDoNotApproveOrBook(t *testing.T) {
	s := NewState()
	s.Job.ID = "job"
	s.Intake = &Intake{Step: "selected"}
	for _, actor := range []string{"customer", "professional"} {
		if err := Apply(&s, Action{Action: "send-message", Actor: actor, Value: "Could we agree a price of 80 euros?"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Intake.Messages) != 2 || s.Job.QuoteApproved || s.Job.Appointment != nil || s.Intake.Messages[1].Role != "professional" {
		t.Fatalf("message changed agreement: %+v", s)
	}
}
func TestCorrectionKeepsPreferencesAndProtectsBookedPlans(t *testing.T) {
	s := NewState()
	s.Intake = &Intake{Step: "selected", Category: "Plumbing", Description: "Sink leak", Urgency: "As soon as possible", Budget: "100 euros"}
	s.Job.ID = "job"
	suggestion := InferIntake("My drain is blocked")
	action := Action{Action: "revise-intake", Actor: "customer", Value: "My drain is blocked", Suggestion: &suggestion}
	if err := Apply(&s, action); err != nil {
		t.Fatal(err)
	}
	if s.Job.ID != "" || s.Intake.Urgency != "As soon as possible" || s.Intake.Budget != "100 euros" || s.Intake.Step != "detail" {
		t.Fatalf("correction lost preferences: %+v", s)
	}
	s.Job.ID = "job"
	s.Job.QuoteApproved = true
	s.Job.Appointment = &Appointment{SlotID: "reserved"}
	if err := Apply(&s, action); err == nil {
		t.Fatal("replaced a booked plan")
	}
}

func TestEmptyExclusionsRemainAnArrayInSelectedPlan(t *testing.T) {
	s := NewState()
	s.Intake = &Intake{Step: "matches", Category: "Plumbing", Description: "Leaking sink", Matches: []Match{{ID: "profile", Category: "Plumbing"}}}
	offer := CatalogOffer{ProfileID: "profile", AccountID: "professional", Category: "Plumbing", DisplayName: "Professional", ServiceName: "Sink leak repair", Currency: "EUR", PriceMinor: 10000, DurationMinutes: 60}
	if err := Apply(&s, Action{Action: "select-match", Actor: "customer", Value: "profile", SelectedOffer: &offer}); err != nil {
		t.Fatal(err)
	}
	if s.Job.Quote.Exclusions == nil || s.AvailableSlots == nil {
		t.Fatal("empty arrays became null and would break plan rendering")
	}
}

func TestNewProfessionalDoesNotSeePreviousProfessionalThread(t *testing.T) {
	state := NewState()
	state.Job.ProfessionalProfileID = "new"
	state.Intake = &Intake{Messages: []Message{{Role: "customer", Text: "Original job"}, {Role: "professional", Text: "Private previous discussion", SenderName: "Previous", ThreadProfileID: "previous"}, {Role: "customer", Text: "Current discussion", ThreadProfileID: "new"}}}
	visible := ConversationForRole(state, "professional")
	if len(visible.Intake.Messages) != 2 || len(state.Intake.Messages) != 3 {
		t.Fatal("conversation isolation lost or modified stored history")
	}
	if len(ConversationForRole(state, "customer").Intake.Messages) != 3 {
		t.Fatal("customer lost discussion history")
	}
}

func TestClarificationCanCorrectAnAlreadyInferredTrade(t *testing.T) {
	suggestion := IntakeSuggestion{Category: "Carpentry", Route: "catalog", Question: "Is the wooden door catching on its frame?", Options: []string{"The door catches"}}
	if got := ClarifiedCategory(Intake{Category: "Plumbing"}, "Actually a wooden door is stuck", &suggestion); got != "Carpentry" {
		t.Fatalf("stale trade retained: %s", got)
	}
}
