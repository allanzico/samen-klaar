package matching

import (
	"context"
	"errors"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"strings"
	"testing"
	"unicode/utf8"
)

type testDirectory struct{ calls []string }

func (*testDirectory) ServiceCatalog(context.Context, string, string, string, ...workflow.Location) ([]Service, error) {
	return []Service{{Name: "Sink leak repair"}, {Name: "Toilet repair"}}, nil
}
func (d *testDirectory) ServiceMatches(_ context.Context, _ string, exclude string, service Service, _ ...workflow.Location) ([]workflow.Match, error) {
	d.calls = append(d.calls, service.Name)
	if exclude != "customer" {
		return nil, errors.New("missing self exclusion")
	}
	return []workflow.Match{{ID: "later", ServiceName: service.Name, EarliestAppointment: &workflow.Appointment{StartUTC: "2026-10-20T09:00:00Z"}}, {ID: "earlier", ServiceName: service.Name, EarliestAppointment: &workflow.Appointment{StartUTC: "2026-10-18T09:00:00Z"}}}, nil
}

type testSelector struct{ selection Selection }

func (s testSelector) SelectServices(context.Context, string, []Service) (Selection, error) {
	return s.selection, nil
}
func TestFitPrecedesAvailability(t *testing.T) {
	d := &testDirectory{}
	matches, question, err := Find(context.Background(), d, testSelector{Selection{Fits: []Fit{{Index: 0, Reason: "This service covers leaking sink connections."}}}}, "Plumbing", "Leaking sink", "customer")
	if err != nil || question != "" || len(matches) != 2 || matches[0].ID != "earlier" || matches[0].ServiceName != "Sink leak repair" || len(d.calls) != 1 || d.calls[0] != "Sink leak repair" {
		t.Fatalf("wrong service eligibility/ranking: %+v %q %v calls=%v", matches, question, err, d.calls)
	}
}
func TestRejectUnknownAndDuplicateModelChoices(t *testing.T) {
	for _, fits := range [][]Fit{{{Index: 99, Reason: "Unknown service must not be selected."}}, {{Index: 0, Reason: "Suitable sink service."}, {Index: 0, Reason: "Duplicate must not be selected."}}} {
		if _, _, err := Find(context.Background(), &testDirectory{}, testSelector{Selection{Fits: fits}}, "Plumbing", "Leaking sink", "customer"); err == nil {
			t.Fatal("invalid AI selection accepted")
		}
	}
}

func TestLongExplanationDoesNotDiscardEligibleService(t *testing.T) {
	for _, reason := range []string{
		strings.Repeat("This published service repairs accessible leaking sink connections. ", 6),
		strings.Repeat("Deze dienst repareert lekkende verbindingen — onderdelen na overleg. ", 6),
	} {
		directory := &testDirectory{}
		selection := testSelector{Selection{Fits: []Fit{{Index: 0, Reason: reason}}}}
		matches, question, err := Find(context.Background(), directory, selection, "Plumbing", "Leaking sink", "customer")
		if err != nil || question != "" || len(matches) != 2 {
			t.Fatalf("a long explanation discarded the eligible service: matches=%d question=%q err=%v", len(matches), question, err)
		}
		for _, match := range matches {
			if match.ServiceName != "Sink leak repair" || !utf8.ValidString(match.FitReason) || len([]rune(match.FitReason)) > 240 || !strings.HasSuffix(match.FitReason, "…") {
				t.Fatalf("expected the eligible service with a bounded, valid explanation: %+v", match)
			}
		}
	}
}

func TestFitExplanationTrimsWhitespaceAndRejectsBlankCopy(t *testing.T) {
	for _, reason := range []string{" \nThis service covers sink leaks.\t ", strings.Repeat(" ", 20)} {
		matches, _, err := Find(context.Background(), &testDirectory{}, testSelector{Selection{Fits: []Fit{{Index: 0, Reason: reason}}}}, "Plumbing", "Leaking sink", "customer")
		if strings.TrimSpace(reason) == "" {
			if err == nil {
				t.Fatal("blank explanation accepted")
			}
		} else if err != nil || len(matches) != 2 || matches[0].FitReason != strings.TrimSpace(reason) {
			t.Fatalf("explanation whitespace was not normalized: matches=%+v err=%v", matches, err)
		}
	}
}
func TestUnsupportedWorkAsksInsteadOfUnrelatedMatch(t *testing.T) {
	matches, q, err := Find(context.Background(), &testDirectory{}, testSelector{Selection{Question: "Which part of the sink is leaking?"}}, "Plumbing", "Not enough detail", "customer")
	if err != nil || len(matches) != 0 || q == "" {
		t.Fatalf("unexpected fallback: %v %q %v", matches, q, err)
	}
}

type postcodeDirectory struct{}

func (postcodeDirectory) ServiceCatalog(context.Context, string, string, string, ...workflow.Location) ([]Service, error) {
	return []Service{{Name: "Sink repair"}}, nil
}

func (postcodeDirectory) ServiceMatches(context.Context, string, string, Service, ...workflow.Location) ([]workflow.Match, error) {
	return []workflow.Match{
		{ID: "city-1", Area: "Amsterdam", EarliestAppointment: &workflow.Appointment{StartUTC: "2026-10-18T09:00:00Z"}},
		{ID: "city-2", Area: "Amsterdam", EarliestAppointment: &workflow.Appointment{StartUTC: "2026-10-19T09:00:00Z"}},
		{ID: "city-3", Area: "Amsterdam", EarliestAppointment: &workflow.Appointment{StartUTC: "2026-10-20T09:00:00Z"}},
		{ID: "postcode", Area: "AMSTERDAM CENTRUM 1020EN"},
	}, nil
}

func TestExactPostcodeProfessionalRemainsVisibleWhenOnlyThreeMatchesAreShown(t *testing.T) {
	matches, _, err := Find(context.Background(), postcodeDirectory{}, testSelector{Selection{Fits: []Fit{{Index: 0, Reason: "This service covers the requested sink repair."}}}}, "Plumbing", "Sink repair", "customer", workflow.Location{Label: "1020EN"})
	if err != nil || len(matches) != 3 || matches[0].ID != "postcode" {
		t.Fatalf("postcode professional omitted from visible matches: %+v, %v", matches, err)
	}
}
