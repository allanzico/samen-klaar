CREATE TABLE parent_requests (
 id text PRIMARY KEY, account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 description text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO parent_requests(id,account_id,description) SELECT id,account_id,COALESCE(state_json#>>'{intake,description}','') FROM account_workspaces;
UPDATE account_workspaces SET state_json=jsonb_set(state_json,'{parentRequestId}',to_jsonb(id));
CREATE FUNCTION assign_parent_request() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state_json->>'parentRequestId' IS NULL THEN
   NEW.state_json=jsonb_set(NEW.state_json,'{parentRequestId}',to_jsonb(NEW.id));
 END IF;
 INSERT INTO parent_requests(id,account_id,description) VALUES(NEW.state_json->>'parentRequestId',NEW.account_id,COALESCE(NEW.state_json#>>'{intake,description}','')) ON CONFLICT(id) DO NOTHING;
 IF NOT EXISTS(SELECT 1 FROM parent_requests WHERE id=NEW.state_json->>'parentRequestId' AND account_id=NEW.account_id) THEN RAISE EXCEPTION 'parent request does not belong to customer'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER account_workspace_parent BEFORE INSERT ON account_workspaces FOR EACH ROW EXECUTE FUNCTION assign_parent_request();
CREATE INDEX workspace_parent_request ON account_workspaces((state_json->>'parentRequestId'));
CREATE TABLE request_job_plans (
 parent_request_id text NOT NULL REFERENCES parent_requests(id) ON DELETE CASCADE,
 plan_id text NOT NULL, workspace_id text NOT NULL REFERENCES account_workspaces(id) ON DELETE CASCADE,
 description text NOT NULL, PRIMARY KEY(parent_request_id,plan_id)
);
