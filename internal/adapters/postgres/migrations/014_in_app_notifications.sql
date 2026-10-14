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

INSERT INTO in_app_notifications (workspace_id, job_id, recipient_account_id, recipient_role, event_key, event_type, created_at)
SELECT w.id, n.job_id,
       CASE n.recipient_role WHEN 'customer' THEN w.account_id ELSE w.state_json #>> '{job,professionalAccountId}' END,
       n.recipient_role, n.event_key, n.event_type, n.created_at
FROM notification_outbox n
JOIN account_workspaces w ON w.state_json #>> '{job,id}' = n.job_id
WHERE n.recipient_role = 'customer'
   OR NULLIF(w.state_json #>> '{job,professionalAccountId}', '') IS NOT NULL
ON CONFLICT (recipient_account_id, event_key) DO NOTHING;

CREATE INDEX in_app_notifications_account_recent_idx
    ON in_app_notifications (recipient_account_id, recipient_role, created_at DESC, id DESC);

CREATE INDEX in_app_notifications_unread_idx
    ON in_app_notifications (recipient_account_id, recipient_role, created_at DESC)
    WHERE read_at IS NULL;
