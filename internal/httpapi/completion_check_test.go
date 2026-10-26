package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	coreai "github.com/allanzico/samen-klaar/internal/ai"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type completionModelFixture func(coreai.GenerationRequest) []byte

func (f completionModelFixture) GenerateJSON(_ context.Context, request coreai.GenerationRequest) ([]byte, error) {
	return f(request), nil
}

func TestProfessionalCompletionAcceptsVerboseValidProviderFeedback(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	model := completionModelFixture(func(coreai.GenerationRequest) []byte {
		data, _ := json.Marshal(map[string]any{"summary": strings.Repeat("De foto documenteert de reparatie nog niet duidelijk genoeg. ", 20), "limitations": "A photo cannot establish workmanship or safety.", "coverage": []map[string]any{{"scopeItem": "work-1", "coverage": "unclear", "evidenceIds": []string{"generated"}, "observation": strings.Repeat("Beschrijf de uitgevoerde reparatie en controles in de werknotitie. ", 20)}}})
		return data
	})
	store := workflow.NewMemoryStore(professionalCompletionDraft())
	server := New(Dependencies{Store: store, AI: coreai.New("fixture", "fixture", model), Evidence: evidenceStorageFixture{"generated": imageBytes.Bytes()}})
	state, status, err := server.runAction(context.Background(), professionalPrincipal(), Action{Action: "submit-evidence", Actor: "professional"})
	if err != nil || status != http.StatusOK {
		t.Fatalf("valid correction feedback became an unavailable error: %d %v", status, err)
	}
	if state.Job.CompletionCheck == nil || state.Job.EvidenceSubmitted || state.Job.CompletionApproved || state.Job.Paid {
		t.Fatal("unclear findings must stay as professional correction feedback")
	}
}

func TestProfessionalCompletionFailureLogsOnlySafeStage(t *testing.T) {
	for _, tc := range []struct {
		cause error
		stage string
	}{{coreai.ErrEvidenceProvider, "provider"}, {coreai.ErrEvidenceReviewInvalid, "response"}, {coreai.ErrEvidenceImage, "image"}} {
		t.Run(tc.stage, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
			defer slog.SetDefault(previous)
			ai := &failedReviewFixture{failure: errors.Join(tc.cause, errors.New("private-provider-diagnostic"))}
			server := New(Dependencies{Store: workflow.NewMemoryStore(professionalCompletionDraft()), AI: ai, Evidence: evidenceStorageFixture{"generated": []byte("synthetic photo")}})
			_, status, err := server.runAction(context.Background(), professionalPrincipal(), Action{Action: "submit-evidence", Actor: "professional"})
			if err == nil || status != http.StatusBadGateway || !strings.Contains(output.String(), "stage="+tc.stage) {
				t.Fatalf("missing safe failure stage: %d %v", status, err)
			}
			if strings.Contains(output.String(), "private-provider-diagnostic") || strings.Contains(output.String(), "Public test address") || strings.Contains(err.Error(), "private-provider-diagnostic") {
				t.Fatal("diagnostics exposed provider or private job content")
			}
		})
	}
}

type completionCheckAI struct {
	intakeStub
	coverage string
	calls    int
	fail     bool
	during   func()
}

func (a *completionCheckAI) ReviewEvidence(_ context.Context, job workflow.Job, files []workflow.EvidenceBlob) (*workflow.EvidenceReview, error) {
	a.calls++
	if job.ServiceAddress != "" || job.Location != nil || job.ServiceLocation != nil {
		return nil, errors.New("private location was included")
	}
	if a.during != nil {
		a.during()
	}
	if a.fail {
		return nil, errors.New("private provider failure")
	}
	return &workflow.EvidenceReview{Summary: "Document the repaired connection and explain what was checked.", ReviewedAt: time.Now().UTC().Format(time.RFC3339), Coverage: []workflow.EvidenceFinding{{ScopeItem: "work-1", Coverage: a.coverage, Observation: "The work note does not explain the repair.", EvidenceIDs: []string{files[0].ID}}}}, nil
}
func professionalCompletionDraft() workflow.State {
	state := workflow.NewState()
	state.Job = workflow.Job{ID: "completion-fixture", ScopeVersion: 1, ProfessionalAccountID: "test-professional", QuoteApproved: true, WorkStarted: true, DetailAdded: true, ServiceAddress: "Public test address", Location: &workflow.Location{Label: "Test city"}, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 10000, Lines: []workflow.QuoteLine{{Label: "Repair the sink connection", AmountMinor: 10000}}}, Evidence: []workflow.Evidence{{ScopeVersion: 1, CompletionRound: 1, AssetID: "generated", ContentType: "image/png", Kind: "photo"}, {ScopeVersion: 1, CompletionRound: 1, Kind: "note", Description: "The work is complete."}}}
	return state
}
func professionalPrincipal() identity.Principal {
	return identity.Principal{User: identity.User{ID: "test-professional"}, ActiveRole: identity.Professional}
}

