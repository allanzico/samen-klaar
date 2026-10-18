package workflow

import (
	"fmt"
	"time"
)

// WaitingState is the operational policy for one outstanding human decision.
// A reminder and operator escalation become due after 24 elapsed hours. The
// operator investigates and contacts the responsible person; this policy never
// approves work, changes a visit, or moves money.
type WaitingState struct {
	EventType        string `json:"eventType"`
	ResponsibleRole  string `json:"responsibleRole"`
	StartedAt        string `json:"startedAt"`
	Deadline         string `json:"deadline"`
	Anchor           string `json:"anchor"`
	ResolutionAction string `json:"resolutionAction"`
	Detail           string `json:"detail"`
}

func (wait WaitingState) Due(now time.Time) bool {
	deadline, err := time.Parse(time.RFC3339, wait.Deadline)
	return err == nil && !deadline.After(now)
}

func (wait WaitingState) EventKey(job Job) string {
	return fmt.Sprintf("%s:scope:%d:%s:%s:%s", job.ID, job.ScopeVersion, wait.EventType, wait.Anchor, wait.ResponsibleRole)
}

// WaitingStates returns active waits, including ones whose deadline has not
// passed. It is shared by reminders, delivery revalidation and operator cases.
func WaitingStates(state State) []WaitingState {
	job := state.Job
	waits := make([]WaitingState, 0, 3)
	if job.ID == "" || state.Cancelled || job.Paid || state.CancellationReview != nil && state.CancellationReview.Status == "pending" ||
		job.Dispute != nil && job.Dispute.Status != "resolved" && job.Dispute.Status != "rework" {
		return waits
	}
	add := func(event, role, started, anchor, action, detail string) {
		from, err := time.Parse(time.RFC3339, started)
		if err != nil || anchor == "" {
			return
		}
		waits = append(waits, WaitingState{EventType: event, ResponsibleRole: role, StartedAt: from.UTC().Format(time.RFC3339), Deadline: from.Add(24 * time.Hour).UTC().Format(time.RFC3339), Anchor: anchor, ResolutionAction: action, Detail: detail})
	}
	other := func(actor string) string {
		if actor == "customer" {
			return "professional"
		}
		return "customer"
	}
	if p := job.PriceProposal; p != nil && p.Status == "pending" && p.ScopeVersion == job.ScopeVersion {
		add("price-proposal-reminder", other(p.Actor), p.CreatedAt, p.ID, "decide-price", "The price proposal has had no response for 24 hours. Contact the recipient and record acceptance, decline or withdrawal.")
	}
	if p := job.DateProposal; p != nil && p.Status == "pending" {
		add("date-proposal-reminder", other(p.Actor), p.CreatedAt, p.ID, "decide-date", "The proposed visit time has had no response for 24 hours. Contact both parties and resolve the outstanding proposal.")
	}
	if p := job.Change; p != nil && p.Status == "pending" {
		add("scope-change-reminder", "customer", p.CreatedAt, fmt.Sprintf("%d:%s", p.ProposedFrom, p.CreatedAt), "decide-scope-change", "The proposed scope change has had no response for 24 hours. Contact the customer; existing scope remains in force until their decision.")
		return waits
	}
	if !job.WorkStarted && job.Appointment != nil && job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status == "confirmed" {
		add("missed-visit-reminder", "professional", job.Appointment.EndUTC, job.Appointment.StartUTC+":"+job.Appointment.EndUTC, "review-missed-visit", "The confirmed visit ended over 24 hours ago without work starting. Contact both parties to establish what happened and arrange a reviewed resolution; do not assume who failed to attend.")
	}
	if job.WorkStarted && !job.EvidenceSubmitted {
		started := job.WorkStartedAt
		if job.Appointment != nil && job.Appointment.EndUTC != "" {
			started = job.Appointment.EndUTC
		}
		add("evidence-overdue", "professional", started, started, "submit-evidence", "Completion evidence is still missing 24 hours after the visit. Contact the professional and request the required evidence.")
	}
	if job.EvidenceSubmitted && !job.DetailAdded {
		add("completion-note-overdue", "professional", job.EvidenceSubmittedAt, job.EvidenceSubmittedAt, "add-evidence", "The work note has been missing for 24 hours. Contact the professional and request the note required for customer review.")
	}
	if job.EvidenceSubmitted && job.DetailAdded && !job.CompletionApproved && len(MissingEvidence(job, true)) == 0 {
		started := job.EvidenceSubmittedAt
		if later(job.DetailAddedAt, started) {
			started = job.DetailAddedAt
		}
		add("completion-review-reminder", "customer", started, job.EvidenceSubmittedAt+":"+job.DetailAddedAt, "approve-completion", "The customer has not reviewed the completion submission within 24 hours. Contact them to review the evidence or report a problem.")
	}
	if job.CompletionApproved && job.PaymentApprovedVersion != job.ScopeVersion {
		add("payment-approval-reminder", "customer", job.CompletionApprovedAt, job.CompletionApprovedAt, "approve-payment", "The separate payment decision has been waiting for 24 hours. Contact the customer; their explicit approval is required.")
	}
	if job.CompletionApproved && job.PaymentApprovedVersion == job.ScopeVersion {
		add("checkout-reminder", "customer", job.PaymentApprovedAt, job.PaymentApprovedAt, "continue-payment", "Payment was approved 24 hours ago but has not completed. Ask the customer to check the saved PayPal status before continuing; reconcile uncertain results before retrying.")
	}
	return waits
}

func later(candidate, baseline string) bool {
	a, errA := time.Parse(time.RFC3339, candidate)
	b, errB := time.Parse(time.RFC3339, baseline)
	return errA == nil && (errB != nil || a.After(b))
}
