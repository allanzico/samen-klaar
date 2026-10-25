package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Store) GetPaymentIntent(ctx context.Context, jobID string, scopeVersion int) (paypal.IntentRecord, bool, error) {
	record, err := scanPaymentIntent(s.pool.QueryRow(ctx, `SELECT `+paymentIntentColumns+` FROM paypal_payment_operations WHERE job_id=$1 AND scope_version=$2`, jobID, scopeVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return paypal.IntentRecord{}, false, nil
	}
	if err != nil {
		return paypal.IntentRecord{}, false, err
	}
	return record, true, nil
}

const paymentIntentColumns = `workspace_id, job_id, scope_version, amount_minor, currency,
 seller_merchant_id, COALESCE(order_id, ''), COALESCE(authorization_id, ''), COALESCE(capture_id, ''),
 approval_url, status, order_request_id, authorize_request_id,
 capture_request_id, void_request_id, refund_request_id, COALESCE(refund_id, ''), authorization_expires_at`

func scanPaymentIntent(row pgx.Row) (paypal.IntentRecord, error) {
	var record paypal.IntentRecord
	var expiresAt *time.Time
	err := row.Scan(
		&record.WorkspaceID, &record.JobID, &record.ScopeVersion, &record.AmountMinor, &record.Currency,
		&record.SellerMerchantID,
		&record.OrderID, &record.AuthorizationID, &record.CaptureID, &record.ApprovalURL,
		&record.Status, &record.OrderRequestID, &record.AuthRequestID,
		&record.CaptureRequestID, &record.VoidRequestID, &record.RefundRequestID, &record.RefundID, &expiresAt,
	)
	if expiresAt != nil {
		record.AuthorizationExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	return record, err
}

func (s *Store) CreatePaymentIntent(ctx context.Context, record paypal.IntentRecord) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := lockPaymentWorkspace(ctx, tx, record.WorkspaceID)
	if err != nil {
		return false, err
	}
	if err := validatePaymentBinding(ctx, tx, state, record, record.Status == "creating"); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO paypal_payment_operations (
			workspace_id, job_id, scope_version, amount_minor, currency, seller_merchant_id, order_id,
			authorization_id, capture_id, approval_url, status, order_request_id,
			authorize_request_id, capture_request_id, void_request_id, refund_request_id, refund_id, authorization_expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10, $11, $12, $13, $14, $15, $16, NULLIF($17, ''), NULLIF($18, '')::timestamptz)
		ON CONFLICT (job_id, scope_version) DO NOTHING`,
		record.WorkspaceID, record.JobID, record.ScopeVersion, record.AmountMinor, record.Currency,
		record.SellerMerchantID, record.OrderID, record.AuthorizationID, record.CaptureID, record.ApprovalURL, record.Status,
		record.OrderRequestID, record.AuthRequestID, record.CaptureRequestID, record.VoidRequestID, record.RefundRequestID, record.RefundID, record.AuthorizationExpiresAt,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) UpdatePaymentIntent(ctx context.Context, record paypal.IntentRecord, expectedStatuses ...string) error {
	if len(expectedStatuses) == 0 {
		return errors.New("PayPal operation update requires an expected state")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE paypal_payment_operations
		SET amount_minor = $3, currency = $4, order_id = NULLIF($5, ''),
		    seller_merchant_id = $18, authorization_id = NULLIF($6, ''), capture_id = NULLIF($7, ''),
		    approval_url = $8, status = $9, order_request_id = $10,
		    authorize_request_id = $11, capture_request_id = $12, void_request_id = $13,
		    refund_request_id = $16, refund_id = NULLIF($17, ''), authorization_expires_at = NULLIF($19, '')::timestamptz,
		    updated_at = now()
		WHERE job_id = $1 AND scope_version = $2 AND workspace_id = $14 AND status = ANY($15::text[])
		  AND order_request_id=$10 AND authorize_request_id=$11 AND void_request_id=$13`,
		record.JobID, record.ScopeVersion, record.AmountMinor, record.Currency,
		record.OrderID, record.AuthorizationID, record.CaptureID, record.ApprovalURL,
		record.Status, record.OrderRequestID, record.AuthRequestID, record.CaptureRequestID,
		record.VoidRequestID, record.WorkspaceID, expectedStatuses, record.RefundRequestID, record.RefundID, record.SellerMerchantID, record.AuthorizationExpiresAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("PayPal operation record was not found for this workspace")
	}
	return nil
}

func (s *Store) ClaimPaymentIntent(ctx context.Context, workspaceID, jobID string, scopeVersion int, from, to string) (bool, error) {
	return s.claimPaymentIntent(ctx, workspaceID, jobID, scopeVersion, from, to, "", "", "")
}

// ClaimPaymentIntentAttempt protects provider work queued for a particular
// attempt from claiming a replacement that happens to have the same status.
func (s *Store) ClaimPaymentIntentAttempt(ctx context.Context, attempt paypal.IntentRecord, from, to string) (bool, error) {
	if attempt.OrderRequestID == "" || attempt.AuthRequestID == "" || attempt.VoidRequestID == "" {
		return false, errors.New("PayPal attempt request identifiers are required")
	}
	return s.claimPaymentIntent(ctx, attempt.WorkspaceID, attempt.JobID, attempt.ScopeVersion, from, to, attempt.OrderRequestID, attempt.AuthRequestID, attempt.VoidRequestID)
}

