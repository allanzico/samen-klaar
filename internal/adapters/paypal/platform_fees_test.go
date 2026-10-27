package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestPlatformFeeSettlementRecoveryAndRefund(t *testing.T) {
	for _, scenario := range []string{"paid", "lost-response", "wrong-fee", "missing-permission"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			state, record := authorizationFixture()
			state.Job.Quote.TotalMinor = 10500
			state.Job.PlatformFeeBasisPoints = workflow.PlatformFeeBasisPoints
			state.Job.CompletionApproved = true
			state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
			record.AmountMinor = 10500
			record.PlatformFeeMinor = 525
			record.Status = "authorized"
			record.AuthorizationID = "authorization"
			record.CaptureRequestID = "capture-request"
			captures, refunds := 0, 0
			captureData := func() map[string]any {
				value := "5.25"
				if scenario == "wrong-fee" {
					value = "5.00"
				}
				return map[string]any{"id": "capture", "status": "COMPLETED", "amount": map[string]string{"value": "105.00", "currency_code": "EUR"}, "seller_receivable_breakdown": map[string]any{"platform_fees": []any{map[string]any{"amount": map[string]string{"value": value, "currency_code": "EUR"}}}}}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/oauth2/token":
					fmt.Fprint(w, `{"access_token":"fixture","expires_in":3600}`)
				case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
					scopes := []string{"https://uri.paypal.com/services/payments/payment/authcapture", "https://uri.paypal.com/services/payments/refund"}
					if scenario != "missing-permission" {
						scopes = append(scopes, "https://uri.paypal.com/services/payments/partnerfee")
					}
					json.NewEncoder(w).Encode(map[string]any{"merchant_id": "seller", "tracking_id": "track", "payments_receivable": true, "primary_email_confirmed": true, "oauth_integrations": []any{map[string]any{"integration_type": "OAUTH_THIRD_PARTY", "integration_method": "PAYPAL", "oauth_third_party": []any{map[string]any{"partner_client_id": "test-platform-client", "scopes": scopes}}}}})
				case "/v2/payments/authorizations/authorization":
					status := "CREATED"
					related := map[string]string{"order_id": "order"}
					if captures > 0 {
						status = "CAPTURED"
						related["capture_id"] = "capture"
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "authorization", "status": status, "amount": map[string]string{"value": "105.00", "currency_code": "EUR"}, "supplementary_data": map[string]any{"related_ids": related}})
				case "/v2/payments/authorizations/authorization/capture":
					var body struct {
						Amount      paymentAmount `json:"amount"`
						Instruction struct {
							Mode string `json:"disbursement_mode"`
							Fees []struct {
								Amount paymentAmount `json:"amount"`
							} `json:"platform_fees"`
						} `json:"payment_instruction"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Amount.Value != "105.00" || body.Instruction.Mode != "INSTANT" || len(body.Instruction.Fees) != 1 || body.Instruction.Fees[0].Amount != (paymentAmount{Value: "5.25", Currency: "EUR"}) || r.Header.Get("PayPal-Request-Id") != record.CaptureRequestID {
						t.Error("capture changed total, fee or request identity")
					}
					captures++
					if scenario == "lost-response" {
						w.WriteHeader(503)
						return
					}
					json.NewEncoder(w).Encode(captureData())
				case "/v2/payments/captures/capture":
					json.NewEncoder(w).Encode(captureData())
				case "/v2/payments/captures/capture/refund":
					var body struct {
						Instruction struct {
							Fees []struct {
								Amount paymentAmount `json:"amount"`
							} `json:"platform_fees"`
						} `json:"payment_instruction"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Instruction.Fees) != 1 || body.Instruction.Fees[0].Amount != (paymentAmount{Value: "5.25", Currency: "EUR"}) {
						t.Error("full refund did not return the platform fee")
					}
					refunds++
					fmt.Fprint(w, `{"id":"refund","status":"COMPLETED"}`)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			service := newTestMarketplaceService(server, &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", TrackingID: "track", MerchantID: "seller", Status: "verified"}})
			saved := &intentStoreFixture{records: map[string]IntentRecord{intentKey(record.JobID, record.ScopeVersion): record}}
			service.SetIntentStore(saved)
			if scenario == "missing-permission" {
				if _, _, err := service.CreateOrder(ctx, state); err == nil || !strings.Contains(err.Error(), "platform fee") {
					t.Fatalf("missing fee permission allowed checkout: %v", err)
				}
				if captures != 0 {
					t.Fatal("capture attempted")
				}
				return
			}
			err := service.Capture(ctx, state)
			if scenario == "paid" && err != nil {
				t.Fatal(err)
			}
			if scenario != "paid" && err == nil {
				t.Fatal("uncertain settlement claimed success")
			}
			if scenario != "paid" {
				status, err := service.Reconcile(ctx, state)
				if scenario == "wrong-fee" {
					if err == nil || status == "captured" {
						t.Fatal("wrong fee was accepted")
					}
					return
				}
				if err != nil || status != "captured" {
					t.Fatalf("lost response not recovered: %s %v", status, err)
				}
			}
			if err := service.Capture(ctx, state); err != nil || captures != 1 {
				t.Fatalf("duplicate capture: %d %v", captures, err)
			}
			changed := state
			changed.Job.PlatformFeeBasisPoints = 0
			if err := service.Capture(ctx, changed); err == nil {
				t.Fatal("captured fee terms could be changed")
			}
			state.Job.Paid = true
			for range 2 {
				if id, err := service.Refund(ctx, state); err != nil || id != "refund" {
					t.Fatalf("refund: %s %v", id, err)
				}
			}
			if refunds != 1 {
				t.Fatalf("refund sent %d times", refunds)
			}
			loaded, _, _ := saved.GetPaymentIntent(ctx, record.JobID, record.ScopeVersion)
			if loaded.PlatformFeeMinor != 525 {
				t.Fatal("stored fee changed")
			}
		})
	}
}

