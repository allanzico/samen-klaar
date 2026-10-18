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
