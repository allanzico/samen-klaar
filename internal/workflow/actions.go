package workflow

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

func Apply(state *State, input Action) error {
	if state.Cancelled && input.Action != "send-message" && !(input.Action == "return-to-matches" && state.CancelledBy == "professional") {
		return errors.New("this request is cancelled; create a new request to book another service")
	}
	if state.CancellationReview != nil && state.CancellationReview.Status == "pending" && input.Action != "send-message" && input.Action != "cancel-request" {
		return errors.New("this visit is waiting for operator review of a cancellation request")
	}
	if state.Job.AppointmentConfirmation != nil && state.Job.AppointmentConfirmation.Status == "pending" && input.Action != "send-message" && input.Action != "cancel-request" && input.Action != "confirm-appointment" && input.Action != "decline-appointment" {
		return errors.New("the professional must confirm or decline this visit before other job steps continue")
	}
	if input.Action == "start-intake" || input.Action == "answer-intake" || input.Action == "set-location" || input.Action == "join-waitlist" || input.Action == "refresh-matches" || input.Action == "select-match" || input.Action == "return-to-matches" || input.Action == "revise-intake" {
		return applyIntake(state, input)
	}
	job := &state.Job
	actor := strings.ToLower(input.Actor)
	customer := actor == "customer"
	professional := actor == "professional"
	if !customer && !professional {
		return errors.New("action requires a customer or professional account")
	}

	if strings.Contains(input.Action, "price") || strings.Contains(input.Action, "date") {
		return applyNegotiation(state, input)
	}
	if job.Change != nil && job.Change.Status == "pending" && (input.Action == "start-job" || input.Action == "submit-evidence" || input.Action == "approve-completion") {
		return errors.New("resolve the proposed scope change first")
	}

	switch input.Action {
	case "set-service-address":
		address := strings.TrimSpace(input.Value)
		if !customer || job.ID == "" || job.WorkStarted || job.Paid || job.Appointment != nil || len(address) < 8 || len(address) > 250 {
			return errors.New("save a full service address after selecting a professional and before a visit is booked")
		}
		job.ServiceAddress = address
		addEvent(job, "Customer saved the private service address for the assigned professional", "done")
	case "cancel-request":
		return cancelRequest(state, input)

	case "send-message":
		text := strings.TrimSpace(input.Value)
		if job.ID == "" || state.Intake == nil || len(text) < 2 || len(text) > 500 {
			return errors.New("send a message of 2 to 500 characters about an assigned request")
		}
		if len(state.Intake.Messages) >= 500 {
			return errors.New("conversation limit reached; use the plan actions to continue")
		}
		name := job.Customer
		if professional {
			name = job.Professional
		}
		state.Intake.Messages = append(state.Intake.Messages, Message{Role: actor, Text: text, SenderName: name, ThreadProfileID: job.ProfessionalProfileID})
	case "report-issue":
		reason := strings.TrimSpace(input.Value)
		if !customer || job.ID == "" || !job.WorkStarted || job.Paid || job.Dispute != nil && job.Dispute.Status == "open" || len(reason) < 10 || len(reason) > 1000 {
			return errors.New("describe the incomplete or damaged work in 10 to 1000 characters while the job is active")
		}
		job.Dispute = &DisputeCase{Status: "open", ReportedBy: actor, Reason: reason, ReportedAt: time.Now().UTC().Format(time.RFC3339)}
		addEvent(job, "Customer reported an issue · payment is paused pending operator review", "attention")
	case "confirm-appointment":
		booking := job.AppointmentConfirmation
		if !professional || job.Appointment == nil || booking == nil || booking.Status != "pending" {
			return errors.New("only the assigned professional can confirm a visit awaiting confirmation")
		}
		due, err := time.Parse(time.RFC3339, booking.DueAt)
		if err != nil || time.Now().After(due) {
			return errors.New("the visit confirmation window has expired; reload for available alternatives")
		}
		booking.Status = "confirmed"
		booking.RespondedAt = time.Now().UTC().Format(time.RFC3339)
		addEvent(job, "Professional confirmed the visit", "done")
	case "decline-appointment":
		booking := job.AppointmentConfirmation
		if !professional || job.Appointment == nil || booking == nil || booking.Status != "pending" {
			return errors.New("only the assigned professional can decline a visit awaiting confirmation")
		}
		booking.Status = "declined"
		booking.RespondedAt = time.Now().UTC().Format(time.RFC3339)
		job.Appointment = nil
		addEvent(job, "Professional could not confirm the visit · choose another available time", "attention")

	case "approve-and-book":
		if !customer || job.ID == "" || job.QuoteApproved || job.NeedsClarification || pendingPrice(*job) || job.Appointment != nil {
			return errors.New("resolve the plan and price before booking")
		}
		var selected *Appointment
		for _, slot := range state.AvailableSlots {
			if slot.SlotID == input.Value {
				copy := slot
				selected = &copy
				break
			}
		}
		if selected == nil {
			return errors.New("choose an available appointment")
		}
		if serviceAddressRequired(*state) && job.ServiceAddress == "" {
			return errors.New("add the full service address for your selected professional before approving and booking")
		}
		job.QuoteApproved, job.Appointment = true, selected
		if err := startAppointmentConfirmation(job, *selected); err != nil {
			return err
		}
		addEvent(job, "Customer approved the current plan and price", "done")
		addEvent(job, "Visit reserved · awaiting professional confirmation within two business hours · "+selected.StartsAt, "attention")
	case "approve-quote":
		if !customer || job.ID == "" || job.QuoteApproved || job.NeedsClarification || pendingPrice(*job) {
			return errors.New("answer the clarification question before approving this quote")
		}
		if serviceAddressRequired(*state) && job.ServiceAddress == "" {
			return errors.New("add the full service address for your selected professional before approving the plan")
		}
		job.QuoteApproved = true
		addEvent(job, "Customer approved the current plan and price", "done")
	case "answer-clarification":
		answer := strings.TrimSpace(input.Value)
		if !customer || !job.NeedsClarification || len(answer) < 3 || len(answer) > 500 {
			return errors.New("provide a clarification answer of 3 to 500 characters")
		}
		job.NeedsClarification = false
		job.ClarificationAnswer = answer
		addEvent(job, "Customer answered the clarification question", "done")
	case "book-appointment":
		if !customer || !job.QuoteApproved || job.Appointment != nil || job.DateProposal != nil && job.DateProposal.Status == "pending" {
			return errors.New("approve the quote before choosing an appointment")
		}
		if serviceAddressRequired(*state) && job.ServiceAddress == "" {
			return errors.New("add the full service address for your selected professional before booking")
		}
		var selected *Appointment
		for _, slot := range state.AvailableSlots {
			if slot.SlotID == input.Value {
				copy := slot
				selected = &copy
				break
			}
		}
		if selected == nil {
			return errors.New("choose an available appointment")
		}
		job.Appointment = selected
		if err := startAppointmentConfirmation(job, *selected); err != nil {
			return err
		}
		available := make([]Appointment, 0, len(state.AvailableSlots)-1)
		for _, slot := range state.AvailableSlots {
			if slot.SlotID != selected.SlotID {
				available = append(available, slot)
			}
		}
		state.AvailableSlots = available
		addEvent(job, "Visit reserved · awaiting professional confirmation within two business hours · "+selected.StartsAt, "attention")
	case "start-job":
		if !professional || !job.QuoteApproved || job.Appointment == nil || job.WorkStarted || job.DateProposal != nil && job.DateProposal.Status == "pending" || job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status != "confirmed" {
			return errors.New("a confirmed appointment is required before work starts")
		}
		job.WorkStarted = true
		job.WorkStartedAt = time.Now().UTC().Format(time.RFC3339)
		addEvent(job, "Professional marked the job in progress", "done")
	case "submit-evidence":
		if !professional || !job.WorkStarted || job.EvidenceSubmitted {
			return errors.New("work must be in progress before completion is submitted")
		}
		hasUpload := false
		for _, evidence := range job.Evidence {
			if evidence.AssetID != "" && evidence.ContentType != "" {
				hasUpload = true
				break
			}
		}
		if !hasUpload {
			return errors.New("upload at least one real photo or document before submitting completion")
		}
		job.EvidenceSubmitted = true
		job.EvidenceSubmittedAt = time.Now().UTC().Format(time.RFC3339)
		job.EvidenceReview = nil
		addEvent(job, "Professional submitted completion documentation", "done")
		if !job.DetailAdded {
			addEvent(job, "Completion work note still needed", "attention")
		}
	case "record-evidence":
		if !professional || !job.WorkStarted || job.Paid || job.CompletionApproved || input.Evidence.AssetID == "" || input.Evidence.ContentType == "" {
			return errors.New("a professional may upload supported evidence after work starts")
		}
		for _, existing := range job.Evidence {
			if existing.AssetID == input.Evidence.AssetID {
				return errors.New("this evidence file has already been recorded")
			}
		}
		job.Evidence = append(job.Evidence, input.Evidence)
		job.EvidenceReview = nil
		addEvent(job, "Professional uploaded completion evidence to private storage", "done")
	case "record-evidence-review":
		if !customer || !job.EvidenceSubmitted || job.Paid || input.EvidenceReview == nil || input.ExpectedScope != job.ScopeVersion {
			return errors.New("the customer can review submitted evidence before payment")
		}
		job.EvidenceReview = input.EvidenceReview
		addEvent(job, "AI prepared an evidence coverage summary · customer review required", "done")
	case "request-details":
		if !customer || !job.EvidenceSubmitted || job.DetailAdded || job.DetailsRequested {
			return errors.New("there is no missing documentation to request")
		}
		job.DetailsRequested = true
		addEvent(job, "Customer requested the completion work note", "attention")
	case "add-evidence":
		note := strings.TrimSpace(input.Value)
		if !professional || (!job.WorkStarted && !job.DetailsRequested) || job.CompletionApproved || job.Paid || job.DetailAdded || len(note) < 12 || len(note) > 1200 {
			return errors.New("add a completion work note while work is in progress or documentation is requested")
		}
		job.DetailAdded = true
		job.Evidence = append(job.Evidence, Evidence{Label: "Professional work note", Description: note, Kind: "note"})
		addEvent(job, "Professional added a completion work note", "done")
	case "approve-completion":
		if !customer || !job.EvidenceSubmitted || !job.DetailAdded || job.CompletionApproved {
			return errors.New("completion stays blocked until required documentation is present and reviewed")
		}
		if job.Dispute != nil && job.Dispute.Status == "open" {
			return errors.New("completion approval is paused while an issue is under operator review")
		}
		job.CompletionApproved = true
		job.CompletionApprovedAt = time.Now().UTC().Format(time.RFC3339)
		addEvent(job, "Customer approved completion for scope v"+strconv.Itoa(job.ScopeVersion), "done")
	case "approve-payment":
		if !customer || !job.CompletionApproved || job.Paid {
			return errors.New("customer completion approval is required before payment approval")
		}
		if job.Dispute != nil && job.Dispute.Status == "open" {
			return errors.New("payment approval is paused while an issue is under operator review")
		}
		if job.PaymentApprovedVersion == job.ScopeVersion {
			return errors.New("payment is already approved for this scope; continue to PayPal or reconcile its status")
		}
		if job.Change != nil && job.Change.Status == "pending" {
			return errors.New("resolve the proposed scope change before payment approval")
		}
		job.PaymentApprovedVersion = job.ScopeVersion
		addEvent(job, "Customer approved the payment amount for scope v"+strconv.Itoa(job.ScopeVersion), "done")
	case "confirm-payment":
		if !customer || !job.CompletionApproved || job.PaymentApprovedVersion != job.ScopeVersion || job.Paid {
			return errors.New("payment requires customer approval for the current approved scope")
		}
		if !input.PayPalCaptured || strings.TrimSpace(input.PaymentCaptureID) == "" {
			return errors.New("a verified PayPal capture is required to mark a request as paid")
		}
		if job.Change != nil && job.Change.Status == "pending" {
			return errors.New("payment is blocked while a scope change is pending")
		}
		job.Paid = true
		job.PaymentTransactionID = input.PaymentCaptureID
		addEvent(job, "PayPal capture completed for scope v"+strconv.Itoa(job.ScopeVersion), "done")
	case "submit-review":
		comment := strings.TrimSpace(input.Value)
		if !customer || !job.Paid || job.PaymentRefunded || !job.CompletionApproved || job.Review != nil || input.Rating < 1 || input.Rating > 5 || len(comment) > 1000 || comment != "" && len(comment) < 3 {
			return errors.New("a customer may leave one 1-to-5-star review with an optional comment after a completed, paid job")
		}
		job.Review = &JobReview{Rating: input.Rating, Comment: comment, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
		addEvent(job, "Customer left a verified completed-job review", "done")
	case "respond-to-review":
		response := strings.TrimSpace(input.Value)
		if !professional || !job.Paid || job.Review == nil || job.Review.ProfessionalResponse != "" || len(response) < 3 || len(response) > 1000 {
			return errors.New("the assigned professional may post one response of 3 to 1000 characters to a verified review")
		}
		job.Review.ProfessionalResponse = response
		job.Review.RespondedAt = time.Now().UTC().Format(time.RFC3339)
		addEvent(job, "Professional responded to the verified job review", "done")
	case "propose-change":
		description := strings.TrimSpace(input.Value)
		if !professional || !job.QuoteApproved || job.Change != nil && job.Change.Status == "pending" || job.Paid || len(description) < 5 || len(description) > 300 || input.ChangeAmountMinor <= 0 || input.ChangeAmountMinor > 100_000_000 {
			return errors.New("a new change cannot be proposed in the current state")
		}
		job.Change = &ScopeChange{Status: "pending", Description: description, AmountMinor: input.ChangeAmountMinor, ProposedFrom: job.ScopeVersion}
		addEvent(job, "Professional proposed an additional service and amount · customer decision required", "attention")
	case "approve-change":
		if !customer || job.Change == nil || job.Change.Status != "pending" {
			return errors.New("there is no pending change to approve")
		}
		job.Change.Status = "approved"
		job.ScopeVersion++
		job.Quote.Lines = append(job.Quote.Lines, QuoteLine{Label: job.Change.Description, AmountMinor: job.Change.AmountMinor, Basis: "Professional proposal · customer-approved change"})
		job.Quote.TotalMinor += job.Change.AmountMinor
		job.ScopeHistory = append(job.ScopeHistory, scopeSnapshot(*job))
		job.CompletionApproved = false
		job.CompletionApprovedAt = ""
		job.PaymentApprovedVersion = 0
		job.DetailAdded = false
		job.DetailsRequested = false
		job.EvidenceSubmitted = false
		job.EvidenceSubmittedAt = ""
		job.EvidenceReview = nil
		addEvent(job, "Customer approved scope v"+strconv.Itoa(job.ScopeVersion)+" · price and documentation updated", "done")
	case "decline-change":
		if !customer || job.Change == nil || job.Change.Status != "pending" {
			return errors.New("there is no pending change to decline")
		}
		job.Change.Status = "declined"
		addEvent(job, "Customer declined the change · approved scope v"+strconv.Itoa(job.ScopeVersion)+" remains", "done")
	default:
		return errors.New("unknown action")
	}
	return nil
}

// ValidatePaymentApproval enforces the workflow preconditions for payment.
func ValidatePaymentApproval(job Job) error {
	if !job.CompletionApproved || job.PaymentApprovedVersion != job.ScopeVersion || job.Paid {
		return errors.New("completion and the current scope amount must be approved before PayPal checkout")
	}
	if job.Change != nil && job.Change.Status == "pending" {
		return errors.New("payment is blocked while a scope change is pending")
	}
	if job.Dispute != nil && job.Dispute.Status == "open" {
		return errors.New("payment is paused while an issue is under operator review")
	}
	return nil
}
