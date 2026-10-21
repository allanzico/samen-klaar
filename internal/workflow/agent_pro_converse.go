package workflow

// The professional's assistant talks to its professional the same way: when something falls outside
// their settings it asks in the chat, acts on the answer, and reports what it agreed.

func init() {
	for key, text := range map[string][2]string{
		"pro_confirm_visit":         {"{customer} reserved {when}. Shall I confirm it?", "{customer} heeft {when} gereserveerd. Zal ik dat bevestigen?"},
		"pro_confirm_visit_outside": {"{customer} reserved {when}, which is outside your usual hours. Shall I confirm it anyway?", "{customer} heeft {when} gereserveerd, dat valt buiten je gebruikelijke uren. Zal ik het toch bevestigen?"},
		"pro_confirm_price":         {"{customer} offers {amount}, below your lowest price of {floor}. Shall I accept it?", "{customer} biedt {amount}, onder je laagste prijs van {floor}. Zal ik dat accepteren?"},
		"pro_confirm_date":          {"{customer} proposes {when}. Shall I accept it?", "{customer} stelt {when} voor. Zal ik dat accepteren?"},
		"pro_confirm_date_outside":  {"{customer} proposes {when}, outside your usual hours, and I couldn't find a closer time. Shall I accept it anyway?", "{customer} stelt {when} voor, buiten je gebruikelijke uren, en ik kon geen dichterbij gelegen tijd vinden. Zal ik het toch accepteren?"},
		"pro_confirmed":             {"Confirmed {when} with {customer}.", "{when} bevestigd met {customer}."},
	} {
		dialogText[key] = text
	}
}

func (c AgentContext) customerName() string {
	if name := firstName(c.State.Job.Customer); name != "" {
		return name
	}
	return map[int]string{0: "The customer", 1: "De klant"}[dialogLocale(c)]
}

// proMilestone reports a confirmed visit to the professional once, whoever confirmed it.
func (c AgentContext) proMilestone() (AgentDecision, bool) {
	job := c.State.Job
	if job.Appointment == nil || job.AppointmentConfirmation == nil || job.AppointmentConfirmation.Status != "confirmed" || c.State.Intake == nil {
		return AgentDecision{}, false
	}
	key := "confirmed:" + job.Appointment.StartUTC
	if d := c.dialog(); d.reported(key) {
		return AgentDecision{}, false
	}
	params := map[string]string{"customer": c.customerName(), "when": c.visitWhen(*job.Appointment)}
	return c.say("tell-confirmed", false, c.text("pro_confirmed", params), nil, func(d *AgentDialog) { d.Stage, d.Pending = "idle", ""; d.report(key) }), true
}

// proConfirm asks the professional a yes/no question about one thing and acts on the answer.
func (c AgentContext) proConfirm(kind, id, key string, params map[string]string) AgentDecision {
	d := c.dialog()
	if d.Stage == "confirm" && d.Pending == kind && d.PendingID == id {
		reply := c.latestReply(d)
		if reply == nil {
			return c.waiting()
		}
		switch in := c.intent(d, reply); {
		case in.Intent == "yes":
			switch kind {
			case "confirm-appointment":
				return act("confirm-visit", Action{Action: "confirm-appointment"}, params)
			case "accept-price":
				return act("accept-price", Action{Action: "accept-price", ProposalID: id}, amountParams(params, c.State.Job.PriceProposal.AmountMinor))
			default:
				return act("accept-visit", Action{Action: "accept-date", ProposalID: id}, params)
			}
		case in.Intent == "no":
			switch kind {
			case "confirm-appointment":
				return act("decline-visit", Action{Action: "decline-appointment"}, params)
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
	return c.say("ask-confirm", true, c.text(key, params), c.yesNo("yes", "no"), func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID, d.Attempts = "confirm", kind, id, 0
	})
}
