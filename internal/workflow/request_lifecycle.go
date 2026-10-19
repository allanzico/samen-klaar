package workflow

import "errors"

// MaxOpenRequests caps how many unconfirmed requests a customer can have at once. Each open
// request keeps an assistant working on intake, matching and negotiation.
const MaxOpenRequests = 3

var (
	ErrOpenRequestLimit    = errors.New("You have 3 open requests. Finish, cancel or delete one before starting another.")
	ErrRequestNotDeletable = errors.New("Only requests without an approved plan can be deleted. Cancel it instead, or message the other person.")
	ErrRequestNotRemovable = errors.New("Only jobs the customer has not approved yet can be removed from your list.")
	ErrRequestNotFound     = errors.New("This request was not found.")
)

// RequestConfirmed reports whether a request has passed the point where its record must be kept:
// the customer approved the plan and price. Every booking, slot reservation, PayPal hold, start,
// evidence upload and payment happens after that approval, so the other checks are a safety net
// for records written by older releases or by an operator.
func RequestConfirmed(state State) bool {
	job := state.Job
	return job.QuoteApproved || job.Appointment != nil || job.AppointmentConfirmation != nil ||
		job.PaymentAuthorizedVersion != 0 || job.PaymentApprovedVersion != 0 || job.PaymentAuthorizationTiming != "" ||
		job.WorkStarted || job.EvidenceSubmitted || len(job.Evidence) > 0 || job.CompletionApproved ||
		job.Paid || job.PaymentTransactionID != "" || job.Dispute != nil || job.Review != nil ||
		state.CancellationReview != nil && state.CancellationReview.Status != ""
}

// RequestOpen reports whether a request counts towards MaxOpenRequests: started but not yet
// approved, cancelled, started or finished.
func RequestOpen(state State) bool {
	return !state.Cancelled && !RequestConfirmed(state)
}

// CustomerCanDelete reports whether the customer may delete the request outright. That covers
// drafts, requests still being described or matched, plans awaiting approval, and requests
// cancelled before approval. Approved, ongoing and finished requests keep their record.
func CustomerCanDelete(state State) bool {
	return !RequestConfirmed(state)
}

// ProfessionalCanRemove reports whether the assigned professional may take the request off their
// list. Removing an active request declines it, so the customer can choose someone else.
func ProfessionalCanRemove(state State) bool {
	return state.Job.ID != "" && !RequestConfirmed(state)
}

// CountOpenRequests counts the states that hold a place under MaxOpenRequests.
func CountOpenRequests(states []State) int {
	count := 0
	for _, state := range states {
		if RequestOpen(state) {
			count++
		}
	}
	return count
}

// CheckOpenRequestLimit returns ErrOpenRequestLimit when adding more requests would go over the cap.
func CheckOpenRequestLimit(open, adding int) error {
	if open+adding > MaxOpenRequests {
		return ErrOpenRequestLimit
	}
	return nil
}

// removeRequest lets the assigned professional take an unapproved request off their list. An
// active one is declined first, so the customer keeps their description and can choose another
// professional.
func removeRequest(state *State, input Action) error {
	if input.Actor != "professional" || !ProfessionalCanRemove(*state) {
		return ErrRequestNotRemovable
	}
	if state.Cancelled {
		return nil
	}
	return cancelRequest(state, input)
}
