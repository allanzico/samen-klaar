package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestBookingAuthorizationRequiresConfiguredPayPalAndCannotBeForged(t *testing.T) {
	server := New(Dependencies{
		Store: workflow.NewMemoryStore(workflow.NewState()), Sessions: customerSession(), AppBaseURL: "http://example.test",
	})
	handler := server.Handler()

	for _, action := range []string{"set-payment-timing", "approve-and-book", "book-appointment", "approve-quote", "accept-date"} {
		t.Run("unavailable provider blocks "+action, func(t *testing.T) {
			body, err := json.Marshal(Action{Action: action, Actor: "customer", PaymentAuthorizationTiming: workflow.PaymentAuthorizationAtBooking})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/actions", bytes.NewReader(body))
			addTestOrigin(request)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("booking-time authorization without PayPal returned %d: %s", response.Code, response.Body.String())
			}
		})
	}

	t.Run("client cannot record a verified authorization", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/actions", bytes.NewBufferString(`{"action":"record-payment-authorization","paypalAuthorized":true}`))
		addTestOrigin(request)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("forged authorization action returned %d: %s", response.Code, response.Body.String())
		}
	})
}
