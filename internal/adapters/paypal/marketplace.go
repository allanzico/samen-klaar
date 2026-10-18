package paypal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type SellerAccount struct {
	AccountID             string
	TrackingID            string
	RequestID             string
	ReferralSelfURL       string
	MerchantID            string
	Status                string
	PaymentsReceivable    bool
	PrimaryEmailConfirmed bool
	Scopes                []string
}

type MarketplaceStore interface {
	GetPayPalSeller(context.Context, string) (SellerAccount, bool, error)
	BeginPayPalSellerReferral(context.Context, string, string, string) (SellerAccount, error)
	SavePayPalSellerReferral(context.Context, string, string, string) error
	UpdatePayPalSellerStatus(context.Context, string, string, string, bool, bool, []string) error
	RevokePayPalSeller(context.Context, string) error
}

type PayoutSetupStatus = workflow.PayPalPayoutSetupStatus

func (p *Service) SetMarketplaceStore(store MarketplaceStore) { p.marketplace = store }

func (p *Service) marketplaceReady() bool {
	return p.platformMerchantID != "" && p.partnerAttributionID != "" && p.marketplace != nil
}

func randomTrackingID() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", errors.New("could not create PayPal seller reference")
	}
	return "sk_" + hex.EncodeToString(value), nil
}

