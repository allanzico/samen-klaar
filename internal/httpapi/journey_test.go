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
	mu          sync.Mutex
	base        string
	trackingID  string
	orders      map[string]string
	captures    int
	failCapture bool
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
				Amount struct {
					Value string `json:"value"`
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
		back, err := url.Parse(payload.Context.ReturnURL)
		if err != nil || !strings.HasPrefix(payload.Context.ReturnURL, p.base+"/") || back.Query().Get("workspaceId") == "" {
			return nil, fmt.Errorf("order return URL must identify the request workspace")
		}
		result = map[string]any{"id": id, "links": []any{map[string]string{"rel": "approve", "href": "https://www.sandbox.paypal.com/checkoutnow?token=" + id}}}
	case strings.HasSuffix(r.URL.Path, "/authorize"):
		if r.Header.Get("PayPal-Auth-Assertion") == "" {
			return nil, fmt.Errorf("authorization omitted the seller assertion")
		}
		result = map[string]any{"purchase_units": []any{map[string]any{"payments": map[string]any{"authorizations": []any{map[string]string{"id": "AUTH-1", "status": "CREATED"}}}}}}
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
		result = map[string]any{"id": "AUTH-1", "status": "CAPTURED", "supplementary_data": map[string]any{"related_ids": map[string]string{"capture_id": "CAPTURE-1"}}}
	case strings.HasSuffix(r.URL.Path, "/void"):
		result = map[string]any{}
	default:
		return nil, fmt.Errorf("unexpected payment API call: %s %s", r.Method, r.URL.Path)
	}
	data, _ := json.Marshal(result)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
}

// The complete journey uses the production HTTP handlers, PostgreSQL store,
// opaque session lookup and PayPal adapter. External responses are deterministic
// contract fixtures; these tests do not claim to verify Auth0 or a PayPal wallet.
func TestFullJourney(t *testing.T) {
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
	evidence := &journeyEvidence{files: map[string]storedEvidence{}}
	server := httpapi.New(httpapi.Dependencies{AppName: "SamenKlaar", WebDirectory: envOr("WEB_DIR", "../../dist"), AppBaseURL: base, Store: store, Auth: auth, AI: journeyAI{}, Evidence: evidence, Payments: payments, Marketplace: payments, Directory: store, Professionals: store, Operators: store, Providers: httpapi.ProviderConfig{AIProvider: "contract_test", EvidenceProvider: "r2", PaymentProvider: "paypal_sandbox"}})
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
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
	json.Unmarshal(request("professional", "POST", "/api/professional/availability", workflow.Appointment{StartUTC: earliestStart.Format(time.RFC3339), EndUTC: earliestStart.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}, 200), &published)
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
	urgent = urgentAction("approve-and-book", published.ID)
	if urgent.Job.Appointment == nil || !urgent.Job.QuoteApproved || urgent.Job.Appointment.SlotID != published.ID {
		t.Fatal("combined customer approval did not reserve the actual earliest visit")
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
	t.Logf("Verified onboarding → AI intake → matching without location → price counter-offer → date counter-offer → reservation → declined reschedule → work → scope change → private upload → note → completion → payment approval → authorization → lost capture response → reconciliation → verified customer review → professional response → persisted state (%d audit events)", events)
}
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (journeyAI) SelectServices(_ context.Context, _ string, services []matching.Service) (matching.Selection, error) {
	for i, s := range services {
		if s.Name == "Sink leak repair" {
			return matching.Selection{Fits: []matching.Fit{{Index: i, Reason: "Published sink leak repair covers the requested work."}}}, nil
		}
	}
	return matching.Selection{Question: "Where is the leak coming from?"}, nil
}
