package workflow

import (
	"errors"
	"strings"
	"time"
)

type ConsumerPolicy struct {
	Kind                   string `json:"kind"`
	Version                string `json:"version"`
	Locale                 string `json:"locale"`
	Title                  string `json:"title"`
	Body                   string `json:"body"`
	RequiresAcknowledgment bool   `json:"requiresAcknowledgment"`
	ReviewerID             string `json:"reviewerId"`
	ReviewRationale        string `json:"reviewRationale"`
	ReviewReference        string `json:"reviewReference"`
	PublishedAt            string `json:"publishedAt"`
}

type PolicyRequirement struct {
	Kind                   string `json:"kind"`
	Version                string `json:"version"`
	RequiresAcknowledgment bool   `json:"requiresAcknowledgment"`
}

type PolicyAcknowledgment struct {
	Kind           string `json:"kind"`
	Version        string `json:"version"`
	ScopeVersion   int    `json:"scopeVersion"`
	AcknowledgedAt string `json:"acknowledgedAt"`
}

// VisitChangeNeedsReview uses the booked visit as the cutoff anchor. Exactly
// 24 hours remains outside the late-change review window.
func VisitChangeNeedsReview(appointment *Appointment, now time.Time) (bool, error) {
	if appointment == nil {
		return false, nil
	}
	start, err := time.Parse(time.RFC3339, appointment.StartUTC)
	if err != nil {
		return false, errors.New("the booked visit time is invalid; contact support to change it")
	}
	return start.Sub(now) < 24*time.Hour, nil
}

func requestWithdrawal(state *State, input Action) error {
	if input.Actor != "customer" || state.Job.ID == "" || state.Cancelled {
		return errors.New("only the customer can request a withdrawal review for this request")
	}
	if state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
		return errors.New("a request is already waiting for operator review")
	}
	note := strings.TrimSpace(input.Value)
	if len(note) < 3 || len(note) > 500 {
		return errors.New("explain the withdrawal request in 3 to 500 characters")
	}
	state.CancellationReview = &CancellationRequest{Kind: "withdrawal", Status: "pending", RequestedBy: "customer", Reason: note, RequestedAt: time.Now().UTC().Format(time.RFC3339)}
	addEvent(&state.Job, "Customer requested a withdrawal review · no cancellation or refund decision has been made", "attention")
	return nil
}

func acknowledgePolicy(state *State, input Action) error {
	p := input.PublishedPolicy
	if input.Actor != "customer" || state.Job.ID == "" || state.Job.WorkStarted || p == nil || p.Kind != input.PolicyKind || p.Version != input.PolicyVersion || p.PublishedAt == "" || p.ReviewerID == "" || len(strings.TrimSpace(p.ReviewRationale)) < 10 || p.ReviewReference == "" {
		return errors.New("an explicit acknowledgment of a published reviewed policy is required before work starts")
	}
	for _, required := range state.Job.PolicyRequirements {
		if required.Kind == p.Kind && required.Version != p.Version {
			return errors.New("acknowledge the policy version recorded with this booking")
		}
	}
	for _, ack := range state.Job.PolicyAcknowledgments {
		if ack.Kind == p.Kind && ack.Version == p.Version && ack.ScopeVersion == state.Job.ScopeVersion {
			return nil
		}
	}
	state.Job.PolicyAcknowledgments = append(state.Job.PolicyAcknowledgments, PolicyAcknowledgment{Kind: p.Kind, Version: p.Version, ScopeVersion: state.Job.ScopeVersion, AcknowledgedAt: time.Now().UTC().Format(time.RFC3339)})
	addEvent(&state.Job, "Customer acknowledged "+p.Kind+" policy "+p.Version+" for the current agreement", "done")
	return nil
}

func ValidateEarlyStartAcknowledgment(job Job) error {
	for _, required := range job.PolicyRequirements {
		if required.Kind != "early_start" || !required.RequiresAcknowledgment {
			continue
		}
		found := false
		for _, ack := range job.PolicyAcknowledgments {
			if ack.Kind == required.Kind && ack.Version == required.Version && ack.ScopeVersion == job.ScopeVersion && ack.AcknowledgedAt != "" {
				found = true
				break
			}
		}
		if !found {
			return errors.New("the customer must acknowledge the booking's published early-start policy before work begins")
		}
	}
	return nil
}
