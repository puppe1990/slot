package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/puppe1990/slot/internal/models"
)

func (s *SQLiteStore) GetCalendarConnection(userID int64) (models.CalendarConnection, error) {
	var connection models.CalendarConnection
	var lastSynced sql.NullInt64
	var createdAt int64
	err := s.db.QueryRow(`SELECT id, user_id, provider, account_email, sync_status, last_synced_at, created_at
		FROM calendar_connections WHERE user_id = ?`, userID,
	).Scan(&connection.ID, &connection.UserID, &connection.Provider, &connection.AccountEmail,
		&connection.SyncStatus, &lastSynced, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.CalendarConnection{}, fmt.Errorf("%w: user %d", ErrConnectionNotFound, userID)
		}
		return models.CalendarConnection{}, fmt.Errorf("get calendar connection: %w", err)
	}
	if lastSynced.Valid {
		connection.LastSyncedAt = fromUnix(lastSynced.Int64)
	}
	connection.CreatedAt = fromUnix(createdAt)
	return connection, nil
}

func (s *SQLiteStore) SaveCalendarConnection(connection models.CalendarConnection) error {
	_, err := s.db.Exec(`INSERT INTO calendar_connections
		(user_id, provider, account_email, sync_status, last_synced_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			provider = excluded.provider,
			account_email = excluded.account_email,
			sync_status = excluded.sync_status,
			last_synced_at = excluded.last_synced_at`,
		connection.UserID, connection.Provider, connection.AccountEmail,
		connection.SyncStatus, nullableUnix(connection.LastSyncedAt),
	)
	if err != nil {
		return fmt.Errorf("save calendar connection for user %d: %w", connection.UserID, err)
	}
	return nil
}

func nullableUnix(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}
	return ts.UTC().Unix()
}
