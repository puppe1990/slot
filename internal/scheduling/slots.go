package scheduling

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// dateLayout is both the override key and the shape the store keeps for a
	// local calendar day.
	dateLayout = "2006-01-02"
	// maxHorizonDays bounds a single computation; a typo in the setting should
	// not spin through decades of empty weeks.
	maxHorizonDays = 400
)

type Interval struct {
	Start time.Time
	End   time.Time
}

// Overlaps is half-open: touching edges (end == start) do not collide, which is
// what lets back-to-back sessions exist when buffers are zero.
func (i Interval) Overlaps(other Interval) bool {
	return i.Start.Before(other.End) && other.Start.Before(i.End)
}

type WeeklyRule struct {
	DayOfWeek int    // 0 = Sunday, matching time.Weekday
	StartTime string // "09:00", owner local wall clock
	EndTime   string // "17:00"
}

type DateOverride struct {
	Date          string // "2026-10-23", owner local date
	IsUnavailable bool
	CustomStart   string
	CustomEnd     string
}

type MeetingType struct {
	DurationMinutes     int
	BufferBeforeMinutes int
	BufferAfterMinutes  int
	MinNoticeHours      int
	BookingHorizonDays  int
}

type Query struct {
	OwnerTZ   *time.Location
	Now       time.Time
	Rules     []WeeklyRule
	Overrides []DateOverride
	Busy      []Interval
	Bookings  []Interval
	Type      MeetingType
}

type Slot struct {
	Start time.Time
	End   time.Time
}

// AvailableSlots returns the bookable starts between the notice cutoff and the
// booking horizon. Every returned edge is UTC; the owner timezone only decides
// which wall-clock windows exist.
func AvailableSlots(q Query) ([]Slot, error) {
	if err := validateQuery(q); err != nil {
		return nil, err
	}

	windows, err := buildWindows(q)
	if err != nil {
		return nil, err
	}

	duration := time.Duration(q.Type.DurationMinutes) * time.Minute
	earliest := q.Now.Add(time.Duration(q.Type.MinNoticeHours) * time.Hour)
	horizonEnd := q.Now.AddDate(0, 0, q.Type.BookingHorizonDays)
	occupied := append(append([]Interval{}, q.Busy...), q.Bookings...)

	seen := make(map[int64]struct{})
	slots := make([]Slot, 0, 64)
	for day := 0; day <= q.Type.BookingHorizonDays; day++ {
		key := q.Now.In(q.OwnerTZ).AddDate(0, 0, day).Format(dateLayout)
		for _, window := range windows[key] {
			slots = append(slots, windowSlots(window, duration, earliest, horizonEnd, occupied, q.Type, seen)...)
		}
	}

	sort.Slice(slots, func(a, b int) bool { return slots[a].Start.Before(slots[b].Start) })
	return slots, nil
}

func windowSlots(window Interval, duration time.Duration, earliest, horizonEnd time.Time, occupied []Interval, meetingType MeetingType, seen map[int64]struct{}) []Slot {
	var slots []Slot
	for start := window.Start; !start.Add(duration).After(window.End); start = start.Add(duration) {
		end := start.Add(duration)
		if start.Before(earliest) || end.After(horizonEnd) {
			continue
		}
		if _, dup := seen[start.Unix()]; dup {
			continue
		}
		block := Interval{
			Start: start.Add(-time.Duration(meetingType.BufferBeforeMinutes) * time.Minute),
			End:   end.Add(time.Duration(meetingType.BufferAfterMinutes) * time.Minute),
		}
		if collides(block, occupied) {
			continue
		}
		seen[start.Unix()] = struct{}{}
		slots = append(slots, Slot{Start: start.UTC(), End: end.UTC()})
	}
	return slots
}

func collides(candidate Interval, occupied []Interval) bool {
	for _, block := range occupied {
		if candidate.Overlaps(block) {
			return true
		}
	}
	return false
}

