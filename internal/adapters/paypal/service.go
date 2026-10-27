package paypal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type paypalIntent struct {
	WorkspaceID            string
	JobID                  string
	ScopeVersion           int
	AmountMinor            int64
	Currency               string
	SellerMerchantID       string
	OrderID                string
	AuthorizationID        string
	AuthorizationExpiresAt string
	CaptureID              string
	ApprovalURL            string
	Status                 string
	OrderRequestID         string
	AuthRequestID          string
	CaptureRequestID       string
	VoidRequestID          string
	RefundRequestID        string
	RefundID               string
}

type IntentRecord struct {
	WorkspaceID            string
	JobID                  string
	ScopeVersion           int
	AmountMinor            int64
	Currency               string
	SellerMerchantID       string
	OrderID                string
	AuthorizationID        string
	AuthorizationExpiresAt string
	CaptureID              string
	ApprovalURL            string
	Status                 string
	OrderRequestID         string
	AuthRequestID          string
	CaptureRequestID       string
	VoidRequestID          string
	RefundRequestID        string
	RefundID               string
}

type WebhookEvent struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	ResourceID  string `json:"resourceId,omitempty"`
	MerchantID  string `json:"merchantId,omitempty"`
	TrackingID  string `json:"trackingId,omitempty"`
	CaptureID   string `json:"captureId,omitempty"`
	OrderID     string `json:"orderId,omitempty"`
	Status      string `json:"status,omitempty"`
	Currency    string `json:"currency,omitempty"`
	AmountMinor int64  `json:"amountMinor,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type WebhookSink interface {
	EnqueuePayPalWebhook(context.Context, WebhookEvent) error
}

type IntentStore interface {
	GetPaymentIntent(context.Context, string, int) (IntentRecord, bool, error)
	CreatePaymentIntent(context.Context, IntentRecord) (bool, error)
	UpdatePaymentIntent(context.Context, IntentRecord, ...string) error
	ClaimPaymentIntent(context.Context, string, string, int, string, string) (bool, error)
	RetryPaymentIntent(context.Context, IntentRecord) (bool, error)
}

type Service struct {
	appName              string
	baseURL              string
	clientID             string
	clientSecret         string
	returnURL            string
	platformMerchantID   string
	partnerAttributionID string
	client               *http.Client
	tokenMu              sync.Mutex
	token                string
	tokenExpiry          time.Time
	mu                   sync.Mutex
	intents              map[string]*paypalIntent
	intentStore          IntentStore
	webhookID            string
	webhookSink          WebhookSink
	marketplace          MarketplaceStore
}

func (p *Service) SetIntentStore(store IntentStore) { p.intentStore = store }
func (p *Service) SetWebhookSink(sink WebhookSink)  { p.webhookSink = sink }

func New(appName string, clients ...*http.Client) (*Service, error) {
	if strings.TrimSpace(os.Getenv("PAYPAL_ENV")) != "sandbox" {
		return nil, errors.New("PayPal sandbox requires PAYPAL_ENV=sandbox")
	}
	clientID := strings.TrimSpace(os.Getenv("PAYPAL_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("PAYPAL_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET are required for PayPal sandbox")
	}
	base := "https://api-m.sandbox.paypal.com"
	returnURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	if returnURL == "" {
		returnURL = "http://localhost:5173"
	}
	if strings.TrimSpace(appName) == "" {
		appName = "Service"
	}
	client := &http.Client{Timeout: 25 * time.Second}
	if len(clients) > 0 && clients[0] != nil {
		client = clients[0]
	}
	return &Service{
		appName: appName, baseURL: base, clientID: clientID, clientSecret: clientSecret, returnURL: returnURL,
		platformMerchantID:   strings.TrimSpace(os.Getenv("PAYPAL_PLATFORM_MERCHANT_ID")),
		partnerAttributionID: strings.TrimSpace(os.Getenv("PAYPAL_PARTNER_ATTRIBUTION_ID")),
		client:               client, intents: map[string]*paypalIntent{}, webhookID: strings.TrimSpace(os.Getenv("PAYPAL_WEBHOOK_ID")),
	}, nil
}

func scopePaymentKey(state workflow.State) string {
	return fmt.Sprintf("%s:%s:%d", state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion)
}

func stableRequestID(operation string) string {
	sum := sha256.Sum256([]byte(operation))
	return hex.EncodeToString(sum[:16])
}

func (p *Service) accessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.token != "" && time.Now().Add(30*time.Second).Before(p.tokenExpiry) {
		return p.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/oauth2/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(p.clientID, p.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", errors.New("PayPal sandbox authentication request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", errors.New("could not read PayPal sandbox authentication response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", newProviderError(resp.StatusCode, "authentication", data, resp.Header)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if json.Unmarshal(data, &result) != nil || result.AccessToken == "" {
		return "", fmt.Errorf("PayPal sandbox authentication returned HTTP %d", resp.StatusCode)
	}
	p.token = result.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return p.token, nil
}

func (p *Service) api(ctx context.Context, method, path string, payload any, requestID string) ([]byte, error) {
	return p.apiWithHeaders(ctx, method, path, payload, requestID, nil)
}

func (p *Service) apiMarketplace(ctx context.Context, method, path string, payload any, requestID, sellerMerchantID string) ([]byte, error) {
	if sellerMerchantID == "" || !p.marketplaceReady() {
		return nil, errors.New("PayPal marketplace payment context is unavailable")
	}
	headers := p.marketplaceHeaders(sellerMerchantID)
	if method == http.MethodPost && strings.HasSuffix(path, "/authorize") {
		headers["Prefer"] = "return=representation"
	}
	return p.apiWithHeaders(ctx, method, path, payload, requestID, headers)
}

func (p *Service) apiWithHeaders(ctx context.Context, method, path string, payload any, requestID string, extraHeaders map[string]string) ([]byte, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, errors.New("could not prepare PayPal request")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return nil, errors.New("could not prepare PayPal request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if requestID != "" {
		req.Header.Set("PayPal-Request-Id", requestID)
	}
	for name, value := range extraHeaders {
		if value != "" {
			req.Header.Set(name, value)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.New("PayPal sandbox request did not return a response")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errors.New("could not read PayPal sandbox response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newProviderError(resp.StatusCode, "API", data, resp.Header)
	}
	return data, nil
}

func moneyDecimal(minor int64) string { return fmt.Sprintf("%d.%02d", minor/100, minor%100) }

type paymentAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency_code"`
}

