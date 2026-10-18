package workflow

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// After a visit is booked the customer's assistant keeps talking. The customer can ask it, in their own
// words and at any time, to move or cancel the visit; it reads the request, asks the customer to confirm
// and then uses the same actions a button would (propose-date, cancel-request), so the professional's
// assistant and the operator rules apply as usual. When the professional marks the work complete it
// asks whether to approve it, and for payment it only tells the customer where PayPal approval is
// theirs: it never pays. Every outcome is reported once, derived from the request itself.

func init() {
	for key, text := range map[string][2]string{
		"ask_unclear":       {"I can move or cancel the visit with {pro}, or help you approve finished work. To write to {pro}, switch the box below to {pro}; nothing was sent to them.", "Ik kan het bezoek met {pro} verplaatsen of annuleren, of je helpen met het goedkeuren van afgerond werk. Om {pro} te schrijven zet je het vak hieronder op {pro}; er is niets naar hen gestuurd."},
		"ask_move_when":     {"When would you like to move the visit to? It is booked for {current}. Tell me a day or time in your own words, for example “Thursday afternoon”.", "Naar wanneer wil je het bezoek verplaatsen? Het staat nu op {current}. Vertel een dag of tijd in je eigen woorden, bijvoorbeeld “donderdagmiddag”."},
		"move_retry":        {"Sorry, I couldn't make out the day. Try something like “Thursday afternoon” or “21 Oct 10–12”, or tell me to leave the visit as it is.", "Sorry, ik kon de dag niet goed lezen. Probeer bijvoorbeeld “donderdagmiddag” of “21 okt 10–12”, of zeg dat het bezoek zo blijft."},
		"move_none":         {"I couldn't find a time with {pro} on those days. Which other days or times suit you? Or tell me to leave the visit as it is.", "Ik kon op die dagen geen tijd bij {pro} vinden. Welke andere dagen of tijden passen je? Of zeg dat het bezoek zo blijft."},
		"move_offer_open":   {"{pro} has {when} open, which fits. Shall I ask {pro} to move your visit there from {current}?", "{pro} heeft {when} open en dat past. Zal ik {pro} vragen je bezoek daarheen te verplaatsen vanaf {current}?"},
		"move_offer_ask":    {"{pro} has no published time then, but I can ask for {when}. Shall I ask {pro} to move your visit there from {current}?", "{pro} heeft dan geen vrije tijd gepubliceerd, maar ik kan {when} vragen. Zal ik {pro} vragen je bezoek daarheen te verplaatsen vanaf {current}?"},
		"move_late":         {" The visit is less than 24 hours away, so a change this late also needs our operator team's approval after {pro} agrees. It stays as it is until then.", " Het bezoek is over minder dan 24 uur, dus een wijziging zo laat moet ook door ons operatorteam worden goedgekeurd nadat {pro} akkoord is. Tot die tijd blijft het zoals het is."},
		"move_yes":          {"Yes, ask {pro}", "Ja, vraag het aan {pro}"},
		"keep":              {"No, keep it as it is", "Nee, laat het zoals het is"},
		"move_kept":         {"Okay, the visit stays on {current}.", "Oké, het bezoek blijft op {current}."},
		"move_sent":         {"Your request to move the visit to {when} is with {pro}. I'll tell you what they say.", "Je verzoek om het bezoek te verplaatsen naar {when} ligt bij {pro}. Ik laat je weten wat ze zeggen."},
		"move_refused":      {"{pro} couldn't do {when}, so the visit stays on {current}. Tell me another day if you'd like me to ask again.", "{pro} kon niet op {when}, dus het bezoek blijft op {current}. Noem een andere dag als ik het opnieuw moet vragen."},
		"move_review":       {"{pro} agreed to {when}. Because the visit is less than 24 hours away, our operator team has to approve the change. The visit stays on {current} until they decide.", "{pro} is akkoord met {when}. Omdat het bezoek over minder dan 24 uur is, moet ons operatorteam de wijziging goedkeuren. Het bezoek blijft op {current} tot zij beslissen."},
		"move_unconfirmed":  {"{pro} hasn't confirmed the current time yet. Once they do, I can ask to move it. If you'd rather not wait, I can cancel it.", "{pro} heeft de huidige tijd nog niet bevestigd. Zodra dat gebeurd is, kan ik vragen om te verplaatsen. Wil je niet wachten, dan kan ik het annuleren."},
		"move_open":         {"There is already a visit proposal waiting for an answer, so let's settle that first.", "Er ligt al een voorstel voor een bezoektijd te wachten op antwoord, dus laten we dat eerst afhandelen."},
		"visit_started":     {"The work has already started, so I can't move or cancel the visit. If something is wrong, you can report it on this page or write to {pro}: switch the box below to {pro}.", "Het werk is al begonnen, dus ik kan het bezoek niet verplaatsen of annuleren. Als er iets mis is, meld het dan op deze pagina of schrijf {pro}: zet het vak hieronder op {pro}."},
		"visit_in_review":   {"A request about this visit is already with our operator team. I'll leave it alone until they decide.", "Er ligt al een verzoek over dit bezoek bij ons operatorteam. Ik laat het met rust tot zij beslissen."},
		"visit_hold":        {"You chose to have PayPal hold the payment when you booked, so changing or cancelling the visit touches your payment. I'll leave that to you: use the buttons on this page, or write to {pro}.", "Je koos ervoor dat PayPal de betaling bij het boeken vasthoudt, dus een wijziging of annulering raakt je betaling. Dat laat ik aan jou: gebruik de knoppen op deze pagina of schrijf {pro}."},
		"cancel_confirm":    {"Do you want me to cancel the visit with {pro} on {when}? {pro} will be told. Nothing has been paid.", "Wil je dat ik het bezoek met {pro} op {when} annuleer? {pro} krijgt bericht. Er is niets betaald."},
		"cancel_late":       {" The visit is less than 24 hours away, so this goes to our operator team as a request. It stays booked until they decide.", " Het bezoek is over minder dan 24 uur, dus dit gaat als verzoek naar ons operatorteam. Het blijft geboekt tot zij beslissen."},
		"cancel_yes":        {"Yes, cancel it", "Ja, annuleer het"},
		"cancel_kept":       {"Okay, I won't cancel. The visit stays on {current}.", "Oké, ik annuleer niet. Het bezoek blijft op {current}."},
		"cancel_done":       {"Your request with {pro} is cancelled and they know. Nothing was charged.", "Je aanvraag bij {pro} is geannuleerd en zij weten het. Er is niets in rekening gebracht."},
		"cancel_review":     {"I sent your cancellation request to our operator team. The visit with {pro} stays booked until they decide.", "Ik heb je annuleringsverzoek naar ons operatorteam gestuurd. Het bezoek met {pro} blijft geboekt tot zij beslissen."},
		"work_ask":          {"{pro} says the work is done. {summary} Shall I approve it? Approving doesn't pay anything: payment is a separate step that only you can do, in PayPal.", "{pro} meldt dat het werk klaar is. {summary} Zal ik het goedkeuren? Goedkeuren betekent nog niet betalen: betalen is een aparte stap die alleen jij kunt doen, in PayPal."},
		"work_both":         {"They added {files} and left this note: “{note}”.", "Ze voegden {files} toe en lieten deze notitie achter: “{note}”."},
		"work_files":        {"They added {files}.", "Ze voegden {files} toe."},
		"work_note":         {"They left this note: “{note}”.", "Ze lieten deze notitie achter: “{note}”."},
		"work_check":        {" Our check of the files: {check}", " Onze controle van de bestanden: {check}"},
		"file_one":          {"1 file", "1 bestand"},
		"file_many":         {"{n} files", "{n} bestanden"},
		"work_yes":          {"Yes, approve it", "Ja, keur goed"},
		"work_no":           {"Not yet", "Nog niet"},
		"work_hold":         {"Okay, I won't approve it and nothing is paid. When you're ready you can approve it or report a problem on this page, or write to {pro}.", "Oké, ik keur het niet goed en er wordt niets betaald. Wanneer je er klaar voor bent kun je het goedkeuren of een probleem melden op deze pagina, of {pro} schrijven."},
		"payment_notice":    {"The work is approved. Paying is the one step I can't do for you: PayPal needs your own approval. You can finish it in the payment step on this page.", "Het werk is goedgekeurd. Betalen is de ene stap die ik niet voor je kan doen: PayPal heeft je eigen goedkeuring nodig. Die rond je af in de betaalstap op deze pagina."},
		"paid_notice":       {"Your payment of {amount} to {pro} is complete. If you like, leave a review on this page.", "Je betaling van {amount} aan {pro} is voltooid. Laat gerust een beoordeling achter op deze pagina."},
		"pro_work_approved": {"{customer} approved the finished work. Payment is next: they still have to approve it in PayPal, and I'll tell you when it arrives.", "{customer} heeft het afgeronde werk goedgekeurd. Nu volgt de betaling: die moet nog in PayPal worden goedgekeurd en ik laat het je weten zodra die binnen is."},
		"pro_pay_approved":  {"{customer} approved the payment of {amount} in PayPal. It is being completed.", "{customer} heeft de betaling van {amount} in PayPal goedgekeurd. Die wordt afgerond."},
		"pro_paid":          {"{customer}'s payment of {amount} has arrived.", "De betaling van {amount} van {customer} is binnen."},
	} {
		dialogText[key] = text
	}
}

