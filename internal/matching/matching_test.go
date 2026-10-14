package matching

import (
	"context"
	"errors"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"testing"
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
func TestUnsupportedWorkAsksInsteadOfUnrelatedMatch(t *testing.T) {
	matches, q, err := Find(context.Background(), &testDirectory{}, testSelector{Selection{Question: "Which part of the sink is leaking?"}}, "Plumbing", "Not enough detail", "customer")
	if err != nil || len(matches) != 0 || q == "" {
		t.Fatalf("unexpected fallback: %v %q %v", matches, q, err)
	}
}