type orderDetails struct {
	ID            string                       `json:"id"`
	Intent        string                       `json:"intent"`
	Status        string                       `json:"status"`
	Links         []struct{ Href, Rel string } `json:"links"`
	PurchaseUnits []struct {
		ReferenceID string        `json:"reference_id"`
		Amount      paymentAmount `json:"amount"`
		Payee       struct {
			MerchantID string `json:"merchant_id"`
		} `json:"payee"`
		Payments struct {
			Authorizations []struct {
				ID, Status string
				Amount     paymentAmount `json:"amount"`
				ExpiresAt  string        `json:"expiration_time"`
			} `json:"authorizations"`
		} `json:"payments"`
	} `json:"purchase_units"`
}

func amountMatches(amount paymentAmount, intent *paypalIntent) bool {
	minor, err := parseMinorUnits(amount.Value)
	return err == nil && minor == intent.AmountMinor && amount.Currency == intent.Currency
}

func validateOrderDetails(order orderDetails, intent *paypalIntent) error {
	if order.ID == "" || order.ID != intent.OrderID || order.Intent != "" && order.Intent != "AUTHORIZE" || len(order.PurchaseUnits) != 1 {
		return errors.New("PayPal order does not match the recorded authorization attempt")
	}
	unit := order.PurchaseUnits[0]
	if unit.ReferenceID != fmt.Sprintf("%s-v%d", intent.JobID, intent.ScopeVersion) ||
		!amountMatches(unit.Amount, intent) || unit.Payee.MerchantID == "" || unit.Payee.MerchantID != intent.SellerMerchantID {
		return errors.New("PayPal order does not match the agreed scope, amount, currency, or seller")
	}
	for _, authorization := range unit.Payments.Authorizations {
		if authorization.ID == "" || !amountMatches(authorization.Amount, intent) || intent.AuthorizationID != "" && intent.AuthorizationID != authorization.ID {
			return errors.New("PayPal authorization does not match the recorded amount or transaction")
		}
	}
	if len(unit.Payments.Authorizations) > 1 {
		return errors.New("PayPal returned multiple authorizations for this payment attempt")
	}
	return nil
}

func intentMatchesState(intent *paypalIntent, state workflow.State) bool {
	return intent.WorkspaceID == state.WorkspaceID && intent.JobID == state.Job.ID && intent.ScopeVersion == state.Job.ScopeVersion &&
		intent.AmountMinor == state.Job.Quote.TotalMinor && intent.Currency == state.Job.Quote.Currency
}

// Authorize responses can use a minimal representation. Retrieve the complete
// order when required fields are omitted, then verify its full payment binding.
func (p *Service) authorizedOrderDetails(ctx context.Context, data []byte, intent *paypalIntent) (orderDetails, error) {
	var order orderDetails
	if json.Unmarshal(data, &order) != nil || order.ID != intent.OrderID {
		return order, errors.New("PayPal authorization response did not identify the original order")
	}
	if validateOrderDetails(order, intent) == nil {
		return order, nil
	}
	data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(intent.OrderID), nil, "", intent.SellerMerchantID)
	if err != nil {
		return order, err
	}
	order = orderDetails{}
	if json.Unmarshal(data, &order) != nil {
		return order, errors.New("PayPal authorization order response was invalid")
	}
	return order, validateOrderDetails(order, intent)
}

// VerifiedAuthorization returns the durable provider identity that may be
// projected into the current booking. The workspace transaction must check this
// exact attempt again when recording it, since the agreement may change meanwhile.
func (p *Service) VerifiedAuthorization(ctx context.Context, state workflow.State) (IntentRecord, error) {
	intent, err := p.loadIntent(ctx, state)
	if err != nil || intent == nil || !intentMatchesState(intent, state) || intent.SellerMerchantID == "" || intent.Status != "authorized" || intent.AuthorizationID == "" || intent.AuthRequestID == "" {
		return IntentRecord{}, errors.New("a verified PayPal authorization for the current scope is required")
	}
	seller, err := p.sellerForPayment(ctx, state.Job.ProfessionalAccountID)
	if err != nil || seller.MerchantID != intent.SellerMerchantID {
		return IntentRecord{}, errors.New("the selected professional's PayPal payout account is no longer eligible")
	}
	if err := p.refreshAuthorization(ctx, intent); err != nil {
		return IntentRecord{}, err
	}
	if intent.Status != "authorized" {
		return IntentRecord{}, errors.New("PayPal authorization is no longer usable; approve a fresh payment before continuing")
	}
	return recordFromIntent(intent), nil
}

func intentFromRecord(record IntentRecord) *paypalIntent {
	return &paypalIntent{
		WorkspaceID: record.WorkspaceID, JobID: record.JobID, ScopeVersion: record.ScopeVersion,
		AmountMinor: record.AmountMinor, Currency: record.Currency, SellerMerchantID: record.SellerMerchantID, OrderID: record.OrderID,
		AuthorizationID: record.AuthorizationID, AuthorizationExpiresAt: record.AuthorizationExpiresAt, CaptureID: record.CaptureID, ApprovalURL: record.ApprovalURL,
		Status: record.Status, OrderRequestID: record.OrderRequestID, AuthRequestID: record.AuthRequestID,
		CaptureRequestID: record.CaptureRequestID, VoidRequestID: record.VoidRequestID,
		RefundRequestID: record.RefundRequestID, RefundID: record.RefundID,
	}
}