func (p *Service) BeginSellerOnboarding(ctx context.Context, accountID string) (string, error) {
	if !p.marketplaceReady() {
		return "", errors.New("PayPal marketplace setup requires PAYPAL_PLATFORM_MERCHANT_ID and PAYPAL_PARTNER_ATTRIBUTION_ID")
	}
	if strings.TrimSpace(accountID) == "" {
		return "", errors.New("professional account is required for PayPal onboarding")
	}

	record, found, err := p.marketplace.GetPayPalSeller(ctx, accountID)
	if err != nil {
		return "", errors.New("PayPal seller setup could not be loaded")
	}
	if found && record.Status == "verified" {
		return "", errors.New("PayPal payouts are already connected")
	}
	if !found || record.Status == "revoked" || record.Status == "ineligible" {
		trackingID, err := randomTrackingID()
		if err != nil {
			return "", err
		}
		requestID := stableRequestID("seller-referral:" + accountID + ":" + trackingID)
		record, err = p.marketplace.BeginPayPalSellerReferral(ctx, accountID, trackingID, requestID)
		if err != nil {
			return "", errors.New("PayPal seller setup could not be reserved")
		}
	}
	if record.ReferralSelfURL != "" {
		selfURL, _ := url.Parse(record.ReferralSelfURL)
		data, err := p.apiWithHeaders(ctx, http.MethodGet, selfURL.RequestURI(), nil, "", p.marketplaceHeaders(p.platformMerchantID))
		if err == nil {
			if actionURL := referralActionURL(data); actionURL != "" {
				return actionURL, nil
			}
		}
	}

	returnURL := p.returnURL + "/?paypal_onboarding=1"
	payload := map[string]any{
		"tracking_id": record.TrackingID,
		"operations": []any{map[string]any{
			"operation": "API_INTEGRATION",
			"api_integration_preference": map[string]any{
				"rest_api_integration": map[string]any{
					"integration_method":  "PAYPAL",
					"integration_type":    "THIRD_PARTY",
					"third_party_details": map[string]any{"features": []string{"PAYMENT", "REFUND"}},
				},
			},
		}},
		"products": []string{"EXPRESS_CHECKOUT"},
		"partner_config_override": map[string]string{
			"return_url":             returnURL,
			"return_url_description": "Return to SamenKlaar to verify your professional payout setup.",
		},
		"legal_consents": []any{map[string]any{"type": "SHARE_DATA_CONSENT", "granted": true}},
	}
	data, err := p.apiWithHeaders(ctx, http.MethodPost, "/v2/customer/partner-referrals", payload, record.RequestID, p.marketplaceHeaders(p.platformMerchantID))
	if err != nil {
		return "", err
	}
	var response struct {
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if json.Unmarshal(data, &response) != nil {
		return "", errors.New("PayPal seller onboarding returned an unreadable response")
	}
	var selfURL, actionURL string
	for _, link := range response.Links {
		switch link.Rel {
		case "self":
			selfURL = link.Href
		case "action_url":
			actionURL = link.Href
		}
	}
	if !safePayPalURL(selfURL) || !safePayPalURL(actionURL) {
		return "", errors.New("PayPal did not return a valid seller onboarding link")
	}
	if err := p.marketplace.SavePayPalSellerReferral(ctx, accountID, record.TrackingID, selfURL); err != nil {
		return "", errors.New("PayPal onboarding link could not be saved")
	}
	return actionURL, nil
}

func referralActionURL(data []byte) string {
	var response struct {
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if json.Unmarshal(data, &response) != nil {
		return ""
	}
	for _, link := range response.Links {
		if link.Rel == "action_url" && safePayPalURL(link.Href) {
			return link.Href
		}
	}
	return ""
}

func safePayPalURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	return parsed.Host == "api-m.sandbox.paypal.com" || parsed.Host == "api.sandbox.paypal.com" || parsed.Host == "www.sandbox.paypal.com" || parsed.Host == "www.paypal.com"
}

func (p *Service) SellerOnboardingStatus(ctx context.Context, accountID, merchantIDFromReturn string) (PayoutSetupStatus, error) {
	status := PayoutSetupStatus{Status: "not_started"}
	if !p.marketplaceReady() {
		return status, errors.New("PayPal marketplace setup requires PAYPAL_PLATFORM_MERCHANT_ID and PAYPAL_PARTNER_ATTRIBUTION_ID")
	}
	record, found, err := p.marketplace.GetPayPalSeller(ctx, accountID)
	if err != nil {
		return status, errors.New("PayPal seller setup could not be loaded")
	}
	if !found {
		return status, nil
	}
	status.Status = record.Status
	status.PaymentsReceivable = record.PaymentsReceivable
	status.PrimaryEmailConfirmed = record.PrimaryEmailConfirmed
	if record.Status == "revoked" {
		return status, nil
	}
	merchantID := strings.TrimSpace(merchantIDFromReturn)
	if merchantID == "" {
		merchantID = record.MerchantID
	}
	if merchantID == "" || !safeMerchantID(merchantID) {
		return status, nil
	}
	verified, err := p.fetchSellerIntegration(ctx, merchantID, record.TrackingID)
	if err != nil {
		return status, err
	}
	if verified.MerchantID != merchantID || verified.TrackingID != record.TrackingID {
		return status, nil
	}
	ready := verified.PaymentsReceivable && verified.PrimaryEmailConfirmed && verified.HasPaymentPermission && verified.HasRefundPermission
	newStatus := "pending"
	if ready {
		newStatus = "verified"
	} else if !verified.PaymentsReceivable {
		newStatus = "ineligible"
	}
	if err := p.marketplace.UpdatePayPalSellerStatus(ctx, accountID, merchantID, newStatus, verified.PaymentsReceivable, verified.PrimaryEmailConfirmed, verified.Scopes); err != nil {
		return status, errors.New("verified PayPal seller status could not be saved")
	}
	status = PayoutSetupStatus{Status: newStatus, Connected: ready, PaymentsReceivable: verified.PaymentsReceivable, PrimaryEmailConfirmed: verified.PrimaryEmailConfirmed}
	return status, nil
}

type sellerIntegration struct {
	MerchantID            string
	TrackingID            string
	PaymentsReceivable    bool
	PrimaryEmailConfirmed bool
	HasPaymentPermission  bool
	HasRefundPermission   bool
	Scopes                []string
}

func (p *Service) fetchSellerIntegration(ctx context.Context, merchantID, trackingID string) (sellerIntegration, error) {
	data, err := p.apiWithHeaders(ctx, http.MethodGet,
		"/v1/customer/partners/"+url.PathEscape(p.platformMerchantID)+"/merchant-integrations/"+url.PathEscape(merchantID),
		nil, "", p.marketplaceHeaders(merchantID))
	if err != nil {
		return sellerIntegration{}, err
	}
	var response struct {
		MerchantID            string `json:"merchant_id"`
		TrackingID            string `json:"tracking_id"`
		PaymentsReceivable    bool   `json:"payments_receivable"`
		PrimaryEmailConfirmed bool   `json:"primary_email_confirmed"`
		OAuthIntegrations     []struct {
			IntegrationType   string `json:"integration_type"`
			IntegrationMethod string `json:"integration_method"`
			ThirdParty        []struct {
				PartnerClientID string   `json:"partner_client_id"`
				Scopes          []string `json:"scopes"`
			} `json:"oauth_third_party"`
		} `json:"oauth_integrations"`
	}
	if json.Unmarshal(data, &response) != nil {
		return sellerIntegration{}, errors.New("PayPal seller status returned an unreadable response")
	}
	result := sellerIntegration{MerchantID: response.MerchantID, TrackingID: response.TrackingID,
		PaymentsReceivable: response.PaymentsReceivable, PrimaryEmailConfirmed: response.PrimaryEmailConfirmed}
	for _, integration := range response.OAuthIntegrations {
		if integration.IntegrationType != "OAUTH_THIRD_PARTY" || integration.IntegrationMethod != "PAYPAL" {
			continue
		}
		for _, thirdParty := range integration.ThirdParty {
			if thirdParty.PartnerClientID != p.clientID {
				continue
			}
			result.Scopes = append(result.Scopes, thirdParty.Scopes...)
			for _, scope := range thirdParty.Scopes {
				result.HasPaymentPermission = result.HasPaymentPermission || strings.Contains(scope, "/payments/payment/authcapture")
				result.HasRefundPermission = result.HasRefundPermission || strings.HasSuffix(scope, "/payments/refund")
			}
		}
	}
	return result, nil
}

func safeMerchantID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (p *Service) sellerForPayment(ctx context.Context, accountID string) (SellerAccount, error) {
	if !p.marketplaceReady() {
		return SellerAccount{}, errors.New("PayPal marketplace payouts are not configured")
	}
	record, found, err := p.marketplace.GetPayPalSeller(ctx, accountID)
	if err != nil {
		return SellerAccount{}, errors.New("PayPal seller payout account could not be loaded")
	}
	if !found || record.Status != "verified" || record.MerchantID == "" {
		return SellerAccount{}, errors.New("the selected professional must finish PayPal payout setup before checkout")
	}
	verified, err := p.fetchSellerIntegration(ctx, record.MerchantID, record.TrackingID)
	if err != nil {
		return SellerAccount{}, errors.New("the selected professional's PayPal payout eligibility could not be verified")
	}
	if verified.MerchantID != record.MerchantID || verified.TrackingID != record.TrackingID ||
		!verified.PaymentsReceivable || !verified.PrimaryEmailConfirmed || !verified.HasPaymentPermission || !verified.HasRefundPermission {
		_ = p.marketplace.UpdatePayPalSellerStatus(ctx, accountID, record.MerchantID, "ineligible", verified.PaymentsReceivable, verified.PrimaryEmailConfirmed, verified.Scopes)
		return SellerAccount{}, errors.New("the selected professional's PayPal account is not currently eligible to receive payments")
	}
	return record, nil
}

func (p *Service) marketplaceHeaders(merchantID string) map[string]string {
	headers := map[string]string{"PayPal-Partner-Attribution-Id": p.partnerAttributionID}
	if merchantID != "" {
		headers["PayPal-Auth-Assertion"] = p.authAssertion(merchantID)
	}
	return headers
}

func (p *Service) authAssertion(merchantID string) string {
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	header, _ := json.Marshal(struct {
		Algorithm string `json:"alg"`
	}{Algorithm: "none"})
	claims, _ := json.Marshal(struct {
		Issuer  string `json:"iss"`
		PayerID string `json:"payer_id"`
	}{Issuer: p.clientID, PayerID: merchantID})
	return encode(string(header)) + "." + encode(string(claims)) + "."
}

func (p *Service) RevokeSellerByMerchant(ctx context.Context, merchantID string) error {
	if p.marketplace == nil || !safeMerchantID(merchantID) {
		return nil
	}
	return p.marketplace.RevokePayPalSeller(ctx, merchantID)
}

func (p *Service) MarketplaceConfigured() bool { return p.marketplaceReady() }
