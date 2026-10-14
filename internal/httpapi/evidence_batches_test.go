package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type evidenceStorageFixture map[string][]byte

func (s evidenceStorageFixture) Put(context.Context, string, string, string, []byte) error {
	return nil
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
