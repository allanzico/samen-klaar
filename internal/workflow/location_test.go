package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func geoLocation(kind string, lat, lon float64) Location {
	return Location{ID: kind + "-fixture", Source: "mapbox", Kind: kind, Country: "nl", Label: "Amsterdam", City: "Amsterdam", Latitude: lat, Longitude: lon}
}

func TestGeographicCoverageIncludesAreaEdgesAndRequiresExactVisit(t *testing.T) {
	city := geoLocation("place", 52.37, 4.9)
	city.Bounds = []float64{4.7, 52.2, 5.1, 52.5}
	base := geoLocation("postcode", 52.45, 5.09)
	if !CoversLocation(base, 2, city) {
		t.Fatal("a professional serving the city edge was excluded by distance from its centre")
	}
	address := geoLocation("address", 52.37, 4.9)
	if CoversLocation(base, 2, address) {
		t.Fatal("a city result incorrectly established coverage at a distant address")
	}
	address.Longitude = 5.09
	address.Latitude = 52.45
	if !CoversLocation(base, 2, address) {
		t.Fatal("a nearby address was excluded")
	}
}

func TestMapboxAddressStaysOutOfSharedIntakeAndSurvivesSelection(t *testing.T) {
	state := readyIntake(t, "leaking sink", "Water runs · pipe drips underneath", "Amsterdam")
	l := geoLocation("address", 52.37, 4.9)
	l.Address = "Damrak 1, 1012 LG Amsterdam"
	l.Label = "1012 LG Amsterdam"
	l.Selection = "private-selection"
	if err := Apply(&state, Action{Actor: "customer", Action: "set-location", Location: &l, Matches: testMatches("noor")}); err != nil {
		t.Fatal(err)
	}
	if state.Intake.ServiceAddress != l.Address || state.Intake.Location.Address != "" || state.Intake.Location.Selection != "" {
		t.Fatal("address was lost or remained in the discovery location")
	}
	pro := ConversationForRole(state, "professional")
	raw, _ := json.Marshal(pro)
	if strings.Contains(string(raw), "Damrak") || strings.Contains(string(raw), "private-selection") {
		t.Fatal("private address leaked before assignment")
	}
	offer := testOffer("noor", "Plumbing")
	base := geoLocation("postcode", 52.37, 4.9)
	offer.BaseLocation = &base
	offer.TravelRadiusKM = 10
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "noor", SelectedOffer: offer}); err != nil {
		t.Fatal(err)
	}
	if state.Job.ServiceAddress != l.Address || state.Job.ServiceLocation == nil {
		t.Fatal("selected professional did not receive the exact visit address")
	}
}

func TestGeographicRankingUsesKnownDistances(t *testing.T) {
	l := geoLocation("address", 52.37, 4.9)
	matches := []Match{{ID: "unknown"}, {ID: "far", DistanceKnown: true, DistanceKM: 8}, {ID: "near", DistanceKnown: true, DistanceKM: 1}}
	OrderMatches(matches, &l)
	if matches[0].ID != "near" || matches[2].ID != "unknown" {
		t.Fatalf("wrong geographic ranking: %+v", matches)
	}
}

func TestLegacyCoverageIsRecheckedAgainstVerifiedAddress(t *testing.T) {
	l := geoLocation("address", 52.37, 4.9)
	l.Postcode = "1012 LG"
	if !ServiceAreaCoversLocation("Amsterdam", l) {
		t.Fatal("Amsterdam address lost legacy coverage")
	}
	l.City = "Rotterdam"
	l.Postcode = "3011 AA"
	l.Label = "3011 AA Rotterdam"
	if ServiceAreaCoversLocation("Amsterdam", l) {
		t.Fatal("customer changed cities without a coverage check")
	}
}
