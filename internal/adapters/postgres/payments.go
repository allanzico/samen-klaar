package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Store) GetPaymentIntent(ctx context.Context, jobID string, scopeVersion int) (paypal.IntentRecord, bool, error) {
	var record paypal.IntentRecord
	err := s.pool.QueryRow(ctx, `
		SELECT workspace_id, job_id, scope_version, amount_minor, currency,
		       seller_merchant_id,
		       COALESCE(order_id, ''), COALESCE(authorization_id, ''), COALESCE(capture_id, ''),
		       approval_url, status, order_request_id, authorize_request_id,
		       capture_request_id, void_request_id, refund_request_id, COALESCE(refund_id, '')
		FROM paypal_payment_operations
		WHERE job_id = $1 AND scope_version = $2`, jobID, scopeVersion).Scan(
		&record.WorkspaceID, &record.JobID, &record.ScopeVersion, &record.AmountMinor, &record.Currency,
		&record.SellerMerchantID,
		&record.OrderID, &record.AuthorizationID, &record.CaptureID, &record.ApprovalURL,
		&record.Status, &record.OrderRequestID, &record.AuthRequestID,
		&record.CaptureRequestID, &record.VoidRequestID, &record.RefundRequestID, &record.RefundID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return paypal.IntentRecord{}, false, nil
	}
	if err != nil {
		return paypal.IntentRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) CreatePaymentIntent(ctx context.Context, record paypal.IntentRecord) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO paypal_payment_operations (
			workspace_id, job_id, scope_version, amount_minor, currency, seller_merchant_id, order_id,
			authorization_id, capture_id, approval_url, status, order_request_id,
			authorize_request_id, capture_request_id, void_request_id, refund_request_id, refund_id
		) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10, $11, $12, $13, $14, $15, $16, NULLIF($17, ''))
		ON CONFLICT (job_id, scope_version) DO NOTHING`,
		record.WorkspaceID, record.JobID, record.ScopeVersion, record.AmountMinor, record.Currency,
		record.SellerMerchantID, record.OrderID, record.AuthorizationID, record.CaptureID, record.ApprovalURL, record.Status,
		record.OrderRequestID, record.AuthRequestID, record.CaptureRequestID, record.VoidRequestID, record.RefundRequestID, record.RefundID,
	)
	if err != nil {
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
		    refund_request_id = $16, refund_id = NULLIF($17, ''),
		    updated_at = now()
		WHERE job_id = $1 AND scope_version = $2 AND workspace_id = $14 AND status = ANY($15::text[])`,
		record.JobID, record.ScopeVersion, record.AmountMinor, record.Currency,
		record.OrderID, record.AuthorizationID, record.CaptureID, record.ApprovalURL,
		record.Status, record.OrderRequestID, record.AuthRequestID, record.CaptureRequestID,
		record.VoidRequestID, record.WorkspaceID, expectedStatuses, record.RefundRequestID, record.RefundID, record.SellerMerchantID,
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var amountMinor int64
	var currency string
	err = tx.QueryRow(ctx, `
		UPDATE paypal_payment_operations
		SET status = $5, updated_at = now()
		WHERE workspace_id = $1 AND job_id = $2 AND scope_version = $3 AND status = $4
		  AND ($4 <> 'refunding' OR $5 <> 'refunding' OR updated_at < now()-interval '2 minutes')
		RETURNING amount_minor,currency`, workspaceID, jobID, scopeVersion, from, to).Scan(&amountMinor, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if from == "authorized" && to == "capturing" {
		var stateJSON []byte
		if err := tx.QueryRow(ctx, `SELECT state_json FROM account_workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&stateJSON); err != nil {
			return false, err
		}
		var state workflow.State
		if err := json.Unmarshal(stateJSON, &state); err != nil {
			return false, err
		}
		if state.Cancelled || state.Job.ID != jobID || state.Job.ScopeVersion != scopeVersion ||
			state.Job.Quote.TotalMinor != amountMinor || state.Job.Quote.Currency != currency {
			return false, errors.New("approved payment scope changed before capture could start")
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
	tag, err := s.pool.Exec(ctx, `
		UPDATE paypal_payment_operations
		SET amount_minor = $3, currency = $4, order_id = NULLIF($5, ''),
		    seller_merchant_id = $17, authorization_id = NULLIF($6, ''), capture_id = NULLIF($7, ''),
		    approval_url = $8, status = $9, order_request_id = $10,
		    authorize_request_id = $11, capture_request_id = $12, void_request_id = $13,
		    refund_request_id = $15, refund_id = NULLIF($16, ''),
		    updated_at = now()
		WHERE job_id = $1 AND scope_version = $2 AND workspace_id = $14
		  AND status IN ('voided', 'denied', 'expired')`,
		record.JobID, record.ScopeVersion, record.AmountMinor, record.Currency,
		record.OrderID, record.AuthorizationID, record.CaptureID, record.ApprovalURL,
		record.Status, record.OrderRequestID, record.AuthRequestID, record.CaptureRequestID,
		record.VoidRequestID, record.WorkspaceID, record.RefundRequestID, record.RefundID, record.SellerMerchantID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
