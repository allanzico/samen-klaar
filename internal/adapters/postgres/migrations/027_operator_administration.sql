CREATE TABLE operator_membership_suspensions (
    account_id text NOT NULL,
    role text NOT NULL CHECK (role IN ('customer', 'professional')),
    previous_status text NOT NULL CHECK (previous_status IN ('pending', 'active')),
    PRIMARY KEY (account_id, role),
    FOREIGN KEY (account_id, role) REFERENCES account_memberships(account_id, role) ON DELETE CASCADE
);

CREATE TABLE operator_admin_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    actor_account_id text REFERENCES accounts(id) ON DELETE SET NULL,
    action text NOT NULL CHECK (action IN ('suspend-role', 'restore-role', 'revoke-sessions')),
    role text NOT NULL DEFAULT '' CHECK (role IN ('', 'customer', 'professional')),
    previous_status text NOT NULL DEFAULT '',
    next_status text NOT NULL DEFAULT '',
    reason text NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 10 AND 1000),
    affected_sessions integer NOT NULL DEFAULT 0 CHECK (affected_sessions >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX operator_admin_events_account_idx ON operator_admin_events(account_id, id DESC);
CREATE INDEX accounts_directory_created_idx ON accounts(created_at DESC, id);
CREATE INDEX accounts_directory_email_idx ON accounts(lower(btrim(email)));
