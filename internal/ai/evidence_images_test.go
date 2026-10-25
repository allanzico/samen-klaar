package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/png"
	"math/rand/v2"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func generatedReviewPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	random := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < len(canvas.Pix); i += 4 {
		canvas.Pix[i] = byte(random.Uint32())
		canvas.Pix[i+1] = byte(random.Uint32())
		canvas.Pix[i+2] = byte(random.Uint32())
		canvas.Pix[i+3] = 255
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestEvidenceReviewFitsAcceptedLargePhotoWithinProviderLimits(t *testing.T) {
	original := generatedReviewPNG(t, 2000, 1400)
	if len(original) <= maxReviewImageBytes || len(original) > 10<<20 {
		t.Fatal("fixture must fit upload limit but exceed review limit")
	}
	before := sha256.Sum256(original)
	client := modelFixture(func(request GenerationRequest) []byte {
		file := request.Attachments[0]
		if len(file.Data) > maxReviewImageBytes || file.ContentType != "image/jpeg" {
			t.Errorf("oversized photo was passed to provider: bytes=%d type=%s", len(file.Data), file.ContentType)
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(file.Data))
		if err != nil || config.Width > 1568 || config.Height > 1568 {
			t.Errorf("review dimensions=%+v, %v", config, err)
		}
		return []byte(`{"summary":"The generated pattern cannot document the repair.","limitations":"Photos cannot confirm workmanship or safety.","coverage":[{"scopeItem":"work-1","evidenceIds":["generated"],"coverage":"unclear","observation":"No identifiable repair is visible."}]}`)
	})
	_, err := New("fixture", "fixture", client).ReviewEvidence(context.Background(), workflow.Job{Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Repair sink connection"}}}}, []workflow.EvidenceBlob{{ID: "generated", ContentType: "image/png", Data: original}})
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(original) != before {
		t.Fatal("comparison changed the original photo")
	}
}

func TestReviewImageKeepsSmallOriginalAndPreparesWideImages(t *testing.T) {
	for _, size := range []image.Point{{X: 64, Y: 64}, {X: 9000, Y: 2}} {
		original := generatedReviewPNG(t, size.X, size.Y)
		file, err := prepareReviewAttachment("image/png", original)
		if err != nil {
			t.Fatal(err)
		}
		if size.X == 64 && (!bytes.Equal(file.Data, original) || file.ContentType != "image/png") {
			t.Fatal("small image was unnecessarily recompressed")
		}
		if size.X > 8000 && file.ContentType != "image/jpeg" {
			t.Fatal("wide image was not prepared")
		}
	}
	if _, err := prepareReviewAttachment("image/png", []byte("invalid")); err == nil {
		t.Fatal("invalid image accepted")
	}
}
