package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/scheduling"
)

// defaultHoldTTL is how long a guest may sit on the confirm form before the
// slot returns to the pool.
const defaultHoldTTL = 10 * time.Minute

const bookingSelect = `SELECT b.id, b.meeting_type_id, b.guest_name, b.guest_email, b.guest_notes,
		b.guest_timezone, b.start_time_utc, b.end_time_utc, b.status, b.hold_expires_at,
		b.cancellation_reason, b.calendar_event_id, b.created_at, b.updated_at,
		mt.slug, mt.title, mt.duration_minutes, mt.location_type,
		u.name, u.email, u.timezone
	FROM bookings b
	JOIN meeting_types mt ON mt.id = b.meeting_type_id
	JOIN users u ON u.id = mt.user_id`

type HoldInput struct {
	MeetingTypeID int64
	Start         time.Time
	End           time.Time
	GuestEmail    string
	GuestTimezone string
	ExpiresAt     time.Time
	Now           time.Time
}

type GuestInput struct {
	Name     string
	Email    string
	Notes    string
	Timezone string
}

type BookingFilter struct {
	Status models.BookingStatus
	From   time.Time
	To     time.Time
	Limit  int
}

type meetingTypeAux struct {
	userID              int64
	bufferBeforeMinutes int
	bufferAfterMinutes  int
}

