package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Server) runAutonomyAction(ctx context.Context, principal identity.Principal, input Action) (State, int, error) {
	state, err := s.currentState(ctx, principal.User.ID, "customer", input.WorkspaceID)
	if err != nil {
		return State{}, http.StatusConflict, errors.New("the booking could not be loaded for automatic review")
	}
	job := state.Job
	if principal.ActiveRole != identity.Customer || !input.ByAgent || !workflow.BookingAutonomyCurrent(job) || job.BookingAutonomy.Version != input.AutonomyVersion || state.Cancelled || state.CancellationReview != nil && state.CancellationReview.Status == "pending" || input.ExpectedScope != job.ScopeVersion || input.ExpectedEvidenceFingerprint != workflow.CompletionEvidenceFingerprint(job) {
		return State{}, http.StatusConflict, errors.New("booking autonomy or the completion record changed; customer review is required")
	}
	if input.Action == "review-completion-evidence" {
		if s.ai == nil || s.r2 == nil {
			return State{}, http.StatusServiceUnavailable, errors.New("automatic photo comparison is unavailable; review the work yourself")
		}
		files, loadErr := loadEvidenceReviewFiles(ctx, s.r2, job, nil)
		if loadErr != nil {
			return State{}, loadErr.status, errors.New(loadErr.message)
		}
		private := job
		private.ServiceAddress = ""
		private.ServiceLocation = nil
		private.Location = nil
		review, err := s.ai.ReviewEvidence(ctx, private, files)
		if err != nil || review == nil {
			return State{}, http.StatusBadGateway, errors.New("automatic photo comparison could not finish; review the work yourself")
		}
		workflow.BindEvidenceReview(job, review)
		input.Action = "record-evidence-review"
		input.EvidenceReview = review
		updated, err := s.applyAction(ctx, principal.User.ID, "customer", input, input.WorkspaceID)
		return updated, http.StatusConflict, err
	}
	if input.Action != "release-authorized-payment" || s.paypal == nil || !job.PaymentApprovedByAgent {
		return State{}, http.StatusConflict, errors.New("the customer must approve this payment")
	}
	if err := workflow.ValidateAutonomousCompletion(job); err != nil {
		return State{}, http.StatusConflict, err
	}
	if err := workflow.ValidatePaymentApproval(job); err != nil {
		return State{}, http.StatusConflict, err
	}
	status, err := s.paypal.Status(ctx, state)
	if err != nil {
		return State{}, http.StatusConflict, errors.New("payment status needs customer review")
	}
	if status == "capturing" || status == "capture_uncertain" {
		status, err = s.paypal.Reconcile(ctx, state)
		if err != nil {
			return State{}, http.StatusConflict, errors.New("payment release needs reconciliation before another attempt")
		}
	}
	if status != "captured" {
		if status != "authorized" {
			return State{}, http.StatusConflict, errors.New("a fresh PayPal approval is needed; automatic release has stopped")
		}
		if err := s.paypal.Capture(ctx, state); err != nil {
			return State{}, http.StatusConflict, errors.New("payment release could not be confirmed; check PayPal status before retrying")
		}
	}
	captureID, err := s.paypal.CaptureID(ctx, state)
	if err != nil || captureID == "" {
		return State{}, http.StatusConflict, errors.New("payment release has no verified receipt; check PayPal status")
	}
	updated, err := s.applyAction(ctx, principal.User.ID, "customer", Action{Action: "confirm-payment", Actor: "customer", ByAgent: true, PayPalCaptured: true, PaymentCaptureID: captureID}, input.WorkspaceID)
	return updated, http.StatusConflict, err
}
