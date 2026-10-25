CREATE TABLE booking_autonomy_consents (
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    actor_account_id text REFERENCES accounts(id) ON DELETE SET NULL,
    mode text NOT NULL CHECK (mode IN ('automatic', 'manual')),
    scope_version integer NOT NULL,
    amount_minor bigint NOT NULL,
    currency text NOT NULL,
    professional_account_id text NOT NULL,
    professional_profile_id text NOT NULL,
    decided_at timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, job_id, version)
);
