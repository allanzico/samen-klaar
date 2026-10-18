package workflow

import (
	"strings"
	"testing"
	"time"
)

// The tests below run on the real clock because the workflow itself validates times against it.

var followupZone = func() *time.Location {
	zone, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		panic(err)
	}
	return zone
}()

func followupAt(base time.Time, hour int) time.Time {
	return time.Date(base.Year(), base.Month(), base.Day(), hour, 0, 0, 0, followupZone)
}

func followupVisit(start time.Time) Appointment {
	return Appointment{SlotID: "booked", StartUTC: start.UTC().Format(time.RFC3339), EndUTC: start.Add(time.Hour).UTC().Format(time.RFC3339), Timezone: "Europe/Amsterdam"}
}

func followupLabel(a Appointment) string {
	return AgentContext{Role: "customer", Locale: "en", Mandate: DefaultMandate("customer")}.visitWhen(a)
}

// bookedFollowup is a confirmed visit eight days out, with the professional's Thursday afternoon open.
func bookedFollowup(now time.Time) (State, Appointment, Appointment) {
	visit := followupVisit(followupAt(now.In(followupZone).AddDate(0, 0, 8), 9))
	thursday := now.In(followupZone).AddDate(0, 0, 2)
	for thursday.Weekday() != time.Thursday {
		thursday = thursday.AddDate(0, 0, 1)
	}
	open := followupVisit(followupAt(thursday, 14))
	open.SlotID = "thu"
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID, state.Job.ServiceAddress = "Allan", "Marieke de Vries", "prof_1", "Damrak 1, Amsterdam"
	state.Job.Quote.DurationMinutes = 60
	state.Job.QuoteApproved = true
	state.Job.PaymentAuthorizationTiming = PaymentAuthorizationAfterCompletion
	state.Job.Appointment = &visit
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "confirmed", Appointment: visit}
	state.Intake = &Intake{Locale: "en", Budget: "EUR 150", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "My sink leaks"}}}
	state.AvailableSlots = []Appointment{open}
	return state, visit, open
}

// ask is what the page sends when the customer types to their assistant with no question open.
func (c *converse) ask(text string) {
	c.t.Helper()
	c.reply(text, "")
}

func newFollowup(t *testing.T) (*converse, Appointment, Appointment) {
	now := time.Now()
	state, visit, open := bookedFollowup(now)
	c := &converse{t: t, state: state, m: DefaultMandate("customer"), at: now}
	if d := c.settle(); d.Kind != AgentWait && d.Code != "awaiting_reply" {
		t.Fatalf("expected the confirmation to be reported first, got %+v", d)
	}
	if !contains(c.lastAssistant().Text, "Marieke confirmed") {
		t.Fatalf("expected the confirmation notice, got %q", c.lastAssistant().Text)
	}
	return c, visit, open
}

func TestCustomerAsksTheAssistantToMoveTheVisit(t *testing.T) {
	c, visit, open := newFollowup(t)
	c.ask("Can we do Thursday instead?")
	if d := c.settle(); d.Code != "awaiting_reply" {
		t.Fatalf("expected a question to confirm, got %+v", d)
	}
	question := c.lastAssistant()
	if !contains(question.Text, followupLabel(open)) || !contains(question.Text, followupLabel(visit)) || !contains(question.Text, "Shall I ask Marieke") || question.Audience != "customer" {
		t.Fatalf("unexpected question: %+v", question)
	}
	if len(c.state.AgentDialog.Choices) != 2 || c.state.Job.DateProposal != nil {
		t.Fatalf("nothing may be asked of the professional before the customer says yes: %+v", c.state.AgentDialog)
	}
	c.reply("Yes, ask Marieke", "yes")
	c.settle()
	proposal := c.state.Job.DateProposal
	if proposal == nil || proposal.Status != "pending" || proposal.Actor != "customer" || proposal.Appointment.StartUTC != open.StartUTC {
		t.Fatalf("expected a pending proposal for the open time: %+v", proposal)
	}
	if c.state.Job.Appointment.StartUTC != visit.StartUTC {
		t.Fatal("the visit moved before the professional agreed")
	}
	if !contains(c.lastAssistant().Text, "is with Marieke") || !contains(c.lastAssistant().Text, followupLabel(open)) {
		t.Fatalf("expected the request to be reported, got %q", c.lastAssistant().Text)
	}
	before := len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("the assistant repeated itself")
	}
	// The professional's agent accepts: the customer hears that the new time is confirmed.
	if err := Apply(&c.state, Action{Action: "accept-date", Actor: "professional", ProposalID: proposal.ID}); err != nil {
		t.Fatal(err)
	}
	c.settle()
	if !contains(c.lastAssistant().Text, "Marieke confirmed "+followupLabel(open)) {
		t.Fatalf("expected the new time to be confirmed, got %q", c.lastAssistant().Text)
	}
}

