package workflow

import (
	"testing"
	"time"
)

func TestSummarizeWorkspaceDisplayFields(t *testing.T) {
	state := State{WorkspaceID: "w1", Job: Job{
		ID: "j1", Title: "Dripping tap", Category: "Plumbing", Customer: "Noor Bakker", Professional: "Daan Mulder",
		Quote:                   Quote{Currency: "EUR", TotalMinor: 9500},
		Appointment:             &Appointment{StartUTC: "2026-10-19T07:00:00Z", EndUTC: "2026-10-19T08:00:00Z", Timezone: "Europe/Amsterdam"},
		AppointmentConfirmation: &AppointmentConfirmation{Status: "pending", DueAt: "2026-10-16T10:02:00Z"},
	}}
	customer := SummarizeWorkspace(state, "customer", time.Time{})
	if customer.Counterpart != "Daan Mulder" || customer.TotalMinor != 9500 || customer.Currency != "EUR" || customer.VisitStartUTC == "" || customer.ConfirmDueAt == "" {
		t.Fatalf("customer summary missing display fields: %+v", customer)
	}
	if pro := SummarizeWorkspace(state, "professional", time.Time{}); pro.Counterpart != "Noor Bakker" {
		t.Fatalf("professional counterpart = %q", pro.Counterpart)
	}
	draft := SummarizeWorkspace(State{WorkspaceID: "w2", Intake: &Intake{Category: "Carpentry", Description: "Door sticks"}}, "customer", time.Time{})
	if draft.Counterpart != "" || draft.TotalMinor != 0 || draft.Category != "Carpentry" {
		t.Fatalf("draft summary leaked job fields: %+v", draft)
	}
}
