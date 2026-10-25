package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/httpapi"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

type autonomySessions map[string]identity.User

func (s autonomySessions) Principal(r *http.Request) (identity.Principal, bool) {
	role := r.Header.Get("Test-Role")
	user, ok := s[role]
	return identity.Principal{User: user, ActiveRole: identity.Role(role), Memberships: []identity.Membership{{Role: identity.Role(role), Status: "active"}}}, ok
}

type autonomyReviewAI struct {
	journeyAI
	unclear bool
	calls   atomic.Int32
	entered chan struct{}
	resume  chan struct{}
}

func (a *autonomyReviewAI) ReviewEvidence(ctx context.Context, job workflow.Job, files []workflow.EvidenceBlob) (*workflow.EvidenceReview, error) {
	if a.calls.Add(1) == 1 && a.entered != nil {
		close(a.entered)
		select {
		case <-a.resume:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	review := &workflow.EvidenceReview{Summary: "The repair is documented.", Limitations: "Photos cannot establish workmanship or safety.", ReviewedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, item := range workflow.ReviewableWorkItems(job) {
		coverage := "documented"
		if a.unclear {
			coverage = "unclear"
		}
		review.Coverage = append(review.Coverage, workflow.EvidenceFinding{ScopeItem: item.ID, Coverage: coverage, EvidenceIDs: []string{files[0].ID}, Observation: "The uploaded fixture documents the connection."})
	}
	return review, nil
}

type autonomyPaymentTransport struct {
	*paymentTransport
	expired atomic.Bool
}

func (p *autonomyPaymentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if p.expired.Load() && r.Method == "GET" && r.URL.Path == "/v2/payments/authorizations/AUTH-1" {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewBufferString(`{"id":"AUTH-1","status":"EXPIRED","amount":{"value":"100.00","currency_code":"EUR"}}`)), Request: r}, nil
	}
	return p.paymentTransport.RoundTrip(r)
}

func TestAutonomousCompletionAndReleaseJourney(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("run with disposable PostgreSQL")
	}
	for _, scenario := range []string{"automatic", "manual", "unclear", "revoked-during-review", "lost-capture-response", "expired-authorization"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if err := postgres.Migrate(ctx, databaseURL); err != nil {
				t.Fatal(err)
			}
			store, err := postgres.Open(ctx, databaseURL)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			pool, err := pgxpool.New(ctx, databaseURL)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			customer, err := store.UpsertIdentity(ctx, "autonomy-fixture", rand.Text(), "customer@example.test", "Test customer")
			if err != nil {
				t.Fatal(err)
			}
			professional, err := store.UpsertIdentity(ctx, "autonomy-fixture", rand.Text(), "professional@example.test", "Test professional")
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=ANY($1)`, []string{customer.ID, professional.ID})
			for _, member := range []struct {
				id   string
				role identity.Role
			}{{customer.ID, identity.Customer}, {professional.ID, identity.Professional}} {
				if _, err := store.EnsureMembership(ctx, member.id, member.role); err != nil {
					t.Fatal(err)
				}
			}
			profile, err := store.SaveProfessionalProfile(ctx, professional.ID, workflow.ProfessionalProfile{DisplayName: "Test professional", Category: "Plumbing", ServiceName: "Sink repair", ServiceArea: "Amsterdam", Currency: "EUR", PriceMinor: 10000, DurationMinutes: 60, MaterialsPolicy: "Parts agreed separately"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE account_id=$1`, professional.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, professional.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SaveAgentMandate(ctx, professional.ID, "professional", workflow.AgentMandate{Enabled: false}); err != nil {
				t.Fatal(err)
			}
			trackingID := rand.Text()
			if _, err := store.BeginPayPalSellerReferral(ctx, professional.ID, trackingID, rand.Text()); err != nil {
				t.Fatal(err)
			}
			if err := store.UpdatePayPalSellerStatus(ctx, professional.ID, "seller-merchant", "verified", true, true, []string{"https://uri.paypal.com/services/payments/payment/authcapture", "https://uri.paypal.com/services/payments/refund"}); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PAYPAL_ENV", "sandbox")
			t.Setenv("PAYPAL_CLIENT_ID", "contract-client")
			t.Setenv("PAYPAL_CLIENT_SECRET", "contract-secret")
			t.Setenv("PAYPAL_PLATFORM_MERCHANT_ID", "platform-merchant")
			t.Setenv("PAYPAL_PARTNER_ATTRIBUTION_ID", "BN-CONTRACT")
			t.Setenv("APP_BASE_URL", "http://example.test")
			transport := &autonomyPaymentTransport{paymentTransport: &paymentTransport{base: "http://example.test", trackingID: trackingID, orders: map[string]string{}, failCapture: scenario == "lost-capture-response"}}
			payments, err := paypal.New("SamenKlaar", &http.Client{Transport: transport})
			if err != nil {
				t.Fatal(err)
			}
			payments.SetIntentStore(store)
			payments.SetMarketplaceStore(store)
			ai := &autonomyReviewAI{unclear: scenario == "unclear"}
			if scenario == "revoked-during-review" {
				ai.entered = make(chan struct{})
				ai.resume = make(chan struct{})
			}
			server := httpapi.New(httpapi.Dependencies{Store: store, Sessions: autonomySessions{"customer": customer, "professional": professional}, Agents: store, Professionals: store, AI: ai, Evidence: &journeyEvidence{files: map[string]storedEvidence{}}, Payments: payments, Marketplace: payments, AppBaseURL: "http://example.test"})
			handler := server.Handler()
			send := func(role, path string, body []byte, contentType string) []byte {
				t.Helper()
				r := httptest.NewRequest("POST", path, bytes.NewReader(body))
				r.Header.Set("Origin", "http://example.test")
				r.Header.Set("Test-Role", role)
				r.Header.Set("Content-Type", contentType)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
				}
				return w.Body.Bytes()
			}
			post := func(role, path string, body any) []byte {
				data, _ := json.Marshal(body)
				return send(role, path, data, "application/json")
			}
			state, err := store.CreateWorkspace(ctx, customer.ID)
			if err != nil {
				t.Fatal(err)
			}
			state.Job = workflow.Job{ID: "autonomy-" + rand.Text(), Professional: professional.DisplayName, Customer: customer.DisplayName, ProfessionalAccountID: professional.ID, ProfessionalProfileID: profile.ID, ScopeVersion: 1, BookingAutonomyRequired: true, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 10000, DurationMinutes: 60, Lines: []workflow.QuoteLine{{Label: "Sink repair", AmountMinor: 10000, Basis: "Published service price"}}}}
			data, _ := json.Marshal(state)
			if _, err := pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, data); err != nil {
				t.Fatal(err)
			}
			action := func(role, name, value string) workflow.State {
				t.Helper()
				var result workflow.State
				json.Unmarshal(post(role, "/api/actions", map[string]string{"workspaceId": state.WorkspaceID, "action": name, "value": value}), &result)
				return result
			}
			choice := "automatic|" + workflow.BookingAutonomyKey(state.Job)
			if scenario == "manual" {
				choice = "manual"
			}
			state = action("customer", "set-booking-autonomy", choice)
			if scenario == "manual" {
				post("customer", "/api/actions", map[string]string{"workspaceId": state.WorkspaceID, "action": "set-payment-timing", "paymentAuthorizationTiming": "at_booking"})
			}
			starts := time.Now().UTC().Add(48 * time.Hour)
			var slot struct{ ID string }
			json.Unmarshal(publishJourneyTime(t, store, professional.ID, workflow.Appointment{StartUTC: starts.Format(time.RFC3339), EndUTC: starts.Add(time.Hour).Format(time.RFC3339), Timezone: "UTC"}), &slot)
			state = action("customer", "approve-and-book", slot.ID)
			var order struct {
				OrderID string `json:"orderId"`
			}
			json.Unmarshal(post("customer", "/api/paypal/create-order?workspaceId="+state.WorkspaceID, map[string]any{}), &order)
			post("customer", "/api/paypal/authorize?workspaceId="+state.WorkspaceID, map[string]string{"orderId": order.OrderID})
			action("professional", "confirm-appointment", "")
			action("professional", "start-job", "")
			var picture bytes.Buffer
			png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 16, 16)))
			var upload bytes.Buffer
			writer := multipart.NewWriter(&upload)
			part, _ := writer.CreateFormFile("file", "generated.png")
			part.Write(picture.Bytes())
			writer.Close()
			send("professional", "/api/evidence?workspaceId="+state.WorkspaceID, upload.Bytes(), writer.FormDataContentType())
			action("professional", "add-evidence", "Replaced the seal and checked the connection with running water.")
			if scenario == "expired-authorization" {
				transport.expired.Store(true)
			}
			state = action("professional", "submit-evidence", "")
			if ai.entered != nil {
				select {
				case <-ai.entered:
				case <-ctx.Done():
					t.Fatal("review never started")
				}
				state = action("customer", "set-booking-autonomy", "manual")
				close(ai.resume)
			}
			for range 3 {
				if err := server.RunAgents(ctx, state.WorkspaceID, true); err != nil {
					t.Fatal(err)
				}
			}
			restored, err := store.LoadWorkspace(ctx, customer.ID, "customer", state.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			transport.mu.Lock()
			captures, authorizes := transport.captures, transport.authorizes
			transport.mu.Unlock()
			shouldPay := scenario == "automatic" || scenario == "lost-capture-response"
			if restored.Job.Paid != shouldPay || (shouldPay && (captures != 1 || !restored.Job.CompletionApprovedByAgent || !restored.Job.PaymentApprovedByAgent || restored.Job.PaymentTransactionID != "CAPTURE-1")) || (!shouldPay && captures != 0) {
				t.Fatalf("scenario=%s paid=%v captures=%d completionByAgent=%v paymentByAgent=%v", scenario, restored.Job.Paid, captures, restored.Job.CompletionApprovedByAgent, restored.Job.PaymentApprovedByAgent)
			}
			if authorizes != 1 {
				t.Fatalf("assistant created another authorization: %d", authorizes)
			}
			if scenario == "manual" && ai.calls.Load() != 0 {
				t.Fatal("manual booking sent evidence to automatic review")
			}
			if (scenario == "unclear" || scenario == "revoked-during-review") && (restored.Job.CompletionApproved || restored.Job.PaymentApprovedVersion != 0) {
				t.Fatal("unsafe completion or payment approval")
			}
			if shouldPay {
				reopened, err := postgres.Open(ctx, databaseURL)
				if err != nil {
					t.Fatal(err)
				}
				persisted, err := reopened.LoadWorkspace(ctx, customer.ID, "customer", state.WorkspaceID)
				reopened.Close()
				if err != nil || !persisted.Job.Paid {
					t.Fatal("automatic payment did not survive restart")
				}
			}
			if _, err := store.SaveAgentMandate(ctx, customer.ID, "customer", workflow.AgentMandate{Enabled: false}); err != nil {
				t.Fatal(err)
			}
			t.Logf("Verified %s: one buyer authorization, %d captures, %d automatic evidence checks", scenario, captures, ai.calls.Load())
		})
	}
}
