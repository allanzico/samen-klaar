ALTER TABLE accounts ADD COLUMN locale text NOT NULL DEFAULT 'en'
    CHECK (locale IN ('en', 'nl'));
