package postgres

import (
	"crypto/rand"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestGeographicCatalogFiltersBeforeLimitAndKeepsCityEdgeResults(t *testing.T) {
	ctx, s := accountTestStore(t)
	category := "geo-" + rand.Text()
	profileIDs := []string{}
	for _, item := range []struct {
		lon    float64
		radius int
	}{{4.9, 5}, {5.09, 2}, {5.7, 2}} {
		id := accountFixture(t, ctx, s)
		if _, err := s.EnsureMembership(ctx, id, identity.Professional); err != nil {
			t.Fatal(err)
		}
		base := &workflow.Location{ID: rand.Text(), Source: "mapbox", Country: "nl", Kind: "postcode", Label: "Amsterdam", City: "Amsterdam", Latitude: 52.37, Longitude: item.lon}
		p, err := s.SaveProfessionalProfile(ctx, id, workflow.ProfessionalProfile{DisplayName: "Geo professional", Category: category, ServiceName: "Geo repair", ServiceDescription: "Repair fixture", ServiceArea: "Amsterdam", BaseLocation: base, TravelRadiusKM: item.radius, Currency: "EUR", PriceMinor: 9000, DurationMinutes: 60, MaterialsPolicy: "Separate", Exclusions: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ListProfessionalApplications(ctx); err != nil {
			t.Fatal(err)
		}
		if p.BaseLocation == nil || p.TravelRadiusKM != item.radius {
			t.Fatal("geographic profile did not round-trip")
		}
		if _, err := s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE id=$1`, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, id); err != nil {
			t.Fatal(err)
		}
		profileIDs = append(profileIDs, p.ID)
	}
	exact := workflow.Location{ID: "exact", Source: "mapbox", Country: "nl", Kind: "address", Label: "1012 LG Amsterdam", City: "Amsterdam", Postcode: "1012 LG", Latitude: 52.37, Longitude: 4.9}
	service := matching.Service{Name: "Geo repair", Description: "Repair fixture", Materials: "Separate", Exclusions: []string{}}
	matches, err := s.ServiceMatches(ctx, category, "customer", service, exact)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != profileIDs[0] || !matches[0].DistanceKnown {
		t.Fatalf("address coverage=%+v", matches)
	}
	city := exact
	city.Kind = "place"
	city.ID = "city"
	city.Label = "Amsterdam"
	city.Bounds = []float64{4.7, 52.2, 5.1, 52.5}
	matches, err = s.ServiceMatches(ctx, category, "customer", service, city)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[1].ID != profileIDs[1] || !matches[1].DistanceApproximate {
		t.Fatalf("city edge coverage=%+v", matches)
	}
	catalog, err := s.ServiceCatalog(ctx, category, "repair", "customer", city)
	if err != nil || len(catalog) != 1 {
		t.Fatalf("catalog=%+v error=%v", catalog, err)
	}
	// A broad city search cannot turn a distant address into a bookable visit.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	offer, err := s.CatalogOffer(ctx, profileIDs[1])
	if err != nil {
		t.Fatal(err)
	}
	job := workflow.Job{ProfessionalAccountID: offer.AccountID, ServiceAddress: "Damrak 1 Amsterdam", ServiceLocation: &exact}
	if err := validateVisitCoverageTx(ctx, tx, job); err == nil {
		t.Fatal("out-of-radius visit passed the transactional check")
	}
	job.ServiceLocation = nil
	if err := validateVisitCoverageTx(ctx, tx, job); err == nil {
		t.Fatal("unverified visit address passed the transactional check")
	}
}