func TestAMoveTheProfessionalDeclinesLeavesTheVisitAlone(t *testing.T) {
	c, visit, open := newFollowup(t)
	c.ask("could we reschedule to Thursday")
	c.settle()
	c.reply("yes", "yes")
	c.settle()
	if err := Apply(&c.state, Action{Action: "decline-date", Actor: "professional", ProposalID: c.state.Job.DateProposal.ID}); err != nil {
		t.Fatal(err)
	}
	c.settle()
	text := c.lastAssistant().Text
	if !contains(text, "couldn't do "+followupLabel(open)) || !contains(text, "stays on "+followupLabel(visit)) || c.state.Job.Appointment.StartUTC != visit.StartUTC {
		t.Fatalf("expected the visit to stay, got %q", text)
	}
}

func TestMovingWithoutADayAsksWhenAndProposesInsideTheWindow(t *testing.T) {
	c, visit, _ := newFollowup(t)
	c.state.AvailableSlots = nil
	c.ask("I need to move the visit")
	if d := c.settle(); d.Code != "awaiting_reply" || !contains(c.lastAssistant().Text, "When would you like to move the visit to?") || !contains(c.lastAssistant().Text, followupLabel(visit)) {
		t.Fatalf("expected to be asked for a day, got %+v %q", d, c.lastAssistant().Text)
	}
	c.reply("something vague", "")
	c.settle()
	if !contains(c.lastAssistant().Text, "couldn't make out the day") {
		t.Fatalf("expected a retry, got %q", c.lastAssistant().Text)
	}
	c.reply("Friday morning", "")
	c.settle()
	if offer := c.lastAssistant().Text; !contains(offer, "has no published time then") || !contains(offer, "Fri") || !contains(offer, "08:00") {
		t.Fatalf("expected an offer to ask for a Friday morning time, got %q", offer)
	}
	// Changing their mind in the answer replaces the days.
	c.reply("No, Saturday afternoon", "")
	c.settle()
	if offer := c.lastAssistant().Text; !contains(offer, "Sat") {
		t.Fatalf("expected the offer to follow the new days, got %q", offer)
	}
	c.reply("Yes", "yes")
	c.settle()
	if p := c.state.Job.DateProposal; p == nil || p.Status != "pending" || p.Actor != "customer" {
		t.Fatalf("expected a pending proposal, got %+v", p)
	}
}

func TestLeavingTheVisitAsItIsNeedsNoAction(t *testing.T) {
	c, visit, _ := newFollowup(t)
	c.ask("Can we do Thursday instead?")
	c.settle()
	c.reply("No, keep it as it is", "no")
	c.settle()
	if c.state.Job.DateProposal != nil || c.state.Job.Appointment.StartUTC != visit.StartUTC || !contains(c.lastAssistant().Text, "stays on "+followupLabel(visit)) {
		t.Fatalf("expected nothing to change, got %q", c.lastAssistant().Text)
	}
}

