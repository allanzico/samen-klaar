package httpapi

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) paypalSellerOnboardingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.marketplace == nil {
		http.Error(w, "PayPal professional payouts are not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	actionURL, err := s.marketplace.BeginSellerOnboarding(ctx, principal.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
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
		http.Error(w, "PayPal professional payouts are not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	status, err := s.marketplace.SellerOnboardingStatus(ctx, principal.User.ID, r.URL.Query().Get("merchantIdInPayPal"))
	if err != nil {
		http.Error(w, "PayPal seller status could not be verified", http.StatusBadGateway)
		return
	}
	writeJSON(w, status)
}
