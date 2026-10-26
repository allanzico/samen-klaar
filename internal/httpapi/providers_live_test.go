package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"testing"
	"time"

	aiprovider "github.com/allanzico/samen-klaar/internal/adapters/ai"
	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/adapters/r2"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

// Opt-in credentials check. This sends only generated test content to the
// configured providers and never authorizes or captures a PayPal payment.
func TestLiveProviders(t *testing.T) {
	if os.Getenv("RUN_LIVE_PROVIDER_CHECKS") != "1" {
		t.Skip("opt in with RUN_LIVE_PROVIDER_CHECKS=1 and local provider settings")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	t.Run("AI", func(t *testing.T) {
		ai, err := aiprovider.New(ctx, os.Getenv("AI_PROVIDER"), os.Getenv("AI_PROVIDER_KEY"), os.Getenv("AI_MODEL"), os.Getenv("AI_EFFORT"))
		if err != nil {
			t.Fatal("AI configuration failed")
		}
		suggestion, err := ai.Clarify(ctx, "Infer from description", "My kitchen sink is leaking underneath, but water still runs from the tap.")
		if err != nil {
			t.Fatalf("Configured AI intake call failed: %v", err)
		}
		if !workflow.ValidSuggestion(suggestion) || suggestion.Category != "Plumbing" {
			t.Fatal("AI did not return a supported plumbing clarification")
		}
		t.Log("Configured AI returned a supported plumbing clarification")
		var picture bytes.Buffer
		canvas := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				canvas.Set(x, y, color.RGBA{R: 120, G: 150, B: 190, A: 255})
			}
		}
		if err := png.Encode(&picture, canvas); err != nil {
			t.Fatal(err)
		}
		job := workflow.Job{Description: "Repair a leaking connection under the sink", Quote: workflow.Quote{Lines: []workflow.QuoteLine{{Label: "Repair leaking sink connection"}}}}
		review, err := ai.ReviewEvidence(ctx, job, []workflow.EvidenceBlob{{ID: "generated-test-image", ContentType: "image/png", Label: "Generated test image, not a work photograph", Data: picture.Bytes()}})
		if err != nil {
			t.Fatalf("Configured AI evidence review failed: %v", err)
		}
		if review == nil || len(review.Coverage) != 1 {
			t.Fatal("Configured AI evidence review failed validation")
		}
		t.Log("Configured AI returned a validated evidence-coverage review for generated test content")
	})
	t.Run("R2", func(t *testing.T) {
		storage, err := r2.New(ctx)
		if err != nil {
			t.Fatal("R2 configuration failed")
		}
		job, asset := "verification-"+rand.Text(), rand.Text()
		content := []byte("%PDF-1.4\nGenerated SamenKlaar storage verification fixture.\n%%EOF")
		if err := storage.Put(ctx, job, asset, "application/pdf", content); err != nil {
			t.Fatal("Configured R2 upload failed")
		}
		defer storage.Delete(context.Background(), job, asset)
		body, ct, err := storage.Get(ctx, job, asset)
		if err != nil {
			t.Fatal("Configured R2 download failed")
		}
		downloaded, err := io.ReadAll(body)
		body.Close()
		if err != nil || ct != "application/pdf" || !bytes.Equal(content, downloaded) {
			t.Fatal("R2 round trip differed from the uploaded file")
		}
		t.Log("Configured R2 uploaded and returned the same private evidence bytes")
	})
	t.Run("PayPal", func(t *testing.T) {
		job := "verification-" + rand.Text()
		service, err := paypal.New("SamenKlaar verification")
		if err != nil {
			t.Fatal("PayPal sandbox configuration failed")
		}
		state := workflow.NewState()
		state.WorkspaceID = "verification-" + rand.Text()
		state.Job = workflow.Job{ID: job, ScopeVersion: 1, CompletionApproved: true, PaymentApprovedVersion: 1, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 100}}
		order, approval, err := service.CreateOrder(ctx, state)
		if err != nil || order == "" || approval == "" {
			t.Fatal("Configured PayPal sandbox order creation failed")
		}
		t.Log("Configured PayPal sandbox created an unapproved EUR 1.00 verification order; no authorization or capture was performed")
	})
}
