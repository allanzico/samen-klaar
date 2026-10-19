package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDutchIntakeKeepsStableOptionMeaning(t *testing.T) {
	state := NewState()
	act := func(action, value string) {
		t.Helper()
		if err := Apply(&state, Action{Actor: "customer", Action: action, Value: value, Locale: "nl", Matches: testMatches("noor")}); err != nil {
			t.Fatal(err)
		}
	}
	act("start-intake", "Mijn kraan lekt onder de gootsteen")
	if state.Intake.Locale != "nl" || state.Intake.Category != "Plumbing" || !strings.Contains(state.Intake.Question, "water") || state.Intake.Options[0] != "Het water loopt · de leiding drupt eronder" {
		t.Fatalf("Dutch fallback intake=%+v", state.Intake)
	}
	if state.Intake.OptionIDs[0] != "intake:detail:water_runs_pipe_drips" {
		t.Fatalf("missing stable option identity: %+v", state.Intake.OptionIDs)
	}
	act("answer-intake", state.Intake.OptionIDs[0])
	if state.Intake.Question != "Wanneer wil je dat iemand komt kijken?" {
		t.Fatalf("timing question=%q", state.Intake.Question)
	}
	act("answer-intake", "intake:urgency:asap")
	if state.Intake.Urgency != "As soon as possible" || state.Intake.Options[2] != "Help me de kosten te begrijpen" {
		t.Fatalf("timing meaning or budget language changed: %+v", state.Intake)
	}
	act("answer-intake", "intake:budget:understand_cost")
	if state.Intake.Step != "location" || !strings.Contains(state.Intake.Question, "postcode") {
		t.Fatalf("location prompt=%+v", state.Intake)
	}
	act("set-location", "Amsterdam Noord")
	if state.Intake.Step != "matches" || !strings.HasPrefix(state.Intake.Question, "Kies een vakpersoon") || strings.Contains(state.Intake.Detail, "intake:") {
		t.Fatalf("localized intake lost routing or exposed answer identifiers: %+v", state.Intake)
	}
}

func TestIntakeOptionIDsStayTheSameAcrossLanguages(t *testing.T) {
	var ids []string
	for _, locale := range []string{"en", "nl"} {
		state := NewState()
		if err := Apply(&state, Action{Actor: "customer", Action: "start-intake", Value: "My sink pipe is dripping", Locale: locale}); err != nil {
			t.Fatal(err)
		}
		if locale == "en" {
			ids = state.Intake.OptionIDs
			continue
		}
		if strings.Join(ids, "|") != strings.Join(state.Intake.OptionIDs, "|") || len(ids) != 4 {
			t.Fatalf("display language changed option identity: en=%v nl=%v", ids, state.Intake.OptionIDs)
		}
	}
}

func TestDutchUrgencyAndStaleOptionDoNotDependOnEnglishLabels(t *testing.T) {
	state := NewState()
	for _, step := range []struct{ action, value string }{{"start-intake", "De leiding onder mijn kraan lekt"}, {"answer-intake", "Het water drupt uit de leiding"}, {"answer-intake", "Graag vandaag, het is dringend"}} {
		if err := Apply(&state, Action{Actor: "customer", Action: step.action, Value: step.value, Locale: "nl"}); err != nil {
			t.Fatal(err)
		}
	}
	if state.Intake.Urgency != "As soon as possible" {
		t.Fatalf("Dutch timing preference=%q", state.Intake.Urgency)
	}
	before, _ := json.Marshal(state)
	if err := Apply(&state, Action{Actor: "customer", Action: "answer-intake", Value: "intake:urgency:asap", Locale: "nl"}); err == nil {
		t.Fatal("stale option from previous question was accepted")
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("rejected stale answer changed intake")
	}
}
