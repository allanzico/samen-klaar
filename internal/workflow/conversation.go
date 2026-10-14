package workflow

// ConversationForRole keeps discussions with a previous professional visible
// to the customer without sharing them with a newly selected professional.
func ConversationForRole(state State, role string) State {
	if role != "professional" || state.Intake == nil {
		return state
	}
	intake := *state.Intake
	intake.Messages = []Message{}
	for _, message := range state.Intake.Messages {
		if message.ThreadProfileID == "" || message.ThreadProfileID == state.Job.ProfessionalProfileID {
			intake.Messages = append(intake.Messages, message)
		}
	}
	state.Intake = &intake
	return state
}
