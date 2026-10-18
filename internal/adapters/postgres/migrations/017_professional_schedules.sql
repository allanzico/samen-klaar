CREATE TABLE professional_schedules (
    professional_account_id text PRIMARY KEY REFERENCES professional_profiles(account_id) ON DELETE CASCADE,
    timezone text NOT NULL DEFAULT 'Europe/Amsterdam',
    gap_minutes integer NOT NULL DEFAULT 30 CHECK (gap_minutes BETWEEN 0 AND 120),
    notice_minutes integer NOT NULL DEFAULT 1440 CHECK (notice_minutes BETWEEN 60 AND 20160),
    horizon_days integer NOT NULL DEFAULT 28 CHECK (horizon_days BETWEEN 7 AND 90),
    materialized_through date,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE professional_working_hours (
    professional_account_id text NOT NULL REFERENCES professional_schedules(professional_account_id) ON DELETE CASCADE,
    weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    starts_local time NOT NULL,
    ends_local time NOT NULL,
    CHECK (starts_local < ends_local),
    PRIMARY KEY (professional_account_id, weekday, starts_local)
);

CREATE TABLE professional_days_off (
    id text PRIMARY KEY,
    professional_account_id text NOT NULL REFERENCES professional_schedules(professional_account_id) ON DELETE CASCADE,
    first_day date NOT NULL,
    last_day date NOT NULL,
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 80),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (first_day <= last_day AND last_day - first_day < 366)
);
CREATE INDEX professional_days_off_window_idx ON professional_days_off (professional_account_id, last_day);

CREATE TABLE professional_slot_exclusions (
    professional_account_id text NOT NULL REFERENCES professional_schedules(professional_account_id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (professional_account_id, starts_at)
);

ALTER TABLE professional_availability_slots
    ADD COLUMN source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'schedule'));
