package workflow

import (
	"testing"
	"time"
)

func TestSavedPaymentChoiceSurvivesApprovalAndCannotBeChangedByAssistant(t *testing.T) {
	for _, action := range []string{"approve-quote", "approve-and-book"} {
		t.Run(action, func(t *testing.T) {
			state := NewDemoState("quote-ready")
			if err := Apply(&state, Action{Actor: "customer", Action: "set-payment-timing", PaymentAuthorizationTiming: PaymentAuthorizationAtBooking}); err != nil {
				t.Fatal(err)
			}
			if state.Job.QuoteApproved || state.Job.PaymentAuthorizedVersion != 0 {
				t.Fatal("saving a preference approved the plan or claimed a hold")
			}
			input := Action{Actor: "customer", Action: action}
			if action == "approve-and-book" {
				input.Value = state.AvailableSlots[0].SlotID
			}
			input.ByAgent, input.PaymentAuthorizationTiming = true, PaymentAuthorizationAfterCompletion
			if err := Apply(&state, input); err == nil {
				t.Fatal("assistant replaced the customer's hold choice")
			}
			input.PaymentAuthorizationTiming = ""
			if err := Apply(&state, input); err != nil {
				t.Fatal(err)
			}
			if state.Job.PaymentAuthorizationTiming != PaymentAuthorizationAtBooking {
				t.Fatal("approval forgot the saved choice")
			}
		})
	}
}

func TestPaymentChoiceRequiresCustomerAndCanRecoverAnUnstartedVisit(t *testing.T) {
	for _, input := range []Action{
		{Actor: "professional", Action: "set-payment-timing", PaymentAuthorizationTiming: PaymentAuthorizationAtBooking},
		{Actor: "customer", Action: "set-payment-timing", ByAgent: true, PaymentAuthorizationTiming: PaymentAuthorizationAtBooking},
		{Actor: "customer", Action: "set-payment-timing", PaymentAuthorizationTiming: "invalid"},
	} {
		state := NewDemoState("quote-ready")
		if err := Apply(&state, input); err == nil {
			t.Fatalf("unsafe choice accepted: %+v", input)
		}
	}
	state := NewDemoState("quote-ready")
	if err := Apply(&state, Action{Actor: "customer", Action: "approve-and-book", Value: state.AvailableSlots[0].SlotID}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "confirm-appointment"}); err != nil {
		t.Fatal(err)
	}
	slot := state.Job.Appointment.SlotID
	if err := Apply(&state, Action{Actor: "customer", Action: "set-payment-timing", PaymentAuthorizationTiming: PaymentAuthorizationAtBooking}); err != nil {
		t.Fatal(err)
	}
	if state.Job.PaymentAuthorizedVersion != 0 || state.Job.Appointment.SlotID != slot || state.Job.AppointmentConfirmation.Status != "pending" {
		t.Fatal("recovery replaced the visit or claimed an unverified hold")
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "confirm-appointment"}); err == nil {
		t.Fatal("recovered choice bypassed authorization")
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "start-job"}); err == nil {
		t.Fatal("work started before authorization")
	}
}

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
