package workflow

import (
	"strings"
	"testing"
	"time"
)

var agentNow = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)

func agentSlot(id, start, end string) Appointment {
	return Appointment{SlotID: id, StartUTC: start, EndUTC: end}
}

func customerMandate() AgentMandate {
	m := AgentMandate{Enabled: true, MaxTotalMinor: 10000, ServiceAddress: "Keizersgracht 1, Amsterdam"}
	if err := m.Normalize("customer"); err != nil {
		panic(err)
	}
	return m
}

func professionalMandate() AgentMandate {
	m := AgentMandate{Enabled: true, AutoConfirmVisits: true, AutoAcceptDates: true}
	if err := m.Normalize("professional"); err != nil {
		panic(err)
	}
	return m
}

func quotedState(total int64) State {
	job := Job{ID: "job_1", Quote: Quote{TotalMinor: total}, ScopeHistory: []ScopeSnapshot{{Version: 1, TotalMinor: 9000}}}
	return State{Job: job, AvailableSlots: []Appointment{
		agentSlot("sat", "2026-10-24T09:00:00Z", "2026-10-24T11:00:00Z"),
		agentSlot("tue", "2026-10-20T09:00:00Z", "2026-10-20T11:00:00Z"),
	}}
}

func TestCustomerAgentBooksInsideMandate(t *testing.T) {
	d := DecideAgent(AgentContext{State: quotedState(9000), Role: "customer", Mandate: customerMandate(), Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "approve-and-book" || d.Action.Value != "tue" || d.Action.PaymentAuthorizationTiming != "after_completion" {
		t.Fatalf("expected booking Tuesday, got %+v %+v", d, d.Action)
	}
}

func TestCustomerReviewsProfessionalBeforeAssistantCanContinue(t *testing.T) {
	for _, bookAlone := range []bool{false, true} {
		state := NewState()
		state.Intake = &Intake{Step: "matches", Budget: "EUR 120", Matches: testMatches("noor")}
		mandate := customerMandate()
		mandate.BookWithoutAsking = bookAlone
		d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: mandate, Now: agentNow})
		if d.Kind != AgentEscalate || d.Code != "choose_professional" || d.Action != nil {
			t.Fatalf("assistant chose a professional before customer confirmation (automatic visits=%v): %+v action=%+v", bookAlone, d, d.Action)
		}
	}
}

func TestProfessionalSelectionRequiresTheCustomersOwnAction(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Category: "Plumbing", Location: &Location{Label: "Amsterdam"}, Matches: testMatches("noor")}
	input := Action{Actor: "customer", Action: "select-match", Value: "noor", ByAgent: true, SelectedOffer: testOffer("noor", "Plumbing"), CustomerName: "Test customer"}
	if err := Apply(&state, input); err == nil || state.Job.ID != "" {
		t.Fatal("assistant assigned a professional without a customer action")
	}
	input.ByAgent = false
	if err := Apply(&state, input); err != nil {
		t.Fatalf("customer could not choose a professional: %v", err)
	}
	if state.Job.ProfessionalProfileID != "noor" {
		t.Fatal("customer choice was not saved")
	}
}

func TestReturningToResultsClosesThePreviousProfessionalsAssistantQuestions(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Category: "Plumbing", Location: &Location{Label: "Amsterdam"}, Matches: testMatches("noor")}
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "noor", SelectedOffer: testOffer("noor", "Plumbing")}); err != nil {
		t.Fatal(err)
	}
	state.AgentDialog = &AgentDialog{Stage: "address", Awaiting: true, Asked: 1}
	state.ProDialog = &AgentDialog{Stage: "confirm", Awaiting: true, Asked: 1}
	if err := Apply(&state, Action{Actor: "customer", Action: "return-to-matches", Matches: testMatches("noor")}); err != nil {
		t.Fatal(err)
	}
	if state.Job.ID != "" || state.AgentDialog != nil || state.ProDialog != nil {
		t.Fatal("previous assistant questions survived the return to professional review")
	}
}

func TestCustomerAgentCountersThenEscalates(t *testing.T) {
	m := customerMandate()
	state := quotedState(11000) // above the 100.00 limit, limit is 91% of the 90.00... published
	state.Job.ScopeHistory[0].TotalMinor = 11000
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "propose-price" || d.Action.PriceAmountMinor != 10000 {
		t.Fatalf("expected a counter at the limit, got %+v", d)
	}
	d = DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow, Counters: 1})
	if d.Kind != AgentEscalate || d.Code != "price_above_limit" {
		t.Fatalf("expected escalation after the counters are used, got %+v", d)
	}
	state.Job.ScopeHistory[0].TotalMinor = 30000
	state.Job.Quote.TotalMinor = 30000
	d = DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow})
	if d.Kind != AgentEscalate {
		t.Fatalf("a far-off price must go to the person, got %+v", d)
	}
}

func TestCustomerAgentEscalatesWhenNoTimeFitsWindow(t *testing.T) {
	state := quotedState(9000)
	state.AvailableSlots = state.AvailableSlots[:1] // Saturday only
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow, DateProposals: 1})
	if d.Kind != AgentEscalate || d.Code != "no_time_in_window" {
		t.Fatalf("expected escalation once a proposal was already sent, got %+v", d)
	}
}

func TestCustomerAgentNeedsAddressAndLimit(t *testing.T) {
	m := customerMandate()
	m.ServiceAddress = ""
	state := quotedState(9000)
	state.Intake = &Intake{Location: &Location{}}
	m.BookWithoutAsking = true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Code != "address_needed" {
		t.Fatalf("expected address escalation, got %+v", d)
	}
	m.MaxTotalMinor, m.ServiceAddress = 0, "Keizersgracht 1, Amsterdam"
	state.Job.ServiceAddress = m.ServiceAddress
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Code != "approve_price" {
		t.Fatalf("with no budget the person approves the price, got %+v", d)
	}
}

func TestDisabledMandateNeverActs(t *testing.T) {
	m := customerMandate()
	m.Enabled = false
	if d := DecideAgent(AgentContext{State: quotedState(9000), Role: "customer", Mandate: m, Now: agentNow}); d.Kind != AgentWait {
		t.Fatalf("a disabled agent must not act: %+v", d)
	}
}

