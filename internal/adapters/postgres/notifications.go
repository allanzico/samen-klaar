package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	mailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// NotificationArgs is a durable notification intent. It contains no message
// content or contact details until a delivery adapter is configured.
type NotificationArgs struct {
	JobID         string `json:"jobId"`
	EventKey      string `json:"eventKey"`
	EventType     string `json:"eventType"`
	RecipientRole string `json:"recipientRole"`
}

func (s *Store) NotificationDeliveryHealth(ctx context.Context) (workflow.NotificationDeliveryHealth, error) {
	var health workflow.NotificationDeliveryHealth
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='failed'),
		       COUNT(*) FILTER (WHERE status='adapter_unconfigured'),
		       COUNT(*) FILTER (WHERE status='sending' AND updated_at < now() - interval '15 minutes')
		FROM notification_outbox`).Scan(&health.Failed, &health.Unconfigured, &health.Stalled)
	return health, err
}

func (NotificationArgs) Kind() string { return "notification" }

type NotificationWorker struct {
	river.WorkerDefaults[NotificationArgs]
	pool       *pgxpool.Pool
	sender     mailadapter.Sender
	appBaseURL string
}

func (w *NotificationWorker) Work(ctx context.Context, job *river.Job[NotificationArgs]) error {
	args := job.Args
	if args.JobID == "" || args.EventKey == "" || args.EventType == "" {
		return fmt.Errorf("notification job is missing its event identity")
	}
	if args.RecipientRole != "customer" && args.RecipientRole != "professional" {
		return fmt.Errorf("notification recipient role is invalid")
	}
	if w.sender == nil {
		_, err := w.pool.Exec(ctx, `UPDATE notification_outbox SET status='adapter_unconfigured', updated_at=now()
			WHERE job_id=$1 AND event_key=$2 AND status <> 'sent'`, args.JobID, args.EventKey)
		return err
	}

	var emailAddress, displayName, workspaceID string
	lookup := w.pool.QueryRow(ctx, `
		SELECT account.email, account.display_name, workspace.id
		FROM account_workspaces workspace
	JOIN accounts account ON account.id = CASE WHEN $2 = 'customer'
			THEN workspace.account_id
			ELSE workspace.state_json #>> '{job,professionalAccountId}' END
		WHERE workspace.state_json #>> '{job,id}' = $1`, args.JobID, args.RecipientRole)
	if err := lookup.Scan(&emailAddress, &displayName, &workspaceID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, updateErr := w.pool.Exec(ctx, `UPDATE notification_outbox SET status='failed', last_error='recipient_unavailable', updated_at=now()
			WHERE job_id=$1 AND event_key=$2 AND status <> 'sent'`, args.JobID, args.EventKey); updateErr != nil {
			return updateErr
		}
		return nil
	}
	if strings.TrimSpace(emailAddress) == "" {
		_, err := w.pool.Exec(ctx, `UPDATE notification_outbox SET status='failed', last_error='recipient_email_missing', updated_at=now()
			WHERE job_id=$1 AND event_key=$2 AND status <> 'sent'`, args.JobID, args.EventKey)
		return err
	}

	var claimed bool
	if err := w.pool.QueryRow(ctx, `UPDATE notification_outbox
		SET status='sending', attempts=attempts+1, last_error='', updated_at=now()
		WHERE job_id=$1 AND event_key=$2 AND status IN ('queued','failed','adapter_unconfigured')
		RETURNING true`, args.JobID, args.EventKey).Scan(&claimed); err != nil {
		// A completed or concurrently claimed event must not be sent twice.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	link := strings.TrimRight(w.appBaseURL, "/") + "/?workspaceId=" + url.QueryEscape(workspaceID)
	if w.appBaseURL == "" {
		link = "http://localhost:8090/?workspaceId=" + url.QueryEscape(workspaceID)
	}
	messageID := notificationMessageID(args.EventKey)
	message := mailadapter.Message{
		To: emailAddress, ToName: displayName, Subject: notificationSubject(args.EventType),
		Body:      notificationBody(args.EventType, link),
		MessageID: messageID,
	}
	if err := w.sender.Send(ctx, message); err != nil {
		_, _ = w.pool.Exec(ctx, `UPDATE notification_outbox SET status='failed', last_error='delivery_failed', updated_at=now()
			WHERE job_id=$1 AND event_key=$2 AND status='sending'`, args.JobID, args.EventKey)
		return err
	}
	_, err := w.pool.Exec(ctx, `UPDATE notification_outbox SET status='sent', last_error='', updated_at=now()
		WHERE job_id=$1 AND event_key=$2 AND status='sending'`, args.JobID, args.EventKey)
	return err
}

