package ai

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type modelFixture func(GenerationRequest) []byte

func (f modelFixture) GenerateJSON(_ context.Context, request GenerationRequest) ([]byte, error) {
	return f(request), nil
}

func TestEvidenceCoverageUsesWorkIDsAfterRepeatedPriceAdjustments(t *testing.T) {
	job := workflow.Job{Description: "Replace the damaged valve", Quote: workflow.Quote{Lines: []workflow.QuoteLine{
		{Label: "Replace valve", Basis: "Published service price"},
		{Label: "Call-out", Basis: "Published provider call-out fee"},
		{Label: "Agreed price adjustment", Basis: "Customer and professional agreed: first offer"},
		{Label: "Agreed price adjustment", Basis: "Customer and professional agreed: second offer"},
		{Label: "Replace valve", Basis: "Professional proposal · customer-approved change"},
	}}}
	files := []workflow.EvidenceBlob{{ID: "asset", ContentType: "image/png", Data: generatedReviewPNG(t, 64, 64)}}
	output := map[string]any{"summary": "Review", "limitations": "Images cannot prove quality", "coverage": []map[string]any{
		{"scopeItem": "work-1", "evidenceIds": []string{"asset"}, "coverage": "documented", "observation": "Valve visible"},
		{"scopeItem": "work-5", "evidenceIds": []string{"asset"}, "coverage": "unclear", "observation": "Second task unclear"},
	}}
	client := modelFixture(func(request GenerationRequest) []byte {
		properties := request.Schema["properties"].(map[string]any)
		finding := properties["coverage"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		itemIDs := finding["scopeItem"].(map[string]any)["enum"].([]string)
		fileIDs := finding["evidenceIds"].(map[string]any)["items"].(map[string]any)["enum"].([]string)
		if !slices.Equal(itemIDs, []string{"work-1", "work-5"}) || !slices.Equal(fileIDs, []string{"asset"}) {
			t.Errorf("response schema permits unexpected work or evidence references")
		}

		if !strings.Contains(request.Prompt, "work-1 (Replace valve)") || !strings.Contains(request.Prompt, "work-5 (Replace valve)") || strings.Contains(request.Prompt, "Agreed price adjustment") {
			t.Errorf("prompt did not isolate stable work items: %q", request.Prompt)
		}
		data, _ := json.Marshal(output)
		return data
	})
	coordinator := New("fixture", "fixture", client)
	if _, err := coordinator.ReviewEvidence(context.Background(), job, files); err != nil {
		t.Fatal(err)
	}
	output["coverage"].([]map[string]any)[1]["scopeItem"] = "work-1"
	if _, err := coordinator.ReviewEvidence(context.Background(), job, files); err == nil {
		t.Fatal("duplicate work item ID was accepted")
	}
	output["coverage"].([]map[string]any)[1]["scopeItem"] = "work-unknown"
	if _, err := coordinator.ReviewEvidence(context.Background(), job, files); err == nil {
		t.Fatal("unknown work item ID was accepted")
	}
}
