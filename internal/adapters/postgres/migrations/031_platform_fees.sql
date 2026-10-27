ALTER TABLE paypal_payment_operations
    ADD COLUMN platform_fee_minor bigint NOT NULL DEFAULT 0
    CHECK (platform_fee_minor >= 0 AND platform_fee_minor <= amount_minor);