func notificationSubject(eventType string) string {
	subjects := map[string]string{
		"cancel-request":             "Service request cancellation update",
		"send-message":               "New message about your service request",
		"propose-price":              "New service price proposal",
		"accept-price":               "Service price proposal accepted",
		"decline-price":              "Service price proposal declined",
		"propose-date":               "New visit time proposal",
		"accept-date":                "Visit time proposal accepted",
		"decline-date":               "Visit time proposal declined",
		"approve-and-book":           "Service visit request update",
		"book-appointment":           "Service visit request update",
		"confirm-appointment":        "Your service visit is confirmed",
		"decline-appointment":        "Choose another service visit time",
		"appointment-expired":        "Choose another service visit time",
		"start-job":                  "Your service visit has started",
		"submit-evidence":            "Work completion is ready for review",
		"approve-completion":         "Work completion approved",
		"approve-payment":            "Payment approval recorded",
		"confirm-payment":            "Service payment complete",
		"refund-completed":           "Your service payment has been refunded",
		"payment-review":             "Your service payment needs review",
		"appointment-reminder":       "Your confirmed service visit is tomorrow",
		"evidence-overdue":           "Completion evidence is due for your service request",
		"completion-note-overdue":    "Add the work note needed for completion review",
		"completion-review-reminder": "Review your service completion",
		"payment-approval-reminder":  "Review the payment for your completed service",
		"submit-review":              "A customer reviewed your completed service",
		"respond-to-review":          "Your professional responded to your review",
	}
	if subject, ok := subjects[eventType]; ok {
		return subject
	}
	return "Service request update"
}

func notificationBody(eventType, link string) string {
	opening := "There is an update to a service request in your SamenKlaar account."
	switch eventType {
	case "appointment-reminder":
		opening = "Your confirmed service visit is scheduled within the next 24 hours. Review the visit details in your account."
	case "evidence-overdue":
		opening = "The service visit has passed, and completion evidence has not been submitted. Upload the required evidence and submit the work for review."
	case "completion-note-overdue":
		opening = "The customer is waiting for a completion review. Add the work note needed before they can review the submitted evidence."
	case "completion-review-reminder":
		opening = "Completion evidence is ready for your review. Approve completion only when the submitted work matches the agreed scope."
	case "payment-approval-reminder":
		opening = "Your completed service is waiting for a separate payment decision. PayPal will only capture payment after you approve it."
	case "payment-review":
		opening = "PayPal reported a payment status that needs operator review. Open your request for the current status."
	}
	return opening + "\n\nOpen your request: " + link + "\n\nYou received this message because you have an active service request."
}

func notificationMessageID(eventKey string) string {
	sum := sha256.Sum256([]byte(eventKey))
	return hex.EncodeToString(sum[:16]) + "@samen-klaar"
}

func requeueUndeliveredNotifications(ctx context.Context, store *Store) error {
	for {
		count, err := requeueNotificationOutbox(ctx, store)
		if err != nil || count < 100 {
			return err
		}
	}
}

func requeueNotificationOutbox(ctx context.Context, store *Store) (int, error) {
	if store.sender == nil || store.river == nil {
		return 0, nil
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// SMTP cannot provide exactly-once delivery across a process crash after
	// acceptance but before the outbox is marked sent. Reclaim old claims so a
	// notification is not silently lost; deterministic Message-IDs let mail
	// systems deduplicate where supported.
	if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='failed',last_error='delivery_interrupted',updated_at=now()
		WHERE status='sending' AND updated_at < now() - interval '15 minutes'`); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT job_id, event_key, event_type, recipient_role FROM notification_outbox
		WHERE status='adapter_unconfigured'
		   OR status='failed' AND (
		      last_error='delivery_interrupted'
		      OR last_error='delivery_failed' AND attempts < 5 AND updated_at < now()-interval '5 minutes')
		ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	items := make([]NotificationArgs, 0)
	for rows.Next() {
		var item NotificationArgs
		if err := rows.Scan(&item.JobID, &item.EventKey, &item.EventType, &item.RecipientRole); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, item := range items {
		if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='queued', last_error='', updated_at=now()
			WHERE job_id=$1 AND event_key=$2`, item.JobID, item.EventKey); err != nil {
			return 0, err
		}
		if _, err := store.river.InsertTx(ctx, tx, item, nil); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(items), nil
}

func (s *Store) runNotificationRecovery(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := requeueUndeliveredNotifications(ctx, s); err != nil && ctx.Err() == nil {
			slog.Error("notification outbox recovery pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func notificationForAction(input workflow.Action, job workflow.Job) (eventType, recipient string, ok bool) {
	if job.ID == "" {
		return "", "", false
	}
	switch input.Action {
	case "cancel-request", "withdraw-price", "withdraw-date", "send-message", "propose-price", "accept-price", "decline-price", "propose-date", "accept-date", "decline-date":
		if input.Actor == "customer" {
			return input.Action, "professional", true
		}
		return input.Action, "customer", true
	case "answer-clarification", "approve-quote", "book-appointment", "approve-and-book", "request-details", "approve-change", "decline-change", "approve-completion", "approve-payment", "confirm-payment", "set-service-address":
		return input.Action, "professional", true
	case "confirm-appointment", "decline-appointment", "start-job", "submit-evidence", "add-evidence", "propose-change":
		return input.Action, "customer", true
	case "submit-review":
		return input.Action, "professional", true
	case "respond-to-review":
		return input.Action, "customer", true
	case "report-issue":
		return "issue-reported", "professional", true
	default:
		return "", "", false
	}
}
