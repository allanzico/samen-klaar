-- name: RecordScopeVersion :exec
INSERT INTO scope_versions (job_id, version, currency, total_minor, lines_json)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (job_id, version) DO NOTHING;
