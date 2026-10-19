package paypal

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// providerError keeps diagnostic identifiers separate from the error text used
// by callers. Never retain the provider body, message, request URL or credentials.
type providerError struct {
	status  int
	kind    string
	code    string
	issue   string
	debugID string
}

func (e *providerError) Error() string {
	return fmt.Sprintf("PayPal sandbox %s returned HTTP %d", e.kind, e.status)
}

func (e *providerError) PayPalDiagnostic() (int, string, string, string) {
	return e.status, e.code, e.issue, e.debugID
}

func (e *providerError) PayPalConnectionStage() string { return "provider_" + e.kind }

func newProviderError(status int, kind string, data []byte, header http.Header) error {
	var body struct {
		Name    string `json:"name"`
		Error   string `json:"error"`
		DebugID string `json:"debug_id"`
		Details []struct {
			Issue string `json:"issue"`
		} `json:"details"`
	}
	_ = json.Unmarshal(data, &body)
	code := body.Name
	if code == "" {
		code = body.Error
	}
	debugID := body.DebugID
	if debugID == "" {
		debugID = header.Get("PayPal-Debug-Id")
	}
	issue := ""
	if len(body.Details) > 0 {
		issue = body.Details[0].Issue
	}
	return &providerError{status: status, kind: kind, code: diagnosticIdentifier(code, false), issue: diagnosticIdentifier(issue, false), debugID: diagnosticIdentifier(debugID, true)}
}

func diagnosticIdentifier(value string, hexOnly bool) string {
	if len(value) > 96 {
		return ""
	}
	for _, r := range value {
		valid := r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
		if !hexOnly {
			valid = valid || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		}
		if !valid {
			return ""
		}
	}
	return value
}

type connectionError struct{ code, message, stage string }

func (e *connectionError) Error() string                 { return e.message }
func (e *connectionError) PayPalConnectionCode() string  { return e.code }
func (e *connectionError) PayPalConnectionStage() string { return e.stage }

func sellerConnectionError(stage, message string) error {
	return &connectionError{code: "", message: message, stage: stage}
}

var (
	errPayoutsUnavailable = &connectionError{"PAYPAL_PAYOUTS_UNAVAILABLE", "PayPal marketplace payouts are not configured", "configuration"}
	errAlreadyConnected   = &connectionError{"PAYPAL_ALREADY_CONNECTED", "PayPal payouts are already connected", "already_connected"}
)
