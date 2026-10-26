package workflow

import (
	"testing"
	"time"
)

func autonomousFixture(t *testing.T) State {
	t.Helper()
	state := NewState()
	state.Job = Job{ID: "autonomy-job", ProfessionalAccountID: "professional", ProfessionalProfileID: "service", ScopeVersion: 1, Quote: Quote{Currency: "EUR", TotalMinor: 12500, Lines: []QuoteLine{{Label: "Repair the sink"}}}, BookingAutonomyRequired: true}
	if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "automatic|" + BookingAutonomyKey(state.Job)}); err != nil {
		t.Fatal(err)
	}
	state.Job.QuoteApproved = true
	state.Job.WorkStarted = true
	state.Job.EvidenceSubmitted = true
	state.Job.DetailAdded = true
	state.Job.Evidence = []Evidence{{AssetID: "photo", ContentType: "image/png", Kind: "photo"}, {Kind: "note", Description: "The seal was replaced and water checked."}}
	state.Job.EvidenceReview = &EvidenceReview{Summary: "The repair is documented.", ReviewedAt: time.Now().UTC().Format(time.RFC3339), Coverage: []EvidenceFinding{{ScopeItem: "work-1", Coverage: "documented", EvidenceIDs: []string{"photo"}, Observation: "The repaired connection is visible."}}}
	BindEvidenceReview(state.Job, state.Job.EvidenceReview)
	return state
}

func TestUnclearLegacyCompletionReturnsToProfessionalWithoutCustomerApprovalTask(t *testing.T) {
	state := autonomousFixture(t)
	state.Job.EvidenceReview.Coverage[0].Coverage = "unclear"
	decision := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()})
	if decision.Kind != AgentAct || decision.Action == nil || decision.Action.Action != "request-completion-correction" {
		t.Fatalf("unclear evidence was left with the customer: %+v", decision)
	}
	decision.Action.Actor, decision.Action.ByAgent = "customer", true
	if err := Apply(&state, *decision.Action); err != nil {
		t.Fatal(err)
	}
	if state.Job.EvidenceSubmitted || !state.Job.CompletionRevision || state.Job.CompletionCheck == nil || state.Job.CompletionApproved || state.Job.Paid {
		t.Fatal("correction did not return safely to the professional")
	}
	if visible := ConversationForRole(state, "customer"); visible.Job.CompletionCheck != nil || requestStatusCode(visible, "customer") != "in_progress" {
		t.Fatal("customer was given a private correction or an approval task")
	}
}

func TestBookingAutonomyRequiresCustomerAndCurrentPrice(t *testing.T) {
	for _, change := range []string{"assistant", "professional", "price", "scope", "profile"} {
		t.Run(change, func(t *testing.T) {
			state := autonomousFixture(t)
			state.Job.WorkStarted = false
			state.Job.EvidenceSubmitted = false
			input := Action{Actor: "customer", Action: "set-booking-autonomy", Value: "automatic|" + BookingAutonomyKey(state.Job)}
			switch change {
			case "assistant":
				input.ByAgent = true
			case "professional":
				input.Actor = "professional"
			case "price":
				state.Job.Quote.TotalMinor++
			case "scope":
				state.Job.ScopeVersion++
			case "profile":
				state.Job.ProfessionalProfileID = "other"
			}
			if err := Apply(&state, input); err == nil {
				t.Fatal("stale or unauthorized financial delegation accepted")
			}
		})
	}
	state := NewDemoState("quote-ready")
	state.Job.BookingAutonomyRequired = true
	if err := Apply(&state, Action{Actor: "customer", Action: "approve-and-book", Value: state.AvailableSlots[0].SlotID}); err == nil {
		t.Fatal("booked without an autonomy decision")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "approve-and-book", Value: state.AvailableSlots[0].SlotID}); err != nil {
		t.Fatal(err)
	}
	if state.Job.PaymentAuthorizationTiming != PaymentAuthorizationAfterCompletion {
		t.Fatal("manual choice forced a PayPal hold")
	}
}

func TestAutonomyApprovesOnlyCurrentDocumentedEvidence(t *testing.T) {
	for _, change := range []string{"valid", "unclear", "missing-reference", "foreign-reference", "new-file", "scope", "round", "changed-price", "dispute", "revoked", "paused"} {
		t.Run(change, func(t *testing.T) {
			state := autonomousFixture(t)
			mandate := DefaultMandate("customer")
			switch change {
			case "unclear":
				state.Job.EvidenceReview.Coverage[0].Coverage = "unclear"
			case "missing-reference":
				state.Job.EvidenceReview.Coverage[0].EvidenceIDs = nil
			case "foreign-reference":
				state.Job.EvidenceReview.Coverage[0].EvidenceIDs = []string{"other-job-photo"}
			case "new-file":
				state.Job.Evidence = append(state.Job.Evidence, Evidence{AssetID: "new", ContentType: "image/png"})
			case "scope":
				state.Job.ScopeVersion++
			case "round":
				state.Job.CompletionRound = 2
			case "changed-price":
				state.Job.Quote.TotalMinor++
			case "dispute":
				state.Job.Dispute = &DisputeCase{Status: "open"}
			case "revoked":
				if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
					t.Fatal(err)
				}
			case "paused":
				mandate.Enabled = false
			}
			d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: mandate, Now: time.Now()})
			if change == "valid" {
				if d.Code != "approve-work" || d.Action.AutonomyVersion == 0 {
					t.Fatalf("decision=%+v", d)
				}
			} else if d.Action != nil && (d.Action.Action == "approve-completion" || d.Action.Action == "approve-payment" || d.Action.Action == "release-authorized-payment") {
				t.Fatalf("unsafe approval for %s: %+v", change, d)
			}
		})
	}
}

