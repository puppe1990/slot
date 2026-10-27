package scheduling

import (
	"testing"
	"time"
)

// The case that started this: a two-week rota where the morning block lands on
// week 0 and the night block on week 1, Monday to Friday.
func TestExpandRecurringBlocksAlternatesWeeks(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, 10, 26, 0, 0, 0, 0, loc) // Monday
	to := time.Date(2026, 11, 13, 23, 59, 0, 0, loc)

	blocks := []RecurringBlock{
		{Weekdays: []int{1, 2, 3, 4, 5}, StartTime: "07:00", EndTime: "11:00", IntervalWeeks: 2, AnchorDate: "2026-10-26"},
		{Weekdays: []int{1, 2, 3, 4, 5}, StartTime: "17:45", EndTime: "23:45", IntervalWeeks: 2, AnchorDate: "2026-11-02"},
	}

	intervals, err := ExpandRecurringBlocks(from, to, loc, blocks)
	if err != nil {
		t.Fatal(err)
	}

	morning := map[string]bool{}
	night := map[string]bool{}
	for _, interval := range intervals {
		if interval.Start.Hour() == 7 {
			morning[interval.Start.Format(dateLayout)] = true
		}
		if interval.Start.Hour() == 17 {
			night[interval.Start.Format(dateLayout)] = true
		}
	}

	for _, day := range []string{"2026-10-26", "2026-10-27", "2026-10-30", "2026-11-09", "2026-11-13"} {
		if !morning[day] {
			t.Errorf("morning block missing on %s", day)
		}
	}
	for _, day := range []string{"2026-11-02", "2026-11-03", "2026-11-06"} {
		if morning[day] {
			t.Errorf("morning block should skip the off week: %s", day)
		}
	}
	for _, day := range []string{"2026-11-02", "2026-11-06"} {
		if !night[day] {
			t.Errorf("night block missing on %s", day)
		}
	}
	if night["2026-10-26"] {
		t.Error("night block should not run on the anchor week")
	}
}

func TestExpandRecurringBlocksWeeklyAndOffWeek(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, 10, 26, 0, 0, 0, 0, loc)
	to := time.Date(2026, 11, 8, 0, 0, 0, 0, loc)

	weekly := []RecurringBlock{{Weekdays: []int{1}, StartTime: "09:00", EndTime: "10:00", IntervalWeeks: 1, AnchorDate: "2026-10-26"}}
	intervals, err := ExpandRecurringBlocks(from, to, loc, weekly)
	if err != nil {
		t.Fatal(err)
	}
	if len(intervals) != 2 {
		t.Fatalf("weekly interval got %d blocks, want 2", len(intervals))
	}

	offWeek := []RecurringBlock{{Weekdays: []int{1}, StartTime: "09:00", EndTime: "10:00", IntervalWeeks: 2, AnchorDate: "2026-11-02"}}
	intervals, err = ExpandRecurringBlocks(from, to, loc, offWeek)
	if err != nil {
		t.Fatal(err)
	}
	if len(intervals) != 1 || intervals[0].Start.Format(dateLayout) != "2026-11-02" {
		t.Fatalf("anchored block = %+v", intervals)
	}
}

func TestRecurringBlockKeepsSlotsOutOfTheWindow(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 10, 24, 8, 0, 0, 0, loc) // Saturday

	busy, err := ExpandRecurringBlocks(now, now.AddDate(0, 0, 21), loc, []RecurringBlock{
		{Weekdays: []int{1}, StartTime: "07:00", EndTime: "11:00", IntervalWeeks: 2, AnchorDate: "2026-10-26"},
	})
	if err != nil {
		t.Fatal(err)
	}

	slots, err := AvailableSlots(Query{
		OwnerTZ: loc,
		Now:     now,
		Rules:   []WeeklyRule{{DayOfWeek: 1, StartTime: "06:00", EndTime: "12:00"}},
		Busy:    busy,
		Type: MeetingType{
			DurationMinutes:    60,
			BookingHorizonDays: 21,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	byDay := map[string][]int{}
	for _, slot := range slots {
		key := slot.Start.In(loc).Format(dateLayout)
		byDay[key] = append(byDay[key], slot.Start.In(loc).Hour())
	}
	if got := byDay["2026-10-26"]; len(got) != 2 || got[0] != 6 || got[1] != 11 {
		t.Fatalf("blocked week slots = %v, want [6 11]", got)
	}
	if got := byDay["2026-11-02"]; len(got) != 6 {
		t.Fatalf("off week slots = %v, want the full 06..11 window", got)
	}
	if got := byDay["2026-11-09"]; len(got) != 2 {
		t.Fatalf("second blocked week slots = %v, want [6 11]", got)
	}
}

func TestExpandRecurringBlocksRejectsABackwardsClock(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, 10, 26, 0, 0, 0, 0, loc)
	_, err := ExpandRecurringBlocks(from, from.AddDate(0, 0, 2), loc, []RecurringBlock{
		{Weekdays: []int{1}, StartTime: "11:00", EndTime: "07:00", IntervalWeeks: 2, AnchorDate: "2026-10-26"},
	})
	if err == nil {
		t.Fatal("backwards clock should be rejected")
	}
}
