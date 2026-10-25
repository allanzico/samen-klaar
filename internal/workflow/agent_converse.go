package workflow

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// The customer's assistant is a conversation, not a form. It asks when the customer is free, compares
// that with the times the professional has open, offers what fits, and books only after the customer
// says yes (unless they chose "book without asking"). All text is written in the customer's language.

var dialogText = map[string][2]string{
	"booking_autonomy":   {"For this booking, may I check the professional’s completion photos and note, approve documented work and release the agreed {amount} from your PayPal hold? I’ll ask you if anything is unclear. Photos cannot confirm workmanship or safety. You can turn this off before payment release starts.", "Mag ik voor deze boeking de opleverfoto’s en notitie van de vakpersoon controleren, gedocumenteerd werk goedkeuren en het afgesproken bedrag van {amount} vrijgeven uit je PayPal-reservering? Bij onduidelijkheid vraag ik het jou. Foto’s bevestigen geen vakmanschap of veiligheid. Je kunt dit uitzetten voordat de betaling wordt vrijgegeven."},
	"autonomy_automatic": {"Yes, review and release for me", "Ja, controleer en betaal voor mij"},
	"autonomy_manual":    {"No, I’ll review and pay myself", "Nee, ik controleer en betaal zelf"},
	"ask_address":        {"What is the full address where {pro} should come? (street, number, postcode, city)", "Wat is het volledige adres waar {pro} naartoe moet komen? (straat, nummer, postcode, plaats)"},
	"address_choose":     {"Choose your full visit address from the suggestions below. I’ll continue once it’s confirmed.", "Kies je volledige bezoekadres uit de suggesties hieronder. Daarna ga ik verder."},
	"address_retry":      {"I need the street, house number and city so {pro} can find you. Could you type the full address?", "Ik heb straat, huisnummer en plaats nodig zodat {pro} je kan vinden. Kun je het volledige adres intypen?"},
	"confirm_price":      {"{pro} charges {amount} for this job. You didn't give me a budget, so I'm checking: shall I go ahead at that price?", "{pro} rekent {amount} voor deze klus. Je noemde geen budget, dus ik controleer het even: zal ik hiermee akkoord gaan?"},
	"confirm_counter":    {"{pro} would do it for {amount}. Shall I accept that?", "{pro} wil het doen voor {amount}. Zal ik dat accepteren?"},
	"confirm_visit":      {"{pro} suggests {when}. Does that work for you?", "{pro} stelt {when} voor. Komt dat jou uit?"},
	"confirm_retry":      {"Sorry, I didn't catch that. Please answer yes or no.", "Sorry, dat verstond ik niet. Antwoord alsjeblieft met ja of nee."},
	"price_manual":       {"No problem. The price is yours to approve on this page whenever you're ready.", "Geen probleem. De prijs keur je zelf goed op deze pagina wanneer je er klaar voor bent."},
	"ask_availability":   {"When are you free for {pro} to visit? Tell me in your own words, for example “Tuesday afternoon or Thursday after 5”, and I'll check what {pro} has open.", "Wanneer ben je vrij voor een bezoek van {pro}? Vertel het in je eigen woorden, bijvoorbeeld “dinsdagmiddag of donderdag na 17 uur”, dan kijk ik wat {pro} open heeft."},
	"ask_more":           {"Okay. Which other days or times suit you?", "Oké. Welke andere dagen of tijden passen je?"},
	"availability_retry": {"Sorry, I couldn't make out the days and times. Try something like “Tuesday afternoon”, “weekday evenings” or “21 Oct 10–12”.", "Sorry, ik kon de dagen en tijden niet goed lezen. Probeer bijvoorbeeld “dinsdagmiddag”, “doordeweeks 's avonds” of “21 okt 10–12”."},
	"offer_one":          {"{pro} has {when} open, and that fits what you told me. Does it suit you?", "{pro} heeft {when} open en dat past bij wat je vertelde. Komt dat jou uit?"},
	"offer_many":         {"These times fit what you told me:\n{list}\nWhich one would you like?", "Deze tijden passen bij wat je vertelde:\n{list}\nWelke wil je?"},
	"offer_none":         {"None of {pro}'s open times fit. The next ones are:\n{list}\nWould one of those work, or shall I ask {pro} for a time that suits you?", "Geen van de open tijden van {pro} past. De eerstvolgende zijn:\n{list}\nIs een daarvan goed, of zal ik {pro} vragen om een tijd die jou past?"},
	"offer_none_again":   {"None of {pro}'s open times fit, and {pro} couldn't meet your times either. The open times are:\n{list}\nWould one of those work, or tell me other days that suit you?", "Geen van de open tijden van {pro} past en {pro} kon ook niet op jouw tijden. De open tijden zijn:\n{list}\nIs een daarvan goed, of vertel me andere dagen die jou passen?"},
	"offer_nothing":      {"{pro} has no open time left that fits what you told me. You can message {pro} yourself or choose another professional, and I'll keep helping from there.", "{pro} heeft geen open tijd meer die past bij wat je vertelde. Je kunt {pro} zelf een bericht sturen of een andere vakman kiezen, en ik help daarna verder."},
	"other_times":        {"Other times suit me better", "Andere tijden passen me beter"},
	"offer_taken":        {"Sorry, that time was just taken. ", "Sorry, die tijd is net vergeven. "},
	"offer_declined":     {"{pro} couldn't take that time. ", "{pro} kon die tijd niet. "},
	"asked_pro":          {"{pro} hasn't got a matching time open, so I asked for {when}, which fits what you told me. I'll let you know what they say.", "{pro} heeft geen passende tijd open, dus ik heb {when} gevraagd, dat past bij wat je vertelde. Ik laat je weten wat ze zeggen."},
	"booked":             {"I reserved {when} with {pro}. They have two business hours to confirm, and I'll tell you as soon as they do.", "Ik heb {when} gereserveerd bij {pro}. Zij hebben twee werkuren om te bevestigen en ik laat het je weten zodra ze dat doen."},
	"confirmed":          {"{pro} confirmed {when}. You're all set.", "{pro} heeft {when} bevestigd. Alles is geregeld."},
	"work_missing":       {"I can't approve the work yet: {reason}. I've left it with you on this page.", "Ik kan het werk nog niet goedkeuren: {reason}. Ik laat het aan jou over op deze pagina."},
	"blocked":            {"I couldn't do that: {reason}. You can take it from here on this page.", "Dat lukte niet: {reason}. Je kunt het vanaf hier zelf doen op deze pagina."},
	"intro":              {"Hi, I'm {bot}, your assistant for this job. ", "Hoi, ik ben {bot}, je assistent voor deze klus. "},
	"yes":                {"Yes", "Ja"}, "no": {"No", "Nee"},
	"yes_go": {"Yes, go ahead", "Ja, ga door"}, "no_myself": {"No, I'll decide myself", "Nee, ik beslis zelf"},
	"yes_book": {"Yes, book it", "Ja, boek het"}, "no_other": {"No, other times", "Nee, andere tijden"},
	"none_of": {"None of these", "Geen van deze"}, "ask_pro": {"Ask {pro} for another time", "Vraag {pro} om een andere tijd"},
	"chip_week": {"This week", "Deze week"}, "chip_next": {"Next week", "Volgende week"},
	"chip_evenings": {"Weekday evenings", "Doordeweeks 's avonds"}, "chip_any": {"Any time", "Altijd"},
}

