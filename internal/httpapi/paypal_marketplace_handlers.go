package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

func (s *Server) paypalSellerOnboardingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.marketplace == nil {
		writePayPalConnectionError(w, "PAYPAL_PAYOUTS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	actionURL, err := s.marketplace.BeginSellerOnboarding(ctx, principal.User.ID)
	if err != nil {
		reportPayPalConnectionError(w, err, "start", "PAYPAL_CONNECTION_START_FAILED", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"actionUrl": actionURL})
}

func (s *Server) paypalSellerStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.marketplace == nil {
		writePayPalConnectionError(w, "PAYPAL_PAYOUTS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	status, err := s.marketplace.SellerOnboardingStatus(ctx, principal.User.ID, r.URL.Query().Get("merchantIdInPayPal"))
	if err != nil {
		reportPayPalConnectionError(w, err, "verify", "PAYPAL_CONNECTION_VERIFY_FAILED", http.StatusBadGateway)
		return
	}
	writeJSON(w, status)
}

func reportPayPalConnectionError(w http.ResponseWriter, err error, operation, code string, status int) {
	var public interface{ PayPalConnectionCode() string }
	if errors.As(err, &public) {
		switch public.PayPalConnectionCode() {
		case "PAYPAL_PAYOUTS_UNAVAILABLE":
			code, status = "PAYPAL_PAYOUTS_UNAVAILABLE", http.StatusServiceUnavailable
		case "PAYPAL_ALREADY_CONNECTED":
			code, status = "PAYPAL_ALREADY_CONNECTED", http.StatusConflict
		}
	}
	fields := []any{"operation", operation, "code", code}
	var stage interface{ PayPalConnectionStage() string }
	if errors.As(err, &stage) {
		fields = append(fields, "stage", stage.PayPalConnectionStage())
	}
	var diagnostic interface {
		PayPalDiagnostic() (int, string, string, string)
	}
	if errors.As(err, &diagnostic) {
		providerStatus, providerCode, issue, debugID := diagnostic.PayPalDiagnostic()
		fields = append(fields, "provider_status", providerStatus, "provider_code", providerCode, "provider_issue", issue, "paypal_debug_id", debugID)
	}
	// Do not log err: errors from an upstream provider or database can contain
	// identifiers or credentials. The adapter supplies bounded diagnostic fields.
	slog.Warn("PayPal professional connection failed", fields...)
	writePayPalConnectionError(w, code, status)
}

func writePayPalConnectionError(w http.ResponseWriter, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	writeJSON(w, map[string]string{"code": code})
}
