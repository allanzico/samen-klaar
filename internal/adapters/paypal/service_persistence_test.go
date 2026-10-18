package paypal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type intentStoreFixture struct {
	records        map[string]IntentRecord
	failRefundSave bool
}

func intentKey(job string, scope int) string { return fmt.Sprintf("%s:%d", job, scope) }
func (s *intentStoreFixture) GetPaymentIntent(_ context.Context, job string, scope int) (IntentRecord, bool, error) {
	record, ok := s.records[intentKey(job, scope)]
	return record, ok, nil
}
func (s *intentStoreFixture) CreatePaymentIntent(_ context.Context, record IntentRecord) (bool, error) {
	key := intentKey(record.JobID, record.ScopeVersion)
	if _, ok := s.records[key]; ok {
		return false, nil
	}
	s.records[key] = record
	return true, nil
}
func (s *intentStoreFixture) UpdatePaymentIntent(_ context.Context, record IntentRecord, expected ...string) error {
	key := intentKey(record.JobID, record.ScopeVersion)
	if s.failRefundSave && record.Status == "refunded" {
		s.failRefundSave = false
		return errors.New("simulated database failure")
	}
	current := s.records[key]
	if !samePaymentAttempt(intentFromRecord(current), record) {
		return errors.New("payment attempt changed")
	}
	matched := false
	for _, status := range expected {
		if current.Status == status {
			matched = true
		}
	}
	if !matched {
		return errors.New("unexpected intent state")
	}
	s.records[key] = record
	return nil
}
func (s *intentStoreFixture) ClaimPaymentIntent(_ context.Context, _, job string, scope int, from, to string) (bool, error) {
	key := intentKey(job, scope)
	record := s.records[key]
	if record.Status != from {
		return false, nil
	}
	record.Status = to
	s.records[key] = record
	return true, nil
}
func (s *intentStoreFixture) ClaimPaymentIntentAttempt(_ context.Context, expected IntentRecord, from, to string) (bool, error) {
	key := intentKey(expected.JobID, expected.ScopeVersion)
	record := s.records[key]
	if record.Status != from || !samePaymentAttempt(intentFromRecord(record), expected) {
		return false, nil
	}
	record.Status = to
	s.records[key] = record
	return true, nil
}
func (s *intentStoreFixture) RetryPaymentIntent(_ context.Context, record IntentRecord) (bool, error) {
	s.records[intentKey(record.JobID, record.ScopeVersion)] = record
	return true, nil
}

func TestPersistedIntentLoadsDoNotFillFallbackMap(t *testing.T) {
	store := &intentStoreFixture{records: map[string]IntentRecord{}}
	service := &Service{intentStore: store, intents: map[string]*paypalIntent{}}
	for i := 0; i < 200; i++ {
		job := fmt.Sprintf("job-%d", i)
		store.records[intentKey(job, 1)] = IntentRecord{WorkspaceID: "workspace", JobID: job, ScopeVersion: 1, Status: "created"}
		if _, err := service.loadIntent(context.Background(), workflow.State{WorkspaceID: "workspace", Job: workflow.Job{ID: job, ScopeVersion: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(service.intents) != 0 {
		t.Fatalf("persisted reads retained %d fallback intents", len(service.intents))
	}
}

func TestScopeVoidRejectsEveryUnresolvedPaymentStatus(t *testing.T) {
	state := workflow.State{WorkspaceID: "workspace", Job: workflow.Job{ID: "job", ScopeVersion: 1}}
	service := &Service{intents: map[string]*paypalIntent{}}
	for _, status := range []string{"created", "creating", "order_uncertain", "authorizing", "authorization_uncertain", "capturing", "capture_uncertain", "captured", "voiding", "void_uncertain", "refunding", "refund_uncertain", "authorized"} {
		service.intents[scopePaymentKey(state)] = &paypalIntent{WorkspaceID: "workspace", JobID: "job", ScopeVersion: 1, Status: status}
		if err := service.VoidCurrent(context.Background(), state); err == nil {
			t.Fatalf("unresolved status %q allowed a scope change", status)
		}
	}
	for _, status := range []string{"voided", "denied", "expired"} {
		service.intents[scopePaymentKey(state)] = &paypalIntent{WorkspaceID: "workspace", JobID: "job", ScopeVersion: 1, Status: status}
		if err := service.VoidCurrent(context.Background(), state); err != nil {
			t.Fatalf("resolved status %q blocked scope: %v", status, err)
		}
	}
}

func TestInterruptedRefundReusesSavedRequestIdentity(t *testing.T) {
	for _, afterProvider := range []bool{false, true} {
		t.Run(fmt.Sprintf("after_provider_%v", afterProvider), func(t *testing.T) {
			calls := 0
			keys := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/oauth2/token":
					_, _ = fmt.Fprint(w, `{"access_token":"token","expires_in":3600}`)
				case "/v2/payments/captures/capture/refund":
					calls++
					keys = append(keys, r.Header.Get("PayPal-Request-Id"))
					_, _ = fmt.Fprint(w, `{"id":"refund","status":"COMPLETED"}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			state := workflow.State{WorkspaceID: "workspace", Job: workflow.Job{ID: "job", ScopeVersion: 1, Paid: true, Quote: workflow.Quote{TotalMinor: 1000, Currency: "EUR"}}}
			record := IntentRecord{WorkspaceID: "workspace", JobID: "job", ScopeVersion: 1, AmountMinor: 1000, Currency: "EUR", SellerMerchantID: "seller", CaptureID: "capture", RefundRequestID: "saved-refund-request", Status: "refunding"}
			if afterProvider {
				record.Status = "captured"
			}
			store := &intentStoreFixture{records: map[string]IntentRecord{intentKey("job", 1): record}, failRefundSave: afterProvider}
			service := newTestMarketplaceService(server, &sellerStoreStub{})
			service.SetIntentStore(store)
			if afterProvider {
				if _, err := service.Refund(context.Background(), state); err == nil {
					t.Fatal("simulated failed save was hidden")
				}
			}
			id, err := service.Refund(context.Background(), state)
			if err != nil || id != "refund" || store.records[intentKey("job", 1)].Status != "refunded" {
				t.Fatalf("refund=%q status=%q err=%v", id, store.records[intentKey("job", 1)].Status, err)
			}
			if len(keys) != calls || calls == 0 {
				t.Fatal("no provider refund was sent")
			}
			for _, key := range keys {
				if key != "saved-refund-request" {
					t.Fatalf("request identity changed: %q", key)
				}
			}
		})
	}
}
