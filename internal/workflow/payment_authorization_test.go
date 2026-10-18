package workflow

import (
	"testing"
	"time"
)

func TestBookingTimeAuthorizationIsRequiredBeforeProfessionalConfirmation(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 1, Quote: Quote{Currency: "EUR", TotalMinor: 12_500}, Timeline: []Event{}}
	state.AvailableSlots = []Appointment{{
		SlotID: "slot", Timezone: "Europe/Amsterdam",
		StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		EndUTC:   time.Now().Add(49 * time.Hour).UTC().Format(time.RFC3339),
	}}
	if err := Apply(&state, Action{
		Action: "approve-and-book", Actor: "customer", Value: "slot",
		PaymentAuthorizationTiming: PaymentAuthorizationAtBooking,
	}); err != nil {
		t.Fatal(err)
	}
	if state.Job.PaymentAuthorizationTiming != PaymentAuthorizationAtBooking {
		t.Fatalf("authorization timing = %q", state.Job.PaymentAuthorizationTiming)
	}
	if err := Apply(&state, Action{Action: "confirm-appointment", Actor: "professional"}); err == nil {
		t.Fatal("professional confirmed a booking before PayPal authorization")
	}
	if err := Apply(&state, Action{Action: "record-payment-authorization", Actor: "customer"}); err == nil {
		t.Fatal("unverified client action recorded a PayPal authorization")
	}
	deferred := state
	if err := Apply(&deferred, Action{Action: "defer-payment-authorization", Actor: "customer"}); err != nil {
		t.Fatalf("customer could not switch to after-completion authorization before PayPal succeeds: %v", err)
	}
	if deferred.Job.PaymentAuthorizationTiming != PaymentAuthorizationAfterCompletion {
		t.Fatal("payment timing fallback was not saved")
	}
	if err := Apply(&state, Action{Action: "record-payment-authorization", Actor: "customer", PayPalAuthorized: true, PaymentAuthorizationScopeVersion: state.Job.ScopeVersion}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Action: "defer-payment-authorization", Actor: "customer"}); err == nil {
		t.Fatal("customer changed timing after booking-time authorization succeeded")
	}
	if state.Job.PaymentAuthorizedVersion != state.Job.ScopeVersion {
		t.Fatal("PayPal authorization was not recorded for the current scope")
	}
	if err := ValidatePaymentApproval(state.Job); err == nil {
		t.Fatal("booking-time authorization bypassed completion and payment approval")
	}
	if err := Apply(&state, Action{Action: "confirm-appointment", Actor: "professional"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Action: "start-job", Actor: "professional"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePayPalAuthorization(state); err == nil {
		t.Fatal("booking-time authorization allowed new checkout after work started without payment approval")
	}
	state.Job.CompletionApproved = true
	state.Job.PaymentApprovedVersion = state.Job.ScopeVersion
	if err := ValidatePaymentApproval(state.Job); err != nil {
		t.Fatalf("separate completion and payment approvals should permit capture: %v", err)
	}
}

func TestNegotiatedVisitKeepsCustomerAuthorizationChoice(t *testing.T) {
	for _, proposer := range []string{"customer", "professional"} {
		t.Run(proposer, func(t *testing.T) {
			state := NewDemoState("quote-ready")
			if err := Apply(&state, Action{Actor: "customer", Action: "approve-quote", PaymentAuthorizationTiming: PaymentAuthorizationAtBooking}); err != nil {
				t.Fatal(err)
			}
			if state.Job.PaymentAuthorizationTiming != PaymentAuthorizationAtBooking {
				t.Fatal("plan approval discarded the customer's authorization choice")
			}
			start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
			slot := Appointment{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(2 * time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}
			if err := Apply(&state, Action{Actor: proposer, Action: "propose-date", ProposedAppointment: &slot}); err != nil {
				t.Fatal(err)
			}
			acceptor := "customer"
			if proposer == "customer" {
				acceptor = "professional"
			}
			if err := Apply(&state, Action{Actor: acceptor, Action: "accept-date", ProposalID: state.Job.DateProposal.ID}); err != nil {
				t.Fatal(err)
			}
			if state.Job.AppointmentConfirmation == nil || state.Job.AppointmentConfirmation.Status != "pending" {
				t.Fatal("negotiated visit was confirmed before the chosen authorization succeeded")
			}
			if err := Apply(&state, Action{Actor: "professional", Action: "confirm-appointment"}); err == nil {
				t.Fatal("professional confirmed without authorization")
			}
			if err := ValidatePayPalAuthorization(state); err != nil {
				t.Fatal(err)
			}
			if err := Apply(&state, Action{Actor: "customer", Action: "record-payment-authorization", PayPalAuthorized: true, PaymentAuthorizationScopeVersion: state.Job.ScopeVersion}); err != nil {
				t.Fatal(err)
			}
			if err := Apply(&state, Action{Actor: "professional", Action: "confirm-appointment"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveredAuthorizationCannotApproveAnotherScope(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 1, QuoteApproved: true, Appointment: &Appointment{}, PaymentAuthorizationTiming: PaymentAuthorizationAtBooking}
	if err := Apply(&state, Action{Actor: "customer", Action: "record-payment-authorization", PayPalAuthorized: true, PaymentAuthorizationScopeVersion: state.Job.ScopeVersion + 1}); err == nil {
		t.Fatal("authorization for a different scope was recorded")
	}
}

func TestBookingWithoutAuthorizationChoiceKeepsPayAfterCompletionDefault(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 1, Quote: Quote{Currency: "EUR", TotalMinor: 12_500}, Timeline: []Event{}}
	state.AvailableSlots = []Appointment{{
		SlotID: "slot", Timezone: "Europe/Amsterdam",
		StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		EndUTC:   time.Now().Add(49 * time.Hour).UTC().Format(time.RFC3339),
	}}
	if err := Apply(&state, Action{Action: "approve-and-book", Actor: "customer", Value: "slot"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.PaymentAuthorizationTiming != PaymentAuthorizationAfterCompletion {
		t.Fatalf("legacy/default authorization timing = %q", state.Job.PaymentAuthorizationTiming)
	}
	if err := Apply(&state, Action{Action: "confirm-appointment", Actor: "professional"}); err != nil {
		t.Fatalf("existing later-payment flow should not require booking-time authorization: %v", err)
	}
}
