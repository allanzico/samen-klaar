package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type intakeStub struct {
	calls int
	fail  bool
}

type directoryStub struct{}

func (directoryStub) ListProfessionalMatches(context.Context, string, workflow.Location, string) ([]workflow.Match, error) {
	return []workflow.Match{{ID: "noor", Name: "Noor de Vries", Category: "Plumbing", Area: "Amsterdam", DistanceKM: 0.4}}, nil
}
func (directoryStub) CatalogOffer(context.Context, string) (workflow.CatalogOffer, error) {
	return workflow.CatalogOffer{ProfileID: "noor", AccountID: "account-noor", DisplayName: "Noor de Vries", Category: "Plumbing", ServiceName: "Sink leak repair", Currency: "USD", PriceMinor: 11200, CalloutMinor: 3500, DurationMinutes: 90}, nil
}
func (directoryStub) AvailableProfessionalSlots(context.Context, string, string) ([]workflow.Appointment, error) {
	return []workflow.Appointment{{SlotID: "test-slot", StartsAt: "Test appointment", Timezone: "UTC", StartUTC: "2026-10-13T08:00:00Z", EndUTC: "2026-10-13T09:30:00Z"}}, nil
}

type testSessions struct{ principal identity.Principal }

func (s *testSessions) Principal(*http.Request) (identity.Principal, bool) { return s.principal, true }

func customerSession() *testSessions {
	return &testSessions{principal: identity.Principal{User: identity.User{ID: "test-customer"}, ActiveRole: identity.Customer, Memberships: []identity.Membership{{Role: identity.Customer, Status: "active"}}}}
}

func addTestOrigin(request *http.Request) {
	request.Header.Set("Origin", "http://example.test")
}

func (s *intakeStub) Clarify(_ context.Context, _, description string) (workflow.IntakeSuggestion, error) {
	s.calls++
	if s.fail {
		return workflow.IntakeSuggestion{}, errors.New("intake temporarily unavailable")
	}
	return workflow.InferIntake(description), nil
}
func (*intakeStub) ReviewEvidence(context.Context, workflow.Job, []workflow.EvidenceBlob) (*workflow.EvidenceReview, error) {
	return nil, errors.New("not used")
}
func TestConversationHTTPRoundTrip(t *testing.T) {
	ai := &intakeStub{}
	sessions := customerSession()
	server := New(Dependencies{Store: workflow.NewMemoryStore(workflow.NewState()), Sessions: sessions, AppBaseURL: "http://example.test", AI: ai, Directory: directoryStub{}})
	handler := server.Handler()
	send := func(action, value string) State {
		t.Helper()
		body, _ := json.Marshal(Action{Actor: "customer", Action: action, Value: value})
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/actions", bytes.NewReader(body))
		addTestOrigin(request)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", action, response.Code, response.Body.String())
		}
		var state State
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	state := send("start-intake", "leaking sink")
	if state.Intake.Category != "Plumbing" || len(state.Intake.Options) == 0 {
		t.Fatal("AI suggestion did not reach response")
	}
	send("answer-intake", "Water runs · pipe drips underneath")
	send("answer-intake", "In the next few days")
	send("answer-intake", "Under $100")
	state = send("set-location", "Amsterdam Centrum")
	if len(state.Intake.Matches) == 0 {
		t.Fatal("no suitable match")
	}
	state = send("select-match", state.Intake.Matches[0].ID)
	state = send("confirm-professional", state.Job.ID)
	state = send("set-service-address", "Damrak 1, 1012 LG Amsterdam")
	send("set-booking-autonomy", "manual")
	send("approve-quote", "")
	send("book-appointment", "test-slot")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	var restored State
	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Job.Appointment == nil || restored.Intake.Location == nil || restored.Job.ServiceAddress == "" || restored.Job.Professional != "Noor de Vries" {
		t.Fatal("state reload lost booking or conversation")
	}
	if ai.calls != 2 {
		t.Fatal("expected initial routing and detail clarification calls")
	}
}
func TestIntakeFailurePreservesConversation(t *testing.T) {
	ai := &intakeStub{fail: true}
	sessions := customerSession()
	server := New(Dependencies{Store: workflow.NewMemoryStore(workflow.NewState()), Sessions: sessions, AppBaseURL: "http://example.test", AI: ai})
	serverSession := server.Handler()
	for _, item := range []struct {
		principal identity.Principal
		body      string
		calls     int
	}{
		{principal: identity.Principal{User: identity.User{ID: "test-professional"}, ActiveRole: identity.Professional, Memberships: []identity.Membership{{Role: identity.Professional, Status: "active"}}}, body: `{"actor":"customer","action":"start-intake","value":"leaking sink"}`, calls: 0},
		{principal: customerSession().principal, body: `{"actor":"professional","action":"start-intake","value":"leaking sink"}`, calls: 1},
	} {
		sessions.principal = item.principal
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/actions", bytes.NewBufferString(item.body))
		addTestOrigin(request)
		serverSession.ServeHTTP(response, request)
		if response.Code == http.StatusOK {
			t.Fatal("invalid/provider-failed action succeeded")
		}
		if ai.calls != item.calls {
			t.Fatalf("provider calls = %d, want %d", ai.calls, item.calls)
		}
	}
	if ai.calls != 1 {
		t.Fatal("browser-supplied actor bypassed account authorization")
	}
	state, err := server.currentState(context.Background(), "test-customer", "customer")
	if err != nil || state.Intake != nil {
		t.Fatal("failed suggestion mutated state")
	}
}

