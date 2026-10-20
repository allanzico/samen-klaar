package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func lifecycleState(edit func(*State)) State {
	state := NewState()
	state.WorkspaceID = "workspace"
	edit(&state)
	return state
}

func assignedJob(state *State) {
	state.Intake = &Intake{Step: "matches", Description: "Leaking sink"}
	state.Job.ID, state.Job.ProfessionalAccountID, state.Job.Professional, state.Job.Customer = "job-1", "pro-account", "Noor", "Sam"
	state.Job.ScopeVersion = 1
}

func TestRequestLifecycleMapping(t *testing.T) {
	cases := []struct {
		name                       string
		state                      State
		open, deletable, removable bool
	}{
		{"empty draft", lifecycleState(func(*State) {}), true, true, false},
		{"describing", lifecycleState(func(s *State) { s.Intake = &Intake{Step: "detail", Description: "Leaking sink"} }), true, true, false},
		{"waitlisted without a professional", lifecycleState(func(s *State) { s.Intake = &Intake{Step: "waitlist", Waitlisted: true, Description: "Leaking sink"} }), true, true, false},
		{"matching", lifecycleState(func(s *State) { s.Intake = &Intake{Step: "matches", Description: "Leaking sink"} }), true, true, false},
		{"plan awaiting approval", lifecycleState(assignedJob), true, true, true},
		{"price under negotiation", lifecycleState(func(s *State) {
			assignedJob(s)
			s.Job.PriceProposal = &PriceProposal{ID: "p", Status: "pending", Actor: "professional", AmountMinor: 9000}
		}), true, true, true},
		{"cancelled before approval", lifecycleState(func(s *State) { assignedJob(s); s.Cancelled, s.CancelledBy = true, "customer" }), false, true, true},
		{"cancelled draft", lifecycleState(func(s *State) { s.Cancelled, s.CancelledBy = true, "customer" }), false, true, false},
		{"plan approved", lifecycleState(func(s *State) { assignedJob(s); s.Job.QuoteApproved = true }), false, false, false},
		{"slot awaiting professional confirmation", lifecycleState(func(s *State) {
			assignedJob(s)
			s.Job.QuoteApproved, s.Job.Appointment = true, &Appointment{SlotID: "slot"}
			s.Job.AppointmentConfirmation = &AppointmentConfirmation{Status: "pending"}
		}), false, false, false},
		{"booking-time PayPal hold", lifecycleState(func(s *State) {
			assignedJob(s)
			s.Job.QuoteApproved, s.Job.PaymentAuthorizationTiming, s.Job.PaymentAuthorizedVersion = true, PaymentAuthorizationAtBooking, 1
		}), false, false, false},
		{"hold left on an unapproved record", lifecycleState(func(s *State) { assignedJob(s); s.Job.PaymentAuthorizedVersion = 1 }), false, false, false},
		{"visit booked", lifecycleState(func(s *State) {
			assignedJob(s)
			s.Job.QuoteApproved, s.Job.Appointment = true, &Appointment{SlotID: "slot"}
		}), false, false, false},
		{"work started", lifecycleState(func(s *State) { assignedJob(s); s.Job.QuoteApproved, s.Job.WorkStarted = true, true }), false, false, false},
		{"evidence submitted", lifecycleState(func(s *State) {
			assignedJob(s)
			s.Job.QuoteApproved, s.Job.WorkStarted, s.Job.EvidenceSubmitted = true, true, true
			s.Job.Evidence = []Evidence{{AssetID: "photo"}}
		}), false, false, false},
		{"paid", lifecycleState(func(s *State) { assignedJob(s); s.Job.QuoteApproved, s.Job.Paid = true, true }), false, false, false},
		{"dispute", lifecycleState(func(s *State) { assignedJob(s); s.Job.Dispute = &DisputeCase{Status: "open"} }), false, false, false},
		{"late cancellation under review", lifecycleState(func(s *State) {
			assignedJob(s)
			s.CancellationReview = &CancellationRequest{Status: "pending"}
		}), false, false, false},
		{"cancelled after approval", lifecycleState(func(s *State) {
			assignedJob(s)
			s.Job.QuoteApproved, s.Cancelled, s.CancelledBy = true, true, "customer"
		}), false, false, false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := RequestOpen(item.state); got != item.open {
				t.Errorf("RequestOpen = %v, want %v", got, item.open)
			}
			if got := CustomerCanDelete(item.state); got != item.deletable {
				t.Errorf("CustomerCanDelete = %v, want %v", got, item.deletable)
			}
			if got := ProfessionalCanRemove(item.state); got != item.removable {
				t.Errorf("ProfessionalCanRemove = %v, want %v", got, item.removable)
			}
			summary := SummarizeWorkspace(item.state, "customer", time.Time{})
			if summary.CanDelete != item.deletable {
				t.Errorf("customer summary CanDelete = %v, want %v", summary.CanDelete, item.deletable)
			}
		})
	}
}