func TestProfessionalAgentPricing(t *testing.T) {
	state := quotedState(9000)
	state.Job.PriceProposal = &PriceProposal{ID: "p1", Actor: "customer", Status: "pending", AmountMinor: 8500}
	m := professionalMandate() // floor 90% of 90.00 = 81.00
	d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "accept-price" || d.Action.ProposalID != "p1" {
		t.Fatalf("expected accept, got %+v", d)
	}
	state.Job.PriceProposal.AmountMinor = 7000
	d = DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "propose-price" || d.Action.PriceAmountMinor != 8100 {
		t.Fatalf("expected counter at the floor, got %+v", d)
	}
	d = DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow, Counters: 1})
	if d.Kind != AgentEscalate || d.Code != "price_below_floor" {
		t.Fatalf("expected escalation, got %+v", d)
	}
}

func TestProfessionalAgentConfirmsOnlyInsideMandate(t *testing.T) {
	state := quotedState(9000)
	slot := agentSlot("tue", "2026-10-20T09:00:00Z", "2026-10-20T11:00:00Z")
	state.Job.Appointment = &slot
	state.Job.QuoteApproved = true
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "pending"}
	m := professionalMandate()
	if d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow}); d.Kind != AgentAct || d.Action.Action != "confirm-appointment" {
		t.Fatalf("expected confirmation, got %+v", d)
	}
	sat := agentSlot("sat", "2026-10-24T09:00:00Z", "2026-10-24T11:00:00Z")
	state.Job.Appointment = &sat
	if d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow}); d.Kind != AgentEscalate {
		t.Fatalf("a Saturday visit is outside the mandate, got %+v", d)
	}
	m.AutoConfirmVisits = false
	state.Job.Appointment = &slot
	if d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow}); d.Code != "confirm_needed" {
		t.Fatalf("auto-confirm off must escalate, got %+v", d)
	}
}

func TestAgentNeverJudgesWorkOrPays(t *testing.T) {
	state := quotedState(9000)
	state.Job.EvidenceSubmitted = true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow}); d.Kind != AgentEscalate || d.Code != "check_the_work" {
		t.Fatalf("approving work stays with the person, got %+v", d)
	}
	state.Job.CompletionApproved = true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow}); d.Code != "payment_needs_you" {
		t.Fatalf("payment stays with the person, got %+v", d)
	}
}

func TestMandateNormalizeRejectsUnsafeAuthority(t *testing.T) {
	cases := map[string]AgentMandate{
		"customer": {MaxTotalMinor: 5_000_000},
	}
	for role, m := range cases {
		if err := m.Normalize(role); err == nil {
			t.Fatalf("%s: expected rejection", role)
		}
	}
	m := AgentMandate{MaxTotalMinor: 1000, AutoApproveExtraMinor: 2000}
	if m.Normalize("customer") == nil {
		t.Fatal("extra work above the limit must be rejected")
	}
	p := AgentMandate{FloorPercent: 10}
	if p.Normalize("professional") == nil {
		t.Fatal("a floor under 50 percent must be rejected")
	}
}

// bookedState is a confirmed visit on Wednesday 21 October 2026 09:00 UTC with a pending date proposal
// from the other side. With agentNow at Monday 08:00 UTC the visit is more than 24 hours away.
func bookedState(proposer, start, end string) State {
	state := quotedState(9000)
	booked := agentSlot("wed", "2026-10-21T09:00:00Z", "2026-10-21T11:00:00Z")
	state.Job.QuoteApproved = true
	state.Job.Appointment = &booked
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "confirmed", Appointment: booked}
	state.Job.PaymentAuthorizationTiming = PaymentAuthorizationAfterCompletion
	state.Job.DateProposal = &DateProposal{ID: "d1", Status: "pending", Actor: proposer, Appointment: agentSlot("", start, end)}
	return state
}

func TestProfessionalAgentAcceptsRescheduleInsideMandate(t *testing.T) {
	state := bookedState("customer", "2026-10-22T09:00:00Z", "2026-10-22T11:00:00Z")
	d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: professionalMandate(), Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "accept-date" || d.Action.ProposalID != "d1" {
		t.Fatalf("expected the new time to be accepted, got %+v", d)
	}
	// Outside the allowed hours, on a weekend, or with auto-accept off, the person decides.
	state = bookedState("customer", "2026-10-24T09:00:00Z", "2026-10-24T11:00:00Z")
	if d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: professionalMandate(), Now: agentNow}); d.Kind != AgentEscalate || d.Code != "visit_outside_window" {
		t.Fatalf("a Saturday reschedule is outside the mandate, got %+v", d)
	}
	state = bookedState("customer", "2026-10-22T09:00:00Z", "2026-10-22T11:00:00Z")
	m := professionalMandate()
	m.AutoAcceptDates = false
	if d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow}); d.Code != "visit_proposal_needs_you" {
		t.Fatalf("auto-accept off must escalate, got %+v", d)
	}
}

func TestAgentsLeaveLateAndPaidChangesToPeople(t *testing.T) {
	late := bookedState("customer", "2026-10-22T09:00:00Z", "2026-10-22T11:00:00Z")
	soon := agentSlot("soon", "2026-10-19T15:00:00Z", "2026-10-19T17:00:00Z") // 7 hours after agentNow
	late.Job.Appointment = &soon
	for _, role := range []string{"professional", "customer"} {
		mandate := professionalMandate()
		proposer := "customer"
		if role == "customer" {
			mandate, proposer = customerMandate(), "professional"
		}
		late.Job.DateProposal.Actor = proposer
		if d := DecideAgent(AgentContext{State: late, Role: role, Mandate: mandate, Now: agentNow}); d.Kind != AgentEscalate || d.Code != "late_change_needs_you" {
			t.Fatalf("%s: a change under 24 hours before the visit goes to a person, got %+v", role, d)
		}
		paid := bookedState(proposer, "2026-10-22T09:00:00Z", "2026-10-22T11:00:00Z")
		paid.Job.PaymentAuthorizationTiming = PaymentAuthorizationAtBooking
		if d := DecideAgent(AgentContext{State: paid, Role: role, Mandate: mandate, Now: agentNow}); d.Kind != AgentEscalate || d.Code != "visit_change_involves_payment" {
			t.Fatalf("%s: a reschedule tied to a payment hold goes to a person, got %+v", role, d)
		}
	}
	// A late cancellation or visit change under operator review is nobody's agent to decide.
	review := bookedState("customer", "2026-10-22T09:00:00Z", "2026-10-22T11:00:00Z")
	review.CancellationReview = &CancellationRequest{Status: "pending", RequestedBy: "customer"}
	for _, role := range []string{"professional", "customer"} {
		mandate := professionalMandate()
		if role == "customer" {
			mandate = customerMandate()
		}
		if d := DecideAgent(AgentContext{State: review, Role: role, Mandate: mandate, Now: agentNow}); d.Kind != AgentWait {
			t.Fatalf("%s: operator review must not be overridden, got %+v", role, d)
		}
	}
}

