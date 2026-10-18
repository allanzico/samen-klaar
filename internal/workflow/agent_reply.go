package workflow

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ReplyQuery is what a language model is asked to interpret: one free-form reply to one question the
// assistant asked. The reply is untrusted text; only the structured intent below comes back.
type ReplyQuery struct {
	Stage    string   `json:"stage"`             // address | confirm | availability | offer | ask | move
	Question string   `json:"question"`          // what the assistant asked
	Options  []string `json:"options,omitempty"` // the offered times, numbered from 1
	Reply    string   `json:"reply"`             // what the person wrote
	Locale   string   `json:"locale"`            // en or nl
	Now      string   `json:"now"`               // RFC 3339, in the person's zone
	Timezone string   `json:"timezone"`
}

// ReplyIntent is what a reply means for the open question.
type ReplyIntent struct {
	Intent  string       `json:"intent"`  // yes | no | choose | availability | ask_pro | address | reschedule | cancel | unclear
	Choice  int          `json:"choice"`  // 1-based, with "choose"
	Windows []TimeWindow `json:"windows"` // with "availability", and with "reschedule" when the person named days or times
	Address string       `json:"address"` // with "address"
}

var stageIntents = map[string][]string{
	"address":      {"address", "unclear"},
	"confirm":      {"yes", "no", "availability", "unclear"},
	"availability": {"availability", "unclear"},
	"offer":        {"choose", "yes", "no", "ask_pro", "availability", "unclear"},
	// A customer's own request to their assistant after booking, and the follow-up asking when to move the visit to.
	"ask":  {"reschedule", "cancel", "unclear"},
	"move": {"availability", "cancel", "no", "unclear"},
}

// Sanitize keeps only what the open question allows, so a model can never steer the assistant outside
// the conversation: bad shapes, out-of-range numbers and impossible times become "unclear".
func (r ReplyIntent) Sanitize(stage string, offered int) ReplyIntent {
	unclear := ReplyIntent{Intent: "unclear"}
	if !slices.Contains(stageIntents[stage], r.Intent) {
		return unclear
	}
	switch r.Intent {
	case "choose":
		if r.Choice < 1 || r.Choice > offered {
			return unclear
		}
		return ReplyIntent{Intent: "choose", Choice: r.Choice}
	case "address":
		address := strings.TrimSpace(r.Address)
		if len(address) < 8 || len(address) > 250 || !strings.ContainsAny(address, "0123456789") {
			return unclear
		}
		return ReplyIntent{Intent: "address", Address: address}
	case "availability":
		windows := cleanWindows(r.Windows)
		if len(windows) == 0 {
			return unclear
		}
		return ReplyIntent{Intent: "availability", Windows: windows}
	case "reschedule":
		// Days or times are optional here: "can we move the visit?" says nothing about when.
		return ReplyIntent{Intent: "reschedule", Windows: cleanWindows(r.Windows)}
	}
	return ReplyIntent{Intent: r.Intent}
}

// cleanWindows keeps the windows that describe a real stretch of time, at most eight.
func cleanWindows(in []TimeWindow) []TimeWindow {
	var windows []TimeWindow
	for _, w := range in {
		if w.From < 0 || w.To > 24*60 || w.From >= w.To || w.FromDate != "" && !validDate(w.FromDate) || w.ToDate != "" && !validDate(w.ToDate) || len(windows) >= 8 {
			continue
		}
		if w.FromDate != "" && w.ToDate != "" && w.ToDate < w.FromDate || slices.ContainsFunc(w.Weekdays, func(d int) bool { return d < 1 || d > 7 }) {
			continue
		}
		windows = append(windows, w)
	}
	return windows
}

func validDate(s string) bool { _, err := time.Parse("2006-01-02", s); return err == nil }

var (
	cancelWords = regexp.MustCompile(`\b(cancel\w*|call (it|this|that|the visit|the appointment) off|no longer need|(don'?t|do not) (need|want) (the|this|that|a|an|him|her|them|it)\b|scrap (it|this|that|the)|annuleer\w*|annuleren|afzeggen|zeg\w* (het |de afspraak |dit )?af|laat maar|niet meer nodig|hoeft niet meer)`)
	notCancel   = regexp.MustCompile(`\b(don'?t|do not|never|please don'?t|niet|geen)\s+(want to |wil )?(cancel\w*|annuleer\w*|annuleren)`)
	moveWords   = regexp.MustCompile(`\b(re-?schedul\w*|move|moving|postpone\w*|push(ed)? (it |this |the visit )?back|change (the|my|our) (visit|time|date|day|appointment)|(another|other|different|new) (day|time|date)|instead|rather|verzet\w*|verplaats\w*|uitstel\w*|(andere|ander|nieuwe) (dag|tijd|tijdstip|datum)|in plaats|liever)`)
)

// replyStage is the kind of question a reply answers: the open question's stage, or "ask" for a customer's
// request that answered nothing.
func replyStage(d AgentDialog, reply *Message) string {
	if reply != nil && reply.Code == askCode {
		return "ask"
	}
	return d.Stage
}

