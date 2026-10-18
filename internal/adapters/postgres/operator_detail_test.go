package postgres

import (
	"encoding/json"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"testing"
)

func TestEvidencePolicyRequiresOperatorApprovalAndSnapshotsRemainIndependent(t *testing.T) {
	ctx, s := accountTestStore(t)
	_, _, state := bookedCancellationFixture(t, ctx, s)
	pro := state.Job.ProfessionalAccountID
	items := []workflow.EvidenceRequirement{{ID: "valve-photo", Label: "Photo of the replaced valve", Kind: "photo"}}
	proposed, err := s.ProposeEvidencePolicy(ctx, pro, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposed.Requirements) != 0 {
		t.Fatal("unreviewed policy published")
	}
	outsider := accountFixture(t, ctx, s)
	if _, err = s.ReviewEvidencePolicy(ctx, outsider, state.Job.ProfessionalProfileID, true); err == nil {
		t.Fatal("customer approved evidence policy")
	}
	op := accountFixture(t, ctx, s)
	if _, err = s.pool.Exec(ctx, `INSERT INTO account_memberships(account_id,role,status) VALUES($1,'operator','active')`, op); err != nil {
		t.Fatal(err)
	}
	policy, err := s.ReviewEvidencePolicy(ctx, op, state.Job.ProfessionalProfileID, true)
	if err != nil || len(policy.Requirements) != 1 {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
	if len(state.Job.WorkItems) != 0 {
		t.Fatal("existing agreement changed when future service evidence changed")
	}
}
func TestOperatorCaseAccessIsNarrowAuditedAndSupportIsRecipientScoped(t *testing.T) {
	ctx, s := accountTestStore(t)
	customer, pro, state := bookedCancellationFixture(t, ctx, s)
	state.Job.WorkStarted = true
	state.Job.ServiceAddress = "private full address"
	state.Job.Dispute = &workflow.DisputeCase{Status: "open", Reason: "The valve is leaking after the visit"}
	raw, _ := json.Marshal(state)
	if _, err := s.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	outsider := accountFixture(t, ctx, s)
	if _, err := s.OperatorCaseDetail(ctx, outsider, state.WorkspaceID, "job-issue"); err == nil {
		t.Fatal("unrelated account read case")
	}
	op := accountFixture(t, ctx, s)
	if _, err := s.pool.Exec(ctx, `INSERT INTO account_memberships(account_id,role,status) VALUES($1,'operator','active')`, op); err != nil {
		t.Fatal(err)
	}
	detail, err := s.OperatorCaseDetail(ctx, op, state.WorkspaceID, "job-issue")
	if err != nil {
		t.Fatal(err)
	}
	if detail.State.Job.ServiceAddress != "" {
		t.Fatal("operator received restricted full address")
	}
	if detail.State.Job.Quote.TotalMinor != state.Job.Quote.TotalMinor {
		t.Fatal("operator missing agreement")
	}
	if err = s.UpdateOperatorCase(ctx, op, state.WorkspaceID, "job-issue", "claim", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateOperatorCase(ctx, op, state.WorkspaceID, "job-issue", "contact", "customer", "Please describe where the valve is leaking.", ""); err != nil {
		t.Fatal(err)
	}
	notes, err := s.ParticipantCaseNotes(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("customer notes=%+v err=%v", notes, err)
	}
	notes, err = s.ParticipantCaseNotes(ctx, pro, "professional", state.WorkspaceID)
	if err != nil || len(notes) != 0 {
		t.Fatalf("professional saw private customer contact: %+v %v", notes, err)
	}
	if _, err = s.ParticipantCaseNotes(ctx, outsider, "customer", state.WorkspaceID); err == nil {
		t.Fatal("unrelated account saw support contact")
	}
}