func TestCustomerAgentAcceptsProfessionalReschedule(t *testing.T) {
	state := bookedState("professional", "2026-10-22T13:00:00Z", "2026-10-22T15:00:00Z")
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "accept-date" || d.Action.PaymentAuthorizationTiming != PaymentAuthorizationAfterCompletion {
		t.Fatalf("expected the new time to be accepted, got %+v", d)
	}
	state = bookedState("professional", "2026-10-22T20:00:00Z", "2026-10-22T22:00:00Z")
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow}); d.Code != "visit_outside_window" {
		t.Fatalf("an evening visit is outside the default hours, got %+v", d)
	}
}

func TestCustomerAgentRebooksAfterDecline(t *testing.T) {
	state := quotedState(9000)
	declined := agentSlot("tue", "2026-10-20T09:00:00Z", "2026-10-20T11:00:00Z")
	state.Job.QuoteApproved = true
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "declined", Appointment: declined}
	state.Job.PaymentAuthorizationTiming = PaymentAuthorizationAfterCompletion
	// The released Tuesday slot is free again, but the professional just declined it.
	state.AvailableSlots = []Appointment{declined, agentSlot("thu", "2026-10-22T09:00:00Z", "2026-10-22T11:00:00Z")}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "book-appointment" || d.Action.Value != "thu" {
		t.Fatalf("expected another time than the declined one, got %+v %+v", d, d.Action)
	}
	state.AvailableSlots = state.AvailableSlots[:1]
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow, DateProposals: 1}); d.Code != "no_time_in_window" {
		t.Fatalf("with only the declined time left the person decides, got %+v", d)
	}
}

func TestCustomerAgentFindsAnotherAfterProfessionalCancels(t *testing.T) {
	state := quotedState(9000)
	state.Cancelled, state.CancelledBy = true, "professional"
	state.Job.ProfessionalProfileID, state.Job.Professional = "pro_1", "Flow plumber"
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "return-to-matches" || d.Params["profileId"] != "pro_1" {
		t.Fatalf("expected the customer agent to go back to the matches, got %+v", d)
	}
	// A customer cancellation is final for both agents.
	state.CancelledBy = "customer"
	for _, role := range []string{"professional", "customer"} {
		mandate := professionalMandate()
		if role == "customer" {
			mandate = customerMandate()
		}
		if d := DecideAgent(AgentContext{State: state, Role: role, Mandate: mandate, Now: agentNow}); d.Kind != AgentWait {
			t.Fatalf("%s: a cancelled request needs no step, got %+v", role, d)
		}
	}
}

func TestCustomerAgentSkipsProfessionalWhoCancelled(t *testing.T) {
	state := State{Intake: &Intake{Step: "matches", Matches: []Match{{ID: "pro_1", Name: "Flow plumber", TotalMinor: 9000}, {ID: "pro_2", Name: "Other", TotalMinor: 9500}}}}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow, Avoid: []string{"pro_1"}})
	if d.Kind != AgentEscalate || d.Code != "choose_professional" || d.Params["profileId"] != "pro_2" || d.Action != nil {
		t.Fatalf("expected the customer to review the second professional, got %+v", d)
	}
	state.Intake.Matches = state.Intake.Matches[:1]
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow, Avoid: []string{"pro_1"}}); d.Code != "no_match_within_limits" {
		t.Fatalf("with nobody else left the person decides, got %+v", d)
	}
}

func TestCustomerAgentProposesAVisitWhenNoPublishedTimeFits(t *testing.T) {
	m := customerMandate()
	m.PreferredHour = 15
	state := quotedState(9000)
	state.Job.Quote.DurationMinutes = 60
	state.AvailableSlots = state.AvailableSlots[:1] // Saturday only, outside the weekday window
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "approve-quote" {
		t.Fatalf("the plan is approved before a time is proposed, got %+v", d)
	}
	state.Job.QuoteApproved = true
	d = DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "propose-date" || d.Action.ProposedAppointment == nil {
		t.Fatalf("expected a visit proposal, got %+v", d)
	}
	// Tuesday 20 October: more than 24 hours after Monday 08:00 UTC, 15:00 in Amsterdam.
	if got := d.Action.ProposedAppointment.StartUTC; got != "2026-10-20T13:00:00Z" {
		t.Fatalf("proposed %s", got)
	}
	d = DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow, DateProposals: 1})
	if d.Kind != AgentEscalate || d.Code != "no_time_in_window" {
		t.Fatalf("one proposal each, then the person decides: %+v", d)
	}
}

