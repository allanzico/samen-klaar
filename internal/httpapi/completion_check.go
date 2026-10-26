package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

// Failed coverage checks save private feedback, never a customer completion submission.
func (s *Server) submitCheckedCompletion(ctx context.Context, principal identity.Principal, input Action) (State, int, error) {
	if principal.ActiveRole != identity.Professional || input.Actor != "professional" {
		return State{}, http.StatusForbidden, errors.New("only the assigned professional can send completion")
	}
	state, err := s.currentState(ctx, principal.User.ID, "professional", input.WorkspaceID)
	if err != nil {
		return State{}, http.StatusConflict, errors.New("the completion draft could not be loaded")
	}
	job := state.Job
	if !job.WorkStarted || job.Paid || job.CompletionApproved || state.Cancelled || job.Change != nil && job.Change.Status == "pending" || job.Dispute != nil && job.Dispute.Status == "open" {
		return State{}, http.StatusConflict, errors.New("completion cannot be sent in the current job state")
	}
	if job.EvidenceSubmitted {
		return state, http.StatusOK, nil // A repeated click cannot send a second completion notice.
	}
	draft := job
	draft.EvidenceReview = nil
	if !job.DetailAdded {
		return State{}, http.StatusConflict, errors.New("add a work note describing what you repaired and checked before sending")
	}
	if err := workflow.ValidateCompletionEvidence(draft, true); err != nil {
		return State{}, http.StatusConflict, err
	}
	if s.ai == nil || s.r2 == nil {
		return State{}, http.StatusServiceUnavailable, errors.New("completion checking is unavailable; your draft has not been sent")
	}
	fingerprint := workflow.CompletionEvidenceFingerprint(job)
	check := job.CompletionCheck
	if check == nil || check.EvidenceFingerprint != fingerprint {
		files, loadErr := loadEvidenceReviewFiles(ctx, s.r2, job, nil)
		if loadErr != nil {
			return State{}, loadErr.status, errors.New("the completion photos could not be loaded; your draft has not been sent")
		}
		draft.ServiceAddress, draft.Location, draft.ServiceLocation = "", nil, nil
		check, err = s.ai.ReviewEvidence(ctx, draft, files)
		if err != nil || check == nil {
			return State{}, http.StatusBadGateway, errors.New("completion checking could not finish; your draft has not been sent")
		}
		workflow.BindEvidenceReview(job, check)
		state, err = s.applyAction(ctx, principal.User.ID, "professional", Action{Action: "record-completion-check", Actor: "professional", CompletionCheckVerified: true, EvidenceReview: check, ExpectedScope: job.ScopeVersion, ExpectedEvidenceFingerprint: fingerprint}, input.WorkspaceID)
		if err != nil {
			return State{}, http.StatusConflict, err
		}
	}
	if workflow.ValidateReviewedCompletion(state.Job, check) != nil {
		return state, http.StatusOK, nil // The professional sees the findings and can improve this draft.
	}
	input.CompletionCheckRequired = true
	input.ExpectedScope, input.ExpectedEvidenceFingerprint = job.ScopeVersion, fingerprint
	state, err = s.applyAction(ctx, principal.User.ID, "professional", input, input.WorkspaceID)
	if err != nil {
		return State{}, http.StatusConflict, err
	}
	return state, http.StatusOK, nil
}
