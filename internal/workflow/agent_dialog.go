package workflow

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Choice is a quick reply offered under an assistant question.
type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AgentDialog is where the customer's conversation with their assistant stands: what it last asked,
// what the customer said about their availability, and the times it offered.
type AgentDialog struct {
	Stage     string        `json:"stage,omitempty"`   // address | confirm | availability | offer | move | manual | idle
	Awaiting  bool          `json:"awaiting"`          // the assistant is waiting for the customer's reply
	Choices   []Choice      `json:"choices,omitempty"` // quick replies for the open question
	Windows   []TimeWindow  `json:"windows,omitempty"` // the customer's stated availability
	Offered   []Appointment `json:"offered,omitempty"` // times the assistant offered
	Asked     int           `json:"asked"`             // Seq of the message in which it last asked
	Attempts  int           `json:"attempts,omitempty"`
	Pending   string        `json:"pending,omitempty"` // what a "yes" confirms: approve-quote | accept-price | accept-date | reschedule | cancel | approve-work
	PendingID string        `json:"pendingId,omitempty"`
	Notified  string        `json:"notified,omitempty"` // the milestones the assistant already reported, space separated
}

const reportedKeep = 12

// askCode marks a customer's message to their assistant that did not answer a question: a request of
// its own ("can we do Thursday instead?"), sent any time after a visit is booked.
const askCode = "ask"

func (d AgentDialog) reported(key string) bool {
	return slices.Contains(strings.Fields(d.Notified), key)
}

// report remembers that a milestone was told, keeping the most recent few.
func (d *AgentDialog) report(key string) {
	keys := append(strings.Fields(d.Notified), key)
	if len(keys) > reportedKeep {
		keys = keys[len(keys)-reportedKeep:]
	}
	d.Notified = strings.Join(keys, " ")
}

func (d *AgentDialog) clone() AgentDialog {
	if d == nil {
		return AgentDialog{}
	}
	copy := *d
	return copy
}

// applyAgentChat handles the two chat actions of the assistant: it speaks (server side only), and the
// customer replies to it.
func applyAgentChat(state *State, input Action) error {
	job := &state.Job
	if state.Intake == nil || job.ID == "" {
		return errors.New("an assistant can only talk once a professional is chosen")
	}
	if len(state.Intake.Messages) >= 500 {
		return errors.New("conversation limit reached; use the plan actions to continue")
	}
	actor := strings.ToLower(input.Actor)
	switch input.Action {
	case "agent-say":
		if !input.ByAgent || actor != "customer" && actor != "professional" {
			return errors.New("only an assistant can post this message")
		}
		text := strings.TrimSpace(input.Value)
		if len(text) < 2 || len(text) > 1500 {
			return errors.New("an assistant message needs 2 to 1500 characters")
		}
		name := job.Customer
		if actor == "professional" {
			name = job.Professional
		}
		audience := input.Audience
		if audience == "" {
			audience = "both"
		}
		state.Intake.Messages = append(state.Intake.Messages, Message{Seq: len(state.Intake.Messages) + 1, Role: "agent", Agent: AssistantName(*job, actor), Text: text, SenderName: name, ThreadProfileID: job.ProfessionalProfileID, Audience: audience, Code: input.NoteCode, Params: input.NoteParams})
		if input.Dialog != nil {
			dialog := *input.Dialog
			dialog.Asked = len(state.Intake.Messages) // the question's own Seq
			if actor == "professional" {
				state.ProDialog = &dialog
			} else {
				state.AgentDialog = &dialog
			}
		}
	case "agent-reply":
		dialog, name := state.AgentDialog, job.Customer
		if actor == "professional" {
			dialog, name = state.ProDialog, job.Professional
		}
		text := strings.TrimSpace(input.Value)
		if actor != "customer" && actor != "professional" || len(text) < 1 || len(text) > 500 {
			return errors.New("there is no question from your assistant to answer")
		}
		autonomyChoice := ""
		if actor == "customer" && dialog != nil && dialog.Stage == "booking-autonomy" && dialog.Awaiting {
			if input.ByAgent || dialog.PendingID != BookingAutonomyKey(*job) {
				return errors.New("the booking agreement changed; choose autonomy again")
			}
			if input.ChoiceID != "manual" && input.ChoiceID != "automatic|"+BookingAutonomyKey(*job) {
				return errors.New("use one of the booking autonomy choices")
			}
			autonomyChoice = input.ChoiceID
		}
		message := Message{Seq: len(state.Intake.Messages) + 1, Role: actor, Text: text, SenderName: name, ThreadProfileID: job.ProfessionalProfileID, Audience: actor, Choice: input.ChoiceID}
		if dialog == nil || !dialog.Awaiting {
			// With no question open, a customer may still ask their assistant for something once a visit is booked.
			if actor != "customer" || job.Appointment == nil || input.ChoiceID != "" {
				return errors.New("there is no question from your assistant to answer")
			}
			if dialog == nil {
				dialog = &AgentDialog{Stage: "idle"}
				state.AgentDialog = dialog
			}
			message.Code = askCode
		}
		if autonomyChoice != "" {
			if err := setBookingAutonomy(state, Action{Action: "set-booking-autonomy", Actor: actor, Value: autonomyChoice}); err != nil {
				return err
			}
		}
		state.Intake.Messages = append(state.Intake.Messages, message)
		dialog.Awaiting = false // the assistant has the answer; it sets this again if it needs more
	default:
		return errors.New("unknown assistant action")
	}
	return nil
}

// AgentNote is the line an assistant leaves in the shared thread when it negotiates for its person, so
// both sides can read the exchange. It returns the English text; viewers localize it from the code.
func AgentNote(role, code string, params map[string]string) (string, bool) {
	amount := func() string { return "€" + params["amount"] }
	start := func() string {
		zone, _ := time.LoadLocation("Europe/Amsterdam")
		t, err := time.Parse(time.RFC3339, params["start"])
		if err != nil {
			return params["start"]
		}
		return t.In(zone).Format("Mon 2 Jan, 15:04")
	}
	switch role + ":" + code {
	case "customer:counter-price":
		return "Could you do this for " + amount() + "? That is the most my customer can agree to.", true
	case "professional:counter-price":
		return "The lowest I can do is " + amount() + ".", true
	case "customer:accept-price", "professional:accept-price":
		return "Agreed on " + amount() + ".", true
	case "customer:propose-visit":
		return "Would " + start() + " work? None of your listed times fit my customer.", true
	case "professional:propose-visit":
		return "How about " + start() + "? It is the closest time I have.", true
	case "customer:reschedule-visit":
		return "Could we move the visit to " + start() + "? My customer asked for it.", true
	case "customer:accept-visit", "professional:accept-visit":
		return "Agreed: " + start() + ".", true
	}
	return "", false
}
