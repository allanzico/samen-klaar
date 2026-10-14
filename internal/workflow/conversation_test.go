package workflow

import "testing"

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
func TestCorrectionKeepsPreferencesAndProtectsApprovedPlans(t *testing.T) {
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
	if err := Apply(&s, action); err == nil {
		t.Fatal("replaced an approved plan")
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
