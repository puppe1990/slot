package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/devlog"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
	caissqlite "github.com/puppe1990/amarra-cais/pkg/cais/sqlite"
	"github.com/puppe1990/amarra-cais/pkg/cais/sqllog"
	_ "modernc.org/sqlite"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/scheduling"
)

var (
	ErrEmailTaken          = errors.New("email already registered")
	ErrSlugTaken           = errors.New("meeting type slug already used")
	ErrMeetingTypeNotFound = errors.New("meeting type not found")
	ErrConnectionNotFound  = errors.New("calendar connection not found")
	ErrBookingNotFound     = errors.New("booking not found")
	// ErrSlotUnavailable is the 409 the UI must surface: another guest either
	// confirmed or is holding an overlapping interval.
	ErrSlotUnavailable = errors.New("slot unavailable")
	// ErrHoldExpired covers both a consumed and an abandoned hold; the guest
	// page sends them back to pick a fresh time either way.
	ErrHoldExpired = errors.New("hold expired or already used")
)

type Store interface {
	FindUserByEmail(email string) (models.User, error)
	GetUser(id int64) (models.User, error)
	FirstOwner() (models.User, error)
	CreateUser(email, passwordHash string) (int64, error)
	CountUsers() (int, error)
	UpdateOwnerProfile(userID int64, name, timezone string) error
	UpdatePassword(userID int64, passwordHash string) error
	CreatePasswordResetToken(userID int64) (string, error)
	ClearPasswordResetTokens(userID int64) error
	FindPasswordResetUserID(token string) (int64, bool)
	ResetPasswordWithToken(token, passwordHash string) error

	GetCalendarConnection(userID int64) (models.CalendarConnection, error)
	SaveCalendarConnection(connection models.CalendarConnection) error

	CreateMeetingType(meetingType models.MeetingType) (int64, error)
	UpdateMeetingType(meetingType models.MeetingType) error
	GetMeetingType(userID, id int64) (models.MeetingType, error)
	GetMeetingTypeBySlug(slug string) (models.MeetingType, error)
	DefaultMeetingType() (models.MeetingType, error)
	ListMeetingTypes(userID int64) ([]models.MeetingType, error)
	SetMeetingTypeActive(userID, id int64, active bool) error

	ListAvailabilityRules(userID int64) ([]models.AvailabilityRule, error)
	ReplaceAvailabilityRules(userID int64, rules []models.AvailabilityRule) error
	ListDateOverrides(userID int64, fromDate, toDate string) ([]models.DateOverride, error)
	UpsertDateOverride(userID int64, override models.DateOverride) (int64, error)
	DeleteDateOverride(userID, id int64) error

	CreateHold(input HoldInput) (models.Booking, string, error)
	ConfirmHold(holdToken string, guest GuestInput, guestTokenHash string, now time.Time) (models.Booking, error)
	ListActiveIntervals(userID int64, from, to, now time.Time) ([]scheduling.Interval, error)
	ListActiveIntervalsExcluding(userID int64, from, to, now time.Time, excludeBookingID int64) ([]scheduling.Interval, error)
	GetHold(holdToken string, now time.Time) (models.Booking, error)
	ReleaseHold(holdToken string) error
	AttachCalendarEvent(bookingID int64, eventID string) error
	AttachMeetingURL(bookingID int64, meetingURL string) error
	SetAttendance(bookingID int64, attendance string) error
	GetBooking(userID, bookingID int64) (models.Booking, error)
	GetBookingByGuestTokenHash(hash string) (models.Booking, error)
	ListBookings(userID int64, filter BookingFilter) ([]models.Booking, error)
	RescheduleBooking(bookingID int64, start, end time.Time, actor, reason string, now time.Time) error
	CancelBooking(bookingID int64, actor, reason string) error
	PurgeExpiredHolds(now time.Time) (int64, error)

	RecordAudit(entry models.AuditLog) error
	ListAudit(bookingID int64) ([]models.AuditLog, error)

	Sessions() session.Store
	Ping() error
	DB() *sql.DB
	Close() error
}

type SQLiteStore struct {
	db *sqllog.DB
}

func NewSQLiteStore(dsn string, env string) (*SQLiteStore, error) {
	if dsn != ":memory:" {
		dir := filepath.Dir(dsn)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", caissqlite.DSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := caissqlite.Configure(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}

	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	cfg := sqllog.ConfigForEnv(env)
	if cfg.Enabled {
		cfg.Writer = devlog.MirrorDefault(os.Stdout)
	}
	wrapped := sqllog.Wrap(db, cfg)
	if err := seedAuthData(wrapped.Raw(), env); err != nil {
		_ = wrapped.Close()
		return nil, err
	}
	return &SQLiteStore{db: wrapped}, nil
}

func seedAuthData(db *sql.DB, env string) error {
	if env != "development" {
		return nil
	}
	if err := session.EnsureSQLiteSchema(db); err != nil {
		return err
	}
	hash, err := session.HashPassword("password")
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT OR IGNORE INTO users (email, password_hash, is_admin) VALUES (?, ?, 1)", "demo@example.com", hash)
	return err
}

func (s *SQLiteStore) FindUserByEmail(email string) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, name, timezone, password_hash, created_at FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Timezone, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return models.User{}, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}

// FirstOwner is the single account this app serves. The CLI and the MCP
// server act on its behalf.
func (s *SQLiteStore) FirstOwner() (models.User, error) {
	var id int64
	if err := s.db.QueryRow("SELECT id FROM users ORDER BY id LIMIT 1").Scan(&id); err != nil {
		return models.User{}, fmt.Errorf("first owner: %w", err)
	}
	return s.GetUser(id)
}

func (s *SQLiteStore) GetUser(id int64) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, name, timezone, password_hash, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Timezone, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return models.User{}, fmt.Errorf("get user %d: %w", id, err)
	}
	return u, nil
}

func (s *SQLiteStore) CreateUser(email, passwordHash string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, passwordHash,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrEmailTaken
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) UpdateOwnerProfile(userID int64, name, timezone string) error {
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("timezone %q: %w", timezone, err)
	}
	_, err := s.db.Exec(
		"UPDATE users SET name = ?, timezone = ? WHERE id = ?",
		strings.TrimSpace(name), timezone, userID,
	)
	if err != nil {
		return fmt.Errorf("update owner profile %d: %w", userID, err)
	}
	return nil
}

func (s *SQLiteStore) Sessions() session.Store {
	return session.NewSQLiteStore(s.db.Raw())
}

func (s *SQLiteStore) Ping() error {
	return s.db.Raw().Ping()
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db.Raw()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func unixSeconds(ts time.Time) int64 {
	return ts.UTC().Unix()
}

func fromUnix(seconds int64) time.Time {
	return time.Unix(seconds, 0).UTC()
}

// CountUsers answers "is this install claimed yet?" — the owner signup closes
// after the first account so a public deployment cannot be taken over.
func (s *SQLiteStore) CountUsers() (int, error) {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// UpdatePassword replaces the owner's password hash. The caller verifies the
// current password; the store never sees it.
func (s *SQLiteStore) UpdatePassword(userID int64, passwordHash string) error {
	if _, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, userID); err != nil {
		return fmt.Errorf("update password for user %d: %w", userID, err)
	}
	return nil
}
