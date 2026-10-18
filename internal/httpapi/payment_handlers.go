package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Server) paypalCreateOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.paypal == nil {
		http.Error(w, "PayPal sandbox is not enabled", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	orderID, approvalURL, err := s.paypal.CreateOrder(ctx, state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	status, err := s.paypal.Status(ctx, state)
	if err != nil {
		http.Error(w, "PayPal operation status is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"orderId": orderID, "approvalUrl": approvalURL, "status": status})
}

func (s *Server) paypalAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.paypal == nil {
		http.Error(w, "PayPal sandbox is not enabled", http.StatusServiceUnavailable)
		return
	}
	var input struct {
		OrderID string `json:"orderId"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.OrderID == "" {
		http.Error(w, "PayPal approval details are invalid", http.StatusBadRequest)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err == nil {
		err = s.paypal.Authorize(ctx, state, input.OrderID)
	}
	if err == nil && state.Job.PaymentAuthorizationTiming == workflow.PaymentAuthorizationAtBooking && !state.Job.WorkStarted {
		state, err = s.recordBookingAuthorization(ctx, principal.User.ID, string(principal.ActiveRole), state)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	status, err := s.paypal.Status(ctx, state)
	if err != nil {
		http.Error(w, "PayPal operation status is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"status": status})
}

func (s *Server) paypalCaptureHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.paypal == nil {
		http.Error(w, "PayPal sandbox is not enabled", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err = workflow.ValidatePaymentApproval(state.Job); err == nil {
		err = s.paypal.Capture(ctx, state)
	}
	if err == nil {
		var captureID string
		captureID, err = s.paypal.CaptureID(ctx, state)
		if err == nil && captureID == "" {
			err = errors.New("PayPal capture completed without a recorded capture ID; reconcile payment status")
		}
		if err == nil {
			state, err = s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), Action{Action: "confirm-payment", Actor: string(principal.ActiveRole), PayPalCaptured: true, PaymentCaptureID: captureID}, r.URL.Query().Get("workspaceId"))
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	status, err := s.paypal.Status(ctx, state)
	if err != nil {
		http.Error(w, "PayPal operation status is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"status": status, "state": state})
	s.TriggerAgents(state.WorkspaceID) // the professional's assistant reports the payment
}

func (s *Server) paypalReconcileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.paypal == nil {
		http.Error(w, "PayPal sandbox is not enabled", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	status, err := s.paypal.Reconcile(ctx, state)
	if err == nil && status == "authorized" && state.Job.PaymentAuthorizationTiming == workflow.PaymentAuthorizationAtBooking && !state.Job.WorkStarted {
		state, err = s.recordBookingAuthorization(ctx, principal.User.ID, string(principal.ActiveRole), state)
	}
	if err == nil && status == "captured" && !state.Job.Paid {
		var captureID string
		captureID, err = s.paypal.CaptureID(ctx, state)
		if err == nil && captureID == "" {
			err = errors.New("PayPal reports a capture without a recorded capture ID")
		}
		if err == nil {
			state, err = s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), Action{Action: "confirm-payment", Actor: string(principal.ActiveRole), PayPalCaptured: true, PaymentCaptureID: captureID}, r.URL.Query().Get("workspaceId"))
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"status": status, "state": state})
	if state.Job.Paid {
		s.TriggerAgents(state.WorkspaceID) // both assistants report the payment
	}
}

// The database repeats these identity checks under the workspace/payment locks.
// A callback that cannot save the projection leaves the durable authorization
// recoverable; booking cancellation/expiry queues its own release transactionally.
func (s *Server) recordBookingAuthorization(ctx context.Context, accountID, role string, state State) (State, error) {
	record, err := s.paypal.VerifiedAuthorization(ctx, state)
	if err != nil {
		return state, err
	}
	return s.applyAction(ctx, accountID, role, Action{
		Action: "record-payment-authorization", Actor: role, PayPalAuthorized: true,
		PaymentAuthorizationID:           record.AuthorizationID,
		PaymentAuthorizationRequestID:    record.AuthRequestID,
		PaymentAuthorizationScopeVersion: record.ScopeVersion,
	}, state.WorkspaceID)
}

func (s *Server) paypalStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.paypal == nil {
		http.Error(w, "PayPal is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal := principalForRequest(r)
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	status, err := s.paypal.Status(ctx, state)
	if err != nil {
		http.Error(w, "PayPal operation status is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"status": status})
}
