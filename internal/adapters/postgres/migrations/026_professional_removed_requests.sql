-- A professional can take a job the customer has not approved off their list. The customer's
-- request is declined, not deleted; only the professional's view changes.
CREATE TABLE professional_removed_requests (
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    removed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, account_id, job_id)
);
CREATE INDEX professional_removed_requests_account_idx ON professional_removed_requests (account_id);