func TestProfessionalAgentCountersAVisitOutsideItsHours(t *testing.T) {
	state := quotedState(9000)
	state.Job.Quote.DurationMinutes = 60
	state.Job.QuoteApproved = true
	state.AvailableSlots = nil
	state.Job.DateProposal = &DateProposal{ID: "d1", Actor: "customer", Status: "pending", Appointment: agentSlot("", "2026-10-20T18:00:00Z", "2026-10-20T19:00:00Z")} // 20:00 Amsterdam
	m := professionalMandate()
	m.StartHour, m.EndHour = 9, 17
	d := DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "propose-date" || d.Action.ProposedAppointment.StartUTC != "2026-10-20T14:00:00Z" {
		t.Fatalf("expected a counter at 16:00 Amsterdam, got %+v %+v", d, d.Action)
	}
	d = DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow, DateProposals: 1})
	if d.Kind != AgentEscalate || d.Code != "visit_outside_window" {
		t.Fatalf("a second counter is not allowed: %+v", d)
	}
	// A free published slot inside the hours is preferred to a computed time.
	state.AvailableSlots = []Appointment{agentSlot("tue", "2026-10-20T09:00:00Z", "2026-10-20T10:00:00Z")}
	d = DecideAgent(AgentContext{State: state, Role: "professional", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.ProposedAppointment.StartUTC != "2026-10-20T09:00:00Z" {
		t.Fatalf("expected the free published slot, got %+v %+v", d, d.Action)
	}
}

func TestBudgetMinor(t *testing.T) {
	for text, want := range map[string]int64{
		"My budget is EUR 120": 12000, "around €1.250,50": 125050, "120,5 euro": 12050, "1.250": 125000, "1,250": 125000,
		"150.00": 15000, "I'm flexible": 0, "": 0, "about 99": 9900,
	} {
		if got := BudgetMinor(text); got != want {
			t.Errorf("BudgetMinor(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestAgentIsOnByDefaultAndUsesTheChatBudget(t *testing.T) {
	m := DefaultMandate("customer")
	if !m.Enabled || m.MaxTotalMinor != 0 {
		t.Fatalf("customer default: %+v", m)
	}
	if p := DefaultMandate("professional"); !p.Enabled || !p.AutoConfirmVisits || !p.AutoAcceptDates || p.FloorPercent != 90 {
		t.Fatalf("professional default: %+v", p)
	}
	m.Weekdays, m.StartHour, m.EndHour, m.BookWithoutAsking = []int{1, 2, 3, 4, 5, 6, 7}, 0, 24, true
	state := quotedState(9000)
	state.Intake = &Intake{Budget: "My budget is EUR 120"}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow})
	if d.Kind != AgentAct || d.Action.Action != "approve-and-book" {
		t.Fatalf("inside the budget named in the chat the agent books: %+v", d)
	}
	state.Intake.Budget = "My budget is EUR 50"
	state.Job.ScopeHistory[0].TotalMinor = 20000
	state.Job.Quote.TotalMinor = 20000
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Kind != AgentEscalate || d.Code != "price_above_limit" {
		t.Fatalf("far above the chat budget goes to the person: %+v", d)
	}
	state.Intake.Budget = "I'm flexible"
	state.Job.Quote.TotalMinor = 9000
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Kind != AgentEscalate || d.Code != "approve_price" {
		t.Fatalf("with no budget the person approves the price: %+v", d)
	}
	state.Job.QuoteApproved = true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Kind != AgentAct || d.Action.Action != "book-appointment" {
		t.Fatalf("once the person approved the price the agent books: %+v", d)
	}
}

func TestWorkApprovalIsDelegatedOnlyWhenTheCustomerSaysSo(t *testing.T) {
	m := DefaultMandate("customer")
	state := quotedState(9000)
	state.Job.QuoteApproved, state.Job.EvidenceSubmitted = true, true
	appt := agentSlot("tue", "2026-10-20T09:00:00Z", "2026-10-20T11:00:00Z")
	state.Job.Appointment, state.Job.WorkStarted = &appt, true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Code != "check_the_work" {
		t.Fatalf("work approval stays with the person by default: %+v", d)
	}
	m.AutoApproveWork = true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Kind != AgentAct || d.Action.Action != "approve-completion" {
		t.Fatalf("delegated work approval: %+v", d)
	}
	state.Job.CompletionApproved = true
	if d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: m, Now: agentNow}); d.Code != "payment_needs_you" {
		t.Fatalf("payment is always the person's: %+v", d)
	}
}

// converse plays an assistant and its customer against the real workflow rules.
type converse struct {
	t     *testing.T
	state State
	m     AgentMandate
	log   []string
	role  string    // defaults to the customer
	at    time.Time // the assistant's clock; agentNow when unset
}

func (c *converse) clock() time.Time {
	if c.at.IsZero() {
		return agentNow
	}
	return c.at
}

func (c *converse) who() string {
	if c.role == "" {
		return "customer"
	}
	return c.role
}

func (c *converse) step() AgentDecision {
	c.t.Helper()
	d := DecideAgent(AgentContext{State: c.state, Role: c.who(), Locale: "en", Mandate: c.m, Now: c.clock()})
	if d.Kind == AgentAct {
		action := *d.Action
		action.Actor, action.ByAgent = c.who(), true
		if err := Apply(&c.state, action); err != nil {
			c.t.Fatalf("%s was rejected: %v", d.Code, err)
		}
		c.log = append(c.log, d.Code)
	}
	return d
}

// settle lets the assistant act until it waits for the customer or has nothing to do.
func (c *converse) settle() AgentDecision {
	c.t.Helper()
	for i := 0; i < 12; i++ {
		if d := c.step(); d.Kind != AgentAct {
			return d
		}
	}
	c.t.Fatalf("the assistant did not settle: %v", c.log)
	return AgentDecision{}
}

func (c *converse) reply(text, choice string) {
	c.t.Helper()
	if err := Apply(&c.state, Action{Action: "agent-reply", Actor: c.who(), Value: text, ChoiceID: choice}); err != nil {
		c.t.Fatalf("reply %q: %v", text, err)
	}
}

func (c *converse) lastAssistant() Message {
	messages := c.state.Intake.Messages
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "agent" {
			return messages[i]
		}
	}
	c.t.Fatal("the assistant said nothing")
	return Message{}
}

func TestAssistantAsksForAvailabilityAndOffersWhatFits(t *testing.T) {
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID = "Alex", "Marieke de Vries", "prof_1"
	state.Job.Quote.DurationMinutes = 60
	state.Intake = &Intake{Locale: "en", Budget: "My budget is EUR 150", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "My sink leaks"}}}
	state.AvailableSlots = []Appointment{
		agentSlot("tue", "2026-10-20T12:00:00Z", "2026-10-20T13:00:00Z"), // Tuesday 14:00 in Amsterdam
		agentSlot("thu", "2026-10-22T08:00:00Z", "2026-10-22T09:00:00Z"), // Thursday 10:00
	}
	m := DefaultMandate("customer")
	c := &converse{t: t, state: state, m: m}

	// 1. It needs the address and asks for it; with no answer yet it waits.
	if d := c.settle(); d.Kind != AgentEscalate || d.Code != "awaiting_reply" {
		t.Fatalf("expected to wait for the address, got %+v", d)
	}
	if text := c.lastAssistant().Text; !contains(text, "full address") || !contains(text, "Marieke") {
		t.Fatalf("unexpected question: %q", text)
	}
	if message := c.lastAssistant(); message.Audience != "customer" {
		t.Fatalf("the question must stay private to the customer: %+v", message)
	}
	// 2. A too-short answer is asked again; a real address is saved and it moves on to availability.
	c.reply("Damrak", "")
	if d := c.settle(); d.Code != "awaiting_reply" || !contains(c.lastAssistant().Text, "street, house number") {
		t.Fatalf("expected the address to be asked again, got %+v %q", d, c.lastAssistant().Text)
	}
	c.reply("Damrak 1, 1012 LG Amsterdam", "")
	if d := c.settle(); d.Code != "awaiting_reply" || !contains(c.lastAssistant().Text, "When are you free") || len(c.state.AgentDialog.Choices) != 4 {
		t.Fatalf("expected the availability question with quick replies, got %+v %+v", d, c.state.AgentDialog)
	}
	if c.state.Job.ServiceAddress != "Damrak 1, 1012 LG Amsterdam" {
		t.Fatal("the address was not saved")
	}
	// 3. The customer says when they are free; the assistant compares it with the open times.
	c.reply("Tuesday afternoon or Friday", "")
	if d := c.settle(); d.Code != "awaiting_reply" {
		t.Fatalf("expected an offer to wait on, got %+v", d)
	}
	offer := c.lastAssistant().Text
	if !contains(offer, "Tue 20 Oct, 14:00–15:00") || contains(offer, "Thu") || !contains(offer, "Does it suit you?") {
		t.Fatalf("expected the single fitting time, got %q", offer)
	}
	// 4. The customer agrees; only then is the visit reserved.
	if c.state.Job.Appointment != nil {
		t.Fatal("the assistant booked before the customer said yes")
	}
	c.reply("Yes, book it", "yes")
	c.settle()
	if c.state.Job.Appointment == nil || c.state.Job.Appointment.SlotID != "tue" || !c.state.Job.QuoteApproved {
		t.Fatalf("expected Tuesday to be booked: %+v", c.state.Job.Appointment)
	}
	if text := c.lastAssistant().Text; !contains(text, "I reserved") || !contains(text, "two business hours") {
		t.Fatalf("expected the reservation notice, got %q", text)
	}
	// 5. When the professional confirms, the customer hears about it once.
	if err := Apply(&c.state, Action{Action: "confirm-appointment", Actor: "professional"}); err != nil {
		t.Fatal(err)
	}
	c.settle()
	if text := c.lastAssistant().Text; !contains(text, "Marieke confirmed") {
		t.Fatalf("expected the confirmation notice, got %q", text)
	}
	before := len(c.state.Intake.Messages)
	c.settle()
	if len(c.state.Intake.Messages) != before {
		t.Fatal("the assistant repeated itself")
	}
}

