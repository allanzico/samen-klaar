ALTER TABLE paypal_payment_operations
    ADD COLUMN authorization_expires_at timestamptz;
