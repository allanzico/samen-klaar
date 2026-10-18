package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

func assertOperator(ctx context.Context, tx pgx.Tx, accountID string) error {
	var yes bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_memberships WHERE account_id=$1 AND role='operator' AND status='active')`, accountID).Scan(&yes)
	if err != nil {
		return err
	}
	if !yes {
		return errors.New("active operator access is required")
	}
	return nil
}
func (s *Store) permittedCase(ctx context.Context, workspaceID, kind string) (workflow.OperatorCase, error) {
	cases, err := s.ListWorkExceptions(ctx)
	if err != nil {
		return workflow.OperatorCase{}, err
	}
	for _, item := range cases {
		if item.WorkspaceID == workspaceID && item.Kind == kind {
			return item, nil
		}
	}
	var exists bool
	err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM operator_case_work WHERE workspace_id=$1 AND kind=$2)`, workspaceID, kind).Scan(&exists)
	if err != nil {
		return workflow.OperatorCase{}, err
	}
	if exists {
		return workflow.OperatorCase{WorkspaceID: workspaceID, Kind: kind}, nil
	}
	return workflow.OperatorCase{}, errors.New("an active or previously assigned operator case is required")
}
func (s *Store) OperatorCaseDetail(ctx context.Context, operatorID, workspaceID, kind string) (workflow.OperatorCaseDetail, error) {
	item, err := s.permittedCase(ctx, workspaceID, kind)
	if err != nil {
		return workflow.OperatorCaseDetail{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return workflow.OperatorCaseDetail{}, err
	}
	defer tx.Rollback(ctx)
	if err = assertOperator(ctx, tx, operatorID); err != nil {
		return workflow.OperatorCaseDetail{}, err
	}
	var raw []byte
	detail := workflow.OperatorCaseDetail{Case: item, Status: "open", Notes: []workflow.CaseNote{}, Decisions: []workflow.CaseDecision{}, PaymentStatus: "not_started"}
	if err = tx.QueryRow(ctx, `SELECT state_json FROM account_workspaces WHERE id=$1`, workspaceID).Scan(&raw); err != nil {
		return detail, err
	}
	if err = json.Unmarshal(raw, &detail.State); err != nil {
		return detail, err
	}
	detail.State.WorkspaceID = workspaceID
	detail.State = workflow.ConversationForRole(detail.State, "professional")
	detail.State.Job.ServiceAddress = ""
	err = tx.QueryRow(ctx, `SELECT COALESCE(assigned_to,''),status,outcome FROM operator_case_work WHERE workspace_id=$1 AND kind=$2`, workspaceID, kind).Scan(&detail.AssignedTo, &detail.Status, &detail.Outcome)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return detail, err
	}
	err = tx.QueryRow(ctx, `SELECT status FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, workspaceID, detail.State.Job.ID, detail.State.Job.ScopeVersion).Scan(&detail.PaymentStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return detail, err
	}
	rows, err := tx.Query(ctx, `SELECT id,recipient_role,note,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM operator_case_notes WHERE workspace_id=$1 AND kind=$2 ORDER BY id`, workspaceID, kind)
	if err != nil {
		return detail, err
	}
	for rows.Next() {
		var note workflow.CaseNote
		if err = rows.Scan(&note.ID, &note.RecipientRole, &note.Note, &note.CreatedAt); err != nil {
			rows.Close()
			return detail, err
		}
		detail.Notes = append(detail.Notes, note)
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT actor,action,scope_version,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM audit_events WHERE workspace_id=$1 ORDER BY revision,id`, workspaceID)
	if err != nil {
		return detail, err
	}
	for rows.Next() {
		var decision workflow.CaseDecision
		if err = rows.Scan(&decision.Actor, &decision.Action, &decision.ScopeVersion, &decision.At); err != nil {
			rows.Close()
			return detail, err
		}
		detail.Decisions = append(detail.Decisions, decision)
	}
	rows.Close()
	if _, err = tx.Exec(ctx, `INSERT INTO operator_case_access(workspace_id,kind,actor_account_id) VALUES($1,$2,$3)`, workspaceID, kind, operatorID); err != nil {
		return detail, err
	}
	err = tx.Commit(ctx)
	return detail, err
}
func (s *Store) UpdateOperatorCase(ctx context.Context, operatorID, workspaceID, kind, action, recipient, note, itemID string) error {
	if _, err := s.permittedCase(ctx, workspaceID, kind); err != nil {
		return err
	}
	note = strings.TrimSpace(note)
	if action != "claim" && (len(note) < 10 || len(note) > 1000) {
		return errors.New("record a 10 to 1000 character case note")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = assertOperator(ctx, tx, operatorID); err != nil {
		return err
	}
	var raw []byte
	var owner string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT account_id,state_json,revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&owner, &raw, &revision); err != nil {
		return err
	}
	var state workflow.State
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	state.WorkspaceID = workspaceID
	if _, err = tx.Exec(ctx, `INSERT INTO operator_case_work(workspace_id,kind) VALUES($1,$2) ON CONFLICT DO NOTHING`, workspaceID, kind); err != nil {
		return err
	}
	switch action {
	case "claim":
		_, err = tx.Exec(ctx, `UPDATE operator_case_work SET assigned_to=$3,status='open',updated_at=now() WHERE workspace_id=$1 AND kind=$2 AND (assigned_to IS NULL OR assigned_to=$3)`, workspaceID, kind, operatorID)
	case "contact":
		if recipient != "customer" && recipient != "professional" || recipient == "professional" && state.Job.ProfessionalAccountID == "" {
			return errors.New("choose an assigned case participant")
		}
	case "wait", "close":
		status := "waiting"
		if action == "close" {
			status = "resolved"
		}
		_, err = tx.Exec(ctx, `UPDATE operator_case_work SET assigned_to=$3,status=$4,outcome=$5,updated_at=now() WHERE workspace_id=$1 AND kind=$2`, workspaceID, kind, operatorID, status, note)
	case "resolve-evidence":
		if err = workflow.ResolveEvidenceGap(&state.Job, itemID, note, time.Now()); err != nil {
			return err
		}
	default:
		return errors.New("unknown case action")
	}
	if err != nil {
		return err
	}
	if action != "claim" {
		if _, err = tx.Exec(ctx, `INSERT INTO operator_case_notes(workspace_id,kind,actor_account_id,recipient_role,note) VALUES($1,$2,$3,$4,$5)`, workspaceID, kind, operatorID, recipient, note); err != nil {
			return err
		}
	}
	state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Kind: "attention", Text: "Operator case update · " + action, When: time.Now().UTC().Format(time.RFC3339)})
	updated, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,revision=revision+1,updated_at=now() WHERE id=$1`, workspaceID, updated); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(account_id,actor_account_id,workspace_id,job_id,actor,action,scope_version,revision) VALUES($1,$2,$3,$4,'operator',$5,$6,$7)`, owner, operatorID, workspaceID, state.Job.ID, "case-"+action, state.Job.ScopeVersion, revision+1); err != nil {
		return err
	}
	if action == "contact" {
		if err = enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: state.Job.ID, EventKey: fmt.Sprintf("%s:%d:operator-contact:%s", state.Job.ID, revision+1, recipient), EventType: "operator-contact", RecipientRole: recipient}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ParticipantCaseNotes(ctx context.Context, accountID, role, workspaceID string) ([]workflow.CaseNote, error) {
	if _, err := s.LoadWorkspace(ctx, accountID, role, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,recipient_role,note,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM operator_case_notes WHERE workspace_id=$1 AND recipient_role=$2 ORDER BY id`, workspaceID, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []workflow.CaseNote{}
	for rows.Next() {
		var n workflow.CaseNote
		if err = rows.Scan(&n.ID, &n.RecipientRole, &n.Note, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}
