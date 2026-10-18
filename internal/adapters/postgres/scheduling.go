package postgres

import (
	"context"
	"encoding/json"
	"errors"
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
		if err := s.materializeRecurringAvailability(ctx); err != nil && ctx.Err() == nil {
			slog.Error("recurring availability materialization failed", "error", err)
		}
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
	cursor := ""
	for {
		next, count, err := s.scheduleLifecycleBatch(ctx, cursor)
		if err != nil {
			return err
		}
		if count < 100 {
			return nil
		}
		cursor = next
	}
}

func (s *Store) scheduleLifecycleBatch(ctx context.Context, cursor string) (string, int, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return cursor, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id,state_json FROM account_workspaces
  WHERE id>$1 AND COALESCE((state_json->>'cancelled')::boolean,false)=false
  AND COALESCE((state_json #>> '{job,paid}')::boolean,false)=false
  AND NULLIF(state_json #>> '{job,id}','') IS NOT NULL
  ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, cursor)
	if err != nil {
		return cursor, 0, err
	}
	type item struct {
		id  string
		raw []byte
	}
	items := make([]item, 0, 100)
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.id, &value.raw); err != nil {
			rows.Close()
			return cursor, 0, err
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return cursor, 0, err
	}
	rows.Close()
	now := time.Now().UTC()
	for _, value := range items {
		cursor = value.id
		var state workflow.State
		if err := json.Unmarshal(value.raw, &state); err != nil {
			return cursor, 0, err
		}
		state.WorkspaceID = value.id
		if err := hydrateWaitingTimes(ctx, tx, &state); err != nil {
			return cursor, 0, err
		}
		job := state.Job
		if job.Appointment != nil && job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status == "confirmed" && !state.Cancelled && !job.Paid {
			start, err := time.Parse(time.RFC3339, job.Appointment.StartUTC)
			if err == nil && start.After(now) && !start.After(now.Add(24*time.Hour)) {
				anchor := job.Appointment.StartUTC + ":" + job.Appointment.EndUTC
				for _, role := range []string{"customer", "professional"} {
					if err := enqueueLifecycleNotice(ctx, tx, s.river, job.ID, "appointment-reminder", fmt.Sprintf("scope:%d:%s", job.ScopeVersion, anchor), role); err != nil {
						return cursor, 0, err
					}
				}
			}
		}
		for _, wait := range workflow.WaitingStates(state) {
			if !wait.Due(now) {
				continue
			}
			if paused, err := coordinationPausesReminder(ctx, tx, job, wait.EventType); err != nil {
				return cursor, 0, err
			} else if paused {
				continue
			}
			if wait.EventType == "checkout-reminder" {
				status, err := paymentStatusTx(ctx, tx, job)
				if err != nil {
					return cursor, 0, err
				}
				if !checkoutReminderActionable(status) {
					continue
				}
			}
			if err := enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: job.ID, EventKey: wait.EventKey(job), EventType: wait.EventType, RecipientRole: wait.ResponsibleRole}); err != nil {
				return cursor, 0, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return cursor, 0, err
	}
	return cursor, len(items), nil
}

// Existing workspaces use durable audit timestamps until their next explicit
// proposal or approval writes the timestamp into the journey snapshot.
type waitingQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func hydrateWaitingTimes(ctx context.Context, tx waitingQuerier, state *workflow.State) error {
	job := &state.Job
	rows, err := tx.Query(ctx, `SELECT action,max(created_at) FROM audit_events WHERE workspace_id=$1 AND scope_version=$2 AND action=ANY($3::text[]) GROUP BY action`, state.WorkspaceID, job.ScopeVersion, []string{"propose-price", "propose-date", "propose-change", "approve-payment", "add-evidence"})
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var at time.Time
		if err := rows.Scan(&action, &at); err != nil {
			return err
		}
		stamp := at.UTC().Format(time.RFC3339)
		switch action {
		case "propose-price":
			if job.PriceProposal != nil && job.PriceProposal.CreatedAt == "" {
				job.PriceProposal.CreatedAt = stamp
			}
		case "propose-date":
			if job.DateProposal != nil && job.DateProposal.CreatedAt == "" {
				job.DateProposal.CreatedAt = stamp
			}
		case "propose-change":
			if job.Change != nil && job.Change.CreatedAt == "" {
				job.Change.CreatedAt = stamp
			}
		case "approve-payment":
			if job.PaymentApprovedAt == "" {
				job.PaymentApprovedAt = stamp
			}
		case "add-evidence":
			if job.DetailAddedAt == "" {
				job.DetailAddedAt = stamp
			}
		}
	}
	return rows.Err()
}

func paymentStatusTx(ctx context.Context, tx waitingQuerier, job workflow.Job) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM paypal_payment_operations WHERE job_id=$1 AND scope_version=$2`, job.ID, job.ScopeVersion).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "not_started", nil
	}
	return status, err
}

func checkoutReminderActionable(status string) bool {
	switch status {
	case "capturing", "capture_uncertain", "captured", "refunding", "refund_uncertain", "refunded", "reversed", "release_pending", "voiding", "void_uncertain":
		return false
	}
	return true
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
		if err := s.enqueueAuthorizationReleaseTx(ctx, tx, item.id, state.Job); err != nil {
			return err
		}
		state.Job.PaymentAuthorizedVersion = 0
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
