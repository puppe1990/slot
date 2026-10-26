package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/puppe1990/slot/internal/models"
)

const meetingTypeColumns = `id, user_id, slug, title, description, duration_minutes,
	buffer_before_minutes, buffer_after_minutes, min_notice_hours, booking_horizon_days,
	location_type, location_details, is_active, created_at, updated_at`

func (s *SQLiteStore) CreateMeetingType(meetingType models.MeetingType) (int64, error) {
	if err := validateMeetingType(meetingType); err != nil {
		return 0, err
	}
	result, err := s.db.Exec(`INSERT INTO meeting_types
		(user_id, slug, title, description, duration_minutes, buffer_before_minutes,
		 buffer_after_minutes, min_notice_hours, booking_horizon_days, location_type,
		 location_details, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		meetingType.UserID, meetingType.Slug, meetingType.Title, meetingType.Description,
		meetingType.DurationMinutes, meetingType.BufferBeforeMinutes, meetingType.BufferAfterMinutes,
		meetingType.MinNoticeHours, meetingType.BookingHorizonDays, meetingType.LocationType,
		meetingType.LocationDetails, boolToInt(meetingType.IsActive),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("%w: %s", ErrSlugTaken, meetingType.Slug)
		}
		return 0, fmt.Errorf("create meeting type %q: %w", meetingType.Slug, err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) UpdateMeetingType(meetingType models.MeetingType) error {
	if err := validateMeetingType(meetingType); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE meeting_types SET
		slug = ?, title = ?, description = ?, duration_minutes = ?, buffer_before_minutes = ?,
		buffer_after_minutes = ?, min_notice_hours = ?, booking_horizon_days = ?,
		location_type = ?, location_details = ?, is_active = ?, updated_at = strftime('%s', 'now')
		WHERE id = ? AND user_id = ?`,
		meetingType.Slug, meetingType.Title, meetingType.Description, meetingType.DurationMinutes,
		meetingType.BufferBeforeMinutes, meetingType.BufferAfterMinutes, meetingType.MinNoticeHours,
		meetingType.BookingHorizonDays, meetingType.LocationType, meetingType.LocationDetails,
		boolToInt(meetingType.IsActive), meetingType.ID, meetingType.UserID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("%w: %s", ErrSlugTaken, meetingType.Slug)
		}
		return fmt.Errorf("update meeting type %d: %w", meetingType.ID, err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return fmt.Errorf("%w: id %d", ErrMeetingTypeNotFound, meetingType.ID)
	}
	return nil
}

func (s *SQLiteStore) GetMeetingType(userID, id int64) (models.MeetingType, error) {
	row := s.db.QueryRow("SELECT "+meetingTypeColumns+" FROM meeting_types WHERE id = ? AND user_id = ?", id, userID)
	return scanMeetingType(row, id)
}

func (s *SQLiteStore) GetMeetingTypeBySlug(slug string) (models.MeetingType, error) {
	row := s.db.QueryRow("SELECT "+meetingTypeColumns+" FROM meeting_types WHERE slug = ?", slug)
	return scanMeetingType(row, 0)
}

func (s *SQLiteStore) ListMeetingTypes(userID int64) ([]models.MeetingType, error) {
	rows, err := s.db.Query("SELECT "+meetingTypeColumns+
		" FROM meeting_types WHERE user_id = ? ORDER BY duration_minutes, id", userID)
	if err != nil {
		return nil, fmt.Errorf("list meeting types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.MeetingType
	for rows.Next() {
		meetingType, err := scanMeetingType(rows, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, meetingType)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetMeetingTypeActive(userID, id int64, active bool) error {
	result, err := s.db.Exec(
		"UPDATE meeting_types SET is_active = ?, updated_at = strftime('%s', 'now') WHERE id = ? AND user_id = ?",
		boolToInt(active), id, userID,
	)
	if err != nil {
		return fmt.Errorf("toggle meeting type %d: %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return fmt.Errorf("%w: id %d", ErrMeetingTypeNotFound, id)
	}
	return nil
}

func scanMeetingType(scanner interface{ Scan(...any) error }, requestedID int64) (models.MeetingType, error) {
	var meetingType models.MeetingType
	var isActive int
	var createdAt, updatedAt int64
	err := scanner.Scan(
		&meetingType.ID, &meetingType.UserID, &meetingType.Slug, &meetingType.Title,
		&meetingType.Description, &meetingType.DurationMinutes, &meetingType.BufferBeforeMinutes,
		&meetingType.BufferAfterMinutes, &meetingType.MinNoticeHours, &meetingType.BookingHorizonDays,
		&meetingType.LocationType, &meetingType.LocationDetails, &isActive, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.MeetingType{}, fmt.Errorf("%w: id %d", ErrMeetingTypeNotFound, requestedID)
		}
		return models.MeetingType{}, fmt.Errorf("scan meeting type: %w", err)
	}
	meetingType.IsActive = isActive == 1
	meetingType.CreatedAt = fromUnix(createdAt)
	meetingType.UpdatedAt = fromUnix(updatedAt)
	return meetingType, nil
}

func validateMeetingType(meetingType models.MeetingType) error {
	if strings.TrimSpace(meetingType.Slug) == "" {
		return errors.New("meeting type slug is required")
	}
	if strings.TrimSpace(meetingType.Title) == "" {
		return errors.New("meeting type title is required")
	}
	if meetingType.UserID == 0 {
		return errors.New("meeting type owner is required")
	}
	if meetingType.DurationMinutes < 5 || meetingType.DurationMinutes > 480 {
		return fmt.Errorf("duration_minutes = %d, want 5-480", meetingType.DurationMinutes)
	}
	if meetingType.BufferBeforeMinutes < 0 || meetingType.BufferBeforeMinutes > 240 ||
		meetingType.BufferAfterMinutes < 0 || meetingType.BufferAfterMinutes > 240 {
		return fmt.Errorf("buffers = %d/%d, want 0-240",
			meetingType.BufferBeforeMinutes, meetingType.BufferAfterMinutes)
	}
	if meetingType.MinNoticeHours < 0 || meetingType.MinNoticeHours > 720 {
		return fmt.Errorf("min_notice_hours = %d, want 0-720", meetingType.MinNoticeHours)
	}
	if meetingType.BookingHorizonDays < 1 || meetingType.BookingHorizonDays > 400 {
		return fmt.Errorf("booking_horizon_days = %d, want 1-400", meetingType.BookingHorizonDays)
	}
	switch meetingType.LocationType {
	case models.LocationGoogleMeet, models.LocationPhone, models.LocationCustom:
	default:
		return fmt.Errorf("location_type = %q, want GOOGLE_MEET, PHONE or CUSTOM", meetingType.LocationType)
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// DefaultMeetingType powers "/" for anonymous visitors: the app is
// single-owner, so the first active format is the right landing.
func (s *SQLiteStore) DefaultMeetingType() (models.MeetingType, error) {
	row := s.db.QueryRow("SELECT " + meetingTypeColumns +
		" FROM meeting_types WHERE is_active = 1 ORDER BY id LIMIT 1")
	return scanMeetingType(row, 0)
}
