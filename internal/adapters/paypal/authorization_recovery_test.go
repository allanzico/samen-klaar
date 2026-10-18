package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func authorizationFixture() (workflow.State, IntentRecord) {
	state := workflow.State{WorkspaceID: "workspace", Job: workflow.Job{
		ID: "job", ScopeVersion: 2, ProfessionalAccountID: "professional", QuoteApproved: true,
		PaymentAuthorizationTiming: workflow.PaymentAuthorizationAtBooking, Appointment: &workflow.Appointment{SlotID: "slot"},
		Quote: workflow.Quote{TotalMinor: 12345, Currency: "EUR"},
	}}
	record := IntentRecord{WorkspaceID: "workspace", JobID: "job", ScopeVersion: 2, AmountMinor: 12345, Currency: "EUR", SellerMerchantID: "seller",
		OrderID: "order", AuthorizationID: "", Status: "authorization_uncertain", OrderRequestID: "order-request", AuthRequestID: "auth-request", VoidRequestID: "void-request"}
	return state, record
}

func authorizationOrder() map[string]any {
	return map[string]any{"id": "order", "intent": "AUTHORIZE", "status": "COMPLETED", "purchase_units": []any{map[string]any{
		"reference_id": "job-v2", "amount": map[string]string{"value": "123.45", "currency_code": "EUR"}, "payee": map[string]string{"merchant_id": "seller"},
		"payments": map[string]any{"authorizations": []any{map[string]any{"id": "authorization", "status": "CREATED", "amount": map[string]string{"value": "123.45", "currency_code": "EUR"}}}},
	}}}
}

