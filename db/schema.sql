-- Canonical current PostgreSQL schema. Runtime upgrades are applied by the
-- ordered migrations in internal/adapters/postgres/migrations.
CREATE TABLE scope_versions (
    job_id text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    currency char(3) NOT NULL,
    total_minor bigint NOT NULL CHECK (total_minor >= 0),
    lines_json jsonb NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, version)
);

CREATE TABLE booking_reservations (
    job_id text PRIMARY KEY,
    professional_id text NOT NULL,
    slot_id text NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    timezone text NOT NULL,
    status text NOT NULL CHECK (status IN ('booked', 'completed', 'released')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (starts_at < ends_at)
);
CREATE INDEX booking_professional_window_idx ON booking_reservations (professional_id, starts_at, ends_at) WHERE status = 'booked';

CREATE TABLE audit_events (
    id bigserial PRIMARY KEY,
    job_id text NOT NULL,
    actor text NOT NULL,
    action text NOT NULL,
    scope_version integer NOT NULL,
    revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    account_id text,
    actor_account_id text,
    workspace_id text
);
CREATE INDEX audit_job_created_idx ON audit_events (job_id, created_at, id);
CREATE INDEX audit_account_created_idx ON audit_events (account_id, created_at, id);
CREATE INDEX audit_actor_account_created_idx ON audit_events (actor_account_id, created_at, id);
CREATE INDEX audit_workspace_created_idx ON audit_events (workspace_id, created_at, id);

CREATE TABLE notification_outbox (
    job_id text NOT NULL,
    event_key text NOT NULL,
    event_type text NOT NULL,
    recipient_role text NOT NULL CHECK (recipient_role IN ('customer', 'professional')),
    status text NOT NULL CHECK (status IN ('queued', 'sending', 'adapter_unconfigured', 'sent', 'failed', 'obsolete')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, event_key)
);
CREATE INDEX notification_outbox_pending_idx ON notification_outbox (status, created_at)
    WHERE status IN ('queued', 'failed', 'adapter_unconfigured');

CREATE TABLE accounts (
    id text PRIMARY KEY,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    preferred_role text NOT NULL DEFAULT 'customer' CHECK (preferred_role IN ('customer', 'professional', 'operator')),
    locale text NOT NULL DEFAULT 'en' CHECK (locale IN ('en', 'nl')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);
CREATE TABLE account_memberships (
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('customer', 'professional', 'operator')),
    status text NOT NULL CHECK (status IN ('pending', 'active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, role)
);
CREATE TABLE account_sessions (
    token_hash text PRIMARY KEY,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    active_role text NOT NULL CHECK (active_role IN ('customer', 'professional', 'operator')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (account_id, active_role) REFERENCES account_memberships(account_id, role) ON DELETE CASCADE
);
CREATE INDEX account_sessions_expiry_idx ON account_sessions (expires_at);
CREATE INDEX account_sessions_account_idx ON account_sessions (account_id);
CREATE TABLE oidc_transactions (
    state_hash text PRIMARY KEY,
    nonce text NOT NULL,
    code_verifier text NOT NULL,
    requested_role text NOT NULL CHECK (requested_role IN ('customer', 'professional', 'operator')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX oidc_transactions_expiry_idx ON oidc_transactions (expires_at);

CREATE TABLE professional_profiles (
    id text PRIMARY KEY,
    account_id text NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    display_name text NOT NULL,
    category text NOT NULL,
    service_name text NOT NULL,
    service_description text NOT NULL DEFAULT '',
    service_area text NOT NULL,
    latitude double precision NOT NULL DEFAULT 0 CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision NOT NULL DEFAULT 0 CHECK (longitude >= -180 AND longitude <= 180),
    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    price_minor bigint NOT NULL CHECK (price_minor >= 0),
    callout_minor bigint NOT NULL CHECK (callout_minor >= 0),
    duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 15 AND 600),
    materials_policy text NOT NULL,
    exclusions_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL CHECK (status IN ('pending', 'active', 'rejected', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    base_location jsonb,
    travel_radius_km integer NOT NULL DEFAULT 0 CHECK (travel_radius_km BETWEEN 0 AND 100),
    CONSTRAINT professional_geographic_coverage CHECK ((base_location IS NULL AND travel_radius_km = 0) OR (base_location IS NOT NULL AND travel_radius_km BETWEEN 1 AND 100))
);
CREATE TABLE professional_review_checks (
    professional_profile_id text PRIMARY KEY REFERENCES professional_profiles(id) ON DELETE CASCADE,
    identity_business_status text NOT NULL DEFAULT 'pending' CHECK (identity_business_status IN ('pending', 'verified')),
    paypal_onboarding_status text NOT NULL DEFAULT 'pending' CHECK (paypal_onboarding_status IN ('pending', 'verified')),
    qualification_status text NOT NULL DEFAULT 'pending' CHECK (qualification_status IN ('pending', 'verified', 'not_required')),
    insurance_status text NOT NULL DEFAULT 'pending' CHECK (insurance_status IN ('pending', 'verified', 'not_required')),
    qualification_rationale text NOT NULL DEFAULT '' CHECK (char_length(qualification_rationale) <= 500),
    insurance_rationale text NOT NULL DEFAULT '' CHECK (char_length(insurance_rationale) <= 500),
    updated_by text REFERENCES accounts(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (qualification_status <> 'not_required' OR length(btrim(qualification_rationale)) > 0),
    CHECK (insurance_status <> 'not_required' OR length(btrim(insurance_rationale)) > 0)
);
CREATE INDEX professional_directory_category_status_idx ON professional_profiles (category, status, service_area);
CREATE TABLE professional_availability_slots (
    id text PRIMARY KEY,
    professional_account_id text NOT NULL REFERENCES professional_profiles(account_id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    timezone text NOT NULL,
    status text NOT NULL CHECK (status IN ('available', 'booked', 'cancelled')) DEFAULT 'available',
    source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'schedule')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (starts_at < ends_at),
    UNIQUE (professional_account_id, starts_at, ends_at)
);
CREATE INDEX professional_availability_window_idx ON professional_availability_slots (professional_account_id, starts_at) WHERE status = 'available';

CREATE TABLE account_workspaces (
    id text PRIMARY KEY,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    state_json jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX account_workspaces_customer_idx ON account_workspaces (account_id, updated_at DESC);
CREATE INDEX account_workspaces_professional_idx ON account_workspaces ((state_json #>> '{job,professionalAccountId}'), updated_at DESC) WHERE COALESCE(state_json #>> '{job,professionalAccountId}', '') <> '';
CREATE INDEX account_workspaces_job_idx ON account_workspaces ((state_json #>> '{job,id}'));

CREATE TABLE in_app_notifications (
    id bigserial PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    recipient_account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    recipient_role text NOT NULL CHECK (recipient_role IN ('customer', 'professional')),
    event_key text NOT NULL,
    event_type text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    UNIQUE (recipient_account_id, event_key)
);

CREATE INDEX in_app_notifications_account_recent_idx
    ON in_app_notifications (recipient_account_id, recipient_role, created_at DESC, id DESC);

CREATE INDEX in_app_notifications_unread_idx
    ON in_app_notifications (recipient_account_id, recipient_role, created_at DESC)
    WHERE read_at IS NULL;

CREATE TABLE paypal_payment_operations (
    job_id text NOT NULL,
    scope_version integer NOT NULL CHECK (scope_version > 0),
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    order_id text UNIQUE,
    authorization_id text,
    capture_id text,
    approval_url text NOT NULL DEFAULT '',
    status text NOT NULL,
    order_request_id text NOT NULL,
    authorize_request_id text NOT NULL,
    capture_request_id text NOT NULL,
    void_request_id text NOT NULL,
    refund_request_id text NOT NULL DEFAULT '',
    refund_id text,
    seller_merchant_id text NOT NULL DEFAULT '',
    authorization_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, scope_version)
);
CREATE INDEX paypal_payment_operations_workspace_idx ON paypal_payment_operations (workspace_id, updated_at DESC);

CREATE TABLE paypal_webhook_events (
    event_id text PRIMARY KEY,
    event_type text NOT NULL,
    event_json jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'processed', 'ignored', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 100),
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);
CREATE INDEX paypal_webhook_events_pending_idx ON paypal_webhook_events (status, received_at)
    WHERE status IN ('queued', 'failed');

CREATE TABLE paypal_seller_accounts (
    account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    tracking_id text NOT NULL UNIQUE,
    request_id text NOT NULL,
    referral_self_url text NOT NULL DEFAULT '',
    merchant_id text UNIQUE,
    status text NOT NULL DEFAULT 'creating' CHECK (status IN ('creating', 'pending', 'verified', 'ineligible', 'revoked')),
    payments_receivable boolean NOT NULL DEFAULT false,
    primary_email_confirmed boolean NOT NULL DEFAULT false,
    permissions_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    checked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX paypal_seller_accounts_status_idx ON paypal_seller_accounts (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS professional_profiles_active_service_idx
ON professional_profiles (category, service_name) WHERE status = 'active';

-- Migrations 017, 019, 021, 022 and 023 (tables only; the parent-request trigger lives in the migration).
CREATE TABLE professional_schedules (
    professional_account_id text PRIMARY KEY REFERENCES professional_profiles(account_id) ON DELETE CASCADE,
    timezone text NOT NULL DEFAULT 'Europe/Amsterdam',
    gap_minutes integer NOT NULL DEFAULT 30 CHECK (gap_minutes BETWEEN 0 AND 120),
    notice_minutes integer NOT NULL DEFAULT 1440 CHECK (notice_minutes BETWEEN 60 AND 20160),
    horizon_days integer NOT NULL DEFAULT 28 CHECK (horizon_days BETWEEN 7 AND 90),
    materialized_through date,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE professional_working_hours (
    professional_account_id text NOT NULL REFERENCES professional_schedules(professional_account_id) ON DELETE CASCADE,
    weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    starts_local time NOT NULL,
    ends_local time NOT NULL,
    CHECK (starts_local < ends_local),
    PRIMARY KEY (professional_account_id, weekday, starts_local)
);

CREATE TABLE professional_days_off (
    id text PRIMARY KEY,
    professional_account_id text NOT NULL REFERENCES professional_schedules(professional_account_id) ON DELETE CASCADE,
    first_day date NOT NULL,
    last_day date NOT NULL,
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 80),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (first_day <= last_day AND last_day - first_day < 366)
);
CREATE INDEX professional_days_off_window_idx ON professional_days_off (professional_account_id, last_day);

CREATE TABLE professional_slot_exclusions (
    professional_account_id text NOT NULL REFERENCES professional_schedules(professional_account_id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (professional_account_id, starts_at)
);


CREATE TABLE professional_evidence_policies (
 profile_id text PRIMARY KEY REFERENCES professional_profiles(id) ON DELETE CASCADE,
 requirements jsonb NOT NULL DEFAULT '[]', proposed jsonb NOT NULL DEFAULT '[]',
 status text NOT NULL DEFAULT 'pending', reviewed_at timestamptz, reviewed_by text REFERENCES accounts(id), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE operator_case_work (
 workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
 kind text NOT NULL, assigned_to text REFERENCES accounts(id), status text NOT NULL DEFAULT 'open',
 outcome text NOT NULL DEFAULT '', updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(workspace_id,kind)
);
CREATE TABLE operator_case_notes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
 kind text NOT NULL, actor_account_id text NOT NULL REFERENCES accounts(id),
 recipient_role text NOT NULL DEFAULT '', note text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX operator_case_notes_workspace ON operator_case_notes(workspace_id,id);
CREATE TABLE operator_case_access (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
 kind text NOT NULL, actor_account_id text NOT NULL REFERENCES accounts(id), accessed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE coordination_tasks (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    scope_version integer NOT NULL CHECK (scope_version > 0),
    completion_round integer NOT NULL CHECK (completion_round > 0),
    trigger text NOT NULL CHECK (trigger IN ('completion-note', 'post-booking-scope')),
    recipient_role text NOT NULL DEFAULT 'professional' CHECK (recipient_role = 'professional'),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'awaiting_reply', 'queued_summary', 'summary_ready', 'operator_review', 'completed', 'obsolete')),
    source_message text NOT NULL DEFAULT '' CHECK (char_length(source_message) <= 1000),
    question text NOT NULL DEFAULT '' CHECK (char_length(question) <= 500),
    reply text NOT NULL DEFAULT '' CHECK (char_length(reply) <= 1500),
    summary text NOT NULL DEFAULT '' CHECK (char_length(summary) <= 2000),
    proposed_action text NOT NULL DEFAULT '' CHECK (proposed_action IN ('', 'request-work-note', 'review-scope', 'review-completion', 'operator-review')),
    outcome text NOT NULL DEFAULT '' CHECK (char_length(outcome) <= 500),
    revision bigint NOT NULL DEFAULT 1,
    workspace_revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(workspace_id,job_id,scope_version,completion_round,trigger)
);
CREATE INDEX coordination_tasks_active_idx ON coordination_tasks(workspace_id,status);

CREATE TABLE parent_requests (
 id text PRIMARY KEY, account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 description text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE request_job_plans (
 parent_request_id text NOT NULL REFERENCES parent_requests(id) ON DELETE CASCADE,
 plan_id text NOT NULL, workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
 description text NOT NULL, PRIMARY KEY(parent_request_id,plan_id)
);

CREATE TABLE consumer_policy_versions (
    kind text NOT NULL CHECK (kind IN ('terms','cancellation','withdrawal','early_start')),
    version text NOT NULL CHECK (length(version) BETWEEN 1 AND 80),
    locale text NOT NULL CHECK (locale IN ('en','nl')),
    title text NOT NULL CHECK (length(title) BETWEEN 3 AND 160),
    body text NOT NULL CHECK (length(body) BETWEEN 10 AND 20000),
    requires_acknowledgment boolean NOT NULL DEFAULT false,
    reviewer_id text NOT NULL REFERENCES accounts(id),
    review_rationale text NOT NULL CHECK (length(review_rationale) BETWEEN 10 AND 1000),
    review_reference text NOT NULL CHECK (length(review_reference) BETWEEN 1 AND 1000),
    published_by text NOT NULL REFERENCES accounts(id),
    published_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind,version)
);
CREATE INDEX consumer_policy_versions_current_idx ON consumer_policy_versions (kind,published_at DESC);

CREATE TABLE consumer_policy_acknowledgments (
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    scope_version integer NOT NULL CHECK (scope_version > 0),
    policy_kind text NOT NULL,
    policy_version text NOT NULL,
    acknowledged_at timestamptz NOT NULL,
    PRIMARY KEY (workspace_id,job_id,scope_version,policy_kind,policy_version),
    FOREIGN KEY (policy_kind,policy_version) REFERENCES consumer_policy_versions(kind,version)
);

CREATE TABLE agent_mandates (
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('customer', 'professional')),
    mandate jsonb NOT NULL,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, role)
);

CREATE TABLE agent_decisions (
    id bigserial PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('customer', 'professional')),
    mandate_version integer NOT NULL CHECK (mandate_version > 0),
    code text NOT NULL CHECK (length(code) BETWEEN 1 AND 80),
    action text NOT NULL DEFAULT '',
    params jsonb NOT NULL DEFAULT '{}'::jsonb,
    outcome text NOT NULL CHECK (outcome IN ('applied', 'rejected')),
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX agent_decisions_workspace_idx ON agent_decisions (workspace_id, account_id, role, id DESC);

CREATE TABLE agent_escalations (
    id bigserial PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('customer', 'professional')),
    code text NOT NULL CHECK (length(code) BETWEEN 1 AND 80),
    params jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);
CREATE UNIQUE INDEX agent_escalations_open_idx ON agent_escalations (workspace_id, account_id, role) WHERE resolved_at IS NULL;
CREATE INDEX agent_escalations_account_idx ON agent_escalations (account_id, role, created_at DESC) WHERE resolved_at IS NULL;

CREATE TABLE professional_removed_requests (
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    removed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, account_id, job_id)
);
CREATE INDEX professional_removed_requests_account_idx ON professional_removed_requests (account_id);

CREATE TABLE operator_membership_suspensions (
    account_id text NOT NULL,
    role text NOT NULL CHECK (role IN ('customer', 'professional')),
    previous_status text NOT NULL CHECK (previous_status IN ('pending', 'active')),
    PRIMARY KEY (account_id, role),
    FOREIGN KEY (account_id, role) REFERENCES account_memberships(account_id, role) ON DELETE CASCADE
);

CREATE TABLE operator_admin_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    actor_account_id text REFERENCES accounts(id) ON DELETE SET NULL,
    action text NOT NULL CHECK (action IN ('suspend-role', 'restore-role', 'revoke-sessions')),
    role text NOT NULL DEFAULT '' CHECK (role IN ('', 'customer', 'professional')),
    previous_status text NOT NULL DEFAULT '',
    next_status text NOT NULL DEFAULT '',
    reason text NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 10 AND 1000),
    affected_sessions integer NOT NULL DEFAULT 0 CHECK (affected_sessions >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX operator_admin_events_account_idx ON operator_admin_events(account_id, id DESC);
CREATE INDEX accounts_directory_created_idx ON accounts(created_at DESC, id);
CREATE INDEX accounts_directory_email_idx ON accounts(lower(btrim(email)));
