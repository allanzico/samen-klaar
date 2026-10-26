package workflow

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPriceAndAutonomyPageDoesNotAdvanceWhileCustomerIsReviewing(t *testing.T) {
	state := NewState()
	state.Intake = &Intake{Step: "matches", Category: "Plumbing", Location: &Location{Label: "Amsterdam"}, Matches: testMatches("noor")}
	if err := Apply(&state, Action{Actor: "customer", Action: "select-match", Value: "noor", SelectedOffer: testOffer("noor", "Plumbing")}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "confirm-professional", Value: state.Job.ID}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", ByAgent: true, Action: "agent-say", Value: "A stale address question"}); err == nil {
		t.Fatal("a queued agent question replaced the review page")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now().Add(time.Duration(i) * time.Second)})
		if d.Action != nil {
			t.Fatalf("review controls disappeared without a customer action: %s", d.Code)
		}
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "propose-price", Value: "My offer for the published work", PriceAmountMinor: 12000}); err != nil {
		t.Fatalf("could not make an offer while reviewing: %v", err)
	}
}

func alternativeDayConversation(t *testing.T) *converse {
	t.Helper()
	state := quotedState(9000)
	state.Job.Quote.Currency = "EUR"
	state.Job.ScopeVersion = 1
	state.Job.ServiceAddress = "Public test address"
	state.Job.Professional = "Test professional"
	state.Job.Quote.DurationMinutes = 60
	state.Job.QuoteApproved = true
	state.Intake = &Intake{Step: "selected", Locale: "en", Budget: "EUR 150", Messages: []Message{{Role: "customer", Text: "Repair the sink"}}}
	now := time.Now()
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	state.AvailableSlots = nil
	for day := 2; day <= 4; day++ {
		date := now.In(zone).AddDate(0, 0, day)
		start := time.Date(date.Year(), date.Month(), date.Day(), 10, 0, 0, 0, zone)
		state.AvailableSlots = append(state.AvailableSlots, Appointment{SlotID: start.Format("2006-01-02"), StartUTC: start.UTC().Format(time.RFC3339), EndUTC: start.Add(time.Hour).UTC().Format(time.RFC3339), Timezone: "Europe/Amsterdam"})
	}
	return &converse{t: t, state: state, m: DefaultMandate("customer"), at: now}
}
func TestRejectedVisitDaysAreNotPresentedAgain(t *testing.T) {
	c := alternativeDayConversation(t)
	c.settle()
	c.reply("Any time", "chip")
	c.settle()
	rejected := append([]Appointment{}, c.state.AgentDialog.Offered...)
	if len(rejected) != 3 {
		t.Fatal("fixture did not present the three dates")
	}
	c.reply("None of these days work for me", "no")
	c.settle()
	c.reply("Any other weekday is fine", "")
	c.settle()
	for _, again := range c.state.AgentDialog.Offered {
		for _, old := range rejected {
			if again.StartUTC[:10] == old.StartUTC[:10] {
				t.Fatal("the assistant repeated a day the customer rejected")
			}
		}
	}
}
func TestAskProfessionalProposesOutsidePresentedDaysAndWaits(t *testing.T) {
	c := alternativeDayConversation(t)
	c.settle()
	c.reply("Any time", "chip")
	c.settle()
	presented := append([]Appointment{}, c.state.AgentDialog.Offered...)
	c.reply("Ask the professional for a different day that suits me", "ask-pro")
	d := c.step()
	if d.Code == "remember-rejected-days" {
		saved, err := json.Marshal(c.state)
		if err != nil {
			t.Fatal(err)
		}
		var restored State
		if err := json.Unmarshal(saved, &restored); err != nil {
			t.Fatal(err)
		}
		c.state = restored
		d = c.step()
	}
	if d.Code != "propose-visit" || d.Action.ProposedAppointment == nil {
		t.Fatalf("did not send a real alternative: %s", d.Code)
	}
	proposed := d.Action.ProposedAppointment
	for _, old := range presented {
		if old.StartUTC[:10] == proposed.StartUTC[:10] {
			t.Fatal("alternative proposal reused a presented day")
		}
	}
	if c.state.Job.Appointment != nil || c.state.Job.DateProposal == nil || c.state.Job.DateProposal.Status != "pending" {
		t.Fatal("proposal booked before the professional answered")
	}
	if next := c.step(); next.Action != nil && next.Action.Action == "accept-date" {
		t.Fatal("customer assistant accepted its own proposal")
	}
}

func TestProfessionalCounterCannotReofferRejectedDay(t *testing.T) {
	c := alternativeDayConversation(t)
	slot := c.state.AvailableSlots[0]
	day, _ := time.Parse(time.RFC3339, slot.StartUTC)
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	c.state.AgentDialog = &AgentDialog{RejectedDates: []string{day.In(zone).Format("2006-01-02")}}
	c.state.Job.DateProposal = &DateProposal{ID: "counter", Actor: "professional", Status: "pending", Appointment: slot}
	d := DecideAgent(AgentContext{State: c.state, Role: "customer", Mandate: c.m, Now: c.clock()})
	if d.Action == nil || d.Action.Action != "decline-date" {
		t.Fatalf("professional counter reused a rejected day: %s", d.Code)
	}
}
