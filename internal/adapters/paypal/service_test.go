package paypal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestRefundRetriesWithSameIdempotencyKeyAndDoesNotDoubleRefund(t *testing.T) {
	var mu sync.Mutex
	var requestIDs []string
	refundCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"test-token","expires_in":3600}`)
		case r.URL.Path == "/v2/payments/captures/cap-123/refund":
			mu.Lock()
			defer mu.Unlock()
			refundCalls++
			requestIDs = append(requestIDs, r.Header.Get("PayPal-Request-Id"))
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("PayPal-Request-Id") == "" {
				t.Errorf("refund request missing method or headers: %s %v", r.Method, r.Header)
			}
			if refundCalls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprint(w, `{"id":"refund-456","status":"COMPLETED"}`)
		default:
			t.Errorf("unexpected PayPal request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	state := workflow.State{WorkspaceID: "workspace-1", Job: workflow.Job{
		ID: "job-1", ScopeVersion: 1, Paid: true,
		Quote: workflow.Quote{TotalMinor: 12345, Currency: "EUR"},
	}}
	service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional-1", TrackingID: "track-1", MerchantID: "seller-merchant", Status: "verified"}})
	service.intents = map[string]*paypalIntent{
		"workspace-1:job-1:1": {WorkspaceID: "workspace-1", JobID: "job-1", ScopeVersion: 1, AmountMinor: 12345, Currency: "EUR", SellerMerchantID: "seller-merchant", CaptureID: "cap-123", Status: "captured"},
	}
	if _, err := service.Refund(context.Background(), state); err == nil {
		t.Fatal("temporary refund response should be treated as uncertain")
	}
	refundID, err := service.Refund(context.Background(), state)
	if err != nil || refundID != "refund-456" {
		t.Fatalf("retry refund = %q, %v", refundID, err)
	}
	refundID, err = service.Refund(context.Background(), state)
	if err != nil || refundID != "refund-456" {
		t.Fatalf("duplicate operator action = %q, %v", refundID, err)
	}
	if refundCalls != 2 || len(requestIDs) != 2 || requestIDs[0] == "" || requestIDs[0] != requestIDs[1] {
		t.Fatalf("refund calls=%d request IDs=%v; want two calls with one stable idempotency key", refundCalls, requestIDs)
	}
}

type sellerStoreStub struct {
	found          bool
	record         SellerAccount
	updatedStatus  string
	referralSelf   string
	referralAction string
}

func (s *sellerStoreStub) GetPayPalSeller(_ context.Context, accountID string) (SellerAccount, bool, error) {
	if !s.found || s.record.AccountID != accountID {
		return SellerAccount{}, false, nil
	}
	return s.record, true, nil
}

func (s *sellerStoreStub) BeginPayPalSellerReferral(_ context.Context, accountID, trackingID, requestID string) (SellerAccount, error) {
	s.found = true
	s.record = SellerAccount{AccountID: accountID, TrackingID: trackingID, RequestID: requestID, Status: "creating"}
	return s.record, nil
}

func (s *sellerStoreStub) SavePayPalSellerReferral(_ context.Context, accountID, trackingID, selfURL string) error {
	s.referralSelf = selfURL
	s.record.ReferralSelfURL = selfURL
	s.record.Status = "pending"
	return nil
}

func (s *sellerStoreStub) UpdatePayPalSellerStatus(_ context.Context, accountID, merchantID, status string, paymentsReceivable, emailConfirmed bool, scopes []string) error {
	s.updatedStatus = status
	s.record.AccountID, s.record.MerchantID, s.record.Status = accountID, merchantID, status
	s.record.PaymentsReceivable, s.record.PrimaryEmailConfirmed, s.record.Scopes = paymentsReceivable, emailConfirmed, scopes
	return nil
}

func (s *sellerStoreStub) RevokePayPalSeller(_ context.Context, merchantID string) error {
	s.record.Status = "revoked"
	return nil
}

func newTestMarketplaceService(server *httptest.Server, store *sellerStoreStub) *Service {
	return &Service{
		baseURL: server.URL, clientID: "test-platform-client", clientSecret: "test-platform-secret",
		platformMerchantID: "platform-merchant", partnerAttributionID: "BN-TEST-CODE",
		returnURL: "https://app.samenklaar.example", client: server.Client(), intents: map[string]*paypalIntent{}, marketplace: store,
	}
}

func TestMarketplaceCapturePaysTheVerifiedProfessional(t *testing.T) {
	store := &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional-1", TrackingID: "track-1", MerchantID: "seller-merchant", Status: "verified", PaymentsReceivable: true, PrimaryEmailConfirmed: true}}
	var orderCreated, authorized, captured bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"test-token","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/seller-merchant":
			if r.Header.Get("PayPal-Partner-Attribution-Id") != "BN-TEST-CODE" || !validSellerAssertion(r.Header.Get("PayPal-Auth-Assertion"), "test-platform-client", "seller-merchant") {
				t.Errorf("seller status request did not carry marketplace headers: %v", r.Header)
			}
			_, _ = fmt.Fprint(w, `{"merchant_id":"seller-merchant","tracking_id":"track-1","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		case "/v2/checkout/orders":
			var body struct {
				Intent        string `json:"intent"`
				PurchaseUnits []struct {
					Payee struct {
						MerchantID string `json:"merchant_id"`
					} `json:"payee"`
				} `json:"purchase_units"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Intent != "AUTHORIZE" || len(body.PurchaseUnits) != 1 || body.PurchaseUnits[0].Payee.MerchantID != "seller-merchant" {
				t.Errorf("order did not target the connected professional: %+v", body)
			}
			if r.Header.Get("PayPal-Partner-Attribution-Id") != "BN-TEST-CODE" || !validSellerAssertion(r.Header.Get("PayPal-Auth-Assertion"), "test-platform-client", "seller-merchant") || r.Header.Get("PayPal-Request-Id") == "" {
				t.Errorf("order request missing partner or idempotency headers: %v", r.Header)
			}
			orderCreated = true
			_, _ = fmt.Fprint(w, `{"id":"order-1","links":[{"href":"https://www.sandbox.paypal.com/checkoutnow?token=order-1","rel":"approve"}]}`)
		case "/v2/checkout/orders/order-1/authorize":
			if !validSellerAssertion(r.Header.Get("PayPal-Auth-Assertion"), "test-platform-client", "seller-merchant") || r.Header.Get("PayPal-Request-Id") == "" {
				t.Errorf("authorize request missing seller context or idempotency: %v", r.Header)
			}
			authorized = true
			_, _ = fmt.Fprint(w, `{"id":"order-1","intent":"AUTHORIZE","purchase_units":[{"reference_id":"job-1-v2","amount":{"value":"123.45","currency_code":"EUR"},"payee":{"merchant_id":"seller-merchant"},"payments":{"authorizations":[{"id":"auth-1","status":"CREATED","amount":{"value":"123.45","currency_code":"EUR"}}]}}]}`)
		case "/v2/payments/authorizations/auth-1/capture":
			var body struct {
				PaymentInstruction struct {
					DisbursementMode string `json:"disbursement_mode"`
				} `json:"payment_instruction"`
				FinalCapture bool `json:"final_capture"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.PaymentInstruction.DisbursementMode != "INSTANT" || !body.FinalCapture {
				t.Errorf("capture did not request an immediate marketplace settlement: %+v", body)
			}
			if !validSellerAssertion(r.Header.Get("PayPal-Auth-Assertion"), "test-platform-client", "seller-merchant") {
				t.Errorf("capture request missing seller assertion: %v", r.Header)
			}
			captured = true
			_, _ = fmt.Fprint(w, `{"id":"capture-1","status":"COMPLETED"}`)
		case "/v2/payments/authorizations/auth-1":
			_, _ = fmt.Fprint(w, `{"id":"auth-1","status":"CREATED","expiration_time":"2099-01-01T00:00:00Z","amount":{"value":"123.45","currency_code":"EUR"},"payee":{"merchant_id":"seller-merchant"},"supplementary_data":{"related_ids":{"order_id":"order-1"}}}`)
		default:
			t.Errorf("unexpected PayPal marketplace request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := newTestMarketplaceService(server, store)
	state := workflow.State{WorkspaceID: "workspace-1", Job: workflow.Job{
		ID: "job-1", ProfessionalAccountID: "professional-1", ScopeVersion: 2, QuoteApproved: true,
		PaymentAuthorizationTiming: workflow.PaymentAuthorizationAtBooking,
		Appointment:                &workflow.Appointment{SlotID: "slot-1"},
		Quote:                      workflow.Quote{Currency: "EUR", TotalMinor: 12345},
	}}
	orderID, approvalURL, err := service.CreateOrder(context.Background(), state)
	if err != nil || orderID != "order-1" || approvalURL == "" {
		t.Fatalf("CreateOrder() = %q, %q, %v", orderID, approvalURL, err)
	}
	if err := service.Authorize(context.Background(), state, orderID); err != nil {
		t.Fatalf("Authorize() = %v", err)
	}
	if err := service.Capture(context.Background(), state); err == nil {
		t.Fatal("booking-time authorization allowed capture before completion and payment approval")
	}
	state.Job.CompletionApproved = true
	state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
	if err := service.Capture(context.Background(), state); err != nil {
		t.Fatalf("Capture() = %v", err)
	}
	if !orderCreated || !authorized || !captured {
		t.Fatalf("marketplace operation did not complete: order=%v authorize=%v capture=%v", orderCreated, authorized, captured)
	}
}

func TestSellerReferralUsesPlatformAssertionAndRequestsRequiredPermissions(t *testing.T) {
	store := &sellerStoreStub{}
	var referralCreated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"test-token","expires_in":3600}`)
		case "/v2/customer/partner-referrals":
			var body struct {
				TrackingID string `json:"tracking_id"`
				Operations []struct {
					Preference struct {
						REST struct {
							Details struct {
								Features []string `json:"features"`
							} `json:"third_party_details"`
						} `json:"rest_api_integration"`
					} `json:"api_integration_preference"`
				} `json:"operations"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrackingID == "" || len(body.Operations) != 1 {
				t.Errorf("invalid referral payload: %+v (%v)", body, err)
			}
			features := strings.Join(body.Operations[0].Preference.REST.Details.Features, ",")
			if features != "PAYMENT,REFUND" {
				t.Errorf("requested seller features = %q", features)
			}
			if r.Header.Get("PayPal-Partner-Attribution-Id") != "BN-TEST-CODE" ||
				!validSellerAssertion(r.Header.Get("PayPal-Auth-Assertion"), "test-platform-client", "platform-merchant") ||
				r.Header.Get("PayPal-Request-Id") == "" {
				t.Errorf("referral request missing partner, platform assertion, or idempotency headers: %v", r.Header)
			}
			referralCreated = true
			_, _ = fmt.Fprint(w, `{"links":[{"href":"https://api.sandbox.paypal.com/v2/customer/partner-referrals/referral-1","rel":"self"},{"href":"https://www.sandbox.paypal.com/bizsignup/partner/entry?token=referral-1","rel":"action_url"}]}`)
		default:
			t.Errorf("unexpected PayPal onboarding request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := newTestMarketplaceService(server, store)
	actionURL, err := service.BeginSellerOnboarding(context.Background(), "professional-1")
	if err != nil || actionURL != "https://www.sandbox.paypal.com/bizsignup/partner/entry?token=referral-1" {
		t.Fatalf("BeginSellerOnboarding() = %q, %v", actionURL, err)
	}
	if !referralCreated || store.record.Status != "pending" || store.referralSelf == "" {
		t.Fatalf("referral not persisted: created=%v record=%+v", referralCreated, store.record)
	}
}

func validSellerAssertion(value, clientID, sellerID string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[2] != "" {
		return false
	}
	header, errHeader := base64.RawURLEncoding.DecodeString(parts[0])
	claims, errClaims := base64.RawURLEncoding.DecodeString(parts[1])
	if errHeader != nil || errClaims != nil {
		return false
	}
	var parsedHeader, parsedClaims map[string]string
	if json.Unmarshal(header, &parsedHeader) != nil || json.Unmarshal(claims, &parsedClaims) != nil {
		return false
	}
	return parsedHeader["alg"] == "none" && parsedClaims["iss"] == clientID && parsedClaims["payer_id"] == sellerID
}

type webhookSinkStub struct {
	events []WebhookEvent
}

func (s *webhookSinkStub) EnqueuePayPalWebhook(_ context.Context, event WebhookEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestReceiveWebhookVerifiesWithPayPalBeforeQueueing(t *testing.T) {
	sink := &webhookSinkStub{}
	verified := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = fmt.Fprint(w, `{"access_token":"test-token","expires_in":3600}`)
		case "/v1/notifications/verify-webhook-signature":
			var payload struct {
				WebhookID    string          `json:"webhook_id"`
				WebhookEvent json.RawMessage `json:"webhook_event"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.WebhookID != "hook-1" || !json.Valid(payload.WebhookEvent) {
				t.Errorf("incorrect signature verification payload: %+v (%v)", payload, err)
			}
			status := "FAILURE"
			if verified {
				status = "SUCCESS"
			}
			_, _ = fmt.Fprintf(w, `{"verification_status":%q}`, status)
		default:
			t.Errorf("unexpected PayPal request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := &Service{baseURL: server.URL, clientID: "client", clientSecret: "secret", client: server.Client(), webhookID: "hook-1", webhookSink: sink}
	body := []byte(`{"id":"WH-1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"cap-1","status":"COMPLETED","amount":{"value":"12.34","currency_code":"EUR"},"supplementary_data":{"related_ids":{"order_id":"order-1"}}}}`)
	headers := make(http.Header)
	headers.Set("PayPal-Auth-Algo", "SHA256withRSA")
	headers.Set("PayPal-Cert-Url", "https://api-m.sandbox.paypal.com/v1/notifications/certs/test")
	headers.Set("PayPal-Transmission-Id", "transmission-1")
	headers.Set("PayPal-Transmission-Sig", "signature")
	headers.Set("PayPal-Transmission-Time", "2026-10-13T10:00:00Z")
	valid, err := service.ReceiveWebhook(context.Background(), headers, body)
	if err != nil || !valid || len(sink.events) != 1 {
		t.Fatalf("ReceiveWebhook() = %v, %v, queued %d", valid, err, len(sink.events))
	}
	event := sink.events[0]
	if event.ID != "WH-1" || event.Type != "PAYMENT.CAPTURE.COMPLETED" || event.CaptureID != "cap-1" || event.OrderID != "order-1" || event.Currency != "EUR" || event.AmountMinor != 1234 {
		t.Fatalf("normalized event = %+v", event)
	}
	verified = false
	valid, err = service.ReceiveWebhook(context.Background(), headers, body)
	if err != nil || valid || len(sink.events) != 1 {
		t.Fatal("invalid webhook signature was queued")
	}
}

func TestNormalizeDisputeAndRejectInvalidWebhookAmounts(t *testing.T) {
	event, err := normalizeWebhook([]byte(`{"id":"WH-2","event_type":"CUSTOMER.DISPUTE.CREATED","resource":{"id":"dispute-1","reason":"MERCHANDISE_OR_SERVICE_NOT_RECEIVED","disputed_transactions":[{"seller_transaction_id":"cap-2"}]}}`))
	if err != nil || event.CaptureID != "cap-2" || !strings.HasPrefix(event.Type, "CUSTOMER.DISPUTE.") {
		t.Fatalf("dispute event = %+v, %v", event, err)
	}
	for _, amount := range []string{"-1.00", "1.999", "bad", "1."} {
		if _, err := parseMinorUnits(amount); err == nil {
			t.Fatalf("accepted invalid PayPal amount %q", amount)
		}
	}
}
