package workflow

import "testing"

func TestEvidenceCannotSatisfyAnotherScopeOrWorkItem(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 2, WorkStarted: true, WorkItems: []WorkItem{{ID: "valve", Kind: "photo", Description: "Valve replacement"}}, Evidence: []Evidence{{AssetID: "old", ContentType: "image/png", Kind: "photo", ScopeVersion: 1, CompletionRound: 1, WorkItemIDs: []string{"valve"}}}}
	if err := Apply(&state, Action{Actor: "professional", Action: "submit-evidence"}); err == nil {
		t.Fatal("old-scope proof satisfied new completion")
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "record-evidence", Evidence: Evidence{AssetID: "new", ContentType: "image/png", Kind: "photo", WorkItemIDs: []string{"unrelated"}}}); err == nil {
		t.Fatal("unrelated work item accepted")
	}
}