// localIntent reads a reply without a model: quick-reply buttons, yes/no words, a number, days and
// times in plain English or Dutch, and an address with a house number.
func (c AgentContext) localIntent(d AgentDialog, reply *Message) ReplyIntent {
	if reply.Choice != "" {
		switch {
		case strings.HasPrefix(reply.Choice, "slot:"):
			for i, slot := range d.Offered {
				if slot.SlotID == strings.TrimPrefix(reply.Choice, "slot:") {
					return ReplyIntent{Intent: "choose", Choice: i + 1}
				}
			}
		case reply.Choice == "ask-pro":
			return ReplyIntent{Intent: "ask_pro"}
		case reply.Choice == "yes" || reply.Choice == "no":
			return ReplyIntent{Intent: reply.Choice}
		}
	}
	text := strings.TrimSpace(reply.Text)
	switch replyStage(d, reply) {
	case "ask":
		lower := strings.ToLower(text)
		windows, hasWindows := ParseAvailability(text, c.Now, c.zone())
		cancel, move := cancelWords.MatchString(lower) && !notCancel.MatchString(lower), moveWords.MatchString(lower)
		switch {
		case cancel && !(move && hasWindows):
			return ReplyIntent{Intent: "cancel"}
		case move && hasWindows:
			return ReplyIntent{Intent: "reschedule", Windows: windows}
		case move:
			return ReplyIntent{Intent: "reschedule"}
		}
	case "move":
		lower := strings.ToLower(text)
		switch windows, ok := ParseAvailability(text, c.Now, c.zone()); {
		case cancelWords.MatchString(lower) && !notCancel.MatchString(lower):
			return ReplyIntent{Intent: "cancel"}
		case ok:
			return ReplyIntent{Intent: "availability", Windows: windows}
		case replyIs(reply, "no", noWords):
			return ReplyIntent{Intent: "no"}
		}
	case "address":
		if len(text) >= 8 && strings.ContainsAny(text, "0123456789") {
			return ReplyIntent{Intent: "address", Address: text}
		}
	case "confirm":
		switch {
		case replyIs(reply, "yes", yesWords):
			return ReplyIntent{Intent: "yes"}
		case replyIs(reply, "no", noWords):
			// "No, I need a Saturday" declines and says when they are free.
			if windows, ok := ParseAvailability(text, c.Now, c.zone()); ok && len(strings.Fields(text)) > 2 {
				return ReplyIntent{Intent: "availability", Windows: windows}
			}
			return ReplyIntent{Intent: "no"}
		}
	case "availability", "offer":
		if d.Stage == "offer" {
			switch {
			case replyIs(reply, "yes", yesWords):
				return ReplyIntent{Intent: "yes"}
			case replyIs(reply, "no", noWords):
				return ReplyIntent{Intent: "no"}
			}
			if n, err := strconv.Atoi(strings.Trim(text, ". ")); err == nil && n >= 1 && n <= len(d.Offered) {
				return ReplyIntent{Intent: "choose", Choice: n}
			}
		}
		if windows, ok := ParseAvailability(text, c.Now, c.zone()); ok {
			return ReplyIntent{Intent: "availability", Windows: windows}
		}
	}
	return ReplyIntent{Intent: "unclear"}
}

// intent is what the latest reply means: a model's reading when one was made for this reply and it
// understood, otherwise the built-in reading.
func (c AgentContext) intent(d AgentDialog, reply *Message) ReplyIntent {
	if c.Interpreted != nil && reply.Seq != 0 && c.InterpretedSeq == reply.Seq && c.Interpreted.Intent != "unclear" {
		return *c.Interpreted
	}
	return c.localIntent(d, reply)
}

// PendingReply returns the query (with the offered count and the reply's Seq) to put to a language model when the person has answered with
// free text the built-in reading cannot settle: typed text (not a button), and not plainly yes/no. Address
// replies never leave the server.
func PendingReply(c AgentContext) (ReplyQuery, int, int, bool) {
	if !c.Mandate.Enabled || c.State.Intake == nil || c.State.Job.ID == "" {
		return ReplyQuery{}, 0, 0, false
	}
	d := c.dialog()
	reply := c.latestReply(d)
	// The service address is only ever read locally: it is shared with the chosen professional and nobody else.
	if reply == nil || reply.Choice != "" || reply.Seq == 0 || d.Stage == "address" {
		return ReplyQuery{}, 0, 0, false
	}
	stage := replyStage(d, reply)
	if stageIntents[stage] == nil {
		return ReplyQuery{}, 0, 0, false
	}
	if local := c.localIntent(d, reply); local.Intent == "yes" || local.Intent == "no" || local.Intent == "choose" || local.Intent == "cancel" || local.Intent == "reschedule" && len(local.Windows) > 0 {
		return ReplyQuery{}, 0, 0, false
	}
	question := ""
	messages := c.State.Intake.Messages
	for i := len(messages) - 1; i >= 0 && stage != "ask"; i-- {
		if messages[i].Role == "agent" && messages[i].Seq == d.Asked {
			question = messages[i].Text
			break
		}
	}
	var options []string
	for _, slot := range d.Offered {
		options = append(options, c.visitWhen(slot))
	}
	locale := "en"
	if dialogLocale(c) == 1 {
		locale = "nl"
	}
	return ReplyQuery{Stage: stage, Question: question, Options: options, Reply: reply.Text, Locale: locale, Now: c.Now.In(c.zone()).Format(time.RFC3339), Timezone: c.Mandate.Timezone}, len(d.Offered), reply.Seq, true
}
