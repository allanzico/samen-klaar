package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	mailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
	dbq "github.com/allanzico/samen-klaar/internal/db"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type Store struct {
	pool          *pgxpool.Pool
	river         *river.Client[pgx.Tx]
	riverCancel   context.CancelFunc
	loopCancel    context.CancelFunc
	loopWG        sync.WaitGroup
	sender        mailadapter.Sender
	appBaseURL    string
	releaseMu     sync.RWMutex
	releaser      AuthorizationReleaser
	coordinatorMu sync.RWMutex
	coordinator   ContextCoordinator
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	return OpenWithNotifications(ctx, databaseURL, nil, "")
}

func OpenWithNotifications(ctx context.Context, databaseURL string, sender mailadapter.Sender, appBaseURL string) (*Store, error) {
	store, err := OpenDirectory(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store.sender = sender
	store.appBaseURL = appBaseURL
	if err := startRiver(ctx, store); err != nil {
		store.pool.Close()
		return nil, fmt.Errorf("initialize River queue: %w", err)
	}
	return store, nil
}

// OpenDirectory opens the same repository without starting queue workers. Local
// data tools must not consume application notifications while measuring reads.
func OpenDirectory(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	store := &Store{pool: pool}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return store, nil
}

func startRiver(ctx context.Context, store *Store) error {
	driver := riverpgxv5.New(store.pool)
	workers := river.NewWorkers()
	river.AddWorker(workers, &NotificationWorker{pool: store.pool, sender: store.sender, appBaseURL: store.appBaseURL})
	river.AddWorker(workers, &PayPalWebhookWorker{store: store})
	river.AddWorker(workers, &AuthorizationReleaseWorker{store: store})
	river.AddWorker(workers, &CoordinationWorker{store: store})
	client, err := river.NewClient(driver, &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}},
		Workers: workers,
	})
	if err != nil {
		return fmt.Errorf("create River client: %w", err)
	}
	riverCtx, cancel := context.WithCancel(context.Background())
	if err := client.Start(riverCtx); err != nil {
		cancel()
		return fmt.Errorf("start River workers: %w", err)
	}
	store.river = client
	store.riverCancel = cancel
	if err := requeueUndeliveredNotifications(ctx, store); err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Stop(stopCtx)
		stopCancel()
		cancel()
		return fmt.Errorf("requeue undelivered notifications: %w", err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	store.loopCancel = loopCancel
	store.loopWG.Add(4)
	go func() { defer store.loopWG.Done(); store.runAppointmentConfirmationExpiry(loopCtx) }()
	go func() { defer store.loopWG.Done(); store.runNotificationRecovery(loopCtx) }()
	go func() { defer store.loopWG.Done(); store.runJobLifecycleNotifications(loopCtx) }()
	go func() { defer store.loopWG.Done(); store.runAuthCleanup(loopCtx) }()
	return nil
}

func (s *Store) runAuthCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		for batch := 0; batch < 10; batch++ {
			passCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			count, err := s.CleanupExpiredAuthRecords(passCtx)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("expired auth record cleanup failed", "error", err)
				}
				break
			}
			if count < 1000 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Migrate is a deployment step. It does not start HTTP or queue workers.
func Migrate(ctx context.Context, databaseURL string) error {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	defer pool.Close()
	// Serialize the entire deployment migration, including initial ledger and
	// River tables. Per-file locks alone do not protect their first creation.
	lock, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lock.Release()
	if _, err := lock.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended('relay_deployment_migrations', 0))`); err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = lock.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended('relay_deployment_migrations', 0))`)
	}()
	if err := migrate(ctx, pool); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("create River migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrate River schema: %w", err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Close() {
	s.stopBackgroundLoops()
	if s.river != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.river.Stop(ctx)
		cancel()
		if err != nil {
			if s.riverCancel != nil {
				s.riverCancel()
			}
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.river.StopAndCancel(stopCtx)
			stopCancel()
		}
	}
	if s.riverCancel != nil {
		s.riverCancel()
	}
	s.pool.Close()
}