// tellIdle says one thing to the customer and returns the dialog to rest.
func (c AgentContext) tellIdle(code, key string, params map[string]string, milestone string) AgentDecision {
	return c.sayLinked(code, false, c.text(key, params), "", nil, func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID, d.Attempts, d.Windows, d.Offered = "idle", "", "", 0, nil, nil
		if milestone != "" {
			d.report(milestone)
		}
	})
}

func (c AgentContext) changeParams() map[string]string {
	params := map[string]string{"pro": c.proName()}
	if appointment := c.State.Job.Appointment; appointment != nil {
		params["current"] = c.visitWhen(*appointment)
	}
	return params
}

// changeBlocker says why the assistant cannot move or cancel the booked visit, as a text key, or "".
// Work in progress, operator reviews and payment holds stay with the person and the operator team.
func (c AgentContext) changeBlocker(kind string) string {
	state, job := c.State, c.State.Job
	switch {
	case job.WorkStarted || job.EvidenceSubmitted || job.CompletionApproved || job.Paid:
		return "visit_started"
	case state.CancellationReview != nil && state.CancellationReview.Status == "pending":
		return "visit_in_review"
	case job.PaymentAuthorizationTiming == PaymentAuthorizationAtBooking:
		return "visit_hold"
	case kind == "reschedule" && job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status == "pending":
		return "move_unconfirmed"
	case kind == "reschedule" && job.DateProposal != nil && job.DateProposal.Status == "pending":
		return "move_open"
	}
	return ""
}

