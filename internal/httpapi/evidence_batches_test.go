package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreai "github.com/allanzico/samen-klaar/internal/ai"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type evidenceStorageFixture map[string][]byte

func (s evidenceStorageFixture) Put(context.Context, string, string, string, []byte) error {
	return nil
}

type failedReviewFixture struct {
	intakeStub
	failure error
}

func (s *failedReviewFixture) ReviewEvidence(context.Context, workflow.Job, []workflow.EvidenceBlob) (*workflow.EvidenceReview, error) {
	return nil, s.failure
}

func TestPhotoComparisonFailureReturnsSafeCodeAndPreservesCompletion(t *testing.T) {
	for _, tc := range []struct {
		failure error
		status  int
		code    string
	}{
		{errors.Join(coreai.ErrEvidenceProvider, errors.New("private-provider-diagnostic")), http.StatusBadGateway, "EVIDENCE_COMPARISON_UNAVAILABLE"},
		{coreai.ErrEvidenceReviewInvalid, http.StatusBadGateway, "EVIDENCE_COMPARISON_UNAVAILABLE"},
		{coreai.ErrEvidenceImage, http.StatusUnprocessableEntity, "EVIDENCE_IMAGE_UNSUPPORTED"},
	} {
		state := workflow.NewState()
		state.Job = workflow.Job{ID: "comparison-fixture", EvidenceSubmitted: true, Evidence: []workflow.Evidence{{AssetID: "generated", ContentType: "image/png"}}}
		store := workflow.NewMemoryStore(state)
		handler := New(Dependencies{Store: store, Sessions: customerSession(), AppBaseURL: "http://example.test", AI: &failedReviewFixture{failure: tc.failure}, Evidence: evidenceStorageFixture{"generated": []byte("fixture")}}).Handler()
		r := httptest.NewRequest(http.MethodPost, "/api/evidence/review", strings.NewReader(`{"assetIds":["generated"]}`))
		addTestOrigin(r)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.code) || strings.Contains(response.Body.String(), "private-provider-diagnostic") {
			t.Fatalf("comparison response=%d %s", response.Code, response.Body.String())
		}
		retained, err := store.Load(context.Background(), "test-customer")
		if err != nil || !retained.Job.EvidenceSubmitted || len(retained.Job.Evidence) != 1 || retained.Job.CompletionApproved || retained.Job.EvidenceReview != nil {
			t.Fatal("failed comparison altered completion or evidence")
		}
	}
}
func (s evidenceStorageFixture) Get(_ context.Context, _, id string) (io.ReadCloser, string, error) {
	return io.NopCloser(bytes.NewReader(s[id])), "image/png", nil
}
func (s evidenceStorageFixture) Delete(context.Context, string, string) {}

func TestEvidenceReviewCanChooseBatchesAfterAcceptedUploads(t *testing.T) {
	job := workflow.Job{ID: "job"}
	storage := evidenceStorageFixture{}
	for _, id := range []string{"old-scope", "new-1", "new-2", "new-3", "new-4", "new-5"} {
		job.Evidence = append(job.Evidence, workflow.Evidence{AssetID: id, ContentType: "image/png", Label: "Evidence photo"})
		storage[id] = []byte("small image")
	}
	files, err := loadEvidenceReviewFiles(context.Background(), storage, job, nil)
	if err != nil || len(files) != 5 {
		t.Fatalf("automatic batch=%d, %v", len(files), err)
	}
	files, err = loadEvidenceReviewFiles(context.Background(), storage, job, map[string]bool{"old-scope": true})
	if err != nil || len(files) != 1 || files[0].ID != "old-scope" {
		t.Fatalf("prior-scope selection=%+v, %v", files, err)
	}
	job.Evidence = []workflow.Evidence{{AssetID: "large-1", ContentType: "image/png"}, {AssetID: "large-2", ContentType: "image/png"}}
	storage["large-1"], storage["large-2"] = make([]byte, 7<<20), make([]byte, 7<<20)
	files, err = loadEvidenceReviewFiles(context.Background(), storage, job, nil)
	if err != nil || len(files) != 1 {
		t.Fatalf("automatic size-bounded batch=%d, %v", len(files), err)
	}
	_, err = loadEvidenceReviewFiles(context.Background(), storage, job, map[string]bool{"large-1": true, "large-2": true})
	if err == nil || err.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized explicit batch status=%v", err)
	}
}
