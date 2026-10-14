package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

func (s *Store) runAppointmentConfirmationExpiry(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.expirePendingAppointmentConfirmations(ctx); err != nil && ctx.Err() == nil {
			slog.Error("appointment confirmation expiry pass failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) runJobLifecycleNotifications(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.scheduleLifecycleNotifications(ctx); err != nil && ctx.Err() == nil {
			slog.Error("job lifecycle notification pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// scheduleLifecycleNotifications queues durable reminders for the confirmed
// visit, missing evidence, completion review and payment approval. Every key
// is tied to the relevant appointment or workflow timestamp, so retries and
// multiple API instances cannot fan out duplicate email intents.
func (s *Store) scheduleLifecycleNotifications(ctx context.Context) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT w.id,w.state_json FROM account_workspaces w
		WHERE COALESCE((w.state_json ->> 'cancelled')::boolean,false)=false
		AND NULLIF(w.state_json #>> '{job,id}','') IS NOT NULL
		AND EXISTS (
		  SELECT 1 FROM (VALUES
		    (CASE WHEN w.state_json #>> '{job,appointmentConfirmation,status}'='confirmed'
		       AND NULLIF(w.state_json #>> '{job,appointment,startUtc}','')::timestamptz > now()
		       AND NULLIF(w.state_json #>> '{job,appointment,startUtc}','')::timestamptz <= now()+interval '24 hours'
		      THEN concat(w.state_json #>> '{job,id}',':appointment-reminder:',
		        COALESCE(NULLIF(w.state_json #>> '{job,appointment,slotId}',''),w.state_json #>> '{job,appointment,startUtc}'),':customer') END),
		    (CASE WHEN w.state_json #>> '{job,appointmentConfirmation,status}'='confirmed'
		       AND NULLIF(w.state_json #>> '{job,appointment,startUtc}','')::timestamptz > now()
		       AND NULLIF(w.state_json #>> '{job,appointment,startUtc}','')::timestamptz <= now()+interval '24 hours'
		      THEN concat(w.state_json #>> '{job,id}',':appointment-reminder:',
		        COALESCE(NULLIF(w.state_json #>> '{job,appointment,slotId}',''),w.state_json #>> '{job,appointment,startUtc}'),':professional') END),
		    (CASE WHEN COALESCE((w.state_json #>> '{job,workStarted}')::boolean,false)
		       AND NOT COALESCE((w.state_json #>> '{job,evidenceSubmitted}')::boolean,false)
		       AND NULLIF(w.state_json #>> '{job,appointment,endUtc}','')::timestamptz <= now()-interval '24 hours'
		      THEN concat(w.state_json #>> '{job,id}',':evidence-overdue:',
		        w.state_json #>> '{job,workStartedAt}',w.state_json #>> '{job,appointment,endUtc}',':professional') END),
		    (CASE WHEN COALESCE((w.state_json #>> '{job,evidenceSubmitted}')::boolean,false)
		       AND NOT COALESCE((w.state_json #>> '{job,completionApproved}')::boolean,false)
		       AND NULLIF(w.state_json #>> '{job,evidenceSubmittedAt}','')::timestamptz <= now()-interval '24 hours'
		      THEN concat(w.state_json #>> '{job,id}',':',
		        CASE WHEN COALESCE((w.state_json #>> '{job,detailAdded}')::boolean,false)
		          THEN 'completion-review-reminder' ELSE 'completion-note-overdue' END,':',
		        w.state_json #>> '{job,evidenceSubmittedAt}',':',
		        CASE WHEN COALESCE((w.state_json #>> '{job,detailAdded}')::boolean,false)
		          THEN 'customer' ELSE 'professional' END) END),
		    (CASE WHEN COALESCE((w.state_json #>> '{job,completionApproved}')::boolean,false)
		       AND COALESCE((w.state_json #>> '{job,paymentApprovedVersion}')::integer,0) < COALESCE((w.state_json #>> '{job,scopeVersion}')::integer,0)
		       AND COALESCE(w.state_json #>> '{job,dispute,status}','') IN ('','resolved','rework')
		       AND NULLIF(w.state_json #>> '{job,completionApprovedAt}','')::timestamptz <= now()-interval '24 hours'
		      THEN concat(w.state_json #>> '{job,id}',':payment-approval-reminder:',
		        w.state_json #>> '{job,completionApprovedAt}',':customer') END)
		  ) AS due(event_key)
		  WHERE due.event_key IS NOT NULL AND NOT EXISTS (
		    SELECT 1 FROM notification_outbox n
		    WHERE n.job_id=w.state_json #>> '{job,id}' AND n.event_key=due.event_key)
		) ORDER BY w.updated_at LIMIT 100 FOR UPDATE OF w SKIP LOCKED`)
	if err != nil {
		return err
	}
	type scheduledWorkspace struct {
		id        string
		stateJSON []byte
	}
	items := make([]scheduledWorkspace, 0)
	for rows.Next() {
		var item scheduledWorkspace
		if err := rows.Scan(&item.id, &item.stateJSON); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	now := time.Now().UTC()
	for _, item := range items {
		var state workflow.State
		if err := json.Unmarshal(item.stateJSON, &state); err != nil {
			return err
		}
		job := state.Job
		if job.ID == "" || state.Cancelled {
			continue
		}
		if job.Appointment != nil && job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status == "confirmed" {
			start, parseErr := time.Parse(time.RFC3339, job.Appointment.StartUTC)
			if parseErr == nil && start.After(now) && !start.After(now.Add(24*time.Hour)) {
				anchor := job.Appointment.SlotID
				if anchor == "" {
					anchor = job.Appointment.StartUTC
				}
				for _, role := range []string{"customer", "professional"} {
					if err := enqueueLifecycleNotice(ctx, tx, s.river, job.ID, "appointment-reminder", anchor, role); err != nil {
						return err
					}
				}
			}
		}
		if job.WorkStarted && !job.EvidenceSubmitted && job.Appointment != nil && dueAt(job.Appointment.EndUTC, now.Add(-24*time.Hour)) {
			if err := enqueueLifecycleNotice(ctx, tx, s.river, job.ID, "evidence-overdue", job.WorkStartedAt+job.Appointment.EndUTC, "professional"); err != nil {
				return err
			}
		}
		if job.EvidenceSubmitted && !job.CompletionApproved && dueAt(job.EvidenceSubmittedAt, now.Add(-24*time.Hour)) {
			eventType, role := "completion-review-reminder", "customer"
			if !job.DetailAdded {
				eventType, role = "completion-note-overdue", "professional"
			}
			if err := enqueueLifecycleNotice(ctx, tx, s.river, job.ID, eventType, job.EvidenceSubmittedAt, role); err != nil {
				return err
			}
		}
		if job.CompletionApproved && job.PaymentApprovedVersion != job.ScopeVersion &&
			(job.Dispute == nil || job.Dispute.Status == "resolved" || job.Dispute.Status == "rework") && dueAt(job.CompletionApprovedAt, now.Add(-24*time.Hour)) {
			if err := enqueueLifecycleNotice(ctx, tx, s.river, job.ID, "payment-approval-reminder", job.CompletionApprovedAt, "customer"); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func dueAt(value string, deadline time.Time) bool {
	parsed, err := time.Parse(time.RFC3339, value)
	return err == nil && !parsed.After(deadline)
}

func enqueueLifecycleNotice(ctx context.Context, tx pgx.Tx, riverClient *river.Client[pgx.Tx], jobID, eventType, anchor, role string) error {
	if anchor == "" {
		return nil
	}
	eventKey := fmt.Sprintf("%s:%s:%s:%s", jobID, eventType, anchor, role)
	if err := enqueueNotificationTx(ctx, tx, riverClient, NotificationArgs{JobID: jobID, EventKey: eventKey, EventType: eventType, RecipientRole: role}); err != nil {
		return fmt.Errorf("enqueue lifecycle reminder: %w", err)
	}
	return nil
}

// expirePendingAppointmentConfirmations releases overdue reservations and
// queues customer/professional notices in one transaction. SKIP LOCKED lets
// multiple API instances share the worker safely.
func (s *Store) expirePendingAppointmentConfirmations(ctx context.Context) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id, account_id, state_json, revision
		FROM account_workspaces
		WHERE state_json #>> '{job,appointmentConfirmation,status}' = 'pending'
		  AND COALESCE((state_json ->> 'cancelled')::boolean, false) = false
		  AND NULLIF(state_json #>> '{job,appointmentConfirmation,dueAt}', '')::timestamptz <= now()
		ORDER BY updated_at LIMIT 50 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	type expiredWorkspace struct {
		id, ownerID string
		stateJSON   []byte
		revision    int64
	}
	items := make([]expiredWorkspace, 0)
	for rows.Next() {
		var item expiredWorkspace
		if err := rows.Scan(&item.id, &item.ownerID, &item.stateJSON, &item.revision); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		var state workflow.State
		if err := json.Unmarshal(item.stateJSON, &state); err != nil {
			return err
		}
		confirmation := state.Job.AppointmentConfirmation
		if confirmation == nil || confirmation.Status != "pending" || state.Job.Appointment == nil {
			continue
		}
		dueAt, err := time.Parse(time.RFC3339, confirmation.DueAt)
		if err != nil || dueAt.After(time.Now()) {
			continue
		}
		if err := releaseCancelledBooking(ctx, tx, state.Job); err != nil {
			return err
		}
		confirmation.Status = "expired"
		confirmation.RespondedAt = time.Now().UTC().Format(time.RFC3339)
		state.Job.Appointment = nil
		state.Job.Timeline = append(state.Job.Timeline, workflow.Event{
			Text: "Professional confirmation window ended · visit released; choose another available time",
			Kind: "attention", When: time.Now().UTC().Format("15:04"),
		})
		state.WorkspaceID = item.id
		stateBytes, err := json.Marshal(state)
		if err != nil {
			return err
		}
		revision := item.revision + 1
		if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2, revision=$3, updated_at=now() WHERE id=$1`, item.id, stateBytes, revision); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id, actor_account_id, workspace_id, job_id, actor, action, scope_version, revision)
			VALUES ($1,NULL,$2,$3,'system','expire-appointment-confirmation',$4,$5)`, item.ownerID, item.id, state.Job.ID, int32(state.Job.ScopeVersion), revision); err != nil {
			return err
		}
		for _, recipient := range []string{"customer", "professional"} {
			eventKey := fmt.Sprintf("%s:%d:appointment-expired:%s", state.Job.ID, revision, recipient)
			if err := enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: state.Job.ID, EventKey: eventKey, EventType: "appointment-expired", RecipientRole: recipient}); err != nil {
				return fmt.Errorf("enqueue expired appointment notice: %w", err)
			}
		}
	}
	return tx.Commit(ctx)
}
