package httpapi_test

import (
	"context"
	"os"
	"testing"
	"time"

	aiprovider "github.com/allanzico/samen-klaar/internal/adapters/ai"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

// Exercises the provider response through the same eligibility validation as a
// location search. All catalog and request text is synthetic; no accounts are used.
func TestLiveServiceMatching(t *testing.T) {
	if os.Getenv("RUN_LIVE_PROVIDER_CHECKS") != "1" {
		t.Skip("opt in with RUN_LIVE_PROVIDER_CHECKS=1 and local provider settings")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	selector, err := aiprovider.New(ctx, os.Getenv("AI_PROVIDER"), os.Getenv("AI_PROVIDER_KEY"), os.Getenv("AI_MODEL"), os.Getenv("AI_EFFORT"))
	if err != nil {
		t.Fatal("AI configuration failed")
	}
	matches, question, err := matching.Find(ctx, liveMatchingDirectory{}, selector, "Plumbing", "My kitchen sink is leaking underneath, but water still runs from the tap.", "synthetic-customer", workflow.Location{Label: "Amsterdam"})
	if err != nil {
		t.Fatalf("service matching failed: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != "Sink leak repair" || question != "" {
		t.Fatalf("expected the published sink repair, got %d matches and clarification=%t", len(matches), question != "")
	}
}

type liveMatchingDirectory struct{}

func (liveMatchingDirectory) ServiceCatalog(context.Context, string, string, string, ...workflow.Location) ([]matching.Service, error) {
	return []matching.Service{
		{Name: "Sink leak repair", Description: "Repair leaking connections under a kitchen sink.", Materials: "Replacement parts require approval.", Exclusions: []string{"Building-wide drains"}},
		{Name: "Toilet repair", Description: "Repair a running toilet cistern.", Materials: "Replacement parts require approval.", Exclusions: []string{"Sink repairs"}},
	}, nil
}

func (liveMatchingDirectory) ServiceMatches(_ context.Context, _, _ string, service matching.Service, _ ...workflow.Location) ([]workflow.Match, error) {
	return []workflow.Match{{ID: service.Name, ServiceName: service.Name}}, nil
}
