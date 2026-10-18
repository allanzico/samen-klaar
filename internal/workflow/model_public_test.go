package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvidenceReviewJSONOmitsModelName(t *testing.T) {
	encoded, err := json.Marshal(EvidenceReview{Summary: "Files checked", Model: "private-model"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-model") || strings.Contains(string(encoded), "model") {
		t.Fatalf("evidence review exposed model details: %s", encoded)
	}
}
