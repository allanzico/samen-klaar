package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// agentQuestionEvent is the notification sent when an agent stops and asks its person. Its event key is
// "agent-question:<escalation id>", so one email is sent per question and a stale one is dropped.
const agentQuestionEvent = "agent-question"

// notificationRequestID is the identity a notification uses for its request: the job once there is one,
// otherwise the workspace itself (an agent can ask a question before any professional is chosen).
func notificationRequestID(alias string) string {
	return "COALESCE(NULLIF(" + alias + ".state_json #>> '{job,id}',''), " + alias + ".id)"
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
		return errors.New("notification job is missing its event identity")
	}
	if args.RecipientRole != "customer" && args.RecipientRole != "professional" {
		return errors.New("notification recipient role is invalid")
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var emailAddress, displayName, workspaceID, locale string
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT COALESCE(account.email,''),COALESCE(account.display_name,''),workspace.id,COALESCE(account.locale,'en'),workspace.state_json
  FROM account_workspaces workspace
  LEFT JOIN accounts account ON account.id=CASE WHEN $2='customer' THEN workspace.account_id ELSE workspace.state_json #>> '{job,professionalAccountId}' END
  WHERE `+notificationRequestID("workspace")+`=$1 FOR SHARE OF workspace`, args.JobID, args.RecipientRole).Scan(&emailAddress, &displayName, &workspaceID, &locale, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='obsolete',last_error='workspace_unavailable',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status NOT IN ('sent','obsolete')`, args.JobID, args.EventKey); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	var state workflow.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	state.WorkspaceID = workspaceID
	if args.EventType == agentQuestionEvent {
		var stillOpen bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agent_escalations WHERE id=$1 AND workspace_id=$2 AND resolved_at IS NULL)`, agentQuestionID(args.EventKey), workspaceID).Scan(&stillOpen); err != nil {
			return err
		}
		if !stillOpen {
			if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='obsolete',last_error='action_no_longer_required',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status NOT IN ('sent','obsolete')`, args.JobID, args.EventKey); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	}
	if isActionableReminder(args.EventType) {
		if err := hydrateWaitingTimes(ctx, tx, &state); err != nil {
			return err
		}
		active := reminderMatchesState(args, state, time.Now().UTC())
		if active {
			paused, err := coordinationPausesReminder(ctx, tx, state.Job, args.EventType)
			if err != nil {
				return err
			}
			active = !paused
		}
		if active && args.EventType == "checkout-reminder" {
			status, err := paymentStatusTx(ctx, tx, state.Job)
			if err != nil {
				return err
			}
			active = checkoutReminderActionable(status)
		}
		if !active {
			if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='obsolete',last_error='action_no_longer_required',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status NOT IN ('sent','obsolete')`, args.JobID, args.EventKey); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	}
	if w.sender == nil {
		if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='adapter_unconfigured',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status NOT IN ('sent','obsolete')`, args.JobID, args.EventKey); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if strings.TrimSpace(emailAddress) == "" {
		if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='failed',last_error='recipient_email_missing',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status NOT IN ('sent','obsolete')`, args.JobID, args.EventKey); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var claimed bool
	if err := tx.QueryRow(ctx, `UPDATE notification_outbox SET status='sending',attempts=attempts+1,last_error='',updated_at=now()
  WHERE job_id=$1 AND event_key=$2 AND status IN ('queued','failed','adapter_unconfigured') RETURNING true`, args.JobID, args.EventKey).Scan(&claimed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	link := strings.TrimRight(w.appBaseURL, "/") + "/?workspaceId=" + url.QueryEscape(workspaceID)
	if w.appBaseURL == "" {
		link = "http://localhost:8090/?workspaceId=" + url.QueryEscape(workspaceID)
	}
	message := mailadapter.Message{To: emailAddress, ToName: displayName, Subject: notificationSubject(args.EventType, locale), Body: notificationBody(args.EventType, link, locale), MessageID: notificationMessageID(args.EventKey)}
	// The workspace SHARE lock is retained through delivery, ordering it against
	// cancellation and decisions. SMTP remains at-least-once across a crash after
	// acceptance but before this transaction commits.
	if err := w.sender.Send(ctx, message); err != nil {
		if _, updateErr := tx.Exec(ctx, `UPDATE notification_outbox SET status='failed',last_error='delivery_failed',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status='sending'`, args.JobID, args.EventKey); updateErr != nil {
			return updateErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return commitErr
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE notification_outbox SET status='sent',last_error='',updated_at=now() WHERE job_id=$1 AND event_key=$2 AND status='sending'`, args.JobID, args.EventKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func agentQuestionID(eventKey string) int64 {
	var id int64
	_, _ = fmt.Sscanf(strings.TrimPrefix(eventKey, agentQuestionEvent+":"), "%d", &id)
	return id
}

func isActionableReminder(event string) bool {
	switch event {
	case "appointment-reminder", "missed-visit-reminder", "price-proposal-reminder", "date-proposal-reminder", "scope-change-reminder", "evidence-overdue", "completion-note-overdue", "completion-review-reminder", "payment-approval-reminder", "checkout-reminder":
		return true
	}
	return false
}

func reminderMatchesState(args NotificationArgs, state workflow.State, now time.Time) bool {
	job := state.Job
	if state.Cancelled || job.Paid || job.Dispute != nil && job.Dispute.Status != "resolved" && job.Dispute.Status != "rework" || state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
		return false
	}
	if args.EventType == "appointment-reminder" {
		if job.Appointment == nil || job.AppointmentConfirmation == nil || job.AppointmentConfirmation.Status != "confirmed" {
			return false
		}
		start, err := time.Parse(time.RFC3339, job.Appointment.StartUTC)
		if err != nil || !start.After(now) || start.After(now.Add(24*time.Hour)) {
			return false
		}
		anchor := fmt.Sprintf("scope:%d:%s:%s", job.ScopeVersion, job.Appointment.StartUTC, job.Appointment.EndUTC)
		return args.EventKey == fmt.Sprintf("%s:%s:%s:%s", job.ID, args.EventType, anchor, args.RecipientRole)
	}
	for _, wait := range workflow.WaitingStates(state) {
		if wait.EventType == args.EventType && wait.ResponsibleRole == args.RecipientRole && wait.Due(now) && wait.EventKey(job) == args.EventKey {
			return true
		}
	}
	return false
}

func notificationSubject(eventType string, locales ...string) string {
	if len(locales) > 0 && locales[0] == "nl" {
		return dutchNotificationSubject(eventType)
	}
	subjects := map[string]string{
		"coordination-question":      "A follow-up about your agreed service",
		"coordination-summary":       "A service explanation is ready for review",
		"cancel-request":             "Service request cancellation update",
		"send-message":               "New message about your service request",
		"propose-price":              "New service price proposal",
		"accept-price":               "Service price proposal accepted",
		"decline-price":              "Service price proposal declined",
		"propose-date":               "New visit time proposal",
		"accept-date":                "Visit time proposal accepted",
		"late-reschedule-requested":  "Late visit change is awaiting review",
		"withdrawal-requested":       "Withdrawal request is awaiting review",
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
		"appointment-reminder":       "Your confirmed service visit is within 24 hours",
		"missed-visit-reminder":      "Please check the status of your service visit",
		"price-proposal-reminder":    "A service price proposal is waiting for your decision",
		"date-proposal-reminder":     "A visit time proposal is waiting for your decision",
		"scope-change-reminder":      "A service scope change is waiting for your decision",
		"checkout-reminder":          "Check the payment status for your completed service",
		"evidence-overdue":           "Completion evidence is due for your service request",
		"completion-note-overdue":    "Add the work note needed for completion review",
		"completion-review-reminder": "Review your service completion",
		"payment-approval-reminder":  "Review the payment for your completed service",
		agentQuestionEvent:           "Your agent needs your decision",
		"submit-review":              "A customer reviewed your completed service",
		"respond-to-review":          "Your professional responded to your review",
	}
	if subject, ok := subjects[eventType]; ok {
		return subject
	}
	return "Service request update"
}

func notificationBody(eventType, link string, locales ...string) string {
	if len(locales) > 0 && locales[0] == "nl" {
		return dutchNotificationBody(eventType, link)
	}
	opening := "There is an update to a service request in your SamenKlaar account."
	switch eventType {
	case agentQuestionEvent:
		opening = "Your agent stopped on a service request because the next step needs your decision or falls outside the limits you set. Open the request to see its question."
	case "missed-visit-reminder":
		opening = "The confirmed visit has passed without a recorded start. Tell us what happened so the visit can be reviewed; no fault has been assigned."
	case "price-proposal-reminder":
		opening = "A price proposal has been waiting for your decision for 24 hours. Review the proposed amount and accept or decline it."
	case "date-proposal-reminder":
		opening = "A proposed visit time has been waiting for your decision for 24 hours. Review the proposal and accept or decline it."
	case "scope-change-reminder":
		opening = "A proposed service change has been waiting for your decision for 24 hours. The existing scope remains in force until you accept a change."
	case "checkout-reminder":
		opening = "You approved the payment amount, but payment has not completed. Check the saved PayPal status before continuing; reconcile any uncertain result before retrying."
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
	case "accept-date":
		if job.DateProposal != nil && job.DateProposal.Status == "review_pending" {
			if input.Actor == "customer" {
				return "late-reschedule-requested", "professional", true
			}
			return "late-reschedule-requested", "customer", true
		}
		if input.Actor == "customer" {
			return input.Action, "professional", true
		}
		return input.Action, "customer", true
	case "request-withdrawal":
		return "withdrawal-requested", "professional", true
	case "cancel-request", "withdraw-price", "withdraw-date", "send-message", "propose-price", "accept-price", "decline-price", "propose-date", "decline-date":
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
