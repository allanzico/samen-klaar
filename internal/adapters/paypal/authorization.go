package paypal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type authorizationDetails struct {
	ID, Status string
	Amount     paymentAmount `json:"amount"`
	ExpiresAt  string        `json:"expiration_time"`
	Payee      struct {
		MerchantID string `json:"merchant_id"`
	} `json:"payee"`
	SupplementaryData struct {
		RelatedIDs struct {
			OrderID   string `json:"order_id"`
			CaptureID string `json:"capture_id"`
		} `json:"related_ids"`
	} `json:"supplementary_data"`
}

func (p *Service) fetchAuthorization(ctx context.Context, intent *paypalIntent) (authorizationDetails, error) {
	var authorization authorizationDetails
	data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/payments/authorizations/"+url.PathEscape(intent.AuthorizationID), nil, "", intent.SellerMerchantID)
	if err != nil {
		return authorization, err
	}
	if json.Unmarshal(data, &authorization) != nil || authorization.ID != intent.AuthorizationID || authorization.Status == "" || !amountMatches(authorization.Amount, intent) ||
		authorization.Payee.MerchantID != "" && authorization.Payee.MerchantID != intent.SellerMerchantID ||
		authorization.SupplementaryData.RelatedIDs.OrderID != "" && authorization.SupplementaryData.RelatedIDs.OrderID != intent.OrderID {
		return authorization, errors.New("PayPal authorization does not match the saved payment attempt")
	}
	if authorization.SupplementaryData.RelatedIDs.OrderID == "" {
		data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(intent.OrderID), nil, "", intent.SellerMerchantID)
		if err != nil {
			return authorization, err
		}
		var order orderDetails
		if json.Unmarshal(data, &order) != nil || validateOrderDetails(order, intent) != nil || len(order.PurchaseUnits[0].Payments.Authorizations) != 1 {
			return authorization, errors.New("PayPal authorization could not be verified against its original order")
		}
	}
	if authorization.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, authorization.ExpiresAt); err != nil {
			return authorization, errors.New("PayPal authorization expiration time was invalid")
		}
	}
	return authorization, nil
}

func authorizationStatus(status, expiresAt string) (string, error) {
	switch status {
	case "CREATED":
		if expiresAt != "" {
			expires, err := time.Parse(time.RFC3339Nano, expiresAt)
			if err != nil {
				return "", errors.New("PayPal authorization expiration time was invalid")
			}
			if !expires.After(time.Now()) {
				return "expired", nil
			}
		}
		return "authorized", nil
	case "EXPIRED", "VOIDED", "DENIED":
		return strings.ToLower(status), nil
	case "CAPTURED":
		return "captured", nil
	case "PENDING":
		return "authorization_pending", nil
	case "PARTIALLY_CAPTURED":
		return "partial_capture_review", nil
	default:
		return "authorization_review", nil
	}
}

func (p *Service) refreshAuthorization(ctx context.Context, intent *paypalIntent) error {
	if intent.AuthorizationID == "" {
		return errors.New("PayPal authorization ID is missing; reconcile the order first")
	}
	authorization, err := p.fetchAuthorization(ctx, intent)
	if err != nil {
		return err
	}
	from := intent.Status
	if authorization.ExpiresAt != "" {
		intent.AuthorizationExpiresAt = authorization.ExpiresAt
	}
	status, err := authorizationStatus(authorization.Status, intent.AuthorizationExpiresAt)
	if err != nil {
		return err
	}
	if status == "captured" {
		if authorization.SupplementaryData.RelatedIDs.CaptureID == "" {
			intent.Status = "capture_uncertain"
			_ = p.updateIntent(ctx, intent, from)
			return errors.New("PayPal reports a capture without its transaction ID; reconcile before continuing")
		}
		intent.CaptureID = authorization.SupplementaryData.RelatedIDs.CaptureID
		if err := p.verifyCaptureFee(ctx, intent, intent.CaptureID, nil); err != nil {
			intent.Status = "capture_uncertain"
			_ = p.updateIntent(ctx, intent, from)
			return err
		}
	}
	intent.Status = status
	return p.updateIntent(ctx, intent, from)
}

// AuthorizationDetails returns saved provider facts after Reconcile or Capture
// has refreshed them. It does not contact PayPal or invent an expiration date.
func (p *Service) AuthorizationDetails(ctx context.Context, state workflow.State) (IntentRecord, error) {
	intent, err := p.loadIntent(ctx, state)
	if err != nil {
		return IntentRecord{}, err
	}
	if intent == nil {
		return IntentRecord{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion, Status: "not_started"}, nil
	}
	if !intentMatchesState(intent, state) {
		return IntentRecord{}, errors.New("PayPal operation does not match the current agreed scope")
	}
	return recordFromIntent(intent), nil
}
