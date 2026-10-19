package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type marketplaceFailureStub struct {
	err        error
	accountID  string
	merchantID string
}

func (m *marketplaceFailureStub) BeginSellerOnboarding(_ context.Context, accountID string) (string, error) {
	m.accountID = accountID
	return "", m.err
}
func (m *marketplaceFailureStub) SellerOnboardingStatus(_ context.Context, accountID, merchantID string) (workflow.PayPalPayoutSetupStatus, error) {
	m.accountID, m.merchantID = accountID, merchantID
	return workflow.PayPalPayoutSetupStatus{}, m.err
}

type publicConnectionFailure string

func (e publicConnectionFailure) Error() string                { return "private error detail" }
func (e publicConnectionFailure) PayPalConnectionCode() string { return string(e) }

func TestPayPalConnectionErrorsAreActionableWithoutExposingProviderDetails(t *testing.T) {
	sessions := &testSessions{principal: identity.Principal{User: identity.User{ID: "professional-account"}, ActiveRole: identity.Professional, Memberships: []identity.Membership{{Role: identity.Professional, Status: "pending"}}}}
	for _, tc := range []struct {
		name, method, path, code string
		marketplace              PayPalMarketplace
		status                   int
	}{
		{"not configured", "POST", "/api/professional/paypal/onboarding", "PAYPAL_PAYOUTS_UNAVAILABLE", nil, 503},
		{"start failure", "POST", "/api/professional/paypal/onboarding", "PAYPAL_CONNECTION_START_FAILED", &marketplaceFailureStub{err: errors.New("token=private-provider-detail")}, 503},
		{"verification failure", "GET", "/api/professional/paypal/status?merchantIdInPayPal=RETURN_MERCHANT", "PAYPAL_CONNECTION_VERIFY_FAILED", &marketplaceFailureStub{err: errors.New("token=private-provider-detail")}, 502},
		{"configuration failure", "GET", "/api/professional/paypal/status", "PAYPAL_PAYOUTS_UNAVAILABLE", &marketplaceFailureStub{err: publicConnectionFailure("PAYPAL_PAYOUTS_UNAVAILABLE")}, 503},
		{"already connected", "POST", "/api/professional/paypal/onboarding", "PAYPAL_ALREADY_CONNECTED", &marketplaceFailureStub{err: publicConnectionFailure("PAYPAL_ALREADY_CONNECTED")}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := New(Dependencies{Sessions: sessions, Marketplace: tc.marketplace, AppBaseURL: "http://example.test"})
			req := httptest.NewRequest(tc.method, tc.path, nil)
			addTestOrigin(req)
			res := httptest.NewRecorder()
			server.Handler().ServeHTTP(res, req)
			if res.Code != tc.status || !strings.Contains(res.Body.String(), `"code":"`+tc.code+`"`) || strings.Contains(res.Body.String(), "private") {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if !strings.Contains(res.Header().Get("Content-Type"), "application/json") || !strings.Contains(res.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("connection failure response must be non-cacheable JSON")
			}
			if stub, ok := tc.marketplace.(*marketplaceFailureStub); ok && stub.accountID != "professional-account" {
				t.Fatal("connection did not use the signed-in professional")
			}
			if stub, ok := tc.marketplace.(*marketplaceFailureStub); ok && tc.name == "verification failure" && stub.merchantID != "RETURN_MERCHANT" {
				t.Fatal("verification lost the returned merchant reference")
			}
		})
	}
}
