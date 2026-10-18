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