func (c AgentContext) lateVisit() bool {
	late, err := VisitChangeNeedsReview(c.State.Job.Appointment, c.Now)
	return err != nil || late
}

// visitNotices reports, once each, what happened to a move, a cancellation, the approved work and the
// payment. They are read from the request, so a button press is reported the same way as a chat request.
func (c AgentContext) visitNotices() (AgentDecision, bool) {
	state := c.State
	job := state.Job
	if state.Intake == nil || job.ID == "" {
		return AgentDecision{}, false
	}
	d := c.dialog()
	params := c.changeParams()
	if job.Appointment != nil {
		if p := job.DateProposal; p != nil && p.Actor == "customer" {
			params["when"] = c.visitWhen(p.Appointment)
			moved := "moved:" + p.ID
			switch {
			case p.Status == "pending" && !d.reported(moved):
				return c.tellIdle("tell-moved", "move_sent", params, moved), true
			case p.Status == "declined" && d.reported(moved) && !d.reported("refused:"+p.ID):
				return c.tellIdle("tell-refused", "move_refused", params, "refused:"+p.ID), true
			}
		}
		if cr := state.CancellationReview; cr != nil && cr.Status == "pending" && cr.RequestedBy == "customer" {
			switch {
			case cr.Kind == "" && !d.reported("cancel-review:"+cr.RequestedAt):
				return c.tellIdle("tell-cancel-review", "cancel_review", params, "cancel-review:"+cr.RequestedAt), true
			case cr.Kind == "late-reschedule" && cr.ProposedAppointment != nil && !d.reported("review:"+cr.ProposalID):
				params["when"] = c.visitWhen(*cr.ProposedAppointment)
				return c.tellIdle("tell-move-review", "move_review", params, "review:"+cr.ProposalID), true
			}
		}
	}
	switch {
	case job.Paid:
		if key := "paid:" + job.PaymentTransactionID; !d.reported(key) {
			params["amount"] = c.money(job.Quote.TotalMinor)
			return c.tellIdle("tell-paid", "paid_notice", params, key), true
		}
	case job.CompletionApproved:
		key := "payment:" + job.CompletionApprovedAt
		if !d.reported(key) {
			// The step itself is PayPal's window; the assistant only points to it.
			return c.sayLinked("tell-payment", false, c.text("payment_notice", params), "link:payment", nil, func(d *AgentDialog) {
				d.Stage, d.Pending, d.PendingID, d.Attempts = "idle", "", "", 0
				d.report(key)
			}), true
		}
	}
	return AgentDecision{}, false
}

