package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestCoordinationRequestsHumanFactsAndRejectsMoneyOrApprovalActions(t *testing.T) {
	response := `{"question":"Welke werkzaamheden tonen deze foto's en wat is nog niet klaar?","summary":"","proposedAction":"request-work-note","needsOperator":false,"reason":""}`
	client := modelFixture(func(request GenerationRequest) []byte {
		if !strings.Contains(request.Prompt, "Replace the leaking valve") || !strings.Contains(request.System, "Never approve") || !strings.Contains(request.Prompt, `"locale":"nl"`) {
			t.Errorf("coordination prompt lost scope/language/boundaries: %s %s", request.System, request.Prompt)
		}
		return []byte(response)
	})
	coordinator := New("fixture", "fixture", client)
	request := workflow.CoordinationRequest{WorkspaceID: "workspace", JobID: "job", ScopeVersion: 1, Round: 1, Trigger: "completion-note", Locale: "nl", AgreedScope: "Replace the leaking valve", EvidenceLabels: []string{"After repair"}}
	if result, err := coordinator.Coordinate(context.Background(), request); err != nil || result.Question == "" {
		t.Fatalf("question: %+v, %v", result, err)
	}
	response = `{"question":"","summary":"The professional replaced the valve.","proposedAction":"capture-payment","needsOperator":false,"reason":""}`
	request.Reply = "The old valve was replaced and the connection checked."
	if _, err := coordinator.Coordinate(context.Background(), request); err == nil {
		t.Fatal("coordination accepted an automatic payment action")
	}
}
