package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type customerProfileFixture struct {
	*workflow.MemoryStore
	profile   workflow.CustomerProfile
	accountID string
}

func (s *customerProfileFixture) CustomerProfile(_ context.Context, id string) (workflow.CustomerProfile, error) {
	s.accountID = id
	return s.profile, nil
}
func (s *customerProfileFixture) SaveCustomerProfile(_ context.Context, id string, p workflow.CustomerProfile) (workflow.CustomerProfile, error) {
	s.accountID = id
	p.Email = s.profile.Email
	s.profile = p
	return p, nil
}

func TestCustomerProfileUsesAccountAndCanonicalLocation(t *testing.T) {
	store := &customerProfileFixture{MemoryStore: workflow.NewMemoryStore(workflow.NewState()), profile: workflow.CustomerProfile{DisplayName: "Sam", Email: "sam@example.test"}}
	server := New(Dependencies{Store: store, Sessions: customerSession(), AppBaseURL: "http://example.test"})
	handler := server.Handler()
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"displayName":"Sam New","phone":"+31 6 12345678","location":null}`, 200},
		{`{"displayName":"S","phone":"","location":null}`, 400},
		{`{"displayName":"Sam","phone":"not a number","location":null}`, 400},
		{`{"displayName":"Sam","phone":"","accountId":"another"}`, 400},
		{`{"displayName":"Sam","phone":"","email":"another@example.test"}`, 400},
		{`{"displayName":"Sam","phone":"","location":{"id":"forged","latitude":52,"longitude":4}}`, 400},
	} {
		r := httptest.NewRequest(http.MethodPut, "/api/customer/profile", strings.NewReader(tc.body))
		addTestOrigin(r)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("profile status %d: %s", w.Code, w.Body.String())
		}
	}
	if store.accountID != "test-customer" || store.profile.DisplayName != "Sam New" || store.profile.Email != "sam@example.test" {
		t.Fatalf("profile ownership or validation failed: %+v", store.profile)
	}
	location := testLocation()
	location.Selection = server.signLocation("test-customer", location)
	location.Address = "client-supplied address"
	location.Latitude = 0
	for _, saved := range []bool{false, true} {
		if saved {
			location.Selection = ""
		}
		raw, err := json.Marshal(map[string]any{"displayName": "Sam New", "phone": "", "location": location})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPut, "/api/customer/profile", strings.NewReader(string(raw)))
		addTestOrigin(r)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || store.profile.Location == nil || store.profile.Location.Address != testLocation().Address || store.profile.Location.Latitude != testLocation().Latitude {
			t.Fatalf("profile accepted altered location, saved=%v: status=%d", saved, w.Code)
		}
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/customer/profile", nil))
	var profile workflow.CustomerProfile
	if err := json.Unmarshal(get.Body.Bytes(), &profile); err != nil || profile.DisplayName != "Sam New" || profile.Location == nil || profile.Location.ID != location.ID {
		t.Fatal("saved profile did not load through HTTP")
	}
	anonymous := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/customer/profile", nil))
	if anonymous.Code != http.StatusServiceUnavailable {
		t.Fatalf("anonymous profile read status=%d", anonymous.Code)
	}
}
