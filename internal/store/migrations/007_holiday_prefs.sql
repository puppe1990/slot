-- up
CREATE TABLE IF NOT EXISTS holiday_prefs (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    holiday_key TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    PRIMARY KEY (user_id, holiday_key)
);

-- down
DROP TABLE IF EXISTS holiday_prefs;