func TestCustomerCancelsTheVisitThroughTheAssistant(t *testing.T) {
	c, visit, _ := newFollowup(t)
	c.ask("Please cancel the visit")
	if d := c.settle(); d.Code != "awaiting_reply" || !contains(c.lastAssistant().Text, "cancel the visit with Marieke on "+followupLabel(visit)) || contains(c.lastAssistant().Text, "operator") {
		t.Fatalf("expected a question to confirm, got %+v %q", d, c.lastAssistant().Text)
	}
	if c.state.Cancelled {
		t.Fatal("cancelled before the customer confirmed")
	}
	c.reply("Yes, cancel it", "yes")
	c.settle()
	if !c.state.Cancelled || c.state.CancelledBy != "customer" {
		t.Fatalf("expected the request to be cancelled by the customer: %+v", c.state.Cancelled)
	}
	if text := c.lastAssistant().Text; !contains(text, "is cancelled") || !contains(text, "Marieke") {
		t.Fatalf("expected the cancellation to be reported, got %q", text)
	}
	before := len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("the assistant repeated itself")
	}
}

func TestACancellationTheCustomerDeclinesChangesNothing(t *testing.T) {
	c, visit, _ := newFollowup(t)
	c.ask("annuleer het bezoek")
	c.settle()
	c.reply("No", "no")
	c.settle()
	if c.state.Cancelled || !contains(c.lastAssistant().Text, "won't cancel") || c.state.Job.Appointment.StartUTC != visit.StartUTC {
		t.Fatalf("expected the visit to stay, got %q", c.lastAssistant().Text)
	}
}

func TestLateChangesSayTheOperatorDecides(t *testing.T) {
	now := time.Now()
	soon := followupVisit(now.Add(5 * time.Hour).Truncate(time.Minute))
	state, _, open := bookedFollowup(now)
	state.Job.Appointment = &soon
	state.Job.AppointmentConfirmation.Appointment = soon
	c := &converse{t: t, state: state, m: DefaultMandate("customer"), at: now}
	c.settle()
	// A move: the customer is told before agreeing that the operator team has to approve it.
	c.ask("Can we do Thursday instead?")
	c.settle()
	if !contains(c.lastAssistant().Text, followupLabel(open)) || !contains(c.lastAssistant().Text, "less than 24 hours") || !contains(c.lastAssistant().Text, "operator") {
		t.Fatalf("expected the late warning, got %q", c.lastAssistant().Text)
	}
	c.reply("No", "no")
	c.settle()
	// A cancellation becomes a request to the operator team; the visit stays booked.
	c.ask("Please cancel the visit")
	c.settle()
	if !contains(c.lastAssistant().Text, "less than 24 hours") {
		t.Fatalf("expected the late warning, got %q", c.lastAssistant().Text)
	}
	c.reply("Yes", "yes")
	c.settle()
	if c.state.Cancelled || c.state.CancellationReview == nil || c.state.CancellationReview.Status != "pending" {
		t.Fatalf("expected an operator review, not a cancellation: %+v", c.state.CancellationReview)
	}
	if text := c.lastAssistant().Text; !contains(text, "operator team") || !contains(text, "stays booked") {
		t.Fatalf("expected the review to be reported, got %q", text)
	}
	// While the review is open the assistant leaves the visit alone.
	c.ask("Actually, move it to Thursday")
	c.settle()
	if !contains(c.lastAssistant().Text, "already with our operator team") || c.state.Job.DateProposal != nil {
		t.Fatalf("expected the assistant to leave it alone, got %q", c.lastAssistant().Text)
	}
}

