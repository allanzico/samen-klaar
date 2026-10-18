package workflow

import "testing"

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
