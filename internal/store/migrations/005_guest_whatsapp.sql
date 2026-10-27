-- up
ALTER TABLE bookings ADD COLUMN guest_whatsapp TEXT NOT NULL DEFAULT '';

-- down
ALTER TABLE bookings DROP COLUMN guest_whatsapp;
