CREATE TABLE IF NOT EXISTS relay_demo_state (
    id smallint PRIMARY KEY CHECK (id = 1),
    scenario text NOT NULL,
    job_id text NOT NULL,
    state_json jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS relay_scope_versions (
    job_id text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    currency char(3) NOT NULL,
    total_minor bigint NOT NULL CHECK (total_minor >= 0),
    lines_json jsonb NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, version)
);

CREATE TABLE IF NOT EXISTS relay_booking_reservations (
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

CREATE INDEX IF NOT EXISTS relay_booking_professional_window_idx
    ON relay_booking_reservations (professional_id, starts_at, ends_at)
    WHERE status = 'booked';

CREATE TABLE IF NOT EXISTS relay_audit_events (
    id bigserial PRIMARY KEY,
    job_id text NOT NULL,
    actor text NOT NULL,
    action text NOT NULL,
    scope_version integer NOT NULL,
    revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS relay_audit_job_created_idx
    ON relay_audit_events (job_id, created_at, id);

CREATE TABLE IF NOT EXISTS relay_notification_outbox (
    job_id text NOT NULL,
    event_key text NOT NULL,
    event_type text NOT NULL,
    recipient_role text NOT NULL CHECK (recipient_role IN ('customer', 'professional')),
    status text NOT NULL CHECK (status IN ('queued', 'adapter_unconfigured', 'sent', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, event_key)
);