func TestConnectedSellerCanAddFeePermissionWithoutChangingMerchant(t *testing.T) {
	store := &sellerStoreStub{found: true, record: SellerAccount{AccountID: "professional", MerchantID: "seller", TrackingID: "track", Status: "verified", RequestID: "old-request", ReferralSelfURL: "https://api-m.sandbox.paypal.com/v2/customer/partner-referrals/old"}}
	refs := 0
	key := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			fmt.Fprint(w, `{"access_token":"fixture","expires_in":3600}`)
		case "/v1/customer/partners/platform-merchant/merchant-integrations/seller":
			fmt.Fprint(w, `{"merchant_id":"seller","tracking_id":"track","payments_receivable":true,"primary_email_confirmed":true,"oauth_integrations":[{"integration_type":"OAUTH_THIRD_PARTY","integration_method":"PAYPAL","oauth_third_party":[{"partner_client_id":"test-platform-client","scopes":["https://uri.paypal.com/services/payments/payment/authcapture","https://uri.paypal.com/services/payments/refund"]}]}]}`)
		case "/v2/customer/partner-referrals":
			if refs > 0 && key != r.Header.Get("PayPal-Request-Id") {
				t.Error("upgrade retry lost idempotency")
			}
			key = r.Header.Get("PayPal-Request-Id")
			refs++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			raw, _ := json.Marshal(body)
			if !strings.Contains(string(raw), `"PARTNER_FEE"`) || body["tracking_id"] != "track" {
				t.Error("upgrade omitted consent or changed seller tracking")
			}
			fmt.Fprint(w, `{"links":[{"rel":"self","href":"https://api-m.sandbox.paypal.com/v2/customer/partner-referrals/upgrade"},{"rel":"action_url","href":"https://www.sandbox.paypal.com/bizsignup/partner/entry?token=upgrade"}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := newTestMarketplaceService(server, store)
	status, err := service.SellerOnboardingStatus(context.Background(), "professional", "")
	if err != nil || !status.Connected || status.PlatformFeeReady {
		t.Fatalf("legacy access should remain connected but need fee consent: %+v %v", status, err)
	}
	for range 2 {
		if link, err := service.BeginSellerOnboarding(context.Background(), "professional"); err != nil || !strings.Contains(link, "token=upgrade") {
			t.Fatalf("upgrade: %s %v", link, err)
		}
	}
	if refs != 2 || key == "" || key == "old-request" || store.record.MerchantID != "seller" {
		t.Fatal("upgrade reset existing payout account or reused old referral identity")
	}
}
