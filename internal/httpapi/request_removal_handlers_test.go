package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestOpenRequestLimitAndDeleteOverHTTP(t *testing.T) {
	store := workflow.NewMemoryStore(workflow.NewState())
	sessions := customerSession()
	handler := New(Dependencies{Store: store, Sessions: sessions, AppBaseURL: "http://example.test"}).Handler()
	call := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader("{}"))
		addTestOrigin(request)
		handler.ServeHTTP(response, request)
		return response
	}
	ids := []string{}
	for range workflow.MaxOpenRequests {
		response := call(http.MethodPost, "/api/workspaces")
		if response.Code != http.StatusOK {
			t.Fatalf("create: %d %s", response.Code, response.Body.String())
		}
		var created struct {
			Workspace workflow.State `json:"workspace"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.Workspace.WorkspaceID)
	}
	if response := call(http.MethodPost, "/api/workspaces"); response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), workflow.ErrOpenRequestLimit.Error()) {
		t.Fatalf("create past the limit: %d %s", response.Code, response.Body.String())
	}
	var list struct {
		Workspaces       []workflow.WorkspaceSummary `json:"workspaces"`
		OpenRequests     int                         `json:"openRequests"`
		OpenRequestLimit int                         `json:"openRequestLimit"`
	}
	if err := json.Unmarshal(call(http.MethodGet, "/api/workspaces").Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.OpenRequests != workflow.MaxOpenRequests || list.OpenRequestLimit != workflow.MaxOpenRequests || len(list.Workspaces) != 3 || !list.Workspaces[0].CanDelete {
		t.Fatalf("list: %+v", list)
	}

	if response := call(http.MethodDelete, "/api/workspaces/"+ids[0]); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"deleted"`) {
		t.Fatalf("delete: %d %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodDelete, "/api/workspaces/"+ids[0]); response.Code != http.StatusNotFound {
		t.Fatalf("delete twice: %d %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/api/workspaces"); response.Code != http.StatusOK {
		t.Fatalf("create after delete: %d %s", response.Code, response.Body.String())
	}

	// Another customer cannot delete it.
	sessions.principal.User.ID = "someone-else"
	if response := call(http.MethodDelete, "/api/workspaces/"+ids[1]); response.Code != http.StatusNotFound {
		t.Fatalf("delete another customer's request: %d", response.Code)
	}
	// The professional removal only runs through DELETE, never as a generic action.
	sessions.principal = identity.Principal{User: identity.User{ID: "pro"}, ActiveRole: identity.Professional, Memberships: []identity.Membership{{Role: identity.Professional, Status: "active"}}}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/actions", strings.NewReader(`{"action":"remove-request","workspaceId":"`+ids[1]+`"}`))
	addTestOrigin(request)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("remove-request as an action: %d %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodDelete, "/api/workspaces/"+ids[1]); response.Code != http.StatusNotFound {
		t.Fatalf("unassigned professional removal: %d %s", response.Code, response.Body.String())
	}
}
