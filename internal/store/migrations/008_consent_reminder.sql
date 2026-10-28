-- up
ALTER TABLE bookings ADD COLUMN consent_at TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN reminder_sent_at TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN guest_locale TEXT NOT NULL DEFAULT '';

-- down
ALTER TABLE bookings DROP COLUMN consent_at;
ALTER TABLE bookings DROP COLUMN reminder_sent_at;
ALTER TABLE bookings DROP COLUMN guest_locale;
