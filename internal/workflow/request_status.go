package workflow

// RequestStatus names the current responsibility, rather than a past step.
func RequestStatus(state State, role string) (string, string) {
	job := state.Job
	if state.Cancelled {
		return "cancelled", "Cancelled"
	}
	if state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
		if state.CancellationReview.Kind == "late-reschedule" {
			return "reschedule_review", "Visit change under review"
		}
		if state.CancellationReview.Kind == "withdrawal" {
			return "withdrawal_review", "Withdrawal request under review"
		}
		return "cancellation_review", "Cancellation request under review"
	}
	if job.ID == "" {
		if state.Intake == nil {
			return "draft", "Start with a message"
		}
		if state.Intake.Step == "matches" {
			return "matching", "Review matching services"
		}
		return "describing", "Describing the job"
	}
	if job.Dispute != nil {
		switch job.Dispute.Status {
		case "open":
			if job.Paid {
				return "dispute_review", "Payment dispute under review"
			}
			return "dispute_review", "Issue under review · payment paused"
		case "refund_pending":
			return "refund_pending", "Full refund processing"
		case "partial_refund_review":
			return "dispute_review", "Partial PayPal refund under review"
		case "payment_reversal_review":
			return "dispute_review", "PayPal payment reversal under review"
		case "refunded":
			return "refunded", "Full refund complete"
		case "resolved", "rework":
			return "issue_resolved", "Issue reviewed"
		}
	}
	if job.Paid {
		return "paid", "Payment complete"
	}
	if job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status == "pending" {
		if role == "professional" {
			return "confirm_visit", "Confirm the reserved visit"
		}
		return "waiting_visit_confirmation", "Waiting for professional confirmation"
	}
	if job.AppointmentConfirmation != nil && (job.AppointmentConfirmation.Status == "declined" || job.AppointmentConfirmation.Status == "expired") {
		return "choose_visit", "Choose another visit time"
	}
	other := job.Professional
	if role == "professional" {
		other = job.Customer
	}
	if other == "" {
		other = "the other person"
	}
	if job.Change != nil && job.Change.Status == "pending" {
		if role == "customer" {
			return "review_change", "Review the additional work"
		}
		return "waiting_change", "Waiting for " + other
	}
	if job.DateProposal != nil && job.DateProposal.Status == "pending" {
		if job.DateProposal.Actor == role {
			return "waiting_date", "Waiting for " + other
		}
		return "review_date", "Review the proposed visit"
	}
	if job.PriceProposal != nil && job.PriceProposal.Status == "pending" {
		if job.PriceProposal.Actor == role {
			return "waiting_price", "Waiting for " + other
		}
		return "review_price", "Review the price offer"
	}
	if job.CompletionApproved {
		return "payment", "Ready for payment"
	}
	if job.EvidenceSubmitted {
		return "review", "Review completion"
	}
	if job.WorkStarted {
		return "in_progress", "Work in progress"
	}
	if job.Appointment != nil {
		return "booked", "Visit booked"
	}
	if job.QuoteApproved {
		return "booking", "Choose or propose a visit"
	}
	if role == "professional" {
		return "quote", "Waiting for customer approval"
	}
	return "quote", "Review the plan and price"
}
