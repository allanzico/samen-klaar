package workflow

import (
	"testing"
	"time"
)

func TestAppointmentConfirmationDeadlineUsesLocalBusinessHours(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		from string
		want string
	}{
		{"within business hours", "2026-10-13T10:00:00+02:00", "2026-10-13T12:00:00+02:00"},
		{"before opening", "2026-10-13T07:00:00+02:00", "2026-10-13T11:00:00+02:00"},
		{"after closing", "2026-10-13T18:00:00+02:00", "2026-10-14T11:00:00+02:00"},
		{"weekend", "2026-10-17T12:00:00+02:00", "2026-10-19T11:00:00+02:00"},
		{"crosses weekend", "2026-10-16T16:00:00+02:00", "2026-10-19T10:00:00+02:00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			from, err := time.Parse(time.RFC3339, test.from)
			if err != nil {
				t.Fatal(err)
			}
			deadline, err := AppointmentConfirmationDeadline(from, "Europe/Amsterdam")
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339, test.want)
			if err != nil {
				t.Fatal(err)
			}
			if !deadline.Equal(want) || deadline.In(zone).Weekday() == time.Saturday || deadline.In(zone).Weekday() == time.Sunday {
				t.Fatalf("deadline = %s, want %s", deadline.In(zone), want.In(zone))
			}
		})
	}
}

func TestCustomerBookingWaitsForProfessionalConfirmation(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", QuoteApproved: true, Quote: Quote{DurationMinutes: 60}, Timeline: []Event{}}
	state.AvailableSlots = []Appointment{{SlotID: "slot", StartsAt: "Tomorrow", Timezone: "Europe/Amsterdam", StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339), EndUTC: time.Now().Add(49 * time.Hour).UTC().Format(time.RFC3339)}}
	if err := Apply(&state, Action{Action: "book-appointment", Actor: "customer", Value: "slot"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.AppointmentConfirmation == nil || state.Job.AppointmentConfirmation.Status != "pending" || state.Job.Appointment == nil {
		t.Fatal("customer booking was not saved as awaiting professional confirmation")
	}
	if err := Apply(&state, Action{Action: "start-job", Actor: "professional"}); err == nil {
		t.Fatal("professional started work before confirming the appointment")
	}
	if err := Apply(&state, Action{Action: "confirm-appointment", Actor: "professional"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.AppointmentConfirmation.Status != "confirmed" {
		t.Fatal("professional confirmation was not recorded")
	}
}

func TestProfessionalDeclineReleasesAppointmentInWorkflow(t *testing.T) {
	state := NewState()
	state.Job = Job{
		ID: "job", QuoteApproved: true,
		Appointment:             &Appointment{SlotID: "slot", StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)},
		AppointmentConfirmation: &AppointmentConfirmation{Status: "pending", DueAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
	}
	if err := Apply(&state, Action{Action: "decline-appointment", Actor: "professional"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.Appointment != nil || state.Job.AppointmentConfirmation.Status != "declined" {
		t.Fatal("declined visit remains actionable")
	}
}