type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// CreateHold reserves an interval for ten minutes. The overlap check and the
// insert share one transaction, and SQLite runs with a single writer
// connection, so two guests cannot both win the same slot.
func (s *SQLiteStore) CreateHold(input HoldInput) (models.Booking, string, error) {
	if !input.End.After(input.Start) {
		return models.Booking{}, "", fmt.Errorf("hold %s-%s, want end after start", input.Start, input.End)
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}
	if input.ExpiresAt.IsZero() {
		input.ExpiresAt = input.Now.Add(defaultHoldTTL)
	}
	if !input.ExpiresAt.After(input.Now) {
		return models.Booking{}, "", fmt.Errorf("hold expiry %s is not after %s", input.ExpiresAt, input.Now)
	}

	aux, err := s.meetingTypeAux(s.db, input.MeetingTypeID)
	if err != nil {
		return models.Booking{}, "", err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return models.Booking{}, "", fmt.Errorf("begin hold: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Expired holds must leave before the unique start index sees them.
	if _, err := tx.Exec("DELETE FROM bookings WHERE status = 'HOLD' AND hold_expires_at <= ?",
		unixSeconds(input.Now)); err != nil {
		return models.Booking{}, "", fmt.Errorf("purge expired holds: %w", err)
	}

	blocked, err := blockedIntervals(tx, aux.userID, input.Now, 0)
	if err != nil {
		return models.Booking{}, "", err
	}
	if collides(input.Start, input.End, aux, blocked) {
		return models.Booking{}, "", fmt.Errorf("%w: %s-%s", ErrSlotUnavailable, input.Start, input.End)
	}

	holdToken := guesttoken.RandomToken()
	result, err := tx.Exec(`INSERT INTO bookings
		(meeting_type_id, guest_email, guest_timezone, start_time_utc, end_time_utc, status,
		 hold_expires_at, hold_token_hash)
		VALUES (?, ?, ?, ?, ?, 'HOLD', ?, ?)`,
		input.MeetingTypeID, strings.TrimSpace(input.GuestEmail), input.GuestTimezone,
		unixSeconds(input.Start), unixSeconds(input.End), unixSeconds(input.ExpiresAt),
		guesttoken.Hash(holdToken),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return models.Booking{}, "", fmt.Errorf("%w: %s", ErrSlotUnavailable, input.Start)
		}
		return models.Booking{}, "", fmt.Errorf("create hold: %w", err)
	}
	bookingID, err := result.LastInsertId()
	if err != nil {
		return models.Booking{}, "", fmt.Errorf("hold id: %w", err)
	}

	if err := recordAuditTx(tx, bookingID, "HOLD_CREATED", models.ActorGuest, map[string]any{
		"start": input.Start.UTC().Format(time.RFC3339),
		"end":   input.End.UTC().Format(time.RFC3339),
		"email": strings.TrimSpace(input.GuestEmail),
	}); err != nil {
		return models.Booking{}, "", err
	}

	booking, err := bookingByID(tx, bookingID)
	if err != nil {
		return models.Booking{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return models.Booking{}, "", fmt.Errorf("commit hold: %w", err)
	}
	return booking, holdToken, nil
}

func (s *SQLiteStore) ConfirmHold(holdToken string, guest GuestInput, guestTokenHash string, now time.Time) (models.Booking, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return models.Booking{}, fmt.Errorf("begin confirm: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var bookingID, meetingTypeID, startUnix, endUnix, expiresUnix int64
	err = tx.QueryRow(`SELECT id, meeting_type_id, start_time_utc, end_time_utc, COALESCE(hold_expires_at, 0)
		FROM bookings WHERE hold_token_hash = ? AND status = 'HOLD'`, guesttoken.Hash(holdToken),
	).Scan(&bookingID, &meetingTypeID, &startUnix, &endUnix, &expiresUnix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Booking{}, fmt.Errorf("%w: token", ErrHoldExpired)
		}
		return models.Booking{}, fmt.Errorf("lookup hold: %w", err)
	}
	if expiresUnix <= unixSeconds(now) {
		return models.Booking{}, fmt.Errorf("%w: expired at %s", ErrHoldExpired, fromUnix(expiresUnix))
	}

	aux, err := s.meetingTypeAux(tx, meetingTypeID)
	if err != nil {
		return models.Booking{}, err
	}
	blocked, err := blockedIntervals(tx, aux.userID, now, bookingID)
	if err != nil {
		return models.Booking{}, err
	}
	if collides(fromUnix(startUnix), fromUnix(endUnix), aux, blocked) {
		return models.Booking{}, fmt.Errorf("%w: %s", ErrSlotUnavailable, fromUnix(startUnix))
	}

	_, err = tx.Exec(`UPDATE bookings SET status = 'CONFIRMED', guest_name = ?, guest_email = ?,
		guest_notes = ?, guest_timezone = ?, guest_token_hash = ?, hold_token_hash = NULL,
		hold_expires_at = NULL, updated_at = strftime('%s', 'now')
		WHERE id = ?`,
		strings.TrimSpace(guest.Name), strings.TrimSpace(guest.Email), strings.TrimSpace(guest.Notes),
		guest.Timezone, guestTokenHash, bookingID,
	)
	if err != nil {
		return models.Booking{}, fmt.Errorf("confirm hold %d: %w", bookingID, err)
	}
	if err := recordAuditTx(tx, bookingID, "BOOKING_CONFIRMED", models.ActorGuest, map[string]any{
		"email": strings.TrimSpace(guest.Email),
	}); err != nil {
		return models.Booking{}, err
	}

	booking, err := bookingByID(tx, bookingID)
	if err != nil {
		return models.Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Booking{}, fmt.Errorf("commit confirm: %w", err)
	}
	return booking, nil
}

// ListActiveIntervals returns confirmed sessions plus unexpired holds,
// expanded by each booking's own buffers, so the read path and the write path
// agree on what "free" means.
func (s *SQLiteStore) ListActiveIntervals(userID int64, from, to, now time.Time) ([]scheduling.Interval, error) {
	return s.ListActiveIntervalsExcluding(userID, from, to, now, 0)
}

// ListActiveIntervalsExcluding drops one booking from the picture. The manage
// page uses it so a guest rescheduling does not see their own session as busy.
func (s *SQLiteStore) ListActiveIntervalsExcluding(userID int64, from, to, now time.Time, excludeBookingID int64) ([]scheduling.Interval, error) {
	blocks, err := blockedIntervalsInRange(s.db, userID, now, excludeBookingID, from.Add(-4*time.Hour), to.Add(4*time.Hour))
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

// GetHold returns the live hold behind a confirm page. An expired or consumed
// token is ErrHoldExpired so the guest restarts from the calendar.
func (s *SQLiteStore) GetHold(holdToken string, now time.Time) (models.Booking, error) {
	row := s.db.QueryRow(bookingSelect+" WHERE b.hold_token_hash = ? AND b.status = 'HOLD'", guesttoken.Hash(holdToken))
	booking, err := scanBooking(row, 0)
	if err != nil {
		return models.Booking{}, fmt.Errorf("%w: token", ErrHoldExpired)
	}
	if !booking.HoldExpiresAt.After(now) {
		return models.Booking{}, fmt.Errorf("%w: expired at %s", ErrHoldExpired, booking.HoldExpiresAt)
	}
	return booking, nil
}

// ReleaseHold deletes an abandoned hold so the slot returns immediately
// instead of waiting out the TTL. Idempotent: a consumed or already released
// token is not an error.
func (s *SQLiteStore) ReleaseHold(holdToken string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin release: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var bookingID int64
	err = tx.QueryRow("SELECT id FROM bookings WHERE hold_token_hash = ? AND status = 'HOLD'",
		guesttoken.Hash(holdToken)).Scan(&bookingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load hold: %w", err)
	}
	if err := recordAuditTx(tx, bookingID, "HOLD_RELEASED", models.ActorGuest, map[string]any{}); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM bookings WHERE id = ?", bookingID); err != nil {
		return fmt.Errorf("release hold %d: %w", bookingID, err)
	}
	return tx.Commit()
}

func (s *SQLiteStore) AttachCalendarEvent(bookingID int64, eventID string) error {
	_, err := s.db.Exec("UPDATE bookings SET calendar_event_id = ?, updated_at = strftime('%s', 'now') WHERE id = ?",
		eventID, bookingID)
	if err != nil {
		return fmt.Errorf("attach calendar event to booking %d: %w", bookingID, err)
	}
	return nil
}

func (s *SQLiteStore) GetBooking(userID, bookingID int64) (models.Booking, error) {
	row := s.db.QueryRow(bookingSelect+" WHERE b.id = ? AND mt.user_id = ?", bookingID, userID)
	return scanBooking(row, bookingID)
}

func (s *SQLiteStore) GetBookingByGuestTokenHash(hash string) (models.Booking, error) {
	row := s.db.QueryRow(bookingSelect+" WHERE b.guest_token_hash = ?", hash)
	return scanBooking(row, 0)
}

func (s *SQLiteStore) ListBookings(userID int64, filter BookingFilter) ([]models.Booking, error) {
	query := bookingSelect + " WHERE mt.user_id = ?"
	args := []any{userID}

	if filter.Status != "" {
		query += " AND b.status = ?"
		args = append(args, string(filter.Status))
	} else {
		query += " AND b.status <> 'HOLD'"
	}
	if !filter.From.IsZero() {
		query += " AND b.end_time_utc > ?"
		args = append(args, unixSeconds(filter.From))
	}
	if !filter.To.IsZero() {
		query += " AND b.start_time_utc < ?"
		args = append(args, unixSeconds(filter.To))
	}
	query += " ORDER BY b.start_time_utc"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.Booking
	for rows.Next() {
		booking, err := scanBooking(rows, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, booking)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RescheduleBooking(bookingID int64, start, end time.Time, actor, reason string, now time.Time) error {
	if !end.After(start) {
		return fmt.Errorf("reschedule %s-%s, want end after start", start, end)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reschedule: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var meetingTypeID int64
	if err := tx.QueryRow("SELECT meeting_type_id FROM bookings WHERE id = ?", bookingID).Scan(&meetingTypeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: id %d", ErrBookingNotFound, bookingID)
		}
		return fmt.Errorf("load booking %d: %w", bookingID, err)
	}
	aux, err := s.meetingTypeAux(tx, meetingTypeID)
	if err != nil {
		return err
	}
	blocked, err := blockedIntervals(tx, aux.userID, now, bookingID)
	if err != nil {
		return err
	}
	if collides(start, end, aux, blocked) {
		return fmt.Errorf("%w: %s", ErrSlotUnavailable, start)
	}

	_, err = tx.Exec(`UPDATE bookings SET start_time_utc = ?, end_time_utc = ?,
		status = 'RESCHEDULED', updated_at = strftime('%s', 'now') WHERE id = ?`,
		unixSeconds(start), unixSeconds(end), bookingID)
	if err != nil {
		return fmt.Errorf("reschedule booking %d: %w", bookingID, err)
	}
	if err := recordAuditTx(tx, bookingID, "BOOKING_RESCHEDULED", actor, map[string]any{
		"start":  start.UTC().Format(time.RFC3339),
		"end":    end.UTC().Format(time.RFC3339),
		"reason": reason,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) CancelBooking(bookingID int64, actor, reason string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin cancel: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`UPDATE bookings SET status = 'CANCELLED', cancellation_reason = ?,
		hold_token_hash = NULL, hold_expires_at = NULL, updated_at = strftime('%s', 'now')
		WHERE id = ? AND status <> 'CANCELLED'`, strings.TrimSpace(reason), bookingID)
	if err != nil {
		return fmt.Errorf("cancel booking %d: %w", bookingID, err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return fmt.Errorf("%w: id %d", ErrBookingNotFound, bookingID)
	}
	if err := recordAuditTx(tx, bookingID, "BOOKING_CANCELLED", actor, map[string]any{
		"reason": strings.TrimSpace(reason),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) PurgeExpiredHolds(now time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM bookings WHERE status = 'HOLD' AND hold_expires_at <= ?",
		unixSeconds(now))
	if err != nil {
		return 0, fmt.Errorf("purge expired holds: %w", err)
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) meetingTypeAux(queryer rowQuerier, meetingTypeID int64) (meetingTypeAux, error) {
	var aux meetingTypeAux
	err := queryer.QueryRow(`SELECT user_id, buffer_before_minutes, buffer_after_minutes
		FROM meeting_types WHERE id = ?`, meetingTypeID,
	).Scan(&aux.userID, &aux.bufferBeforeMinutes, &aux.bufferAfterMinutes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return meetingTypeAux{}, fmt.Errorf("%w: id %d", ErrMeetingTypeNotFound, meetingTypeID)
		}
		return meetingTypeAux{}, fmt.Errorf("load meeting type %d: %w", meetingTypeID, err)
	}
	return aux, nil
}

func blockedIntervals(queryer rowQuerier, userID int64, now time.Time, excludeBookingID int64) ([]scheduling.Interval, error) {
	return blockedIntervalsInRange(queryer, userID, now, excludeBookingID, time.Time{}, time.Time{})
}

func blockedIntervalsInRange(queryer rowQuerier, userID int64, now time.Time, excludeBookingID int64, from, to time.Time) ([]scheduling.Interval, error) {
	query := `SELECT b.start_time_utc, b.end_time_utc, mt.buffer_before_minutes, mt.buffer_after_minutes
		FROM bookings b JOIN meeting_types mt ON mt.id = b.meeting_type_id
		WHERE mt.user_id = ?
		  AND b.id <> ?
		  AND (b.status IN ('CONFIRMED', 'RESCHEDULED')
		       OR (b.status = 'HOLD' AND b.hold_expires_at > ?))`
	args := []any{userID, excludeBookingID, unixSeconds(now)}
	if !from.IsZero() {
		query += " AND b.end_time_utc > ?"
		args = append(args, unixSeconds(from))
	}
	if !to.IsZero() {
		query += " AND b.start_time_utc < ?"
		args = append(args, unixSeconds(to))
	}

	rows, err := queryer.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("load blocked intervals for user %d: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []scheduling.Interval
	for rows.Next() {
		var startUnix, endUnix int64
		var before, after int
		if err := rows.Scan(&startUnix, &endUnix, &before, &after); err != nil {
			return nil, fmt.Errorf("scan blocked interval: %w", err)
		}
		out = append(out, scheduling.Interval{
			Start: fromUnix(startUnix).Add(-time.Duration(before) * time.Minute),
			End:   fromUnix(endUnix).Add(time.Duration(after) * time.Minute),
		})
	}
	return out, rows.Err()
}

func collides(start, end time.Time, aux meetingTypeAux, blocked []scheduling.Interval) bool {
	candidate := scheduling.Interval{
		Start: start.Add(-time.Duration(aux.bufferBeforeMinutes) * time.Minute),
		End:   end.Add(time.Duration(aux.bufferAfterMinutes) * time.Minute),
	}
	for _, block := range blocked {
		if candidate.Overlaps(block) {
			return true
		}
	}
	return false
}

func bookingByID(queryer rowQuerier, bookingID int64) (models.Booking, error) {
	row := queryer.QueryRow(bookingSelect+" WHERE b.id = ?", bookingID)
	return scanBooking(row, bookingID)
}

func scanBooking(scanner interface{ Scan(...any) error }, requestedID int64) (models.Booking, error) {
	var booking models.Booking
	var status string
	var holdExpires sql.NullInt64
	var startUnix, endUnix, createdAt, updatedAt int64
	err := scanner.Scan(
		&booking.ID, &booking.MeetingTypeID, &booking.GuestName, &booking.GuestEmail,
		&booking.GuestNotes, &booking.GuestTimezone, &startUnix, &endUnix, &status, &holdExpires,
		&booking.CancellationReason, &booking.CalendarEventID, &createdAt, &updatedAt,
		&booking.MeetingTypeSlug, &booking.MeetingTypeTitle, &booking.MeetingTypeDuration,
		&booking.MeetingTypeLocation, &booking.OwnerName, &booking.OwnerEmail, &booking.OwnerTimezone,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Booking{}, fmt.Errorf("%w: id %d", ErrBookingNotFound, requestedID)
		}
		return models.Booking{}, fmt.Errorf("scan booking: %w", err)
	}
	booking.Status = models.BookingStatus(status)
	booking.StartUTC = fromUnix(startUnix)
	booking.EndUTC = fromUnix(endUnix)
	if holdExpires.Valid {
		booking.HoldExpiresAt = fromUnix(holdExpires.Int64)
	}
	booking.CreatedAt = fromUnix(createdAt)
	booking.UpdatedAt = fromUnix(updatedAt)
	return booking, nil
}