func (*intakeStub) SelectServices(context.Context, string, []matching.Service) (matching.Selection, error) {
	return matching.Selection{Fits: []matching.Fit{{Index: 0, Reason: "Published sink repair covers this leaking sink."}}}, nil
}
func (directoryStub) ServiceCatalog(context.Context, string, string, string, ...workflow.Location) ([]matching.Service, error) {
	return []matching.Service{{Name: "Sink leak repair"}}, nil
}
func (d directoryStub) ServiceMatches(ctx context.Context, category, exclude string, _ matching.Service, _ ...workflow.Location) ([]workflow.Match, error) {
	return d.ListProfessionalMatches(ctx, category, workflow.Location{}, exclude)
}

// A later uncertain routing response must not erase a trade already inferred
// from a clear free-form request and force the customer through location again.
type uncertainFollowupIntake struct{ intakeStub }

func (s *uncertainFollowupIntake) Clarify(_ context.Context, _, description string) (workflow.IntakeSuggestion, error) {
	s.calls++
	if s.calls == 1 {
		return workflow.InferIntake(description), nil
	}
	return workflow.IntakeSuggestion{Category: "Unclear", Route: "assessment", Question: "Where is the problem, and what happens?", Options: []string{"I'll describe more"}}, nil
}
func TestFreeFormIntakeFindsPlumberAfterOneLocationSearch(t *testing.T) {
	server := New(Dependencies{Store: workflow.NewMemoryStore(workflow.NewState()), Sessions: customerSession(), AppBaseURL: "http://example.test", AI: &uncertainFollowupIntake{}, Directory: directoryStub{}})
	var state State
	for _, step := range [][2]string{{"start-intake", "My kitchen sink is leaking"}, {"answer-intake", "Water drips from the connection beneath the basin when the tap runs"}, {"answer-intake", "Sometime next week"}, {"answer-intake", "Please show the available prices"}, {"set-location", "Amsterdam"}} {
		body, _ := json.Marshal(Action{Action: step[0], Value: step[1]})
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/actions", bytes.NewReader(body))
		addTestOrigin(request)
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", step[0], response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
	}
	if state.Intake.Step != "matches" || len(state.Intake.Matches) == 0 || state.Intake.Location == nil {
		t.Fatalf("free-form request needed another search: step=%s category=%s", state.Intake.Step, state.Intake.Category)
	}
}

type recoveryIntake struct {
	intakeStub
	noFit, selectorUnavailable bool
}

func (s *recoveryIntake) SelectServices(context.Context, string, []matching.Service) (matching.Selection, error) {
	if s.selectorUnavailable {
		return matching.Selection{}, errors.New("synthetic selector unavailable")
	}
	if s.noFit {
		return matching.Selection{Question: "Which connection beneath the sink needs repair?"}, nil
	}
	return matching.Selection{Fits: []matching.Fit{{Index: 0, Reason: "This service covers the described sink leak."}}}, nil
}
func TestUnmatchedFreeTextSettlesAndBackUsesSavedProfessionals(t *testing.T) {
	for _, noFit := range []bool{true, false} {
		t.Run(map[bool]string{true: "unavailable", false: "back-without-provider"}[noFit], func(t *testing.T) {
			ai := &recoveryIntake{noFit: noFit}
			server := New(Dependencies{Store: workflow.NewMemoryStore(workflow.NewState()), Sessions: customerSession(), AppBaseURL: "http://example.test", AI: ai, Directory: directoryStub{}})
			send := func(action, value string) State {
				t.Helper()
				body, _ := json.Marshal(Action{Action: action, Value: value})
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/api/actions", bytes.NewReader(body))
				addTestOrigin(request)
				server.Handler().ServeHTTP(response, request)
				if response.Code != 200 {
					t.Fatalf("%s: %d %s", action, response.Code, response.Body.String())
				}
				var state State
				if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
					t.Fatal(err)
				}
				return state
			}
			send("start-intake", "My kitchen sink is leaking")
			send("answer-intake", "The pipe beneath the sink drips when the tap runs")
			send("answer-intake", "Next week")
			send("answer-intake", "Show the available prices")
			state := send("set-location", "Amsterdam")
			if noFit {
				if state.Intake.Step != "detail" {
					t.Fatalf("expected one focused follow-up, got %s", state.Intake.Step)
				}
				state = send("answer-intake", "The drain connection right below the basin leaks")
				if state.Intake.Step != "waitlist" || state.Intake.Location == nil || state.Job.ID != "" {
					t.Fatal("no-match request returned to another address/detail question")
				}
			} else {
				state = send("select-match", state.Intake.Matches[0].ID)
				ai.selectorUnavailable = true
				state = send("return-to-matches", "")
				if state.Intake.Step != "matches" || len(state.Intake.Matches) != 1 || state.Job.ID != "" {
					t.Fatal("back navigation lost the professional results")
				}
			}
			reload := httptest.NewRecorder()
			server.Handler().ServeHTTP(reload, httptest.NewRequest(http.MethodGet, "/api/state", nil))
			var restored State
			if reload.Code != 200 || json.Unmarshal(reload.Body.Bytes(), &restored) != nil || restored.Intake.Step != state.Intake.Step || restored.Intake.Location == nil {
				t.Fatal("reload lost the terminal search/navigation state")
			}
		})
	}
}