func (s *Store) stopBackgroundLoops() {
	if s.loopCancel != nil {
		s.loopCancel()
		s.loopCancel = nil
	}
	s.loopWG.Wait()
}

func (s *Store) Load(ctx context.Context, accountID string) (workflow.State, error) {
	return s.LoadFor(ctx, accountID, "customer")
}

func (s *Store) LoadFor(ctx context.Context, accountID, role string) (workflow.State, error) {
	if accountID == "" || role != "customer" && role != "professional" {
		return workflow.State{}, errors.New("workspace role cannot load job state")
	}
	query := `SELECT id, state_json FROM account_workspaces WHERE account_id = $1 ORDER BY updated_at DESC LIMIT 1`
	if role == "professional" {
		query = `SELECT id, state_json FROM account_workspaces WHERE state_json #>> '{job,professionalAccountId}' = $1 AND ` + notRemovedByProfessional + ` ORDER BY updated_at DESC LIMIT 1`
	}
	var workspaceID string
	var stateJSON []byte
	err := s.pool.QueryRow(ctx, query, accountID).Scan(&workspaceID, &stateJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.NewState(), nil
	}
	if err != nil {
		return workflow.State{}, err
	}
	var state workflow.State
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return workflow.State{}, err
	}
	state.WorkspaceID = workspaceID
	if state.Cancelled {
		state.AvailableSlots = []workflow.Appointment{}
	} else {
		state.AvailableSlots, err = s.availableSlots(ctx, state.Job.ProfessionalAccountID, state.Job.ID)
	}
	return workflow.ConversationForRole(state, role), err
}

const workspaceSummaryJSON = `jsonb_build_object(
  'cancelled', state_json->'cancelled',
  'cancellationReview', jsonb_build_object('status', state_json #> '{cancellationReview,status}'),
  'intake', jsonb_build_object('step', state_json #> '{intake,step}', 'description', state_json #> '{intake,description}'),
  'job', jsonb_build_object(
    'id', state_json #> '{job,id}', 'title', state_json #> '{job,title}',
    'description', state_json #> '{job,description}',
    'customer', state_json #> '{job,customer}', 'professional', state_json #> '{job,professional}',
    'paid', state_json #> '{job,paid}', 'quoteApproved', state_json #> '{job,quoteApproved}',
    'appointment', state_json #> '{job,appointment}',
    'appointmentConfirmation', state_json #> '{job,appointmentConfirmation}',
    'dispute', state_json #> '{job,dispute}', 'change', state_json #> '{job,change}',
    'dateProposal', state_json #> '{job,dateProposal}', 'priceProposal', state_json #> '{job,priceProposal}',
    'completionApproved', state_json #> '{job,completionApproved}',
    'evidenceSubmitted', state_json #> '{job,evidenceSubmitted}',
    'workStarted', state_json #> '{job,workStarted}',
    'paymentAuthorizedVersion', state_json #> '{job,paymentAuthorizedVersion}',
    'paymentApprovedVersion', state_json #> '{job,paymentApprovedVersion}',
    'paymentAuthorizationTiming', state_json #> '{job,paymentAuthorizationTiming}',
    'paymentTransactionId', state_json #> '{job,paymentTransactionId}'))`

type workspaceCursor struct {
	UpdatedAt time.Time `json:"updatedAt"`
	ID        string    `json:"id"`
}

func (s *Store) ListWorkspaces(ctx context.Context, accountID, role string) ([]workflow.WorkspaceSummary, error) {
	items, err := s.listWorkspaceSummaries(ctx, accountID, role, nil, 0)
	return items, err
}

