ALTER TABLE paypal_payment_operations
    ADD COLUMN refund_request_id text NOT NULL DEFAULT '',
    ADD COLUMN refund_id text;

CREATE TABLE paypal_webhook_events (
    event_id text PRIMARY KEY,
    event_type text NOT NULL,
    event_json jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'processed', 'ignored', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 100),
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);

CREATE INDEX paypal_webhook_events_pending_idx
    ON paypal_webhook_events (status, received_at)
    WHERE status IN ('queued', 'failed');