func TestPaymentHoldsAndStartedWorkStayWithThePerson(t *testing.T) {
	c, _, _ := newFollowup(t)
	c.state.Job.PaymentAuthorizationTiming = PaymentAuthorizationAtBooking
	c.state.Job.PaymentAuthorizedVersion = c.state.Job.ScopeVersion
	c.ask("Can we do Thursday instead?")
	c.settle()
	if !contains(c.lastAssistant().Text, "touches your payment") || c.state.Job.DateProposal != nil {
		t.Fatalf("a paid hold must stay with the person, got %q", c.lastAssistant().Text)
	}
	c.ask("cancel it")
	c.settle()
	if !contains(c.lastAssistant().Text, "touches your payment") || c.state.Cancelled {
		t.Fatalf("a paid hold must stay with the person, got %q", c.lastAssistant().Text)
	}
	c.state.Job.PaymentAuthorizationTiming = PaymentAuthorizationAfterCompletion
	c.state.Job.WorkStarted = true
	c.ask("cancel it")
	c.settle()
	if !contains(c.lastAssistant().Text, "work has already started") || c.state.Cancelled {
		t.Fatalf("started work cannot be cancelled here, got %q", c.lastAssistant().Text)
	}
}

func TestAnUnrelatedMessageIsNotGuessedAt(t *testing.T) {
	c, _, _ := newFollowup(t)
	c.ask("The key is under the mat")
	c.settle()
	text := c.lastAssistant().Text
	if !contains(text, "switch the box below to Marieke") || !contains(text, "nothing was sent") || c.state.Job.DateProposal != nil || c.state.Cancelled {
		t.Fatalf("expected a pointer to the professional, got %q", text)
	}
	before := len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("the assistant answered the same message twice")
	}
}

func TestAskingBeforeBookingOrFromTheProfessionalIsRefused(t *testing.T) {
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID = "Allan", "Marieke", "prof_1"
	state.Intake = &Intake{Locale: "en", Messages: []Message{{Role: "customer", Text: "leak"}}}
	if err := Apply(&state, Action{Action: "agent-reply", Actor: "customer", Value: "cancel it"}); err == nil {
		t.Fatal("a customer with no booked visit and no open question has nothing to ask yet")
	}
	booked, _, _ := bookedFollowup(time.Now())
	if err := Apply(&booked, Action{Action: "agent-reply", Actor: "professional", Value: "cancel it"}); err == nil {
		t.Fatal("only the customer's assistant takes requests")
	}
	if err := Apply(&booked, Action{Action: "agent-reply", Actor: "customer", Value: "cancel it", ChoiceID: "yes"}); err == nil {
		t.Fatal("a quick reply answers a question; it is not a request")
	}
	if err := Apply(&booked, Action{Action: "agent-reply", Actor: "customer", Value: "cancel it"}); err != nil {
		t.Fatal(err)
	}
	if last := booked.Intake.Messages[len(booked.Intake.Messages)-1]; last.Code != askCode || last.Audience != "customer" || last.Seq == 0 {
		t.Fatalf("expected a private request: %+v", last)
	}
}

func TestMessagesToTheProfessionalAreOrderedWithTheChat(t *testing.T) {
	state, _, _ := bookedFollowup(time.Now())
	if err := Apply(&state, Action{Action: "send-message", Actor: "professional", Value: "I will be 10 minutes late"}); err != nil {
		t.Fatal(err)
	}
	if last := state.Intake.Messages[len(state.Intake.Messages)-1]; last.Seq != len(state.Intake.Messages) {
		t.Fatalf("a message to the other person needs a place in the order: %+v", last)
	}
}

func TestReadingARequestToTheAssistant(t *testing.T) {
	now := time.Now()
	c := AgentContext{Role: "customer", Mandate: DefaultMandate("customer"), Now: now}
	cases := []struct {
		text    string
		want    string
		windows bool
	}{
		{"Can we do Thursday instead?", "reschedule", true},
		{"could we move it to next week", "reschedule", true},
		{"I need to reschedule", "reschedule", false},
		{"Kunnen we donderdag doen in plaats daarvan?", "reschedule", true},
		{"verplaats het bezoek naar vrijdag", "reschedule", true},
		{"please cancel the visit", "cancel", false},
		{"I don't need the plumber any more", "cancel", false},
		{"annuleer de afspraak", "cancel", false},
		{"please don't cancel it", "unclear", false},
		{"The key is under the mat", "unclear", false},
		{"Thursday", "unclear", false},
	}
	for _, tc := range cases {
		reply := &Message{Role: "customer", Text: tc.text, Code: askCode, Seq: 3}
		got := c.localIntent(AgentDialog{Stage: "idle"}, reply)
		if got.Intent != tc.want || (len(got.Windows) > 0) != tc.windows {
			t.Errorf("%q: got %+v, want %s (windows %v)", tc.text, got, tc.want, tc.windows)
		}
	}
}

