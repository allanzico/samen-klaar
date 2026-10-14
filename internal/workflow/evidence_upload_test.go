package workflow

import "testing"

func TestCompletionRequiresStoredUploadBesidesWorkNote(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", WorkStarted: true, Evidence: []Evidence{{Kind: "note", Label: "Professional work note", Description: "I repaired the valve"}}, DetailAdded: true}
	if err := Apply(&state, Action{Action: "submit-evidence", Actor: "professional"}); err == nil || state.Job.EvidenceSubmitted {
		t.Fatal("note-only completion was accepted")
	}
	if err := Apply(&state, Action{Action: "record-evidence", Actor: "professional", Evidence: Evidence{AssetID: "stored-asset", ContentType: "image/png", Kind: "photo", Label: "Evidence photo"}}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Action: "submit-evidence", Actor: "professional"}); err != nil || !state.Job.EvidenceSubmitted {
		t.Fatalf("stored upload plus note could not be submitted: %v", err)
	}
}
