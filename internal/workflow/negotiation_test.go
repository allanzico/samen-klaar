package workflow

import (
	"testing"
	"time"
)

func TestPriceNegotiationRequiresOtherPartyAndCurrentProposal(t *testing.T) {
	state := NewDemoState("quote-ready")
	if err := Apply(&state, Action{Actor: "customer", Action: "propose-price", PriceAmountMinor: 10000, Value: "My budget is one hundred"}); err != nil {
		t.Fatal(err)
	}
	first := state.Job.PriceProposal.ID
	if state.Job.Quote.TotalMinor != 14700 {
		t.Fatal("unaccepted proposal changed price")
	}
	for _, a := range []Action{
		{Actor: "customer", Action: "accept-price", ProposalID: first},
		{Actor: "customer", Action: "approve-quote"},
		{Actor: "professional", Action: "accept-price", ProposalID: "stale"},
	} {
		if err := Apply(&state, a); err == nil {
			t.Fatal("invalid price decision succeeded")
		}
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "propose-price", PriceAmountMinor: 12000, Value: "I can include the call-out for this total"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "accept-price", ProposalID: first}); err == nil {
		t.Fatal("stale proposal accepted")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "accept-price", ProposalID: state.Job.PriceProposal.ID}); err != nil {
		t.Fatal(err)
	}
	if state.Job.Quote.TotalMinor != 12000 || state.Job.ScopeVersion != 2 || state.Job.QuoteApproved || state.Job.ScopeHistory[0].TotalMinor != 14700 {
		t.Fatal("agreement lost version history or skipped approval")
	}
	var sum int64
	for _, line := range state.Job.Quote.Lines {
		sum += line.AmountMinor
	}
	if sum != state.Job.Quote.TotalMinor {
		t.Fatal("quote lines do not reconcile")
	}
	if err := apply(&state, "approve-quote", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "propose-price", PriceAmountMinor: 13000, Value: "Change agreed price"}); err == nil {
		t.Fatal("approved quote renegotiated without a scope change")
	}
}

func TestDateCounterProposalAndRescheduleRequireAgreement(t *testing.T) {
	state := NewDemoState("quote-ready")
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	slot := &Appointment{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(2 * time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}
	if err := Apply(&state, Action{Actor: "customer", Action: "propose-date", ProposedAppointment: slot}); err == nil {
		t.Fatal("date proposed before plan approval")
	}
	if err := apply(&state, "approve-quote", "customer", ""); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "propose-date", ProposedAppointment: slot}); err != nil {
		t.Fatal(err)
	}
	first := state.Job.DateProposal.ID
	if state.Job.Appointment != nil {
		t.Fatal("unaccepted date booked")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "accept-date", ProposalID: first}); err == nil {
		t.Fatal("self acceptance succeeded")
	}
	slot.StartUTC, slot.EndUTC = start.Add(time.Hour).Format(time.RFC3339), start.Add(3*time.Hour).Format(time.RFC3339)
	if err := Apply(&state, Action{Actor: "professional", Action: "propose-date", ProposedAppointment: slot}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "accept-date", ProposalID: first}); err == nil {
		t.Fatal("stale date accepted")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "accept-date", ProposalID: state.Job.DateProposal.ID}); err != nil {
		t.Fatal(err)
	}
	old := *state.Job.Appointment
	if err := Apply(&state, Action{Actor: "customer", Action: "propose-date", ProposedAppointment: slot}); err != nil {
		t.Fatal(err)
	}
	if *state.Job.Appointment != old {
		t.Fatal("reschedule released previous appointment before agreement")
	}
	if err := apply(&state, "start-job", "professional", ""); err == nil {
		t.Fatal("work started while date decision pending")
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "decline-date", ProposalID: state.Job.DateProposal.ID}); err != nil {
		t.Fatal(err)
	}
	if *state.Job.Appointment != old {
		t.Fatal("declined reschedule changed booked time")
	}
}

func TestProposedDatesRejectExpiredInvalidOrShortVisits(t *testing.T) {
	start := time.Now().UTC().Add(48 * time.Hour)
	for _, slot := range []Appointment{
		{StartUTC: "invalid", EndUTC: start.Format(time.RFC3339), Timezone: "UTC"},
		{StartUTC: start.Add(-72 * time.Hour).Format(time.RFC3339), EndUTC: start.Format(time.RFC3339), Timezone: "UTC"},
		{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(15 * time.Minute).Format(time.RFC3339), Timezone: "UTC"},
		{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(2 * time.Hour).Format(time.RFC3339), Timezone: "Made/up"},
	} {
		if _, err := ValidateProposedAppointment(slot, 90); err == nil {
			t.Fatal("invalid visit accepted")
		}
	}
}

func TestProfessionalCanSubmitNoteWithoutForcedCustomerRequest(t *testing.T) {
	state := NewDemoState("quote-ready")
	state.Job.QuoteApproved, state.Job.WorkStarted = true, true
	state.Job.Evidence = []Evidence{{AssetID: "uploaded", ContentType: "image/png"}}
	if err := apply(&state, "add-evidence", "professional", "Repaired the pipe joint and checked it under running water."); err != nil {
		t.Fatal(err)
	}
	if err := apply(&state, "submit-evidence", "professional", ""); err != nil {
		t.Fatal(err)
	}
	if !state.Job.DetailAdded {
		t.Fatal("submission discarded the completion note")
	}
	if err := apply(&state, "approve-completion", "customer", ""); err != nil {
		t.Fatal(err)
	}
}
