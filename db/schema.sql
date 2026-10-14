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
    status text NOT NULL CHECK (status IN ('queued', 'sending', 'adapter_unconfigured', 'sent', 'failed')),
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
    updated_at timestamptz NOT NULL DEFAULT now()
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
