package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func intakeAnswer(t *testing.T, state *State, action, value string) {
	t.Helper()
	input := Action{Actor: "customer", Action: action, Value: value}
	if (action == "refresh-matches" || action == "set-location" || action == "answer-intake" && state.Intake != nil && state.Intake.Step == "budget") && state.Intake != nil {
		if state.Intake.Category == "Plumbing" {
			input.Matches = testMatches("noor", "jonas", "sam")
		}
		if state.Intake.Category == "Electrical assessment" {
			input.Matches = testMatches("eva")
		}
		if state.Intake.Category == "Carpentry" {
			input.Matches = testMatches("milan")
		}
	}
	if action == "select-match" && state.Intake != nil {
		input.CustomerName = "Test customer"
		input.AvailableSlots = []Appointment{{SlotID: "test-slot", StartsAt: "Test appointment", Timezone: "UTC", StartUTC: "2026-10-13T08:00:00Z", EndUTC: "2026-10-13T09:30:00Z"}}
		input.SelectedOffer = testOffer(value, state.Intake.Category)
	}
	if err := Apply(state, input); err != nil {
		t.Fatal(err)
	}
	if action == "select-match" && state.Job.Location != nil {
		if err := Apply(state, Action{Actor: "customer", Action: "confirm-professional", Value: state.Job.ID}); err != nil {
			t.Fatal(err)
		}
		if err := Apply(state, Action{Actor: "customer", Action: "set-service-address", Value: "Damrak 1, 1012 LG Amsterdam"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFinishedIntakeKeepsExactPostcodeProfessionalFirst(t *testing.T) {
	intake := &Intake{Category: "Plumbing", Urgency: "As soon as possible", Location: &Location{Label: "1020EN"}}
	finishIntake(intake, []Match{
		{ID: "postcode", Area: "AMSTERDAM CENTRUM 1020EN"},
		{ID: "city", Name: "City professional", Area: "Amsterdam", EarliestAppointment: &Appointment{StartUTC: "2026-10-18T09:00:00Z", StartsAt: "Sunday 09:00", Timezone: "Europe/Amsterdam"}},
	})
	if intake.Step != "matches" || intake.Matches[0].ID != "postcode" {
		t.Fatalf("exact-postcode professional lost its ranking: %+v", intake.Matches)
	}
	if !strings.Contains(intake.Messages[len(intake.Messages)-1].Text, "City professional") {
		t.Fatal("urgent guidance overlooked the earliest published appointment")
	}
}

func testMatches(ids ...string) []Match {
	profiles := map[string]Match{
		"jonas": {ID: "jonas", Name: "Jonas Vermeer", Category: "Plumbing", Area: "Amsterdam West", DistanceKM: 3.1},
		"noor":  {ID: "noor", Name: "Noor de Vries", Category: "Plumbing", Area: "Amsterdam Centrum", DistanceKM: 0.4},
		"sam":   {ID: "sam", Name: "Sam Bakker", Category: "Plumbing", Area: "Amsterdam Noord", DistanceKM: 4.2},
		"eva":   {ID: "eva", Name: "Eva Smit", Category: "Electrical assessment", Area: "Amsterdam Centrum", DistanceKM: 0.3},
		"milan": {ID: "milan", Name: "Milan Visser", Category: "Carpentry", Area: "Amsterdam West", DistanceKM: 2.9},
	}
	matches := make([]Match, 0, len(ids))
	for _, id := range ids {
		matches = append(matches, profiles[id])
	}
	return matches
}

func testOffer(profileID, category string) *CatalogOffer {
	services := map[string]CatalogOffer{
		"jonas": {ProfileID: "jonas", AccountID: "account-jonas", DisplayName: "Jonas Vermeer", Category: "Plumbing", ServiceName: "Sink leak repair", Currency: "USD", PriceMinor: 11200, CalloutMinor: 3500, DurationMinutes: 90},
		"noor":  {ProfileID: "noor", AccountID: "account-noor", DisplayName: "Noor de Vries", Category: "Plumbing", ServiceName: "Sink leak repair", Currency: "USD", PriceMinor: 11200, CalloutMinor: 3500, DurationMinutes: 90},
		"sam":   {ProfileID: "sam", AccountID: "account-sam", DisplayName: "Sam Bakker", Category: "Plumbing", ServiceName: "Sink leak repair", Currency: "USD", PriceMinor: 11200, CalloutMinor: 3500, DurationMinutes: 90},
		"eva":   {ProfileID: "eva", AccountID: "account-eva", DisplayName: "Eva Smit", Category: "Electrical assessment", ServiceName: "Electrical assessment", Currency: "USD", PriceMinor: 5500, CalloutMinor: 2500, DurationMinutes: 45},
		"milan": {ProfileID: "milan", AccountID: "account-milan", DisplayName: "Milan Visser", Category: "Carpentry", ServiceName: "Cupboard door alignment", Currency: "USD", PriceMinor: 9400, CalloutMinor: 3000, DurationMinutes: 75},
	}
	offer := services[profileID]
	if offer.Category != category {
		return nil
	}
	return &offer
}

func readyIntake(t *testing.T, description, detail, area string) State {
	t.Helper()
	state := NewState()
	intakeAnswer(t, &state, "start-intake", description)
	intakeAnswer(t, &state, "answer-intake", detail)
	intakeAnswer(t, &state, "answer-intake", "In the next few days")
	intakeAnswer(t, &state, "answer-intake", "Help me understand the cost")
	if state.Intake != nil && state.Intake.Step == "location" {
		if area == "" {
			area = "Amsterdam"
		}
		intakeAnswer(t, &state, "set-location", area)
	}
	return state
}
func TestConversationMatchesByAreaBeforeSharingAddress(t *testing.T) {
	state := readyIntake(t, "leaking sink", "Water runs · pipe drips underneath", "Amsterdam")
	if state.Intake.Category != "Plumbing" || len(state.Intake.Matches) != 3 {
		t.Fatalf("wrong matching: %+v", state.Intake)
	}
	if state.Intake.Matches[0].ID != "noor" {
		t.Fatal("closest suitable person was not first")
	}
	for _, match := range state.Intake.Matches {
		if match.Category != "Plumbing" || match.DistanceKM > 25 {
			t.Fatal("unsuitable match")
		}
	}
	intakeAnswer(t, &state, "select-match", "noor")
	if state.Job.Professional != "Noor de Vries" || state.Job.Location == nil || state.Job.ServiceAddress == "" || state.Job.Appointment != nil || state.Job.QuoteApproved {
		t.Fatal("selection lost context or booked without approval")
	}
	if err := apply(&state, "book-appointment", "customer", "slot-tue"); err == nil {
		t.Fatal("booked without approval")
	}
	intakeAnswer(t, &state, "set-booking-autonomy", "manual")
	intakeAnswer(t, &state, "approve-quote", "")
	intakeAnswer(t, &state, "book-appointment", "test-slot")
	if err := apply(&state, "start-intake", "customer", "Another leaking sink"); err == nil {
		t.Fatal("active booking replaced")
	}
}
func TestAssessmentRequestsContinueToMatching(t *testing.T) {
	for _, test := range []struct{ description, detail string }{
		{"industrial sink leaking in factory", "Water runs · pipe drips underneath"},
		{"leaking sink", "No water comes out"},
		{"leaking sink", "The drain is backing up"},
		{"leaking sink", "I'm not sure"},
		{"My cupboard door is stuck", "The wood or frame is damaged"},
	} {
		t.Run(test.description+test.detail, func(t *testing.T) {
			state := readyIntake(t, test.description, test.detail, "")
			if state.Intake.NeedsReview || state.Intake.Step != "matches" || len(state.Intake.Matches) == 0 {
				t.Fatalf("request was blocked by review: %+v", state.Intake)
			}
			intakeAnswer(t, &state, "select-match", state.Intake.Matches[0].ID)
			if state.Job.ID == "" || state.Job.Appointment != nil {
				t.Fatal("selection failed or booked without approval")
			}
		})
	}
}

func TestUnclearRequestAsksMoreThenMatchesWithoutRepeatingBudget(t *testing.T) {
	state := readyIntake(t, "Something is broken", "I don't know what it is", "")
	if state.Intake.NeedsReview || state.Intake.Step != "detail" || len(state.Intake.Matches) != 0 {
		t.Fatal("unclear request was sent to an operator")
	}
	urgency, budget := state.Intake.Urgency, state.Intake.Budget
	if err := Apply(&state, Action{Actor: "customer", Action: "answer-intake", Value: "It is the kitchen sink and water is leaking", Matches: testMatches("noor")}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "set-location", Value: "Amsterdam Centrum", Matches: testMatches("noor")}); err != nil {
		t.Fatal(err)
	}
	if state.Intake.Category != "Plumbing" || state.Intake.Step != "matches" || state.Intake.Urgency != urgency || state.Intake.Budget != budget || len(state.Intake.Matches) != 1 {
		t.Fatal("clarification did not retain the answers and move to matching")
	}
}

func TestAICategoryIsNotOverruledByNarrowKeywordFallback(t *testing.T) {
	state := NewState()
	suggestion := IntakeSuggestion{Category: "Plumbing", Route: "assessment", Question: "Where do you notice the water?", Options: []string{"At the appliance"}}
	if err := Apply(&state, Action{Actor: "customer", Action: "start-intake", Value: "My appliance is making a strange noise", Suggestion: &suggestion}); err != nil {
		t.Fatal(err)
	}
	intakeAnswer(t, &state, "answer-intake", "At the appliance")
	intakeAnswer(t, &state, "answer-intake", "As soon as possible")
	intakeAnswer(t, &state, "answer-intake", "I am flexible")
	intakeAnswer(t, &state, "set-location", "Amsterdam")
	if state.Intake.Step != "matches" || state.Intake.NeedsReview || state.Intake.Category != "Plumbing" {
		t.Fatal("valid AI category did not continue through area matching")
	}
}

func TestLocationRejectsStreetAddressAndUnrelatedActionsDoNotMutateState(t *testing.T) {
	state := readyIntake(t, "leaking sink", "Water runs · pipe drips underneath", "Amsterdam")
	if len(state.Intake.Matches) != 3 || state.Intake.Location == nil {
		t.Fatal("area matching did not finish")
	}
	before, _ := json.Marshal(state)
	for _, input := range []Action{
		{Actor: "customer", Action: "set-location", Value: "Damrak 1, Amsterdam"},
		{Actor: "customer", Action: "select-match", Value: "eva"},
		{Actor: "professional", Action: "refresh-matches"},
	} {
		if err := Apply(&state, input); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("rejected input changed state")
	}
}
func TestIntakeRoundTripAndElectricalAssessment(t *testing.T) {
	state := readyIntake(t, "An outlet feels warm", "One outlet", "Amsterdam")
	if state.Intake.Category != "Electrical assessment" || state.Intake.Matches[0].ID != "eva" {
		t.Fatal("electrical request routed incorrectly")
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	intakeAnswer(t, &restored, "select-match", "eva")
	if !strings.Contains(restored.Job.Quote.Lines[0].Label, "assessment") || restored.Job.Professional != "Eva Smit" {
		t.Fatal("assessment changed into repair")
	}
}
func TestTimingPreferencesAllowMatchingWithoutDispatch(t *testing.T) {
	for _, timing := range []string{"As soon as possible", "today please", "This is an emergency", "In the next few days", "I'm flexible"} {
		t.Run(timing, func(t *testing.T) {
			state := NewState()
			for _, step := range []struct{ action, value string }{{"start-intake", "My kitchen sink is leaking"}, {"answer-intake", "Water runs · pipe drips underneath"}, {"answer-intake", timing}, {"answer-intake", "My budget is EUR 120"}} {
				intakeAnswer(t, &state, step.action, step.value)
			}
			intakeAnswer(t, &state, "set-location", "Amsterdam")
			if state.Intake.NeedsReview || state.Intake.Step != "matches" || len(state.Intake.Matches) != 3 || state.Job.ID != "" || state.Job.Appointment != nil {
				t.Fatalf("timing blocked matching or dispatched a visit: %+v", state)
			}
			intakeAnswer(t, &state, "select-match", "noor")
			if state.Job.Appointment != nil || state.Job.QuoteApproved {
				t.Fatal("urgent selection booked without a date agreement")
			}
		})
	}
}

func TestSavedTimingOnlyReviewCanResume(t *testing.T) {
	state := readyIntake(t, "leaking sink", "Water runs · pipe drips underneath", "")
	state.Intake.Step, state.Intake.Urgency, state.Intake.NeedsReview = "review", "As soon as possible", true
	state.Intake.Matches = nil
	data, _ := json.Marshal(state)
	var restored State
	json.Unmarshal(data, &restored)
	intakeAnswer(t, &restored, "refresh-matches", "")
	if restored.Intake.Step != "matches" || restored.Intake.NeedsReview || len(restored.Intake.Matches) != 3 || restored.Intake.Budget != state.Intake.Budget || restored.Intake.Detail != state.Intake.Detail {
		t.Fatal("saved routine request could not resume with its answers")
	}
	complex := readyIntake(t, "industrial sink leaking", "Water runs · pipe drips underneath", "")
	complex.Intake.Step, complex.Intake.NeedsReview = "review", true
	intakeAnswer(t, &complex, "refresh-matches", "")
	if complex.Intake.Step != "matches" || complex.Intake.NeedsReview {
		t.Fatal("saved assessment request still requires operator review")
	}
}

func TestUrgentMatchingOrdersVisitsAndRequiresApproval(t *testing.T) {
	state := readyIntake(t, "leaking sink", "No water comes out", "")
	state.Intake.Urgency = "As soon as possible"
	early := Appointment{SlotID: "early", StartsAt: "Two days from now", Timezone: "UTC", StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339), EndUTC: time.Now().Add(49*time.Hour + 30*time.Minute).UTC().Format(time.RFC3339)}
	late := early
	late.SlotID = "late"
	late.StartUTC = time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	matches := testMatches("noor", "jonas", "sam")
	matches[0].EarliestAppointment = &late
	matches[1].EarliestAppointment = &early
	if err := Apply(&state, Action{Actor: "customer", Action: "refresh-matches", Matches: matches}); err != nil {
		t.Fatal(err)
	}
	if state.Intake.Matches[0].ID != "jonas" || !strings.Contains(state.Intake.Messages[len(state.Intake.Messages)-1].Text, "Two days from now") {
		t.Fatal("earliest actual visit was not surfaced")
	}
	offer := testOffer("jonas", "Plumbing")
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "jonas", SelectedOffer: offer, CustomerAccountID: offer.AccountID}); err == nil {
		t.Fatal("own service was selectable")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "jonas", SelectedOffer: offer, CustomerAccountID: "other-customer", AvailableSlots: []Appointment{early}}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "confirm-professional", Value: state.Job.ID}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "set-service-address", Value: "Damrak 1, 1012 LG Amsterdam"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.Appointment != nil || state.Job.QuoteApproved {
		t.Fatal("selection skipped customer approval")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "approve-and-book", Value: "expired-slot"}); err == nil || state.Job.QuoteApproved {
		t.Fatal("invalid visit partially approved the plan")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "approve-and-book", Value: early.SlotID}); err != nil {
		t.Fatal(err)
	}
	if !state.Job.QuoteApproved || state.Job.Appointment.SlotID != early.SlotID {
		t.Fatal("explicit approval did not confirm the plan and visit")
	}
}
