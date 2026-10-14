package workflow

import (
	"errors"
	"strings"
	"time"
)

func cancelRequest(state *State, input Action) error {
	job := &state.Job
	if state.Cancelled || job.WorkStarted || job.EvidenceSubmitted || job.CompletionApproved || job.Paid || job.PaymentApprovedVersion != 0 {
		return errors.New("cancellation is available before work and payment begin; message the other person about an active job")
	}
	if state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
		return errors.New("your late cancellation request is already waiting for operator review")
	}
	if len(strings.TrimSpace(input.Value)) > 500 {
		return errors.New("cancellation note must be 500 characters or fewer")
	}
	now := time.Now().UTC()
	if input.Actor == "customer" && job.Appointment != nil {
		start, err := time.Parse(time.RFC3339, job.Appointment.StartUTC)
		if err != nil {
			return errors.New("the booked visit time is invalid; contact support to cancel")
		}
		if start.Sub(now) < 24*time.Hour {
			state.CancellationReview = &CancellationRequest{
				Status: "pending", RequestedBy: input.Actor, Reason: strings.TrimSpace(input.Value), RequestedAt: now.Format(time.RFC3339),
			}
			addEvent(job, "Customer requested a late cancellation · operator review required; the visit remains scheduled for now", "attention")
			return nil
		}
	}
	state.Cancelled = true
	state.CancelledBy = input.Actor
	state.AvailableSlots = []Appointment{}
	if job.PriceProposal != nil && job.PriceProposal.Status == "pending" {
		job.PriceProposal.Status = "withdrawn"
	}
	if job.DateProposal != nil && job.DateProposal.Status == "pending" {
		job.DateProposal.Status = "withdrawn"
	}
	if job.Change != nil && job.Change.Status == "pending" {
		job.Change.Status = "withdrawn"
	}
	if job.ID != "" {
		addEvent(job, input.Actor+" cancelled the request before work began. Any booked visit was released. "+strings.TrimSpace(input.Value), "done")
	}
	return nil
}

func startAppointmentConfirmation(job *Job, appointment Appointment) error {
	due, err := AppointmentConfirmationDeadline(time.Now(), appointment.Timezone)
	if err != nil {
		return err
	}
	job.AppointmentConfirmation = &AppointmentConfirmation{
		Status: "pending", DueAt: due.UTC().Format(time.RFC3339), Appointment: appointment,
	}
	return nil
}
