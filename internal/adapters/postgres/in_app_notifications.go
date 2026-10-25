package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

func (s *Store) ListNotifications(ctx context.Context, accountID, role string) ([]workflow.Notification, error) {
	if accountID == "" || role != "customer" && role != "professional" {
		return nil, errors.New("notification workspace is invalid")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_type, workspace_id, created_at, read_at
		FROM in_app_notifications
		WHERE recipient_account_id=$1 AND recipient_role=$2
		ORDER BY created_at DESC, id DESC
		LIMIT 50`, accountID, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]workflow.Notification, 0)
	for rows.Next() {
		var item workflow.Notification
		if err := rows.Scan(&item.ID, &item.EventType, &item.WorkspaceID, &item.CreatedAt, &item.ReadAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) MarkNotificationRead(ctx context.Context, accountID, role string, id int64) (bool, error) {
	if accountID == "" || role != "customer" && role != "professional" || id <= 0 {
		return false, errors.New("notification workspace is invalid")
	}
	var updated bool
	err := s.pool.QueryRow(ctx, `
		UPDATE in_app_notifications
		SET read_at=COALESCE(read_at, now())
		WHERE id=$1 AND recipient_account_id=$2 AND recipient_role=$3
		RETURNING true`, id, accountID, role).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return updated, err
}

func enqueueNotificationTx(ctx context.Context, tx pgx.Tx, riverClient *river.Client[pgx.Tx], args NotificationArgs) error {
	if args.JobID == "" || args.EventKey == "" || args.EventType == "" || args.RecipientRole != "customer" && args.RecipientRole != "professional" {
		return errors.New("notification identity is invalid")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO in_app_notifications (workspace_id,job_id,recipient_account_id,recipient_role,event_key,event_type)
		SELECT w.id, $1,
		       CASE $4 WHEN 'customer' THEN w.account_id ELSE w.state_json #>> '{job,professionalAccountId}' END,
		       $4, $2, $3
		FROM account_workspaces w
		JOIN accounts recipient ON recipient.id = CASE $4 WHEN 'customer' THEN w.account_id ELSE w.state_json #>> '{job,professionalAccountId}' END
		WHERE `+notificationRequestID("w")+` = $1
		ON CONFLICT (recipient_account_id,event_key) DO NOTHING`, args.JobID, args.EventKey, args.EventType, args.RecipientRole)
	if err != nil {
		return err
	}
	if !notificationUsesEmail(args.EventType) {
		return nil
	}
	// A deleted recipient can remain referenced by a historical workspace
	// snapshot. Keep the outbox intent for delivery monitoring, but do not create
	// an inbox row referencing an account that no longer exists.
	result, err := tx.Exec(ctx, `
		INSERT INTO notification_outbox (job_id,event_key,event_type,recipient_role,status)
		VALUES ($1,$2,$3,$4,'queued') ON CONFLICT (job_id,event_key) DO NOTHING`, args.JobID, args.EventKey, args.EventType, args.RecipientRole)
	if err != nil || result.RowsAffected() == 0 || riverClient == nil {
		return err
	}
	if _, err := riverClient.InsertTx(ctx, tx, args, nil); err != nil {
		return fmt.Errorf("enqueue in-app/email notification: %w", err)
	}
	return nil
}
