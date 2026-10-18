package workflow

import (
	"testing"
	"time"
)

func TestLateRescheduleKeepsVisitForReviewForEitherInitiator(t *testing.T) {
	for _, initiator := range []string{"customer", "professional"} {
		t.Run(initiator, func(t *testing.T) {
			state := NewState()
			now := time.Now().UTC()
			state.Job = Job{ID: "job", ScopeVersion: 1, QuoteApproved: true, Quote: Quote{DurationMinutes: 60}, Appointment: &Appointment{SlotID: "old-visit", StartUTC: now.Add(23 * time.Hour).Format(time.RFC3339), EndUTC: now.Add(24 * time.Hour).Format(time.RFC3339), Timezone: "UTC"}}
			other := "customer"
			if initiator == other {
				other = "professional"
			}
			proposal := Appointment{StartUTC: now.Add(72 * time.Hour).Format(time.RFC3339), EndUTC: now.Add(73 * time.Hour).Format(time.RFC3339), Timezone: "UTC"}
			if err := Apply(&state, Action{Actor: initiator, Action: "propose-date", ProposedAppointment: &proposal, Value: "Please move the visit"}); err != nil {
				t.Fatal(err)
			}
			if err := Apply(&state, Action{Actor: other, Action: "accept-date", ProposalID: state.Job.DateProposal.ID}); err != nil {
				t.Fatal(err)
			}
			if state.CancellationReview == nil || state.CancellationReview.Kind != "late-reschedule" || state.CancellationReview.RequestedBy != initiator || state.Job.Appointment.SlotID != "old-visit" || state.Job.DateProposal.Status != "review_pending" {
				t.Fatalf("late reschedule changed the visit without review: %+v", state)
			}
			if err := Apply(&state, Action{Actor: "professional", Action: "start-job"}); err == nil {
				t.Fatal("work started while late visit change awaits review")
			}
		})
	}
}

func TestWithdrawalRequestAlwaysWaitsForHumanDecision(t *testing.T) {
	for _, active := range []bool{false, true} {
		state := NewState()
		state.Job = Job{ID: "job", ScopeVersion: 1, WorkStarted: active, Paid: active}
		if err := Apply(&state, Action{Actor: "customer", Action: "request-withdrawal", Value: "Please review withdrawing this request"}); err != nil {
			t.Fatal(err)
		}
		if state.Cancelled || state.CancellationReview == nil || state.CancellationReview.Kind != "withdrawal" || state.CancellationReview.Status != "pending" || state.Job.Paid != active {
			t.Fatalf("withdrawal made an automatic eligibility or refund decision: %+v", state)
		}
	}
}

func TestEarlyStartRequiresAcknowledgmentOfSnapshottedPolicy(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", ScopeVersion: 1, QuoteApproved: true, Appointment: &Appointment{SlotID: "visit"}, AppointmentConfirmation: &AppointmentConfirmation{Status: "confirmed"}, PolicyRequirements: []PolicyRequirement{{Kind: "early_start", Version: "reviewed-v1", RequiresAcknowledgment: true}}}
	if err := Apply(&state, Action{Actor: "professional", Action: "start-job"}); err == nil {
		t.Fatal("work started without acknowledgment of the booking's early-start policy")
	}
	policy := ConsumerPolicy{Kind: "early_start", Version: "reviewed-v1", ReviewerID: "reviewer", ReviewRationale: "Reviewed externally for this pilot", ReviewReference: "decision-1", PublishedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := Apply(&state, Action{Actor: "customer", Action: "acknowledge-policy", PolicyKind: "early_start", PolicyVersion: "reviewed-v1", PublishedPolicy: &policy}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&state, Action{Actor: "professional", Action: "start-job"}); err != nil {
		t.Fatal(err)
	}
	if len(state.Job.PolicyAcknowledgments) != 1 || state.Job.PolicyAcknowledgments[0].ScopeVersion != 1 {
		t.Fatalf("policy acknowledgment was not bound to the agreement: %+v", state.Job.PolicyAcknowledgments)
	}
}

func TestLateVisitChangeBoundaryUsesStrictlyLessThan24Hours(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	visit := &Appointment{StartUTC: now.Add(24 * time.Hour).Format(time.RFC3339)}
	for _, test := range []struct {
		offset time.Duration
		late   bool
	}{{-time.Nanosecond, false}, {0, false}, {time.Nanosecond, true}} {
		late, err := VisitChangeNeedsReview(visit, now.Add(test.offset))
		if err != nil || late != test.late {
			t.Fatalf("24h boundary offset=%v late=%v err=%v", test.offset, late, err)
		}
	}
}