func TestAModelCanReadARequestButOnlyAsAnIntent(t *testing.T) {
	cases := []struct {
		name  string
		stage string
		in    ReplyIntent
		want  string
	}{
		{"a move with days", "ask", ReplyIntent{Intent: "reschedule", Windows: []TimeWindow{{Weekdays: []int{4}, From: 480, To: 1200}}}, "reschedule"},
		{"a move with no days", "ask", ReplyIntent{Intent: "reschedule"}, "reschedule"},
		{"a move with an impossible window drops the window", "ask", ReplyIntent{Intent: "reschedule", Windows: []TimeWindow{{From: 900, To: 600}}}, "reschedule"},
		{"a cancellation", "ask", ReplyIntent{Intent: "cancel"}, "cancel"},
		{"a yes is not a request", "ask", ReplyIntent{Intent: "yes"}, "unclear"},
		{"a booking is not allowed", "ask", ReplyIntent{Intent: "choose", Choice: 1}, "unclear"},
		{"cancelling while saying when", "move", ReplyIntent{Intent: "cancel"}, "cancel"},
		{"days in the follow-up", "move", ReplyIntent{Intent: "availability", Windows: []TimeWindow{{From: 480, To: 720}}}, "availability"},
		{"a move cannot start from a confirmation", "confirm", ReplyIntent{Intent: "cancel"}, "unclear"},
	}
	for _, tc := range cases {
		got := tc.in.Sanitize(tc.stage, 0)
		if got.Intent != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got.Intent, tc.want)
		}
		if tc.name == "a move with an impossible window drops the window" && len(got.Windows) != 0 {
			t.Errorf("%s: kept %+v", tc.name, got.Windows)
		}
	}
}

func TestAHardToReadRequestGoesToTheModelOnce(t *testing.T) {
	c, _, _ := newFollowup(t)
	c.ask("any chance of pushing things back a few days?")
	ctx := AgentContext{State: c.state, Role: "customer", Mandate: c.m, Now: c.at}
	query, offered, seq, ok := PendingReply(ctx)
	if !ok || query.Stage != "ask" || query.Question != "" || offered != 0 || seq == 0 || query.Reply != "any chance of pushing things back a few days?" {
		t.Fatalf("expected a request for the model, got %+v %v", query, ok)
	}
	// With no reading the assistant says what it can do; with the model's reading it asks when.
	if d := DecideAgent(ctx); d.Kind != AgentAct || !contains(d.Action.Value, "switch the box below") {
		t.Fatalf("expected the fallback without a model, got %+v", d)
	}
	reading := ReplyIntent{Intent: "reschedule"}.Sanitize("ask", 0)
	ctx.Interpreted, ctx.InterpretedSeq = &reading, seq
	if d := DecideAgent(ctx); d.Kind != AgentAct || !contains(d.Action.Value, "When would you like to move the visit to?") {
		t.Fatalf("expected a question about the day, got %+v", d)
	}
	// A plain request the built-in reading already settles never reaches the model.
	c2, _, _ := newFollowup(t)
	c2.ask("Please cancel the visit")
	if _, _, _, ok := PendingReply(AgentContext{State: c2.state, Role: "customer", Mandate: c2.m, Now: c2.at}); ok {
		t.Fatal("a clear cancellation should not be sent to a model")
	}
}

