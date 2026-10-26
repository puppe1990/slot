package store

import (
	"encoding/json"
	"fmt"

	"github.com/puppe1990/amarra-cais/pkg/cais/sqllog"

	"github.com/puppe1990/slot/internal/models"
)

func (s *SQLiteStore) RecordAudit(entry models.AuditLog) error {
	return recordAudit(s.db, entry)
}

func (s *SQLiteStore) ListAudit(bookingID int64) ([]models.AuditLog, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(booking_id, 0), action, actor, payload_json, created_at
		FROM audit_logs WHERE booking_id = ? ORDER BY id`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("list audit for booking %d: %w", bookingID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.AuditLog
	for rows.Next() {
		var entry models.AuditLog
		var createdAt int64
		if err := rows.Scan(&entry.ID, &entry.BookingID, &entry.Action, &entry.Actor, &entry.Payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		entry.CreatedAt = fromUnix(createdAt)
		out = append(out, entry)
	}
	return out, rows.Err()
}

func recordAuditTx(tx *sqllog.Tx, bookingID int64, action, actor string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode audit payload for %s: %w", action, err)
	}
	_, err = tx.Exec(`INSERT INTO audit_logs (booking_id, action, actor, payload_json) VALUES (?, ?, ?, ?)`,
		bookingID, action, actor, string(encoded))
	if err != nil {
		return fmt.Errorf("record audit %s for booking %d: %w", action, bookingID, err)
	}
	return nil
}

func recordAudit(db *sqllog.DB, entry models.AuditLog) error {
	var bookingID any
	if entry.BookingID != 0 {
		bookingID = entry.BookingID
	}
	payload := entry.Payload
	if payload == "" {
		payload = "{}"
	}
	_, err := db.Exec(`INSERT INTO audit_logs (booking_id, action, actor, payload_json) VALUES (?, ?, ?, ?)`,
		bookingID, entry.Action, entry.Actor, payload)
	if err != nil {
		return fmt.Errorf("record audit %s: %w", entry.Action, err)
	}
	return nil
}
