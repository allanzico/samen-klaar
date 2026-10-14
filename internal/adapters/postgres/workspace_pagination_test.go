package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestWorkspaceSummariesAreProjectedAndPaginated(t *testing.T) {
	ctx, store := accountTestStore(t)
	account := accountFixture(t, ctx, store)
	for i := 0; i < 105; i++ {
		state, err := store.CreateWorkspace(ctx, account)
		if err != nil {
			t.Fatal(err)
		}
		state.Intake = &workflow.Intake{Step: "matches", Description: fmt.Sprintf("Request %d", i), Messages: make([]workflow.Message, 100)}
		for j := range state.Intake.Messages {
			state.Intake.Messages[j] = workflow.Message{Role: "customer", Text: "A detailed but irrelevant conversation message"}
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, data); err != nil {
			t.Fatal(err)
		}
	}
	first, cursor, err := store.ListWorkspacesPage(ctx, account, "customer", "")
	if err != nil || len(first) != 100 || cursor == "" {
		t.Fatalf("first page=%d cursor=%q err=%v", len(first), cursor, err)
	}
	second, next, err := store.ListWorkspacesPage(ctx, account, "customer", cursor)
	if err != nil || len(second) != 5 || next != "" {
		t.Fatalf("second page=%d cursor=%q err=%v", len(second), next, err)
	}
	seen := map[string]bool{}
	for _, item := range append(first, second...) {
		if seen[item.ID] || item.Status != "matching" || item.Title == "" {
			t.Fatalf("bad summary or duplicate: %+v", item)
		}
		seen[item.ID] = true
	}
	var fullBytes, summaryBytes int
	if err := store.pool.QueryRow(ctx, `SELECT octet_length(state_json::text),octet_length((`+workspaceSummaryJSON+`)::text)
		FROM account_workspaces WHERE id=$1`, first[0].ID).Scan(&fullBytes, &summaryBytes); err != nil {
		t.Fatal(err)
	}
	if summaryBytes >= fullBytes/2 {
		t.Fatalf("summary projection transferred %d bytes from %d byte state", summaryBytes, fullBytes)
	}
	if _, _, err := store.ListWorkspacesPage(context.Background(), account, "customer", "invalid!"); err == nil {
		t.Fatal("invalid cursor accepted")
	}
}
