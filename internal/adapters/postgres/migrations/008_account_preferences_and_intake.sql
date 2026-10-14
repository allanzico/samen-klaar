ALTER TABLE accounts ADD COLUMN preferred_role text NOT NULL DEFAULT 'customer'
    CHECK (preferred_role IN ('customer', 'professional', 'operator'));

-- One identity can buy services as well as offer them. Keep disabled grants disabled.
INSERT INTO account_memberships (account_id, role, status)
SELECT id, 'customer', 'active' FROM accounts
ON CONFLICT (account_id, role) DO NOTHING;

UPDATE accounts a SET preferred_role = recent.active_role
FROM (
    SELECT DISTINCT ON (s.account_id) s.account_id, s.active_role
    FROM account_sessions s
    JOIN account_memberships m ON m.account_id = s.account_id AND m.role = s.active_role
    WHERE m.status IN ('active', 'pending')
    ORDER BY s.account_id, s.last_seen_at DESC, s.created_at DESC, s.token_hash
) recent WHERE recent.account_id = a.id;

-- Resume previously blocked conversations without losing the customer's answers.
UPDATE account_workspaces
SET state_json = jsonb_set(state_json, '{intake}',
    (state_json->'intake') || jsonb_build_object(
        'step', CASE WHEN state_json #>> '{intake,category}' = 'Unclear' THEN 'detail' ELSE 'matches' END,
        'needsReview', false,
        'question', CASE WHEN state_json #>> '{intake,category}' = 'Unclear'
            THEN 'Where is the problem, and what happens when you notice it?'
            ELSE 'Check available professionals to continue your saved request.' END,
        'options', CASE WHEN state_json #>> '{intake,category}' = 'Unclear'
            THEN '["I will describe more"]'::jsonb ELSE '[]'::jsonb END,
        'messages', COALESCE((SELECT jsonb_agg(message ORDER BY position)
            FROM jsonb_array_elements(CASE WHEN jsonb_typeof(state_json #> '{intake,messages}') = 'array' THEN state_json #> '{intake,messages}' ELSE '[]'::jsonb END) WITH ORDINALITY AS messages(message, position)
            WHERE NOT (message->>'role' = 'assistant' AND message->>'text' ILIKE '%saved for review%')), '[]'::jsonb)
    )), revision = revision + 1, updated_at = now()
WHERE state_json #>> '{intake,step}' = 'review';
