-- A customer account can own many requests. Professionals can be assigned to many jobs.
CREATE TABLE account_workspaces (
    id text PRIMARY KEY,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    state_json jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX account_workspaces_customer_idx ON account_workspaces (account_id, updated_at DESC);
CREATE INDEX account_workspaces_professional_idx
    ON account_workspaces ((state_json #>> '{job,professionalAccountId}'), updated_at DESC)
    WHERE COALESCE(state_json #>> '{job,professionalAccountId}', '') <> '';
CREATE INDEX account_workspaces_job_idx ON account_workspaces ((state_json #>> '{job,id}'));

INSERT INTO account_workspaces (id, account_id, state_json, revision, updated_at)
SELECT 'legacy_' || account_id,
       account_id,
       jsonb_set(state_json, '{workspaceId}', to_jsonb('legacy_' || account_id), true),
       revision,
       updated_at
FROM account_states
ON CONFLICT (id) DO NOTHING;

ALTER TABLE audit_events ADD COLUMN workspace_id text;
CREATE INDEX audit_workspace_created_idx ON audit_events (workspace_id, created_at, id);
