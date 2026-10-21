package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type locationStub struct{ results []workflow.Location }

func (s locationStub) Search(context.Context, string, string, bool) ([]workflow.Location, error) {
	return s.results, nil
}
func testLocation() workflow.Location {
	return workflow.Location{ID: "mapbox-fixture", Source: "mapbox", Kind: "address", Country: "nl", Label: "1012 LG Amsterdam", City: "Amsterdam", Postcode: "1012 LG", Latitude: 52.37, Longitude: 4.9, Address: "Damrak 1, 1012 LG Amsterdam"}
}

func TestLocationSearchRequiresAccountAndSignsCanonicalSelection(t *testing.T) {
	server := New(Dependencies{Sessions: customerSession(), Locations: locationStub{[]workflow.Location{testLocation()}}})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/locations?q=Damrak", nil))
	if response.Code != 200 {
		t.Fatalf("search=%d %s", response.Code, response.Body.String())
	}
	var result struct {
		Locations []workflow.Location `json:"locations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	l := result.Locations[0]
	if l.Selection == "" {
		t.Fatal("unsigned location")
	}
	l.Latitude = 0
	l.Address = "invented address"
	canonical, err := server.selectedLocation("test-customer", &l)
	if err != nil || canonical.Latitude != 52.37 || canonical.Address == "invented address" {
		t.Fatal("client altered verified location")
	}
	if _, err := server.selectedLocation("another-account", &l); err == nil {
		t.Fatal("another account reused a selection")
	}
	l.Selection = strings.Replace(l.Selection, ".", ".tampered", 1)
	if _, err := server.selectedLocation("test-customer", &l); err == nil {
		t.Fatal("tampered signature accepted")
	}
	anon := New(Dependencies{Locations: locationStub{}})
	response = httptest.NewRecorder()
	anon.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/locations?q=Amsterdam", nil))
	if response.Code == 200 {
		t.Fatal("anonymous search bypassed account protection")
	}
}

func TestVisitAddressRejectsCityAndAcceptsSignedAddress(t *testing.T) {
	server := New(Dependencies{Locations: locationStub{[]workflow.Location{{Source: "mapbox", Kind: "place"}}}})
	if _, err := server.resolveVisitAddress(context.Background(), "customer", "Amsterdam", nil); err == nil {
		t.Fatal("city accepted as full visit address")
	}
	l := testLocation()
	l.Selection = server.signLocation("customer", l)
	if _, err := server.resolveVisitAddress(context.Background(), "customer", "ignored client text", &l); err != nil {
		t.Fatal(err)
	}
}

func TestAmbiguousVisitAddressNeedsSelectionAndAcceptsTheChosenResult(t *testing.T) {
	first := testLocation()
	second := first
	second.ID, second.Address = "another-address", "Damrak 10, 1012 LG Amsterdam"
	server := New(Dependencies{Locations: locationStub{[]workflow.Location{first, second}}})
	if _, err := server.resolveVisitAddress(context.Background(), "customer", first.Address, nil); err == nil {
		t.Fatal("an ambiguous address was guessed without the customer's selection")
	}
	first.Selection = server.signLocation("customer", first)
	chosen, err := server.resolveVisitAddress(context.Background(), "customer", first.Address, &first)
	if err != nil || chosen.Address != first.Address {
		t.Fatalf("the selected address did not resolve ambiguity: %+v %v", chosen, err)
	}
}

type geographicDirectoryStub struct {
	directoryStub
	base workflow.Location
}

func (g geographicDirectoryStub) CatalogOffer(ctx context.Context, id string) (workflow.CatalogOffer, error) {
	offer, err := g.directoryStub.CatalogOffer(ctx, id)
	offer.BaseLocation = &g.base
	offer.TravelRadiusKM = 5
	return offer, err
}

func TestGeocodedIntakeKeepsPrivateAddressThroughHTTPSelection(t *testing.T) {
	location := testLocation()
	base := location
	base.Kind = "postcode"
	base.Address = ""
	server := New(Dependencies{Store: workflow.NewMemoryStore(workflow.NewState()), Sessions: customerSession(), AppBaseURL: "http://example.test", AI: &intakeStub{}, Directory: geographicDirectoryStub{base: base}, Locations: locationStub{[]workflow.Location{location}}})
	handler := server.Handler()
	send := func(action, value string, l *workflow.Location, want int) workflow.State {
		t.Helper()
		data, _ := json.Marshal(Action{Action: action, Value: value, Location: l})
		req := httptest.NewRequest(http.MethodPost, "/api/actions", strings.NewReader(string(data)))
		addTestOrigin(req)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s: status=%d body=%s", action, response.Code, response.Body.String())
		}
		var state workflow.State
		if want == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
		}
		return state
	}
	send("start-intake", "leaking sink", nil, 200)
	send("answer-intake", "Water runs · pipe drips underneath", nil, 200)
	send("answer-intake", "In the next few days", nil, 200)
	send("answer-intake", "Help me understand the cost", nil, 200)
	send("set-location", "invented coordinates", &location, 400)
	location.Selection = server.signLocation("test-customer", location)
	state := send("set-location", "ignored client label", &location, 200)
	if state.Intake.Location.Label != location.Label || state.Intake.Location.Address != "" {
		t.Fatal("location result was not canonical or exposed the address in its public label")
	}
	state = send("select-match", state.Intake.Matches[0].ID, nil, 200)
	if state.Job.ServiceAddress != location.Address || state.Job.ServiceLocation == nil {
		t.Fatal("exact address was not carried into the selected job")
	}
	outside := location
	outside.Longitude = 5.5
	outside.Address = "Outside street 1"
	outside.Selection = server.signLocation("test-customer", outside)
	send("set-service-address", outside.Address, &outside, 409)
}
