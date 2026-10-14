CREATE TABLE professional_review_checks (
    professional_profile_id text PRIMARY KEY REFERENCES professional_profiles(id) ON DELETE CASCADE,
    identity_business_status text NOT NULL DEFAULT 'pending' CHECK (identity_business_status IN ('pending', 'verified')),
    paypal_onboarding_status text NOT NULL DEFAULT 'pending' CHECK (paypal_onboarding_status IN ('pending', 'verified')),
    qualification_status text NOT NULL DEFAULT 'pending' CHECK (qualification_status IN ('pending', 'verified', 'not_required')),
    insurance_status text NOT NULL DEFAULT 'pending' CHECK (insurance_status IN ('pending', 'verified', 'not_required')),
    qualification_rationale text NOT NULL DEFAULT '' CHECK (char_length(qualification_rationale) <= 500),
    insurance_rationale text NOT NULL DEFAULT '' CHECK (char_length(insurance_rationale) <= 500),
    updated_by text REFERENCES accounts(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (qualification_status <> 'not_required' OR length(btrim(qualification_rationale)) > 0),
    CHECK (insurance_status <> 'not_required' OR length(btrim(insurance_rationale)) > 0)
);

INSERT INTO professional_review_checks (professional_profile_id)
SELECT id FROM professional_profiles
ON CONFLICT (professional_profile_id) DO NOTHING;