func TestProfessionalCompletionCheckStopsUnclearSubmission(t *testing.T) {
	store := workflow.NewMemoryStore(professionalCompletionDraft())
	ai := &completionCheckAI{coverage: "unclear"}
	server := New(Dependencies{Store: store, AI: ai, Evidence: evidenceStorageFixture{"generated": []byte("synthetic photo")}})
	state, status, err := server.runAction(context.Background(), professionalPrincipal(), Action{Action: "submit-evidence", Actor: "professional"})
	if err != nil || status != http.StatusOK {
		t.Fatalf("check failed: %d %v", status, err)
	}
	if state.Job.EvidenceSubmitted || ai.calls != 1 {
		t.Fatalf("unclear proof reached the customer: submitted=%v checks=%d", state.Job.EvidenceSubmitted, ai.calls)
	}
	if state.Job.CompletionCheck == nil || workflow.ConversationForRole(state, "customer").Job.CompletionCheck != nil || len(workflow.ConversationForRole(state, "customer").Job.Evidence) != 0 {
		t.Fatal("correction guidance must remain private to the professional")
	}
	// Reload and retry the same draft reuses feedback instead of spending another model call.
	state, _, err = server.runAction(context.Background(), professionalPrincipal(), Action{Action: "submit-evidence", Actor: "professional"})
	if err != nil || state.Job.EvidenceSubmitted || ai.calls != 1 {
		t.Fatal("unchanged draft bypassed the failed check or repeated the model call")
	}
	if _, _, err = server.runAction(context.Background(), professionalPrincipal(), Action{Action: "add-evidence", Actor: "professional", Value: "Replaced the leaking seal and checked the joint under running water."}); err != nil {
		t.Fatal(err)
	}
	ai.coverage = "documented"
	state, _, err = server.runAction(context.Background(), professionalPrincipal(), Action{Action: "submit-evidence", Actor: "professional"})
	if err != nil || !state.Job.EvidenceSubmitted || ai.calls != 2 || state.Job.EvidenceReview == nil {
		t.Fatalf("corrected record was not checked and sent: %v", err)
	}
	if state.Job.CompletionApproved || state.Job.Paid {
		t.Fatal("a professional check granted completion or payment approval")
	}
}

func TestProfessionalCompletionCheckFailsClosedAndRejectsStaleOrForgedChecks(t *testing.T) {
	for _, scenario := range []string{"provider-offline", "provider-error", "missing-note", "files-changed", "forged-check", "wrong-role"} {
		t.Run(scenario, func(t *testing.T) {
			draft := professionalCompletionDraft()
			if scenario == "missing-note" {
				draft.Job.DetailAdded = false
			}
			store := workflow.NewMemoryStore(draft)
			ai := &completionCheckAI{coverage: "documented", fail: scenario == "provider-error"}
			server := New(Dependencies{Store: store, AI: ai, Evidence: evidenceStorageFixture{"generated": []byte("synthetic photo")}})
			if scenario == "provider-offline" {
				server.ai = nil
			}
			if scenario == "files-changed" {
				ai.during = func() {
					_, err := store.Apply(context.Background(), "test-professional", Action{Action: "add-evidence", Actor: "professional", Value: "Another detail added while the check was running."})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			input := Action{Action: "submit-evidence", Actor: "professional"}
			principal := professionalPrincipal()
			if scenario == "forged-check" {
				input.Action = "record-completion-check"
				input.CompletionCheckVerified = true
			}
			if scenario == "wrong-role" {
				principal.ActiveRole = identity.Customer
				input.Actor = "customer"
			}
			_, status, err := server.runAction(context.Background(), principal, input)
			if err == nil || status < 400 {
				t.Fatalf("unsafe submission accepted: %d %v", status, err)
			}
			retained, _ := store.Load(context.Background(), "test-professional")
			if retained.Job.EvidenceSubmitted || retained.Job.CompletionApproved || retained.Job.Paid {
				t.Fatal("failed check sent or approved the record")
			}
		})
	}
}
