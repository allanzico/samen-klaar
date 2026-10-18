package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/httpapi"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"
)

type journeyAI struct{}

func (journeyAI) Clarify(_ context.Context, _, description string) (workflow.IntakeSuggestion, error) {
	return workflow.InferIntake(description), nil
}
func (journeyAI) ReviewEvidence(_ context.Context, job workflow.Job, files []workflow.EvidenceBlob) (*workflow.EvidenceReview, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no uploaded evidence")
	}
	return &workflow.EvidenceReview{Summary: "The submitted photo and work note are ready for customer review.", Limitations: "This does not establish workmanship or safety.", Model: "contract-test", ReviewedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

type storedEvidence struct {
	data        []byte
	contentType string
}
type journeyEvidence struct {
	mu    sync.Mutex
	files map[string]storedEvidence
}

func (s *journeyEvidence) Put(_ context.Context, job, asset, ct string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[job+"/"+asset] = storedEvidence{append([]byte(nil), data...), ct}
	return nil
}
func (s *journeyEvidence) Get(_ context.Context, job, asset string) (io.ReadCloser, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[job+"/"+asset]
	if !ok {
		return nil, "", fmt.Errorf("not found")
	}
	return io.NopCloser(bytes.NewReader(f.data)), f.contentType, nil
}
func (s *journeyEvidence) Delete(_ context.Context, job, asset string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, job+"/"+asset)
}

type paymentTransport struct {
	mu            sync.Mutex
	base          string
	trackingID    string
	orders        map[string]string
	references    map[string]string
	authorizes    int
	failAuthorize bool
	captures      int
	failCapture   bool
	voids         int
}

