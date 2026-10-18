package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

func (s *Store) resolveConsumerCaseTx(ctx context.Context, tx pgx.Tx, state *workflow.State, kind, decision, rationale string) (string, error) {
	request := state.CancellationReview
	if request == nil || request.Status != "pending" || request.Kind != kind {
		return "", errors.New("this visit change or withdrawal is no longer awaiting review")
	}
	action := ""
	switch kind {
	case "late-reschedule":
		if state.Job.WorkStarted || state.Job.Paid {
			return "", errors.New("a visit cannot be rescheduled after work or payment")
		}
		p := state.Job.DateProposal
		if p == nil || p.ID != request.ProposalID || p.Status != "review_pending" || request.ProposedAppointment == nil {
			return "", errors.New("the reviewed visit proposal changed")
		}
		switch decision {
		case "approve-reschedule":
			slot, err := workflow.ValidateProposedAppointment(*request.ProposedAppointment, state.Job.Quote.DurationMinutes)
			if err != nil {
				return "", err
			}
			state.Job.Appointment = &slot
			if err := reserveAgreedDate(ctx, tx, &state.Job); err != nil {
				return "", err
			}
			p.Status = "accepted"
			state.Job.AppointmentConfirmation = &workflow.AppointmentConfirmation{Status: "confirmed", RespondedAt: time.Now().UTC().Format(time.RFC3339), Appointment: *state.Job.Appointment}
			request.Status = "approved"
			action = "approve-late-reschedule"
		case "keep-visit":
			p.Status = "declined"
			request.Status = "declined"
			action = "decline-late-reschedule"
		default:
			return "", errors.New("choose approve-reschedule or keep-visit")
		}
	case "withdrawal":
		switch decision {
		case "approve-withdrawal":
			if state.Job.WorkStarted || state.Job.EvidenceSubmitted || state.Job.CompletionApproved || state.Job.Paid {
				return "", errors.New("active or paid work requires an issue review; no automatic cancellation or refund is available")
			}
			if err := s.enqueueAuthorizationReleaseTx(ctx, tx, state.WorkspaceID, state.Job); err != nil {
				return "", err
			}
			if state.Job.ProfessionalAccountID != "" {
				if err := releaseCancelledBooking(ctx, tx, state.Job); err != nil {
					return "", err
				}
			}
			state.Cancelled, state.CancelledBy = true, "customer"
			state.AvailableSlots = []workflow.Appointment{}
			state.Job.PaymentAuthorizedVersion = 0
			if state.Job.DateProposal != nil && (state.Job.DateProposal.Status == "pending" || state.Job.DateProposal.Status == "review_pending") {
				state.Job.DateProposal.Status = "withdrawn"
			}
			if state.Job.PriceProposal != nil && state.Job.PriceProposal.Status == "pending" {
				state.Job.PriceProposal.Status = "withdrawn"
			}
			if state.Job.Change != nil && state.Job.Change.Status == "pending" {
				state.Job.Change.Status = "withdrawn"
			}
			request.Status = "approved"
			action = "approve-withdrawal"
		case "review-work":
			if state.Job.Dispute == nil || state.Job.Dispute.Status == "resolved" || state.Job.Dispute.Status == "rework" {
				state.Job.Dispute = &workflow.DisputeCase{Status: "open", ReportedBy: "customer", Reason: request.Reason, ReportedAt: request.RequestedAt}
			}
			request.Status = "referred"
			action = "refer-withdrawal-to-work-review"
		case "keep-request":
			request.Status = "declined"
			action = "decline-withdrawal"
		default:
			return "", errors.New("choose approve-withdrawal, review-work, or keep-request")
		}
	default:
		return "", errors.New("unsupported consumer case")
	}
	request.Resolution, request.ResolvedAt = rationale, time.Now().UTC().Format(time.RFC3339)
	addOperatorEvent(&state.Job, "Operator reviewed "+kind+" · "+decision, rationale)
	return action, nil
}
