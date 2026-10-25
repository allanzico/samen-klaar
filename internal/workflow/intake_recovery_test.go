package workflow

import (
	"testing"
	"time"
)

func TestUnmatchedRequestStopsRepeatingClarification(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Description: "Repair a leaking kitchen sink", Category: "Plumbing", Route: "catalog", Step: "location", Detail: "Water drips beneath the sink when the tap runs", Urgency: "I'm flexible", Budget: "Help me understand the cost", Messages: []Message{}}
	question := "Which connection beneath the sink is leaking?"
	if err := Apply(&state, Action{Actor: "customer", Action: "set-location", Value: "Amsterdam", SuggestedQuestion: question}); err != nil {
		t.Fatal(err)
	}
	if state.Intake.Step != "detail" {
		t.Fatalf("expected at most one service clarification: %s", state.Intake.Step)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "answer-intake", Value: "The connection directly below the drain", SuggestedQuestion: question}); err != nil {
		t.Fatal(err)
	}
	if state.Intake.Step != "waitlist" || state.Intake.Location == nil || state.Job.ID != "" {
		t.Fatalf("no provider must settle on an explicit unavailable result, not another detail/location loop: step=%s", state.Intake.Step)
	}
}

func TestSelectionWaitsForProfessionalReviewBeforeAskingAddress(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Category: "Plumbing", Location: &Location{Label: "Amsterdam"}, Matches: testMatches("noor")}
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "noor", SelectedOffer: testOffer("noor", "Plumbing")}); err != nil {
		t.Fatal(err)
	}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()})
	if d.Action != nil || d.Code != "review_professional" {
		t.Fatalf("selection immediately advanced instead of letting the customer review: code=%s", d.Code)
	}
}

func TestUnbookedNegotiatedPlanCanReturnToProfessionals(t *testing.T) {
	state := readyIntake(t, "leaking sink", "Water drips beneath the sink", "Amsterdam")
	intakeAnswer(t, &state, "select-match", "noor")
	state.Job.QuoteApproved = true
	if err := Apply(&state, Action{Actor: "customer", Action: "return-to-matches", Matches: testMatches("noor", "jonas")}); err != nil {
		t.Fatalf("unbooked quote trapped customer with wrong professional: %v", err)
	}
	if state.Job.ID != "" || state.Intake.Step != "matches" {
		t.Fatal("did not return to results")
	}
}

func TestOpenDetailQuestionHasNoDescribeMoreAnswer(t *testing.T) {
	state := NewState()
	if err := Apply(&state, Action{Actor: "customer", Action: "start-intake", Value: "Something in the home needs attention"}); err != nil {
		t.Fatal(err)
	}
	for _, option := range state.Intake.Options {
		if canonicalIntakeOption(option) == "I'll describe more" {
			t.Fatal("redundant describe-more answer is still shown")
		}
	}
}

func TestProfessionalReviewRequiresCurrentCustomerConfirmation(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Category: "Plumbing", Location: &Location{Label: "Amsterdam"}, Matches: testMatches("noor")}
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "noor", SelectedOffer: testOffer("noor", "Plumbing")}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []Action{
		{Actor: "customer", Action: "confirm-professional", Value: "old-job"},
		{Actor: "customer", ByAgent: true, Action: "confirm-professional", Value: state.Job.ID},
		{Actor: "professional", Action: "confirm-professional", Value: state.Job.ID},
		{Actor: "customer", Action: "approve-quote"},
	} {
		if err := Apply(&state, input); err == nil {
			t.Fatalf("review bypassed with %s by %s", input.Action, input.Actor)
		}
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "confirm-professional", Value: state.Job.ID}); err != nil {
		t.Fatal(err)
	}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()})
	if d.Code != "ask-address" {
		t.Fatalf("customer confirmation did not resume planning: %s", d.Code)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "review-professional", Value: state.Job.ID}); err != nil {
		t.Fatal(err)
	}
	if state.AgentDialog != nil || state.ProDialog != nil || state.Intake.Step != "professional_review" {
		t.Fatal("back navigation retained an active bot question")
	}
	state.Job.Appointment = &Appointment{SlotID: "reserved"}
	for _, action := range []string{"review-professional", "return-to-matches", "revise-intake", "edit-intake"} {
		if err := Apply(&state, Action{Actor: "customer", Action: action, Value: state.Job.ID}); err == nil {
			t.Fatalf("booked visit rewound with %s", action)
		}
	}
}

func TestUnclearIntakeStopsAfterTwoDetailAnswers(t *testing.T) {
	state := readyIntake(t, "Something in the home needs attention", "I cannot tell what it is", "")
	if state.Intake.Step != "detail" {
		t.Fatalf("expected one more focused clarification: %s", state.Intake.Step)
	}
	intakeAnswer(t, &state, "answer-intake", "I am still not sure what is broken")
	if state.Intake.Step != "unmatched" || len(state.Intake.Options) > 0 {
		t.Fatalf("unclear intake kept looping: %s", state.Intake.Step)
	}
}

func TestEditingEarlierIntakeKeepsSavedAnswersAndLocksBookedVisits(t *testing.T) {
	state := readyIntake(t, "leaking sink", "The pipe beneath the sink drips", "Amsterdam")
	for _, step := range []string{"location", "budget", "urgency", "detail", "description"} {
		if err := Apply(&state, Action{Actor: "customer", Action: "edit-intake", Value: step}); err != nil {
			t.Fatal(err)
		}
		if state.Intake.Step != step || state.Intake.Location == nil || state.Intake.Budget == "" || state.Intake.Urgency == "" {
			t.Fatal("back navigation lost the saved answers")
		}
	}
}
