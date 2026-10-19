package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type adminStoreFixture struct {
	*workflow.MemoryStore
	actor, target string
	filter        workflow.DirectoryFilter
	calls         int
}

func (s *adminStoreFixture) OperatorUsers(_ context.Context, actor string, filter workflow.DirectoryFilter) (workflow.AdminUserPage, error) {
	s.actor, s.filter = actor, filter
	s.calls++
	return workflow.AdminUserPage{Users: []workflow.AdminUser{}}, nil
}
func (s *adminStoreFixture) OperatorUser(_ context.Context, actor, target string) (workflow.AdminUserDetail, error) {
	s.actor, s.target = actor, target
	s.calls++
	return workflow.AdminUserDetail{User: workflow.AdminUser{ID: target}, Events: []workflow.AdminEvent{}}, nil
}
func (s *adminStoreFixture) AdministerUser(_ context.Context, actor, target string, _ workflow.AdminUserAction) error {
	s.actor, s.target = actor, target
	s.calls++
	return nil
}
func (s *adminStoreFixture) OperatorLocations(_ context.Context, actor, query, status string, offset int) (workflow.AdminLocationPage, error) {
	s.actor = actor
	s.filter = workflow.DirectoryFilter{Query: query, Status: status, Offset: offset}
	s.calls++
	return workflow.AdminLocationPage{Locations: []workflow.AdminLocation{}}, nil
}

func TestOperatorDirectoriesRequireActiveOperatorAndValidateMutations(t *testing.T) {
	store := &adminStoreFixture{MemoryStore: workflow.NewMemoryStore(workflow.NewState())}
	sessions := customerSession()
	handler := New(Dependencies{Store: store, Sessions: sessions, AppBaseURL: "http://example.test"}).Handler()
	request := func(method, path, body, origin string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Origin", origin)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		if want == 200 && !strings.Contains(res.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("admin response was cacheable")
		}
	}
	for _, path := range []string{"/api/operator/users", "/api/operator/users/target", "/api/operator/locations"} {
		request("GET", path, "", "", 403)
	}
	sessions.principal = identity.Principal{User: identity.User{ID: "operator-current"}, ActiveRole: identity.Operator, Memberships: []identity.Membership{{Role: identity.Operator, Status: "pending"}}}
	request("GET", "/api/operator/users", "", "", 403)
	if store.calls != 0 {
		t.Fatal("unauthorized request reached admin store")
	}
	sessions.principal.Memberships[0].Status = "active"
	request("GET", "/api/operator/users?q=Jane&role=professional&status=disabled&offset=25", "", "", 200)
	if store.actor != "operator-current" || store.filter.Role != "professional" || store.filter.Offset != 25 {
		t.Fatalf("wrong query/actor: %+v", store)
	}
	request("GET", "/api/operator/locations?q=Amsterdam&status=waitlisted", "", "", 200)
	if store.filter.Status != "waitlisted" {
		t.Fatal("coverage filter lost")
	}
	request("GET", "/api/operator/users?role=root", "", "", 400)
	request("GET", "/api/operator/users?offset=-1", "", "", 400)
	request("GET", "/api/operator/locations?status=bogus", "", "", 400)
	valid := `{"action":"suspend-role","role":"professional","expectedStatus":"active","reason":"Investigating a reported issue."}`
	request("POST", "/api/operator/users/target", valid, "https://elsewhere.test", 403)
	request("POST", "/api/operator/users/target", `{"action":"suspend-role","role":"operator","expectedStatus":"active","reason":"Investigating a reported issue."}`, "http://example.test", 400)
	request("POST", "/api/operator/users/target", strings.Replace(valid, "Investigating a reported issue.", "short", 1), "http://example.test", 400)
	request("POST", "/api/operator/users/target", strings.TrimSuffix(valid, "}")+`,"actorId":"forged"}`, "http://example.test", 400)
	request("POST", "/api/operator/users/target", valid+` {}`, "http://example.test", 400)
	request("POST", "/api/operator/users/target", valid, "http://example.test", 200)
	if store.actor != "operator-current" || store.target != "target" {
		t.Fatal("admin action used client identity")
	}
	request("DELETE", "/api/operator/users/target", "", "http://example.test", 405)
}
