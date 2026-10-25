package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

func agentRole(role string) bool { return role == "customer" || role == "professional" }

func (s *Store) AgentMandate(ctx context.Context, accountID, role string) (workflow.AgentMandate, error) {
	if accountID == "" || !agentRole(role) {
		return workflow.AgentMandate{}, errors.New("an agent needs a customer or professional account")
	}
	var raw []byte
	var version int
	err := s.pool.QueryRow(ctx, `SELECT mandate, version FROM agent_mandates WHERE account_id=$1 AND role=$2`, accountID, role).Scan(&raw, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.defaultAgentMandate(ctx, accountID, role)
	}
	if err != nil {
		return workflow.AgentMandate{}, err
	}
	var mandate workflow.AgentMandate
	if err := json.Unmarshal(raw, &mandate); err != nil {
		return workflow.AgentMandate{}, err
	}
	mandate.Version = version
	return mandate, nil
}

// defaultAgentMandate is what an agent may do until its person changes it. A professional's days and
// hours follow their working-hours schedule when they have one.
func (s *Store) defaultAgentMandate(ctx context.Context, accountID, role string) (workflow.AgentMandate, error) {
	mandate := workflow.DefaultMandate(role)
	if role != "professional" {
		return mandate, nil
	}
	var weekdays []int32
	var start, end *int
	var zone *string
	err := s.pool.QueryRow(ctx, `
		SELECT array_agg(DISTINCT h.weekday)::int[], min(EXTRACT(hour FROM h.starts_local))::int, ceil(max(EXTRACT(epoch FROM h.ends_local)) / 3600)::int, min(sc.timezone)
		FROM professional_working_hours h JOIN professional_schedules sc ON sc.professional_account_id = h.professional_account_id
		WHERE h.professional_account_id = $1`, accountID).Scan(&weekdays, &start, &end, &zone)
	if err != nil || len(weekdays) == 0 || start == nil || end == nil {
		return mandate, nil
	}
	mandate.Weekdays = mandate.Weekdays[:0]
	for _, day := range weekdays {
		mandate.Weekdays = append(mandate.Weekdays, int(day))
	}
	mandate.StartHour, mandate.EndHour = *start, min(*end, 24)
	if zone != nil {
		mandate.Timezone = *zone
	}
	if err := mandate.Normalize(role); err != nil {
		return workflow.DefaultMandate(role), nil
	}
	return mandate, nil
}

// SaveAgentMandate validates and stores a mandate. Every change bumps the version, so each logged
// decision can be traced to the authority in force when it was made.
func (s *Store) SaveAgentMandate(ctx context.Context, accountID, role string, mandate workflow.AgentMandate) (workflow.AgentMandate, error) {
	if accountID == "" || !agentRole(role) {
		return workflow.AgentMandate{}, errors.New("an agent needs a customer or professional account")
	}
	if err := mandate.Normalize(role); err != nil {
		return workflow.AgentMandate{}, err
	}
	mandate.Version = 0
	raw, err := json.Marshal(mandate)
	if err != nil {
		return workflow.AgentMandate{}, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO agent_mandates (account_id,role,mandate) VALUES ($1,$2,$3)
		ON CONFLICT (account_id,role) DO UPDATE SET mandate=EXCLUDED.mandate, version=agent_mandates.version+1, updated_at=now()
		RETURNING version`, accountID, role, raw).Scan(&mandate.Version)
	return mandate, err
}

func (s *Store) RecordAgentDecision(ctx context.Context, workspaceID, accountID, role string, mandateVersion int, code, action string, params map[string]string, outcome, failure string) error {
	if params == nil {
		params = map[string]string{}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if len(failure) > 500 {
		failure = failure[:500]
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO agent_decisions (workspace_id,account_id,role,mandate_version,code,action,params,outcome,error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, workspaceID, accountID, role, max(mandateVersion, 1), code, action, raw, outcome, failure)
	return err
}

// AgentCounters is how many price counter-offers this agent has already sent on the request.
func (s *Store) AgentCounters(ctx context.Context, workspaceID, accountID, role string) (prices, visits int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE code='counter-price'), count(*) FILTER (WHERE code='propose-visit') FROM agent_decisions
		WHERE workspace_id=$1 AND account_id=$2 AND role=$3 AND outcome='applied'`, workspaceID, accountID, role).Scan(&prices, &visits)
	return
}

// AgentAvoidedProfiles lists the professionals this customer's agent went back to the matches for on
// this request because they cancelled it.
func (s *Store) AgentAvoidedProfiles(ctx context.Context, workspaceID, accountID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT params->>'profileId' FROM agent_decisions
		WHERE workspace_id=$1 AND account_id=$2 AND role='customer' AND code='find-another' AND outcome='applied'
		  AND COALESCE(params->>'profileId','') <> ''`, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) AgentActivity(ctx context.Context, workspaceID, accountID, role string, limit int) ([]workflow.AgentActivity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, role, code, action, params, outcome, error, created_at FROM agent_decisions
		WHERE workspace_id=$1 AND account_id=$2 AND role=$3 ORDER BY id DESC LIMIT $4`, workspaceID, accountID, role, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]workflow.AgentActivity, 0)
	for rows.Next() {
		var item workflow.AgentActivity
		var raw []byte
		if err := rows.Scan(&item.ID, &item.Role, &item.Code, &item.Action, &raw, &item.Outcome, &item.Error, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Params)
		items = append(items, item)
	}
	return items, rows.Err()
}