// finishedWork is the booked visit after the professional marked the work complete.
func finishedWork(now time.Time) State {
	state, _, _ := bookedFollowup(now)
	state.Job.WorkStarted, state.Job.EvidenceSubmitted, state.Job.DetailAdded = true, true, true
	state.Job.EvidenceSubmittedAt = now.UTC().Format(time.RFC3339)
	state.Job.Evidence = []Evidence{
		{Label: "Work photo", Kind: "photo", AssetID: "a1", ContentType: "image/png"},
		{Label: "Work photo", Kind: "photo", AssetID: "a2", ContentType: "image/png"},
		{Label: "Professional work note", Kind: "note", Description: "Replaced the leaking connection and checked it under running water."},
	}
	state.Job.EvidenceReview = &EvidenceReview{Summary: "Both photos show a dry connection."}
	return state
}

func TestAssistantAsksBeforeApprovingFinishedWorkAndNeverPays(t *testing.T) {
	now := time.Now()
	c := &converse{t: t, state: finishedWork(now), m: DefaultMandate("customer"), at: now}
	c.settle()
	question := c.lastAssistant()
	for _, want := range []string{"Marieke says the work is done", "2 files", "Replaced the leaking connection", "Both photos show a dry connection", "Shall I approve it?", "only you can do, in PayPal"} {
		if !contains(question.Text, want) {
			t.Fatalf("the question is missing %q: %q", want, question.Text)
		}
	}
	if !c.state.AgentDialog.Awaiting || len(c.state.AgentDialog.Choices) != 2 || question.Audience != "customer" || c.state.Job.CompletionApproved {
		t.Fatalf("expected an open yes/no question and nothing approved: %+v", c.state.AgentDialog)
	}
	before := len(c.state.Intake.Messages)
	if d := c.settle(); d.Code != "awaiting_reply" || len(c.state.Intake.Messages) != before {
		t.Fatalf("the assistant must wait and not repeat itself: %+v", d)
	}
	c.reply("Yes, approve it", "yes")
	d := c.settle()
	if !c.state.Job.CompletionApproved {
		t.Fatalf("a yes should approve the work: %v", c.log)
	}
	notice := c.lastAssistant()
	if !contains(notice.Text, "The work is approved") || !contains(notice.Text, "PayPal needs your own approval") || notice.Code != "link:payment" || notice.Audience != "customer" {
		t.Fatalf("expected the payment notice with a link, got %+v", notice)
	}
	if d.Code != "payment_needs_you" || c.state.Job.PaymentApprovedVersion != 0 || c.state.Job.Paid {
		t.Fatalf("the assistant must leave payment to the customer: %+v %v", d, c.log)
	}
	for _, step := range c.log {
		if strings.Contains(step, "pay") && step != "tell-payment" {
			t.Fatalf("the assistant took a payment step: %v", c.log)
		}
	}
	// Paying happens in PayPal; the assistant then confirms, once.
	c.state.Job.PaymentApprovedVersion, c.state.Job.Paid, c.state.Job.PaymentTransactionID = c.state.Job.ScopeVersion, true, "CAPTURE-1"
	c.settle()
	if text := c.lastAssistant().Text; !contains(text, "payment of €90.00 to Marieke is complete") {
		t.Fatalf("expected the payment confirmation, got %q", text)
	}
	before = len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("the assistant repeated itself")
	}
}

func TestDecliningTheWorkLeavesItWithTheCustomer(t *testing.T) {
	now := time.Now()
	c := &converse{t: t, state: finishedWork(now), m: DefaultMandate("customer"), at: now}
	c.settle()
	c.reply("Not yet", "no")
	d := c.settle()
	if c.state.Job.CompletionApproved || d.Code != "check_the_work" || !contains(c.lastAssistant().Text, "I won't approve it and nothing is paid") {
		t.Fatalf("a no must not approve anything: %+v %q", d, c.lastAssistant().Text)
	}
	// A new submission is asked about again.
	c.state.Job.EvidenceSubmittedAt = now.Add(time.Hour).UTC().Format(time.RFC3339)
	c.settle()
	if !c.state.AgentDialog.Awaiting || !contains(c.lastAssistant().Text, "says the work is done") {
		t.Fatalf("expected a new question for a new submission, got %q", c.lastAssistant().Text)
	}
}

