CREATE TABLE accounts (
    id text PRIMARY KEY,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
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

CREATE TABLE account_states (
    account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    state_json jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE audit_events ADD COLUMN account_id text REFERENCES accounts(id) ON DELETE SET NULL;
ALTER TABLE audit_events ADD COLUMN actor_account_id text REFERENCES accounts(id) ON DELETE SET NULL;
CREATE INDEX audit_account_created_idx ON audit_events (account_id, created_at, id);
CREATE INDEX audit_actor_account_created_idx ON audit_events (actor_account_id, created_at, id);