func TestMemoryStoreCapsOpenRequestsAndFreesPlacesOnDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(NewState())
	ids := []string{}
	for range MaxOpenRequests {
		state, err := store.CreateWorkspace(ctx, "customer")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, state.WorkspaceID)
	}
	if _, err := store.CreateWorkspace(ctx, "customer"); !errors.Is(err, ErrOpenRequestLimit) {
		t.Fatalf("fourth open request: %v, want ErrOpenRequestLimit", err)
	}
	if _, err := store.CreateWorkspace(ctx, "someone-else"); err != nil {
		t.Fatalf("the limit is per customer: %v", err)
	}
	if count, _ := store.OpenRequestCount(ctx, "customer"); count != MaxOpenRequests {
		t.Fatalf("open count = %d", count)
	}

	// An approved request stops counting but can no longer be deleted.
	approved := store.states[ids[0]]
	assignedJob(&approved)
	approved.Job.QuoteApproved = true
	store.states[ids[0]] = approved
	if _, err := store.RemoveRequest(ctx, "customer", "customer", ids[0]); !errors.Is(err, ErrRequestNotDeletable) {
		t.Fatalf("delete approved request: %v", err)
	}
	if _, ok := store.states[ids[0]]; !ok {
		t.Fatal("approved request was deleted")
	}
	if _, err := store.CreateWorkspace(ctx, "customer"); err != nil {
		t.Fatalf("approval should free a place: %v", err)
	}
	if _, err := store.CreateWorkspace(ctx, "customer"); !errors.Is(err, ErrOpenRequestLimit) {
		t.Fatalf("limit again: %v", err)
	}

	if _, err := store.RemoveRequest(ctx, "intruder", "customer", ids[1]); err == nil {
		t.Fatal("another account deleted the request")
	}
	if status, err := store.RemoveRequest(ctx, "customer", "customer", ids[1]); err != nil || status != "deleted" {
		t.Fatalf("delete draft: %q %v", status, err)
	}
	if _, err := store.CreateWorkspace(ctx, "customer"); err != nil {
		t.Fatalf("deleting should free a place: %v", err)
	}
}

func TestMemoryStoreProfessionalRemovalDeclinesWithoutDeletingCustomerData(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(NewState())
	created, err := store.CreateWorkspace(ctx, "customer")
	if err != nil {
		t.Fatal(err)
	}
	state := store.states[created.WorkspaceID]
	assignedJob(&state)
	store.states[created.WorkspaceID] = state

	if _, err := store.RemoveRequest(ctx, "other-pro", "professional", created.WorkspaceID); err == nil {
		t.Fatal("an unassigned professional removed the request")
	}
	if status, err := store.RemoveRequest(ctx, "pro-account", "professional", created.WorkspaceID); err != nil || status != "removed" {
		t.Fatalf("remove: %q %v", status, err)
	}
	kept, ok := store.states[created.WorkspaceID]
	if !ok || !kept.Cancelled || kept.CancelledBy != "professional" || kept.Intake == nil || kept.Intake.Description != "Leaking sink" {
		t.Fatalf("customer request should be declined and kept: %+v", kept)
	}
	if jobs, _ := store.ListWorkspaces(ctx, "pro-account", "professional"); len(jobs) != 0 {
		t.Fatalf("removed job still listed for the professional: %+v", jobs)
	}
	if requests, _ := store.ListWorkspaces(ctx, "customer", "customer"); len(requests) != 1 || requests[0].Status != "cancelled" {
		t.Fatalf("customer list changed: %+v", requests)
	}

	// The customer can still look for someone else, which reopens the request under the limit.
	for range MaxOpenRequests {
		if _, err := store.CreateWorkspace(ctx, "customer"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ApplyWorkspace(ctx, "customer", "customer", created.WorkspaceID, Action{Actor: "customer", Action: "return-to-matches"}); !errors.Is(err, ErrOpenRequestLimit) {
		t.Fatalf("reopening past the limit: %v", err)
	}
}

func TestProfessionalCannotRemoveApprovedJob(t *testing.T) {
	state := lifecycleState(func(s *State) { assignedJob(s); s.Job.QuoteApproved = true })
	if err := Apply(&state, Action{Actor: "professional", Action: "remove-request"}); !errors.Is(err, ErrRequestNotRemovable) {
		t.Fatalf("remove approved job: %v", err)
	}
	if state.Cancelled {
		t.Fatal("approved job was cancelled")
	}
	draft := lifecycleState(assignedJob)
	if err := Apply(&draft, Action{Actor: "customer", Action: "remove-request"}); !errors.Is(err, ErrRequestNotRemovable) {
		t.Fatalf("customer used the professional removal: %v", err)
	}
}