// SyncAgentEscalation makes the open escalation match the agent's current decision: a code opens or
// replaces it, an empty code resolves it. It reports whether a new escalation was opened.
func (s *Store) SyncAgentEscalation(ctx context.Context, workspaceID, accountID, role, code string, params map[string]string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var openID int64
	var openCode string
	err = tx.QueryRow(ctx, `SELECT id, code FROM agent_escalations WHERE workspace_id=$1 AND account_id=$2 AND role=$3 AND resolved_at IS NULL FOR UPDATE`, workspaceID, accountID, role).Scan(&openID, &openCode)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	exists := err == nil
	if exists && openCode == code {
		return false, nil
	}
	if exists {
		if _, err := tx.Exec(ctx, `UPDATE agent_escalations SET resolved_at=now() WHERE id=$1`, openID); err != nil {
			return false, err
		}
	}
	opened := false
	if code != "" {
		if params == nil {
			params = map[string]string{}
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return false, err
		}
		var escalationID int64
		if err := tx.QueryRow(ctx, `INSERT INTO agent_escalations (workspace_id,account_id,role,code,params) VALUES ($1,$2,$3,$4,$5) RETURNING id`, workspaceID, accountID, role, code, raw).Scan(&escalationID); err != nil {
			return false, err
		}
		opened = true
		// Tell the person in the app, in the same transaction as the question itself.
		var requestID string
		if err := tx.QueryRow(ctx, `SELECT `+notificationRequestID("w")+` FROM account_workspaces w WHERE w.id=$1`, workspaceID).Scan(&requestID); err != nil {
			return false, err
		}
		args := NotificationArgs{JobID: requestID, EventKey: fmt.Sprintf("%s:%d", agentQuestionEvent, escalationID), EventType: agentQuestionEvent, RecipientRole: role}
		if err := enqueueNotificationTx(ctx, tx, s.river, args); err != nil {
			return false, err
		}
	}
	return opened, tx.Commit(ctx)
}

func (s *Store) AgentEscalations(ctx context.Context, accountID, role string) ([]workflow.AgentEscalation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, workspace_id, role, code, params, created_at FROM agent_escalations
		WHERE account_id=$1 AND role=$2 AND resolved_at IS NULL ORDER BY created_at DESC LIMIT 50`, accountID, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]workflow.AgentEscalation, 0)
	for rows.Next() {
		var item workflow.AgentEscalation
		var raw []byte
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Role, &item.Code, &raw, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Params)
		items = append(items, item)
	}
	return items, rows.Err()
}

// AgentParties returns the accounts that may have an agent on this request.
func (s *Store) AgentParties(ctx context.Context, workspaceID string) (customer, professional workflow.AgentParty, err error) {
	var proID string
	err = s.pool.QueryRow(ctx, `
		SELECT w.account_id, COALESCE(w.state_json #>> '{job,professionalAccountId}', ''), c.display_name, c.locale,
		  EXISTS (SELECT 1 FROM account_memberships m WHERE m.account_id = c.id AND m.role = 'customer' AND m.status = 'active')
		FROM account_workspaces w JOIN accounts c ON c.id = w.account_id WHERE w.id=$1`, workspaceID).Scan(&customer.AccountID, &proID, &customer.DisplayName, &customer.Locale, &customer.Active)
	if err != nil {
		return
	}
	if proID != "" {
		professional.AccountID = proID
		err = s.pool.QueryRow(ctx, `SELECT display_name, locale, EXISTS (SELECT 1 FROM account_memberships m WHERE m.account_id = accounts.id AND m.role = 'professional' AND m.status = 'active') FROM accounts WHERE id=$1`, proID).Scan(&professional.DisplayName, &professional.Locale, &professional.Active)
	}
	return
}

// AgentWorkspaces lists recently active requests where either person has an enabled agent. The
// sweeper uses it to catch time-based steps no user action triggers, such as a stale price offer.
func (s *Store) AgentWorkspaces(ctx context.Context, since time.Time, limit int) ([]string, error) {
	// Agents are on unless their person paused them, so a request qualifies unless every agent on it is paused.
	rows, err := s.pool.Query(ctx, `
		SELECT w.id FROM account_workspaces w
		WHERE w.updated_at > $1
		  AND (NOT COALESCE((w.state_json->>'cancelled')::boolean, false) OR w.state_json->>'cancelledBy' = 'professional')
		  AND (NOT COALESCE((w.state_json #>> '{job,paid}')::boolean, false) OR w.updated_at > now() - interval '1 day')
		  AND (w.state_json #>> '{intake,step}' = 'matches' OR COALESCE(w.state_json #>> '{job,id}', '') <> '')
		  AND (
			NOT EXISTS (SELECT 1 FROM agent_mandates m WHERE m.account_id=w.account_id AND m.role='customer' AND NOT (m.mandate->>'enabled')::boolean)
			OR (COALESCE(w.state_json #>> '{job,professionalAccountId}', '') <> '' AND NOT EXISTS (SELECT 1 FROM agent_mandates m WHERE m.account_id = w.state_json #>> '{job,professionalAccountId}' AND m.role='professional' AND NOT (m.mandate->>'enabled')::boolean)))
		ORDER BY w.updated_at DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) OpenAgentEscalation(ctx context.Context, workspaceID, accountID, role string) (string, error) {
	var code string
	err := s.pool.QueryRow(ctx, `SELECT code FROM agent_escalations WHERE workspace_id=$1 AND account_id=$2 AND role=$3 AND resolved_at IS NULL`, workspaceID, accountID, role).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return code, err
}
