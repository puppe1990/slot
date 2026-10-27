-- up
ALTER TABLE bookings ADD COLUMN meeting_url TEXT NOT NULL DEFAULT '';

-- down
ALTER TABLE bookings DROP COLUMN meeting_url;