func TestWorkThatCannotBeApprovedYetIsLeftToThePerson(t *testing.T) {
	now := time.Now()
	state := finishedWork(now)
	state.Job.DetailAdded = false
	c := &converse{t: t, state: state, m: DefaultMandate("customer"), at: now}
	if d := c.settle(); d.Code != "check_the_work" {
		t.Fatalf("with the work note missing the workflow would refuse, so the assistant must not ask: %+v", d)
	}
}

func TestAssistantAsksInDutchAndReadsAnAnswer(t *testing.T) {
	now := time.Now()
	state := finishedWork(now)
	state.Intake.Locale = "nl"
	c := &converse{t: t, state: state, m: DefaultMandate("customer"), at: now}
	c.settle()
	if text := c.lastAssistant().Text; !contains(text, "meldt dat het werk klaar is") || !contains(text, "2 bestanden") || !contains(text, "alleen jij kunt doen, in PayPal") {
		t.Fatalf("expected the Dutch question, got %q", text)
	}
	c.reply("ja", "")
	c.settle()
	if !c.state.Job.CompletionApproved || !contains(c.lastAssistant().Text, "Het werk is goedgekeurd") {
		t.Fatalf("expected the approval and the Dutch payment notice, got %q", c.lastAssistant().Text)
	}
}

func TestProfessionalAssistantReportsTheApprovalAndPayment(t *testing.T) {
	now := time.Now()
	state := finishedWork(now)
	c := &converse{t: t, state: state, m: DefaultMandate("professional"), role: "professional", at: now}
	c.settle() // the confirmed visit is reported first
	before := len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("nothing new to report yet")
	}
	c.state.Job.CompletionApproved, c.state.Job.CompletionApprovedAt = true, now.UTC().Format(time.RFC3339)
	c.settle()
	report := c.lastAssistant()
	if !contains(report.Text, "Allan approved the finished work") || !contains(report.Text, "they still have to approve it in PayPal") || report.Audience != "professional" {
		t.Fatalf("expected the approval report, got %+v", report)
	}
	c.state.Job.PaymentApprovedVersion, c.state.Job.PaymentApprovedAt = c.state.Job.ScopeVersion, now.UTC().Format(time.RFC3339)
	c.settle()
	if text := c.lastAssistant().Text; !contains(text, "approved the payment of €90.00 in PayPal") {
		t.Fatalf("expected the payment approval report, got %q", text)
	}
	c.state.Job.Paid, c.state.Job.PaymentTransactionID = true, "CAPTURE-1"
	c.settle()
	if text := c.lastAssistant().Text; text != "Allan's payment of €90.00 has arrived." {
		t.Fatalf("expected the payment report, got %q", text)
	}
	before = len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("the assistant repeated itself")
	}
	// The customer's side of the chat never shows it.
	for _, message := range ConversationForRole(c.state, "customer").Intake.Messages {
		if message.Audience == "professional" {
			t.Fatalf("the customer can see the professional's chat: %+v", message)
		}
	}
	// In Dutch.
	c.state.Intake.Locale = "nl"
	c.state.Job.PaymentTransactionID = "CAPTURE-2"
	c.m = DefaultMandate("professional")
	d := DecideAgent(AgentContext{State: c.state, Role: "professional", Locale: "nl", Mandate: c.m, Now: now})
	if d.Kind != AgentAct || !contains(d.Action.Value, "van Allan is binnen") {
		t.Fatalf("expected the Dutch payment report, got %+v", d)
	}
}

func TestMilestonesAreRememberedTogether(t *testing.T) {
	var d AgentDialog
	for i := 0; i < 20; i++ {
		d.report(string(rune('a' + i)))
	}
	if d.reported("a") || !d.reported("t") || len(strings.Fields(d.Notified)) != reportedKeep {
		t.Fatalf("expected the most recent %d milestones: %q", reportedKeep, d.Notified)
	}
}
