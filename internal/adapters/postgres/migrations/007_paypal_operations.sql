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
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, scope_version)
);

CREATE INDEX paypal_payment_operations_workspace_idx ON paypal_payment_operations (workspace_id, updated_at DESC);