func TestRecoveredAuthorizationMustMatchTheAgreedScope(t *testing.T) {
	for _, mismatch := range []string{"none", "order", "scope", "amount", "currency", "seller", "authorization_amount"} {
		t.Run(mismatch, func(t *testing.T) {
			state, record := authorizationFixture()
			order := authorizationOrder()
			unit := order["purchase_units"].([]any)[0].(map[string]any)
			switch mismatch {
			case "order":
				order["id"] = "other-order"
			case "scope":
				unit["reference_id"] = "job-v1"
			case "amount":
				unit["amount"].(map[string]string)["value"] = "1.00"
			case "currency":
				unit["amount"].(map[string]string)["currency_code"] = "USD"
			case "seller":
				unit["payee"].(map[string]string)["merchant_id"] = "other-seller"
			case "authorization_amount":
				unit["payments"].(map[string]any)["authorizations"].([]any)[0].(map[string]any)["amount"].(map[string]string)["value"] = "1.00"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/oauth2/token":
					_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
				case "/v2/checkout/orders/order":
					_ = json.NewEncoder(w).Encode(order)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
			service := newTestMarketplaceService(server, &sellerStoreStub{})
			service.SetIntentStore(store)
			status, err := service.Reconcile(context.Background(), state)
			if mismatch == "none" {
				if err != nil || status != "authorized" {
					t.Fatalf("valid recovery status=%q err=%v", status, err)
				}
			} else if err == nil || status == "authorized" {
				t.Fatalf("mismatched %s unlocked authorization: status=%q err=%v", mismatch, status, err)
			}
		})
	}
}

func TestReconcileCannotClearAnUnresolvedAuthorizationAttempt(t *testing.T) {
	for _, status := range []string{"authorizing", "authorization_uncertain"} {
		t.Run(status, func(t *testing.T) {
			state, record := authorizationFixture()
			record.Status = status
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/oauth2/token":
					_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
				case "/v2/checkout/orders/order":
					order := authorizationOrder()
					order["status"] = "APPROVED"
					order["links"] = []any{map[string]string{"rel": "approve", "href": "https://www.sandbox.paypal.com/checkoutnow?token=order"}}
					delete(order["purchase_units"].([]any)[0].(map[string]any), "payments")
					_ = json.NewEncoder(w).Encode(order)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
			service := newTestMarketplaceService(server, &sellerStoreStub{})
			service.SetIntentStore(store)
			_, _ = service.Reconcile(context.Background(), state)
			if actual, err := service.Status(context.Background(), state); err != nil || actual != status {
				t.Fatalf("unresolved authorization became safe to abandon: status=%q err=%v", actual, err)
			}
		})
	}
}

func TestVerifiedAuthorizationRequiresTheCurrentAgreementAndSeller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
			_, _ = fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"tracking","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		case "/v2/payments/authorizations/authorization":
			_, _ = fmt.Fprint(w, `{"id":"authorization","status":"CREATED","amount":{"value":"123.45","currency_code":"EUR"},"supplementary_data":{"related_ids":{"order_id":"order"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, mismatch := range []string{"none", "workspace", "job", "scope", "amount", "currency", "seller", "request", "uncertain"} {
		t.Run(mismatch, func(t *testing.T) {
			state, record := authorizationFixture()
			record.Status, record.AuthorizationID = "authorized", "authorization"
			switch mismatch {
			case "workspace":
				record.WorkspaceID = "other"
			case "job":
				record.JobID = "other"
			case "scope":
				record.ScopeVersion = 1
			case "amount":
				record.AmountMinor = 100
			case "currency":
				record.Currency = "USD"
			case "seller":
				record.SellerMerchantID = "other"
			case "request":
				record.AuthRequestID = ""
			case "uncertain":
				record.Status = "authorization_uncertain"
			}
			store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(state.Job.ID, state.Job.ScopeVersion): record}}
			service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "tracking", Status: "verified"}})
			service.SetIntentStore(store)
			verified, err := service.VerifiedAuthorization(context.Background(), state)
			if mismatch == "none" {
				if err != nil || verified.AuthorizationID != "authorization" || verified.AuthRequestID != "auth-request" || verified.ScopeVersion != 2 {
					t.Fatalf("verified=%+v err=%v", verified, err)
				}
			} else if err == nil {
				t.Fatalf("mismatched %s authorization was accepted: %+v", mismatch, verified)
			}
		})
	}
}

func TestVoidCurrentRecoversInterruptedVoid(t *testing.T) {
	state, record := authorizationFixture()
	record.Status, record.AuthorizationID = "void_uncertain", "authorization"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v2/payments/authorizations/authorization":
			_, _ = fmt.Fprint(w, `{"id":"authorization","status":"CREATED","amount":{"value":"123.45","currency_code":"EUR"},"supplementary_data":{"related_ids":{"order_id":"order"}}}`)
		case "/v2/payments/authorizations/authorization/void":
			if r.Header.Get("PayPal-Request-Id") != "void-request" {
				t.Error("void request identity changed")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
	service := newTestMarketplaceService(server, &sellerStoreStub{})
	service.SetIntentStore(store)
	if err := service.VoidCurrent(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "voided" {
		t.Fatalf("release status=%q err=%v", status, err)
	}
}

func TestReleaseSettlesUncertainAuthorizationAcrossRestart(t *testing.T) {
	state, expected := authorizationFixture()
	record := expected
	record.Status = "release_pending"
	issued, voidCalls := false, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v2/checkout/orders/order":
			order := authorizationOrder()
			if !issued {
				order["status"] = "APPROVED"
				delete(order["purchase_units"].([]any)[0].(map[string]any), "payments")
			}
			_ = json.NewEncoder(w).Encode(order)
		case "/v2/checkout/orders/order/authorize":
			if r.Header.Get("PayPal-Request-Id") != "auth-request" {
				t.Error("authorization request identity changed during release")
			}
			issued = true
			_ = json.NewEncoder(w).Encode(authorizationOrder())
		case "/v2/payments/authorizations/authorization":
			_, _ = fmt.Fprint(w, `{"id":"authorization","status":"CREATED","amount":{"value":"123.45","currency_code":"EUR"},"supplementary_data":{"related_ids":{"order_id":"order"}}}`)
		case "/v2/payments/authorizations/authorization/void":
			if r.Header.Get("PayPal-Request-Id") != "void-request" {
				t.Error("void request identity changed during release")
			}
			voidCalls++
			if voidCalls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected release request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
	service := newTestMarketplaceService(server, &sellerStoreStub{})
	service.SetIntentStore(store)
	if err := service.ReleaseAuthorization(context.Background(), expected); err == nil {
		t.Fatal("provider interruption was hidden")
	}
	// A fresh adapter must finish the queued original attempt, including the
	// authorization ID recovered before the interrupted void response.
	service = newTestMarketplaceService(server, &sellerStoreStub{})
	service.SetIntentStore(store)
	if err := service.ReleaseAuthorization(context.Background(), expected); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "voided" {
		t.Fatalf("release status=%q err=%v", status, err)
	}
	if !issued || voidCalls != 2 {
		t.Fatalf("original uncertain operation was not settled and retried: issued=%v voids=%d", issued, voidCalls)
	}
}

func TestReleaseNeverTouchesAReplacementAttempt(t *testing.T) {
	state, expected := authorizationFixture()
	replacement := expected
	replacement.OrderRequestID, replacement.AuthRequestID, replacement.VoidRequestID = "replacement-order", "replacement-auth", "replacement-void"
	replacement.OrderID, replacement.AuthorizationID, replacement.Status = "replacement", "replacement-auth-id", "authorized"
	store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(replacement.JobID, replacement.ScopeVersion): replacement}}
	service := &Service{}
	service.SetIntentStore(store)
	if err := service.ReleaseAuthorization(context.Background(), expected); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "authorized" {
		t.Fatalf("replacement attempt changed: status=%q err=%v", status, err)
	}
}

type replacementBeforeClaimStore struct {
	*intentStoreFixture
	replacement IntentRecord
}

func (s *replacementBeforeClaimStore) ClaimPaymentIntentAttempt(ctx context.Context, expected IntentRecord, from, to string) (bool, error) {
	s.records[intentKey(expected.JobID, expected.ScopeVersion)] = s.replacement
	return s.intentStoreFixture.ClaimPaymentIntentAttempt(ctx, expected, from, to)
}

func (s *replacementBeforeClaimStore) ClaimPaymentIntent(ctx context.Context, workspace, job string, scope int, from, to string) (bool, error) {
	s.records[intentKey(job, scope)] = s.replacement
	return s.intentStoreFixture.ClaimPaymentIntent(ctx, workspace, job, scope, from, to)
}

func TestDelayedAuthorizationCannotClaimAReplacementOrder(t *testing.T) {
	state, expected := authorizationFixture()
	expected.Status = "created"
	replacement := expected
	replacement.OrderID, replacement.OrderRequestID, replacement.AuthRequestID, replacement.VoidRequestID = "replacement", "replacement-order", "replacement-auth", "replacement-void"
	store := &replacementBeforeClaimStore{intentStoreFixture: &intentStoreFixture{records: map[string]IntentRecord{intentKey(expected.JobID, expected.ScopeVersion): expected}}, replacement: replacement}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
			_, _ = fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"tracking","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		default:
			t.Errorf("delayed callback contacted the old provider order: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "tracking", Status: "verified"}})
	service.SetIntentStore(store)
	if err := service.Authorize(context.Background(), state, expected.OrderID); err == nil {
		t.Fatal("delayed callback accepted a replaced payment attempt")
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "created" {
		t.Fatalf("replacement order was claimed: status=%q err=%v", status, err)
	}
}

func TestReleaseCannotClaimReplacementCreatedAfterItsRead(t *testing.T) {
	state, expected := authorizationFixture()
	expected.Status, expected.AuthorizationID = "authorized", "old-authorization"
	replacement := expected
	replacement.OrderRequestID, replacement.AuthRequestID, replacement.VoidRequestID = "replacement-order", "replacement-auth", "replacement-void"
	replacement.AuthorizationID = "replacement-authorization"
	store := &replacementBeforeClaimStore{intentStoreFixture: &intentStoreFixture{records: map[string]IntentRecord{intentKey(expected.JobID, expected.ScopeVersion): expected}}, replacement: replacement}
	service := &Service{}
	service.SetIntentStore(store)
	if err := service.ReleaseAuthorization(context.Background(), expected); err == nil {
		t.Fatal("release did not detect replacement during the claim")
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "authorized" {
		t.Fatalf("replacement attempt was claimed: status=%q err=%v", status, err)
	}
}

func TestReleaseCreatedOrderDoesNotAuthorizeFunds(t *testing.T) {
	state, expected := authorizationFixture()
	expected.Status = "created"
	record := expected
	record.Status = "release_pending"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v2/checkout/orders/order":
			order := authorizationOrder()
			order["status"] = "APPROVED"
			delete(order["purchase_units"].([]any)[0].(map[string]any), "payments")
			_ = json.NewEncoder(w).Encode(order)
		default:
			t.Errorf("releasing an unapproved order attempted %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
	service := newTestMarketplaceService(server, &sellerStoreStub{})
	service.SetIntentStore(store)
	if err := service.ReleaseAuthorization(context.Background(), expected); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "deferred" {
		t.Fatalf("release status=%q err=%v", status, err)
	}
}

func TestReconcileCannotTakeOverQueuedOrderReleaseAfterProviderFailure(t *testing.T) {
	state, expected := authorizationFixture()
	expected.Status = "created"
	record := expected
	record.Status = "release_pending"
	orderLookups, authorizeCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v2/checkout/orders/order":
			orderLookups++
			if orderLookups == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			order := authorizationOrder()
			order["status"] = "APPROVED"
			delete(order["purchase_units"].([]any)[0].(map[string]any), "payments")
			_ = json.NewEncoder(w).Encode(order)
		case "/v2/checkout/orders/order/authorize":
			authorizeCalls++
			_ = json.NewEncoder(w).Encode(authorizationOrder())
		case "/v2/payments/authorizations/authorization":
			_, _ = fmt.Fprint(w, `{"id":"authorization","status":"CREATED","amount":{"value":"123.45","currency_code":"EUR"},"supplementary_data":{"related_ids":{"order_id":"order"}}}`)
		case "/v2/payments/authorizations/authorization/void":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
	service := newTestMarketplaceService(server, &sellerStoreStub{})
	service.SetIntentStore(store)
	if err := service.ReleaseAuthorization(context.Background(), expected); err == nil {
		t.Fatal("provider interruption was hidden")
	}
	if status, err := service.Reconcile(context.Background(), state); err != nil || status != "release_pending" {
		t.Errorf("queued provenance was lost: status=%q err=%v", status, err)
	}
	if authorizeCalls != 0 {
		t.Errorf("reconciliation created a hold for an order that was never authorized: authorizations=%d", authorizeCalls)
	}
	if err := service.ReleaseAuthorization(context.Background(), expected); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "deferred" {
		t.Fatalf("original queued order did not finish release: status=%q err=%v", status, err)
	}
}

func TestVoidCurrentLeavesQueuedReleaseWithItsOriginalSnapshot(t *testing.T) {
	state, record := authorizationFixture()
	record.Status = "release_pending"
	service := &Service{}
	service.SetIntentStore(&intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}})
	if err := service.VoidCurrent(context.Background(), state); err == nil {
		t.Fatal("synchronous release took over a queued attempt without its original status")
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "release_pending" {
		t.Fatalf("queued attempt changed: status=%q err=%v", status, err)
	}
}

func TestAuthorizeVerifiesTheCompleteOrderWhenProviderReturnsMinimalDetails(t *testing.T) {
	state, record := authorizationFixture()
	record.Status = "created"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
			_, _ = fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"tracking","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		case "/v2/checkout/orders/order/authorize":
			if r.Header.Get("Prefer") != "return=representation" {
				t.Error("authorize did not request the complete resource representation")
			}
			_, _ = fmt.Fprint(w, `{"id":"order","purchase_units":[{"reference_id":"job-v2","payments":{"authorizations":[{"id":"authorization","status":"CREATED","amount":{"value":"123.45","currency_code":"EUR"}}]}}]}`)
		case "/v2/checkout/orders/order":
			order := authorizationOrder()
			delete(order, "intent")
			_ = json.NewEncoder(w).Encode(order)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "tracking", Status: "verified"}})
	service.SetIntentStore(&intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}})
	if err := service.Authorize(context.Background(), state, "order"); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(context.Background(), state); err != nil || status != "authorized" {
		t.Fatalf("authorization status=%q err=%v", status, err)
	}
}

func TestTerminalLegacyAttemptCanRetryWithTheVerifiedSeller(t *testing.T) {
	for _, terminal := range []string{"voided", "denied", "expired", "deferred"} {
		t.Run(terminal, func(t *testing.T) {
			state, record := authorizationFixture()
			record.Status, record.SellerMerchantID = terminal, ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/oauth2/token":
					_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
				case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
					_, _ = fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"tracking","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
				case "/v2/checkout/orders":
					if r.Header.Get("PayPal-Request-Id") == "order-request" {
						t.Error("replacement order reused the released attempt identity")
					}
					_, _ = fmt.Fprint(w, `{"id":"replacement","links":[{"rel":"approve","href":"https://www.sandbox.paypal.com/checkoutnow?token=replacement"}]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "tracking", Status: "verified"}})
			service.SetIntentStore(&intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}})
			id, _, err := service.CreateOrder(context.Background(), state)
			if err != nil || id != "replacement" {
				t.Fatalf("legacy terminal retry id=%q err=%v", id, err)
			}
		})
	}
}
