package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ResolveWorkCase(ctx context.Context, operatorID, workspaceID, kind, decision, rationale string) error {
	if operatorID == "" || workspaceID == "" || len(rationale) < 10 || len(rationale) > 1000 {
		return errors.New("operator, request, and a 10 to 1000 character resolution rationale are required")
	}
	rationale = strings.TrimSpace(rationale)
	if len(rationale) < 10 {
		return errors.New("resolution rationale must contain at least 10 characters")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id, state_json, revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return errors.New("operator case was not found")
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return err
	}
	state.WorkspaceID = workspaceID
	action := ""
	switch kind {
	case "late-cancellation":
		request := state.CancellationReview
		if request == nil || request.Status != "pending" {
			return errors.New("late cancellation is no longer awaiting review")
		}
		switch decision {
		case "approve-cancellation":
			state.Cancelled = true
			state.CancelledBy = request.RequestedBy
			request.Status = "approved"
			action = "approve-late-cancellation"
			if state.Job.ProfessionalAccountID != "" {
				if err := releaseCancelledBooking(ctx, tx, state.Job); err != nil {
					return err
				}
			}
		case "keep-visit":
			request.Status = "declined"
			action = "decline-late-cancellation"
		default:
			return errors.New("choose approve-cancellation or keep-visit")
		}
		request.Resolution = rationale
		request.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
		addOperatorEvent(&state.Job, "Operator reviewed the late cancellation · "+decision, rationale)
	case "job-issue":
		issue := state.Job.Dispute
		reconciliationOnly := issue != nil && (issue.Status == "partial_refund_review" || issue.Status == "payment_reversal_review")
		if issue == nil || issue.Status != "open" && !reconciliationOnly {
			return errors.New("job issue is no longer awaiting review")
		}
		switch decision {
		case "request-rework":
			if reconciliationOnly || state.Job.Paid {
				return errors.New("a captured payment must be refunded before paid work can be reopened")
			}
			issue.Status = "rework"
			action = "request-work-rework"
			state.Job.EvidenceSubmitted = false
			state.Job.EvidenceSubmittedAt = ""
			state.Job.DetailsRequested = true
			state.Job.DetailAdded = false
			state.Job.CompletionApproved = false
			state.Job.CompletionApprovedAt = ""
			state.Job.PaymentApprovedVersion = 0
			state.Job.EvidenceReview = nil
		case "close-review":
			issue.Status = "resolved"
			action = "close-job-issue"
		default:
			return errors.New("choose request-rework or close-review")
		}
		issue.Resolution = rationale
		issue.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
		addOperatorEvent(&state.Job, "Operator recorded an issue resolution · "+decision, rationale)
	default:
		return errors.New("operator case type cannot be resolved here")
	}

	stateBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2, revision=revision+1, updated_at=now() WHERE id=$1`, workspaceID, stateBytes); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id, actor_account_id, workspace_id, job_id, actor, action, scope_version, revision)
		VALUES ($1,$2,$3,$4,'operator',$5,$6,$7)`, ownerID, operatorID, workspaceID, state.Job.ID, action, int32(state.Job.ScopeVersion), revision+1); err != nil {
		return err
	}
	for _, recipient := range []string{"customer", "professional"} {
		if state.Job.ProfessionalAccountID == "" && recipient == "professional" {
			continue
		}
		eventKey := fmt.Sprintf("%s:%d:%s:%s", state.Job.ID, revision+1, action, recipient)
		if err := enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: state.Job.ID, EventKey: eventKey, EventType: action, RecipientRole: recipient}); err != nil {
			return fmt.Errorf("enqueue operator case notification: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) BeginWorkRefund(ctx context.Context, operatorID, workspaceID, rationale string) (workflow.State, error) {
	rationale = strings.TrimSpace(rationale)
	if operatorID == "" || workspaceID == "" || len(rationale) < 10 || len(rationale) > 1000 {
		return workflow.State{}, errors.New("operator, request, and a 10 to 1000 character refund rationale are required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return workflow.State{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id, state_json, revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return workflow.State{}, errors.New("paid issue was not found")
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return workflow.State{}, err
	}
	issue := state.Job.Dispute
	if issue == nil || issue.Status != "open" && issue.Status != "refund_pending" || !state.Job.Paid || state.Job.PaymentRefunded {
		return workflow.State{}, errors.New("a captured payment with an unresolved issue is required")
	}
	firstAttempt := issue.Status != "refund_pending"
	issue.Status = "refund_pending"
	issue.Resolution = rationale
	if firstAttempt {
		addOperatorEvent(&state.Job, "Operator approved a full PayPal refund · processing", rationale)
	}
	state.WorkspaceID = workspaceID
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return workflow.State{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2, revision=revision+1, updated_at=now() WHERE id=$1`, workspaceID, stateBytes); err != nil {
		return workflow.State{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id, actor_account_id, workspace_id, job_id, actor, action, scope_version, revision)
		VALUES ($1,$2,$3,$4,'operator','begin-full-refund',$5,$6)`, ownerID, operatorID, workspaceID, state.Job.ID, int32(state.Job.ScopeVersion), revision+1); err != nil {
		return workflow.State{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workflow.State{}, err
	}
	return state, nil
}

func (s *Store) CompleteWorkRefund(ctx context.Context, operatorID, workspaceID, refundID string) error {
	if operatorID == "" || workspaceID == "" {
		return errors.New("operator and request are required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id, state_json, revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return errors.New("refund case was not found")
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return err
	}
	issue := state.Job.Dispute
	if issue == nil || !state.Job.Paid {
		return errors.New("a paid issue is required to complete the refund")
	}
	if issue.Status == "refunded" && state.Job.PaymentRefunded {
		return nil
	}
	if issue.Status != "refund_pending" {
		return errors.New("refund was not approved for this issue")
	}
	issue.Status = "refunded"
	issue.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
	state.Job.PaymentRefunded = true
	state.Job.RefundTransactionID = refundID
	addOperatorEvent(&state.Job, "Full PayPal refund completed", issue.Resolution)
	state.WorkspaceID = workspaceID
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2, revision=revision+1, updated_at=now() WHERE id=$1`, workspaceID, stateBytes); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id, actor_account_id, workspace_id, job_id, actor, action, scope_version, revision)
		VALUES ($1,$2,$3,$4,'operator','complete-full-refund',$5,$6)`, ownerID, operatorID, workspaceID, state.Job.ID, int32(state.Job.ScopeVersion), revision+1); err != nil {
		return err
	}
	for _, recipient := range []string{"customer", "professional"} {
		eventKey := fmt.Sprintf("%s:%d:refund-completed:%s", state.Job.ID, revision+1, recipient)
		if err := enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: state.Job.ID, EventKey: eventKey, EventType: "refund-completed", RecipientRole: recipient}); err != nil {
			return fmt.Errorf("enqueue refund notice: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func addOperatorEvent(job *workflow.Job, text, rationale string) {
	if job.ID == "" {
		return
	}
	job.Timeline = append(job.Timeline, workflow.Event{Text: text + ". " + rationale, Kind: "done", When: time.Now().UTC().Format("15:04")})
}
