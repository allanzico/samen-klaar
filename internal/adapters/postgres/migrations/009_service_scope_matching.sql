-- Narrow published scope before joining availability; inactive profiles are excluded.
CREATE INDEX IF NOT EXISTS professional_profiles_active_service_idx
ON professional_profiles (category, service_name) WHERE status = 'active';
