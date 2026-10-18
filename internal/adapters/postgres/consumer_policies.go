package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

const consumerPolicyColumns = `kind,version,locale,title,body,requires_acknowledgment,reviewer_id,review_rationale,review_reference,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func scanConsumerPolicy(row pgx.Row) (workflow.ConsumerPolicy, error) {
	var p workflow.ConsumerPolicy
	err := row.Scan(&p.Kind, &p.Version, &p.Locale, &p.Title, &p.Body, &p.RequiresAcknowledgment, &p.ReviewerID, &p.ReviewRationale, &p.ReviewReference, &p.PublishedAt)
	return p, err
}

func (s *Store) CurrentConsumerPolicies(ctx context.Context) ([]workflow.ConsumerPolicy, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (kind) `+consumerPolicyColumns+` FROM consumer_policy_versions ORDER BY kind,published_at DESC,version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []workflow.ConsumerPolicy{}
	for rows.Next() {
		p, err := scanConsumerPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) PublishConsumerPolicy(ctx context.Context, operatorID string, p workflow.ConsumerPolicy) error {
	if p.Kind != "terms" && p.Kind != "cancellation" && p.Kind != "withdrawal" && p.Kind != "early_start" {
		return errors.New("choose a supported consumer policy kind")
	}
	if len(strings.TrimSpace(p.Version)) < 1 || len(p.Version) > 80 || p.Locale != "en" && p.Locale != "nl" || len(strings.TrimSpace(p.Title)) < 3 || len(p.Title) > 160 || len(strings.TrimSpace(p.Body)) < 10 || len(p.Body) > 20000 || p.ReviewerID == "" || len(strings.TrimSpace(p.ReviewRationale)) < 10 || len(p.ReviewRationale) > 1000 || strings.TrimSpace(p.ReviewReference) == "" || len(p.ReviewReference) > 1000 {
		return errors.New("a policy version, exact reviewed text, reviewer, rationale, and review reference are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := assertOperator(ctx, tx, operatorID); err != nil {
		return err
	}
	if err := assertOperator(ctx, tx, p.ReviewerID); err != nil {
		return errors.New("the named policy reviewer must have active operator access")
	}
	_, err = tx.Exec(ctx, `INSERT INTO consumer_policy_versions(kind,version,locale,title,body,requires_acknowledgment,reviewer_id,review_rationale,review_reference,published_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, p.Kind, p.Version, p.Locale, p.Title, p.Body, p.RequiresAcknowledgment, p.ReviewerID, p.ReviewRationale, p.ReviewReference, operatorID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func prepareConsumerPolicyActionTx(ctx context.Context, tx pgx.Tx, state *workflow.State, input *workflow.Action) error {
	if input.Action == "acknowledge-policy" {
		policy, err := scanConsumerPolicy(tx.QueryRow(ctx, `SELECT `+consumerPolicyColumns+` FROM consumer_policy_versions WHERE kind=$1 AND version=$2 FOR SHARE`, input.PolicyKind, input.PolicyVersion))
		if err != nil {
			return errors.New("the requested reviewed policy version is not published")
		}
		bound := false
		for _, required := range state.Job.PolicyRequirements {
			if required.Kind == policy.Kind {
				if required.Version != policy.Version {
					return errors.New("acknowledge the policy version recorded with this booking")
				}
				bound = true
			}
		}
		if !bound {
			var version string
			if err := tx.QueryRow(ctx, `SELECT version FROM consumer_policy_versions WHERE kind=$1 ORDER BY published_at DESC,version DESC LIMIT 1`, policy.Kind).Scan(&version); err != nil {
				return err
			}
			if version != policy.Version {
				return errors.New("the current policy version changed; review it before acknowledging")
			}
		}
		input.PublishedPolicy = &policy
	}
	if state.Job.Appointment != nil || input.Action != "book-appointment" && input.Action != "approve-and-book" && input.Action != "accept-date" {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (kind) kind,version,requires_acknowledgment FROM consumer_policy_versions ORDER BY kind,published_at DESC,version DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	state.Job.PolicyRequirements = []workflow.PolicyRequirement{}
	for rows.Next() {
		var required workflow.PolicyRequirement
		if err := rows.Scan(&required.Kind, &required.Version, &required.RequiresAcknowledgment); err != nil {
			return err
		}
		state.Job.PolicyRequirements = append(state.Job.PolicyRequirements, required)
	}
	return rows.Err()
}

func persistConsumerPolicyAcknowledgmentTx(ctx context.Context, tx pgx.Tx, accountID string, state workflow.State, input workflow.Action) error {
	if input.Action != "acknowledge-policy" {
		return nil
	}
	for _, ack := range state.Job.PolicyAcknowledgments {
		if ack.Kind != input.PolicyKind || ack.Version != input.PolicyVersion || ack.ScopeVersion != state.Job.ScopeVersion {
			continue
		}
		_, err := tx.Exec(ctx, `INSERT INTO consumer_policy_acknowledgments(workspace_id,account_id,job_id,scope_version,policy_kind,policy_version,acknowledged_at) VALUES($1,$2,$3,$4,$5,$6,$7::timestamptz) ON CONFLICT DO NOTHING`, state.WorkspaceID, accountID, state.Job.ID, ack.ScopeVersion, ack.Kind, ack.Version, ack.AcknowledgedAt)
		return err
	}
	return errors.New("the policy acknowledgment did not identify the current agreement")
}
