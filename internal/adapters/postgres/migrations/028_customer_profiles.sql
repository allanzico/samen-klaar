ALTER TABLE accounts ADD COLUMN display_name_custom boolean NOT NULL DEFAULT false;
CREATE TABLE customer_profiles (
 account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 phone text NOT NULL DEFAULT '' CHECK (char_length(phone) <= 30),
 location jsonb,
 updated_at timestamptz NOT NULL DEFAULT now()
);