func (s *Store) ListWorkspacesPage(ctx context.Context, accountID, role, encodedCursor string) ([]workflow.WorkspaceSummary, string, error) {
	var before *workspaceCursor
	if encodedCursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(encodedCursor)
		if err != nil {
			return nil, "", errors.New("invalid request list cursor")
		}
		var decoded workspaceCursor
		if json.Unmarshal(data, &decoded) != nil || decoded.ID == "" || decoded.UpdatedAt.IsZero() {
			return nil, "", errors.New("invalid request list cursor")
		}
		before = &decoded
	}
	items, err := s.listWorkspaceSummaries(ctx, accountID, role, before, 101)
	if err != nil {
		return nil, "", err
	}
	if len(items) <= 100 {
		return items, "", nil
	}
	items = items[:100]
	last := items[len(items)-1]
	data, err := json.Marshal(workspaceCursor{UpdatedAt: last.UpdatedAt, ID: last.ID})
	if err != nil {
		return nil, "", err
	}
	return items, base64.RawURLEncoding.EncodeToString(data), nil
}

func (s *Store) listWorkspaceSummaries(ctx context.Context, accountID, role string, before *workspaceCursor, limit int) ([]workflow.WorkspaceSummary, error) {
	if accountID == "" || role != "customer" && role != "professional" {
		return nil, errors.New("workspace role cannot list requests")
	}
	query := `SELECT id, ` + workspaceSummaryJSON + `, updated_at FROM account_workspaces WHERE account_id = $1`
	if role == "professional" {
		query = `SELECT id, ` + workspaceSummaryJSON + `, updated_at FROM account_workspaces WHERE state_json #>> '{job,professionalAccountId}' = $1 AND ` + notRemovedByProfessional
	}
	var beforeAt any
	beforeID := ""
	if before != nil {
		beforeAt, beforeID = before.UpdatedAt, before.ID
	}
	query += ` AND ($2::timestamptz IS NULL OR (updated_at,id) < ($2,$3)) ORDER BY updated_at DESC,id DESC`
	if limit > 0 {
		query += ` LIMIT $4`
	}
	args := []any{accountID, beforeAt, beforeID}
	if limit > 0 {
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]workflow.WorkspaceSummary, 0)
	for rows.Next() {
		var id string
		var raw []byte
		var updatedAt time.Time
		if err := rows.Scan(&id, &raw, &updatedAt); err != nil {
			return nil, err
		}
		var state workflow.State
		if err := json.Unmarshal(raw, &state); err != nil {
			return nil, err
		}
		state.WorkspaceID = id
		items = append(items, workflow.SummarizeWorkspace(state, role, updatedAt))
	}
	return items, rows.Err()
}

