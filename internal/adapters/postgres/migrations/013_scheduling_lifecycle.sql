-- Lifecycle workers filter and order workspace JSON by state and updated_at.
-- Keep the scan ordered without casting user state into timestamptz index
-- expressions: PostgreSQL correctly treats those casts as non-immutable.
CREATE INDEX account_workspaces_lifecycle_scan_idx
    ON account_workspaces (updated_at)
    WHERE COALESCE((state_json ->> 'cancelled')::boolean, false) = false;
