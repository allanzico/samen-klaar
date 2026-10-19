package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"sort"
	"strings"
	"time"
)

func (s *Store) ParentRequest(ctx context.Context, accountID, workspaceID string) (workflow.ParentRequest, error) {
	var parentID string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(state_json->>'parentRequestId',id) FROM account_workspaces WHERE id=$1 AND account_id=$2`, workspaceID, accountID).Scan(&parentID); err != nil {
		return workflow.ParentRequest{}, errors.New("customer request was not found")
	}
	result := workflow.ParentRequest{ID: parentID, Jobs: []workflow.RequestJob{}, Timeline: []workflow.RequestTimelineEvent{}}
	if err := s.pool.QueryRow(ctx, `SELECT description FROM parent_requests WHERE id=$1 AND account_id=$2`, parentID, accountID).Scan(&result.Description); err != nil {
		return result, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,state_json,updated_at FROM account_workspaces WHERE account_id=$1 AND COALESCE(state_json->>'parentRequestId',id)=$2 ORDER BY created_at,id`, accountID, parentID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var raw []byte
		var updated time.Time
		if err = rows.Scan(&id, &raw, &updated); err != nil {
			return result, err
		}
		var state workflow.State
		if err = json.Unmarshal(raw, &state); err != nil {
			return result, err
		}
		state.WorkspaceID = id
		summary := workflow.SummarizeWorkspace(state, "customer", updated)
		result.Jobs = append(result.Jobs, workflow.RequestJob{Workspace: summary, ScopeVersion: state.Job.ScopeVersion, TotalMinor: state.Job.Quote.TotalMinor, Currency: state.Job.Quote.Currency, Professional: state.Job.Professional})
		for _, event := range state.Job.Timeline {
			result.Timeline = append(result.Timeline, workflow.RequestTimelineEvent{WorkspaceID: id, JobTitle: summary.Title, Event: event})
		}
	}
	sort.SliceStable(result.Timeline, func(i, j int) bool { return result.Timeline[i].Event.At < result.Timeline[j].Event.At })
	return result, rows.Err()
}

// AddRequestJobs creates only proposed independent job intakes. It cannot select
// a professional, approve a quote, reserve a visit or create a payment.
func (s *Store) AddRequestJobs(ctx context.Context, accountID, workspaceID string, plans []workflow.JobPlan, locale string) (workflow.ParentRequest, error) {
	if len(plans) < 1 || len(plans) > 5 {
		return workflow.ParentRequest{}, errors.New("add between one and five separately described jobs")
	}
	seen := map[string]bool{}
	for _, p := range plans {
		if len(p.ID) < 8 || len(p.ID) > 80 || seen[p.ID] || len(strings.TrimSpace(p.Description)) < 8 || len(p.Description) > 1000 {
			return workflow.ParentRequest{}, errors.New("each job needs a distinct plan ID and an 8 to 1000 character description")
		}
		seen[p.ID] = true
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return workflow.ParentRequest{}, err
	}
	defer tx.Rollback(ctx)
	var parentID string
	var original []byte
	if err = tx.QueryRow(ctx, `SELECT COALESCE(state_json->>'parentRequestId',id),state_json FROM account_workspaces WHERE id=$1 AND account_id=$2`, workspaceID, accountID).Scan(&parentID, &original); err != nil {
		return workflow.ParentRequest{}, errors.New("customer request was not found")
	}
	var lockedParent string
	if err = tx.QueryRow(ctx, `SELECT id FROM parent_requests WHERE id=$1 AND account_id=$2 FOR UPDATE`, parentID, accountID).Scan(&lockedParent); err != nil {
		return workflow.ParentRequest{}, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM account_workspaces WHERE account_id=$1 AND COALESCE(state_json->>'parentRequestId',id)=$2`, accountID, parentID).Scan(&count); err != nil {
		return workflow.ParentRequest{}, err
	}
	if count+len(plans) > 8 {
		return workflow.ParentRequest{}, errors.New("this request already has the maximum number of job plans")
	}
	adding := 0
	for _, plan := range plans {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM request_job_plans WHERE parent_request_id=$1 AND plan_id=$2)`, parentID, plan.ID).Scan(&exists); err != nil {
			return workflow.ParentRequest{}, err
		}
		if !exists {
			adding++
		}
	}
	if adding > 0 {
		open, err := lockOpenRequestCountTx(ctx, tx, accountID, "")
		if err != nil {
			return workflow.ParentRequest{}, err
		}
		if err := workflow.CheckOpenRequestLimit(open, adding); err != nil {
			return workflow.ParentRequest{}, err
		}
	}
	for _, plan := range plans {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM request_job_plans WHERE parent_request_id=$1 AND plan_id=$2)`, parentID, plan.ID).Scan(&exists); err != nil {
			return workflow.ParentRequest{}, err
		}
		if exists {
			var desc string
			if err = tx.QueryRow(ctx, `SELECT description FROM request_job_plans WHERE parent_request_id=$1 AND plan_id=$2`, parentID, plan.ID).Scan(&desc); err != nil {
				return workflow.ParentRequest{}, err
			}
			if desc != plan.Description {
				return workflow.ParentRequest{}, errors.New("a replayed job plan must retain its original description")
			}
			continue
		}
		state := workflow.NewState()
		state.WorkspaceID = rand.Text()
		state.ParentRequestID = parentID
		suggestion := workflow.InferIntake(plan.Description)
		if err = workflow.Apply(&state, workflow.Action{Actor: "customer", Action: "start-intake", Value: plan.Description, Suggestion: &suggestion, Locale: locale}); err != nil {
			return workflow.ParentRequest{}, err
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return workflow.ParentRequest{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_workspaces(id,account_id,state_json) VALUES($1,$2,$3)`, state.WorkspaceID, accountID, raw); err != nil {
			return workflow.ParentRequest{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO request_job_plans(parent_request_id,plan_id,workspace_id,description) VALUES($1,$2,$3,$4)`, parentID, plan.ID, state.WorkspaceID, plan.Description); err != nil {
			return workflow.ParentRequest{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.ParentRequest{}, err
	}
	return s.ParentRequest(ctx, accountID, workspaceID)
}
