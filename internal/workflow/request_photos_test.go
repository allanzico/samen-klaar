package workflow

import "testing"

func TestRequestPhotosSurviveProfessionalReviewAndReselection(t *testing.T) {
	state := NewState()
	state.WorkspaceID, state.ParentRequestID = "workspace-photo", "parent-photo"
	state.Intake = &Intake{Step: "matches", Category: "Plumbing", Location: &Location{Label: "Amsterdam"}, Matches: testMatches("noor", "jonas")}
	photo := Evidence{AssetID: "problem-photo", ContentType: "image/png", Label: "Photo of the problem", Kind: "photo"}
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: photo}); err != nil {
		t.Fatal(err)
	}
	for _, professional := range []string{"noor", "jonas"} {
		if err := Apply(&state, Action{Action: "select-match", Actor: "customer", Value: professional, SelectedOffer: testOffer(professional, "Plumbing")}); err != nil {
			t.Fatal(err)
		}
		if len(state.RequestPhotos) != 1 || state.RequestPhotos[0].AssetID != photo.AssetID || state.WorkspaceID != "workspace-photo" || state.ParentRequestID != "parent-photo" {
			t.Fatal("selecting a professional dropped the request photo or its owning request identity")
		}
		if len(state.Job.Evidence) != 0 {
			t.Fatal("a problem photo became completion evidence")
		}
		if err := Apply(&state, Action{Action: "return-to-matches", Actor: "customer", Matches: testMatches("noor", "jonas")}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCustomerRequestPhotoRules(t *testing.T) {
	photo := Evidence{AssetID: "asset-1", ContentType: "image/png", Label: "Photo of the problem", Kind: "photo"}
	state := NewState()
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "professional", Evidence: photo}); err == nil {
		t.Fatal("a professional attached a customer request photo")
	}
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: Evidence{AssetID: "doc", ContentType: "application/pdf"}}); err == nil {
		t.Fatal("a non-image was accepted as a request photo")
	}
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: photo}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: photo}); err == nil {
		t.Fatal("the same photo was recorded twice")
	}
	if len(state.Job.Evidence) != 0 {
		t.Fatal("a request photo leaked into completion evidence")
	}
	// A pending visit confirmation blocks job steps but not describing the problem.
	state.Job.ID = "job-1"
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "pending"}
	more := photo
	more.AssetID = "asset-2"
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: more}); err != nil {
		t.Fatalf("photo blocked while the visit awaits confirmation: %v", err)
	}
	for i := 0; len(state.RequestPhotos) < MaxRequestPhotos; i++ {
		extra := photo
		extra.AssetID = "fill-" + string(rune('a'+i))
		if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: extra}); err != nil {
			t.Fatal(err)
		}
	}
	over := photo
	over.AssetID = "overflow"
	if err := Apply(&state, Action{Action: "record-request-photo", Actor: "customer", Evidence: over}); err == nil {
		t.Fatal("more than six photos were accepted")
	}
}
