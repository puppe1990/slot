package store

import (
	"fmt"
	"strings"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/scheduling"
)

func (s *SQLiteStore) ListAvailabilityRules(userID int64) ([]models.AvailabilityRule, error) {
	rows, err := s.db.Query(`SELECT id, user_id, day_of_week, start_time, end_time, is_active
		FROM availability_rules WHERE user_id = ? ORDER BY day_of_week, start_time`, userID)
	if err != nil {
		return nil, fmt.Errorf("list availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.AvailabilityRule
	for rows.Next() {
		var rule models.AvailabilityRule
		var isActive int
		if err := rows.Scan(&rule.ID, &rule.UserID, &rule.DayOfWeek, &rule.StartTime, &rule.EndTime, &isActive); err != nil {
			return nil, fmt.Errorf("scan availability: %w", err)
		}
		rule.IsActive = isActive == 1
		out = append(out, rule)
	}
	return out, rows.Err()
}

// ReplaceAvailabilityRules swaps the whole weekly grid in one transaction: the
// editor submits the week as a unit, so a partial write would leave torn hours.
func (s *SQLiteStore) ReplaceAvailabilityRules(userID int64, rules []models.AvailabilityRule) error {
	if err := validateRules(rules); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin availability swap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM availability_rules WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("clear availability: %w", err)
	}
	for _, rule := range rules {
		_, err := tx.Exec(`INSERT INTO availability_rules
			(user_id, day_of_week, start_time, end_time, is_active) VALUES (?, ?, ?, ?, ?)`,
			userID, rule.DayOfWeek, rule.StartTime, rule.EndTime, boolToInt(rule.IsActive),
		)
		if err != nil {
			return fmt.Errorf("insert availability day %d: %w", rule.DayOfWeek, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListDateOverrides(userID int64, fromDate, toDate string) ([]models.DateOverride, error) {
	rows, err := s.db.Query(`SELECT id, user_id, date, is_unavailable, custom_start_time, custom_end_time
		FROM date_overrides WHERE user_id = ? AND date BETWEEN ? AND ? ORDER BY date`,
		userID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("list date overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.DateOverride
	for rows.Next() {
		var override models.DateOverride
		var unavailable int
		if err := rows.Scan(&override.ID, &override.UserID, &override.Date, &unavailable,
			&override.CustomStartTime, &override.CustomEndTime); err != nil {
			return nil, fmt.Errorf("scan date override: %w", err)
		}
		override.IsUnavailable = unavailable == 1
		out = append(out, override)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpsertDateOverride(userID int64, override models.DateOverride) (int64, error) {
	if override.CustomStartTime != "" {
		if _, _, err := scheduling.ParseClock(override.CustomStartTime); err != nil {
			return 0, fmt.Errorf("override %s start: %w", override.Date, err)
		}
	}
	if override.CustomEndTime != "" {
		if _, _, err := scheduling.ParseClock(override.CustomEndTime); err != nil {
			return 0, fmt.Errorf("override %s end: %w", override.Date, err)
		}
	}
	_, err := s.db.Exec(`INSERT INTO date_overrides
		(user_id, date, is_unavailable, custom_start_time, custom_end_time) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, date) DO UPDATE SET
			is_unavailable = excluded.is_unavailable,
			custom_start_time = excluded.custom_start_time,
			custom_end_time = excluded.custom_end_time`,
		userID, override.Date, boolToInt(override.IsUnavailable),
		override.CustomStartTime, override.CustomEndTime,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert date override %s: %w", override.Date, err)
	}

	var id int64
	if err := s.db.QueryRow("SELECT id FROM date_overrides WHERE user_id = ? AND date = ?",
		userID, override.Date).Scan(&id); err != nil {
		return 0, fmt.Errorf("read back override %s: %w", override.Date, err)
	}
	return id, nil
}

func (s *SQLiteStore) DeleteDateOverride(userID, id int64) error {
	_, err := s.db.Exec("DELETE FROM date_overrides WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return fmt.Errorf("delete date override %d: %w", id, err)
	}
	return nil
}

func validateRules(rules []models.AvailabilityRule) error {
	for _, rule := range rules {
		if rule.DayOfWeek < 0 || rule.DayOfWeek > 6 {
			return fmt.Errorf("rule day_of_week = %d, want 0-6", rule.DayOfWeek)
		}
		startHour, startMinute, err := scheduling.ParseClock(rule.StartTime)
		if err != nil {
			return err
		}
		endHour, endMinute, err := scheduling.ParseClock(rule.EndTime)
		if err != nil {
			return err
		}
		if startHour*60+startMinute >= endHour*60+endMinute {
			return fmt.Errorf("rule %s-%s on day %d, want start before end",
				strings.TrimSpace(rule.StartTime), strings.TrimSpace(rule.EndTime), rule.DayOfWeek)
		}
	}
	return nil
}
