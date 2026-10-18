package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

func (s *Store) EvidencePolicy(ctx context.Context, profileID string) (workflow.ServiceEvidencePolicy, error) {
	policy := workflow.ServiceEvidencePolicy{ProfileID: profileID, Requirements: []workflow.EvidenceRequirement{}, Status: "not_set"}
	var approved, proposed []byte
	err := s.pool.QueryRow(ctx, `SELECT requirements,proposed,status,COALESCE(to_char(reviewed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),'') FROM professional_evidence_policies WHERE profile_id=$1`, profileID).Scan(&approved, &proposed, &policy.Status, &policy.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	if err = json.Unmarshal(approved, &policy.Requirements); err != nil {
		return policy, err
	}
	err = json.Unmarshal(proposed, &policy.Proposed)
	return policy, err
}
func (s *Store) ProposeEvidencePolicy(ctx context.Context, accountID string, items []workflow.EvidenceRequirement) (workflow.ServiceEvidencePolicy, error) {
	if err := workflow.ValidateEvidenceRequirements(items); err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	var profile string
	err = s.pool.QueryRow(ctx, `INSERT INTO professional_evidence_policies(profile_id,proposed,status) SELECT id,$2,'pending' FROM professional_profiles WHERE account_id=$1 ON CONFLICT(profile_id) DO UPDATE SET proposed=$2,status='pending',updated_at=now() RETURNING profile_id`, accountID, raw).Scan(&profile)
	if err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	return s.EvidencePolicy(ctx, profile)
}
func (s *Store) ReviewEvidencePolicy(ctx context.Context, operatorID, profileID string, approve bool) (workflow.ServiceEvidencePolicy, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	defer tx.Rollback(ctx)
	if err = assertOperator(ctx, tx, operatorID); err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	var proposal []byte
	if err = tx.QueryRow(ctx, `SELECT proposed FROM professional_evidence_policies WHERE profile_id=$1 AND status='pending' FOR UPDATE`, profileID).Scan(&proposal); err != nil {
		return workflow.ServiceEvidencePolicy{}, errors.New("evidence policy is not awaiting review")
	}
	var items []workflow.EvidenceRequirement
	if err = json.Unmarshal(proposal, &items); err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	if err = workflow.ValidateEvidenceRequirements(items); err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	status := "rejected"
	if approve {
		status = "approved"
	}
	_, err = tx.Exec(ctx, `UPDATE professional_evidence_policies SET requirements=CASE WHEN $3 THEN proposed ELSE requirements END,status=$2,reviewed_at=now(),reviewed_by=$4,updated_at=now() WHERE profile_id=$1`, profileID, status, approve, operatorID)
	if err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.ServiceEvidencePolicy{}, err
	}
	return s.EvidencePolicy(ctx, profileID)
}

func (s *Store) ListEvidencePolicies(ctx context.Context) ([]workflow.ServiceEvidencePolicy, error) {
	rows, err := s.pool.Query(ctx, `SELECT profile_id FROM professional_evidence_policies WHERE status='pending' ORDER BY updated_at LIMIT 100`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	items := []workflow.ServiceEvidencePolicy{}
	for _, id := range ids {
		item, err := s.EvidencePolicy(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
