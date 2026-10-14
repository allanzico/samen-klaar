package postgres

import (
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestTradeChangeWithdrawsPublishedProfessionalOffer(t *testing.T) {
	ctx, s := accountTestStore(t)
	id := accountFixture(t, ctx, s)
	if _, err := s.EnsureMembership(ctx, id, identity.Professional); err != nil {
		t.Fatal(err)
	}
	input := workflow.ProfessionalProfile{DisplayName: "Approved provider", Category: "Plumbing", ServiceName: "Sink repair", ServiceDescription: "Repair a sink", ServiceArea: "Amsterdam", Currency: "EUR", PriceMinor: 10000, DurationMinutes: 60, MaterialsPolicy: "Parts quoted separately", Exclusions: []string{}}
	profile, err := s.SaveProfessionalProfile(ctx, id, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE id=$1`, profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, id); err != nil {
		t.Fatal(err)
	}
	input.PriceMinor = 11000
	priceEdit, err := s.SaveProfessionalProfile(ctx, id, input)
	if err != nil || priceEdit.Status != "active" {
		t.Fatalf("price edit status=%q, %v", priceEdit.Status, err)
	}
	input.Category = "Electrical assessment"
	input.ServiceName = "Electrical safety assessment"
	tradeEdit, err := s.SaveProfessionalProfile(ctx, id, input)
	if err != nil || tradeEdit.Status != "pending" {
		t.Fatalf("trade edit status=%q, %v", tradeEdit.Status, err)
	}
	if _, err := s.CatalogOffer(ctx, profile.ID); err == nil {
		t.Fatal("changed trade remained publicly bookable")
	}
	applications, err := s.ListProfessionalApplications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, application := range applications {
		if application.Profile.ID == profile.ID {
			found = true
			if application.Checks.Qualification != "pending" || application.Checks.Insurance != "pending" {
				t.Fatal("trade change kept old approval checks")
			}
		}
	}
	if !found {
		t.Fatal("changed trade did not enter operator review queue")
	}
}
