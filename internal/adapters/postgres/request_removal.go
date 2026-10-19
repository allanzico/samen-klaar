package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

// notRemovedByProfessional hides a job the professional took off their list. It needs the
// professional's account ID as $1. A later job in the same workspace shows again.
const notRemovedByProfessional = `NOT EXISTS (SELECT 1 FROM professional_removed_requests removed
  WHERE removed.workspace_id = account_workspaces.id AND removed.account_id = $1
    AND removed.job_id = COALESCE(account_workspaces.state_json #>> '{job,id}', ''))`

// lockOpenRequestCountTx serialises request creation per customer and counts their open
// requests, leaving out excludeWorkspaceID. The lock lasts until the transaction ends.
func lockOpenRequestCountTx(ctx context.Context, tx pgx.Tx, accountID, excludeWorkspaceID string) (int, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('open-requests:' || $1))`, accountID); err != nil {
		return 0, err
	}
	return openRequestCount(ctx, tx, accountID, excludeWorkspaceID)
}

type rowQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func openRequestCount(ctx context.Context, db rowQuerier, accountID, excludeWorkspaceID string) (int, error) {
	// Cancelled and approved requests never count, so only the rest are decoded and checked.
	rows, err := db.Query(ctx, `SELECT `+workspaceSummaryJSON+` FROM account_workspaces
	  WHERE account_id = $1 AND id <> $2
	    AND COALESCE((state_json->>'cancelled')::boolean, false) = false
	    AND COALESCE((state_json #>> '{job,quoteApproved}')::boolean, false) = false`, accountID, excludeWorkspaceID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	states := []workflow.State{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		var state workflow.State
		if err := json.Unmarshal(raw, &state); err != nil {
			return 0, err
		}
		states = append(states, state)
	}
	return workflow.CountOpenRequests(states), rows.Err()
}

// OpenRequestCount reports how many of the customer's requests count towards the limit.
func (s *Store) OpenRequestCount(ctx context.Context, accountID string) (int, error) {
	return openRequestCount(ctx, s.pool, accountID, "")
}

// RemoveRequest deletes a customer's unapproved request, or takes an unapproved job off a
// professional's list (declining it first if it is still active). It returns "deleted" or "removed".
func (s *Store) RemoveRequest(ctx context.Context, accountID, role, workspaceID string) (string, error) {
	switch {
	case accountID == "" || workspaceID == "":
		return "", workflow.ErrRequestNotFound
	case role == "professional":
		var assigned bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_workspaces WHERE id = $1 AND state_json #>> '{job,professionalAccountId}' = $2)`, workspaceID, accountID).Scan(&assigned); err != nil {
			return "", err
		}
		if !assigned {
			return "", workflow.ErrRequestNotFound
		}
		// ApplyWorkspace checks the assignment and the rule again under the row lock.
		if _, err := s.ApplyWorkspace(ctx, accountID, role, workspaceID, workflow.Action{Actor: role, Action: "remove-request"}); err != nil {
			return "", err
		}
		return "removed", nil
	case role == "customer":
		return "deleted", s.deleteCustomerRequest(ctx, accountID, workspaceID)
	default:
		return "", errors.New("workspace role cannot remove requests")
	}
}

func (s *Store) deleteCustomerRequest(ctx context.Context, accountID, workspaceID string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var raw []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT state_json, revision FROM account_workspaces WHERE id = $1 AND account_id = $2 FOR UPDATE`, workspaceID, accountID).Scan(&raw, &revision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workflow.ErrRequestNotFound
		}
		return err
	}
	var state workflow.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if !workflow.CustomerCanDelete(state) {
		return workflow.ErrRequestNotDeletable
	}
	// The state alone says nothing was approved. Refuse anyway if any payment, booking,
	// support case or coordination record exists, so no such record is ever cascaded away.
	var kept bool
	if err := tx.QueryRow(ctx, `SELECT
	    EXISTS (SELECT 1 FROM paypal_payment_operations WHERE workspace_id = $1 OR ($2 <> '' AND job_id = $2))
	 OR EXISTS (SELECT 1 FROM booking_reservations WHERE $2 <> '' AND job_id = $2)
	 OR EXISTS (SELECT 1 FROM operator_case_work WHERE workspace_id = $1)
	 OR EXISTS (SELECT 1 FROM operator_case_notes WHERE workspace_id = $1)
	 OR EXISTS (SELECT 1 FROM coordination_tasks WHERE workspace_id = $1)`, workspaceID, state.Job.ID).Scan(&kept); err != nil {
		return err
	}
	if kept {
		return workflow.ErrRequestNotDeletable
	}
	if state.Job.ID != "" {
		if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status = 'obsolete', updated_at = now() WHERE job_id = $1 AND status IN ('queued', 'failed', 'adapter_unconfigured')`, state.Job.ID); err != nil {
			return err
		}
	}
	// The audit trail outlives the request: audit_events has no foreign key to the workspace.
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id, actor_account_id, workspace_id, job_id, actor, action, scope_version, revision) VALUES ($1, $1, $2, $3, 'customer', 'delete-request', $4, $5)`, accountID, workspaceID, state.Job.ID, int32(state.Job.ScopeVersion), revision+1); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM account_workspaces WHERE id = $1`, workspaceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