func (s *Store) CreateWorkspace(ctx context.Context, accountID string) (workflow.State, error) {
	if accountID == "" {
		return workflow.State{}, errors.New("account is required to create a request")
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return workflow.State{}, err
	}
	id := hex.EncodeToString(idBytes[:])
	state := workflow.NewState()
	state.WorkspaceID = id
	state.ParentRequestID = id
	raw, err := json.Marshal(state)
	if err != nil {
		return workflow.State{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return workflow.State{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	open, err := lockOpenRequestCountTx(ctx, tx, accountID, "")
	if err != nil {
		return workflow.State{}, err
	}
	if err := workflow.CheckOpenRequestLimit(open, 1); err != nil {
		return workflow.State{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO account_workspaces (id, account_id, state_json) VALUES ($1, $2, $3)`, id, accountID, raw); err != nil {
		return workflow.State{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workflow.State{}, err
	}
	return state, nil
}

func (s *Store) LoadWorkspace(ctx context.Context, accountID, role, workspaceID string) (workflow.State, error) {
	if accountID == "" || workspaceID == "" || role != "customer" && role != "professional" {
		return workflow.State{}, errors.New("workspace was not found for this account")
	}
	query := `SELECT state_json FROM account_workspaces WHERE id = $1 AND account_id = $2`
	if role == "professional" {
		query = `SELECT state_json FROM account_workspaces WHERE id = $1 AND state_json #>> '{job,professionalAccountId}' = $2`
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, query, workspaceID, accountID).Scan(&raw); err != nil {
		return workflow.State{}, errors.New("workspace was not found for this account")
	}
	var state workflow.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return workflow.State{}, err
	}
	state.WorkspaceID = workspaceID
	var err error
	if state.Cancelled {
		state.AvailableSlots = []workflow.Appointment{}
	} else {
		state.AvailableSlots, err = s.availableSlots(ctx, state.Job.ProfessionalAccountID, state.Job.ID)
	}
	return workflow.ConversationForRole(state, role), err
}

func (s *Store) Apply(ctx context.Context, accountID string, input workflow.Action) (workflow.State, error) {
	return s.ApplyFor(ctx, accountID, "customer", input)
}

func (s *Store) ApplyFor(ctx context.Context, accountID, role string, input workflow.Action) (workflow.State, error) {
	workspaceID := input.WorkspaceID
	if workspaceID == "" {
		state, err := s.LoadFor(ctx, accountID, role)
		if err != nil {
			return workflow.State{}, err
		}
		workspaceID = state.WorkspaceID
		if workspaceID == "" && role == "customer" && input.Action == "start-intake" {
			state, err = s.CreateWorkspace(ctx, accountID)
			if err != nil {
				return workflow.State{}, err
			}
			workspaceID = state.WorkspaceID
		}
	}
	return s.ApplyWorkspace(ctx, accountID, role, workspaceID, input)
}

func (s *Store) ApplyWorkspace(ctx context.Context, actorAccountID, role, workspaceID string, input workflow.Action) (workflow.State, error) {
	if workspaceID == "" || actorAccountID == "" || role != "customer" && role != "professional" {
		return workflow.State{}, errors.New("account and request are required to update workspace state")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return workflow.State{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var stateJSON []byte
	var revision int64
	var ownerID string
	if err = tx.QueryRow(ctx, `SELECT account_id, state_json, revision FROM account_workspaces WHERE id = $1 FOR UPDATE`, workspaceID).Scan(&ownerID, &stateJSON, &revision); err != nil {
		return workflow.State{}, err
	}
	var current workflow.State
	if err = json.Unmarshal(stateJSON, &current); err != nil {
		return workflow.State{}, err
	}
	current.WorkspaceID = workspaceID
	if role == "customer" && ownerID != actorAccountID {
		return workflow.State{}, errors.New("customer does not own this request")
	}
	if role == "professional" && current.Job.ProfessionalAccountID != actorAccountID {
		return workflow.State{}, errors.New("professional is not assigned to this request")
	}
	if input.ByAgent && (input.AutonomyVersion > 0 || input.Action == "approve-payment") || input.Action == "set-booking-autonomy" && strings.HasPrefix(input.Value, "automatic|") || input.Action == "agent-reply" && strings.HasPrefix(input.ChoiceID, "automatic|") {
		if err := customerAgentEnabledTx(ctx, tx, workspaceID); err != nil {
			return workflow.State{}, err
		}
	}
	if (input.Action == "set-booking-autonomy" && input.Value == "manual" || input.Action == "agent-reply" && input.ChoiceID == "manual") && current.Job.BookingAutonomy != nil && current.Job.BookingAutonomy.Mode == "automatic" {
		var releasing bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM paypal_payment_operations WHERE workspace_id=$1 AND job_id=$2 AND status IN ('capturing','capture_uncertain','captured'))`, workspaceID, current.Job.ID).Scan(&releasing); err != nil {
			return workflow.State{}, err
		}
		if releasing {
			return workflow.State{}, errors.New("payment release has already started; check PayPal status before changing autonomy")
		}
	}
	// PayPal can deliver PAYMENT.CAPTURE.COMPLETED before the buyer returns to
	// the app. Treat the browser return for that same capture as an idempotent
	// confirmation instead of rejecting a payment already reconciled by webhook.
	if role == "customer" && input.Action == "confirm-payment" && current.Job.Paid &&
		input.PaymentCaptureID != "" && current.Job.PaymentTransactionID == input.PaymentCaptureID {
		_ = tx.Rollback(ctx)
		current.AvailableSlots, err = s.availableSlots(ctx, current.Job.ProfessionalAccountID, current.Job.ID)
		return workflow.ConversationForRole(current, role), err
	}
	input.WorkspaceID = workspaceID
	input.CustomerAccountID = ownerID
	if input.SelectedOffer != nil && input.SelectedOffer.AccountID == ownerID || current.Job.ProfessionalAccountID != "" && current.Job.ProfessionalAccountID == ownerID {
		return workflow.State{}, errors.New("you cannot book your own services")
	}
	// Job concurrency is limited by the professional's actual appointment slots.
	if input.Action == "select-match" && input.SelectedOffer == nil {
		return workflow.State{}, errors.New("a published professional offer is required")
	}
	if input.Action == "book-appointment" || input.Action == "approve-and-book" {
		current.AvailableSlots, err = s.availableSlots(ctx, current.Job.ProfessionalAccountID, current.Job.ID)
		if err != nil {
			return workflow.State{}, err
		}
		found := false
		for _, slot := range current.AvailableSlots {
			if slot.SlotID == input.Value {
				found = true
				break
			}
		}
		if !found {
			return workflow.State{}, errors.New("that appointment is no longer available; choose another time")
		}
	}
	if input.Action == "decline-appointment" {
		if err = releaseCancelledBooking(ctx, tx, current.Job); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "defer-payment-authorization" {
		if err := deferPaymentAuthorizationTx(ctx, tx, current); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "record-payment-authorization" {
		if err := validatePaymentAuthorizationProjectionTx(ctx, tx, current, input); err != nil {
			return workflow.State{}, err
		}
	}
	if (input.Action == "confirm-appointment" || input.Action == "start-job") && current.Job.PaymentAuthorizationTiming == workflow.PaymentAuthorizationAtBooking {
		if err := validateBookingAuthorizationTx(ctx, tx, current); err != nil {
			return workflow.State{}, err
		}
	}
	if current.Job.ID != "" && (input.Action == "return-to-matches" || input.Action == "revise-intake" || input.Action == "edit-intake" || input.Action == "review-planning") {
		if err := validateBookingPaymentReleaseTx(ctx, tx, current); err != nil {
			return workflow.State{}, err
		}
	}
	if current.Job.Appointment == nil && (input.Action == "book-appointment" || input.Action == "approve-and-book" || input.Action == "accept-date") {
		if err := validateBookingPaymentReleaseTx(ctx, tx, current); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "approve-change" || input.Action == "report-issue" {
		var paymentStatus string
		err = tx.QueryRow(ctx, `SELECT status FROM paypal_payment_operations WHERE job_id=$1 AND scope_version=$2`, current.Job.ID, current.Job.ScopeVersion).Scan(&paymentStatus)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return workflow.State{}, err
		}
		if err == nil {
			if input.Action == "approve-change" && paymentStatus != "voided" && paymentStatus != "denied" && paymentStatus != "expired" && paymentStatus != "deferred" {
				return workflow.State{}, errors.New("resolve the existing PayPal payment before changing scope")
			}
			if input.Action == "report-issue" && (paymentStatus == "capturing" || paymentStatus == "capture_uncertain" || paymentStatus == "captured") {
				return workflow.State{}, errors.New("PayPal payment has already started; reconcile payment before reporting an issue")
			}
		}
	}

	// Finding another professional reopens a request the professional cancelled, so it takes
	// a place under the open-request limit again.
	if role == "customer" && (input.Action == "return-to-matches" && current.Cancelled || (input.Action == "return-to-matches" || input.Action == "revise-intake" || input.Action == "edit-intake" || input.Action == "review-planning") && !workflow.RequestOpen(current)) {
		open, err := lockOpenRequestCountTx(ctx, tx, ownerID, workspaceID)
		if err != nil {
			return workflow.State{}, err
		}
		if err := workflow.CheckOpenRequestLimit(open, 1); err != nil {
			return workflow.State{}, err
		}
	}
	if err := prepareConsumerPolicyActionTx(ctx, tx, &current, &input); err != nil {
		return workflow.State{}, err
	}
	wasCancelled := current.Cancelled
	previousTiming := current.Job.PaymentAuthorizationTiming
	previousAutonomyVersion := 0
	if current.Job.BookingAutonomy != nil {
		previousAutonomyVersion = current.Job.BookingAutonomy.Version
	}
	if err = workflow.Apply(&current, input); err != nil {
		return workflow.State{}, err
	}
	if input.Action == "set-service-address" {
		if err := validateVisitCoverageTx(ctx, tx, current.Job); err != nil {
			return workflow.State{}, err
		}
	}
	state := current
	if consent := state.Job.BookingAutonomy; consent != nil && consent.Version != previousAutonomyVersion {
		if _, err := tx.Exec(ctx, `INSERT INTO booking_autonomy_consents(workspace_id,job_id,version,actor_account_id,mode,scope_version,amount_minor,currency,professional_account_id,professional_profile_id,decided_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::timestamptz)`, workspaceID, state.Job.ID, consent.Version, actorAccountID, consent.Mode, consent.ScopeVersion, consent.AmountMinor, consent.Currency, consent.ProfessionalAccountID, consent.ProfessionalProfileID, consent.DecidedAt); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action != "defer-payment-authorization" && previousTiming != state.Job.PaymentAuthorizationTiming {
		if err := deferPaymentAuthorizationTx(ctx, tx, state); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "cancel-request" && state.Cancelled && state.Job.ProfessionalAccountID != "" {
		if err = releaseCancelledBooking(ctx, tx, state.Job); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "decline-appointment" || input.Action == "cancel-request" && state.Cancelled {
		if err := s.enqueueAuthorizationReleaseTx(ctx, tx, workspaceID, state.Job); err != nil {
			return workflow.State{}, err
		}
		state.Job.PaymentAuthorizedVersion = 0
	}
	if input.Action == "accept-date" && state.Job.DateProposal != nil && state.Job.DateProposal.Status == "accepted" {
		if err = reserveAgreedDate(ctx, tx, &state.Job); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "book-appointment" || input.Action == "approve-and-book" {
		if err = reserveBooking(ctx, tx, state.Job); err != nil {
			return workflow.State{}, err
		}
	}
	if input.Action == "confirm-payment" {
		if _, err = tx.Exec(ctx, `UPDATE booking_reservations SET status = 'completed' WHERE job_id = $1 AND status = 'booked'`, state.Job.ID); err != nil {
			return workflow.State{}, err
		}
	}
	if err = persistScopeHistory(ctx, tx, state.Job); err != nil {
		return workflow.State{}, err
	}
	state.WorkspaceID = workspaceID
	if err := persistConsumerPolicyAcknowledgmentTx(ctx, tx, actorAccountID, state, input); err != nil {
		return workflow.State{}, err
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return workflow.State{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_workspaces SET state_json = $2, revision = revision + 1, updated_at = now() WHERE id = $1`, workspaceID, stateBytes); err != nil {
		return workflow.State{}, err
	}
	if err = s.enqueueCoordinationTx(ctx, tx, workspaceID, state, input, revision+1); err != nil {
		return workflow.State{}, err
	}
	if input.Action == "remove-request" {
		if _, err = tx.Exec(ctx, `INSERT INTO professional_removed_requests (workspace_id, account_id, job_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, workspaceID, actorAccountID, state.Job.ID); err != nil {
			return workflow.State{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events (account_id, actor_account_id, workspace_id, job_id, actor, action, scope_version, revision) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, ownerID, actorAccountID, workspaceID, state.Job.ID, input.Actor, input.Action, int32(state.Job.ScopeVersion), revision+1); err != nil {
		return workflow.State{}, err
	}
	notifyInput := input
	if input.Action == "remove-request" && !wasCancelled {
		// Removing an active request declines it; the customer hears about it as a cancellation.
		notifyInput.Action = "cancel-request"
	}
	if eventType, recipient, ok := notificationForAction(notifyInput, state.Job); ok {
		eventKey := fmt.Sprintf("%s:%d:%s", state.Job.ID, revision+1, input.Action)
		if err = enqueueNotificationTx(ctx, tx, s.river, NotificationArgs{JobID: state.Job.ID, EventKey: eventKey, EventType: eventType, RecipientRole: recipient}); err != nil {
			return workflow.State{}, fmt.Errorf("enqueue notification transactionally: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.State{}, err
	}

	state.WorkspaceID = workspaceID
	if state.Cancelled {
		state.AvailableSlots = []workflow.Appointment{}
	} else {
		state.AvailableSlots, err = s.availableSlots(ctx, state.Job.ProfessionalAccountID, state.Job.ID)
	}
	return workflow.ConversationForRole(state, role), err
}

func persistScopeHistory(ctx context.Context, tx pgx.Tx, job workflow.Job) error {
	queries := dbq.New(tx)
	lastRecorded := 0
	if len(job.ScopeHistory) > 3 {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0) FROM scope_versions WHERE job_id=$1`, job.ID).Scan(&lastRecorded); err != nil {
			return err
		}
	}
	for _, snapshot := range job.ScopeHistory {
		if snapshot.Version <= lastRecorded {
			continue
		}
		lines, err := json.Marshal(snapshot.Lines)
		if err != nil {
			return err
		}
		if err = queries.RecordScopeVersion(ctx, dbq.RecordScopeVersionParams{JobID: job.ID, Version: int32(snapshot.Version), Currency: job.Quote.Currency, TotalMinor: snapshot.TotalMinor, LinesJson: lines}); err != nil {
			return err
		}
	}
	return nil
}

func reserveBooking(ctx context.Context, tx pgx.Tx, job workflow.Job, agreed ...bool) error {
	if job.Appointment == nil {
		return errors.New("appointment is required")
	}
	slot := *job.Appointment
	professionalID := job.ProfessionalAccountID
	if professionalID == "" {
		return errors.New("a real professional account is required to reserve an appointment")
	}
	requestedStart, err := time.Parse(time.RFC3339, slot.StartUTC)
	if err != nil {
		return errors.New("appointment start time is invalid")
	}
	requestedEnd, err := time.Parse(time.RFC3339, slot.EndUTC)
	if err != nil {
		return errors.New("appointment end time is invalid")
	}
	if requestedStart.Before(time.Now()) || requestedEnd.Sub(requestedStart) < time.Duration(job.Quote.DurationMinutes)*time.Minute {
		return errors.New("choose a future appointment long enough for the agreed service")
	}
	// Serialize overlap checks by professional. The lock and insert share the same transaction.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, professionalID); err != nil {
		return err
	}
	if err := validateVisitCoverageTx(ctx, tx, job); err != nil {
		return err
	}
	if err := validateProfessionalSuspensionTx(ctx, tx, professionalID); err != nil {
		return err
	}
	if len(agreed) == 0 || !agreed[0] {
		if err := validateScheduledBookingTx(ctx, tx, professionalID, requestedStart, requestedEnd, job.ID); err != nil {
			return err
		}
	}
	var startsAt, endsAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT starts_at, ends_at, timezone FROM professional_availability_slots
		WHERE id = $1 AND professional_account_id = $2 AND status = 'available' FOR UPDATE`, slot.SlotID, professionalID).Scan(&startsAt, &endsAt, &slot.Timezone)
	if err != nil {
		return errors.New("appointment is no longer in the professional's availability")
	}
	if !startsAt.Equal(requestedStart) || !endsAt.Equal(requestedEnd) {
		return errors.New("appointment time changed; choose an available time again")
	}

	var overlaps bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM booking_reservations
			WHERE professional_id = $1 AND status = 'booked' AND job_id <> $2
			  AND starts_at < $3 AND ends_at > $4
		)`, professionalID, job.ID, endsAt, startsAt).Scan(&overlaps); err != nil {
		return err
	}
	if overlaps {
		return errors.New("that professional time was just booked; choose another available appointment")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO booking_reservations (job_id, professional_id, slot_id, starts_at, ends_at, timezone, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'booked')
		ON CONFLICT (job_id) DO UPDATE SET professional_id = EXCLUDED.professional_id, slot_id = EXCLUDED.slot_id,
		  starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at, timezone = EXCLUDED.timezone, status = 'booked'`,
		job.ID, professionalID, slot.SlotID, startsAt, endsAt, slot.Timezone)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE professional_availability_slots SET status = 'booked' WHERE id = $1 AND professional_account_id = $2 AND status = 'available'`, slot.SlotID, job.ProfessionalAccountID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("appointment is no longer available")
	}
	return err
}

// Date acceptance and reservation are atomic. A conflicting replacement leaves
// the previous appointment and pending proposal intact when the transaction rolls back.
func reserveAgreedDate(ctx context.Context, tx pgx.Tx, job *workflow.Job) error {
	if job.Appointment == nil {
		return errors.New("an agreed visit is required")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, job.ProfessionalAccountID); err != nil {
		return err
	}
	if err := validateProfessionalSuspensionTx(ctx, tx, job.ProfessionalAccountID); err != nil {
		return err
	}
	var conflict bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM booking_reservations WHERE professional_id=$1 AND job_id<>$2 AND status='booked' AND starts_at<$3 AND ends_at>$4)`, job.ProfessionalAccountID, job.ID, mustParseUTC(job.Appointment.EndUTC), mustParseUTC(job.Appointment.StartUTC)).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return errors.New("that professional time was just booked; agree another visit time")
	}
	if _, err := tx.Exec(ctx, `UPDATE professional_availability_slots SET status = 'available' WHERE id IN
		(SELECT slot_id FROM booking_reservations WHERE job_id = $1 AND status = 'booked')`, job.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE booking_reservations SET status = 'released' WHERE job_id = $1 AND status = 'booked'`, job.ID); err != nil {
		return err
	}
	slot := job.Appointment
	err := tx.QueryRow(ctx, `SELECT id FROM professional_availability_slots WHERE professional_account_id = $1
		AND starts_at = $2 AND ends_at = $3 AND status = 'available' ORDER BY id LIMIT 1`,
		job.ProfessionalAccountID, mustParseUTC(slot.StartUTC), mustParseUTC(slot.EndUTC)).Scan(&slot.SlotID)
	if errors.Is(err, pgx.ErrNoRows) {
		slot.SlotID, err = randomID("slot_")
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO professional_availability_slots (id, professional_account_id, starts_at, ends_at, timezone)
			VALUES ($1, $2, $3, $4, $5)`, slot.SlotID, job.ProfessionalAccountID, mustParseUTC(slot.StartUTC), mustParseUTC(slot.EndUTC), slot.Timezone)
	}
	if err != nil {
		return err
	}
	return reserveBooking(ctx, tx, *job, true)
}

func mustParseUTC(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}

func (s *Store) availableSlots(ctx context.Context, professionalID, excludeJobID string) ([]workflow.Appointment, error) {
	if professionalID != "" {
		return s.AvailableProfessionalSlots(ctx, professionalID, excludeJobID)
	}
	return []workflow.Appointment{}, nil
}

// Cancellation uses the same professional lock as booking and rescheduling.
// Releasing the reservation, slot and workspace happens in one transaction.
func releaseCancelledBooking(ctx context.Context, tx pgx.Tx, job workflow.Job) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, job.ProfessionalAccountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE professional_availability_slots a SET status='available'
 WHERE a.professional_account_id=$2 AND a.status='booked' AND a.id IN
 (SELECT slot_id FROM booking_reservations WHERE job_id=$1 AND status='booked')
 AND NOT EXISTS(SELECT 1 FROM booking_reservations b WHERE b.job_id<>$1 AND b.professional_id=$2 AND b.status='booked' AND b.starts_at<a.ends_at AND b.ends_at>a.starts_at)`, job.ID, job.ProfessionalAccountID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE booking_reservations SET status='released' WHERE job_id=$1 AND professional_id=$2 AND status='booked'`, job.ID, job.ProfessionalAccountID)
	return err
}
