package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// AuthorizationReleaser settles one stored payment attempt. The request IDs in
// the snapshot distinguish it from a later retry for the same job and scope.
type AuthorizationReleaser interface {
	ReleaseAuthorization(context.Context, paypal.IntentRecord) error
}

type AuthorizationReleaseArgs struct {
	Attempt paypal.IntentRecord `json:"attempt"`
}

func (AuthorizationReleaseArgs) Kind() string { return "paypal_authorization_release" }

type AuthorizationReleaseWorker struct {
	river.WorkerDefaults[AuthorizationReleaseArgs]
	store *Store
}

func (s *Store) SetAuthorizationReleaser(releaser AuthorizationReleaser) {
	s.releaseMu.Lock()
	defer s.releaseMu.Unlock()
	s.releaser = releaser
}

func (w *AuthorizationReleaseWorker) Work(ctx context.Context, job *river.Job[AuthorizationReleaseArgs]) error {
	attempt := job.Args.Attempt
	current, found, err := w.store.GetPaymentIntent(ctx, attempt.JobID, attempt.ScopeVersion)
	if err != nil {
		return err
	}
	// A workspace may have been removed, or this job may have completed before
	// River persisted its acknowledgement. Never send an obsolete attempt back
	// to the provider once a replacement has acquired new request IDs.
	if !found || current.WorkspaceID != attempt.WorkspaceID || current.OrderRequestID != attempt.OrderRequestID || current.AuthRequestID != attempt.AuthRequestID {
		return nil
	}
	switch current.Status {
	case "voided", "denied", "expired", "deferred", "captured", "refunding", "refunded", "reversed":
		return nil
	}
	w.store.releaseMu.RLock()
	releaser := w.store.releaser
	w.store.releaseMu.RUnlock()
	if releaser == nil {
		return river.JobSnooze(time.Minute)
	}
	return releaser.ReleaseAuthorization(ctx, attempt)
}

// Called only while the workspace is locked. Queue insertion and the pending
// release marker commit together with cancellation/expiry, so another checkout
// cannot reuse the abandoned attempt even before the worker runs.
func (s *Store) enqueueAuthorizationReleaseTx(ctx context.Context, tx pgx.Tx, workspaceID string, job workflow.Job) error {
	attempt, err := scanPaymentIntent(tx.QueryRow(ctx, `SELECT `+paymentIntentColumns+` FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3 FOR UPDATE`, workspaceID, job.ID, job.ScopeVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch attempt.Status {
	case "voided", "denied", "expired", "deferred", "release_pending":
		return nil
	case "capturing", "capture_uncertain", "captured", "refunding", "refunded", "reversed":
		return errors.New("PayPal capture has already started; reconcile payment before releasing this visit")
	}
	if s.river == nil {
		return errors.New("PayPal authorization release queue is unavailable")
	}
	if _, err := s.river.InsertTx(ctx, tx, AuthorizationReleaseArgs{Attempt: attempt}, &river.InsertOpts{MaxAttempts: 100}); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE paypal_payment_operations SET status='release_pending',updated_at=now() WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, workspaceID, job.ID, job.ScopeVersion)
	return err
}

func deferPaymentAuthorizationTx(ctx context.Context, tx pgx.Tx, state workflow.State) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3 FOR UPDATE`, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch status {
	case "created":
		_, err = tx.Exec(ctx, `UPDATE paypal_payment_operations SET status='deferred',updated_at=now() WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3`, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion)
		return err
	case "voided", "denied", "expired", "deferred":
		return nil
	default:
		return errors.New("reconcile the existing PayPal authorization before changing payment timing")
	}
}

func validatePaymentAuthorizationProjectionTx(ctx context.Context, tx pgx.Tx, state workflow.State, input workflow.Action) error {
	record, err := scanPaymentIntent(tx.QueryRow(ctx, `SELECT `+paymentIntentColumns+` FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3 FOR UPDATE`, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion))
	if err != nil {
		return errors.New("a durable verified PayPal authorization is required")
	}
	if record.Status != "authorized" || record.AuthorizationID == "" || record.SellerMerchantID == "" ||
		record.AuthorizationID != input.PaymentAuthorizationID || record.AuthRequestID != input.PaymentAuthorizationRequestID ||
		record.ScopeVersion != input.PaymentAuthorizationScopeVersion {
		return errors.New("the PayPal authorization changed before it could be recorded")
	}
	return validatePaymentBinding(ctx, tx, state, record, true)
}

func validateBookingAuthorizationTx(ctx context.Context, tx pgx.Tx, state workflow.State) error {
	record, err := scanPaymentIntent(tx.QueryRow(ctx, `SELECT `+paymentIntentColumns+` FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3 FOR UPDATE`, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion))
	if err != nil || record.Status != "authorized" || record.AuthorizationID == "" || record.SellerMerchantID == "" {
		return errors.New("the current PayPal authorization must be verified before the visit can proceed")
	}
	if record.AuthorizationExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, record.AuthorizationExpiresAt)
		if err != nil || !expiresAt.After(time.Now()) {
			return errors.New("the PayPal authorization expired; the customer must approve a fresh authorization before the visit can proceed")
		}
	}
	return validatePaymentBinding(ctx, tx, state, record, true)
}

func validateBookingPaymentReleaseTx(ctx context.Context, tx pgx.Tx, state workflow.State) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND scope_version=$3 FOR UPDATE`, state.WorkspaceID, state.Job.ID, state.Job.ScopeVersion).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch status {
	case "voided", "denied", "expired", "deferred":
		return nil
	default:
		return errors.New("release the previous PayPal authorization before choosing another visit")
	}
}