// cancelNotice is what the assistant says once after the customer cancelled the request.
func (c AgentContext) cancelNotice() AgentDecision {
	if c.State.Intake == nil || c.State.Job.ID == "" || c.dialog().reported("cancelled") {
		return wait()
	}
	return c.tellIdle("tell-cancelled", "cancel_done", c.changeParams(), "cancelled")
}

// visitRequests runs the customer's own requests to their assistant after booking: a move or a
// cancellation, from the first ask to the confirmed action.
func (c AgentContext) visitRequests() (AgentDecision, bool) {
	if c.State.Intake == nil || c.State.Job.Appointment == nil {
		return AgentDecision{}, false
	}
	d := c.dialog()
	switch {
	case d.Stage == "confirm" && (d.Pending == "reschedule" || d.Pending == "cancel"):
		return c.confirmChange(d), true
	case d.Stage == "move":
		return c.moveWhen(d), true
	}
	reply := c.latestReply(d)
	if reply == nil || reply.Code != askCode {
		return AgentDecision{}, false
	}
	switch in := c.intent(d, reply); in.Intent {
	case "reschedule":
		return c.startReschedule(in.Windows), true
	case "cancel":
		return c.startCancel(), true
	}
	return c.tellIdle("tell-ask", "ask_unclear", c.changeParams(), ""), true
}

func (c AgentContext) keepChip() Choice { return Choice{ID: "no", Label: c.text("keep", nil)} }

func (c AgentContext) startReschedule(windows []TimeWindow) AgentDecision {
	if key := c.changeBlocker("reschedule"); key != "" {
		return c.tellIdle("tell-ask", key, c.changeParams(), "")
	}
	if len(windows) == 0 {
		return c.say("ask-move", true, c.text("ask_move_when", c.changeParams()), []Choice{c.keepChip()}, func(d *AgentDialog) {
			d.Stage, d.Pending, d.PendingID, d.Windows, d.Offered, d.Attempts = "move", "", "", nil, nil, 0
		})
	}
	return c.offerMove(windows)
}

// offerMove finds a time on the days the customer named (one the professional has open, else one to
// ask for) and asks the customer whether to request it.
func (c AgentContext) offerMove(windows []TimeWindow) AgentDecision {
	job := c.State.Job
	current := *job.Appointment
	params := c.changeParams()
	var target Appointment
	open := false
	if fits, _ := c.openTimes(AgentDialog{Windows: windows}, current.StartUTC); len(fits) > 0 {
		target, open = fits[0], true
	} else if candidate, ok := c.candidateFromWindows(windows, current.StartUTC); ok {
		target = candidate
	} else {
		return c.say("ask-move", true, c.text("move_none", params), []Choice{c.keepChip()}, func(d *AgentDialog) {
			d.Stage, d.Pending, d.PendingID, d.Windows, d.Offered = "move", "", "", nil, nil
			d.Attempts++
		})
	}
	// The visit keeps its length, even when the free time is longer.
	start, _ := time.Parse(time.RFC3339, target.StartUTC)
	zone := target.Timezone
	if zone == "" {
		zone = c.Mandate.Timezone
	}
	target = Appointment{StartUTC: target.StartUTC, EndUTC: start.Add(visitLength(job)).UTC().Format(time.RFC3339), Timezone: zone}
	params["when"] = c.visitWhen(target)
	key := "move_offer_ask"
	if open {
		key = "move_offer_open"
	}
	text := c.text(key, params)
	if c.lateVisit() {
		text += c.text("move_late", params)
	}
	choices := []Choice{{ID: "yes", Label: c.text("move_yes", params)}, c.keepChip()}
	return c.say("ask-move", true, text, choices, func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID, d.Windows, d.Offered, d.Attempts = "confirm", "reschedule", target.StartUTC, windows, []Appointment{target}, 0
	})
}

