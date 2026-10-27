-- up
ALTER TABLE bookings ADD COLUMN attendance TEXT NOT NULL DEFAULT '';

-- down
ALTER TABLE bookings DROP COLUMN attendance;