func TestAssistantOffersChoicesAndAsksTheProWhenNothingFits(t *testing.T) {
	now := time.Now()
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	thursday := now.In(zone).AddDate(0, 0, 2)
	for thursday.Weekday() != time.Thursday {
		thursday = thursday.AddDate(0, 0, 1)
	}
	thursday = time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 10, 0, 0, 0, zone)
	friday := thursday.AddDate(0, 0, 1)
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID, state.Job.ServiceAddress = "Alex", "Marieke", "prof_1", "Damrak 1, Amsterdam"
	state.Job.Quote.DurationMinutes = 60
	state.Intake = &Intake{Locale: "nl", Budget: "150 euro", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "lek"}}}
	state.AvailableSlots = []Appointment{agentSlot("thu", thursday.UTC().Format(time.RFC3339), thursday.Add(time.Hour).UTC().Format(time.RFC3339)), agentSlot("fri", friday.UTC().Format(time.RFC3339), friday.Add(time.Hour).UTC().Format(time.RFC3339))}
	c := &converse{t: t, state: state, m: DefaultMandate("customer"), at: now}
	c.settle()
	c.reply("dinsdagavond", "")
	c.settle()
	offer := c.lastAssistant()
	if !contains(offer.Text, "Geen van de open tijden") || !contains(offer.Text, "10:00–11:00") {
		t.Fatalf("expected a Dutch offer of the next times, got %q", offer.Text)
	}
	if last := c.state.AgentDialog.Choices[len(c.state.AgentDialog.Choices)-1]; last.ID != "ask-pro" {
		t.Fatalf("expected the option to ask the professional, got %+v", c.state.AgentDialog.Choices)
	}
	// Asking the professional approves the plan, then proposes a time inside the customer's evening.
	c.reply("Vraag Marieke om een andere tijd", "ask-pro")
	d := c.step()
	if d.Code != "approve-plan" {
		t.Fatalf("expected the plan to be approved first, got %+v", d)
	}
	d = c.step()
	if d.Code != "propose-visit" || d.Action.ProposedAppointment == nil {
		t.Fatalf("expected a proposal to the professional, got %+v", d)
	}
}

func TestAssistantConfirmsAPriceWhenThereIsNoBudget(t *testing.T) {
	state := quotedState(13000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID, state.Job.ServiceAddress = "Alex", "Marieke", "prof_1", "Damrak 1, Amsterdam"
	state.Intake = &Intake{Locale: "en", Budget: "I'm flexible", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "x"}}}
	c := &converse{t: t, state: state, m: DefaultMandate("customer")}
	if d := c.settle(); d.Code != "awaiting_reply" || !contains(c.lastAssistant().Text, "€130.00") {
		t.Fatalf("expected a price question, got %+v %q", d, c.lastAssistant().Text)
	}
	c.reply("No, I'll decide myself", "no")
	d := c.settle()
	if d.Code != "approve_price" || c.state.Job.QuoteApproved {
		t.Fatalf("a no leaves the price with the person, got %+v", d)
	}
}

func TestOnlyAnAssistantCanPostAgentMessages(t *testing.T) {
	state := quotedState(9000)
	state.Intake = &Intake{Messages: []Message{}}
	if err := Apply(&state, Action{Action: "agent-say", Actor: "customer", Value: "I am your assistant"}); err == nil {
		t.Fatal("a person must not be able to post as an assistant")
	}
	view := ConversationForRole(State{Intake: &Intake{Messages: []Message{{Role: "agent", Text: "private", Audience: "customer"}, {Role: "agent", Text: "shared", Audience: "both"}}}, AgentDialog: &AgentDialog{Windows: []TimeWindow{{From: 1}}}}, "professional")
	if len(view.Intake.Messages) != 1 || view.Intake.Messages[0].Text != "shared" || view.AgentDialog != nil {
		t.Fatalf("the professional saw the customer's private exchange: %+v", view)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func proState() State {
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID = "Alex Example", "Marieke", "prof_1"
	state.Job.Quote.DurationMinutes = 60
	state.Job.QuoteApproved = true
	state.Intake = &Intake{Locale: "en", Messages: []Message{{Role: "customer", Text: "leak"}}}
	return state
}

func TestProfessionalAssistantAsksBeforeConfirmingOutsideItsHoursAndReports(t *testing.T) {
	state := proState()
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	future := time.Now().In(zone).AddDate(0, 0, 2)
	start := time.Date(future.Year(), future.Month(), future.Day(), 20, 0, 0, 0, zone)
	late := agentSlot("evening", start.UTC().Format(time.RFC3339), start.Add(time.Hour).UTC().Format(time.RFC3339))
	state.Job.Appointment = &late
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "pending", DueAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339), Appointment: late}
	m := DefaultMandate("professional")
	c := &converse{t: t, state: state, m: m, role: "professional"}
	if d := c.settle(); d.Code != "awaiting_reply" {
		t.Fatalf("expected a question, got %+v", d)
	}
	question := c.lastAssistant()
	if !contains(question.Text, "Alex reserved ") || !contains(question.Text, "20:00–21:00") || !contains(question.Text, "outside your usual hours") || question.Audience != "professional" {
		t.Fatalf("unexpected question %+v", question)
	}
	if c.state.ProDialog == nil || c.state.AgentDialog != nil {
		t.Fatalf("the professional's chat must use its own dialog: %+v %+v", c.state.ProDialog, c.state.AgentDialog)
	}
	c.reply("Yes", "yes")
	c.settle()
	if c.state.Job.AppointmentConfirmation.Status != "confirmed" {
		t.Fatalf("a yes should confirm the visit: %+v", c.state.Job.AppointmentConfirmation)
	}
	if text := c.lastAssistant().Text; !strings.HasPrefix(text, "Confirmed ") || !contains(text, "20:00–21:00 with Alex.") {
		t.Fatalf("expected a report of what was agreed, got %q", text)
	}
	// A visit inside the hours is confirmed without a question, and still reported.
	insideStart := time.Now().In(zone).AddDate(0, 0, 1)
	for insideStart.Weekday() == time.Saturday || insideStart.Weekday() == time.Sunday {
		insideStart = insideStart.AddDate(0, 0, 1)
	}
	insideStart = time.Date(insideStart.Year(), insideStart.Month(), insideStart.Day(), 11, 0, 0, 0, zone)
	inside := agentSlot("inside-hours", insideStart.UTC().Format(time.RFC3339), insideStart.Add(time.Hour).UTC().Format(time.RFC3339))
	state = proState()
	state.Job.Appointment = &inside
	state.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "pending", DueAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339), Appointment: inside}
	c = &converse{t: t, state: state, m: m, role: "professional"}
	c.settle()
	if len(c.log) != 2 || c.log[0] != "confirm-visit" || c.log[1] != "tell-confirmed" {
		t.Fatalf("expected confirm then report, got %v", c.log)
	}
}

