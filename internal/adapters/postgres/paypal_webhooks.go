package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type PayPalWebhookArgs struct {
	EventID string `json:"eventId"`
}

func (PayPalWebhookArgs) Kind() string { return "paypal_webhook" }

type PayPalWebhookWorker struct {
	river.WorkerDefaults[PayPalWebhookArgs]
	store *Store
}

func (w *PayPalWebhookWorker) Work(ctx context.Context, job *river.Job[PayPalWebhookArgs]) error {
	return w.store.processPayPalWebhook(ctx, job.Args.EventID)
}

func (s *Store) EnqueuePayPalWebhook(ctx context.Context, event paypal.WebhookEvent) error {
	if s.river == nil || event.ID == "" || event.Type == "" {
		return errors.New("PayPal webhook queue is unavailable")
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `INSERT INTO paypal_webhook_events (event_id,event_type,event_json,status)
		VALUES ($1,$2,$3,'queued') ON CONFLICT (event_id) DO NOTHING`, event.ID, event.Type, eventJSON)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		if _, err := s.river.InsertTx(ctx, tx, PayPalWebhookArgs{EventID: event.ID}, nil); err != nil {
			return fmt.Errorf("enqueue verified PayPal event: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) processPayPalWebhook(ctx context.Context, eventID string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var eventJSON []byte
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT event_json,status FROM paypal_webhook_events WHERE event_id=$1 FOR UPDATE`, eventID).Scan(&eventJSON, &currentStatus); err != nil {
		return err
	}
	if currentStatus == "processed" || currentStatus == "ignored" {
		return nil
	}
	var event paypal.WebhookEvent
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE paypal_webhook_events SET status='processing', attempts=attempts+1, last_error='' WHERE event_id=$1`, eventID); err != nil {
		return err
	}

	processed := false
	switch event.Type {
	case "PAYMENT.CAPTURE.COMPLETED":
		processed, err = s.applyCaptureCompleted(ctx, tx, event)
	case "PAYMENT.CAPTURE.DENIED":
		processed, err = s.applyCaptureDenied(ctx, tx, event)
	case "PAYMENT.CAPTURE.REFUNDED":
		processed, err = s.applyCaptureRefunded(ctx, tx, event)
	case "PAYMENT.CAPTURE.REVERSED":
		processed, err = s.applyCaptureReversed(ctx, tx, event)
	case "CUSTOMER.DISPUTE.CREATED", "CUSTOMER.DISPUTE.UPDATED", "CUSTOMER.DISPUTE.RESOLVED":
		processed, err = s.applyPayPalDispute(ctx, tx, event)
	case "MERCHANT.ONBOARDING.COMPLETED":
		if event.MerchantID != "" && event.TrackingID != "" {
			_, err = tx.Exec(ctx, `UPDATE paypal_seller_accounts
				SET merchant_id=$2, status='pending', payments_receivable=false,
					primary_email_confirmed=false, permissions_json='[]'::jsonb, updated_at=now()
				WHERE tracking_id=$1 AND status IN ('creating', 'pending')`, event.TrackingID, event.MerchantID)
			processed = err == nil
		}
	case "MERCHANT.PARTNER-CONSENT.REVOKED":
		if event.MerchantID != "" {
			if _, err = tx.Exec(ctx, `UPDATE paypal_seller_accounts
				SET status='revoked', payments_receivable=false, updated_at=now() WHERE merchant_id=$1`, event.MerchantID); err == nil {
				_, err = tx.Exec(ctx, `UPDATE professional_review_checks c
					SET paypal_onboarding_status='pending', updated_at=now()
					FROM professional_profiles p JOIN paypal_seller_accounts a ON a.account_id=p.account_id
					WHERE p.id=c.professional_profile_id AND a.merchant_id=$1`, event.MerchantID)
			}
			processed = err == nil
		}
	}
	if err != nil {
		return err
	}
	status := "ignored"
	if processed {
		status = "processed"
	}
	if _, err := tx.Exec(ctx, `UPDATE paypal_webhook_events SET status=$2, processed_at=now(), last_error='' WHERE event_id=$1`, eventID, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type webhookPayment struct {
	workspaceID, jobID, currency, captureID, status string
	scopeVersion                                    int
	amountMinor                                     int64
	platformFeeMinor                                int64
}

func findWebhookPayment(ctx context.Context, tx pgx.Tx, event paypal.WebhookEvent) (webhookPayment, error) {
	// Resolve the owner without taking the operation lock first. All payment
	// mutations lock workspace then operation, including webhook reconciliation,
	// so a capture notification cannot deadlock a concurrent checkout claim.
	var workspaceID string
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM paypal_payment_operations
		WHERE ($1 <> '' AND order_id=$1) OR ($2 <> '' AND capture_id=$2)
		ORDER BY updated_at DESC LIMIT 1`, event.OrderID, event.CaptureID).Scan(&workspaceID); err != nil {
		return webhookPayment{}, err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&lockedID); err != nil {
		return webhookPayment{}, err
	}
	var payment webhookPayment
	err := tx.QueryRow(ctx, `SELECT workspace_id,job_id,scope_version,amount_minor,currency,COALESCE(capture_id,''),status,platform_fee_minor
		FROM paypal_payment_operations
		WHERE workspace_id=$3 AND (($1 <> '' AND order_id=$1) OR ($2 <> '' AND capture_id=$2))
		ORDER BY updated_at DESC LIMIT 1 FOR UPDATE`, event.OrderID, event.CaptureID, workspaceID).Scan(
		&payment.workspaceID, &payment.jobID, &payment.scopeVersion, &payment.amountMinor, &payment.currency, &payment.captureID, &payment.status, &payment.platformFeeMinor,
	)
	return payment, err
}

func (s *Store) applyCaptureCompleted(ctx context.Context, tx pgx.Tx, event paypal.WebhookEvent) (bool, error) {
	if event.Status != "COMPLETED" || event.AmountMinor <= 0 || event.Currency == "" {
		return false, nil
	}
	payment, err := findWebhookPayment(ctx, tx, event)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if payment.amountMinor != event.AmountMinor || payment.currency != event.Currency || event.CaptureID == "" {
		return false, nil
	}
	if payment.platformFeeMinor > 0 && payment.status != "captured" && (!event.PlatformFeeReported || event.PlatformFeeMinor != payment.platformFeeMinor) {
		return false, nil // Reconcile the original capture; do not invent fee collection.
	}
	if payment.status == "refunded" {
		return true, nil
	}
	if payment.status == "captured" {
		if payment.captureID != event.CaptureID {
			return false, nil
		}
		return s.markWebhookCaptureOnWorkspace(ctx, tx, payment, event.CaptureID, event.AmountMinor, event.Currency)
	}
	if _, err := tx.Exec(ctx, `UPDATE paypal_payment_operations SET capture_id=$2,status='captured',updated_at=now()
		WHERE workspace_id=$1 AND job_id=$3 AND scope_version=$4 AND status <> 'refunded'`, payment.workspaceID, event.CaptureID, payment.jobID, payment.scopeVersion); err != nil {
		return false, err
	}
	return s.markWebhookCaptureOnWorkspace(ctx, tx, payment, event.CaptureID, event.AmountMinor, event.Currency)
}

func (s *Store) markWebhookCaptureOnWorkspace(ctx context.Context, tx pgx.Tx, payment webhookPayment, captureID string, amountMinor int64, currency string) (bool, error) {
	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id,state_json,revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, payment.workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return false, err
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return false, err
	}
	if state.Job.ID != payment.jobID {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE booking_reservations SET status='completed' WHERE job_id=$1 AND status='booked'`, payment.jobID); err != nil {
		return false, err
	}
	if state.Job.Paid && state.Job.PaymentTransactionID == captureID {
		return true, nil
	}
	state.WorkspaceID = payment.workspaceID
	wasPaid := state.Job.Paid
	state.Job.Paid = true
	state.Job.PaymentTransactionID = captureID
	if state.Job.ScopeVersion != payment.scopeVersion || state.Job.Quote.TotalMinor != amountMinor || state.Job.Quote.Currency != currency || state.Job.PaymentApprovedVersion != payment.scopeVersion {
		if state.Job.Dispute == nil {
			state.Job.Dispute = &workflow.DisputeCase{Status: "open", ReportedBy: "paypal", Reason: "PayPal confirmed a capture that does not match the current approved scope. Operator review is required.", ReportedAt: nowRFC3339()}
		}
		state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Text: "PayPal reported a capture outside the current approved scope · operator review required", Kind: "attention", When: nowClock()})
	} else if !wasPaid {
		state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Text: "PayPal confirmed payment for the approved service", Kind: "done", When: nowClock()})
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	newRevision := revision + 1
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,revision=$3,updated_at=now() WHERE id=$1`, payment.workspaceID, stateBytes, newRevision); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id,actor_account_id,workspace_id,job_id,actor,action,scope_version,revision)
		VALUES ($1,NULL,$2,$3,'paypal','capture-completed',$4,$5)`, ownerID, payment.workspaceID, payment.jobID, int32(payment.scopeVersion), newRevision); err != nil {
		return false, err
	}
	if err := enqueueWebhookNotice(ctx, tx, s.river, payment.jobID, newRevision, "confirm-payment"); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) applyCaptureDenied(ctx context.Context, tx pgx.Tx, event paypal.WebhookEvent) (bool, error) {
	payment, err := findWebhookPayment(ctx, tx, event)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if payment.status == "captured" || payment.status == "refunded" {
		return true, nil
	}
	_, err = tx.Exec(ctx, `UPDATE paypal_payment_operations SET status='denied',updated_at=now()
		WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, payment.workspaceID, payment.jobID, payment.scopeVersion)
	return err == nil, err
}

func (s *Store) applyCaptureRefunded(ctx context.Context, tx pgx.Tx, event paypal.WebhookEvent) (bool, error) {
	if event.CaptureID == "" || event.Status != "REFUNDED" && event.Status != "PARTIALLY_REFUNDED" {
		return false, nil
	}
	payment, err := findWebhookPayment(ctx, tx, event)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id,state_json,revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, payment.workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return false, err
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return false, err
	}
	if state.Job.ID != payment.jobID {
		return false, nil
	}
	if event.Status == "PARTIALLY_REFUNDED" {
		// A capture can be refunded in multiple parts. Keep the job in an
		// operator review state until the cumulative refund is reconciled; never
		// present a partial refund as a completed full refund.
		if state.Job.PaymentRefunded || payment.status == "refunded" {
			return true, nil
		}
		if _, err := tx.Exec(ctx, `UPDATE paypal_payment_operations SET status='partially_refunded',updated_at=now()
			WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, payment.workspaceID, payment.jobID, payment.scopeVersion); err != nil {
			return false, err
		}
		if state.Job.Dispute == nil {
			state.Job.Dispute = &workflow.DisputeCase{ReportedBy: "paypal", ReportedAt: nowRFC3339()}
		}
		state.Job.Dispute.Status = "partial_refund_review"
		state.Job.Dispute.Reason = strings.TrimSpace(state.Job.Dispute.Reason + " PayPal reported a partial capture refund outside the full-refund workflow. Verify the cumulative refund in PayPal before resolving this case.")
		state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Text: "PayPal reported a partial refund · operator reconciliation required", Kind: "attention", When: nowClock()})
		stateBytes, err := json.Marshal(state)
		if err != nil {
			return false, err
		}
		newRevision := revision + 1
		if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,revision=$3,updated_at=now() WHERE id=$1`, payment.workspaceID, stateBytes, newRevision); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id,actor_account_id,workspace_id,job_id,actor,action,scope_version,revision)
			VALUES ($1,NULL,$2,$3,'paypal','partial-refund-review',$4,$5)`, ownerID, payment.workspaceID, payment.jobID, int32(payment.scopeVersion), newRevision); err != nil {
			return false, err
		}
		if err := enqueueWebhookNotice(ctx, tx, s.river, payment.jobID, newRevision, "payment-review"); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE paypal_payment_operations SET status='refunded',updated_at=now()
		WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, payment.workspaceID, payment.jobID, payment.scopeVersion); err != nil {
		return false, err
	}
	state.Job.Paid = state.Job.Paid || payment.status == "captured" || payment.status == "refunded"
	if state.Job.PaymentTransactionID == "" {
		state.Job.PaymentTransactionID = payment.captureID
	}
	if state.Job.PaymentRefunded {
		return true, nil
	}
	state.Job.PaymentRefunded = true
	if state.Job.Dispute == nil {
		state.Job.Dispute = &workflow.DisputeCase{Status: "refunded", ReportedBy: "paypal", Reason: "PayPal reported a completed refund.", ReportedAt: nowRFC3339()}
	} else {
		state.Job.Dispute.Status = "refunded"
		state.Job.Dispute.ResolvedAt = nowRFC3339()
	}
	state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Text: "PayPal confirmed the service payment refund", Kind: "done", When: nowClock()})
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	newRevision := revision + 1
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,revision=$3,updated_at=now() WHERE id=$1`, payment.workspaceID, stateBytes, newRevision); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id,actor_account_id,workspace_id,job_id,actor,action,scope_version,revision)
		VALUES ($1,NULL,$2,$3,'paypal','refund-completed',$4,$5)`, ownerID, payment.workspaceID, payment.jobID, int32(payment.scopeVersion), newRevision); err != nil {
		return false, err
	}
	if err := enqueueWebhookNotice(ctx, tx, s.river, payment.jobID, newRevision, "refund-completed"); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) applyCaptureReversed(ctx context.Context, tx pgx.Tx, event paypal.WebhookEvent) (bool, error) {
	if event.CaptureID == "" || event.Status != "REVERSED" {
		return false, nil
	}
	payment, err := findWebhookPayment(ctx, tx, event)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if payment.status == "refunded" {
		return true, nil
	}
	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id,state_json,revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, payment.workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return false, err
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return false, err
	}
	if state.Job.ID != payment.jobID || state.Job.PaymentRefunded {
		return state.Job.ID == payment.jobID, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE paypal_payment_operations SET status='reversed',updated_at=now()
		WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, payment.workspaceID, payment.jobID, payment.scopeVersion); err != nil {
		return false, err
	}
	if state.Job.Dispute == nil {
		state.Job.Dispute = &workflow.DisputeCase{ReportedBy: "paypal", ReportedAt: nowRFC3339()}
	}
	state.Job.Dispute.Status = "payment_reversal_review"
	state.Job.Dispute.Reason = strings.TrimSpace(state.Job.Dispute.Reason + " PayPal reported a payment reversal. Verify the transaction and any related dispute in PayPal before resolving this case.")
	state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Text: "PayPal reported a payment reversal · operator reconciliation required", Kind: "attention", When: nowClock()})
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	newRevision := revision + 1
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,revision=$3,updated_at=now() WHERE id=$1`, payment.workspaceID, stateBytes, newRevision); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id,actor_account_id,workspace_id,job_id,actor,action,scope_version,revision)
		VALUES ($1,NULL,$2,$3,'paypal','payment-reversal-review',$4,$5)`, ownerID, payment.workspaceID, payment.jobID, int32(payment.scopeVersion), newRevision); err != nil {
		return false, err
	}
	if err := enqueueWebhookNotice(ctx, tx, s.river, payment.jobID, newRevision, "payment-review"); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) applyPayPalDispute(ctx context.Context, tx pgx.Tx, event paypal.WebhookEvent) (bool, error) {
	if event.CaptureID == "" {
		return false, nil
	}
	payment, err := findWebhookPayment(ctx, tx, event)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var ownerID string
	var stateJSON []byte
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT account_id,state_json,revision FROM account_workspaces WHERE id=$1 FOR UPDATE`, payment.workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return false, err
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return false, err
	}
	if state.Job.ID != payment.jobID {
		return false, nil
	}
	if event.Type == "CUSTOMER.DISPUTE.CREATED" {
		reason := event.Reason
		if reason == "" {
			reason = "PayPal reported a customer payment dispute."
		}
		state.Job.Dispute = &workflow.DisputeCase{Status: "open", ReportedBy: "paypal", Reason: reason, ReportedAt: nowRFC3339()}
	} else if event.Type == "CUSTOMER.DISPUTE.RESOLVED" && state.Job.Dispute != nil {
		state.Job.Dispute.Status = "resolved"
		state.Job.Dispute.ResolvedAt = nowRFC3339()
	} else if event.Type == "CUSTOMER.DISPUTE.UPDATED" && state.Job.Dispute == nil {
		state.Job.Dispute = &workflow.DisputeCase{Status: "open", ReportedBy: "paypal", Reason: "PayPal reported an updated customer payment dispute.", ReportedAt: nowRFC3339()}
	}
	state.Job.Timeline = append(state.Job.Timeline, workflow.Event{Text: "PayPal reported a payment dispute status update · operator review required", Kind: "attention", When: nowClock()})
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	newRevision := revision + 1
	if _, err := tx.Exec(ctx, `UPDATE account_workspaces SET state_json=$2,revision=$3,updated_at=now() WHERE id=$1`, payment.workspaceID, stateBytes, newRevision); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (account_id,actor_account_id,workspace_id,job_id,actor,action,scope_version,revision)
		VALUES ($1,NULL,$2,$3,'paypal',$4,$5,$6)`, ownerID, payment.workspaceID, payment.jobID, event.Type, int32(payment.scopeVersion), newRevision); err != nil {
		return false, err
	}
	if event.Type == "CUSTOMER.DISPUTE.CREATED" || event.Type == "CUSTOMER.DISPUTE.RESOLVED" {
		if err := enqueueWebhookNotice(ctx, tx, s.river, payment.jobID, newRevision, "issue-reported"); err != nil {
			return false, err
		}
	}
	return true, nil
}

func enqueueWebhookNotice(ctx context.Context, tx pgx.Tx, riverClient *river.Client[pgx.Tx], jobID string, revision int64, eventType string) error {
	for _, recipient := range []string{"customer", "professional"} {
		eventKey := fmt.Sprintf("%s:%d:%s:%s", jobID, revision, eventType, recipient)
		if err := enqueueNotificationTx(ctx, tx, riverClient, NotificationArgs{JobID: jobID, EventKey: eventKey, EventType: eventType, RecipientRole: recipient}); err != nil {
			return fmt.Errorf("enqueue payment notice: %w", err)
		}
	}
	return nil
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
func nowClock() string   { return time.Now().UTC().Format("15:04") }
