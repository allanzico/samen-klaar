package workflow

// ConversationForRole is the request as one participant may see it: the other side's private
// exchanges with their assistant are removed, and a professional sees only their own thread.
func ConversationForRole(state State, role string) State {
	state.CompletionAutomation = ""
	if role == "customer" && BookingAutonomyCurrent(state.Job) {
		code := requestStatusCode(state, "customer")
		if code == "review" || code == "payment" {
			state.CompletionAutomation = "checking"
			if review := state.Job.EvidenceReview; review != nil && review.EvidenceFingerprint == CompletionEvidenceFingerprint(state.Job) {
				if ValidateAutonomousCompletion(state.Job) != nil {
					state.CompletionAutomation = "needs_review"
				} else if state.Job.CompletionApproved {
					state.CompletionAutomation = "releasing"
				}
			}
		}
	}
	if state.Job.PriceProposal != nil {
		proposal := *state.Job.PriceProposal
		dialog := state.ProDialog
		proposal.ProfessionalReviewRequired = proposal.Status == "pending" && proposal.Actor == "customer" && dialog != nil && dialog.Awaiting && dialog.Stage == "confirm" && dialog.Pending == "accept-price" && dialog.PendingID == proposal.ID
		state.Job.PriceProposal = &proposal
	}
	if role == "professional" {
		state.AgentDialog = nil
	} else {
		state.ProDialog = nil
	}
	if state.Intake == nil {
		return state
	}
	intake := *state.Intake
	intake.Messages = []Message{}
	for _, message := range state.Intake.Messages {
		switch message.Audience {
		case "customer":
			if role == "professional" {
				continue
			}
		case "professional":
			if role != "professional" {
				continue
			}
		}
		if role == "professional" && message.ThreadProfileID != "" && message.ThreadProfileID != state.Job.ProfessionalProfileID {
			continue
		}
		intake.Messages = append(intake.Messages, message)
	}
	state.Intake = &intake
	if role == "professional" {
		intake.ServiceAddress = ""
	}
	return state
}
