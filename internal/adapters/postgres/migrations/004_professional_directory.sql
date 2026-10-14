CREATE TABLE professional_profiles (
    id text PRIMARY KEY,
    account_id text NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    display_name text NOT NULL,
    category text NOT NULL,
    service_name text NOT NULL,
    service_description text NOT NULL DEFAULT '',
    service_area text NOT NULL,
    latitude double precision NOT NULL DEFAULT 0 CHECK (latitude >= -90 AND latitude <= 90),
    longitude double precision NOT NULL DEFAULT 0 CHECK (longitude >= -180 AND longitude <= 180),
    currency char(3) NOT NULL CHECK (currency = upper(currency)),
    price_minor bigint NOT NULL CHECK (price_minor >= 0),
    callout_minor bigint NOT NULL CHECK (callout_minor >= 0),
    duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 15 AND 600),
    materials_policy text NOT NULL,
    exclusions_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL CHECK (status IN ('pending', 'active', 'rejected', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX professional_directory_category_status_idx ON professional_profiles (category, status, service_area);

CREATE TABLE professional_availability_slots (
    id text PRIMARY KEY,
    professional_account_id text NOT NULL REFERENCES professional_profiles(account_id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    timezone text NOT NULL,
    status text NOT NULL CHECK (status IN ('available', 'booked', 'cancelled')) DEFAULT 'available',
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (starts_at < ends_at),
    UNIQUE (professional_account_id, starts_at, ends_at)
);

CREATE INDEX professional_availability_window_idx ON professional_availability_slots (professional_account_id, starts_at) WHERE status = 'available';