func TestRevocationInvalidatesPreparedApprovalAndRelease(t *testing.T) {
	state := autonomousFixture(t)
	ctx := AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()}
	d := DecideAgent(ctx)
	input := *d.Action
	input.Actor = "customer"
	input.ByAgent = true
	if err := Apply(&state, input); err != nil {
		t.Fatal(err)
	}
	ctx.State = state
	d = DecideAgent(ctx)
	if d.Action == nil || d.Action.Action != "approve-payment" {
		t.Fatalf("expected separate payment approval, got %+v", d)
	}
	prepared := *d.Action
	prepared.Actor = "customer"
	prepared.ByAgent = true
	if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, prepared); err == nil {
		t.Fatal("approval survived revocation")
	}
	if state.Job.CompletionApproved || state.Job.PaymentApprovedVersion != 0 {
		t.Fatal("revocation retained automatic approvals")
	}
}

func TestAutonomousPhotoReviewPrecedesApprovalAndPaymentIsSeparate(t *testing.T) {
	state := autonomousFixture(t)
	state.Job.EvidenceReview = nil
	c := AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()}
	if d := DecideAgent(c); d.Action == nil || d.Action.Action != "review-completion-evidence" {
		t.Fatalf("did not check photos first: %+v", d)
	}
	if err := Apply(&state, Action{Actor: "customer", ByAgent: true, Action: "approve-payment"}); err == nil {
		t.Fatal("assistant approved payment without delegated review")
	}
}

func TestAssistantCanAskButOnlyCustomersExplicitChoiceGrantsAutonomy(t *testing.T) {
	state := autonomousFixture(t)
	state.Job.BookingAutonomy = nil
	state.Job.WorkStarted = false
	state.Job.EvidenceSubmitted = false
	state.Job.DetailAdded = false
	state.Job.Evidence = nil
	state.Job.EvidenceReview = nil
	state.Intake = &Intake{Messages: []Message{{Role: "customer", Text: "Repair the sink"}}}
	c := AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()}
	d := DecideAgent(c)
	if d.Action == nil || d.Code != "ask-booking-autonomy" {
		t.Fatalf("autonomy question=%+v", d)
	}
	question := *d.Action
	question.Actor = "customer"
	question.ByAgent = true
	if err := Apply(&state, question); err != nil {
		t.Fatal(err)
	}
	if state.Job.BookingAutonomy != nil {
		t.Fatal("asking granted permission")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "agent-reply", Value: "yes", ChoiceID: "yes"}); err == nil {
		t.Fatal("generic yes granted financial delegation")
	}
	choice := "automatic|" + BookingAutonomyKey(state.Job)
	if err := Apply(&state, Action{Actor: "customer", ByAgent: true, Action: "agent-reply", Value: "Yes", ChoiceID: choice}); err == nil {
		t.Fatal("assistant granted its own permission")
	}
	if err := Apply(&state, Action{Actor: "customer", Action: "agent-reply", Value: "Review and release for me", ChoiceID: choice}); err != nil {
		t.Fatal(err)
	}
	if !BookingAutonomyCurrent(state.Job) || state.Job.PaymentAuthorizationTiming != PaymentAuthorizationAtBooking {
		t.Fatal("explicit booking choice did not bind the hold and permission")
	}
	if state.Job.QuoteApproved && state.Job.Appointment != nil {
		t.Fatal("permission itself booked a visit")
	}
}

func TestManualBookingChoiceOverridesLegacyAutomaticWorkPreference(t *testing.T) {
	state := autonomousFixture(t)
	if err := Apply(&state, Action{Actor: "customer", Action: "set-booking-autonomy", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	mandate := DefaultMandate("customer")
	mandate.AutoApproveWork = true
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: mandate, Now: time.Now()})
	if d.Action != nil && d.Action.Action == "approve-completion" {
		t.Fatal("a general preference overrode this booking's manual choice")
	}
}

func TestProposedFirstVisitAsksForAutonomyBeforeBookingConfirmation(t *testing.T) {
	state := autonomousFixture(t)
	state.Job.BookingAutonomy = nil
	state.Job.WorkStarted = false
	state.Job.EvidenceSubmitted = false
	state.Intake = &Intake{Messages: []Message{{Role: "customer", Text: "Repair the sink"}}}
	state.Job.DateProposal = &DateProposal{ID: "proposal", Actor: "professional", Status: "pending", Appointment: agentSlot("", time.Now().Add(7*24*time.Hour).UTC().Format(time.RFC3339), time.Now().Add(7*24*time.Hour+time.Hour).UTC().Format(time.RFC3339))}
	d := DecideAgent(AgentContext{State: state, Role: "customer", Mandate: DefaultMandate("customer"), Now: time.Now()})
	if d.Code != "ask-booking-autonomy" {
		t.Fatalf("first visit skipped permission choice: %+v", d)
	}
}