// moveWhen reads the answer to "when would you like to move it to?".
func (c AgentContext) moveWhen(d AgentDialog) AgentDecision {
	reply := c.latestReply(d)
	if reply == nil {
		return c.waiting()
	}
	switch in := c.intent(d, reply); in.Intent {
	case "availability":
		return c.offerMove(in.Windows)
	case "cancel":
		return c.startCancel()
	case "no":
		return c.tellIdle("tell-kept", "move_kept", c.changeParams(), "")
	}
	if d.Attempts >= 2 {
		return escalate("agent_misunderstood", nil)
	}
	return c.say("ask-move", true, c.text("move_retry", nil), []Choice{c.keepChip()}, func(d *AgentDialog) { d.Attempts++ })
}

func (c AgentContext) startCancel() AgentDecision {
	if key := c.changeBlocker("cancel"); key != "" {
		return c.tellIdle("tell-ask", key, c.changeParams(), "")
	}
	params := c.changeParams()
	params["when"] = params["current"]
	text := c.text("cancel_confirm", params)
	if c.lateVisit() {
		text += c.text("cancel_late", params)
	}
	choices := []Choice{{ID: "yes", Label: c.text("cancel_yes", nil)}, c.keepChip()}
	return c.say("ask-cancel", true, text, choices, func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID, d.Windows, d.Offered, d.Attempts = "confirm", "cancel", c.State.Job.Appointment.StartUTC, nil, nil, 0
	})
}

// confirmChange acts on the customer's answer to "shall I ask for that time?" or "shall I cancel?".
func (c AgentContext) confirmChange(d AgentDialog) AgentDecision {
	reply := c.latestReply(d)
	if reply == nil {
		return c.waiting()
	}
	kind := d.Pending
	params := c.changeParams()
	switch in := c.intent(d, reply); {
	case in.Intent == "yes":
		// The request may have changed since it was asked.
		if key := c.changeBlocker(kind); key != "" {
			return c.tellIdle("tell-ask", key, params, "")
		}
		if kind == "cancel" {
			return act("cancel-visit", Action{Action: "cancel-request", Value: "Cancelled by the customer through their assistant."}, map[string]string{"start": c.State.Job.Appointment.StartUTC})
		}
		if len(d.Offered) == 0 {
			return c.startReschedule(d.Windows)
		}
		target := d.Offered[0]
		return act("reschedule-visit", proposeDate(target, "The customer asked to move the visit."), map[string]string{"start": target.StartUTC, "end": target.EndUTC, "from": c.State.Job.Appointment.StartUTC})
	case in.Intent == "no":
		if kind == "cancel" {
			return c.tellIdle("tell-kept", "cancel_kept", params, "")
		}
		return c.tellIdle("tell-kept", "move_kept", params, "")
	case in.Intent == "availability":
		// "No, Friday instead": new days for the same request.
		return c.offerMove(in.Windows)
	case d.Attempts < 2:
		return c.say("ask-confirm", true, c.text("confirm_retry", nil), c.yesNo("yes", "no"), func(d *AgentDialog) { d.Attempts++ })
	}
	return escalate("agent_misunderstood", nil)
}

// workApprovable is true when the workflow would accept an approval of the finished work.
func workApprovable(job Job) bool {
	if !job.EvidenceSubmitted || !job.DetailAdded || job.CompletionApproved || job.Dispute != nil && job.Dispute.Status == "open" {
		return false
	}
	return ValidateCompletionEvidence(job, true) == nil
}

func (c AgentContext) files(n int) string {
	if n == 1 {
		return c.text("file_one", nil)
	}
	return c.text("file_many", map[string]string{"n": fmt.Sprint(n)})
}

