CREATE TABLE paypal_seller_accounts (
    account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    tracking_id text NOT NULL UNIQUE,
    request_id text NOT NULL,
    referral_self_url text NOT NULL DEFAULT '',
    merchant_id text UNIQUE,
    status text NOT NULL DEFAULT 'creating'
        CHECK (status IN ('creating', 'pending', 'verified', 'ineligible', 'revoked')),
    payments_receivable boolean NOT NULL DEFAULT false,
    primary_email_confirmed boolean NOT NULL DEFAULT false,
    permissions_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    checked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX paypal_seller_accounts_status_idx ON paypal_seller_accounts (status, updated_at DESC);

ALTER TABLE paypal_payment_operations
    ADD COLUMN seller_merchant_id text NOT NULL DEFAULT '';