func buildWindows(q Query) (map[string][]Interval, error) {
	overrides := make(map[string]DateOverride, len(q.Overrides))
	for _, override := range q.Overrides {
		overrides[override.Date] = override
	}

	rulesByDay := make(map[int][]WeeklyRule, 7)
	for _, rule := range q.Rules {
		rulesByDay[rule.DayOfWeek] = append(rulesByDay[rule.DayOfWeek], rule)
	}

	startOfToday := q.Now.In(q.OwnerTZ)
	windows := make(map[string][]Interval, q.Type.BookingHorizonDays+1)
	for day := 0; day <= q.Type.BookingHorizonDays; day++ {
		date := startOfToday.AddDate(0, 0, day)
		key := date.Format(dateLayout)
		dayWindows, err := windowsForDate(date, overrides[key], rulesByDay[int(date.Weekday())], q.OwnerTZ)
		if err != nil {
			return nil, err
		}
		windows[key] = dayWindows
	}
	return windows, nil
}

func windowsForDate(date time.Time, override DateOverride, rules []WeeklyRule, loc *time.Location) ([]Interval, error) {
	if override.Date != "" {
		if override.IsUnavailable {
			return nil, nil
		}
		if override.CustomStart != "" && override.CustomEnd != "" {
			window, err := clockWindow(date, override.CustomStart, override.CustomEnd, loc)
			if err != nil {
				return nil, fmt.Errorf("override %s: %w", override.Date, err)
			}
			return []Interval{window}, nil
		}
	}

	windows := make([]Interval, 0, len(rules))
	for _, rule := range rules {
		window, err := clockWindow(date, rule.StartTime, rule.EndTime, loc)
		if err != nil {
			return nil, fmt.Errorf("weekday %d: %w", rule.DayOfWeek, err)
		}
		if window.End.After(window.Start) {
			windows = append(windows, window)
		}
	}
	return windows, nil
}

// clockWindow anchors both edges to the same local date. time.Date applies the
// zone offset for that instant, so a DST jump between them changes the real
// duration instead of shifting the window.
func clockWindow(date time.Time, startClock, endClock string, loc *time.Location) (Interval, error) {
	startHour, startMinute, err := ParseClock(startClock)
	if err != nil {
		return Interval{}, err
	}
	endHour, endMinute, err := ParseClock(endClock)
	if err != nil {
		return Interval{}, err
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), startHour, startMinute, 0, 0, loc)
	end := time.Date(date.Year(), date.Month(), date.Day(), endHour, endMinute, 0, 0, loc)
	return Interval{Start: start, End: end}, nil
}

func ParseClock(clock string) (hour, minute int, err error) {
	parts := strings.Split(clock, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("clock %q, want HH:MM", clock)
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("clock %q has hour %q, want 00-23", clock, parts[0])
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("clock %q has minute %q, want 00-59", clock, parts[1])
	}
	return hour, minute, nil
}

func validateQuery(q Query) error {
	if q.OwnerTZ == nil {
		return errors.New("scheduling: owner timezone is required")
	}
	if q.Now.IsZero() {
		return errors.New("scheduling: now is required")
	}
	if q.Type.DurationMinutes <= 0 || q.Type.DurationMinutes > 24*60 {
		return fmt.Errorf("scheduling: duration_minutes = %d, want 1-%d", q.Type.DurationMinutes, 24*60)
	}
	if q.Type.BufferBeforeMinutes < 0 || q.Type.BufferAfterMinutes < 0 {
		return fmt.Errorf("scheduling: buffers = %d/%d, want non-negative", q.Type.BufferBeforeMinutes, q.Type.BufferAfterMinutes)
	}
	if q.Type.MinNoticeHours < 0 {
		return fmt.Errorf("scheduling: min_notice_hours = %d, want non-negative", q.Type.MinNoticeHours)
	}
	if q.Type.BookingHorizonDays <= 0 || q.Type.BookingHorizonDays > maxHorizonDays {
		return fmt.Errorf("scheduling: booking_horizon_days = %d, want 1-%d", q.Type.BookingHorizonDays, maxHorizonDays)
	}
	for _, rule := range q.Rules {
		if rule.DayOfWeek < 0 || rule.DayOfWeek > 6 {
			return fmt.Errorf("scheduling: rule day_of_week = %d, want 0-6", rule.DayOfWeek)
		}
	}
	for _, override := range q.Overrides {
		if _, err := time.Parse(dateLayout, override.Date); err != nil {
			return fmt.Errorf("scheduling: override date %q, want YYYY-MM-DD", override.Date)
		}
	}
	return nil
}