func clip(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

// workSummary tells the customer what the professional handed in: the files, the note and, when one was
// made, the file check.
func (c AgentContext) workSummary() string {
	job := c.State.Job
	files, note := 0, ""
	for _, e := range job.Evidence {
		if !CurrentEvidence(job, e) {
			continue
		}
		if e.AssetID != "" {
			files++
		} else if e.Kind == "note" && strings.TrimSpace(e.Description) != "" {
			note = e.Description
		}
	}
	params := map[string]string{"files": c.files(files), "note": clip(note, 160)}
	var summary string
	switch {
	case files > 0 && note != "":
		summary = c.text("work_both", params)
	case files > 0:
		summary = c.text("work_files", params)
	case note != "":
		summary = c.text("work_note", params)
	}
	if review := job.EvidenceReview; review != nil && strings.TrimSpace(review.Summary) != "" {
		summary += c.text("work_check", map[string]string{"check": clip(review.Summary, 240)})
	}
	return strings.TrimSpace(summary)
}

// dialogWork asks the customer whether to approve the finished work and acts on the answer. A no leaves
// it with the person; payment is never part of this question.
func (c AgentContext) dialogWork() AgentDecision {
	d := c.dialog()
	id := c.State.Job.EvidenceSubmittedAt
	params := map[string]string{"pro": c.proName()}
	if d.Stage == "confirm" && d.PendingID == id && (d.Pending == "approve-work" || d.Pending == "work-hold") {
		if d.Pending == "work-hold" {
			return escalate("check_the_work", nil)
		}
		reply := c.latestReply(d)
		if reply == nil {
			return c.waiting()
		}
		switch in := c.intent(d, reply); in.Intent {
		case "yes":
			// Say exactly what is missing instead of trying, failing and leaving the customer waiting.
			if err := ValidateCompletionEvidence(c.State.Job, true); err != nil && !workApprovable(c.State.Job) {
				return c.say("tell-work-hold", false, c.text("work_missing", map[string]string{"reason": err.Error()}), nil, func(d *AgentDialog) { d.Stage, d.Pending, d.PendingID = "confirm", "work-hold", id })
			}
			return act("approve-work", Action{Action: "approve-completion"}, nil)
		case "no":
			return c.say("tell-work-hold", false, c.text("work_hold", params), nil, func(d *AgentDialog) { d.Stage, d.Pending, d.PendingID = "confirm", "work-hold", id })
		}
		if d.Attempts >= 2 {
			return escalate("agent_misunderstood", nil)
		}
		return c.say("ask-confirm", true, c.text("confirm_retry", nil), c.workChoices(), func(d *AgentDialog) { d.Attempts++ })
	}
	params["summary"] = c.workSummary()
	return c.say("ask-work", true, c.text("work_ask", params), c.workChoices(), func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID, d.Attempts = "confirm", "approve-work", id, 0
	})
}

func (c AgentContext) workChoices() []Choice {
	return []Choice{{ID: "yes", Label: c.text("work_yes", nil)}, {ID: "no", Label: c.text("work_no", nil)}}
}

// proPaymentMilestones is the professional's assistant reporting, once each and in order, that the
// customer approved the work, approved the payment in PayPal and that the payment arrived.
func (c AgentContext) proPaymentMilestones() (AgentDecision, bool) {
	job := c.State.Job
	if c.State.Intake == nil || !job.CompletionApproved {
		return AgentDecision{}, false
	}
	d := c.dialog()
	params := map[string]string{"customer": c.customerName(), "amount": c.money(job.Quote.TotalMinor)}
	report := func(code, text, key string) (AgentDecision, bool) {
		return c.say(code, false, c.text(text, params), nil, func(d *AgentDialog) { d.Stage, d.Pending = "idle", ""; d.report(key) }), true
	}
	if key := "approved:" + job.CompletionApprovedAt; !d.reported(key) {
		return report("tell-work-approved", "pro_work_approved", key)
	}
	if key := "payapproved:" + job.PaymentApprovedAt; job.PaymentApprovedAt != "" && job.PaymentApprovedVersion == job.ScopeVersion && !d.reported(key) {
		return report("tell-payment-approved", "pro_pay_approved", key)
	}
	if key := "paid:" + job.PaymentTransactionID; job.Paid && !d.reported(key) {
		return report("tell-paid", "pro_paid", key)
	}
	return AgentDecision{}, false
}

// workBlocked tells the customer, once, exactly what documentation is still missing when the
// professional handed in work that cannot be approved yet, then leaves it with the person.
func (c AgentContext) workBlocked() AgentDecision {
	job := c.State.Job
	d := c.dialog()
	key := "work-missing:" + job.EvidenceSubmittedAt
	err := ValidateCompletionEvidence(job, true)
	if err == nil || d.reported(key) {
		return escalate("check_the_work", nil)
	}
	return c.say("tell-work-hold", false, c.text("work_missing", map[string]string{"reason": err.Error()}), nil, func(d *AgentDialog) {
		d.Stage, d.Pending, d.PendingID = "confirm", "work-hold", job.EvidenceSubmittedAt
		d.report(key)
	})
}