func dialogLocale(c AgentContext) int {
	locale := c.Locale
	if c.Role != "professional" && c.State.Intake != nil && c.State.Intake.Locale != "" {
		locale = c.State.Intake.Locale
	}
	if locale == "nl" {
		return 1
	}
	return 0
}

func (c AgentContext) text(key string, params map[string]string) string {
	out := dialogText[key][dialogLocale(c)]
	for k, v := range params {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

var (
	enWeekday = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	nlWeekday = []string{"zo", "ma", "di", "wo", "do", "vr", "za"}
	enMonth   = []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	nlMonth   = []string{"", "jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"}
)

// visitWhen writes a visit as people say it: "Tue 21 Oct, 14:00–15:00".
func (c AgentContext) visitWhen(a Appointment) string {
	zone, err := time.LoadLocation(c.Mandate.Timezone)
	start, e1 := time.Parse(time.RFC3339, a.StartUTC)
	end, e2 := time.Parse(time.RFC3339, a.EndUTC)
	if err != nil || e1 != nil || e2 != nil {
		return a.StartUTC
	}
	s, e := start.In(zone), end.In(zone)
	days, months := enWeekday, enMonth
	if dialogLocale(c) == 1 {
		days, months = nlWeekday, nlMonth
	}
	return fmt.Sprintf("%s %d %s, %s–%s", days[s.Weekday()], s.Day(), months[s.Month()], s.Format("15:04"), e.Format("15:04"))
}

func (c AgentContext) money(minor int64) string {
	if code := c.State.Job.Quote.Currency; code != "" && code != "EUR" {
		amount := euros(minor)
		if dialogLocale(c) == 1 {
			amount = strings.Replace(amount, ".", ",", 1)
		}
		return code + " " + amount
	}
	if dialogLocale(c) == 1 {
		return strings.Replace(fmt.Sprintf("€ %.2f", float64(minor)/100), ".", ",", 1)
	}
	return fmt.Sprintf("€%.2f", float64(minor)/100)
}

// firstName is how an assistant names a person: their first name, or the part of an email address
// before the @ when that is all the account has.
func firstName(full string) string {
	full = strings.TrimSpace(full)
	if at := strings.Index(full, "@"); at > 0 {
		return full[:at]
	}
	if fields := strings.Fields(full); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func (c AgentContext) proName() string {
	if name := firstName(c.State.Job.Professional); name != "" {
		return name
	}
	return map[int]string{0: "the professional", 1: "de vakman"}[dialogLocale(c)]
}

func (c AgentContext) dialog() AgentDialog {
	if c.Role == "professional" {
		return c.State.ProDialog.clone()
	}
	return c.State.AgentDialog.clone()
}

// latestReply is the customer's answer to the assistant's last question, if they gave one.
func (c AgentContext) latestReply(d AgentDialog) *Message {
	if c.State.Intake == nil {
		return nil
	}
	messages := c.State.Intake.Messages
	for i := len(messages) - 1; i >= 0 && messages[i].Seq > d.Asked; i-- {
		if messages[i].Role == c.Role && messages[i].Audience == c.Role {
			return &messages[i]
		}
	}
	return nil
}

var (
	yesWords = []string{"yes", "y", "yeah", "yep", "yup", "ok", "okay", "sure", "go ahead", "please do", "that works", "fine", "sounds good", "ja", "jazeker", "prima", "akkoord", "doe maar", "oke", "oké", "goed", "is goed", "dat werkt", "top"}
	noWords  = []string{"no", "nope", "nah", "no thanks", "not really", "nee", "nee bedankt", "niet", "geen", "none"}
)

func replyIs(m *Message, choice string, words []string) bool {
	if m == nil {
		return false
	}
	if m.Choice != "" {
		return m.Choice == choice
	}
	text := strings.Trim(strings.ToLower(strings.TrimSpace(m.Text)), " .!")
	return slices.Contains(words, text) || choice != "" && (strings.HasPrefix(text, choice+" ") || strings.HasPrefix(text, choice+","))
}

// say builds one assistant message to the customer and the dialog state that goes with it.
func (c AgentContext) say(code string, question bool, text string, choices []Choice, update func(*AgentDialog)) AgentDecision {
	return c.sayLinked(code, question, text, "", choices, update)
}

// sayLinked is say with a link code the page turns into a button to the step the message is about
// ("link:payment").
func (c AgentContext) sayLinked(code string, question bool, text, link string, choices []Choice, update func(*AgentDialog)) AgentDecision {
	d := c.dialog()
	d.Awaiting, d.Choices = question, choices
	if update != nil {
		update(&d)
	}
	// The first thing an assistant says in a request introduces it by its short name.
	introduced := c.State.AgentDialog != nil
	if c.Role == "professional" {
		introduced = c.State.ProDialog != nil
	}
	if !introduced {
		text = c.text("intro", map[string]string{"bot": AssistantName(c.State.Job, c.Role)}) + text
	}
	return act(code, Action{Action: "agent-say", Value: text, Audience: c.Role, Dialog: &d, NoteCode: link}, nil)
}

func (c AgentContext) waiting() AgentDecision { return escalate("awaiting_reply", nil) }

func (c AgentContext) yesNo(yes, no string) []Choice {
	return []Choice{{ID: "yes", Label: c.text(yes, nil)}, {ID: "no", Label: c.text(no, nil)}}
}

// Milestones the assistant reports once each: the visit was reserved, and the professional confirmed.
func (c AgentContext) milestone() (AgentDecision, bool) {
	job := c.State.Job
	if job.Appointment == nil || job.AppointmentConfirmation == nil || c.State.Intake == nil {
		return AgentDecision{}, false
	}
	d := c.dialog()
	key := job.AppointmentConfirmation.Status + ":" + job.Appointment.StartUTC
	params := map[string]string{"pro": c.proName(), "when": c.visitWhen(*job.Appointment)}
	switch job.AppointmentConfirmation.Status {
	case "pending":
		if !d.reported(key) {
			return c.say("tell-booked", false, c.text("booked", params), nil, func(d *AgentDialog) { d.Stage, d.Pending = "idle", ""; d.report(key) }), true
		}
	case "confirmed":
		if !d.reported(key) {
			return c.say("tell-confirmed", false, c.text("confirmed", params), nil, func(d *AgentDialog) { d.Stage, d.Pending = "idle", ""; d.report(key) }), true
		}
	}
	return AgentDecision{}, false
}

// dialogConfirm asks the customer a yes/no question about one thing and acts on the answer.
func (c AgentContext) dialogConfirm(kind, id, key string, params map[string]string) AgentDecision {
	d := c.dialog()
	if d.Stage == "confirm" && d.Pending == kind && d.PendingID == id {
		reply := c.latestReply(d)
		if reply == nil {
			return c.waiting()
		}
		switch in := c.intent(d, reply); {
		case in.Intent == "yes":
			switch kind {
			case "approve-quote":
				return act("approve-plan", Action{Action: "approve-quote", PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, amountParams(params, c.State.Job.Quote.TotalMinor))
			case "accept-price":
				return act("accept-price", Action{Action: "accept-price", ProposalID: id}, amountParams(params, c.State.Job.PriceProposal.AmountMinor))
			default:
				return act("accept-visit", Action{Action: "accept-date", ProposalID: id, PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, params)
			}
		case in.Intent == "no" || in.Intent == "availability" && kind != "approve-quote":
			switch kind {
			case "approve-quote":
				return c.say("tell-manual", false, c.text("price_manual", nil), nil, func(d *AgentDialog) { d.Stage, d.Pending = "manual", "" })
			case "accept-price":
				return act("decline-price", Action{Action: "decline-price", ProposalID: id}, params)
			default:
				return act("decline-visit", Action{Action: "decline-date", ProposalID: id}, params)
			}
		case d.Attempts < 2:
			return c.say("ask-confirm", true, c.text("confirm_retry", nil), c.yesNo("yes", "no"), func(d *AgentDialog) { d.Attempts++ })
		default:
			return escalate("agent_misunderstood", nil)
		}
	}
	return c.say("ask-confirm", true, c.text(key, params), c.yesNo("yes_go", "no_myself"), func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID, d.Attempts = "confirm", kind, id, 0
	})
}

func (c AgentContext) dialogAddress() AgentDecision {
	d := c.dialog()
	params := map[string]string{"pro": c.proName()}
	if d.Stage == "address" {
		reply := c.latestReply(d)
		if reply == nil {
			return c.waiting()
		}
		if in := c.intent(d, reply); in.Intent == "address" {
			return act("set-address", Action{Action: "set-service-address", Value: in.Address}, nil)
		}
		if d.Attempts >= 2 {
			return escalate("address_needed", nil)
		}
		return c.say("ask-address", true, c.text("address_retry", params), nil, func(d *AgentDialog) { d.Attempts++ })
	}
	return c.say("ask-address", true, c.text("ask_address", params), nil, func(d *AgentDialog) { d.Stage, d.Attempts = "address", 0 })
}

func (c AgentContext) askAvailability(key string) AgentDecision {
	params := map[string]string{"pro": c.proName()}
	chips := []Choice{{ID: "chip", Label: c.text("chip_week", nil)}, {ID: "chip", Label: c.text("chip_next", nil)}, {ID: "chip", Label: c.text("chip_evenings", nil)}, {ID: "chip", Label: c.text("chip_any", nil)}}
	return c.say("ask-availability", true, c.text(key, params), chips, func(d *AgentDialog) {
		d.Stage, d.Windows, d.Offered, d.Pending = "availability", nil, nil, ""
		if key == "availability_retry" {
			d.Attempts++
		} else {
			d.Attempts = 0
		}
	})
}

func (c AgentContext) zone() *time.Location {
	zone, err := time.LoadLocation(c.Mandate.Timezone)
	if err != nil {
		return time.UTC
	}
	return zone
}

// openTimes are the professional's free published times that fit the customer's stated windows.
func (c AgentContext) openTimes(d AgentDialog, released string) (fits, others []Appointment) {
	slots := slices.Clone(c.State.AvailableSlots)
	slices.SortFunc(slots, func(a, b Appointment) int { return strings.Compare(a.StartUTC, b.StartUTC) })
	// The customer is asked before anything is booked, so the booking horizon does not hide later times.
	horizon := c.Now.Add(90 * 24 * time.Hour)
	for _, slot := range slots {
		start, err := time.Parse(time.RFC3339, slot.StartUTC)
		if err != nil || !start.After(c.Now.Add(2*time.Hour)) || start.After(horizon) || slot.StartUTC == released {
			continue
		}
		if len(d.Windows) > 0 && WindowsAllow(d.Windows, slot, c.zone()) {
			fits = append(fits, slot)
		} else {
			others = append(others, slot)
		}
	}
	return
}

// spread picks up to three times, preferring different days so the customer sees real choices.
func spread(slots []Appointment, zone *time.Location) []Appointment {
	var picked []Appointment
	seen := map[string]bool{}
	for _, slot := range slots {
		start, _ := time.Parse(time.RFC3339, slot.StartUTC)
		if day := start.In(zone).Format("2006-01-02"); !seen[day] {
			seen[day] = true
			picked = append(picked, slot)
		}
		if len(picked) == 3 {
			return picked
		}
	}
	for _, slot := range slots {
		if len(picked) == 3 {
			break
		}
		if !slices.ContainsFunc(picked, func(p Appointment) bool { return p.SlotID == slot.SlotID }) {
			picked = append(picked, slot)
		}
	}
	return picked
}

func (c AgentContext) slotChoices(slots []Appointment) ([]Choice, string) {
	var choices []Choice
	var list []string
	for i, slot := range slots {
		label := c.visitWhen(slot)
		choices = append(choices, Choice{ID: "slot:" + slot.SlotID, Label: label})
		list = append(list, fmt.Sprintf("%d. %s", i+1, label))
	}
	return choices, strings.Join(list, "\n")
}

// offerTimes compares the customer's windows with the professional's open times and asks about the best fit.
func (c AgentContext) offerTimes(d AgentDialog, released, prefix string) AgentDecision {
	fits, others := c.openTimes(d, released)
	params := map[string]string{"pro": c.proName()}
	windows := d.Windows
	switch {
	case len(fits) == 1:
		params["when"] = c.visitWhen(fits[0])
		offered := fits[:1]
		return c.say("ask-offer", true, prefix+c.text("offer_one", params), c.yesNo("yes_book", "no_other"), func(d *AgentDialog) {
			d.Stage, d.Windows, d.Offered, d.Pending = "offer", windows, offered, ""
		})
	case len(fits) > 1:
		picked := spread(fits, c.zone())
		choices, list := c.slotChoices(picked)
		params["list"] = list
		choices = append(choices, Choice{ID: "no", Label: c.text("none_of", nil)})
		return c.say("ask-offer", true, prefix+c.text("offer_many", params), choices, func(d *AgentDialog) {
			d.Stage, d.Windows, d.Offered, d.Pending = "offer", windows, picked, ""
		})
	case len(others) > 0:
		picked := spread(others, c.zone())
		choices, list := c.slotChoices(picked)
		params["list"] = list
		key := "offer_none"
		if c.DateProposals == 0 {
			choices = append(choices, Choice{ID: "ask-pro", Label: c.text("ask_pro", params)})
		} else {
			// The professional was already asked once; the customer decides what happens next.
			key = "offer_none_again"
			choices = append(choices, Choice{ID: "no", Label: c.text("other_times", nil)})
		}
		return c.say("ask-offer", true, prefix+c.text(key, params), choices, func(d *AgentDialog) {
			d.Stage, d.Windows, d.Offered, d.Pending = "offer", windows, picked, ""
		})
	case c.DateProposals > 0 && !d.reported("nothing"):
		return c.say("tell-nothing", false, prefix+c.text("offer_nothing", params), nil, func(d *AgentDialog) { d.Stage = "idle"; d.report("nothing") })
	}
	return c.proposeFromWindows(d, released)
}

// proposeFromWindows asks the professional for a time inside the customer's windows when none of their
// published times fits (or they have published none).
func (c AgentContext) proposeFromWindows(d AgentDialog, released string) AgentDecision {
	job := c.State.Job
	if c.DateProposals > 0 {
		return escalate("no_time_in_window", nil)
	}
	candidate, ok := c.candidateFromWindows(d.Windows, released)
	if !ok {
		return escalate("no_time_in_window", nil)
	}
	if !job.QuoteApproved {
		return act("approve-plan", Action{Action: "approve-quote", PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, map[string]string{"amount": euros(job.Quote.TotalMinor)})
	}
	return act("propose-visit", proposeDate(candidate, "This time suits the customer."), map[string]string{"start": candidate.StartUTC, "end": candidate.EndUTC})
}

func (c AgentContext) candidateFromWindows(windows []TimeWindow, released string) (Appointment, bool) {
	zone, m := c.zone(), c.Mandate
	length := visitLength(c.State.Job)
	for day := 1; day <= max(m.MaxDaysAhead, 1)+7; day++ {
		d := c.Now.In(zone).AddDate(0, 0, day)
		for _, w := range windows {
			start := time.Date(d.Year(), d.Month(), d.Day(), w.From/60, w.From%60, 0, 0, zone)
			if start.Hour() < 8 && w.From < 8*60 {
				start = time.Date(d.Year(), d.Month(), d.Day(), 8, 0, 0, 0, zone) // never propose before 08:00
			}
			candidate := Appointment{StartUTC: start.UTC().Format(time.RFC3339), EndUTC: start.Add(length).UTC().Format(time.RFC3339), Timezone: m.Timezone}
			if start.Before(c.Now.Add(24*time.Hour)) || candidate.StartUTC == released || !WindowsAllow([]TimeWindow{w}, candidate, zone) {
				continue
			}
			if int(start.Weekday()) == 0 && !slices.Contains(m.Weekdays, 7) {
				continue
			}
			return candidate, true
		}
	}
	return Appointment{}, false
}

// dialogSchedule runs the scheduling conversation for a plan the customer has accepted.
func (c AgentContext) dialogSchedule(released string) AgentDecision {
	d := c.dialog()
	job := c.State.Job
	reply := c.latestReply(d)
	params := map[string]string{"pro": c.proName()}
	if d.Stage == "confirm" && reply != nil {
		// "No, I need a Saturday": a decline that also says when the customer is free.
		if in := c.intent(d, reply); in.Intent == "availability" {
			d.Windows = in.Windows
		}
	}

	switch d.Stage {
	case "availability":
		if reply == nil {
			return c.waiting()
		}
		in := c.intent(d, reply)
		if in.Intent != "availability" {
			if d.Attempts >= 2 {
				return escalate("agent_misunderstood", nil)
			}
			return c.askAvailability("availability_retry")
		}
		d.Windows = in.Windows
		return c.offerTimes(d, released, "")
	case "offer":
		if reply == nil {
			return c.waiting()
		}
		pick := func(slot Appointment) AgentDecision {
			if !slices.ContainsFunc(c.State.AvailableSlots, func(s Appointment) bool { return s.SlotID == slot.SlotID }) {
				return c.offerTimes(d, released, c.text("offer_taken", nil))
			}
			action := "approve-and-book"
			if job.QuoteApproved {
				action = "book-appointment"
			}
			return act("book-visit", Action{Action: action, Value: slot.SlotID, PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, map[string]string{"start": slot.StartUTC, "end": slot.EndUTC, "amount": euros(job.Quote.TotalMinor)})
		}
		switch in := c.intent(d, reply); in.Intent {
		case "choose":
			if in.Choice >= 1 && in.Choice <= len(d.Offered) {
				return pick(d.Offered[in.Choice-1])
			}
		case "ask_pro":
			return c.proposeFromWindows(d, released)
		case "yes":
			if len(d.Offered) == 1 {
				return pick(d.Offered[0])
			}
		case "no":
			return c.askAvailability("ask_more")
		case "availability":
			d.Windows = in.Windows
			return c.offerTimes(d, released, "")
		}
		if d.Attempts >= 2 {
			return escalate("agent_misunderstood", nil)
		}
		return c.askAvailability("availability_retry")
	}
	if len(d.Windows) == 0 {
		return c.askAvailability("ask_availability")
	}
	// The customer's availability is known: offer again, after a decline or a change in the open times.
	prefix := ""
	if released != "" {
		prefix = c.text("offer_declined", params)
	}
	return c.offerTimes(d, released, prefix)
}

// BlockedNotice is what an assistant tells its person when the workflow refused a step it tried, so a
// reply never hangs: the plain reason, and that the person can carry on from the page. The dialog is
// left idle, except a refused work approval, which stays with the person.
func BlockedNotice(c AgentContext, code, reason string) Action {
	d := c.dialog()
	if code == "set-address" && c.Role == "customer" {
		d.Stage, d.Pending, d.PendingID = "address", "", ""
		d.Awaiting, d.Choices = true, nil
		text := c.text("address_choose", nil)
		if !strings.HasPrefix(reason, "Choose a full street address") {
			text = strings.TrimRight(strings.TrimSpace(reason), ".") + ". " + text
		}
		return Action{Action: "agent-say", Value: text, Audience: c.Role, Dialog: &d, NoteCode: "address:choose", ByAgent: true}
	}
	reason = strings.TrimRight(strings.TrimSpace(reason), ".")
	d.Awaiting, d.Choices = false, nil
	if code == "approve-work" {
		d.Stage, d.Pending, d.PendingID = "confirm", "work-hold", c.State.Job.EvidenceSubmittedAt
	} else {
		d.Stage, d.Pending = "idle", ""
	}
	return Action{Action: "agent-say", Value: c.text("blocked", map[string]string{"reason": reason}), Audience: c.Role, Dialog: &d, ByAgent: true}
}
