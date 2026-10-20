-- Weekly hours and days off replace unbooked one-off offers and per-time exclusions.
-- Existing reservations and booked visits retain their agreed time.
DELETE FROM professional_slot_exclusions;
DELETE FROM professional_availability_slots a
WHERE a.source = 'manual' AND a.status = 'available'
  AND NOT EXISTS (
    SELECT 1 FROM booking_reservations b
    WHERE b.slot_id = a.id AND b.status = 'booked'
  );
