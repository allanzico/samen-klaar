CREATE TABLE consumer_policy_versions (
    kind text NOT NULL CHECK (kind IN ('terms','cancellation','withdrawal','early_start')),
    version text NOT NULL CHECK (length(version) BETWEEN 1 AND 80),
    locale text NOT NULL CHECK (locale IN ('en','nl')),
    title text NOT NULL CHECK (length(title) BETWEEN 3 AND 160),
    body text NOT NULL CHECK (length(body) BETWEEN 10 AND 20000),
    requires_acknowledgment boolean NOT NULL DEFAULT false,
    reviewer_id text NOT NULL REFERENCES accounts(id),
    review_rationale text NOT NULL CHECK (length(review_rationale) BETWEEN 10 AND 1000),
    review_reference text NOT NULL CHECK (length(review_reference) BETWEEN 1 AND 1000),
    published_by text NOT NULL REFERENCES accounts(id),
    published_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind,version)
);
CREATE INDEX consumer_policy_versions_current_idx ON consumer_policy_versions (kind,published_at DESC);

CREATE TABLE consumer_policy_acknowledgments (
    workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
    account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    job_id text NOT NULL,
    scope_version integer NOT NULL CHECK (scope_version > 0),
    policy_kind text NOT NULL,
    policy_version text NOT NULL,
    acknowledged_at timestamptz NOT NULL,
    PRIMARY KEY (workspace_id,job_id,scope_version,policy_kind,policy_version),
    FOREIGN KEY (policy_kind,policy_version) REFERENCES consumer_policy_versions(kind,version)
);
