-- up
CREATE TABLE IF NOT EXISTS recurring_blocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    weekdays TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    interval_weeks INTEGER NOT NULL DEFAULT 2 CHECK (interval_weeks BETWEEN 1 AND 8),
    anchor_date TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recurring_blocks_user ON recurring_blocks (user_id);

-- down
DROP INDEX IF EXISTS idx_recurring_blocks_user;
DROP TABLE IF EXISTS recurring_blocks;