func TestProfessionalAssistantAsksAboutAPriceBelowTheFloor(t *testing.T) {
	state := proState()
	state.Job.QuoteApproved = false
	state.Job.PriceProposal = &PriceProposal{ID: "p1", Actor: "customer", Status: "pending", AmountMinor: 7000}
	c := &converse{t: t, state: state, m: DefaultMandate("professional"), role: "professional"}
	c.step() // the first offer under the floor is countered automatically
	if c.log[0] != "counter-price" {
		t.Fatalf("expected an automatic counter, got %v", c.log)
	}
	// The customer comes back with the same low offer: now the professional is asked.
	c.state.Job.PriceProposal = &PriceProposal{ID: "p2", Actor: "customer", Status: "pending", AmountMinor: 7500}
	d := DecideAgent(AgentContext{State: c.state, Role: "professional", Locale: "nl", Mandate: c.m, Now: agentNow, Counters: 1})
	if d.Kind != AgentAct || d.Action.Action != "agent-say" || !contains(d.Action.Value, "onder je laagste prijs van € 81,00") {
		t.Fatalf("expected a Dutch question about the price, got %+v %q", d, d.Action.Value)
	}
}

func TestReplyIntentSanitizeKeepsAModelInsideTheConversation(t *testing.T) {
	cases := []struct {
		name   string
		stage  string
		in     ReplyIntent
		offers int
		want   string
	}{
		{"valid choice", "offer", ReplyIntent{Intent: "choose", Choice: 2}, 3, "choose"},
		{"choice out of range", "offer", ReplyIntent{Intent: "choose", Choice: 4}, 3, "unclear"},
		{"intent not allowed at this stage", "address", ReplyIntent{Intent: "yes"}, 0, "unclear"},
		{"an address without a number", "address", ReplyIntent{Intent: "address", Address: "somewhere nice"}, 0, "unclear"},
		{"a real address", "address", ReplyIntent{Intent: "address", Address: " Damrak 1, Amsterdam "}, 0, "address"},
		{"impossible window", "availability", ReplyIntent{Intent: "availability", Windows: []TimeWindow{{From: 900, To: 600}}}, 0, "unclear"},
		{"bad weekday", "availability", ReplyIntent{Intent: "availability", Windows: []TimeWindow{{Weekdays: []int{9}, From: 600, To: 900}}}, 0, "unclear"},
		{"bad date", "availability", ReplyIntent{Intent: "availability", Windows: []TimeWindow{{FromDate: "tomorrow", From: 600, To: 900}}}, 0, "unclear"},
		{"good window", "availability", ReplyIntent{Intent: "availability", Windows: []TimeWindow{{Weekdays: []int{1, 2}, From: 1080, To: 1260}}}, 0, "availability"},
	}
	for _, c := range cases {
		if got := c.in.Sanitize(c.stage, c.offers).Intent; got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestAssistantUsesAModelReadingWhenTheBuiltInOneCannot(t *testing.T) {
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID, state.Job.ServiceAddress = "Alex", "Marieke", "prof_1", "Damrak 1, Amsterdam"
	state.Job.Quote.DurationMinutes = 60
	state.Intake = &Intake{Locale: "en", Budget: "EUR 150", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "leak"}}}
	state.AvailableSlots = []Appointment{agentSlot("tue", "2026-10-20T12:00:00Z", "2026-10-20T13:00:00Z")}
	c := &converse{t: t, state: state, m: DefaultMandate("customer")}
	c.settle() // asks for availability
	c.reply("I'm usually back around six", "")
	ctx := AgentContext{State: c.state, Role: "customer", Mandate: c.m, Now: agentNow}
	query, offered, seq, ok := PendingReply(ctx)
	if !ok || query.Stage != "availability" || query.Reply != "I'm usually back around six" || offered != 0 || seq == 0 || !contains(query.Question, "When are you free") {
		t.Fatalf("expected a question for the model, got %+v %v", query, ok)
	}
	// Without a model the assistant asks again; with one it understands.
	if d := DecideAgent(ctx); d.Kind != AgentAct || !contains(d.Action.Value, "couldn't make out") {
		t.Fatalf("expected a retry without a model, got %+v", d)
	}
	reading := ReplyIntent{Intent: "availability", Windows: []TimeWindow{{From: 12 * 60, To: 17 * 60}}}.Sanitize("availability", 0)
	ctx.Interpreted, ctx.InterpretedSeq = &reading, seq
	if d := DecideAgent(ctx); d.Kind != AgentAct || !contains(d.Action.Value, "Does it suit you?") {
		t.Fatalf("expected an offer from the model's reading, got %+v", d)
	}
	// A plain yes never needs the model.
	c.state.AgentDialog.Stage, c.state.AgentDialog.Offered = "offer", []Appointment{agentSlot("tue", "2026-10-20T12:00:00Z", "2026-10-20T13:00:00Z")}
	c.state.AgentDialog.Asked = len(c.state.Intake.Messages)
	c.state.AgentDialog.Awaiting = true
	c.reply("yes", "")
	if _, _, _, ok := PendingReply(AgentContext{State: c.state, Role: "customer", Mandate: c.m, Now: agentNow}); ok {
		t.Fatal("a plain yes should not be sent to a model")
	}
}

