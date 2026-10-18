package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type localeStoreFixture struct {
	*workflow.MemoryStore
	locale, accountID string
}

func (s *localeStoreFixture) AccountLocale(_ context.Context, id string) (string, error) {
	s.accountID = id
	return s.locale, nil
}
func (s *localeStoreFixture) SetAccountLocale(_ context.Context, id, locale string) error {
	s.accountID = id
	s.locale = locale
	return nil
}

func TestAccountLocaleEndpointUsesSignedInAccountAndValidatesPreference(t *testing.T) {
	store := &localeStoreFixture{MemoryStore: workflow.NewMemoryStore(workflow.NewState()), locale: "en"}
	handler := New(Dependencies{Store: store, Sessions: customerSession(), AppBaseURL: "http://example.test"}).Handler()
	for _, tc := range []struct {
		method, body string
		status       int
	}{
		{http.MethodGet, "", http.StatusOK}, {http.MethodPost, `{"locale":"nl"}`, http.StatusOK},
		{http.MethodPost, `{"locale":"fr"}`, http.StatusBadRequest}, {http.MethodPost, `{"locale":"en","accountId":"someone-else"}`, http.StatusBadRequest},
	} {
		request := httptest.NewRequest(tc.method, "/api/account/locale", strings.NewReader(tc.body))
		addTestOrigin(request)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s %s: status=%d body=%s", tc.method, tc.body, response.Code, response.Body.String())
		}
	}
	if store.locale != "nl" || store.accountID != "test-customer" {
		t.Fatalf("preference changed another account: %+v", store)
	}
}