func (p *paymentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var result any
	switch {
	case r.URL.Path == "/v1/oauth2/token":
		result = map[string]any{"access_token": "contract-token", "expires_in": 3600}
	case r.URL.Path == "/v2/customer/partner-referrals" && r.Method == "POST":
		var payload struct {
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
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.Operations) != 1 || r.Header.Get("PayPal-Auth-Assertion") == "" {
			return nil, fmt.Errorf("invalid marketplace referral contract")
		}
		features := payload.Operations[0].Preference.REST.Details.Features
		if len(features) != 2 || features[0] != "PAYMENT" || features[1] != "REFUND" || r.Header.Get("PayPal-Partner-Attribution-Id") != "BN-CONTRACT" {
			return nil, fmt.Errorf("referral did not request the sandbox payment/refund permissions")
		}
		result = map[string]any{"links": []any{
			map[string]string{"rel": "self", "href": "https://api-m.sandbox.paypal.com/v2/customer/partner-referrals/referral-1"},
			map[string]string{"rel": "action_url", "href": "https://www.sandbox.paypal.com/bizsignup/partner/entry?token=referral-1"},
		}}
	case r.URL.Path == "/v1/customer/partners/platform-merchant/merchant-integrations/seller-merchant" && r.Method == "GET":
		if p.trackingID == "" || r.Header.Get("PayPal-Auth-Assertion") == "" {
			return nil, fmt.Errorf("seller eligibility check is missing tracking or platform authorization")
		}
		result = map[string]any{
			"merchant_id": "seller-merchant", "tracking_id": p.trackingID,
			"payments_receivable": true, "primary_email_confirmed": true,
			"oauth_integrations": []any{map[string]any{
				"integration_type": "OAUTH_THIRD_PARTY", "integration_method": "PAYPAL",
				"oauth_third_party": []any{map[string]any{
					"partner_client_id": "contract-client",
					"scopes":            []string{"https://uri.paypal.com/services/payments/payment/authcapture", "https://uri.paypal.com/services/payments/refund"},
				}},
			}},
		}
	case r.URL.Path == "/v2/checkout/orders" && r.Method == "POST":
		var payload struct {
			Intent string `json:"intent"`
			Units  []struct {
				ReferenceID string `json:"reference_id"`
				Amount      struct {
					Value    string `json:"value"`
					Currency string `json:"currency_code"`
				} `json:"amount"`
				Payee struct {
					MerchantID string `json:"merchant_id"`
				} `json:"payee"`
			} `json:"purchase_units"`
			Context struct {
				ReturnURL string `json:"return_url"`
			} `json:"application_context"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Intent != "AUTHORIZE" || len(payload.Units) != 1 || payload.Units[0].Payee.MerchantID != "seller-merchant" || r.Header.Get("PayPal-Request-Id") == "" || r.Header.Get("PayPal-Auth-Assertion") == "" {
			return nil, fmt.Errorf("invalid order contract")
		}
		id := fmt.Sprintf("ORDER-%d", len(p.orders)+1)
		p.orders[id] = payload.Units[0].Amount.Value
		if p.references == nil {
			p.references = map[string]string{}
		}
		p.references[id] = payload.Units[0].ReferenceID
		back, err := url.Parse(payload.Context.ReturnURL)
		if err != nil || !strings.HasPrefix(payload.Context.ReturnURL, p.base+"/") || back.Query().Get("workspaceId") == "" {
			return nil, fmt.Errorf("order return URL must identify the request workspace")
		}
		result = map[string]any{"id": id, "links": []any{map[string]string{"rel": "approve", "href": "https://www.sandbox.paypal.com/checkoutnow?token=" + id}}}
	case strings.HasSuffix(r.URL.Path, "/authorize"):
		if r.Header.Get("PayPal-Auth-Assertion") == "" {
			return nil, fmt.Errorf("authorization omitted the seller assertion")
		}
		p.authorizes++
		if p.failAuthorize {
			p.failAuthorize = false
			return nil, fmt.Errorf("response lost after upstream authorization")
		}
		result = p.authorizedOrder(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/checkout/orders/"), "/authorize"))
	case strings.HasPrefix(r.URL.Path, "/v2/checkout/orders/") && r.Method == "GET":
		result = p.authorizedOrder(strings.TrimPrefix(r.URL.Path, "/v2/checkout/orders/"))
	case strings.HasSuffix(r.URL.Path, "/capture"):
		var payload struct {
			Amount struct {
				Value string `json:"value"`
			} `json:"amount"`
			Instruction struct {
				DisbursementMode string `json:"disbursement_mode"`
			} `json:"payment_instruction"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Amount.Value != p.orders["ORDER-1"] || payload.Instruction.DisbursementMode != "INSTANT" || r.Header.Get("PayPal-Request-Id") == "" || r.Header.Get("PayPal-Auth-Assertion") == "" {
			return nil, fmt.Errorf("capture differs from agreed amount or omits seller settlement")
		}
		p.captures++
		if p.failCapture {
			p.failCapture = false
			return nil, fmt.Errorf("response lost after upstream capture")
		}
		result = map[string]string{"id": "CAPTURE-1", "status": "COMPLETED"}
	case strings.HasSuffix(r.URL.Path, "/AUTH-1") && r.Method == "GET":
		status := "CREATED"
		related := map[string]string{"order_id": "ORDER-1"}
		if p.captures > 0 {
			status = "CAPTURED"
			related["capture_id"] = "CAPTURE-1"
		} else if p.voids > 0 {
			status = "VOIDED"
		}
		result = map[string]any{"id": "AUTH-1", "status": status, "amount": map[string]string{"value": p.orders["ORDER-1"], "currency_code": "EUR"}, "supplementary_data": map[string]any{"related_ids": related}}
	case strings.HasSuffix(r.URL.Path, "/void"):
		p.voids++
		result = map[string]any{}
	default:
		return nil, fmt.Errorf("unexpected payment API call: %s %s", r.Method, r.URL.Path)
	}
	data, _ := json.Marshal(result)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
}

func (p *paymentTransport) authorizedOrder(orderID string) map[string]any {
	amount := map[string]string{"value": p.orders[orderID], "currency_code": "EUR"}
	return map[string]any{"id": orderID, "intent": "AUTHORIZE", "status": "COMPLETED", "purchase_units": []any{map[string]any{
		"reference_id": p.references[orderID], "amount": amount, "payee": map[string]string{"merchant_id": "seller-merchant"},
		"payments": map[string]any{"authorizations": []any{map[string]any{"id": "AUTH-1", "status": "CREATED", "amount": amount}}},
	}}}
}

// The complete journey uses the production HTTP handlers, PostgreSQL store,
// opaque session lookup and PayPal adapter. External responses are deterministic
// contract fixtures; these tests do not claim to verify Auth0 or a PayPal wallet.
func TestFullJourney(t *testing.T) {
	runFullJourney(t, journeyOptions{})
}

func TestBookingTimeAuthorizationRecoveryJourney(t *testing.T) {
	runFullJourney(t, journeyOptions{bookingTime: true})
}

func TestNegotiatedBookingTimeAuthorizationRecoveryJourney(t *testing.T) {
	runFullJourney(t, journeyOptions{bookingTime: true, negotiatedDate: true})
}

func TestCustomerProposedBookingTimeAuthorizationRecoveryJourney(t *testing.T) {
	runFullJourney(t, journeyOptions{bookingTime: true, negotiatedDate: true, customerProposesDate: true})
}

func TestCancelledBookingRejectsLateAuthorizationCallback(t *testing.T) {
	runFullJourney(t, journeyOptions{bookingTime: true, cancelAfterUncertain: true})
}

func TestAgentsNegotiateBookAndEscalate(t *testing.T) {
	runFullJourney(t, journeyOptions{agents: true})
}

func TestGeneratedAvailabilityBookingJourney(t *testing.T) {
	runFullJourney(t, journeyOptions{generatedTime: true})
}

type journeyOptions struct {
	bookingTime, negotiatedDate, customerProposesDate, cancelAfterUncertain bool
	generatedTime, agents                                                   bool
}

func runFullJourney(t *testing.T, options journeyOptions) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("run make check-flow for the full PostgreSQL-backed journey")
	}
	ctx := context.Background()
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + listener.Addr().String()

	var issuer *httptest.Server
	issuer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer.URL + "/", "authorization_endpoint": issuer.URL + "/authorize", "token_endpoint": issuer.URL + "/token", "jwks_uri": issuer.URL + "/keys"})
	}))
	defer issuer.Close()
	auth, err := identity.New(oidc.ClientContext(ctx, issuer.Client()), identity.Config{IssuerURL: issuer.URL + "/", ClientID: "contract-client", ClientSecret: "contract-secret", RedirectURL: base + "/api/auth/callback", AppBaseURL: base}, store)
	if err != nil {
		t.Fatal(err)
	}
	cookies := map[string]*http.Cookie{}
	accounts := map[string]string{}
	for _, role := range []string{"customer", "professional", "operator", "outsider"} {
		user, e := store.UpsertIdentity(ctx, "contract-test", rand.Text(), role+"@example.test", "Flow "+role)
		if e != nil {
			t.Fatal(e)
		}
		accounts[role] = user.ID
		defer pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, user.ID)
		active := identity.Role(role)
		if role == "outsider" {
			active = identity.Customer
		}
		if role == "operator" {
			_, e = pool.Exec(ctx, `INSERT INTO account_memberships(account_id,role,status) VALUES ($1,'operator','active')`, user.ID)
		} else {
			_, e = store.EnsureMembership(ctx, user.ID, active)
		}
		if e != nil {
			t.Fatal(e)
		}
		token := rand.Text()
		hash := sha256.Sum256([]byte(token))
		if e = store.CreateSession(ctx, hex.EncodeToString(hash[:]), user.ID, active, time.Now().Add(time.Hour)); e != nil {
			t.Fatal(e)
		}
		cookies[role] = &http.Cookie{Name: "app_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
		// Agents are on by default. The manual journeys press every button themselves, so their people pause them.
		if !options.agents && role != "operator" {
			agentRole := role
			if role == "outsider" {
				agentRole = "customer"
			}
			if _, e = store.SaveAgentMandate(ctx, user.ID, agentRole, workflow.AgentMandate{Enabled: false}); e != nil {
				t.Fatal(e)
			}
		}
	}
	t.Setenv("PAYPAL_ENV", "sandbox")
	t.Setenv("PAYPAL_CLIENT_ID", "contract-client")
	t.Setenv("PAYPAL_CLIENT_SECRET", "contract-secret")
	t.Setenv("PAYPAL_PLATFORM_MERCHANT_ID", "platform-merchant")
	t.Setenv("PAYPAL_PARTNER_ATTRIBUTION_ID", "BN-CONTRACT")
	t.Setenv("APP_BASE_URL", base)
	transport := &paymentTransport{base: base, orders: map[string]string{}, failCapture: true}
	payments, err := paypal.New("SamenKlaar", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	payments.SetIntentStore(store)
	payments.SetMarketplaceStore(store)
	store.SetAuthorizationReleaser(payments)
	evidence := &journeyEvidence{files: map[string]storedEvidence{}}
	server := httpapi.New(httpapi.Dependencies{AppName: "SamenKlaar", WebDirectory: envOr("WEB_DIR", "../../dist"), AppBaseURL: base, Store: store, Auth: auth, AI: journeyAI{}, Evidence: evidence, Payments: payments, Marketplace: payments, Directory: store, Professionals: store, Operators: store, Agents: store, Providers: httpapi.ProviderConfig{AIProvider: "contract_test", EvidenceProvider: "r2", PaymentProvider: "paypal_sandbox"}})
	var currentHandler atomic.Value
	currentHandler.Store(http.HandlerFunc(server.Handler().ServeHTTP))
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentHandler.Load().(http.Handler).ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second}
	go httpServer.Serve(listener)
	defer httpServer.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	request := func(role, method, path string, input any, status int) []byte {
		t.Helper()
		var body io.Reader
		if input != nil {
			raw, _ := json.Marshal(input)
			body = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, base+path, body)
		req.AddCookie(cookies[role])
		req.Header.Set("Origin", base)
		req.Header.Set("Content-Type", "application/json")
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode != status {
			t.Fatalf("%s %s as %s: HTTP %d, want %d: %s", method, path, role, response.StatusCode, status, raw)
		}
		return raw
	}
	decodeState := func(raw []byte) workflow.State {
		t.Helper()
		var s workflow.State
		if e := json.Unmarshal(raw, &s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	// One customer identity can start professional onboarding and return to its
	// customer pane, but neither a role parameter nor onboarding grants operator access.
	request("outsider", "POST", "/api/auth/role", map[string]string{"role": "professional"}, 403)
	request("outsider", "POST", "/api/auth/role", map[string]string{"role": "operator"}, 403)
	request("outsider", "POST", "/api/professional/onboarding", map[string]string{"role": "operator", "status": "active"}, 200)
	var onboardingSession struct {
		Account     identity.User         `json:"account"`
		ActiveRole  identity.Role         `json:"activeRole"`
		Memberships []identity.Membership `json:"memberships"`
	}
	json.Unmarshal(request("outsider", "GET", "/api/auth/session", nil, 200), &onboardingSession)
	if onboardingSession.Account.ID != accounts["outsider"] || onboardingSession.ActiveRole != identity.Professional {
		t.Fatal("onboarding created a separate identity or failed to switch panes")
	}
	var hasCustomer, hasPendingProfessional bool
	for _, membership := range onboardingSession.Memberships {
		if membership.Role == identity.Operator {
			t.Fatal("onboarding granted operator privileges")
		}
		hasCustomer = hasCustomer || membership.Role == identity.Customer && membership.Status == "active"
		hasPendingProfessional = hasPendingProfessional || membership.Role == identity.Professional && membership.Status == "pending"
	}
	if !hasCustomer || !hasPendingProfessional {
		t.Fatal("onboarding lost customer access or prematurely activated provider access")
	}
	request("outsider", "POST", "/api/actions", map[string]string{"action": "start-job", "actor": "customer"}, 403)
	request("outsider", "POST", "/api/auth/role", map[string]string{"role": "customer"}, 200)
	// Professional onboarding and operator approval use the same public endpoints as the browser.
	raw := request("professional", "POST", "/api/professional/profile", workflow.ProfessionalProfile{DisplayName: "Flow plumber", Category: "Plumbing", ServiceName: "Sink leak repair", ServiceDescription: "Repair the leaking connection", ServiceArea: "Amsterdam", Currency: "EUR", PriceMinor: 11000, CalloutMinor: 2000, DurationMinutes: 60, MaterialsPolicy: "Replacement parts require approval", Exclusions: []string{"Wall repairs"}}, 200)
	var profile struct {
		Profile workflow.ProfessionalProfile `json:"profile"`
	}
	json.Unmarshal(raw, &profile)
	if options.generatedTime {
		// Working hours are available during onboarding, before approval and
		// marketplace payout eligibility have completed.
		request("customer", "PUT", "/api/professional/schedule", map[string]any{}, 403)
		request("professional", "PUT", "/api/professional/schedule", workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 1440, HorizonDays: 7, Week: []workflow.WorkingRange{}}, 200)
		for _, invalid := range []struct {
			input   map[string]any
			message string
		}{
			{map[string]any{"timezone": "Unknown/Nowhere"}, "Choose a valid timezone."},
			{map[string]any{"gapMinutes": 121}, "Choose a gap from 0 to 120 minutes."},
			{map[string]any{"noticeMinutes": 0}, "Choose notice from 1 hour to 14 days."},
			{map[string]any{"horizonDays": 91}, "Choose a booking window from 7 to 90 days."},
			{map[string]any{"week": []workflow.WorkingRange{{Weekday: 1, Start: "08:00", End: "17:00"}, {Weekday: 1, Start: "08:00", End: "17:00"}}}, "Choose one time range per day."},
			{map[string]any{"week": []workflow.WorkingRange{{Weekday: 1, Start: "08:01", End: "17:00"}}}, "Choose times in 15-minute steps."},
			{map[string]any{"week": []workflow.WorkingRange{{Weekday: 1, Start: "17:00", End: "08:00"}}}, "The end must be after the start."},
			{map[string]any{"week": []workflow.WorkingRange{{Weekday: 1, Start: "08:00", End: "08:15"}}}, "This day is shorter than one visit."},
		} {
			input := map[string]any{"timezone": "Europe/Amsterdam", "gapMinutes": 30, "noticeMinutes": 1440, "horizonDays": 7, "week": []workflow.WorkingRange{}}
			for field, value := range invalid.input {
				input[field] = value
			}
			body := request("professional", "POST", "/api/professional/schedule/preview", input, 400)
			if !strings.Contains(string(body), invalid.message) {
				t.Fatalf("schedule validation message=%s want %q", body, invalid.message)
			}
		}
	}
	completedReviewChecks := workflow.ProfessionalReviewChecks{
		IdentityBusiness: "verified", PayPalOnboarding: "verified", Qualification: "verified", Insurance: "verified",
	}
	request("customer", "POST", "/api/operator/professionals/"+profile.Profile.ID+"/review", map[string]any{"decision": "approve", "checks": completedReviewChecks}, 403)
	request("operator", "POST", "/api/operator/professionals/"+profile.Profile.ID+"/review", map[string]any{"decision": "approve", "checks": completedReviewChecks}, 409)
	partialReviewChecks := workflow.ProfessionalReviewChecks{
		IdentityBusiness: "verified", PayPalOnboarding: "pending", Qualification: "verified", Insurance: "verified",
	}
	request("operator", "POST", "/api/operator/professionals/"+profile.Profile.ID+"/review", map[string]any{"decision": "save", "checks": partialReviewChecks}, 200)
	request("operator", "POST", "/api/operator/professionals/"+profile.Profile.ID+"/review", map[string]any{"decision": "approve", "checks": partialReviewChecks}, 409)
	queueRaw := request("operator", "GET", "/api/operator/queue", nil, 200)
	var reviewQueue struct {
		Applications         []workflow.ProfessionalApplication   `json:"professionalApplications"`
		NotificationDelivery *workflow.NotificationDeliveryHealth `json:"notificationDelivery"`
	}
	if err := json.Unmarshal(queueRaw, &reviewQueue); err != nil {
		t.Fatal(err)
	}
	if reviewQueue.NotificationDelivery == nil {
		t.Fatal("operator queue omitted notification delivery health")
	}
	partialReviewPersisted := false
	for _, application := range reviewQueue.Applications {
		if application.Profile.ID == profile.Profile.ID {
			partialReviewPersisted = application.Profile.Status == "pending" && application.Checks == partialReviewChecks
			break
		}
	}
	if !partialReviewPersisted {
		t.Fatal("incomplete operator checks were not persisted while approval stayed pending")
	}
	var referral struct {
		ActionURL string `json:"actionUrl"`
	}
	json.Unmarshal(request("professional", "POST", "/api/professional/paypal/onboarding", map[string]any{}, 200), &referral)
	if !strings.Contains(referral.ActionURL, "sandbox.paypal.com") {
		t.Fatalf("professional onboarding returned an unsafe or empty PayPal link: %q", referral.ActionURL)
	}
	if err := pool.QueryRow(ctx, `SELECT tracking_id FROM paypal_seller_accounts WHERE account_id=$1`, accounts["professional"]).Scan(&transport.trackingID); err != nil {
		t.Fatal(err)
	}
	var payoutStatus struct {
		Connected bool   `json:"connected"`
		Status    string `json:"status"`
	}
	json.Unmarshal(request("professional", "GET", "/api/professional/paypal/status?merchantIdInPayPal=seller-merchant", nil, 200), &payoutStatus)
	if !payoutStatus.Connected || payoutStatus.Status != "verified" {
		t.Fatalf("seller eligibility did not complete: %+v", payoutStatus)
	}
	request("operator", "POST", "/api/operator/professionals/"+profile.Profile.ID+"/review", map[string]any{"decision": "approve", "checks": completedReviewChecks}, 200)
	// Self-booking is excluded from matching and rejected even for a cached result.
	request("professional", "POST", "/api/auth/role", map[string]string{"role": "customer"}, 200)
	var ownRequest struct {
		Workspace workflow.State `json:"workspace"`
	}
	json.Unmarshal(request("professional", "POST", "/api/workspaces", map[string]any{}, 200), &ownRequest)
	for _, step := range []struct{ action, value string }{{"start-intake", "My kitchen sink is leaking"}, {"answer-intake", "The drain is backing up"}, {"answer-intake", "As soon as possible"}, {"answer-intake", "My budget is EUR 120"}, {"set-location", "Amsterdam"}} {
		ownRequest.Workspace = decodeState(request("professional", "POST", "/api/actions", map[string]string{"workspaceId": ownRequest.Workspace.WorkspaceID, "action": step.action, "value": step.value}, 200))
	}
	if ownRequest.Workspace.Intake.Step != "waitlist" || ownRequest.Workspace.Intake.NeedsReview || len(ownRequest.Workspace.Intake.Matches) != 0 {
		t.Fatal("provider's own service was shown or assessment blocked matching")
	}
	staleMatches, _ := json.Marshal([]workflow.Match{{ID: profile.Profile.ID, Name: "Flow plumber", Category: "Plumbing"}})
	if _, err = pool.Exec(ctx, `UPDATE account_workspaces SET state_json=jsonb_set(state_json,'{intake,matches}',$2::jsonb) WHERE id=$1`, ownRequest.Workspace.WorkspaceID, staleMatches); err != nil {
		t.Fatal(err)
	}
	rejection := request("professional", "POST", "/api/actions", map[string]string{"workspaceId": ownRequest.Workspace.WorkspaceID, "action": "select-match", "value": profile.Profile.ID}, 409)
	if !strings.Contains(string(rejection), "own services") {
		t.Fatal("API did not reject cached self selection")
	}
	request("professional", "POST", "/api/auth/role", map[string]string{"role": "professional"}, 200)
	// An urgent assessment takes the earliest published two-day visit and waits
	// for an explicit combined price/date approval before reserving anything.
	earliestStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	var published struct {
		ID string `json:"id"`
	}
	if options.generatedTime {
		schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 1440, HorizonDays: 7}
		for weekday := 1; weekday <= 7; weekday++ {
			schedule.Week = append(schedule.Week, workflow.WorkingRange{Weekday: weekday, Start: "08:00", End: "17:00"})
		}
		request("professional", "PUT", "/api/professional/schedule", schedule, 200)
		var window workflow.ProfessionalAvailabilityWindow
		if err := json.Unmarshal(request("professional", "GET", "/api/professional/availability?days=7", nil, 200), &window); err != nil {
			t.Fatal(err)
		}
		if len(window.Slots) == 0 || window.Slots[0].Source != "schedule" {
			t.Fatal("weekly working hours did not publish bookable generated times")
		}
		published.ID = window.Slots[0].SlotID
		earliestStart = mustParseJourneyTime(t, window.Slots[0].StartUTC)
	} else {
		json.Unmarshal(request("professional", "POST", "/api/professional/availability", workflow.Appointment{StartUTC: earliestStart.Format(time.RFC3339), EndUTC: earliestStart.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}, 200), &published)
	}
	if options.agents {
		wide := map[string]any{"enabled": true, "weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "startHour": 0, "endHour": 24}
		customerMandate := func(limit int64) map[string]any {
			m := map[string]any{"maxTotalMinor": limit, "serviceAddress": "Damrak 1, 1012 LG Amsterdam", "bookWithoutAsking": true}
			for k, v := range wide {
				m[k] = v
			}
			return m
		}
		proMandate := map[string]any{"autoConfirmVisits": true}
		for k, v := range wide {
			proMandate[k] = v
		}
		later := earliestStart.Add(26 * time.Hour)
		request("professional", "POST", "/api/professional/availability", workflow.Appointment{StartUTC: later.Format(time.RFC3339), EndUTC: later.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}, 200)
		request("professional", "PUT", "/api/agent/mandate", proMandate, 200)
		request("customer", "PUT", "/api/agent/mandate", map[string]any{"enabled": true, "maxTotalMinor": 99999999}, 400)
		request("customer", "PUT", "/api/agent/mandate", customerMandate(20000), 200)
		actor := "customer" // the customer account whose intake and state the helpers below use
		runIntake := func() string {
			var created struct {
				Workspace workflow.State `json:"workspace"`
			}
			json.Unmarshal(request(actor, "POST", "/api/workspaces", map[string]any{}, 200), &created)
			id := created.Workspace.WorkspaceID
			for _, step := range []struct{ action, value string }{{"start-intake", "My kitchen sink is leaking underneath"}, {"answer-intake", "Water runs but the pipe drips underneath"}, {"answer-intake", "This is an emergency"}, {"answer-intake", "My budget is EUR 120"}, {"set-location", "Amsterdam"}} {
				request(actor, "POST", "/api/actions", map[string]string{"workspaceId": id, "action": step.action, "value": step.value}, 200)
			}
			return id
		}
		waitFor := func(id string, ok func(workflow.State) bool) workflow.State {
			t.Helper()
			var last workflow.State
			for i := 0; i < 80; i++ {
				last = decodeState(request(actor, "GET", "/api/state?workspaceId="+id, nil, 200))
				if ok(last) {
					return last
				}
				time.Sleep(250 * time.Millisecond)
			}
			t.Logf("professional activity: %s", request("professional", "GET", "/api/agent/activity?workspaceId="+id, nil, 200))
			t.Logf("customer questions: %s", request(actor, "GET", "/api/agent/escalations", nil, 200))
			t.Logf("customer activity: %s", request(actor, "GET", "/api/agent/activity?workspaceId="+id, nil, 200))
			t.Fatalf("agents did not reach the expected state; status=%+v", last.Job.AppointmentConfirmation)
			return last
		}
		// The people only describe the job. Their agents choose, book and confirm inside the mandates.
		booked := waitFor(runIntake(), func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "confirmed"
		})
		if !booked.Job.QuoteApproved || booked.Job.Appointment == nil || booked.Job.PaymentAuthorizationTiming != workflow.PaymentAuthorizationAfterCompletion {
			t.Fatalf("agents did not book inside the mandate: %+v", booked.Job)
		}
		var activity []workflow.AgentActivity
		json.Unmarshal(request("customer", "GET", "/api/agent/activity?workspaceId="+booked.WorkspaceID, nil, 200), &activity)
		codes := map[string]bool{}
		for _, item := range activity {
			codes[item.Code] = item.Outcome == "applied"
		}
		if !codes["select-match"] || !codes["set-address"] || !codes["book-visit"] {
			t.Fatalf("customer agent activity is missing steps: %+v", activity)
		}
		json.Unmarshal(request("professional", "GET", "/api/agent/activity?workspaceId="+booked.WorkspaceID, nil, 200), &activity)
		// The chat said "budget EUR 120" against a 130.00 price, so the customer agent offered 120.00
		// and the professional agent, whose floor is 90 percent, accepted before confirming the visit.
		proCodes := map[string]bool{}
		for _, item := range activity {
			proCodes[item.Code] = item.Outcome == "applied"
		}
		if len(activity) != 3 || !proCodes["accept-price"] || !proCodes["confirm-visit"] || !proCodes["tell-confirmed"] || booked.Job.Quote.TotalMinor != 12000 {
			t.Fatalf("professional agent activity is wrong: %+v total %d", activity, booked.Job.Quote.TotalMinor)
		}
		request("outsider", "GET", "/api/agent/activity?workspaceId="+booked.WorkspaceID, nil, 404)
		// A tighter mandate means the agent may not book: it stops and asks its person.
		request("customer", "PUT", "/api/agent/mandate", customerMandate(5000), 200)
		blockedID := runIntake()
		var escalations []workflow.AgentEscalation
		for i := 0; i < 80 && len(escalations) == 0; i++ {
			json.Unmarshal(request("customer", "GET", "/api/agent/escalations", nil, 200), &escalations)
			time.Sleep(250 * time.Millisecond)
		}
		if len(escalations) != 1 || escalations[0].WorkspaceID != blockedID || escalations[0].Code != "no_match_within_limits" {
			t.Fatalf("expected one escalation for no match within the limit, got %+v", escalations)
		}
		stopped := decodeState(request("customer", "GET", "/api/state?workspaceId="+blockedID, nil, 200))
		if stopped.Job.Appointment != nil || stopped.Job.QuoteApproved {
			t.Fatal("an agent booked outside its mandate")
		}
		// Raising the limit is the person's decision; the agent then finishes the booking.
		request("customer", "PUT", "/api/agent/mandate", customerMandate(20000), 200)
		waitFor(blockedID, func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "confirmed"
		})
		json.Unmarshal(request("customer", "GET", "/api/agent/escalations", nil, 200), &escalations)
		if len(escalations) != 0 {
			t.Fatalf("the escalation should resolve once the agent can act: %+v", escalations)
		}
		// Slightly under the price: the customer agent counters at its limit and the professional agent,
		// whose floor is 90 percent, accepts. Then the customer agent books the agreed price.
		third := earliestStart.Add(50 * time.Hour)
		request("professional", "POST", "/api/professional/availability", workflow.Appointment{StartUTC: third.Format(time.RFC3339), EndUTC: third.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}, 200)
		request("customer", "PUT", "/api/agent/mandate", customerMandate(12000), 200)
		negotiated := waitFor(runIntake(), func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "confirmed"
		})
		if negotiated.Job.Quote.TotalMinor != 12000 || negotiated.Job.ScopeVersion != 2 {
			t.Fatalf("agents did not settle on the counter-offer: total=%d version=%d", negotiated.Job.Quote.TotalMinor, negotiated.Job.ScopeVersion)
		}
		// Reschedules, declines and cancellations. Every request below stays inside the mandates; the people
		// only press the buttons a person would press (propose a time, decline, cancel). A second customer
		// keeps these requests clear of the first customer's intake rate limit and open questions.
		second, e := store.UpsertIdentity(ctx, "contract-test", rand.Text(), "customer2@example.test", "Flow customer two")
		if e != nil {
			t.Fatal(e)
		}
		defer pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, second.ID)
		if _, e = store.EnsureMembership(ctx, second.ID, identity.Customer); e != nil {
			t.Fatal(e)
		}
		secondToken := rand.Text()
		secondHash := sha256.Sum256([]byte(secondToken))
		if e = store.CreateSession(ctx, hex.EncodeToString(secondHash[:]), second.ID, identity.Customer, time.Now().Add(time.Hour)); e != nil {
			t.Fatal(e)
		}
		cookies["customer2"] = &http.Cookie{Name: "app_session", Value: secondToken, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
		request("customer2", "PUT", "/api/agent/mandate", customerMandate(12000), 200)
		actor = "customer2"
		for _, offset := range []time.Duration{100, 110, 120, 130} {
			at := earliestStart.Add(offset * time.Hour)
			request("professional", "POST", "/api/professional/availability", workflow.Appointment{StartUTC: at.Format(time.RFC3339), EndUTC: at.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}, 200)
		}
		proMandateWith := func(autoConfirm, autoAccept bool) map[string]any {
			m := map[string]any{"autoConfirmVisits": autoConfirm, "autoAcceptDates": autoAccept}
			for k, v := range wide {
				m[k] = v
			}
			return m
		}
		agentNotices := func(requestID, role string) int {
			t.Helper()
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE job_id=$1 AND event_type='agent-question' AND recipient_role=$2`, requestID, role).Scan(&count); err != nil {
				t.Fatal(err)
			}
			return count
		}
		waitQuestion := func(role, workspaceID, code string) {
			t.Helper()
			var open []workflow.AgentEscalation
			for i := 0; i < 80; i++ {
				json.Unmarshal(request(role, "GET", "/api/agent/escalations", nil, 200), &open)
				for _, item := range open {
					if item.WorkspaceID == workspaceID && item.Code == code {
						return
					}
				}
				time.Sleep(250 * time.Millisecond)
			}
			t.Fatalf("%s agent did not ask %q on %s: %+v", role, code, workspaceID, open)
		}
		confirmed := func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "confirmed"
		}
		visitAt := func(at time.Time) func(workflow.State) bool {
			return func(s workflow.State) bool {
				return confirmed(s) && s.Job.Appointment != nil && s.Job.Appointment.StartUTC == at.Format(time.RFC3339)
			}
		}
		proposeVisit := func(role, workspaceID string, at time.Time) {
			request(role, "POST", "/api/actions", map[string]any{
				"workspaceId": workspaceID, "action": "propose-date", "value": "Could we move the visit?",
				"proposedAppointment": workflow.Appointment{StartUTC: at.Format(time.RFC3339), EndUTC: at.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"},
			}, 200)
		}

		// The customer asks for a new time. Without auto-accept the professional's agent only asks (and emails);
		// once the professional allows it the agent accepts a time inside the hours.
		moved := waitFor(runIntake(), confirmed)
		request("professional", "PUT", "/api/agent/mandate", proMandateWith(true, false), 200)
		newTime := earliestStart.Add(160 * time.Hour)
		proposeVisit(actor, moved.WorkspaceID, newTime)
		waitQuestion("professional", moved.WorkspaceID, "awaiting_reply")
		if agentNotices(moved.Job.ID, "professional") != 1 {
			t.Fatal("the professional was not notified that their agent had a question")
		}
		request("professional", "PUT", "/api/agent/mandate", proMandateWith(true, true), 200)
		moved = waitFor(moved.WorkspaceID, visitAt(newTime))
		// The professional asks for another time and the customer's agent accepts it.
		newTime = earliestStart.Add(170 * time.Hour)
		proposeVisit("professional", moved.WorkspaceID, newTime)
		moved = waitFor(moved.WorkspaceID, visitAt(newTime))
		// Less than 24 hours before the visit a change belongs to the operator team: the agent asks and
		// does not accept, so no review is opened on its behalf.
		soon := time.Now().UTC().Add(5 * time.Hour).Truncate(time.Minute)
		if _, err := pool.Exec(ctx, `UPDATE account_workspaces SET state_json=jsonb_set(jsonb_set(state_json,'{job,appointment,startUtc}',to_jsonb($2::text)),'{job,appointment,endUtc}',to_jsonb($3::text)) WHERE id=$1`, moved.WorkspaceID, soon.Format(time.RFC3339), soon.Add(time.Hour).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		proposeVisit("professional", moved.WorkspaceID, earliestStart.Add(180*time.Hour))
		waitQuestion(actor, moved.WorkspaceID, "late_change_needs_you")
		late := decodeState(request(actor, "GET", "/api/state?workspaceId="+moved.WorkspaceID, nil, 200))
		if late.CancellationReview != nil || late.Job.DateProposal == nil || late.Job.DateProposal.Status != "pending" || late.Job.Appointment.StartUTC != soon.Format(time.RFC3339) {
			t.Fatalf("an agent decided a late visit change: review=%+v proposal=%+v", late.CancellationReview, late.Job.DateProposal)
		}

		// The professional declines a reserved visit: the customer's agent books another time, not the same one.
		request("professional", "PUT", "/api/agent/mandate", proMandateWith(false, true), 200)
		declining := waitFor(runIntake(), func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "pending" && s.Job.Appointment != nil
		})
		waitQuestion("professional", declining.WorkspaceID, "awaiting_reply")
		declinedStart := declining.Job.Appointment.StartUTC
		request("professional", "POST", "/api/actions", map[string]string{"workspaceId": declining.WorkspaceID, "action": "decline-appointment"}, 200)
		rebooked := waitFor(declining.WorkspaceID, func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "pending" && s.Job.Appointment != nil && s.Job.Appointment.StartUTC != declinedStart
		})
		if rebooked.Job.Appointment.StartUTC == declinedStart {
			t.Fatal("the customer's agent booked the declined time again")
		}
		request("professional", "PUT", "/api/agent/mandate", proMandateWith(true, true), 200)
		waitFor(declining.WorkspaceID, confirmed)

		// The professional cancels: the customer's agent goes back to the matches, does not pick the same
		// professional again, and tells its person by email and in the app that nobody else fits.
		cancelled := waitFor(runIntake(), confirmed)
		request("professional", "POST", "/api/actions", map[string]string{"workspaceId": cancelled.WorkspaceID, "action": "cancel-request", "value": "I cannot take this job after all."}, 200)
		waitQuestion(actor, cancelled.WorkspaceID, "no_match_within_limits")
		back := decodeState(request(actor, "GET", "/api/state?workspaceId="+cancelled.WorkspaceID, nil, 200))
		if back.Cancelled || back.Job.ID != "" || back.Intake == nil || back.Intake.Step != "matches" {
			t.Fatalf("the customer's agent did not return to the matches: cancelled=%v job=%q", back.Cancelled, back.Job.ID)
		}
		if agentNotices(cancelled.WorkspaceID, "customer") != 1 {
			t.Fatal("the customer was not notified that their agent needs them")
		}

		actor = "customer"
		// Too far apart: both agents counter once, then the customer's agent hands the decision to its person.
		request("customer", "PUT", "/api/agent/mandate", customerMandate(10000), 200)
		stuckID := runIntake()
		escalations = nil
		for i := 0; i < 80 && len(escalations) == 0; i++ {
			json.Unmarshal(request("customer", "GET", "/api/agent/escalations", nil, 200), &escalations)
			time.Sleep(250 * time.Millisecond)
		}
		if len(escalations) != 1 || escalations[0].WorkspaceID != stuckID || escalations[0].Code != "price_above_limit" {
			t.Fatalf("expected a price escalation after the counters, got %+v", escalations)
		}
		stuck := decodeState(request("customer", "GET", "/api/state?workspaceId="+stuckID, nil, 200))
		if stuck.Job.QuoteApproved || stuck.Job.Appointment != nil || stuck.Job.Quote.TotalMinor != 13000 {
			t.Fatalf("agents committed money outside the mandate: %+v", stuck.Job.Quote)
		}
		// With no published free time left, the customer agent proposes a visit at its preferred hour and
		// the professional agent accepts it, or counters once inside its own hours. Both run as a third customer, clear of the others' intake limits and questions.
		if _, err := pool.Exec(ctx, `UPDATE professional_availability_slots SET status='cancelled' WHERE professional_account_id=$1 AND status='available'`, accounts["professional"]); err != nil {
			t.Fatal(err)
		}
		extra, e := store.UpsertIdentity(ctx, "contract-test", rand.Text(), "customer3@example.test", "Flow customer three")
		if e != nil {
			t.Fatal(e)
		}
		defer pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, extra.ID)
		if _, e = store.EnsureMembership(ctx, extra.ID, identity.Customer); e != nil {
			t.Fatal(e)
		}
		extraToken := rand.Text()
		extraHash := sha256.Sum256([]byte(extraToken))
		if e = store.CreateSession(ctx, hex.EncodeToString(extraHash[:]), extra.ID, identity.Customer, time.Now().Add(time.Hour)); e != nil {
			t.Fatal(e)
		}
		cookies["customer3"] = &http.Cookie{Name: "app_session", Value: extraToken, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
		actor = "customer3"
		// A brand-new customer has set nothing and their assistant is already talking to them. Everything below
		// happens in the chat: it asks for the address, asks when they are free, compares that with the open
		// times, asks whether one suits them, and only then books. The professional's agent confirms.
		// Fixed local hours, so the offered times never straddle midnight whatever time of day the test runs.
		localZone, _ := time.LoadLocation("Europe/Amsterdam")
		baseDay := earliestStart.Add(200 * time.Hour).In(localZone)
		for _, at := range []time.Time{
			time.Date(baseDay.Year(), baseDay.Month(), baseDay.Day(), 10, 0, 0, 0, localZone),
			time.Date(baseDay.Year(), baseDay.Month(), baseDay.Day()+1, 14, 0, 0, 0, localZone),
		} {
			request("professional", "POST", "/api/professional/availability", workflow.Appointment{StartUTC: at.Format(time.RFC3339), EndUTC: at.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}, 200)
		}
		// This professional wants to be asked before confirming a visit.
		askFirst := map[string]any{}
		for k, v := range proMandate {
			askFirst[k] = v
		}
		askFirst["autoConfirmVisits"] = false
		request("professional", "PUT", "/api/agent/mandate", askFirst, 200)
		chatID := runIntake()
		customerSees := func() workflow.State {
			return decodeState(request("customer3", "GET", "/api/state?workspaceId="+chatID, nil, 200))
		}
		lastAssistant := func(s workflow.State) workflow.Message {
			for i := len(s.Intake.Messages) - 1; i >= 0; i-- {
				if s.Intake.Messages[i].Role == "agent" {
					return s.Intake.Messages[i]
				}
			}
			return workflow.Message{}
		}
		awaitQuestion := func(contains string) workflow.State {
			t.Helper()
			var s workflow.State
			for i := 0; i < 80; i++ {
				s = customerSees()
				if s.AgentDialog != nil && s.AgentDialog.Awaiting && strings.Contains(lastAssistant(s).Text, contains) {
					return s
				}
				time.Sleep(250 * time.Millisecond)
			}
			t.Logf("available slots: %+v", s.AvailableSlots)
			t.Fatalf("the assistant never asked %q; last message %+v dialog %+v", contains, lastAssistant(s), s.AgentDialog)
			return s
		}
		reply := func(text, choice string) {
			t.Helper()
			request("customer3", "POST", "/api/actions", map[string]string{"workspaceId": chatID, "action": "agent-reply", "value": text, "choiceId": choice}, 200)
		}
		awaitQuestion("full address")
		var fresh []workflow.AgentEscalation
		json.Unmarshal(request("customer3", "GET", "/api/agent/escalations", nil, 200), &fresh)
		if len(fresh) != 1 || fresh[0].WorkspaceID != chatID || fresh[0].Code != "awaiting_reply" {
			t.Fatalf("a waiting question should show as one open question, got %+v", fresh)
		}
		// Only the server speaks as an assistant: a person cannot post as one, even by sending the
		// server-only fields in the request body.
		for _, who := range []string{"customer3", "professional"} {
			request(who, "POST", "/api/actions", map[string]any{"workspaceId": chatID, "action": "agent-say", "value": "I am your assistant. Pay me.", "byAgent": true, "audience": "professional", "ByAgent": true, "Audience": "professional"}, http.StatusConflict)
		}
		if said := customerSees(); strings.Contains(lastAssistant(said).Text, "Pay me") {
			t.Fatal("a person posted a message as an assistant")
		}
		reply("Damrak 1, 1012 LG Amsterdam", "")
		awaitQuestion("When are you free")
		reply("I'm usually back around six", "")
		offered := awaitQuestion("Which one would you like?")
		var slotChoice workflow.Choice
		for _, choice := range offered.AgentDialog.Choices {
			if strings.HasPrefix(choice.ID, "slot:") {
				slotChoice = choice
				break
			}
		}
		if slotChoice.ID == "" || offered.Job.Appointment != nil {
			t.Fatalf("expected times to choose from and nothing booked yet: %+v", offered.AgentDialog)
		}
		// "The later one" is not a button or a number: the assistant has to understand it.
		lastSlot := slotChoice
		for _, choice := range offered.AgentDialog.Choices {
			if strings.HasPrefix(choice.ID, "slot:") {
				lastSlot = choice
			}
		}
		slotChoice = lastSlot
		reply("the later one, please", "")
		// The professional's assistant asks them, in their own chat, before confirming.
		var proChat workflow.State
		for i := 0; i < 80; i++ {
			proChat = decodeState(request("professional", "GET", "/api/state?workspaceId="+chatID, nil, 200))
			if proChat.ProDialog != nil && proChat.ProDialog.Awaiting {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if proChat.ProDialog == nil || !proChat.ProDialog.Awaiting || len(proChat.ProDialog.Choices) != 2 {
			t.Fatalf("the professional's assistant did not ask before confirming: %+v", proChat.ProDialog)
		}
		if customerNow := customerSees(); customerNow.ProDialog != nil {
			t.Fatal("the customer can see the professional's private dialog")
		}
		request("professional", "POST", "/api/actions", map[string]string{"workspaceId": chatID, "action": "agent-reply", "value": "Yes", "choiceId": "yes"}, 200)
		var chatBooked workflow.State
		for i := 0; i < 80; i++ {
			chatBooked = customerSees()
			if chatBooked.Job.AppointmentConfirmation != nil && chatBooked.Job.AppointmentConfirmation.Status == "confirmed" && strings.Contains(lastAssistant(chatBooked).Text, "confirmed") {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if chatBooked.Job.Appointment == nil || "slot:"+chatBooked.Job.Appointment.SlotID != slotChoice.ID || !strings.Contains(lastAssistant(chatBooked).Text, "confirmed") {
			t.Logf("professional activity: %s", request("professional", "GET", "/api/agent/activity?workspaceId="+chatID, nil, 200))
			t.Logf("professional questions: %s", request("professional", "GET", "/api/agent/escalations", nil, 200))
			t.Logf("professional state: %s", request("professional", "GET", "/api/state?workspaceId="+chatID, nil, 200))
			t.Fatalf("the chosen time was not chatBooked and confirmed: %+v / %+v", chatBooked.Job.Appointment, lastAssistant(chatBooked))
		}
		// The professional reads the agents' exchange but never the customer's private chat with their assistant.
		proView := decodeState(request("professional", "GET", "/api/state?workspaceId="+chatID, nil, 200))
		for _, message := range proView.Intake.Messages {
			if message.Audience == "customer" {
				t.Fatalf("the professional can see the customer's private chat: %+v", message)
			}
		}
		if proView.AgentDialog != nil {
			t.Fatal("the professional can see the customer's stated availability")
		}
		reported := false
		for _, message := range proView.Intake.Messages {
			reported = reported || message.Role == "agent" && message.Audience == "professional" && strings.HasPrefix(message.Text, "Confirmed ")
		}
		if !reported {
			t.Fatalf("the professional's assistant did not report what it agreed: %+v", proView.Intake.Messages)
		}
		for _, message := range customerSees().Intake.Messages {
			if message.Audience == "professional" {
				t.Fatalf("the customer can see the professional's private chat: %+v", message)
			}
		}
		// A suspended account's agent must not act in its name: the API refuses that account, and so does the runner.
		if _, err := pool.Exec(ctx, `UPDATE account_memberships SET status='disabled' WHERE account_id=$1 AND role='professional'`, accounts["professional"]); err != nil {
			t.Fatal(err)
		}
		if _, proParty, err := store.AgentParties(ctx, chatID); err != nil || proParty.Active {
			t.Fatalf("a disabled professional's agent is still active: %+v %v", proParty, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, accounts["professional"]); err != nil {
			t.Fatal(err)
		}
		if _, proParty, err := store.AgentParties(ctx, chatID); err != nil || !proParty.Active {
			t.Fatalf("an active professional's agent is not active: %+v %v", proParty, err)
		}
		// After booking the assistants stay in the conversation. The customer asks for another day in their own
		// words, the assistant confirms before asking the professional, and the professional's agent accepts.
		said := func(s workflow.State, audience, contains string) bool {
			for _, message := range s.Intake.Messages {
				if message.Role == "agent" && message.Audience == audience && strings.Contains(message.Text, contains) {
					return true
				}
			}
			return false
		}
		waitSaid := func(role, workspaceID, audience, contains string) workflow.State {
			t.Helper()
			var s workflow.State
			for i := 0; i < 80; i++ {
				s = decodeState(request(role, "GET", "/api/state?workspaceId="+workspaceID, nil, 200))
				if said(s, audience, contains) {
					return s
				}
				time.Sleep(250 * time.Millisecond)
			}
			t.Fatalf("%s's assistant never said %q: %+v", role, contains, s.Intake.Messages)
			return s
		}
		customerAction := func(workspaceID, action, value, choice string) {
			t.Helper()
			request("customer3", "POST", "/api/actions", map[string]string{"workspaceId": workspaceID, "action": action, "value": value, "choiceId": choice}, 200)
		}
		askFirst["autoAcceptDates"] = true
		request("professional", "PUT", "/api/agent/mandate", askFirst, 200)
		newDay := earliestStart.Add(10 * 24 * time.Hour).In(mustLoadJourneyZone(t))
		newStart := time.Date(newDay.Year(), newDay.Month(), newDay.Day(), 10, 0, 0, 0, newDay.Location())
		customerAction(chatID, "agent-reply", fmt.Sprintf("Can we do %d %s 10-12 instead?", newDay.Day(), strings.ToLower(newDay.Month().String()[:3])), "")
		asked := awaitQuestion("Shall I ask ")
		if asked.Job.DateProposal != nil && asked.Job.DateProposal.Status == "pending" || asked.Job.Appointment.StartUTC != chatBooked.Job.Appointment.StartUTC {
			t.Fatal("the assistant asked the professional before the customer said yes")
		}
		if !strings.Contains(lastAssistant(asked).Text, newStart.Format("Mon 2 Jan, 15:04")) || len(asked.AgentDialog.Choices) != 2 {
			t.Fatalf("unexpected question about the move: %+v", lastAssistant(asked))
		}
		reply("Yes, ask Flow", "yes")
		var rescheduled workflow.State
		for i := 0; i < 80; i++ {
			rescheduled = customerSees()
			if rescheduled.Job.Appointment != nil && rescheduled.Job.Appointment.StartUTC == newStart.UTC().Format(time.RFC3339) && rescheduled.Job.AppointmentConfirmation.Status == "confirmed" && said(rescheduled, "customer", "Flow confirmed") && rescheduled.Job.DateProposal != nil && rescheduled.Job.DateProposal.Actor == "customer" {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if rescheduled.Job.Appointment == nil || rescheduled.Job.Appointment.StartUTC != newStart.UTC().Format(time.RFC3339) || !said(rescheduled, "customer", "Flow confirmed") {
			t.Logf("professional activity: %s", request("professional", "GET", "/api/agent/activity?workspaceId="+chatID, nil, 200))
			t.Fatalf("the visit was not rescheduled: appointment=%+v proposal=%+v last=%+v", rescheduled.Job.Appointment, rescheduled.Job.DateProposal, lastAssistant(rescheduled))
		}
		noted := false
		for _, message := range decodeState(request("professional", "GET", "/api/state?workspaceId="+chatID, nil, 200)).Intake.Messages {
			noted = noted || message.Code == "customer:reschedule-visit" && message.Audience == "both"
		}
		if !noted {
			t.Fatal("the professional did not see the customer's assistant ask for the move")
		}

		// The professional marks the work complete: the customer's assistant summarises it and asks whether to
		// approve. Payment is never the assistant's: it points to the PayPal step and the professional's
		// assistant reports the approval and the payment.
		request("professional", "POST", "/api/actions", map[string]string{"workspaceId": chatID, "action": "start-job"}, 200)
		request("professional", "POST", "/api/actions", map[string]string{"workspaceId": chatID, "action": "add-evidence", "value": "Replaced the leaking connection and checked it under running water."}, 200)
		uploadJourneyEvidence(t, client, base, chatID, cookies["professional"])
		request("professional", "POST", "/api/actions", map[string]string{"workspaceId": chatID, "action": "submit-evidence"}, 200)
		review := awaitQuestion("says the work is done")
		for _, want := range []string{"1 file", "Replaced the leaking connection", "Shall I approve it?", "only you can do, in PayPal"} {
			if !strings.Contains(lastAssistant(review).Text, want) {
				t.Fatalf("the work question is missing %q: %q", want, lastAssistant(review).Text)
			}
		}
		if review.Job.CompletionApproved || len(review.AgentDialog.Choices) != 2 {
			t.Fatal("the assistant approved the work before the customer answered")
		}
		waitQuestion("customer3", chatID, "awaiting_reply")
		reply("Yes, approve it", "yes")
		approved := waitSaid("customer3", chatID, "customer", "The work is approved")
		if !approved.Job.CompletionApproved || approved.Job.PaymentApprovedVersion != 0 || approved.Job.Paid {
			t.Fatalf("the assistant went beyond approving the work: %+v", approved.Job)
		}
		linked := false
		for _, message := range approved.Intake.Messages {
			linked = linked || message.Code == "link:payment" && message.Audience == "customer"
		}
		if !linked {
			t.Fatal("the payment message has no link to the payment step")
		}
		waitQuestion("customer3", chatID, "payment_needs_you")
		waitSaid("professional", chatID, "professional", "approved the finished work")
		request("customer3", "POST", "/api/actions", map[string]string{"workspaceId": chatID, "action": "approve-payment"}, 200)
		var paying struct {
			OrderID string `json:"orderId"`
		}
		json.Unmarshal(request("customer3", "POST", "/api/paypal/create-order?workspaceId="+chatID, map[string]any{}, 200), &paying)
		request("customer3", "POST", "/api/paypal/authorize?workspaceId="+chatID, map[string]string{"orderId": paying.OrderID}, 200)
		request("customer3", "POST", "/api/paypal/capture?workspaceId="+chatID, map[string]any{}, 409)
		request("customer3", "POST", "/api/paypal/reconcile?workspaceId="+chatID, map[string]any{}, 200)
		paid := waitSaid("customer3", chatID, "customer", "is complete")
		if !paid.Job.Paid {
			t.Fatal("the reconciled payment was not recorded")
		}
		proReport := waitSaid("professional", chatID, "professional", "has arrived")
		if !said(proReport, "professional", "approved the payment of") || !said(paid, "customer", "payment of") {
			t.Fatalf("the assistants did not report the payment: %+v", proReport.Intake.Messages)
		}
		// Free the unused time so the next scenarios still start without published times.
		if _, err := pool.Exec(ctx, `UPDATE professional_availability_slots SET status='cancelled' WHERE professional_account_id=$1 AND status='available'`, accounts["professional"]); err != nil {
			t.Fatal(err)
		}
		amsterdam, _ := time.LoadLocation("Europe/Amsterdam")
		withHours := func(base map[string]any, from, to int, extra map[string]any) map[string]any {
			m := map[string]any{}
			for k, v := range base {
				m[k] = v
			}
			m["startHour"], m["endHour"] = from, to
			for k, v := range extra {
				m[k] = v
			}
			return m
		}
		confirmedState := func(s workflow.State) bool {
			return s.Job.AppointmentConfirmation != nil && s.Job.AppointmentConfirmation.Status == "confirmed"
		}
		request("customer3", "PUT", "/api/agent/mandate", withHours(customerMandate(20000), 0, 24, map[string]any{"preferredHour": 20}), 200)
		request("professional", "PUT", "/api/agent/mandate", withHours(proMandate, 0, 24, map[string]any{"autoAcceptDates": true}), 200)
		proposed := waitFor(runIntake(), confirmedState)
		if hour := mustParseJourneyTime(t, proposed.Job.Appointment.StartUTC).In(amsterdam).Hour(); hour != 20 || proposed.Job.DateProposal == nil || proposed.Job.DateProposal.Actor != "customer" {
			t.Fatalf("expected the customer agent's proposal at 20:00 to be accepted, got hour %d proposal %+v", hour, proposed.Job.DateProposal)
		}
		// The professional works 09:00 to 17:00, so it counters with the latest time inside its hours, and the
		// customer agent, whose hours reach into the afternoon, accepts the counter.
		request("customer3", "PUT", "/api/agent/mandate", withHours(customerMandate(20000), 8, 22, map[string]any{"preferredHour": 20}), 200)
		request("professional", "PUT", "/api/agent/mandate", withHours(proMandate, 9, 17, map[string]any{"autoAcceptDates": true}), 200)
		countered := waitFor(runIntake(), confirmedState)
		if hour := mustParseJourneyTime(t, countered.Job.Appointment.StartUTC).In(amsterdam).Hour(); hour != 16 && hour != 9 || countered.Job.DateProposal == nil || countered.Job.DateProposal.Actor != "professional" {
			t.Fatalf("expected the professional agent's counter at 16:00 (or 09:00 when 16:00 is under 24 hours away) to be accepted, got hour %d proposal %+v", hour, countered.Job.DateProposal)
		}
		// A request the built-in reading cannot place goes to the AI, which only returns an intent; the assistant
		// confirms before it cancels, and the professional is told.
		customerAction(countered.WorkspaceID, "agent-reply", "Thanks, but I sorted it out myself", "")
		cancelQuestion := waitSaid("customer3", countered.WorkspaceID, "customer", "cancel the visit with Flow")
		if cancelQuestion.Cancelled {
			t.Fatal("the assistant cancelled before the customer confirmed")
		}
		customerAction(countered.WorkspaceID, "agent-reply", "Yes, cancel it", "yes")
		waitSaid("customer3", countered.WorkspaceID, "customer", "is cancelled")
		gone := decodeState(request("customer3", "GET", "/api/state?workspaceId="+countered.WorkspaceID, nil, 200))
		if !gone.Cancelled || gone.CancelledBy != "customer" {
			t.Fatalf("the confirmed cancellation was not applied: %+v", gone.CancelledBy)
		}
		return
	}
	var urgentRequest struct {
		Workspace workflow.State `json:"workspace"`
	}
	json.Unmarshal(request("customer", "POST", "/api/workspaces", map[string]any{}, 200), &urgentRequest)
	urgentAction := func(action, value string) workflow.State {
		return decodeState(request("customer", "POST", "/api/actions", map[string]string{"workspaceId": urgentRequest.Workspace.WorkspaceID, "action": action, "value": value}, 200))
	}
	urgentAction("start-intake", "My kitchen sink is leaking underneath")
	urgentAction("answer-intake", "Water runs but the pipe drips underneath")
	urgentAction("answer-intake", "This is an emergency")
	urgent := urgentAction("answer-intake", "My budget is EUR 120")
	urgent = urgentAction("set-location", "Amsterdam")
	if urgent.Intake.Step != "matches" || urgent.Intake.NeedsReview || len(urgent.Intake.Matches) != 1 || urgent.Intake.Matches[0].EarliestAppointment == nil || urgent.Intake.Matches[0].EarliestAppointment.StartUTC != earliestStart.Format(time.RFC3339) {
		t.Fatal("urgent assessment did not offer the actual earliest visit")
	}
	urgent = urgentAction("select-match", profile.Profile.ID)
	if urgent.Job.Appointment != nil || urgent.Job.QuoteApproved {
		t.Fatal("urgent selection skipped customer approval")
	}
	urgent = urgentAction("set-service-address", "Damrak 1, 1012 LG Amsterdam")
	booking := map[string]any{"workspaceId": urgentRequest.Workspace.WorkspaceID, "action": "approve-and-book", "value": published.ID}
	if options.bookingTime {
		booking["paymentAuthorizationTiming"] = workflow.PaymentAuthorizationAtBooking
	}
	if options.negotiatedDate {
		proposer, accepter := "professional", "customer"
		approve := map[string]any{"workspaceId": urgent.WorkspaceID, "action": "approve-quote"}
		if options.customerProposesDate {
			proposer, accepter = "customer", "professional"
			approve["paymentAuthorizationTiming"] = workflow.PaymentAuthorizationAtBooking
		}
		request("customer", "POST", "/api/actions", approve, 200)
		proposal := decodeState(request(proposer, "POST", "/api/actions", map[string]any{
			"workspaceId": urgent.WorkspaceID, "action": "propose-date", "value": "The earliest published visit works for me.",
			"proposedAppointment": workflow.Appointment{StartUTC: earliestStart.Format(time.RFC3339), EndUTC: earliestStart.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"},
		}, 200))
		accept := map[string]any{
			"workspaceId": urgent.WorkspaceID, "action": "accept-date", "proposalId": proposal.Job.DateProposal.ID,
		}
		if accepter == "customer" {
			accept["paymentAuthorizationTiming"] = workflow.PaymentAuthorizationAtBooking
		}
		urgent = decodeState(request(accepter, "POST", "/api/actions", accept, 200))
	} else {
		urgent = decodeState(request("customer", "POST", "/api/actions", booking, 200))
	}
	if urgent.Job.Appointment == nil || !urgent.Job.QuoteApproved || urgent.Job.Appointment.SlotID != published.ID {
		t.Fatal("combined customer approval did not reserve the actual earliest visit")
	}
	if options.generatedTime {
		request("professional", "DELETE", "/api/professional/availability/"+published.ID, nil, 409)
		confirmed := decodeState(request("professional", "POST", "/api/actions", map[string]string{"workspaceId": urgent.WorkspaceID, "action": "confirm-appointment"}, 200))
		restored, err := store.LoadWorkspace(ctx, accounts["customer"], "customer", urgent.WorkspaceID)
		if err != nil || restored.Job.Appointment == nil || restored.Job.Appointment.SlotID != published.ID || confirmed.Job.AppointmentConfirmation.Status != "confirmed" {
			t.Fatalf("generated booking was not persisted and protected: %v", err)
		}
		limited := false
		for i := 0; i < 80; i++ {
			req, _ := http.NewRequest(http.MethodPost, base+"/api/professional/schedule/preview", strings.NewReader(`{}`))
			req.AddCookie(cookies["professional"])
			req.Header.Set("Origin", base)
			req.Header.Set("Content-Type", "application/json")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusTooManyRequests {
				limited = true
				break
			}
		}
		if !limited {
			t.Fatal("working-hours writes bypassed the per-account rate limit")
		}
		t.Log("Verified pending professional working-hours setup → generated free times → real service-area matching → customer booking → booked slot protected from removal → professional confirmation")
		return
	}
	if options.bookingTime {
		id := urgent.WorkspaceID
		originalJob, originalSlot := urgent.Job.ID, urgent.Job.Appointment.SlotID
		act := func(role, action, value string, status int) workflow.State {
			t.Helper()
			raw := request(role, "POST", "/api/actions", map[string]string{"workspaceId": id, "action": action, "value": value}, status)
			if status != http.StatusOK {
				return workflow.State{}
			}
			return decodeState(raw)
		}
		if urgent.Job.PaymentAuthorizationTiming != workflow.PaymentAuthorizationAtBooking || urgent.Job.PaymentAuthorizedVersion != 0 {
			t.Fatal("booking-time choice did not wait for verified PayPal authorization")
		}
		act("professional", "confirm-appointment", "", 409)
		request("customer", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 409)
		var order struct {
			OrderID string `json:"orderId"`
		}
		json.Unmarshal(request("customer", "POST", "/api/paypal/create-order?workspaceId="+id, map[string]any{}, 200), &order)
		transport.failAuthorize = true
		request("customer", "POST", "/api/paypal/authorize?workspaceId="+id, map[string]string{"orderId": order.OrderID}, 409)
		act("customer", "defer-payment-authorization", "", 409)
		act("professional", "confirm-appointment", "", 409)
		// Restart the adapter and handler with a fresh store, keeping only durable records.
		restartedStore, err := postgres.Open(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer restartedStore.Close()
		restartedPayments, err := paypal.New("SamenKlaar", &http.Client{Transport: transport})
		if err != nil {
			t.Fatal(err)
		}
		restartedPayments.SetIntentStore(restartedStore)
		restartedPayments.SetMarketplaceStore(restartedStore)
		restartedStore.SetAuthorizationReleaser(restartedPayments)
		restartedServer := httpapi.New(httpapi.Dependencies{AppName: "SamenKlaar", AppBaseURL: base, Store: restartedStore, Auth: auth, AI: journeyAI{}, Evidence: evidence, Payments: restartedPayments, Marketplace: restartedPayments, Directory: restartedStore, Professionals: restartedStore, Operators: restartedStore})
		currentHandler.Store(http.HandlerFunc(restartedServer.Handler().ServeHTTP))
		var recovered struct {
			State  workflow.State `json:"state"`
			Status string         `json:"status"`
		}
		if options.cancelAfterUncertain {
			cancelled := act("customer", "cancel-request", "The visit is no longer needed.", 200)
			if !cancelled.Cancelled || cancelled.Job.PaymentAuthorizedVersion != 0 {
				t.Fatal("cancellation did not clear the booking authorization projection")
			}
			request("customer", "POST", "/api/paypal/authorize?workspaceId="+id, map[string]string{"orderId": order.OrderID}, 409)
			request("customer", "POST", "/api/paypal/reconcile?workspaceId="+id, map[string]any{}, 200)
			act("professional", "confirm-appointment", "", 409)
			deadline := time.Now().Add(5 * time.Second)
			for {
				var status struct {
					Status string `json:"status"`
				}
				if err := json.Unmarshal(request("customer", "GET", "/api/paypal/status?workspaceId="+id, nil, 200), &status); err != nil {
					t.Fatal(err)
				}
				if status.Status == "voided" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("cancelled uncertain authorization was not released: %s", status.Status)
				}
				time.Sleep(20 * time.Millisecond)
			}
			persisted, err := restartedStore.LoadWorkspace(ctx, accounts["customer"], "customer", id)
			transport.mu.Lock()
			orders, authorizes, captures, voids := len(transport.orders), transport.authorizes, transport.captures, transport.voids
			transport.mu.Unlock()
			var activeReservations int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM booking_reservations WHERE job_id=$1 AND status='booked'`, originalJob).Scan(&activeReservations); err != nil {
				t.Fatal(err)
			}
			if err != nil || !persisted.Cancelled || persisted.Job.PaymentAuthorizedVersion != 0 || activeReservations != 0 || orders != 1 || authorizes != 1 || captures != 0 || voids != 1 {
				t.Fatalf("late callback changed the cancelled booking or failed to release its single original hold: %v (orders=%d authorize=%d capture=%d void=%d reservations=%d)", err, orders, authorizes, captures, voids, activeReservations)
			}
			t.Log("Verified lost authorization response → cancellation → late callback rejected → transactional queue release of original hold → no booking/capture restored")
			return
		}
		if err := json.Unmarshal(request("customer", "POST", "/api/paypal/reconcile?workspaceId="+id, map[string]any{}, 200), &recovered); err != nil {
			t.Fatal(err)
		}
		if recovered.Status != "authorized" || recovered.State.Job.PaymentAuthorizedVersion != recovered.State.Job.ScopeVersion {
			t.Fatalf("recovered PayPal authorization did not unblock the persisted booking: status=%s authorized scope=%d current scope=%d", recovered.Status, recovered.State.Job.PaymentAuthorizedVersion, recovered.State.Job.ScopeVersion)
		}
		// Replaying the browser return must reuse the recovered hold and leave one
		// authorization event on the same booking.
		request("customer", "POST", "/api/paypal/authorize?workspaceId="+id, map[string]string{"orderId": order.OrderID}, 200)
		replayed := decodeState(request("customer", "GET", "/api/state?workspaceId="+id, nil, 200))
		if len(replayed.Job.Timeline) != len(recovered.State.Job.Timeline) || replayed.Job.PaymentAuthorizedVersion != recovered.State.Job.PaymentAuthorizedVersion {
			t.Fatal("approval callback replay duplicated or invalidated the recovered authorization")
		}
		act("customer", "defer-payment-authorization", "", 409)
		confirmed := act("professional", "confirm-appointment", "", 200)
		if confirmed.Job.ID != originalJob || confirmed.Job.Appointment == nil || confirmed.Job.Appointment.SlotID != originalSlot || confirmed.Job.AppointmentConfirmation.Status != "confirmed" || transport.authorizes != 1 || len(transport.orders) != 1 {
			t.Fatal("authorization recovery replaced the booking or created another PayPal hold")
		}
		request("professional", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 403)
		request("customer", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 409)
		act("customer", "approve-payment", "", 409)
		if transport.captures != 0 {
			t.Fatal("booking-time authorization reached capture before completion approval")
		}
		act("professional", "start-job", "", 200)
		act("professional", "add-evidence", "Repaired the leaking connection and checked it under running water.", 200)
		uploadJourneyEvidence(t, client, base, id, cookies["professional"])
		exerciseRequestPhoto(t, client, base, id, cookies)
		act("professional", "submit-evidence", "", 200)
		request("customer", "POST", "/api/evidence/review?workspaceId="+id, map[string]any{}, 200)
		completed := act("customer", "approve-completion", "", 200)
		request("customer", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 409)
		if completed.Job.PaymentApprovedVersion != 0 || transport.captures != 0 {
			t.Fatal("completion approval also approved or captured the payment")
		}
		act("customer", "approve-payment", "", 200)
		// The original booking-time hold is captured after both distinct approvals.
		// Losing this response must still recover without another authorize/capture.
		request("customer", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 409)
		if err := json.Unmarshal(request("customer", "POST", "/api/paypal/reconcile?workspaceId="+id, map[string]any{}, 200), &recovered); err != nil {
			t.Fatal(err)
		}
		persisted, err := restartedStore.LoadWorkspace(ctx, accounts["professional"], "professional", id)
		if err != nil || recovered.Status != "captured" || !persisted.Job.Paid || persisted.Job.PaymentTransactionID != "CAPTURE-1" || persisted.Job.PaymentApprovedVersion != 1 || persisted.Job.PaymentAuthorizedVersion != 1 || persisted.Job.Appointment == nil || persisted.Job.Appointment.SlotID != originalSlot || transport.captures != 1 || transport.authorizes != 1 || len(transport.orders) != 1 {
			t.Fatalf("booking-time journey did not persist one hold/capture and the original booking: %v", err)
		}
		t.Log("Verified booking-time choice → lost authorization response → adapter/store restart → reconciliation → callback replay → same booking confirmed → distinct completion and payment approvals → original hold captured once")
		return
	}
	raw = request("customer", "POST", "/api/workspaces", map[string]any{}, 200)
	var created struct {
		Workspace workflow.State `json:"workspace"`
	}
	json.Unmarshal(raw, &created)
	id := created.Workspace.WorkspaceID
	act := func(role, action, value string, extra map[string]any, status int) workflow.State {
		t.Helper()
		payload := map[string]any{"workspaceId": id, "action": action, "value": value, "actor": "customer"}
		for k, v := range extra {
			payload[k] = v
		}
		raw := request(role, "POST", "/api/actions", payload, status)
		if status != 200 {
			return workflow.State{}
		}
		return decodeState(raw)
	}
	state := act("customer", "start-intake", "My kitchen sink is leaking", nil, 200)
	if state.Intake.Category != "Plumbing" {
		t.Fatal("wrong trade")
	}
	act("customer", "answer-intake", "Water runs and the pipe drips underneath", nil, 200)
	act("customer", "answer-intake", "As soon as possible", nil, 200)
	state = act("customer", "answer-intake", "My budget is EUR 120", nil, 200)
	state = act("customer", "set-location", "Amsterdam", nil, 200)
	if state.Intake.Step != "matches" || state.Intake.Location == nil || len(state.Intake.Matches) != 1 {
		t.Fatal("published service-area matching or planning without published availability failed")
	}
	state = act("customer", "select-match", profile.Profile.ID, nil, 200)
	state = act("customer", "set-service-address", "Damrak 1, 1012 LG Amsterdam", nil, 200)
	if len(state.AvailableSlots) != 0 || state.Job.Quote.TotalMinor != 13000 {
		t.Fatal("unexpected initial plan")
	}
	state = act("customer", "send-message", "Can we discuss the price before booking?", nil, 200)
	state = act("professional", "send-message", "Yes, send your proposed total below.", nil, 200)
	var customerInbox, professionalInbox struct {
		Notifications []workflow.Notification `json:"notifications"`
	}
	json.Unmarshal(request("customer", "GET", "/api/notifications", nil, 200), &customerInbox)
	json.Unmarshal(request("professional", "GET", "/api/notifications", nil, 200), &professionalInbox)
	var customerMessageNotice, professionalMessageNotice int64
	for _, item := range customerInbox.Notifications {
		if item.EventType == "send-message" && item.WorkspaceID == id {
			customerMessageNotice = item.ID
			break
		}
	}
	for _, item := range professionalInbox.Notifications {
		if item.EventType == "send-message" && item.WorkspaceID == id {
			professionalMessageNotice = item.ID
			break
		}
	}
	if customerMessageNotice == 0 || professionalMessageNotice == 0 {
		t.Fatal("transactional messages did not appear in the correct account inboxes")
	}
	request("customer", "POST", "/api/notifications/read", map[string]int64{"id": professionalMessageNotice}, 404)
	request("professional", "POST", "/api/notifications/read", map[string]int64{"id": professionalMessageNotice}, 200)
	json.Unmarshal(request("professional", "GET", "/api/notifications", nil, 200), &professionalInbox)
	readStatePersisted := false
	for _, item := range professionalInbox.Notifications {
		if item.ID == professionalMessageNotice && item.ReadAt != nil {
			readStatePersisted = true
		}
	}
	if !readStatePersisted {
		t.Fatal("notification read state did not persist for its recipient")
	}
	professionalState := decodeState(request("professional", "GET", "/api/state?workspaceId="+id, nil, 200))
	if len(professionalState.Intake.Messages) < 2 || professionalState.Intake.Messages[len(professionalState.Intake.Messages)-1].Role != "professional" || state.Job.QuoteApproved || state.Job.Appointment != nil {
		t.Fatal("conversation was not shared or message changed plan")
	}
	request("outsider", "GET", "/api/state?workspaceId="+id, nil, 503)
	act("outsider", "approve-quote", "", nil, 409)
	state = act("customer", "propose-price", "Could we keep this within EUR 100?", map[string]any{"priceAmountMinor": 10000}, 200)
	first := state.Job.PriceProposal.ID
	act("customer", "accept-price", "", map[string]any{"proposalId": first}, 409)
	act("customer", "approve-quote", "", nil, 409)
	state = act("professional", "propose-price", "EUR 120 including the visit", map[string]any{"priceAmountMinor": 12000}, 200)
	act("professional", "accept-price", "", map[string]any{"proposalId": first}, 409)
	state = act("customer", "accept-price", "", map[string]any{"proposalId": state.Job.PriceProposal.ID}, 200)
	if state.Job.Quote.TotalMinor != 12000 || state.Job.ScopeVersion != 2 {
		t.Fatal("negotiation not persisted")
	}
	act("customer", "approve-quote", "", nil, 200)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	date := func(s time.Time) map[string]any {
		return map[string]any{"proposedAppointment": workflow.Appointment{StartUTC: s.Format(time.RFC3339), EndUTC: s.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}}
	}
	state = act("customer", "propose-date", "Could you come in the morning?", date(start), 200)
	firstDate := state.Job.DateProposal.ID
	state = act("professional", "propose-date", "I can come an hour later", date(start.Add(time.Hour)), 200)
	act("professional", "accept-date", "", map[string]any{"proposalId": firstDate}, 409)
	state = act("customer", "accept-date", "", map[string]any{"proposalId": state.Job.DateProposal.ID}, 200)
	if state.Job.Appointment == nil || state.Job.Appointment.SlotID == "" {
		t.Fatal("agreed date not reserved")
	}
	// A rejected reschedule must preserve the previous booking.
	before := state.Job.Appointment.StartUTC
	state = act("customer", "propose-date", "Could we move the visit?", date(start.Add(24*time.Hour)), 200)
	state = act("professional", "decline-date", "", map[string]any{"proposalId": state.Job.DateProposal.ID}, 200)
	if state.Job.Appointment.StartUTC != before {
		t.Fatal("declined reschedule changed booked time")
	}
	// An accepted replacement must release the old slot and reserve the new one.
	oldSlot := state.Job.Appointment.SlotID
	state = act("professional", "propose-date", "I can offer the following day instead", date(start.Add(24*time.Hour)), 200)
	state = act("customer", "accept-date", "", map[string]any{"proposalId": state.Job.DateProposal.ID}, 200)
	var oldSlotStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM professional_availability_slots WHERE id=$1`, oldSlot).Scan(&oldSlotStatus); err != nil || oldSlotStatus != "available" {
		t.Fatal("accepted reschedule did not release the original slot")
	}
	bookedStart := state.Job.Appointment.StartUTC
	// Arrange another confirmed booking, then prove a conflicting replacement is atomic.
	blocked := start.Add(72 * time.Hour)
	blocker := "conflict-" + rand.Text()
	_, err = pool.Exec(ctx, `INSERT INTO booking_reservations(job_id,professional_id,slot_id,starts_at,ends_at,timezone,status) VALUES ($1,$2,$3,$4,$5,'UTC','booked')`, blocker, accounts["professional"], blocker, blocked, blocked.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	state = act("customer", "propose-date", "Try this other time", date(blocked), 200)
	act("professional", "accept-date", "", map[string]any{"proposalId": state.Job.DateProposal.ID}, 409)
	state = decodeState(request("customer", "GET", "/api/state?workspaceId="+id, nil, 200))
	if state.Job.Appointment.StartUTC != bookedStart || state.Job.DateProposal.Status != "pending" {
		t.Fatal("conflicting replacement changed the existing booking")
	}
	var persistedStart time.Time
	bookedInstant, parseErr := time.Parse(time.RFC3339, bookedStart)
	if err := pool.QueryRow(ctx, `SELECT starts_at FROM booking_reservations WHERE job_id=$1 AND status='booked'`, state.Job.ID).Scan(&persistedStart); err != nil || parseErr != nil || !persistedStart.Equal(bookedInstant) {
		t.Fatal("failed replacement released the database reservation")
	}
	state = act("professional", "decline-date", "", map[string]any{"proposalId": state.Job.DateProposal.ID}, 200)
	_, _ = pool.Exec(ctx, `DELETE FROM booking_reservations WHERE job_id=$1`, blocker)
	act("professional", "start-job", "", nil, 200)
	state = act("professional", "propose-change", "Replace the damaged valve", map[string]any{"changeAmountMinor": 2500}, 200)
	act("professional", "submit-evidence", "", nil, 409)
	act("customer", "approve-change", "", nil, 200)
	act("professional", "add-evidence", "Replaced the valve and checked the connection under running water.", nil, 200)
	act("professional", "submit-evidence", "", nil, 409)
	request("professional", "POST", "/api/actions", map[string]any{"workspaceId": id, "action": "record-evidence", "evidence": map[string]any{"assetId": "fabricated", "contentType": "image/png"}}, 400)
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	file, _ := writer.CreateFormFile("file", "work.pdf")
	file.Write([]byte("%PDF-1.4\nVerification fixture: repaired pipe.\n%%EOF"))
	writer.Close()
	req, _ := http.NewRequest("POST", base+"/api/evidence?workspaceId="+id, &upload)
	req.AddCookie(cookies["professional"])
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	uploaded, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("upload failed: %s", uploaded)
	}
	state = decodeState(uploaded)
	asset := state.Job.Evidence[len(state.Job.Evidence)-1].AssetID
	request("customer", "GET", "/api/evidence/"+asset+"?workspaceId="+id, nil, 200)
	request("outsider", "GET", "/api/evidence/"+asset+"?workspaceId="+id, nil, 503)
	state = act("professional", "submit-evidence", "", nil, 200)
	if !state.Job.DetailAdded {
		t.Fatal("completion discarded the work note")
	}
	act("customer", "approve-payment", "", nil, 409)
	request("customer", "POST", "/api/evidence/review?workspaceId="+id, map[string]any{}, 200)
	act("customer", "approve-completion", "", nil, 200)
	act("customer", "approve-payment", "", nil, 200)
	raw = request("customer", "POST", "/api/paypal/create-order?workspaceId="+id, map[string]any{}, 200)
	var order struct {
		OrderID string `json:"orderId"`
	}
	json.Unmarshal(raw, &order)
	if transport.orders[order.OrderID] != "145.00" {
		t.Fatal("PayPal did not use the negotiated total plus approved change")
	}
	request("customer", "POST", "/api/paypal/authorize?workspaceId="+id, map[string]string{"orderId": order.OrderID}, 200)
	request("professional", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 403)
	request("customer", "POST", "/api/paypal/capture?workspaceId="+id, map[string]any{}, 409)
	raw = request("customer", "POST", "/api/paypal/reconcile?workspaceId="+id, map[string]any{}, 200)
	var settled struct {
		State workflow.State `json:"state"`
	}
	json.Unmarshal(raw, &settled)
	if !settled.State.Job.Paid || settled.State.Job.PaymentTransactionID != "CAPTURE-1" || transport.captures != 1 {
		t.Fatal("capture uncertainty was not reconciled exactly once")
	}
	reviewed := decodeState(request("customer", "POST", "/api/actions", map[string]any{
		"workspaceId": id, "action": "submit-review", "rating": 5, "value": "The repair was clear and on time.",
	}, 200))
	if reviewed.Job.Review == nil || reviewed.Job.Review.Rating != 5 {
		t.Fatal("completed paid job did not accept a verified customer review")
	}
	responded := decodeState(request("professional", "POST", "/api/actions", map[string]any{
		"workspaceId": id, "action": "respond-to-review", "value": "Thank you for sharing your experience.",
	}, 200))
	if responded.Job.Review == nil || responded.Job.Review.ProfessionalResponse == "" {
		t.Fatal("assigned professional response was not saved")
	}
	// A paid job cannot be cancelled by withdrawal; a person reviews it. Operators resolve it through the API.
	withdrawn := decodeState(request("customer", "POST", "/api/actions", map[string]any{"workspaceId": id, "action": "request-withdrawal", "value": "The repair did not hold after a day."}, 200))
	if withdrawn.CancellationReview == nil || withdrawn.CancellationReview.Kind != "withdrawal" || withdrawn.CancellationReview.Status != "pending" || withdrawn.Cancelled {
		t.Fatalf("paid withdrawal was not queued for review: %+v", withdrawn.CancellationReview)
	}
	resolve := map[string]any{"kind": "withdrawal", "decision": "approve-withdrawal", "rationale": "Approve the withdrawal request."}
	request("customer", "POST", "/api/operator/cases/"+id+"/resolve", resolve, 403)
	request("operator", "POST", "/api/operator/cases/"+id+"/resolve", resolve, 409)
	request("operator", "POST", "/api/operator/cases/"+id+"/resolve", map[string]any{"kind": "withdrawal", "decision": "review-work", "rationale": "Refer the paid job to work review."}, 200)
	referred := decodeState(request("customer", "GET", "/api/state?workspaceId="+id, nil, 200))
	if referred.Cancelled || !referred.Job.Paid || referred.Job.Dispute == nil || referred.Job.Dispute.Status != "open" {
		t.Fatalf("referred withdrawal did not open a work review on the paid job: %+v", referred.Job.Dispute)
	}
	// Reload using a second store to prove persistence rather than an in-memory happy path.
	reopened, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.LoadWorkspace(ctx, accounts["professional"], "professional", id)
	if err != nil || !restored.Job.Paid || restored.Job.Quote.TotalMinor != 14500 || restored.Job.ScopeVersion != 3 || restored.Job.Appointment == nil || restored.Job.Review == nil || restored.Job.Review.ProfessionalResponse == "" {
		t.Fatalf("persisted journey failed: %v", err)
	}
	var reservations, events, intents int
	pool.QueryRow(ctx, `SELECT count(*) FROM booking_reservations WHERE job_id=$1 AND status='completed'`, restored.Job.ID).Scan(&reservations)
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE workspace_id=$1`, id).Scan(&events)
	pool.QueryRow(ctx, `SELECT count(*) FROM paypal_payment_operations WHERE job_id=$1 AND status='captured'`, restored.Job.ID).Scan(&intents)
	if reservations != 1 || events < 20 || intents != 1 {
		t.Fatalf("database evidence: completed bookings=%d audit=%d captures=%d", reservations, events, intents)
	}
	t.Logf("Verified onboarding → AI intake → location-filtered matching → price counter-offer → date counter-offer → reservation → declined reschedule → work → scope change → private upload → note → completion → payment approval → authorization → lost capture response → reconciliation → verified customer review → professional response → persisted state (%d audit events)", events)
}
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustParseJourneyTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func uploadJourneyEvidence(t *testing.T, client *http.Client, base, workspaceID string, cookie *http.Cookie) workflow.State {
	t.Helper()
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	file, err := writer.CreateFormFile("file", "work.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("%PDF-1.4\nVerification fixture: repaired pipe.\n%%EOF"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/evidence?workspaceId="+workspaceID, &upload)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upload failed: HTTP %d: %s", response.StatusCode, raw)
	}
	var state workflow.State
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func mustLoadJourneyZone(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

// InterpretReply stands in for the language model: it reads three free-form replies the built-in reading cannot.
func (journeyAI) InterpretReply(_ context.Context, q workflow.ReplyQuery) (workflow.ReplyIntent, error) {
	switch {
	case q.Stage == "availability" && strings.Contains(q.Reply, "usually back around six"):
		return workflow.ReplyIntent{Intent: "availability", Windows: []workflow.TimeWindow{{From: 0, To: 24 * 60}}}, nil
	case q.Stage == "offer" && strings.Contains(q.Reply, "the later one"):
		return workflow.ReplyIntent{Intent: "choose", Choice: len(q.Options)}, nil
	case q.Stage == "ask" && strings.Contains(q.Reply, "sorted it out myself"):
		return workflow.ReplyIntent{Intent: "cancel"}, nil
	}
	return workflow.ReplyIntent{Intent: "unclear"}, nil
}

func (journeyAI) SelectServices(_ context.Context, _ string, services []matching.Service) (matching.Selection, error) {
	for i, s := range services {
		if s.Name == "Sink leak repair" {
			return matching.Selection{Fits: []matching.Fit{{Index: i, Reason: "Published sink leak repair covers the requested work."}}}, nil
		}
	}
	return matching.Selection{Question: "Where is the leak coming from?"}, nil
}

// exerciseRequestPhoto covers customer-supplied problem photos: only the owning customer can add one,
// only the owner and the assigned professional can read it, and it is never served as job evidence.
func exerciseRequestPhoto(t *testing.T, client *http.Client, base, workspaceID string, cookies map[string]*http.Cookie) {
	t.Helper()
	t.Log("request photo checks running")
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9b\xa4\x14\x00\x00\x00\x00IEND\xaeB`\x82")
	post := func(role, name string, body []byte) (int, []byte) {
		var upload bytes.Buffer
		writer := multipart.NewWriter(&upload)
		file, _ := writer.CreateFormFile("file", name)
		_, _ = file.Write(body)
		_ = writer.Close()
		req, _ := http.NewRequest(http.MethodPost, base+"/api/request-photos?workspaceId="+workspaceID, &upload)
		req.AddCookie(cookies[role])
		req.Header.Set("Origin", base)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return response.StatusCode, raw
	}
	if status, _ := post("customer", "notes.txt", []byte("not an image")); status != http.StatusUnsupportedMediaType {
		t.Fatalf("non-image request photo returned HTTP %d", status)
	}
	if status, _ := post("professional", "x.png", png); status != http.StatusForbidden {
		t.Fatalf("professional uploaded a customer request photo: HTTP %d", status)
	}
	status, raw := post("customer", "problem.png", png)
	if status != http.StatusOK {
		t.Fatalf("customer photo upload failed: HTTP %d: %s", status, raw)
	}
	var state workflow.State
	if err := json.Unmarshal(raw, &state); err != nil || len(state.RequestPhotos) != 1 {
		t.Fatalf("request photo was not recorded: %v %+v", err, state.RequestPhotos)
	}
	get := func(role string) int {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/request-photos/"+state.RequestPhotos[0].AssetID+"?workspaceId="+workspaceID, nil)
		req.AddCookie(cookies[role])
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}
	if get("customer") != http.StatusOK || get("professional") != http.StatusOK {
		t.Fatal("the owning customer and assigned professional must read the request photo")
	}
	if get("outsider") == http.StatusOK {
		t.Fatal("an unrelated account read another customer's request photo")
	}
}
