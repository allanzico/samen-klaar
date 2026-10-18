package postgres

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type coordinationFixture struct {
	mu        sync.Mutex
	requests  []workflow.CoordinationRequest
	uncertain bool
}

func (f *coordinationFixture) Coordinate(_ context.Context, input workflow.CoordinationRequest) (workflow.CoordinationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, input)
	if f.uncertain {
		return workflow.CoordinationResponse{NeedsOperator: true, Reason: "The supplied explanation does not establish what remains incomplete.", ProposedAction: "operator-review"}, nil
	}
	if input.Trigger == "post-booking-scope" {
		if input.Reply != "" {
			return workflow.CoordinationResponse{Summary: "The professional reports that the cabinet was outside the agreed valve replacement. The customer must decide any separate scope proposal.", ProposedAction: "review-scope"}, nil
		}
		return workflow.CoordinationResponse{Question: "How does the cabinet request relate to the agreed valve replacement, and which work remains outside that scope?", ProposedAction: "review-scope"}, nil
	}
	if input.Reply != "" {
		return workflow.CoordinationResponse{Summary: "The professional reports replacing the valve and checking the connection. The customer still needs to review this account against the agreed scope.", ProposedAction: "review-completion"}, nil
	}
	return workflow.CoordinationResponse{Question: "Which work does this evidence show for the agreed valve replacement, and what remains incomplete?", ProposedAction: "request-work-note"}, nil
}

func coordinationPhotoFixture(t *testing.T, ctx context.Context, store *Store) (string, string, workflow.State) {
	t.Helper()
	customer, professional, state := bookedCancellationFixture(t, ctx, store)
	state.Job.WorkStarted = true
	state.Job.AppointmentConfirmation.Status = "confirmed"
	state.Job.Description = "Replace the leaking valve"
	state.Job.ServiceAddress = "Private street 12"
	persistLifecycleState(t, ctx, store, state)
	state, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "record-evidence", Evidence: workflow.Evidence{AssetID: "asset-first", ContentType: "image/jpeg", Kind: "photo", Label: "Repaired valve"}})
	if err != nil {
		t.Fatal(err)
	}
	return customer, professional, state
}

func TestPhotoFirstCoordinationAsksOnceSummarizesReplyAndStopsForWorkNote(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	provider := &coordinationFixture{}
	store.SetCoordinator(provider)
	customer, professional, state := coordinationPhotoFixture(t, ctx, store)
	tasks, err := store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("photo-first task: %+v, %v", tasks, err)
	}
	task := tasks[0]
	if err := store.processCoordinationTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	tasks, err = store.ListCoordinationTasks(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || tasks[0].Status != "awaiting_reply" || !strings.Contains(tasks[0].Question, "valve") {
		t.Fatalf("context question: %+v, %v", tasks, err)
	}
	if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "record-evidence", Evidence: workflow.Evidence{AssetID: "asset-second", ContentType: "image/jpeg", Kind: "photo", Label: "Connection"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.processCoordinationTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("duplicate upload repeated the question: %d", len(provider.requests))
	}
	outsider := accountFixture(t, ctx, store)
	if err := store.ReplyCoordinationTask(ctx, outsider, state.WorkspaceID, task.ID, "The valve was replaced and checked."); err == nil {
		t.Fatal("another professional answered the private task")
	}
	if err := store.ReplyCoordinationTask(ctx, professional, state.WorkspaceID, task.ID, "The old valve was replaced and its connection checked."); err != nil {
		t.Fatal(err)
	}
	if err := store.processCoordinationTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	tasks, err = store.ListCoordinationTasks(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || tasks[0].Status != "summary_ready" || tasks[0].Summary == "" || tasks[0].ProposedAction != "review-completion" {
		t.Fatalf("reviewable summary: %+v, %v", tasks, err)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || loaded.Job.CompletionApproved || loaded.Job.PaymentApprovedVersion != 0 || loaded.Job.Paid || loaded.Job.DetailAdded {
		t.Fatalf("coordination performed a human approval: %+v, %v", loaded.Job, err)
	}
	for _, request := range provider.requests {
		if strings.Contains(request.AgreedScope, "Private street") || request.AgreedScope != "Replace the leaking valve" {
			t.Fatalf("scope/privacy boundary lost: %+v", request)
		}
	}
	if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "add-evidence", Value: "The leaking valve was replaced and its connection checked."}); err != nil {
		t.Fatal(err)
	}
	tasks, err = store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
	if err != nil || tasks[0].Status != "completed" {
		t.Fatalf("work note did not stop coordination: %+v, %v", tasks, err)
	}
	if err := store.processCoordinationTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 {
		t.Fatal("completed task called the provider again")
	}
}

