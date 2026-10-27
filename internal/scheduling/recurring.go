package scheduling

import (
	"fmt"
	"time"
)

// RecurringBlock describes a window that closes on the listed weekdays, every
// IntervalWeeks weeks, counted from the week of AnchorDate. The clock is the
// owner's wall clock, so a DST change moves the block with the wall.
type RecurringBlock struct {
	Weekdays      []int
	StartTime     string
	EndTime       string
	IntervalWeeks int
	AnchorDate    string
}

// ExpandRecurringBlocks turns the patterns into concrete busy intervals inside
// [from, to]. Weeks are counted from Monday of the anchor week, so "every 2
// weeks" stays aligned no matter which weekday the owner picked.
func ExpandRecurringBlocks(from, to time.Time, loc *time.Location, blocks []RecurringBlock) ([]Interval, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	first := dateOnly(from.In(loc), loc)
	last := dateOnly(to.In(loc), loc)
	out := make([]Interval, 0, len(blocks)*4)

	for _, block := range blocks {
		anchor, err := time.ParseInLocation(dateLayout, block.AnchorDate, loc)
		if err != nil {
			return nil, fmt.Errorf("recurring block anchor %q, want YYYY-MM-DD: %w", block.AnchorDate, err)
		}
		startHour, startMinute, err := ParseClock(block.StartTime)
		if err != nil {
			return nil, fmt.Errorf("recurring block start: %w", err)
		}
		endHour, endMinute, err := ParseClock(block.EndTime)
		if err != nil {
			return nil, fmt.Errorf("recurring block end: %w", err)
		}
		if endHour*60+endMinute <= startHour*60+startMinute {
			return nil, fmt.Errorf("recurring block end %s must be after start %s", block.EndTime, block.StartTime)
		}
		interval := block.IntervalWeeks
		if interval < 1 {
			interval = 1
		}
		anchorWeek := mondayOf(anchor)

		for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
			if !containsWeekday(block.Weekdays, int(day.Weekday())) {
				continue
			}
			weeks := daysBetween(anchorWeek, mondayOf(day)) / 7
			if weeks < 0 || weeks%interval != 0 {
				continue
			}
			start := time.Date(day.Year(), day.Month(), day.Day(), startHour, startMinute, 0, 0, loc)
			end := time.Date(day.Year(), day.Month(), day.Day(), endHour, endMinute, 0, 0, loc)
			out = append(out, Interval{Start: start, End: end})
		}
	}
	return out, nil
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func mondayOf(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7
	return dateOnly(t, t.Location()).AddDate(0, 0, -offset)
}

// daysBetween counts calendar days, using noon UTC so a DST transition inside
// the span cannot shave or add an hour and flip the week parity.
func daysBetween(from, to time.Time) int {
	fy, fm, fd := from.Date()
	ty, tm, td := to.Date()
	start := time.Date(fy, fm, fd, 12, 0, 0, 0, time.UTC)
	end := time.Date(ty, tm, td, 12, 0, 0, 0, time.UTC)
	return int(end.Sub(start).Hours() / 24)
}

func containsWeekday(days []int, weekday int) bool {
	for _, day := range days {
		if day == weekday {
			return true
		}
	}
	return false
}