func TestAnAddressReplyIsNeverSentToAModel(t *testing.T) {
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID = "Alex", "Marieke", "prof_1"
	state.Intake = &Intake{Locale: "en", Budget: "EUR 150", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "leak"}}}
	c := &converse{t: t, state: state, m: DefaultMandate("customer")}
	c.settle() // asks for the address
	if c.state.AgentDialog == nil || c.state.AgentDialog.Stage != "address" {
		t.Fatalf("expected the address question, got %+v", c.state.AgentDialog)
	}
	for _, text := range []string{"Damrak 1, 1012 LG Amsterdam", "the blue door at the end of the street"} {
		c.reply(text, "")
		if _, _, _, ok := PendingReply(AgentContext{State: c.state, Role: "customer", Mandate: c.m, Now: agentNow}); ok {
			t.Fatalf("an address reply %q must stay on the server", text)
		}
		c.state.AgentDialog.Awaiting = true
	}
}

func TestADeclineThatNamesNewTimesDeclinesAndKeepsThem(t *testing.T) {
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID, state.Job.ServiceAddress = "Alex", "Marieke", "prof_1", "Damrak 1, Amsterdam"
	state.Job.Quote.DurationMinutes = 60
	state.Job.QuoteApproved = true
	state.Intake = &Intake{Locale: "en", Budget: "EUR 150", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "leak"}}}
	state.Job.DateProposal = &DateProposal{ID: "d1", Actor: "professional", Status: "pending", Appointment: agentSlot("", "2026-10-30T09:00:00Z", "2026-10-30T10:00:00Z")}
	state.AvailableSlots = []Appointment{agentSlot("sat", "2026-10-31T09:00:00Z", "2026-10-31T10:00:00Z")}
	c := &converse{t: t, state: state, m: DefaultMandate("customer")}
	c.settle() // asks whether the suggested time works
	c.reply("No, I only have Saturdays", "")
	if d := c.step(); d.Code != "decline-visit" {
		t.Fatalf("expected the suggestion to be declined, got %+v", d)
	}
	d := c.settle()
	if d.Code != "awaiting_reply" || !contains(c.lastAssistant().Text, "Sat 31 Oct, 10:00–11:00") {
		t.Fatalf("expected an offer inside the new window, got %+v %q", d, c.lastAssistant().Text)
	}
}

func TestAssistantsHaveShortDistinctStableNamesAndIntroduceThemselvesOnce(t *testing.T) {
	job := Job{Customer: "Alex Example", Professional: "Marieke de Vries", ProfessionalAccountID: "acct_pro_1"}
	customer, professional := AssistantNames(job)
	if customer == "" || professional == "" || customer == professional || len(customer) > 5 || len(professional) > 5 {
		t.Fatalf("expected two short, different names, got %q and %q", customer, professional)
	}
	if again, _ := AssistantNames(job); again != customer {
		t.Fatal("a person's assistant must keep its name")
	}
	for _, seed := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m"} {
		job.Customer, job.ProfessionalAccountID = seed, seed
		if c, p := AssistantNames(job); c == p {
			t.Fatalf("both assistants were named %q for seed %q", c, seed)
		}
	}
	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID = "Alex", "Marieke", "prof_1"
	state.Intake = &Intake{Locale: "en", Budget: "EUR 150", Location: &Location{}, Messages: []Message{{Role: "customer", Text: "leak"}}}
	c := &converse{t: t, state: state, m: DefaultMandate("customer")}
	c.settle()
	first := c.lastAssistant()
	name := AssistantName(c.state.Job, "customer")
	if !contains(first.Text, "Hi, I'm "+name+", your assistant for this job.") || first.Agent != name {
		t.Fatalf("expected an introduction by %s, got %+v", name, first)
	}
	c.reply("Damrak 1, 1012 LG Amsterdam", "")
	c.settle()
	if second := c.lastAssistant(); contains(second.Text, "your assistant for this job") || second.Agent != name {
		t.Fatalf("the introduction should come only once: %+v", second)
	}
}

func TestMissingDocumentationIsNamedAndTheAssistantSaysSoInsteadOfHanging(t *testing.T) {
	job := Job{ScopeVersion: 1, CompletionRound: 1,
		WorkItems: []WorkItem{{ID: "w1", Description: "Photo of the repaired pipe joint", Kind: "photo"}, {ID: "w2", Description: "Work note on the parts replaced", Kind: "note"}},
		Evidence:  []Evidence{{ScopeVersion: 1, CompletionRound: 1, AssetID: "a1", ContentType: "image/jpeg", Kind: "photo", WorkItemIDs: []string{"w1"}}}}
	err := ValidateCompletionEvidence(job, true)
	if err == nil || !contains(err.Error(), "Work note on the parts replaced") || contains(err.Error(), "Photo of the repaired pipe joint") {
		t.Fatalf("the error should name exactly what is missing, got %v", err)
	}

	state := quotedState(9000)
	state.Job.Customer, state.Job.Professional, state.Job.ProfessionalProfileID = "Alex", "Marieke", "prof_1"
	state.Job.WorkItems, state.Job.Evidence, state.Job.ScopeVersion, state.Job.CompletionRound = job.WorkItems, job.Evidence, 1, 1
	state.Job.QuoteApproved, state.Job.WorkStarted, state.Job.EvidenceSubmitted, state.Job.DetailAdded = true, true, true, true
	state.Job.EvidenceSubmittedAt = "2026-10-20T10:00:00Z"
	state.Intake = &Intake{Locale: "en", Messages: []Message{{Role: "customer", Text: "leak"}}}
	state.AgentDialog = &AgentDialog{Stage: "confirm", Awaiting: true, Pending: "approve-work", PendingID: state.Job.EvidenceSubmittedAt, Asked: 0}
	state.Intake.Messages = append(state.Intake.Messages, Message{Seq: 1, Role: "agent", Audience: "customer", Text: "Approve?"})
	c := &converse{t: t, state: state, m: DefaultMandate("customer")}
	c.state.AgentDialog.Asked = 1
	c.reply("Yes", "yes")
	d := c.settle()
	if text := c.lastAssistant().Text; !contains(text, "I can't approve the work yet") || !contains(text, "Work note on the parts replaced") || d.Code != "check_the_work" {
		t.Fatalf("expected the assistant to say what is missing, got %q (%+v)", text, d)
	}
	if c.state.Job.CompletionApproved {
		t.Fatal("work must not be approved with documentation missing")
	}
	// A step the workflow refuses is explained in the chat and leaves the person in charge.
	notice := BlockedNotice(AgentContext{State: c.state, Role: "customer", Mandate: c.m, Now: agentNow}, "book-visit", "that time was just taken")
	if !contains(notice.Value, "that time was just taken") || notice.Dialog == nil || notice.Dialog.Awaiting {
		t.Fatalf("unexpected notice %+v", notice)
	}
}

