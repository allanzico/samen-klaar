ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_status_check;

ALTER TABLE notification_outbox
    ADD CONSTRAINT notification_outbox_status_check
        CHECK (status IN ('queued', 'sending', 'adapter_unconfigured', 'sent', 'failed')),
    ADD COLUMN attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    ADD COLUMN last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 100);

CREATE INDEX notification_outbox_pending_idx
    ON notification_outbox (status, created_at)
    WHERE status IN ('queued', 'failed', 'adapter_unconfigured');
