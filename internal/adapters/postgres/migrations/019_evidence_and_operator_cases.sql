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
