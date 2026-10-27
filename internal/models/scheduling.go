package models

import (
	"strconv"
	"strings"
	"time"
)

type MeetingType struct {
	ID                  int64
	UserID              int64
	Slug                string
	Title               string
	Description         string
	DurationMinutes     int
	BufferBeforeMinutes int
	BufferAfterMinutes  int
	MinNoticeHours      int
	BookingHorizonDays  int
	LocationType        string
	LocationDetails     string
	IsActive            bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

const (
	LocationGoogleMeet = "GOOGLE_MEET"
	LocationPhone      = "PHONE"
	LocationCustom     = "CUSTOM"
)

type AvailabilityRule struct {
	ID        int64
	UserID    int64
	DayOfWeek int
	StartTime string
	EndTime   string
	IsActive  bool
}

type DateOverride struct {
	ID              int64
	UserID          int64
	Date            string
	IsUnavailable   bool
	CustomStartTime string
	CustomEndTime   string
}

// RecurringBlock closes a wall-clock window on the chosen weekdays, repeating
// every IntervalWeeks from the week of AnchorDate. It is how a two-week rota
// ("this week mornings, next week nights") stops being a manual chore.
type RecurringBlock struct {
	ID            int64
	UserID        int64
	Title         string
	Weekdays      []int
	StartTime     string
	EndTime       string
	IntervalWeeks int
	AnchorDate    string
}

// WeekdayCSV is the storage form: "1,2,3,4,5".
func (b RecurringBlock) WeekdayCSV() string {
	parts := make([]string, 0, len(b.Weekdays))
	for _, day := range b.Weekdays {
		parts = append(parts, strconv.Itoa(day))
	}
	return strings.Join(parts, ",")
}

// ParseWeekdays reads the storage form, dropping anything outside 0..6.
func ParseWeekdays(csv string) []int {
	days := make([]int, 0, 7)
	seen := map[int]struct{}{}
	for _, part := range strings.Split(csv, ",") {
		day, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || day < 0 || day > 6 {
			continue
		}
		if _, dup := seen[day]; dup {
			continue
		}
		seen[day] = struct{}{}
		days = append(days, day)
	}
	return days
}

type BookingStatus string

const (
	BookingStatusHold        BookingStatus = "HOLD"
	BookingStatusConfirmed   BookingStatus = "CONFIRMED"
	BookingStatusRescheduled BookingStatus = "RESCHEDULED"
	BookingStatusCancelled   BookingStatus = "CANCELLED"
)

type Booking struct {
	ID                 int64
	MeetingTypeID      int64
	GuestName          string
	GuestEmail         string
	GuestWhatsApp      string
	GuestNotes         string
	GuestTimezone      string
	StartUTC           time.Time
	EndUTC             time.Time
	Status             BookingStatus
	HoldExpiresAt      time.Time
	CancellationReason string
	CalendarEventID    string
	MeetingURL         string
	Attendance         string
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Joined columns used by the owner dashboard and the guest manage page.
	MeetingTypeSlug     string
	MeetingTypeTitle    string
	MeetingTypeDuration int
	MeetingTypeLocation string
	OwnerName           string
	OwnerEmail          string
	OwnerTimezone       string
}

// AttendanceGoing is the guest's RSVP. Empty means "did not answer yet".
const AttendanceGoing = "GOING"

func (b Booking) Active() bool {
	return b.Status == BookingStatusConfirmed || b.Status == BookingStatusRescheduled
}

type CalendarConnection struct {
	ID           int64
	UserID       int64
	Provider     string
	AccountEmail string
	SyncStatus   string
	LastSyncedAt time.Time
	CreatedAt    time.Time
}

const (
	CalendarProviderGoogle = "google"
	CalendarProviderFake   = "fake"
)

type AuditLog struct {
	ID        int64
	BookingID int64
	Action    string
	Actor     string
	Payload   string
	CreatedAt time.Time
}

const (
	ActorOwner  = "OWNER"
	ActorGuest  = "GUEST"
	ActorSystem = "SYSTEM"
)
