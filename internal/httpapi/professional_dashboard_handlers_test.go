package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type dashboardFixture struct {
	*workflow.MemoryStore
	accountID string
}

func (s *dashboardFixture) ProfessionalDashboard(_ context.Context, id string) (workflow.ProfessionalDashboard, error) {
	s.accountID = id
	return workflow.ProfessionalDashboard{Balances: []workflow.ProfessionalBalance{{Currency: "EUR", ReceivedMinor: 12000}}}, nil
}

func TestProfessionalDashboardIsScopedToSignedInProfessional(t *testing.T) {
	store := &dashboardFixture{MemoryStore: workflow.NewMemoryStore(workflow.NewState())}
	for _, role := range []string{"customer", "professional"} {
		session := customerSession()
		session.principal.ActiveRole = identity.Role(role)
		session.principal.Memberships = []identity.Membership{{Role: identity.Role(role), Status: "active"}}
		handler := New(Dependencies{Store: store, Sessions: session}).Handler()
		request := httptest.NewRequest(http.MethodGet, "/api/professional/dashboard?accountId=someone-else", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		want := http.StatusOK
		if role == "customer" {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("%s status=%d", role, response.Code)
		}
	}
	if store.accountID != "test-customer" {
		t.Fatalf("queried account=%q", store.accountID)
	}
}
