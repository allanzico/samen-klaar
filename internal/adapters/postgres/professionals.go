package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

func randomID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}

func scanProfile(row pgx.Row) (workflow.ProfessionalProfile, error) {
	var profile workflow.ProfessionalProfile
	var exclusions []byte
	err := row.Scan(&profile.ID, &profile.AccountID, &profile.DisplayName, &profile.Category, &profile.ServiceName, &profile.ServiceDescription,
		&profile.ServiceArea, &profile.Latitude, &profile.Longitude, &profile.Currency, &profile.PriceMinor, &profile.CalloutMinor,
		&profile.DurationMinutes, &profile.MaterialsPolicy, &exclusions, &profile.Status)
	if err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	if err := json.Unmarshal(exclusions, &profile.Exclusions); err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	return profile, nil
}

const profileColumns = `id, account_id, display_name, category, service_name, service_description,
 service_area, latitude, longitude, currency, price_minor, callout_minor, duration_minutes, materials_policy, exclusions_json, status`

func qualifiedProfileColumns(alias string) string {
	columns := strings.Split(profileColumns, ",")
	for i, column := range columns {
		columns[i] = alias + "." + strings.TrimSpace(column)
	}
	return strings.Join(columns, ", ")
}

func (s *Store) ProfessionalProfile(ctx context.Context, accountID string) (workflow.ProfessionalProfile, bool, error) {
	profile, err := scanProfile(s.pool.QueryRow(ctx, `SELECT `+profileColumns+` FROM professional_profiles WHERE account_id = $1`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.ProfessionalProfile{}, false, nil
	}
	return profile, err == nil, err
}

func (s *Store) SaveProfessionalProfile(ctx context.Context, accountID string, profile workflow.ProfessionalProfile) (workflow.ProfessionalProfile, error) {
	profileID, err := randomID("prof_")
	if err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	exclusions, err := json.Marshal(profile.Exclusions)
	if err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockProfessionalTimes(ctx, tx, accountID); err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	previous, previousErr := scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM professional_profiles WHERE account_id=$1 FOR UPDATE`, accountID))
	if previousErr != nil && !errors.Is(previousErr, pgx.ErrNoRows) {
		return workflow.ProfessionalProfile{}, previousErr
	}
	reviewChanged := previousErr == nil && previous.Status == "active" && (previous.DisplayName != profile.DisplayName || previous.Category != profile.Category ||
		previous.ServiceName != profile.ServiceName || previous.ServiceDescription != profile.ServiceDescription ||
		previous.ServiceArea != profile.ServiceArea || previous.Latitude != profile.Latitude || previous.Longitude != profile.Longitude ||
		previous.DurationMinutes != profile.DurationMinutes || previous.MaterialsPolicy != profile.MaterialsPolicy ||
		!slices.Equal(previous.Exclusions, profile.Exclusions))
	saved, err := scanProfile(tx.QueryRow(ctx, `
		INSERT INTO professional_profiles (
			id, account_id, display_name, category, service_name, service_description, service_area,
			latitude, longitude, currency, price_minor, callout_minor, duration_minutes, materials_policy, exclusions_json, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, 'pending')
		ON CONFLICT (account_id) DO UPDATE SET
			display_name = EXCLUDED.display_name, category = EXCLUDED.category, service_name = EXCLUDED.service_name,
			service_description = EXCLUDED.service_description, service_area = EXCLUDED.service_area,
			latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude, currency = EXCLUDED.currency,
			price_minor = EXCLUDED.price_minor, callout_minor = EXCLUDED.callout_minor,
			duration_minutes = EXCLUDED.duration_minutes, materials_policy = EXCLUDED.materials_policy,
			exclusions_json = EXCLUDED.exclusions_json,
			status = CASE WHEN professional_profiles.status = 'active' AND NOT $16 THEN 'active' ELSE 'pending' END,
			updated_at = now()
		RETURNING `+profileColumns,
		profileID, accountID, profile.DisplayName, profile.Category, profile.ServiceName, profile.ServiceDescription,
		profile.ServiceArea, profile.Latitude, profile.Longitude, profile.Currency, profile.PriceMinor, profile.CalloutMinor,
		profile.DurationMinutes, profile.MaterialsPolicy, exclusions, reviewChanged))
	if err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	if saved.Status == "pending" && (previousErr != nil || previous.Status != "active" || reviewChanged) {
		_, err = tx.Exec(ctx, `
			INSERT INTO professional_review_checks (professional_profile_id) VALUES ($1)
			ON CONFLICT (professional_profile_id) DO UPDATE SET
				identity_business_status = 'pending', paypal_onboarding_status = 'pending',
				qualification_status = 'pending', insurance_status = 'pending',
				qualification_rationale = '', insurance_rationale = '', updated_by = NULL, updated_at = now()`, saved.ID)
		if err != nil {
			return workflow.ProfessionalProfile{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE account_memberships SET status='pending',updated_at=now()
			WHERE account_id=$1 AND role='professional' AND status='disabled'`, accountID); err != nil {
			return workflow.ProfessionalProfile{}, err
		}
	}
	if previousErr != nil || previous.DurationMinutes != saved.DurationMinutes {
		if _, _, err := s.materializeSchedule(ctx, tx, accountID, time.Now()); err != nil {
			return workflow.ProfessionalProfile{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return workflow.ProfessionalProfile{}, err
	}
	return saved, nil
}

func (s *Store) ListProfessionalMatches(ctx context.Context, category string, location workflow.Location, excludeAccountID string) ([]workflow.Match, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT p.id, p.account_id, p.display_name, p.category, p.service_area,
               earliest.id, earliest.starts_at, earliest.ends_at, earliest.timezone
        FROM professional_profiles p
        JOIN account_memberships m ON m.account_id = p.account_id AND m.role = 'professional'
        LEFT JOIN LATERAL (
            SELECT a.id, a.starts_at, a.ends_at, a.timezone
            FROM professional_availability_slots a
            WHERE a.professional_account_id = p.account_id AND `+bookableAvailabilitySQL("a", "p", "")+`
            ORDER BY a.starts_at, a.id LIMIT 1
        ) earliest ON true
        WHERE p.status = 'active' AND m.status = 'active' AND p.category = $1 AND p.account_id <> $2
        ORDER BY earliest.starts_at ASC NULLS LAST, p.display_name, p.id
        LIMIT 50`, category, excludeAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matches := make([]workflow.Match, 0)
	for rows.Next() {
		var match workflow.Match
		var slotID, timezone *string
		var start, end *time.Time
		if err := rows.Scan(&match.ID, &match.AccountID, &match.Name, &match.Category, &match.Area, &slotID, &start, &end, &timezone); err != nil {
			return nil, err
		}
		if slotID != nil && start != nil && end != nil && timezone != nil {
			zone := loadTimezone(*timezone)
			match.EarliestAppointment = &workflow.Appointment{SlotID: *slotID, Timezone: *timezone,
				StartUTC: start.UTC().Format(time.RFC3339), EndUTC: end.UTC().Format(time.RFC3339),
				StartsAt: start.In(zone).Format("Mon, 2 Jan · 15:04") + "–" + end.In(zone).Format("15:04")}
		}
		if !serviceAreaCovers(match.Area, location.Label) {
			continue
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

func (s *Store) CatalogOffer(ctx context.Context, profileID string) (workflow.CatalogOffer, error) {
	profile, err := scanProfile(s.pool.QueryRow(ctx, `
		SELECT `+qualifiedProfileColumns("p")+`
		FROM professional_profiles p
		JOIN account_memberships m ON m.account_id = p.account_id AND m.role = 'professional'
		WHERE p.id = $1 AND p.status = 'active' AND m.status = 'active'`, profileID))
	if err != nil {
		return workflow.CatalogOffer{}, err
	}
	return workflow.CatalogOffer{
		ProfileID: profile.ID, AccountID: profile.AccountID, DisplayName: profile.DisplayName,
		Category: profile.Category, ServiceName: profile.ServiceName, ServiceDescription: profile.ServiceDescription,
		ServiceArea: profile.ServiceArea, Currency: strings.TrimSpace(profile.Currency), PriceMinor: profile.PriceMinor,
		CalloutMinor: profile.CalloutMinor, DurationMinutes: profile.DurationMinutes, MaterialsPolicy: profile.MaterialsPolicy,
		Exclusions: profile.Exclusions,
	}, nil
}

func (s *Store) AddProfessionalAvailability(ctx context.Context, accountID string, slot workflow.Appointment) (string, error) {
	id, err := randomID("slot_")
	if err != nil {
		return "", err
	}
	start, err := time.Parse(time.RFC3339, slot.StartUTC)
	if err != nil {
		return "", errors.New("slot start must be an RFC3339 timestamp")
	}
	end, err := time.Parse(time.RFC3339, slot.EndUTC)
	if err != nil || !end.After(start) {
		return "", errors.New("slot end must be after its start")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, accountID); err != nil {
		return "", err
	}
	if err := validateManualAvailabilityTx(ctx, tx, accountID, start, end); err != nil {
		return "", err
	}
	var overlaps bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM professional_availability_slots
			WHERE professional_account_id = $1 AND status IN ('available', 'booked')
			  AND starts_at < $3 AND ends_at > $2
		)`, accountID, start, end).Scan(&overlaps)
	if err != nil {
		return "", err
	}
	if overlaps {
		return "", errors.New("availability overlaps an existing time slot")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO professional_availability_slots (id, professional_account_id, starts_at, ends_at, timezone)
		VALUES ($1, $2, $3, $4, $5)`, id, accountID, start, end, slot.Timezone); err != nil {
		return "", err
	}
	if _, _, err := s.materializeSchedule(ctx, tx, accountID, time.Now()); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) ListProfessionalAvailability(ctx context.Context, accountID string) ([]workflow.AvailabilitySlot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.starts_at, a.ends_at, a.timezone, a.status
		FROM professional_availability_slots a JOIN professional_profiles p ON p.account_id=a.professional_account_id
		WHERE a.professional_account_id = $1 AND a.starts_at > now() AND (a.status='booked' OR (`+bookableAvailabilitySQL("a", "p", "")+`))
		ORDER BY a.starts_at LIMIT 100`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]workflow.AvailabilitySlot, 0)
	for rows.Next() {
		var entry workflow.AvailabilitySlot
		var startsAt, endsAt time.Time
		if err := rows.Scan(&entry.SlotID, &startsAt, &endsAt, &entry.Timezone, &entry.Status); err != nil {
			return nil, err
		}
		entry.StartUTC, entry.EndUTC = startsAt.UTC().Format(time.RFC3339), endsAt.UTC().Format(time.RFC3339)
		entry.StartsAt = startsAt.In(loadTimezone(entry.Timezone)).Format("Mon, 2 Jan · 15:04")
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func loadTimezone(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return location
}

func (s *Store) AvailableProfessionalSlots(ctx context.Context, accountID, excludeJobID string) ([]workflow.Appointment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.starts_at, a.ends_at, a.timezone
		FROM professional_availability_slots a
		JOIN professional_profiles p ON p.account_id = a.professional_account_id
		JOIN account_memberships m ON m.account_id = p.account_id AND m.role = 'professional'
		WHERE a.professional_account_id = $1 AND p.status = 'active' AND m.status = 'active'
		 AND `+bookableAvailabilitySQL("a", "p", "$2")+`
		ORDER BY a.starts_at LIMIT 20`, accountID, excludeJobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	slots := make([]workflow.Appointment, 0)
	for rows.Next() {
		var slot workflow.Appointment
		var start, end time.Time
		if err := rows.Scan(&slot.SlotID, &start, &end, &slot.Timezone); err != nil {
			return nil, err
		}
		zone := loadTimezone(slot.Timezone)
		slot.StartsAt = start.In(zone).Format("Mon, 2 Jan · 15:04") + "–" + end.In(zone).Format("15:04")
		slot.StartUTC, slot.EndUTC = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

func (s *Store) ListProfessionalApplications(ctx context.Context) ([]workflow.ProfessionalApplication, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+qualifiedProfileColumns("p")+`, a.email,
			c.identity_business_status, c.paypal_onboarding_status, c.qualification_status, c.insurance_status,
			c.qualification_rationale, c.insurance_rationale
		FROM professional_profiles p JOIN accounts a ON a.id = p.account_id
		JOIN professional_review_checks c ON c.professional_profile_id = p.id
	JOIN account_memberships m ON m.account_id = p.account_id AND m.role = 'professional'
	WHERE p.status = 'pending' AND m.status IN ('pending','active') ORDER BY p.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applications := make([]workflow.ProfessionalApplication, 0)
	for rows.Next() {
		var application workflow.ProfessionalApplication
		var profile workflow.ProfessionalProfile
		var exclusions []byte
		if err := rows.Scan(&profile.ID, &profile.AccountID, &profile.DisplayName, &profile.Category, &profile.ServiceName, &profile.ServiceDescription,
			&profile.ServiceArea, &profile.Latitude, &profile.Longitude, &profile.Currency, &profile.PriceMinor, &profile.CalloutMinor,
			&profile.DurationMinutes, &profile.MaterialsPolicy, &exclusions, &profile.Status, &application.Email,
			&application.Checks.IdentityBusiness, &application.Checks.PayPalOnboarding, &application.Checks.Qualification,
			&application.Checks.Insurance, &application.Checks.QualificationReason, &application.Checks.InsuranceReason); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(exclusions, &profile.Exclusions); err != nil {
			return nil, err
		}
		application.Profile = profile
		applications = append(applications, application)
	}
	return applications, rows.Err()
}

func (s *Store) ListWorkExceptions(ctx context.Context) ([]workflow.OperatorCase, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,account_id,state_json FROM account_workspaces
  WHERE COALESCE((state_json #>> '{intake,needsReview}')::boolean,false)
   OR COALESCE((state_json #>> '{intake,waitlisted}')::boolean,false)
   OR state_json #>> '{cancellationReview,status}'='pending'
   OR state_json #>> '{job,dispute,status}' IN ('open','refund_pending','partial_refund_review','payment_reversal_review')
   OR (NULLIF(state_json #>> '{job,id}','') IS NOT NULL AND NOT COALESCE((state_json->>'cancelled')::boolean,false) AND NOT COALESCE((state_json #>> '{job,paid}')::boolean,false))
  ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	type workspaceException struct {
		workspaceID, accountID string
		stateJSON              []byte
	}
	workspaces := make([]workspaceException, 0)
	for rows.Next() {
		var item workspaceException
		if err := rows.Scan(&item.workspaceID, &item.accountID, &item.stateJSON); err != nil {
			rows.Close()
			return nil, err
		}
		workspaces = append(workspaces, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	cases := make([]workflow.OperatorCase, 0)
	for _, item := range workspaces {
		workspaceID, accountID, stateJSON := item.workspaceID, item.accountID, item.stateJSON
		var state workflow.State
		if err := json.Unmarshal(stateJSON, &state); err != nil {
			return nil, err
		}
		state.WorkspaceID = workspaceID
		if err := hydrateWaitingTimes(ctx, s.pool, &state); err != nil {
			return nil, err
		}
		if state.Intake != nil && state.Intake.NeedsReview {
			cases = append(cases, workflow.OperatorCase{ID: "intake:" + workspaceID, AccountID: accountID, WorkspaceID: workspaceID, Kind: "intake-review", Description: state.Intake.Description, Category: state.Intake.Category, Detail: state.Intake.Detail, Urgency: state.Intake.Urgency, Location: state.Intake.Location})
		}
		if state.Intake != nil && state.Intake.Waitlisted {
			cases = append(cases, workflow.OperatorCase{ID: "waitlist:" + workspaceID, AccountID: accountID, WorkspaceID: workspaceID, Kind: "area-waitlist", Description: state.Intake.Description, Category: state.Intake.Category, Detail: state.Intake.Detail, Urgency: state.Intake.Urgency, Location: state.Intake.Location})
		}
		if state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
			kind := state.CancellationReview.Kind
			if kind == "" {
				kind = "late-cancellation"
			}
			cases = append(cases, workflow.OperatorCase{ID: kind + ":" + workspaceID, AccountID: accountID, WorkspaceID: workspaceID, Kind: kind, Description: state.CancellationReview.Reason, ReportedAt: state.CancellationReview.RequestedAt, Category: state.Job.Category, JobID: state.Job.ID, JobTitle: state.Job.Title, Paid: state.Job.Paid})
		}
		if state.Job.Change != nil && state.Job.Change.Status == "pending" {
			cases = append(cases, workflow.OperatorCase{ID: "change:" + state.Job.ID, AccountID: accountID, WorkspaceID: workspaceID, Kind: "scope-change", Description: state.Job.Change.Description, Category: state.Job.Category, JobID: state.Job.ID, JobTitle: state.Job.Title})
		}
		if state.Job.Dispute != nil && (state.Job.Dispute.Status == "open" || state.Job.Dispute.Status == "refund_pending" || state.Job.Dispute.Status == "partial_refund_review" || state.Job.Dispute.Status == "payment_reversal_review") {
			cases = append(cases, workflow.OperatorCase{ID: "issue:" + workspaceID, AccountID: accountID, WorkspaceID: workspaceID, Kind: "job-issue", Description: state.Job.Dispute.Reason, ReportedAt: state.Job.Dispute.ReportedAt, Status: state.Job.Dispute.Status, Category: state.Job.Category, JobID: state.Job.ID, JobTitle: state.Job.Title, Paid: state.Job.Paid})
		}
		if state.Job.EvidenceSubmitted && !state.Job.CompletionApproved && (!state.Job.DetailAdded || len(workflow.MissingEvidence(state.Job, true)) > 0) {
			cases = append(cases, workflow.OperatorCase{ID: "evidence:" + state.Job.ID, AccountID: accountID, WorkspaceID: workspaceID, Kind: "missing-evidence", Description: state.Job.Description, Category: state.Job.Category, JobID: state.Job.ID, JobTitle: state.Job.Title})
		}
		for _, wait := range workflow.WaitingStates(state) {
			if !wait.Due(time.Now().UTC()) {
				continue
			}
			if wait.EventType == "checkout-reminder" {
				status, err := paymentStatusTx(ctx, s.pool, state.Job)
				if err != nil {
					return nil, err
				}
				if !checkoutReminderActionable(status) {
					continue
				}
			}
			cases = append(cases, workflow.OperatorCase{ID: "stalled:" + workspaceID + ":" + wait.EventType, AccountID: accountID, WorkspaceID: workspaceID, Kind: "stalled-job", Description: wait.Detail + " Responsible participant: " + wait.ResponsibleRole + ". Operator action: " + wait.ResolutionAction + ".", ReportedAt: wait.StartedAt, Status: wait.EventType, Category: state.Job.Category, JobID: state.Job.ID, JobTitle: state.Job.Title, ReasonCode: wait.EventType, Responsible: wait.ResponsibleRole})
		}
	}
	coordination, err := s.coordinationExceptions(ctx)
	if err != nil {
		return nil, err
	}
	return append(cases, coordination...), nil
}

func (s *Store) ReviewProfessionalApplication(ctx context.Context, operatorID, professionalID, decision string, checks workflow.ProfessionalReviewChecks) error {
	if err := checks.Validate(); err != nil {
		return err
	}
	if decision != "save" && decision != "approve" && decision != "reject" {
		return errors.New("unsupported professional review decision")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var accountID string
	if err := tx.QueryRow(ctx, `
		SELECT account_id FROM professional_profiles
		WHERE id = $1 AND status = 'pending' FOR UPDATE`, professionalID).Scan(&accountID); err != nil {
		return err
	}
	var paypalConnected bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM paypal_seller_accounts
		WHERE account_id=$1 AND status='verified' AND payments_receivable AND primary_email_confirmed
	)`, accountID).Scan(&paypalConnected); err != nil {
		return err
	}
	if paypalConnected {
		checks.PayPalOnboarding = "verified"
	} else {
		checks.PayPalOnboarding = "pending"
	}
	if decision == "approve" && !checks.ReadyToApprove() {
		return errors.New("required professional review checks are incomplete or PayPal payouts are not connected")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO professional_review_checks (
			professional_profile_id, identity_business_status, paypal_onboarding_status, qualification_status,
			insurance_status, qualification_rationale, insurance_rationale, updated_by, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (professional_profile_id) DO UPDATE SET
			identity_business_status = EXCLUDED.identity_business_status,
			paypal_onboarding_status = EXCLUDED.paypal_onboarding_status,
			qualification_status = EXCLUDED.qualification_status,
			insurance_status = EXCLUDED.insurance_status,
			qualification_rationale = EXCLUDED.qualification_rationale,
			insurance_rationale = EXCLUDED.insurance_rationale,
			updated_by = EXCLUDED.updated_by, updated_at = now()`,
		professionalID, checks.IdentityBusiness, checks.PayPalOnboarding, checks.Qualification,
		checks.Insurance, strings.TrimSpace(checks.QualificationReason), strings.TrimSpace(checks.InsuranceReason), operatorID)
	if err != nil {
		return err
	}
	if decision == "approve" || decision == "reject" {
		profileStatus, membershipStatus := "rejected", "disabled"
		if decision == "approve" {
			profileStatus, membershipStatus = "active", "active"
		}
		result, err := tx.Exec(ctx, `
			UPDATE professional_profiles SET status = $2, updated_at = now()
			WHERE id = $1 AND status = 'pending'`, professionalID, profileStatus)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return errors.New("professional application is no longer pending review")
		}
		result, err = tx.Exec(ctx, `
			UPDATE account_memberships SET status = CASE WHEN status='active' AND $2='disabled' THEN 'active' ELSE $2 END, updated_at = now()
			WHERE account_id = $1 AND role = 'professional' AND status IN ('pending','active')`, accountID, membershipStatus)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return errors.New("professional workspace is not pending review")
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (account_id, actor_account_id, job_id, actor, action, scope_version, revision)
		VALUES ($1, $2, $3, 'operator', $4, 0, 1)`, accountID, operatorID, "profile:"+professionalID, map[string]string{
		"save": "save-professional-review-checks", "approve": "approve-professional", "reject": "reject-professional",
	}[decision])
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
