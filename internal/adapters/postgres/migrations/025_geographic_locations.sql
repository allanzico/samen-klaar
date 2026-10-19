-- Existing profiles retain their published-label coverage until their owner sets a base.
ALTER TABLE professional_profiles ADD COLUMN base_location jsonb;
ALTER TABLE professional_profiles ADD COLUMN travel_radius_km integer NOT NULL DEFAULT 0 CHECK (travel_radius_km BETWEEN 0 AND 100);
ALTER TABLE professional_profiles ADD CONSTRAINT professional_geographic_coverage CHECK (
    (base_location IS NULL AND travel_radius_km = 0) OR
    (base_location IS NOT NULL AND travel_radius_km BETWEEN 1 AND 100)
);
