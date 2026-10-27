package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type modelFixture func(GenerationRequest) []byte

func (f modelFixture) GenerateJSON(_ context.Context, request GenerationRequest) ([]byte, error) {
	return f(request), nil
}

func TestEvidenceReviewRetriesInvalidStructureOnceWithoutChangingEvidence(t *testing.T) {
	job := workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Replace the sink trap"}}}}
	files := []workflow.EvidenceBlob{{ID: "photo", ContentType: "image/png", Data: generatedReviewPNG(t, 32, 32)}}
	valid := `{"summary":"Replacement documented","coverage":[{"scopeItem":"work-1","evidenceIds":["photo"],"coverage":"documented","observation":"Replacement trap visible"}],"limitations":"Photos cannot verify workmanship or safety"}`
	for _, invalid := range []string{`{`, `{"summary":"Review","coverage":[]}`, strings.Replace(valid, `"work-1"`, `"unknown"`, 1), strings.Replace(valid, `["photo"]`, `["unknown"]`, 1)} {
		t.Run(invalid, func(t *testing.T) {
			calls := 0
			var firstRequest []byte
			client := modelFixture(func(request GenerationRequest) []byte {
				calls++
				encoded, _ := json.Marshal(request)
				if calls == 1 {
					firstRequest = encoded
					return []byte(invalid)
				}
				if string(firstRequest) != string(encoded) {
					t.Fatal("retry changed the evidence, scope or response constraints")
				}
				return []byte(valid)
			})
			review, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files)
			if err != nil || calls != 2 || review == nil || review.Coverage[0].Coverage != "documented" {
				t.Fatalf("invalid response was not recovered safely: calls=%d, err=%v", calls, err)
			}
		})
	}
}

func TestEvidenceReviewNeverRetriesAValidNegativeFinding(t *testing.T) {
	job := workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Replace the sink trap"}}}}
	files := []workflow.EvidenceBlob{{ID: "photo", ContentType: "image/png", Data: generatedReviewPNG(t, 32, 32)}}
	for _, coverage := range []string{"unclear", "not_shown"} {
		calls := 0
		client := modelFixture(func(GenerationRequest) []byte {
			calls++
			return []byte(fmt.Sprintf(`{"summary":"More evidence needed","coverage":[{"scopeItem":"work-1","evidenceIds":["photo"],"coverage":%q,"observation":"The repair is not documented"}]}`, coverage))
		})
		review, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files)
		if err != nil || calls != 1 || review.Coverage[0].Coverage != coverage {
			t.Fatalf("negative finding was retried or changed: calls=%d, err=%v", calls, err)
		}
	}
}

func TestEvidenceReviewStopsAfterTwoInvalidResponses(t *testing.T) {
	calls := 0
	client := modelFixture(func(GenerationRequest) []byte { calls++; return []byte(`{}`) })
	job := workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Replace the sink trap"}}}}
	files := []workflow.EvidenceBlob{{ID: "photo", ContentType: "image/png", Data: generatedReviewPNG(t, 32, 32)}}
	if review, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files); review != nil || !errors.Is(err, ErrEvidenceReviewInvalid) || calls != 2 {
		t.Fatalf("invalid responses must stay blocked after two attempts: calls=%d, err=%v", calls, err)
	}
}

type failingReviewModel struct{ calls int }

func (m *failingReviewModel) GenerateJSON(context.Context, GenerationRequest) ([]byte, error) {
	m.calls++
	return nil, context.DeadlineExceeded
}

func TestEvidenceReviewDoesNotRetryProviderFailure(t *testing.T) {
	client := &failingReviewModel{}
	job := workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Replace the sink trap"}}}}
	files := []workflow.EvidenceBlob{{ID: "photo", ContentType: "image/png", Data: generatedReviewPNG(t, 32, 32)}}
	_, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files)
	if client.calls != 1 || !errors.Is(err, ErrEvidenceProvider) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider failure was retried or hidden: calls=%d, err=%v", client.calls, err)
	}
}

func TestEvidenceReviewBoundsVerboseTextWithoutDiscardingValidFindings(t *testing.T) {
	job := workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Repair the sink connection"}}}}
	files := []workflow.EvidenceBlob{{ID: "generated", ContentType: "image/png", Data: generatedReviewPNG(t, 64, 64)}}
	for _, coverage := range []string{"unclear", "documented"} {
		t.Run(coverage, func(t *testing.T) {
			output := map[string]any{"summary": strings.Repeat("De foto toont een aansluiting, maar de reparatie is niet duidelijk. ", 20), "limitations": strings.Repeat("Een foto kan vakmanschap of veiligheid niet bevestigen. ", 20), "coverage": []map[string]any{{"scopeItem": "work-1", "evidenceIds": []string{"generated"}, "coverage": coverage, "observation": strings.Repeat("Voeg een duidelijkere foto en een beschrijving van de uitgevoerde controle toe. ", 20)}}}
			client := modelFixture(func(GenerationRequest) []byte { data, _ := json.Marshal(output); return data })
			review, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files)
			if err != nil {
				t.Fatalf("verbose valid findings were discarded: %v", err)
			}
			for _, text := range []string{review.Summary, review.Limitations, review.Coverage[0].Observation} {
				if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 500 {
					t.Fatal("display text is not bounded valid Unicode")
				}
			}
			if utf8.RuneCountInString(review.Coverage[0].Observation) > 400 || review.Coverage[0].Coverage != coverage || review.Coverage[0].ScopeItem != "work-1" || review.Coverage[0].EvidenceIDs[0] != "generated" {
				t.Fatal("bounding display text changed the evidence decision or references")
			}
			output["coverage"].([]map[string]any)[0]["scopeItem"] = "unknown"
			if _, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files); err == nil {
				t.Fatal("verbose output bypassed work-item validation")
			}
			finding := output["coverage"].([]map[string]any)[0]
			finding["scopeItem"] = "work-1"
			finding["evidenceIds"] = []string{"unknown-file"}
			if _, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files); err == nil {
				t.Fatal("verbose output bypassed file-reference validation")
			}
		})
	}
}

func TestEvidenceReviewTextLimitsCountCharactersInsteadOfUTF8Bytes(t *testing.T) {
	job := workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Repair the sink connection"}}}}
	files := []workflow.EvidenceBlob{{ID: "generated", ContentType: "image/png", Data: generatedReviewPNG(t, 32, 32)}}
	summary, observation := strings.Repeat("é", 500), strings.Repeat("界", 400)
	client := modelFixture(func(GenerationRequest) []byte {
		data, _ := json.Marshal(map[string]any{"summary": summary, "limitations": "Photos cannot establish workmanship or safety.", "coverage": []map[string]any{{"scopeItem": "work-1", "evidenceIds": []string{"generated"}, "coverage": "unclear", "observation": observation}}})
		return data
	})
	review, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), job, files)
	if err != nil || review.Summary != summary || review.Coverage[0].Observation != observation {
		t.Fatalf("text within character limits was discarded or damaged: %v", err)
	}
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