func TestMissingDocumentationQuotesTheFileCheckWhenItFlaggedTheWork(t *testing.T) {
	job := Job{ScopeVersion: 1, CompletionRound: 1,
		Evidence:       []Evidence{{ScopeVersion: 1, CompletionRound: 1, AssetID: "a1", ContentType: "image/jpeg", Kind: "photo"}},
		EvidenceReview: &EvidenceReview{Coverage: []EvidenceFinding{{ScopeItem: "work-1", Coverage: "not_shown", Observation: "The submitted image depicts a large building and does not show a kitchen sink."}}}}
	err := ValidateCompletionEvidence(job, true)
	if err == nil || !contains(err.Error(), "the file check says: The submitted image depicts a large building and does not show a kitchen sink") || contains(err.Error(), "work-1") {
		t.Fatalf("expected what the check saw, got %v", err)
	}
}

func TestProfessionalOutsideHoursPermissionKeepsDaysOffAndPaymentGates(t *testing.T) {
	state := bookedState("customer", "2026-10-24T19:00:00Z", "2026-10-24T21:00:00Z")
	mandate := professionalMandate()
	view := &ProfessionalScheduleView{Schedule: &ProfessionalSchedule{Timezone: "Europe/Amsterdam", Week: []WorkingRange{{Weekday: 6, Start: "09:00", End: "17:00"}}}}
	ctx := AgentContext{State: state, Role: "professional", Mandate: mandate, Now: agentNow, ProfessionalSchedule: view}
	if d := DecideAgent(ctx); d.Code != "visit_outside_window" {
		t.Fatalf("outside hours accepted without permission: %+v", d)
	}
	ctx.Mandate.AcceptOutsideHours = true
	if d := DecideAgent(ctx); d.Kind != AgentAct || d.Action.Action != "accept-date" {
		t.Fatalf("explicit outside-hours permission ignored: %+v", d)
	}
	ctx.ProfessionalSchedule.DaysOff = []ProfessionalDayOff{{FirstDay: "2026-10-24", LastDay: "2026-10-24"}}
	if d := DecideAgent(ctx); d.Code != "visit_outside_window" {
		t.Fatalf("permission bypassed a day off: %+v", d)
	}
	ctx.ProfessionalSchedule.DaysOff = nil
	ctx.State.Job.PaymentAuthorizationTiming = PaymentAuthorizationAtBooking
	if d := DecideAgent(ctx); d.Code != "visit_change_involves_payment" {
		t.Fatalf("permission bypassed payment gate: %+v", d)
	}
}

func TestAcceptedPriceConfirmationKeepsCanonicalAmountParameters(t *testing.T) {
	state := quotedState(10000)
	state.Job.PriceProposal = &PriceProposal{ID: "offer", Actor: "professional", Status: "pending", AmountMinor: 9900, ScopeVersion: state.Job.ScopeVersion}
	state.Intake = &Intake{Locale: "en", Messages: []Message{{Seq: 1, Role: "agent", Audience: "customer", Text: "Shall I accept?"}, {Seq: 2, Role: "customer", Audience: "customer", Text: "Yes", Choice: "yes"}}}
	state.AgentDialog = &AgentDialog{Stage: "confirm", Pending: "accept-price", PendingID: "offer", Asked: 1}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: agentNow})
	if d.Code != "accept-price" || d.Params["amount"] != "99.00" {
		t.Fatalf("accepted price leaked display formatting into machine parameters: %+v", d)
	}
}

func TestUnverifiedAddressKeepsAssistantAtOneRecoverableQuestion(t *testing.T) {
	state := quotedState(9900)
	state.Job.ServiceAddress = ""
	state.Intake = &Intake{Locale: "en", Messages: []Message{{Seq: 1, Role: "customer", Audience: "customer", Text: "Damrak 1, Amsterdam"}}}
	state.AgentDialog = &AgentDialog{Stage: "address", Asked: 0}
	ctx := AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: agentNow}
	notice := BlockedNotice(ctx, "set-address", "Choose a full street address from the location suggestions.")
	if notice.Dialog == nil || !notice.Dialog.Awaiting || notice.Dialog.Stage != "address" || notice.NoteCode != "address:choose" || strings.Contains(notice.Value, "take it from here") {
		t.Fatalf("address ambiguity abandoned the booking instead of asking for a selection: %+v", notice)
	}
	notice.Actor = "customer"
	if err := Apply(&state, notice); err != nil {
		t.Fatal(err)
	}
	if state.Job.ServiceAddress != "" || !state.AgentDialog.Awaiting {
		t.Fatal("unverified address was accepted or the question was lost")
	}
}

func TestAssistantOfferedVisitKeepsCustomersPayPalHoldChoice(t *testing.T) {
	state := quotedState(9900)
	state.Job.QuoteApproved = true
	state.Job.PaymentAuthorizationTiming = PaymentAuthorizationAtBooking
	slot := agentSlot("hold-slot", "2026-10-22T08:00:00Z", "2026-10-22T09:00:00Z")
	state.AvailableSlots = []Appointment{slot}
	state.Intake = &Intake{Locale: "en", Messages: []Message{{Seq: 1, Role: "agent", Audience: "customer", Text: "Which time?"}, {Seq: 2, Role: "customer", Audience: "customer", Text: "First one", Choice: "slot:hold-slot"}}}
	state.AgentDialog = &AgentDialog{Stage: "offer", Asked: 1, Offered: []Appointment{slot}}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: agentNow})
	if d.Action == nil || d.Action.PaymentAuthorizationTiming != PaymentAuthorizationAtBooking {
		t.Fatalf("assistant silently changed the customer's PayPal hold choice: %+v", d)
	}
}

func TestAnExplicitlyApprovedPriceIsNotRenegotiatedByTheAssistant(t *testing.T) {
	state := quotedState(11000)
	state.Job.QuoteApproved = true
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: customerMandate(), Now: agentNow})
	if d.Action == nil || d.Action.Action != "book-appointment" {
		t.Fatalf("assistant tried to renegotiate an approved plan: %+v", d)
	}
}
