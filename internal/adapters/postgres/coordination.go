package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type ContextCoordinator interface {
	Coordinate(context.Context, workflow.CoordinationRequest) (workflow.CoordinationResponse, error)
}

func (s *Store) SetCoordinator(provider ContextCoordinator) {
	s.coordinatorMu.Lock()
	defer s.coordinatorMu.Unlock()
	s.coordinator = provider
}

type CoordinationArgs struct {
	TaskID string `json:"taskId"`
}

func (CoordinationArgs) Kind() string { return "contextual_coordination" }

type CoordinationWorker struct {
	river.WorkerDefaults[CoordinationArgs]
	store *Store
}

func (w *CoordinationWorker) Work(ctx context.Context, job *river.Job[CoordinationArgs]) error {
	return w.store.processCoordinationTask(ctx, job.Args.TaskID)
}

// The caller holds the workspace lock and has persisted the new snapshot in
// this transaction. The unique trigger identity allows one question per round.
func (s *Store) enqueueCoordinationTx(ctx context.Context, tx pgx.Tx, workspaceID string, state workflow.State, input workflow.Action, revision int64) error {
	job := state.Job
	if job.ID == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE coordination_tasks SET status='obsolete',outcome='The job scope or completion round changed.',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND status NOT IN ('completed','obsolete') AND (job_id<>$2 OR scope_version<>$3 OR completion_round<>$4)`, workspaceID, job.ID, job.ScopeVersion, workflow.EvidenceRound(job)); err != nil {
		return err
	}
	if workflow.HasCurrentWorkNote(job) {
		_, err := tx.Exec(ctx, `UPDATE coordination_tasks SET status='completed',outcome='The professional supplied the current completion work note.',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3 AND completion_round=$4 AND status NOT IN ('completed','obsolete')`, workspaceID, job.ID, job.ScopeVersion, workflow.EvidenceRound(job))
		return err
	}
	if state.Cancelled || job.Paid {
		_, err := tx.Exec(ctx, `UPDATE coordination_tasks SET status='obsolete',outcome='The request no longer requires this follow-up.',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND status NOT IN ('completed','obsolete')`, workspaceID)
		return err
	}
	if job.ProfessionalAccountID == "" {
		return nil
	}
	if job.Dispute != nil && job.Dispute.Status != "resolved" && job.Dispute.Status != "rework" || state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
		_, err := tx.Exec(ctx, `UPDATE coordination_tasks SET status='operator_review',proposed_action='operator-review',outcome='An unresolved issue or cancellation requires an operator response.',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND status IN ('queued','awaiting_reply','queued_summary','summary_ready')`, workspaceID)
		return err
	}
	trigger, message := "", ""
	switch input.Action {
	case "record-evidence", "submit-evidence":
		for _, e := range job.Evidence {
			if e.Kind != "note" && workflow.CurrentEvidence(job, e) {
				trigger = "completion-note"
				break
			}
		}
	case "send-message":
		if job.QuoteApproved && job.Appointment != nil && scopeMessageNeedsClarification(input.Value) {
			trigger = "post-booking-scope"
			message = strings.TrimSpace(input.Value)
		}
	}
	if trigger == "" {
		return nil
	}
	if s.river == nil {
		return errors.New("contextual coordination queue is unavailable")
	}
	id := "coord_" + rand.Text()
	tag, err := tx.Exec(ctx, `INSERT INTO coordination_tasks(id,workspace_id,job_id,scope_version,completion_round,trigger,source_message,workspace_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(workspace_id,job_id,scope_version,completion_round,trigger) DO NOTHING`, id, workspaceID, job.ID, job.ScopeVersion, workflow.EvidenceRound(job), trigger, message, revision)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	_, err = s.river.InsertTx(ctx, tx, CoordinationArgs{TaskID: id}, nil)
	return err
}

func scopeMessageNeedsClarification(value string) bool {
	lower := strings.ToLower(value)
	for _, phrase := range []string{"also ", "extra", "additional", "different", "change ", "scope", "not sure", "unclear", "ook ", "meer ", "anders", "wijzig", "weet niet", "onduidelijk"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

const coordinationColumns = `id,workspace_id,job_id,scope_version,completion_round,trigger,recipient_role,status,question,reply,summary,proposed_action,outcome,revision,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func scanCoordinationTask(row pgx.Row) (workflow.CoordinationTask, error) {
	var task workflow.CoordinationTask
	err := row.Scan(&task.ID, &task.WorkspaceID, &task.JobID, &task.ScopeVersion, &task.Round, &task.Trigger, &task.RecipientRole, &task.Status, &task.Question, &task.Reply, &task.Summary, &task.ProposedAction, &task.Outcome, &task.Revision, &task.CreatedAt, &task.UpdatedAt)
	return task, err
}

func (s *Store) ListCoordinationTasks(ctx context.Context, actorID, role, workspaceID string) ([]workflow.CoordinationTask, error) {
	if _, err := s.LoadWorkspace(ctx, actorID, role, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+coordinationColumns+` FROM coordination_tasks WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 20`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]workflow.CoordinationTask, 0)
	for rows.Next() {
		task, err := scanCoordinationTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) ReplyCoordinationTask(ctx context.Context, actorID, workspaceID, taskID, reply string) error {
	reply = strings.TrimSpace(reply)
	if len(reply) < 3 || len(reply) > 1500 {
		return errors.New("reply with 3 to 1500 characters about the agreed work")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	state, err := lockPaymentWorkspace(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	if state.Job.ProfessionalAccountID != actorID {
		return errors.New("only the assigned professional can answer this follow-up")
	}
	task, err := scanCoordinationTask(tx.QueryRow(ctx, `SELECT `+coordinationColumns+` FROM coordination_tasks WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, taskID, workspaceID))
	if err != nil {
		return errors.New("coordination task was not found")
	}
	if task.Status != "awaiting_reply" || task.JobID != state.Job.ID || task.ScopeVersion != state.Job.ScopeVersion || task.Round != workflow.EvidenceRound(state.Job) || state.Cancelled || state.Job.Paid || workflow.HasCurrentWorkNote(state.Job) {
		return errors.New("this follow-up is no longer awaiting a reply")
	}
	status, outcome := "queued_summary", ""
	if workflow.CoordinationReplyUncertain(reply) {
		status = "operator_review"
		outcome = "The professional could not provide a clear factual explanation. An operator must contact both parties."
	}
	if _, err := tx.Exec(ctx, `UPDATE coordination_tasks SET reply=$2,status=$3,outcome=$4,proposed_action=CASE WHEN $3='operator_review' THEN 'operator-review' ELSE proposed_action END,revision=revision+1,updated_at=now() WHERE id=$1`, taskID, reply, status, outcome); err != nil {
		return err
	}
	if status == "queued_summary" {
		if s.river == nil {
			return errors.New("coordination queue is unavailable")
		}
		if _, err := s.river.InsertTx(ctx, tx, CoordinationArgs{TaskID: taskID}, nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) processCoordinationTask(ctx context.Context, taskID string) error {
	// Resolve the workspace before locking the task, preserving the repository's
	// workspace-first lock order for actions, worker retries and replies.
	var workspaceID string
	if err := s.pool.QueryRow(ctx, `SELECT workspace_id FROM coordination_tasks WHERE id=$1`, taskID).Scan(&workspaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	state, err := lockPaymentWorkspace(ctx, tx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	task, err := scanCoordinationTask(tx.QueryRow(ctx, `SELECT `+coordinationColumns+` FROM coordination_tasks WHERE id=$1 FOR UPDATE`, taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	finish := func(status, outcome string) error {
		_, err := tx.Exec(ctx, `UPDATE coordination_tasks SET status=$2,outcome=$3,proposed_action=CASE WHEN $2='operator_review' THEN 'operator-review' ELSE proposed_action END,revision=revision+1,updated_at=now() WHERE id=$1`, taskID, status, outcome)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if task.Status != "queued" && task.Status != "queued_summary" {
		return nil
	}
	if task.JobID != state.Job.ID || task.ScopeVersion != state.Job.ScopeVersion || task.Round != workflow.EvidenceRound(state.Job) || state.Cancelled || state.Job.Paid {
		return finish("obsolete", "The request scope or completion round changed before the follow-up was delivered.")
	}
	if workflow.HasCurrentWorkNote(state.Job) {
		return finish("completed", "The professional supplied the current completion work note.")
	}
	if state.Job.Dispute != nil && state.Job.Dispute.Status != "resolved" && state.Job.Dispute.Status != "rework" || state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
		return finish("operator_review", "An unresolved issue or cancellation requires an operator response.")
	}
	if task.Trigger == "completion-note" {
		current := false
		for _, e := range state.Job.Evidence {
			if e.Kind != "note" && workflow.CurrentEvidence(state.Job, e) {
				current = true
			}
		}
		if !current {
			return finish("obsolete", "The original completion evidence is no longer current.")
		}
	}
	s.coordinatorMu.RLock()
	provider := s.coordinator
	s.coordinatorMu.RUnlock()
	if provider == nil {
		// Startup installs the provider after opening the durable queue. Do not
		// turn that brief wiring interval into a permanent operator case.
		return river.JobSnooze(time.Minute)
	}
	locale := "en"
	if err := tx.QueryRow(ctx, `SELECT locale FROM accounts WHERE id=$1`, state.Job.ProfessionalAccountID).Scan(&locale); err != nil {
		return finish("operator_review", "The assigned professional is unavailable; an operator must contact the participants.")
	}
	var message string
	if err := tx.QueryRow(ctx, `SELECT source_message FROM coordination_tasks WHERE id=$1`, taskID).Scan(&message); err != nil {
		return err
	}
	input := coordinationRequest(state, task, locale, message)
	callCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	response, err := provider.Coordinate(callCtx, input)
	if err != nil || !workflow.ValidCoordinationResponseForRequest(input, response) {
		return finish("operator_review", "A reliable explanation could not be prepared. An operator must review the supplied facts.")
	}
	if response.NeedsOperator {
		return finish("operator_review", response.Reason)
	}
	status, question, summary, recipient := "awaiting_reply", response.Question, "", "professional"
	if task.Reply != "" {
		status, question, summary, recipient = "summary_ready", task.Question, response.Summary, "customer"
	}
	if _, err := tx.Exec(ctx, `UPDATE coordination_tasks SET status=$2,question=$3,summary=$4,proposed_action=$5,revision=revision+1,updated_at=now() WHERE id=$1`, taskID, status, question, summary, response.ProposedAction); err != nil {
		return err
	}
	event := "coordination-question"
	if status == "summary_ready" {
		event = "coordination-summary"
	}
	if err := enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: task.JobID, EventKey: fmt.Sprintf("%s:%s", task.ID, event), EventType: event, RecipientRole: recipient}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func coordinationRequest(state workflow.State, task workflow.CoordinationTask, locale, message string) workflow.CoordinationRequest {
	redact := func(value string) string {
		if state.Job.ServiceAddress != "" {
			return strings.ReplaceAll(value, state.Job.ServiceAddress, "[address omitted]")
		}
		return value
	}
	scope := redact(state.Job.Description)
	for _, item := range state.Job.WorkItems {
		scope += "\n" + redact(item.Description)
	}
	labels := make([]string, 0)
	for _, e := range state.Job.Evidence {
		if workflow.CurrentEvidence(state.Job, e) {
			labels = append(labels, redact(e.Label))
			if len(labels) == 12 {
				break
			}
		}
	}
	return workflow.CoordinationRequest{WorkspaceID: state.WorkspaceID, JobID: state.Job.ID, ScopeVersion: state.Job.ScopeVersion, Round: workflow.EvidenceRound(state.Job), Trigger: task.Trigger, Locale: locale, JobTitle: state.Job.Title, AgreedScope: scope, Message: redact(message), Reply: redact(task.Reply), EvidenceLabels: labels}
}

func (s *Store) coordinationExceptions(ctx context.Context) ([]workflow.OperatorCase, error) {
	rows, err := s.pool.Query(ctx, `SELECT t.workspace_id,w.account_id,t.job_id,w.state_json #>> '{job,title}',COALESCE(NULLIF(t.outcome,''),'The coordination follow-up has waited for 24 hours. An operator must contact the assigned professional and record the missing facts.'),to_char(t.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM coordination_tasks t JOIN account_workspaces w ON w.id=t.workspace_id WHERE (t.status='operator_review' OR t.status IN ('queued','awaiting_reply','queued_summary') AND t.updated_at<=now()-interval '24 hours') AND t.job_id=w.state_json #>> '{job,id}' AND t.scope_version=COALESCE((w.state_json #>> '{job,scopeVersion}')::integer,0) AND t.completion_round=GREATEST(COALESCE((w.state_json #>> '{job,completionRound}')::integer,1),1) AND NOT COALESCE((w.state_json->>'cancelled')::boolean,false) ORDER BY t.updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cases := make([]workflow.OperatorCase, 0)
	for rows.Next() {
		var item workflow.OperatorCase
		if err := rows.Scan(&item.WorkspaceID, &item.AccountID, &item.JobID, &item.JobTitle, &item.Description, &item.ReportedAt); err != nil {
			return nil, err
		}
		item.Kind = "coordination-review"
		item.ID = "coordination:" + item.WorkspaceID
		cases = append(cases, item)
	}
	return cases, rows.Err()
}

func coordinationPausesReminder(ctx context.Context, q waitingQuerier, job workflow.Job, event string) (bool, error) {
	if event != "completion-note-overdue" && event != "evidence-overdue" {
		return false, nil
	}
	var paused bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coordination_tasks WHERE job_id=$1 AND scope_version=$2 AND completion_round=$3 AND status='operator_review')`, job.ID, job.ScopeVersion, workflow.EvidenceRound(job)).Scan(&paused)
	return paused, err
}
