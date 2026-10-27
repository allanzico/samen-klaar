package paypal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ReceiveWebhook verifies PayPal's transmission before enqueueing a minimal
// normalized event. The HTTP handler can acknowledge delivery immediately;
// event application runs through PostgreSQL and River.
func (p *Service) ReceiveWebhook(ctx context.Context, headers http.Header, rawBody []byte) (bool, error) {
	if p.webhookID == "" || p.webhookSink == nil {
		return false, errors.New("PayPal webhook ID and persistent event queue are required")
	}
	if len(rawBody) == 0 || !json.Valid(rawBody) {
		return false, nil
	}
	verification := map[string]any{
		"auth_algo":         headers.Get("PayPal-Auth-Algo"),
		"cert_url":          headers.Get("PayPal-Cert-Url"),
		"transmission_id":   headers.Get("PayPal-Transmission-Id"),
		"transmission_sig":  headers.Get("PayPal-Transmission-Sig"),
		"transmission_time": headers.Get("PayPal-Transmission-Time"),
		"webhook_id":        p.webhookID,
		"webhook_event":     json.RawMessage(rawBody),
	}
	for _, key := range []string{"auth_algo", "cert_url", "transmission_id", "transmission_sig", "transmission_time"} {
		if strings.TrimSpace(verification[key].(string)) == "" {
			return false, nil
		}
	}
	data, err := p.api(ctx, http.MethodPost, "/v1/notifications/verify-webhook-signature", verification, "")
	if err != nil {
		return false, fmt.Errorf("PayPal webhook verification unavailable: %w", err)
	}
	var result struct {
		Status string `json:"verification_status"`
	}
	if json.Unmarshal(data, &result) != nil || result.Status != "SUCCESS" {
		return false, nil
	}
	event, err := normalizeWebhook(rawBody)
	if err != nil {
		return false, nil
	}
	if err := p.webhookSink.EnqueuePayPalWebhook(ctx, event); err != nil {
		return false, err
	}
	return true, nil
}

func normalizeWebhook(body []byte) (WebhookEvent, error) {
	var wire struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Resource  struct {
			Breakdown  sellerReceivableBreakdown `json:"seller_receivable_breakdown"`
			ID         string                    `json:"id"`
			MerchantID string                    `json:"merchant_id"`
			TrackingID string                    `json:"tracking_id"`
			Status     string                    `json:"status"`
			Reason     string                    `json:"reason"`
			Amount     struct {
				Value        string `json:"value"`
				CurrencyCode string `json:"currency_code"`
			} `json:"amount"`
			SupplementaryData struct {
				RelatedIDs struct {
					OrderID   string `json:"order_id"`
					CaptureID string `json:"capture_id"`
				} `json:"related_ids"`
			} `json:"supplementary_data"`
			DisputedTransactions []struct {
				SellerTransactionID string `json:"seller_transaction_id"`
			} `json:"disputed_transactions"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &wire); err != nil || strings.TrimSpace(wire.ID) == "" || strings.TrimSpace(wire.EventType) == "" {
		return WebhookEvent{}, errors.New("PayPal webhook event is invalid")
	}
	event := WebhookEvent{
		ID: wire.ID, Type: wire.EventType, ResourceID: wire.Resource.ID,
		MerchantID: wire.Resource.MerchantID,
		TrackingID: wire.Resource.TrackingID,
		CaptureID:  wire.Resource.SupplementaryData.RelatedIDs.CaptureID,
		OrderID:    wire.Resource.SupplementaryData.RelatedIDs.OrderID,
		Status:     wire.Resource.Status, Currency: strings.ToUpper(wire.Resource.Amount.CurrencyCode),
		Reason: wire.Resource.Reason,
	}
	if event.MerchantID == "" && strings.HasPrefix(event.Type, "MERCHANT.") {
		event.MerchantID = event.ResourceID
	}
	if event.CaptureID == "" && strings.HasPrefix(event.Type, "PAYMENT.CAPTURE.") {
		event.CaptureID = event.ResourceID
	}
	if event.CaptureID == "" && len(wire.Resource.DisputedTransactions) > 0 {
		event.CaptureID = wire.Resource.DisputedTransactions[0].SellerTransactionID
	}
	if wire.Resource.Amount.Value != "" {
		minor, err := parseMinorUnits(wire.Resource.Amount.Value)
		if err != nil {
			return WebhookEvent{}, err
		}
		event.AmountMinor = minor
	}
	if strings.HasPrefix(event.Type, "PAYMENT.CAPTURE.") && len(wire.Resource.Breakdown.PlatformFees) > 0 {
		if len(wire.Resource.Breakdown.PlatformFees) != 1 {
			return WebhookEvent{}, errors.New("PayPal platform fee is invalid")
		}
		fee := wire.Resource.Breakdown.PlatformFees[0].Amount
		minor, err := parseMinorUnits(fee.Value)
		if err != nil || fee.Currency != event.Currency {
			return WebhookEvent{}, errors.New("PayPal platform fee is invalid")
		}
		event.PlatformFeeMinor, event.PlatformFeeReported = minor, true
	}
	return event, nil
}

func parseMinorUnits(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("PayPal webhook amount is invalid")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("PayPal webhook amount is invalid")
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 || parts[1] == "" {
			return 0, errors.New("PayPal webhook amount is invalid")
		}
		fraction = parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil || whole > (int64(^uint64(0)>>1)-cents)/100 {
		return 0, errors.New("PayPal webhook amount is invalid")
	}
	return whole*100 + cents, nil
}
