package workflow

import "testing"

func TestVerifiedJobReviewRequiresPaidCompletionAndAllowsOneProfessionalResponse(t *testing.T) {
	state := NewState()
	state.Job = Job{ID: "job", Paid: true, CompletionApproved: true, ScopeVersion: 1, PaymentApprovedVersion: 1}
	for _, rating := range []int{0, 6} {
		if err := Apply(&state, Action{Action: "submit-review", Actor: "customer", Rating: rating, Value: "Good work"}); err == nil {
			t.Fatalf("invalid rating %d was accepted", rating)
		}
	}
	if err := Apply(&state, Action{Action: "submit-review", Actor: "professional", Rating: 5}); err == nil {
		t.Fatal("professional submitted a customer review")
	}
	if err := Apply(&state, Action{Action: "submit-review", Actor: "customer", Rating: 5, Value: "Good work"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.Review == nil || state.Job.Review.Rating != 5 || state.Job.Review.Comment != "Good work" || state.Job.Review.CreatedAt == "" {
		t.Fatal("verified review was not persisted on the job")
	}
	if err := Apply(&state, Action{Action: "respond-to-review", Actor: "customer", Value: "Thank you"}); err == nil {
		t.Fatal("customer posted a professional response")
	}
	if err := Apply(&state, Action{Action: "respond-to-review", Actor: "professional", Value: "Thank you for the feedback"}); err != nil {
		t.Fatal(err)
	}
	if state.Job.Review.ProfessionalResponse != "Thank you for the feedback" || state.Job.Review.RespondedAt == "" {
		t.Fatal("professional response was not persisted")
	}
	if err := Apply(&state, Action{Action: "respond-to-review", Actor: "professional", Value: "A second reply"}); err == nil {
		t.Fatal("professional posted more than one response")
	}
	if err := Apply(&state, Action{Action: "submit-review", Actor: "customer", Rating: 4}); err == nil {
		t.Fatal("customer changed the one-time review")
	}
}

func TestVerifiedJobReviewBlocksUnpaidAndRefundedJobs(t *testing.T) {
	for _, test := range []struct {
		name string
		job  Job
	}{
		{name: "unpaid", job: Job{ID: "job", CompletionApproved: true}},
		{name: "refunded", job: Job{ID: "job", Paid: true, PaymentRefunded: true, CompletionApproved: true}},
		{name: "not complete", job: Job{ID: "job", Paid: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewState()
			state.Job = test.job
			if err := Apply(&state, Action{Action: "submit-review", Actor: "customer", Rating: 5}); err == nil {
				t.Fatal("review was accepted outside a completed and paid job")
			}
		})
	}
}
