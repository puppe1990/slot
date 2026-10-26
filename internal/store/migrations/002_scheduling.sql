-- up
ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';

CREATE TABLE IF NOT EXISTS calendar_connections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('google', 'fake')),
    account_email TEXT NOT NULL DEFAULT '',
    sync_status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (sync_status IN ('ACTIVE', 'ERROR', 'REVOKED')),
    last_synced_at INTEGER,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE TABLE IF NOT EXISTS meeting_types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slug TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    duration_minutes INTEGER NOT NULL CHECK (duration_minutes BETWEEN 5 AND 480),
    buffer_before_minutes INTEGER NOT NULL DEFAULT 10 CHECK (buffer_before_minutes BETWEEN 0 AND 240),
    buffer_after_minutes INTEGER NOT NULL DEFAULT 10 CHECK (buffer_after_minutes BETWEEN 0 AND 240),
    min_notice_hours INTEGER NOT NULL DEFAULT 24 CHECK (min_notice_hours BETWEEN 0 AND 720),
    booking_horizon_days INTEGER NOT NULL DEFAULT 30 CHECK (booking_horizon_days BETWEEN 1 AND 400),
    location_type TEXT NOT NULL DEFAULT 'GOOGLE_MEET' CHECK (location_type IN ('GOOGLE_MEET', 'PHONE', 'CUSTOM')),
    location_details TEXT NOT NULL DEFAULT '',
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE TABLE IF NOT EXISTS availability_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day_of_week INTEGER NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    UNIQUE (user_id, day_of_week, start_time)
);

CREATE TABLE IF NOT EXISTS date_overrides (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    is_unavailable INTEGER NOT NULL DEFAULT 1 CHECK (is_unavailable IN (0, 1)),
    custom_start_time TEXT NOT NULL DEFAULT '',
    custom_end_time TEXT NOT NULL DEFAULT '',
    UNIQUE (user_id, date)
);

CREATE TABLE IF NOT EXISTS bookings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    meeting_type_id INTEGER NOT NULL REFERENCES meeting_types(id) ON DELETE CASCADE,
    guest_name TEXT NOT NULL DEFAULT '',
    guest_email TEXT NOT NULL DEFAULT '',
    guest_notes TEXT NOT NULL DEFAULT '',
    guest_timezone TEXT NOT NULL DEFAULT 'UTC',
    start_time_utc INTEGER NOT NULL,
    end_time_utc INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('HOLD', 'CONFIRMED', 'RESCHEDULED', 'CANCELLED')),
    hold_expires_at INTEGER,
    hold_token_hash TEXT UNIQUE,
    guest_token_hash TEXT UNIQUE,
    cancellation_reason TEXT NOT NULL DEFAULT '',
    calendar_event_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    CHECK (end_time_utc > start_time_utc)
);

-- Second line of defence for the booking race: even if two requests pass the
-- overlap check, only one active row may own an exact start. Expired holds are
-- purged inside the creating transaction so they never trip this index.
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_active_start
    ON bookings (meeting_type_id, start_time_utc)
    WHERE status IN ('HOLD', 'CONFIRMED', 'RESCHEDULED');

CREATE INDEX IF NOT EXISTS idx_bookings_meeting_type_range
    ON bookings (meeting_type_id, start_time_utc, end_time_utc);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    booking_id INTEGER REFERENCES bookings(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    actor TEXT NOT NULL CHECK (actor IN ('OWNER', 'GUEST', 'SYSTEM')),
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- down
DROP TABLE IF EXISTS audit_logs;
DROP INDEX IF EXISTS idx_bookings_meeting_type_range;
DROP INDEX IF EXISTS idx_bookings_active_start;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS date_overrides;
DROP TABLE IF EXISTS availability_rules;
DROP TABLE IF EXISTS meeting_types;
DROP TABLE IF EXISTS calendar_connections;
ALTER TABLE users DROP COLUMN timezone;
ALTER TABLE users DROP COLUMN name;