func recordFromIntent(intent *paypalIntent) IntentRecord {
	return IntentRecord{
		WorkspaceID: intent.WorkspaceID, JobID: intent.JobID, ScopeVersion: intent.ScopeVersion,
		AmountMinor: intent.AmountMinor, Currency: intent.Currency, SellerMerchantID: intent.SellerMerchantID, OrderID: intent.OrderID,
		AuthorizationID: intent.AuthorizationID, AuthorizationExpiresAt: intent.AuthorizationExpiresAt, CaptureID: intent.CaptureID, ApprovalURL: intent.ApprovalURL,
		Status: intent.Status, OrderRequestID: intent.OrderRequestID, AuthRequestID: intent.AuthRequestID,
		CaptureRequestID: intent.CaptureRequestID, VoidRequestID: intent.VoidRequestID,
		RefundRequestID: intent.RefundRequestID, RefundID: intent.RefundID,
	}
}

func (p *Service) loadIntent(ctx context.Context, state workflow.State) (*paypalIntent, error) {
	job := state.Job
	key := scopePaymentKey(state)
	if p.intentStore != nil {
		record, found, err := p.intentStore.GetPaymentIntent(ctx, job.ID, job.ScopeVersion)
		if err != nil || !found {
			return nil, err
		}
		if record.WorkspaceID != state.WorkspaceID {
			return nil, errors.New("PayPal operation belongs to another request workspace")
		}
		return intentFromRecord(record), nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if intent := p.intents[key]; intent != nil {
		copy := *intent
		return &copy, nil
	}
	return nil, nil
}

func (p *Service) createIntent(ctx context.Context, intent *paypalIntent) (bool, error) {
	key := fmt.Sprintf("%s:%s:%d", intent.WorkspaceID, intent.JobID, intent.ScopeVersion)
	if p.intentStore != nil {
		created, err := p.intentStore.CreatePaymentIntent(ctx, recordFromIntent(intent))
		if err != nil || !created {
			return created, err
		}
		return true, nil
	}
	p.mu.Lock()
	copy := *intent
	p.intents[key] = &copy
	p.mu.Unlock()
	return true, nil
}

func (p *Service) updateIntent(ctx context.Context, intent *paypalIntent, expected ...string) error {
	if p.intentStore != nil {
		if err := p.intentStore.UpdatePaymentIntent(ctx, recordFromIntent(intent), expected...); err != nil {
			return err
		}
		return nil
	}
	key := fmt.Sprintf("%s:%s:%d", intent.WorkspaceID, intent.JobID, intent.ScopeVersion)
	p.mu.Lock()
	if p.intentStore == nil && len(expected) > 0 {
		current := p.intents[key]
		matched := false
		if current != nil && samePaymentAttempt(current, recordFromIntent(intent)) {
			for _, status := range expected {
				if current.Status == status {
					matched = true
					break
				}
			}
		}
		if !matched {
			p.mu.Unlock()
			return errors.New("PayPal operation state changed before it could be saved")
		}
	}
	copy := *intent
	p.intents[key] = &copy
	p.mu.Unlock()
	return nil
}

func (p *Service) claimIntent(ctx context.Context, state workflow.State, expected *paypalIntent, from, to string) (bool, error) {
	key := scopePaymentKey(state)
	if p.intentStore != nil {
		store, ok := p.intentStore.(interface {
			ClaimPaymentIntentAttempt(context.Context, IntentRecord, string, string) (bool, error)
		})
		if !ok {
			return false, errors.New("PayPal store cannot lock the exact payment attempt")
		}
		return store.ClaimPaymentIntentAttempt(ctx, recordFromIntent(expected), from, to)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	intent := p.intents[key]
	if intent == nil || intent.Status != from || !samePaymentAttempt(intent, recordFromIntent(expected)) {
		return false, nil
	}
	intent.Status = to
	return true, nil
}

func (p *Service) retryIntent(ctx context.Context, intent *paypalIntent) (bool, error) {
	if p.intentStore != nil {
		updated, err := p.intentStore.RetryPaymentIntent(ctx, recordFromIntent(intent))
		if err != nil || !updated {
			return updated, err
		}
		return true, nil
	}
	key := fmt.Sprintf("%s:%s:%d", intent.WorkspaceID, intent.JobID, intent.ScopeVersion)
	p.mu.Lock()
	copy := *intent
	p.intents[key] = &copy
	p.mu.Unlock()
	return true, nil
}

func (p *Service) orderPayload(intent *paypalIntent) map[string]any {
	returnQuery := url.Values{"paypal_return": []string{"1"}, "workspaceId": []string{intent.WorkspaceID}}
	cancelQuery := url.Values{"paypal_cancelled": []string{"1"}, "workspaceId": []string{intent.WorkspaceID}}
	returnURL := p.returnURL + "/?" + returnQuery.Encode()
	cancelURL := p.returnURL + "/?" + cancelQuery.Encode()
	return map[string]any{
		"intent": "AUTHORIZE",
		"purchase_units": []any{map[string]any{
			"reference_id": fmt.Sprintf("%s-v%d", intent.JobID, intent.ScopeVersion),
			"description":  "Approved service scope",
			"amount":       map[string]string{"currency_code": intent.Currency, "value": moneyDecimal(intent.AmountMinor)},
			"payee":        map[string]string{"merchant_id": intent.SellerMerchantID},
		}},
		// The service price is final before checkout and the visit address is
		// managed privately in the job. PAY_NOW approves the exact order; the
		// AUTHORIZE intent still holds funds until a separate approved capture.
		"payment_source": map[string]any{"paypal": map[string]any{
			"experience_context": map[string]string{
				"brand_name": p.appName, "user_action": "PAY_NOW", "shipping_preference": "NO_SHIPPING",
				"return_url": returnURL, "cancel_url": cancelURL,
			},
		}},
	}
}

func (p *Service) CreateOrder(ctx context.Context, state workflow.State) (string, string, error) {
	job := state.Job
	if err := workflow.ValidatePayPalAuthorization(state); err != nil {
		return "", "", err
	}
	key := scopePaymentKey(state)
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return "", "", errors.New("PayPal operation record could not be loaded")
	}
	if intent != nil && !intentMatchesState(intent, state) {
		return "", "", errors.New("PayPal operation does not match the current agreed scope")
	}
	if intent != nil && intent.SellerMerchantID == "" && intent.Status != "voided" && intent.Status != "denied" && intent.Status != "expired" && intent.Status != "deferred" {
		return "", "", errors.New("this older PayPal order is not linked to a professional payout account; reconcile it before retrying")
	}
	seller, err := p.sellerForPayment(ctx, job.ProfessionalAccountID)
	if err != nil {
		return "", "", err
	}
	sellerMerchantID := seller.MerchantID
	if intent != nil && intent.SellerMerchantID != "" && intent.SellerMerchantID != sellerMerchantID {
		return "", "", errors.New("this PayPal payment is bound to a different professional payout account")
	}
	if intent != nil && intent.Status == "created" && intent.ApprovalURL != "" {
		return intent.OrderID, intent.ApprovalURL, nil
	}
	expectedOrderStatus := "creating"
	if intent != nil {
		if intent.Status == "voided" || intent.Status == "denied" || intent.Status == "expired" || intent.Status == "deferred" {
			attempt := time.Now().UTC().Format(time.RFC3339Nano)
			intent.OrderID, intent.AuthorizationID, intent.CaptureID, intent.ApprovalURL, intent.AuthorizationExpiresAt = "", "", "", "", ""
			intent.Status, intent.SellerMerchantID = "creating", sellerMerchantID
			intent.OrderRequestID = stableRequestID("order:" + key + ":" + attempt)
			intent.AuthRequestID = stableRequestID("authorize:" + key + ":" + attempt)
			intent.CaptureRequestID = stableRequestID("capture:" + key + ":" + attempt)
			intent.VoidRequestID = stableRequestID("void:" + key + ":" + attempt)
			updated, err := p.retryIntent(ctx, intent)
			if err != nil {
				return "", "", errors.New("PayPal retry operation could not be recorded")
			}
			if !updated {
				return "", "", errors.New("PayPal operation changed in another request; reload its status")
			}
		} else if intent.Status == "creating" || intent.Status == "order_uncertain" && intent.OrderID == "" {
			expectedOrderStatus = intent.Status
			// Retry order creation with the stored request ID so PayPal can return
			// the same result after an interrupted response.
			intent.Status = "creating"
		} else {
			return "", "", fmt.Errorf("PayPal order is %s; reconcile its status before trying again", intent.Status)
		}
	} else {
		attempt := time.Now().UTC().Format(time.RFC3339Nano)
		intent = &paypalIntent{
			WorkspaceID: state.WorkspaceID, JobID: job.ID, ScopeVersion: job.ScopeVersion,
			AmountMinor: job.Quote.TotalMinor, Currency: job.Quote.Currency, SellerMerchantID: sellerMerchantID, Status: "creating",
			OrderRequestID: stableRequestID("order:" + key + ":" + attempt), AuthRequestID: stableRequestID("authorize:" + key + ":" + attempt),
			CaptureRequestID: stableRequestID("capture:" + key + ":" + attempt), VoidRequestID: stableRequestID("void:" + key + ":" + attempt),
		}
		created, err := p.createIntent(ctx, intent)
		if err != nil {
			return "", "", errors.New("PayPal operation could not be recorded")
		}
		if !created {
			intent, err = p.loadIntent(ctx, state)
			if err != nil || intent == nil {
				return "", "", errors.New("PayPal operation changed in another request; reload its status")
			}
			if intent.Status == "created" && intent.ApprovalURL != "" {
				return intent.OrderID, intent.ApprovalURL, nil
			}
			if intent.Status != "creating" && intent.Status != "order_uncertain" {
				return "", "", fmt.Errorf("PayPal order is %s; reconcile its status before trying again", intent.Status)
			}
			expectedOrderStatus = intent.Status
		}
	}

	data, err := p.apiMarketplace(ctx, http.MethodPost, "/v2/checkout/orders", p.orderPayload(intent), intent.OrderRequestID, intent.SellerMerchantID)
	if err != nil {
		intent.Status = "order_uncertain"
		_ = p.updateIntent(ctx, intent, expectedOrderStatus)
		return "", "", err
	}
	var result struct {
		ID    string `json:"id"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if json.Unmarshal(data, &result) != nil || result.ID == "" {
		intent.Status = "order_uncertain"
		_ = p.updateIntent(ctx, intent, expectedOrderStatus)
		return "", "", errors.New("PayPal created an order but returned an unreadable response")
	}
	approval := ""
	for _, link := range result.Links {
		if link.Rel == "approve" || link.Rel == "payer-action" {
			approval = link.Href
			break
		}
	}
	if approval == "" {
		intent.Status = "order_uncertain"
		intent.OrderID = result.ID
		_ = p.updateIntent(ctx, intent, expectedOrderStatus)
		return "", "", errors.New("PayPal did not return an approval link")
	}
	intent.OrderID, intent.ApprovalURL, intent.Status = result.ID, approval, "created"
	if err := p.updateIntent(ctx, intent, expectedOrderStatus); err != nil {
		return "", "", errors.New("PayPal order exists but its operation record could not be updated")
	}
	return result.ID, approval, nil
}

func (p *Service) Authorize(ctx context.Context, state workflow.State, orderID string) error {
	if err := workflow.ValidatePayPalAuthorization(state); err != nil {
		return err
	}
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return errors.New("PayPal operation record could not be loaded")
	}
	if intent == nil || intent.OrderID != orderID || intent.AmountMinor != state.Job.Quote.TotalMinor || intent.Currency != state.Job.Quote.Currency {
		return errors.New("this PayPal order does not match the current approved scope")
	}
	seller, err := p.sellerForPayment(ctx, state.Job.ProfessionalAccountID)
	if err != nil || seller.MerchantID != intent.SellerMerchantID {
		return errors.New("the selected professional's PayPal payout account is no longer eligible")
	}
	if intent.Status == "authorized" {
		if err := p.refreshAuthorization(ctx, intent); err != nil {
			return err
		}
		if intent.Status != "authorized" {
			return errors.New("this PayPal authorization is no longer usable; approve a fresh order")
		}
		return nil
	}
	if intent.Status != "created" {
		return fmt.Errorf("PayPal order status is %s; reconcile it before continuing", intent.Status)
	}
	claimed, err := p.claimIntent(ctx, state, intent, "created", "authorizing")
	if err != nil {
		return errors.New("PayPal authorization state could not be reserved")
	}
	if !claimed {
		latest, loadErr := p.loadIntent(ctx, state)
		if loadErr == nil && latest != nil && latest.Status == "authorized" {
			return nil
		}
		return errors.New("PayPal order is being processed; check its status before retrying")
	}
	intent.Status = "authorizing"

	data, err := p.apiMarketplace(ctx, http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(orderID)+"/authorize", map[string]any{}, intent.AuthRequestID, intent.SellerMerchantID)
	if err != nil {
		intent.Status = "authorization_uncertain"
		_ = p.updateIntent(ctx, intent, "authorizing")
		return err
	}
	result, err := p.authorizedOrderDetails(ctx, data, intent)
	if err != nil || len(result.PurchaseUnits[0].Payments.Authorizations) != 1 {
		intent.Status = "authorization_uncertain"
		_ = p.updateIntent(ctx, intent, "authorizing")
		return errors.New("PayPal order was approved but authorization details were not returned")
	}
	authorization := result.PurchaseUnits[0].Payments.Authorizations[0]
	if authorization.ID == "" || authorization.Status != "CREATED" {
		intent.Status = "authorization_uncertain"
		_ = p.updateIntent(ctx, intent, "authorizing")
		return errors.New("PayPal authorization is not in the expected state")
	}
	intent.AuthorizationID, intent.AuthorizationExpiresAt = authorization.ID, authorization.ExpiresAt
	intent.Status, err = authorizationStatus(authorization.Status, authorization.ExpiresAt)
	if err != nil {
		return err
	}
	if err := p.updateIntent(ctx, intent, "authorizing"); err != nil {
		return err
	}
	if intent.Status != "authorized" {
		return errors.New("PayPal authorization expired; approve a fresh order")
	}
	return nil
}

func (p *Service) Capture(ctx context.Context, state workflow.State) error {
	if err := workflow.ValidatePaymentApproval(state.Job); err != nil {
		return err
	}
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return errors.New("PayPal operation record could not be loaded")
	}
	if intent != nil && intent.Status == "captured" && intent.CaptureID != "" {
		return nil
	}
	if intent == nil || intent.Status != "authorized" || intent.AuthorizationID == "" || intent.AmountMinor != state.Job.Quote.TotalMinor || intent.Currency != state.Job.Quote.Currency {
		return errors.New("approve this amount through PayPal sandbox before capture")
	}
	seller, err := p.sellerForPayment(ctx, state.Job.ProfessionalAccountID)
	if err != nil || seller.MerchantID != intent.SellerMerchantID {
		return errors.New("the selected professional's PayPal payout account is no longer eligible")
	}
	if err := p.refreshAuthorization(ctx, intent); err != nil {
		return err
	}
	if intent.Status == "captured" && intent.CaptureID != "" {
		return nil
	}
	if intent.Status != "authorized" {
		return errors.New("PayPal authorization is no longer usable; approve a fresh order before capture")
	}
	claimed, err := p.claimIntent(ctx, state, intent, "authorized", "capturing")
	if err != nil {
		return errors.New("PayPal capture state could not be reserved")
	}
	if !claimed {
		latest, loadErr := p.loadIntent(ctx, state)
		if loadErr == nil && latest != nil && latest.Status == "captured" && latest.CaptureID != "" {
			return nil
		}
		return errors.New("PayPal capture is being processed; reconcile its status before retrying")
	}
	intent.Status = "capturing"

	capturePayload := map[string]any{
		"amount":              map[string]string{"currency_code": state.Job.Quote.Currency, "value": moneyDecimal(state.Job.Quote.TotalMinor)},
		"final_capture":       true,
		"payment_instruction": map[string]string{"disbursement_mode": "INSTANT"},
	}
	data, err := p.apiMarketplace(ctx, http.MethodPost, "/v2/payments/authorizations/"+url.PathEscape(intent.AuthorizationID)+"/capture", capturePayload, intent.CaptureRequestID, intent.SellerMerchantID)
	if err != nil {
		intent.Status = "capture_uncertain"
		_ = p.updateIntent(ctx, intent, "capturing")
		return err
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &result) != nil || result.Status != "COMPLETED" || result.ID == "" {
		intent.Status = "capture_uncertain"
		_ = p.updateIntent(ctx, intent, "capturing")
		return errors.New("PayPal capture response was not complete; reconcile before retrying")
	}
	intent.CaptureID, intent.Status = result.ID, "captured"
	return p.updateIntent(ctx, intent, "capturing")
}

func (p *Service) Reconcile(ctx context.Context, state workflow.State) (string, error) {
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return "unavailable", errors.New("PayPal operation record could not be loaded")
	}
	if intent == nil {
		return "not_started", nil
	}
	if !intentMatchesState(intent, state) {
		return intent.Status, errors.New("PayPal operation does not match the current agreed scope")
	}
	if intent.SellerMerchantID == "" {
		return intent.Status, errors.New("PayPal operation is not linked to a professional payout account")
	}
	status, authID, orderID := intent.Status, intent.AuthorizationID, intent.OrderID
	if status == "voiding" || status == "void_uncertain" {
		if err := p.ReleaseAuthorization(ctx, recordFromIntent(intent)); err != nil {
			return status, err
		}
		return p.Status(ctx, state)
	}
	if (status == "refunding" || status == "refund_uncertain" && intent.RefundID == "") &&
		state.Job.Dispute != nil && state.Job.Dispute.Status == "refund_pending" {
		if _, err := p.Refund(ctx, state); err != nil {
			return status, err
		}
		return "refunded", nil
	}
	if status == "refund_uncertain" && intent.RefundID != "" {
		claimed, err := p.claimIntent(ctx, state, intent, status, "refunding")
		if err != nil {
			return status, err
		}
		if !claimed {
			return status, errors.New("PayPal refund is already being reconciled")
		}
		intent.Status = "refunding"
		return p.reconcileRefund(ctx, intent)
	}
	if (status == "order_uncertain" || status == "creating") && orderID == "" {
		_, _, err := p.CreateOrder(ctx, state)
		if err != nil {
			latest, loadErr := p.loadIntent(ctx, state)
			if loadErr == nil && latest != nil {
				return latest.Status, err
			}
			return status, err
		}
		return "created", nil
	}
	if (status == "authorized" || status == "capture_uncertain" || status == "capturing" || status == "authorization_pending" || status == "partial_capture_review" || status == "authorization_review") && authID != "" {
		if err := p.refreshAuthorization(ctx, intent); err != nil {
			return status, err
		}
		return intent.Status, nil
	}
	if (status == "order_uncertain" || status == "creating" || status == "authorization_uncertain" || status == "authorizing") && orderID != "" {
		data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(orderID), nil, "", intent.SellerMerchantID)
		if err != nil {
			return status, errors.New("PayPal order status could not be reconciled")
		}
		var result orderDetails
		if json.Unmarshal(data, &result) != nil || validateOrderDetails(result, intent) != nil {
			return status, errors.New("PayPal order status response was invalid")
		}
		for _, link := range result.Links {
			if link.Rel == "approve" || link.Rel == "payer-action" {
				intent.ApprovalURL = link.Href
				break
			}
		}
		if len(result.PurchaseUnits) > 0 && len(result.PurchaseUnits[0].Payments.Authorizations) > 0 {
			auth := result.PurchaseUnits[0].Payments.Authorizations[0]
			intent.AuthorizationID, intent.AuthorizationExpiresAt = auth.ID, auth.ExpiresAt
			intent.Status, err = authorizationStatus(auth.Status, auth.ExpiresAt)
			if err != nil {
				return status, err
			}
			if err := p.updateIntent(ctx, intent, status); err != nil {
				return status, errors.New("PayPal authorization status could not be saved")
			}
			return intent.Status, nil
		}
		if result.Status == "APPROVED" || result.Status == "CREATED" {
			if status == "authorizing" || status == "authorization_uncertain" {
				return status, errors.New("PayPal authorization result remains uncertain; retry reconciliation or release the original attempt")
			}
			intent.Status = "created"
			if intent.ApprovalURL == "" {
				return status, errors.New("PayPal order exists but its approval link is not available")
			}
			if err := p.updateIntent(ctx, intent, status); err != nil {
				return status, errors.New("PayPal order status could not be saved")
			}
			return "created", nil
		}
	}
	return status, nil
}

func (p *Service) Status(ctx context.Context, state workflow.State) (string, error) {
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return "unavailable", err
	}
	if intent == nil {
		return "not_started", nil
	}
	return intent.Status, nil
}

func (p *Service) CaptureID(ctx context.Context, state workflow.State) (string, error) {
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return "", err
	}
	if intent == nil {
		return "", nil
	}
	return intent.CaptureID, nil
}

func (p *Service) VoidCurrent(ctx context.Context, state workflow.State) error {
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return errors.New("PayPal operation record could not be loaded")
	}
	if intent == nil {
		return nil
	}
	if intent.Status == "release_pending" {
		return errors.New("PayPal authorization release is queued; retry after it finishes")
	}
	return p.ReleaseAuthorization(ctx, recordFromIntent(intent))
}

func samePaymentAttempt(current *paypalIntent, expected IntentRecord) bool {
	return current.WorkspaceID == expected.WorkspaceID && current.JobID == expected.JobID && current.ScopeVersion == expected.ScopeVersion &&
		current.AmountMinor == expected.AmountMinor && current.Currency == expected.Currency && current.SellerMerchantID == expected.SellerMerchantID &&
		current.OrderRequestID == expected.OrderRequestID && current.AuthRequestID == expected.AuthRequestID && current.VoidRequestID == expected.VoidRequestID
}

// ReleaseAuthorization finishes only the persisted attempt supplied by the
// release queue. A replacement attempt is never contacted or overwritten.
func (p *Service) ReleaseAuthorization(ctx context.Context, expected IntentRecord) error {
	state := workflow.State{WorkspaceID: expected.WorkspaceID, Job: workflow.Job{ID: expected.JobID, ScopeVersion: expected.ScopeVersion}}
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return err
	}
	if intent == nil || !samePaymentAttempt(intent, expected) {
		return nil
	}
	if expected.Status == "release_pending" {
		return errors.New("PayPal authorization release requires its original queued attempt status")
	}
	switch intent.Status {
	case "voided", "denied", "expired", "deferred":
		return nil
	case "created", "creating", "order_uncertain", "authorizing", "authorization_uncertain", "authorized", "voiding", "void_uncertain", "release_pending":
	default:
		return fmt.Errorf("PayPal payment is %s; authorization release requires operator review", intent.Status)
	}
	if intent.WorkspaceID == "" || intent.JobID == "" || intent.ScopeVersion < 1 || intent.AmountMinor <= 0 || intent.Currency == "" || intent.SellerMerchantID == "" ||
		intent.OrderRequestID == "" || intent.AuthRequestID == "" || intent.VoidRequestID == "" {
		return errors.New("PayPal authorization release is missing its recorded attempt identity")
	}
	originalStatus := intent.Status
	if originalStatus == "release_pending" || originalStatus == "voiding" || originalStatus == "void_uncertain" {
		originalStatus = expected.Status
	}
	if (originalStatus == "voiding" || originalStatus == "void_uncertain") && intent.AuthorizationID == "" {
		return errors.New("PayPal release cannot establish whether an authorization was originally issued; retry its durable queued attempt")
	}
	releaseStatus := "voiding"
	if intent.Status == "release_pending" {
		// The queue snapshot is the source of truth for whether an authorize
		// request was issued. Keep its ownership across crashes and retries so
		// synchronous reconciliation cannot infer a different provenance.
		releaseStatus = "release_pending"
	}
	claimed, err := p.claimIntent(ctx, state, intent, intent.Status, releaseStatus)
	if err != nil {
		return err
	}
	if !claimed {
		return errors.New("PayPal authorization release changed; retry its saved attempt")
	}
	intent.Status = releaseStatus
	if err := p.releaseIntent(ctx, intent, originalStatus, releaseStatus); err != nil {
		intent.Status = "void_uncertain"
		if releaseStatus == "release_pending" {
			intent.Status = "release_pending"
		} else if intent.AuthorizationID == "" {
			switch originalStatus {
			case "created", "creating", "order_uncertain":
				intent.Status = originalStatus
			case "authorizing", "authorization_uncertain":
				intent.Status = "authorization_uncertain"
			}
		}
		_ = p.updateIntent(ctx, intent, releaseStatus)
		return err
	}
	return p.updateIntent(ctx, intent, releaseStatus)
}

func (p *Service) releaseIntent(ctx context.Context, intent *paypalIntent, originalStatus, releaseStatus string) error {
	if intent.OrderID == "" {
		if originalStatus != "creating" && originalStatus != "order_uncertain" {
			return errors.New("PayPal release cannot identify the original order")
		}
		// Replaying order creation cannot authorize funds, and retrieves the
		// order when its original response was lost.
		data, err := p.apiMarketplace(ctx, http.MethodPost, "/v2/checkout/orders", p.orderPayload(intent), intent.OrderRequestID, intent.SellerMerchantID)
		if err != nil {
			return err
		}
		var order orderDetails
		if json.Unmarshal(data, &order) != nil || order.ID == "" {
			return errors.New("PayPal release could not recover the original order")
		}
		intent.OrderID = order.ID
		if err := p.updateIntent(ctx, intent, releaseStatus); err != nil {
			return err
		}
	}
	if intent.AuthorizationID == "" {
		data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(intent.OrderID), nil, "", intent.SellerMerchantID)
		if err != nil {
			return err
		}
		var order orderDetails
		if json.Unmarshal(data, &order) != nil || validateOrderDetails(order, intent) != nil {
			return errors.New("PayPal release order details do not match the saved attempt")
		}
		if len(order.PurchaseUnits[0].Payments.Authorizations) == 0 && (originalStatus == "authorizing" || originalStatus == "authorization_uncertain") {
			// Settle the already-issued ambiguous request using its original
			// identity, then release the resulting authorization immediately.
			data, err = p.apiMarketplace(ctx, http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(intent.OrderID)+"/authorize", map[string]any{}, intent.AuthRequestID, intent.SellerMerchantID)
			if err != nil {
				return err
			}
			order, err = p.authorizedOrderDetails(ctx, data, intent)
			if err != nil {
				return errors.New("PayPal release could not recover the original authorization")
			}
		}
		if len(order.PurchaseUnits[0].Payments.Authorizations) == 0 {
			if order.Status != "CREATED" && order.Status != "APPROVED" && order.Status != "PAYER_ACTION_REQUIRED" && order.Status != "VOIDED" {
				return errors.New("PayPal order has no confirmed authorization release outcome")
			}
			if originalStatus == "authorizing" || originalStatus == "authorization_uncertain" {
				return errors.New("PayPal authorization result remains uncertain")
			}
			intent.Status = "deferred"
			return nil
		}
		authorization := order.PurchaseUnits[0].Payments.Authorizations[0]
		intent.AuthorizationID = authorization.ID
		if err := p.updateIntent(ctx, intent, releaseStatus); err != nil {
			return err
		}
	}
	data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/payments/authorizations/"+url.PathEscape(intent.AuthorizationID), nil, "", intent.SellerMerchantID)
	if err != nil {
		return err
	}
	var authorization struct {
		ID, Status string
		Amount     paymentAmount `json:"amount"`
		Payee      struct {
			MerchantID string `json:"merchant_id"`
		} `json:"payee"`
		SupplementaryData struct {
			RelatedIDs struct {
				OrderID string `json:"order_id"`
			} `json:"related_ids"`
		} `json:"supplementary_data"`
	}
	if json.Unmarshal(data, &authorization) != nil || authorization.ID != intent.AuthorizationID || !amountMatches(authorization.Amount, intent) ||
		authorization.Payee.MerchantID != "" && authorization.Payee.MerchantID != intent.SellerMerchantID ||
		authorization.SupplementaryData.RelatedIDs.OrderID != "" && authorization.SupplementaryData.RelatedIDs.OrderID != intent.OrderID {
		return errors.New("PayPal release authorization does not match the saved attempt")
	}
	if authorization.SupplementaryData.RelatedIDs.OrderID == "" {
		// Older authorization representations omit related_ids. Verify the
		// transaction through its complete original order before releasing it.
		data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(intent.OrderID), nil, "", intent.SellerMerchantID)
		if err != nil {
			return err
		}
		var order orderDetails
		if json.Unmarshal(data, &order) != nil || validateOrderDetails(order, intent) != nil || len(order.PurchaseUnits[0].Payments.Authorizations) != 1 {
			return errors.New("PayPal release could not verify the original order relationship")
		}
	}
	switch authorization.Status {
	case "VOIDED", "EXPIRED", "DENIED":
		intent.Status = strings.ToLower(authorization.Status)
		return nil
	case "CREATED":
	default:
		return fmt.Errorf("PayPal authorization is %s; release requires operator review", authorization.Status)
	}
	_, err = p.apiMarketplace(ctx, http.MethodPost, "/v2/payments/authorizations/"+url.PathEscape(intent.AuthorizationID)+"/void", map[string]any{}, intent.VoidRequestID, intent.SellerMerchantID)
	if err != nil {
		return err
	}
	intent.Status = "voided"
	return nil
}

// Refund issues a single full refund for an operator-approved paid case. The
// stable request ID is stored before contacting PayPal so an uncertain result
// can safely be retried with the same idempotency key.
func (p *Service) Refund(ctx context.Context, state workflow.State) (string, error) {
	if !state.Job.Paid || state.Job.ID == "" {
		return "", errors.New("only a captured service payment can be refunded")
	}
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return "", errors.New("PayPal payment record could not be loaded")
	}
	if intent == nil || intent.CaptureID == "" || intent.AmountMinor != state.Job.Quote.TotalMinor || intent.Currency != state.Job.Quote.Currency {
		return "", errors.New("captured PayPal payment does not match the current service scope")
	}
	if intent.Status == "refunded" {
		return intent.RefundID, nil
	}
	if intent.Status != "captured" && intent.Status != "refund_uncertain" && intent.Status != "refunding" {
		return "", fmt.Errorf("PayPal payment status is %s; refund is not available", intent.Status)
	}
	from := intent.Status
	if from == "refunding" && p.intentStore == nil {
		return "", errors.New("PayPal refund is already being processed")
	}
	claimed, err := p.claimIntent(ctx, state, intent, from, "refunding")
	if err != nil {
		return "", errors.New("PayPal refund state could not be reserved")
	}
	if !claimed {
		latest, loadErr := p.loadIntent(ctx, state)
		if loadErr == nil && latest != nil && latest.Status == "refunded" && latest.RefundID != "" {
			return latest.RefundID, nil
		}
		return "", errors.New("PayPal refund is already being processed; retry after checking status")
	}
	intent.Status = "refunding"
	if intent.RefundRequestID == "" {
		intent.RefundRequestID = stableRequestID("refund:" + scopePaymentKey(state))
	}
	if err := p.updateIntent(ctx, intent, "refunding"); err != nil {
		return "", errors.New("PayPal refund request could not be saved")
	}
	if intent.RefundID != "" {
		status, err := p.reconcileRefund(ctx, intent)
		if err != nil {
			return "", err
		}
		if status == "refunded" {
			return intent.RefundID, nil
		}
		return "", errors.New("PayPal refund is still pending; check its status before retrying")
	}
	data, err := p.apiMarketplace(ctx, http.MethodPost, "/v2/payments/captures/"+url.PathEscape(intent.CaptureID)+"/refund", map[string]any{}, intent.RefundRequestID, intent.SellerMerchantID)
	if err != nil {
		intent.Status = "refund_uncertain"
		_ = p.updateIntent(ctx, intent, "refunding")
		return "", err
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &result) != nil || result.ID == "" {
		intent.Status = "refund_uncertain"
		_ = p.updateIntent(ctx, intent, "refunding")
		return "", errors.New("PayPal refund response was not complete; retry the operator action to reconcile it")
	}
	intent.RefundID = result.ID
	if result.Status != "COMPLETED" {
		intent.Status = "refund_uncertain"
		if err := p.updateIntent(ctx, intent, "refunding"); err != nil {
			return "", errors.New("PayPal refund was accepted but its operation record could not be updated")
		}
		return "", errors.New("PayPal refund is pending; check its status before retrying")
	}
	intent.RefundID, intent.Status = result.ID, "refunded"
	if err := p.updateIntent(ctx, intent, "refunding"); err != nil {
		return "", errors.New("PayPal refund completed but its operation record could not be updated")
	}
	return result.ID, nil
}

func (p *Service) reconcileRefund(ctx context.Context, intent *paypalIntent) (string, error) {
	data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/payments/refunds/"+url.PathEscape(intent.RefundID), nil, "", intent.SellerMerchantID)
	if err != nil {
		intent.Status = "refund_uncertain"
		_ = p.updateIntent(ctx, intent, "refunding")
		return intent.Status, errors.New("PayPal refund status could not be reconciled")
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &result) != nil || result.ID != intent.RefundID {
		return intent.Status, errors.New("PayPal refund status response was invalid")
	}
	if result.Status == "COMPLETED" {
		intent.Status = "refunded"
	} else {
		intent.Status = "refund_uncertain"
	}
	if err := p.updateIntent(ctx, intent, "refunding"); err != nil {
		return intent.Status, errors.New("PayPal refund status could not be saved")
	}
	return intent.Status, nil
}
