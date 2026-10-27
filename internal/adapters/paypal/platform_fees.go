package paypal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

type sellerReceivableBreakdown struct {
	PlatformFees []struct {
		Amount paymentAmount `json:"amount"`
	} `json:"platform_fees"`
}

type captureDetails struct {
	ID, Status string
	Amount     paymentAmount             `json:"amount"`
	Breakdown  sellerReceivableBreakdown `json:"seller_receivable_breakdown"`
}

func capturedFeeMatches(data []byte, intent *paypalIntent, captureID string) bool {
	var captured captureDetails
	if json.Unmarshal(data, &captured) != nil || captured.ID != captureID || captured.Status != "COMPLETED" || !amountMatches(captured.Amount, intent) || len(captured.Breakdown.PlatformFees) != 1 {
		return false
	}
	fee := captured.Breakdown.PlatformFees[0].Amount
	minor, err := parseMinorUnits(fee.Value)
	return err == nil && minor == intent.PlatformFeeMinor && fee.Currency == intent.Currency
}

func (p *Service) verifyCaptureFee(ctx context.Context, intent *paypalIntent, captureID string, data []byte) error {
	if intent.PlatformFeeMinor == 0 {
		return nil
	}
	if capturedFeeMatches(data, intent, captureID) {
		return nil
	}
	// Recover the original capture; never create another transfer or assume
	// that a missing fee was collected.
	data, err := p.apiMarketplace(ctx, http.MethodGet, "/v2/payments/captures/"+url.PathEscape(captureID), nil, "", intent.SellerMerchantID)
	if err != nil || !capturedFeeMatches(data, intent, captureID) {
		return errors.New("PayPal capture needs reconciliation of the agreed platform fee")
	}
	return nil
}