func (s *Store) claimPaymentIntent(ctx context.Context, workspaceID, jobID string, scopeVersion int, from, to, orderRequestID, authRequestID, voidRequestID string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Workspace mutations and payment claims always lock in this order. In
	// particular, switching authorization timing must exclude a stale checkout
	// request before that request can make a provider call.
	state, err := lockPaymentWorkspace(ctx, tx, workspaceID)
	if err != nil {
		return false, err
	}
	var amountMinor int64
	var currency string
	var sellerMerchantID string
	err = tx.QueryRow(ctx, `
		UPDATE paypal_payment_operations
		SET status = $5, updated_at = now()
		WHERE workspace_id = $1 AND job_id = $2 AND scope_version = $3 AND status = $4
		  AND ($4 <> 'refunding' OR $5 <> 'refunding' OR updated_at < now()-interval '2 minutes')
		  AND ($6='' OR (order_request_id=$6 AND authorize_request_id=$7 AND void_request_id=$8))
		  AND ($5 <> 'capturing' OR authorization_expires_at IS NULL OR authorization_expires_at > now())
		RETURNING amount_minor,currency,seller_merchant_id`, workspaceID, jobID, scopeVersion, from, to, orderRequestID, authRequestID, voidRequestID).Scan(&amountMinor, &currency, &sellerMerchantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if to == "authorizing" || to == "capturing" {
		record := paypal.IntentRecord{WorkspaceID: workspaceID, JobID: jobID, ScopeVersion: scopeVersion, AmountMinor: amountMinor, Currency: currency, SellerMerchantID: sellerMerchantID}
		if err := validatePaymentBinding(ctx, tx, state, record, to == "authorizing"); err != nil {
			return false, err
		}
	}
	if to == "capturing" {
		if state.Job.PaymentApprovedByAgent {
			if err := customerAgentEnabledTx(ctx, tx, workspaceID); err != nil {
				return false, err
			}
		}
		if err := workflow.ValidatePaymentApproval(state.Job); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) RetryPaymentIntent(ctx context.Context, record paypal.IntentRecord) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := lockPaymentWorkspace(ctx, tx, record.WorkspaceID)
	if err != nil {
		return false, err
	}
	if err := validatePaymentBinding(ctx, tx, state, record, true); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE paypal_payment_operations
		SET amount_minor = $3, currency = $4, order_id = NULLIF($5, ''),
		    seller_merchant_id = $17, authorization_id = NULLIF($6, ''), capture_id = NULLIF($7, ''),
		    approval_url = $8, status = $9, order_request_id = $10,
		    authorize_request_id = $11, capture_request_id = $12, void_request_id = $13,
		    refund_request_id = $15, refund_id = NULLIF($16, ''), authorization_expires_at = NULLIF($18, '')::timestamptz,
		    updated_at = now()
		WHERE job_id = $1 AND scope_version = $2 AND workspace_id = $14
		  AND status IN ('voided', 'denied', 'expired', 'deferred')`,
		record.JobID, record.ScopeVersion, record.AmountMinor, record.Currency,
		record.OrderID, record.AuthorizationID, record.CaptureID, record.ApprovalURL,
		record.Status, record.OrderRequestID, record.AuthRequestID, record.CaptureRequestID,
		record.VoidRequestID, record.WorkspaceID, record.RefundRequestID, record.RefundID, record.SellerMerchantID, record.AuthorizationExpiresAt,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func lockPaymentWorkspace(ctx context.Context, tx pgx.Tx, workspaceID string) (workflow.State, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT state_json FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&raw); err != nil {
		return workflow.State{}, err
	}
	var state workflow.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return workflow.State{}, err
	}
	state.WorkspaceID = workspaceID
	return state, nil
}

func validatePaymentBinding(ctx context.Context, tx pgx.Tx, state workflow.State, record paypal.IntentRecord, authorize bool) error {
	if state.Cancelled || state.CancellationReview != nil && state.CancellationReview.Status == "pending" ||
		state.Job.ID != record.JobID || state.Job.ScopeVersion != record.ScopeVersion ||
		state.Job.Quote.TotalMinor != record.AmountMinor || state.Job.Quote.Currency != record.Currency {
		return errors.New("approved payment scope changed before the payment operation could start")
	}
	if record.SellerMerchantID != "" {
		var merchantID, status string
		var receivable, confirmed bool
		err := tx.QueryRow(ctx, `SELECT COALESCE(merchant_id,''),status,payments_receivable,primary_email_confirmed FROM paypal_seller_accounts WHERE account_id=$1 FOR SHARE`, state.Job.ProfessionalAccountID).Scan(&merchantID, &status, &receivable, &confirmed)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err != nil || merchantID != record.SellerMerchantID || status != "verified" || !receivable || !confirmed {
			return errors.New("the professional's PayPal payout account changed before the payment operation could start")
		}
	}
	if authorize {
		return workflow.ValidatePayPalAuthorization(state)
	}
	return nil
}
