package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCaptureRefreshesAuthorizationBeforeMovingMoney(t *testing.T) {
	for _, outcome := range []struct{ status, expiry, expected string }{
		{"CREATED", "2000-01-01T00:00:00Z", "expired"},
		{"EXPIRED", "", "expired"},
		{"VOIDED", "", "voided"},
		{"DENIED", "", "denied"},
		{"PENDING", "", "authorization_pending"},
		{"PARTIALLY_CAPTURED", "", "partial_capture_review"},
		{"CREATED", "not-a-date", "authorized"},
		{"UNAVAILABLE", "", "authorized"},
	} {
		t.Run(outcome.expected, func(t *testing.T) {
			state, record := authorizationFixture()
			state.Job.CompletionApproved, state.Job.PaymentApprovedVersion = true, 2
			record.Status, record.AuthorizationID = "authorized", "authorization"
			captures := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/oauth2/token":
					_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
				case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
					_, _ = fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"tracking","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
				case "/v2/payments/authorizations/authorization":
					if outcome.status == "UNAVAILABLE" {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "authorization", "status": outcome.status, "expiration_time": outcome.expiry, "amount": map[string]string{"value": "123.45", "currency_code": "EUR"}, "payee": map[string]string{"merchant_id": "seller"}, "supplementary_data": map[string]any{"related_ids": map[string]string{"order_id": "order"}}})
				case "/v2/payments/authorizations/authorization/capture":
					captures++
					_, _ = fmt.Fprint(w, `{"id":"capture","status":"COMPLETED"}`)
				default:
					t.Errorf("unexpected provider operation: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "tracking", Status: "verified"}})
			service.SetIntentStore(&intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}})
			if err := service.Capture(context.Background(), state); err == nil {
				t.Error("unusable authorization allowed capture")
			}
			if captures != 0 {
				t.Errorf("unusable authorization moved money: captures=%d", captures)
			}
			if status, err := service.Status(context.Background(), state); err != nil || status != outcome.expected {
				t.Errorf("authorization status=%q err=%v; want %q", status, err, outcome.expected)
			}
		})
	}
}

func TestExpiredAuthorizationNeedsFreshExplicitBuyerApproval(t *testing.T) {
	state, original := authorizationFixture()
	state.Job.CompletionApproved, state.Job.PaymentApprovedVersion, state.Job.WorkStarted = true, 2, true
	original.Status, original.AuthorizationID = "authorized", "authorization"
	orders, authorizations := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
			_, _ = fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"tracking","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		case "/v2/payments/authorizations/authorization":
			_, _ = fmt.Fprint(w, `{"id":"authorization","status":"CREATED","expiration_time":"2000-01-01T00:00:00Z","amount":{"value":"123.45","currency_code":"EUR"},"payee":{"merchant_id":"seller"},"supplementary_data":{"related_ids":{"order_id":"order"}}}`)
		case "/v2/checkout/orders":
			orders++
			var body struct {
				Units []struct {
					Amount paymentAmount `json:"amount"`
					Payee  struct {
						MerchantID string `json:"merchant_id"`
					} `json:"payee"`
				} `json:"purchase_units"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Units) != 1 || body.Units[0].Amount.Value != "123.45" || body.Units[0].Amount.Currency != "EUR" || body.Units[0].Payee.MerchantID != "seller" {
				t.Error("fresh approval changed the amount, currency, or seller")
			}
			if r.Header.Get("PayPal-Request-Id") == "order-request" {
				t.Error("fresh approval reused the expired attempt identity")
			}
			_, _ = fmt.Fprint(w, `{"id":"fresh-order","links":[{"rel":"approve","href":"https://www.sandbox.paypal.com/checkoutnow?token=fresh-order"}]}`)
		case "/v2/checkout/orders/fresh-order/authorize":
			authorizations++
			if r.Header.Get("PayPal-Request-Id") == "auth-request" {
				t.Error("fresh authorization reused the expired attempt identity")
			}
			order := authorizationOrder()
			order["id"] = "fresh-order"
			auth := order["purchase_units"].([]any)[0].(map[string]any)["payments"].(map[string]any)["authorizations"].([]any)[0].(map[string]any)
			auth["id"], auth["expiration_time"] = "fresh-authorization", "2099-01-01T00:00:00Z"
			_ = json.NewEncoder(w).Encode(order)
		default:
			t.Errorf("unexpected automatic money operation: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "tracking", Status: "verified"}})
	store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(original.JobID, original.ScopeVersion): original}}
	service.SetIntentStore(store)
	if status, err := service.Reconcile(context.Background(), state); err != nil || status != "expired" {
		t.Fatalf("expiry status=%q err=%v", status, err)
	}
	details, err := service.AuthorizationDetails(context.Background(), state)
	if err != nil || details.AuthorizationExpiresAt != "2000-01-01T00:00:00Z" {
		t.Fatalf("expiry facts=%+v err=%v", details, err)
	}
	if orders != 0 || authorizations != 0 {
		t.Fatalf("expiry created a replacement without buyer approval: orders=%d authorizations=%d", orders, authorizations)
	}
	if err := service.Capture(context.Background(), state); err == nil {
		t.Error("expired hold allowed capture")
	}
	id, _, err := service.CreateOrder(context.Background(), state)
	if err != nil || id != "fresh-order" {
		t.Fatalf("fresh checkout id=%q err=%v", id, err)
	}
	details, err = service.AuthorizationDetails(context.Background(), state)
	if err != nil || details.Status != "created" || details.AuthorizationExpiresAt != "" || details.AuthorizationID != "" || authorizations != 0 {
		t.Fatalf("fresh checkout inherited expired approval: %+v err=%v authorizations=%d", details, err, authorizations)
	}
	if err := service.Authorize(context.Background(), state, id); err != nil {
		t.Fatal(err)
	}
	details, err = service.AuthorizationDetails(context.Background(), state)
	if err != nil || details.Status != "authorized" || details.AuthorizationID != "fresh-authorization" || details.AuthorizationExpiresAt != "2099-01-01T00:00:00Z" || orders != 1 || authorizations != 1 {
		t.Fatalf("fresh buyer approval=%+v err=%v orders=%d authorizations=%d", details, err, orders, authorizations)
	}
}