func TestCoordinationUncertaintyEscalatesWithoutRepeatedQuestions(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	provider := &coordinationFixture{}
	store.SetCoordinator(provider)
	_, professional, state := coordinationPhotoFixture(t, ctx, store)
	tasks, err := store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks: %+v, %v", tasks, err)
	}
	task := tasks[0]
	if err := store.processCoordinationTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplyCoordinationTask(ctx, professional, state.WorkspaceID, task.ID, "I don't know what the picture shows"); err != nil {
		t.Fatal(err)
	}
	if err := store.processCoordinationTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	tasks, err = store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
	if err != nil || tasks[0].Status != "operator_review" || tasks[0].Outcome == "" || len(provider.requests) != 1 {
		t.Fatalf("uncertainty did not stop questioning: %+v, calls=%d, %v", tasks, len(provider.requests), err)
	}
	cases, err := store.ListWorkExceptions(ctx)
	if err != nil || !hasOperatorCase(cases, "coordination-review", state.WorkspaceID) {
		t.Fatalf("uncertainty has no accountable operator path: %+v, %v", cases, err)
	}
	state.Job.EvidenceSubmitted = true
	state.Job.EvidenceSubmittedAt = time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339)
	persistLifecycleState(t, ctx, store, state)
	if err := store.scheduleLifecycleNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxCount(t, ctx, store, state.Job.ID, "completion-note-overdue", 0)
}

func TestCoordinationQueuedWorkIsStaleAfterScopeRoundOrCancellation(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	provider := &coordinationFixture{}
	store.SetCoordinator(provider)
	for _, change := range []string{"scope", "round", "cancel"} {
		t.Run(change, func(t *testing.T) {
			_, professional, state := coordinationPhotoFixture(t, ctx, store)
			tasks, err := store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
			if err != nil || len(tasks) != 1 {
				t.Fatalf("tasks: %+v, %v", tasks, err)
			}
			task := tasks[0]
			switch change {
			case "scope":
				state.Job.ScopeVersion++
			case "round":
				state.Job.CompletionRound = 2
			case "cancel":
				state.Cancelled = true
			}
			persistLifecycleState(t, ctx, store, state)
			if err := store.processCoordinationTask(ctx, task.ID); err != nil {
				t.Fatal(err)
			}
			tasks, err = store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
			if err != nil || tasks[0].Status != "obsolete" {
				t.Fatalf("old question remained active: %+v, %v", tasks, err)
			}
		})
	}
	if len(provider.requests) != 0 {
		t.Fatal("stale work reached the model")
	}
}

func TestAmbiguousPostBookingMessageCreatesBoundedScopeTask(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	provider := &coordinationFixture{}
	store.SetCoordinator(provider)
	customer, professional, state := bookedCancellationFixture(t, ctx, store)
	state.Job.AppointmentConfirmation.Status = "confirmed"
	persistLifecycleState(t, ctx, store, state)
	for range 2 {
		if _, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "send-message", Value: "Can you also repair the damaged cabinet? I am not sure this is included."}); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
	if err != nil || len(tasks) != 1 || tasks[0].Trigger != "post-booking-scope" {
		t.Fatalf("scope ambiguity task: %+v, %v", tasks, err)
	}
	if err := store.processCoordinationTask(ctx, tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || !strings.Contains(provider.requests[0].Message, "damaged cabinet") {
		t.Fatalf("question lost the relevant context: %+v", provider.requests)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || loaded.Job.ScopeVersion != state.Job.ScopeVersion || loaded.Job.Quote.TotalMinor != state.Job.Quote.TotalMinor || loaded.Job.Appointment.StartUTC != state.Job.Appointment.StartUTC {
		t.Fatalf("coordination changed the agreed plan: %+v, %v", loaded.Job, err)
	}
}

func TestCoordinationQueuedQuestionResumesAfterStoreRestart(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	if err := store.river.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	_, professional, state := coordinationPhotoFixture(t, ctx, store)
	restarted, err := OpenDirectory(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	provider := &coordinationFixture{}
	restarted.SetCoordinator(provider)
	if err := startRiver(ctx, restarted); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		tasks, err := store.ListCoordinationTasks(ctx, professional, "professional", state.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) == 1 && tasks[0].Status == "awaiting_reply" {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("queued coordination did not resume: %+v", tasks)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.requests) != 1 {
		t.Fatalf("restart generated %d questions", len(provider.requests))
	}
}
