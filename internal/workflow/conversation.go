package workflow

// ConversationForRole is the request as one participant may see it: the other side's private
// exchanges with their assistant are removed, and a professional sees only their own thread.
func ConversationForRole(state State, role string) State {
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
		state.AgentDialog = nil
	} else {
		state.ProDialog = nil
	}
	return state
}
