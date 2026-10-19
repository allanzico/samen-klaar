package paypal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSellerConnectionRetainsSafeProviderDiagnostics(t *testing.T) {
	for _, authFailure := range []bool{true, false} {
		t.Run(fmt.Sprint("authentication=", authFailure), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth2/token" && !authFailure {
					_, _ = fmt.Fprint(w, `{"access_token":"test-token","expires_in":3600}`)
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"name":"NOT_AUTHORIZED","error":"invalid_client","debug_id":"abc123def456","message":"private-provider-detail","access_token":"private-token","details":[{"issue":"PERMISSION_DENIED","value":"private-seller-email"}]}`)
			}))
			defer server.Close()
			service := newTestMarketplaceService(server, &sellerStoreStub{})
			_, err := service.BeginSellerOnboarding(context.Background(), "professional-1")
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("unsafe or missing connection error: %v", err)
			}
			var diagnostic *providerError
			if !errors.As(err, &diagnostic) {
				t.Fatalf("missing diagnostic: %v", err)
			}
			status, code, issue, debugID := diagnostic.PayPalDiagnostic()
			if status != 403 || code != "NOT_AUTHORIZED" || issue != "PERMISSION_DENIED" || debugID != "abc123def456" {
				t.Fatalf("unexpected diagnostic: %d %s %s %s", status, code, issue, debugID)
			}
		})
	}
	unsafe := newProviderError(500, "API", []byte(`{"name":"bad\nlog","debug_id":"private-token","details":[{"issue":"private@example.test"}]}`), nil).(*providerError)
	_, code, issue, debugID := unsafe.PayPalDiagnostic()
	if code != "" || issue != "" || debugID != "" {
		t.Fatal("diagnostic accepted free text or an invalid debug identifier")
	}
}

func TestSellerVerificationRetriesWithoutTrustingReturnFlags(t *testing.T) {
	store := &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional-1", TrackingID: "track-1", Status: "pending"}}
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"test-token","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/returned-seller":
			attempts++
			if attempts == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprint(w, `{"merchant_id":"returned-seller","tracking_id":"track-1","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		default:
			t.Errorf("unexpected verification request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := newTestMarketplaceService(server, store)
	if _, err := service.SellerOnboardingStatus(context.Background(), "professional-1", "returned-seller"); err == nil || store.record.MerchantID != "" || store.record.Status != "pending" {
		t.Fatal("failed verification must not mark the seller as connected")
	}
	status, err := service.SellerOnboardingStatus(context.Background(), "professional-1", "returned-seller")
	if err != nil || !status.Connected || store.record.MerchantID != "returned-seller" || store.record.Status != "verified" || attempts != 2 {
		t.Fatalf("verification retry did not persist the verified seller: %+v %v", status, err)
	}
}
