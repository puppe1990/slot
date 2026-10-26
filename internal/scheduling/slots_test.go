package scheduling_test

import (
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/scheduling"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("Parse(%q): %v", value, err)
	}
	return ts
}

func weekdayRule(day int, start, end string) scheduling.WeeklyRule {
	return scheduling.WeeklyRule{DayOfWeek: day, StartTime: start, EndTime: end}
}

func singleDay(meetingType scheduling.MeetingType) scheduling.MeetingType {
	meetingType.BookingHorizonDays = 1
	return meetingType
}

func baseMeetingType() scheduling.MeetingType {
	return scheduling.MeetingType{
		DurationMinutes:     30,
		BufferBeforeMinutes: 0,
		BufferAfterMinutes:  0,
		MinNoticeHours:      0,
		BookingHorizonDays:  30,
	}
}

func startsUTC(slots []scheduling.Slot) []string {
	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slot.Start.UTC().Format(time.RFC3339))
	}
	return out
}

func assertStarts(t *testing.T, got []scheduling.Slot, want []string) {
	t.Helper()
	gotStarts := startsUTC(got)
	if len(gotStarts) != len(want) {
		t.Fatalf("slot count = %d (%v), want %d (%v)", len(gotStarts), gotStarts, len(want), want)
	}
	for i := range want {
		if gotStarts[i] != want[i] {
			t.Fatalf("slot[%d] = %s, want %s (all: %v)", i, gotStarts[i], want[i], gotStarts)
		}
	}
}

// 2026-10-26 is a Monday. New York is EDT (UTC-4) until Nov 1.
func TestAvailableSlotsFillsWeeklyWindow(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T04:00:00Z"), // Mon 00:00 EDT
		Rules:   []scheduling.WeeklyRule{weekdayRule(1, "09:00", "11:00")},
		Type:    singleDay(baseMeetingType()),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, slots, []string{
		"2026-10-26T13:00:00Z", // 09:00 EDT
		"2026-10-26T13:30:00Z",
		"2026-10-26T14:00:00Z",
		"2026-10-26T14:30:00Z",
	})
}

func TestAvailableSlotsSkipsCalendarBusyIntervals(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T04:00:00Z"),
		Rules:   []scheduling.WeeklyRule{weekdayRule(1, "09:00", "11:00")},
		Type:    singleDay(baseMeetingType()),
		Busy: []scheduling.Interval{
			{Start: mustTime(t, "2026-10-26T13:30:00Z"), End: mustTime(t, "2026-10-26T14:00:00Z")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, slots, []string{
		"2026-10-26T13:00:00Z",
		"2026-10-26T14:00:00Z",
		"2026-10-26T14:30:00Z",
	})
}

func TestAvailableSlotsAppliesBuffersAroundActiveBookings(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	meetingType := baseMeetingType()
	meetingType.BufferBeforeMinutes = 10
	meetingType.BufferAfterMinutes = 10

	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T04:00:00Z"),
		Rules:   []scheduling.WeeklyRule{weekdayRule(1, "09:00", "11:00")},
		Type:    singleDay(meetingType),
		Bookings: []scheduling.Interval{
			{Start: mustTime(t, "2026-10-26T13:00:00Z"), End: mustTime(t, "2026-10-26T13:30:00Z")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 09:00 booked, 09:30 falls inside the 10 minute buffer tail, 10:00 is free.
	assertStarts(t, slots, []string{
		"2026-10-26T14:00:00Z",
		"2026-10-26T14:30:00Z",
	})
}

func TestAvailableSlotsRespectsMinimumNotice(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	meetingType := baseMeetingType()
	meetingType.MinNoticeHours = 3

	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T12:00:00Z"), // 08:00 EDT
		Rules:   []scheduling.WeeklyRule{weekdayRule(1, "09:00", "14:00")},
		Type:    singleDay(meetingType),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Earliest start is 11:00 EDT (15:00Z); 09:00, 09:30, 10:00, 10:30 are out.
	assertStarts(t, slots, []string{
		"2026-10-26T15:00:00Z",
		"2026-10-26T15:30:00Z",
		"2026-10-26T16:00:00Z",
		"2026-10-26T16:30:00Z",
		"2026-10-26T17:00:00Z",
		"2026-10-26T17:30:00Z",
	})
}

func TestAvailableSlotsRespectsBookingHorizon(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	meetingType := baseMeetingType()
	meetingType.BookingHorizonDays = 1

	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T04:00:00Z"),
		Rules: []scheduling.WeeklyRule{
			weekdayRule(1, "09:00", "10:00"),
			weekdayRule(2, "09:00", "10:00"),
		},
		Type: meetingType,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, slots, []string{
		"2026-10-26T13:00:00Z",
		"2026-10-26T13:30:00Z",
	})
}

func TestAvailableSlotsHonorsUnavailableDateOverride(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T04:00:00Z"),
		Rules:   []scheduling.WeeklyRule{weekdayRule(1, "09:00", "10:00")},
		Type:    singleDay(baseMeetingType()),
		Overrides: []scheduling.DateOverride{
			{Date: "2026-10-26", IsUnavailable: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Fatalf("slots = %v, want none on an unavailable override", startsUTC(slots))
	}
}

func TestAvailableSlotsHonorsCustomDateOverrideWindow(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-10-26T04:00:00Z"),
		Rules:   []scheduling.WeeklyRule{weekdayRule(1, "09:00", "10:00")},
		Type:    singleDay(baseMeetingType()),
		Overrides: []scheduling.DateOverride{
			{Date: "2026-10-26", CustomStart: "15:00", CustomEnd: "16:00"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, slots, []string{
		"2026-10-26T19:00:00Z", // 15:00 EDT
		"2026-10-26T19:30:00Z",
	})
}

// 2026-03-08: New York jumps 02:00 EST -> 03:00 EDT. A 01:00-04:00 window is
// two real hours long, so 30 minute sessions yield four slots and none land on
// the nonexistent 02:00 local hour.
func TestAvailableSlotsHandlesSpringForwardGap(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-03-08T00:00:00Z"), // 19:00 EST on Mar 7
		Rules:   []scheduling.WeeklyRule{weekdayRule(0, "01:00", "04:00")},
		Type:    singleDay(baseMeetingType()),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, slots, []string{
		"2026-03-08T06:00:00Z", // 01:00 EST
		"2026-03-08T06:30:00Z",
		"2026-03-08T07:00:00Z", // 03:00 EDT
		"2026-03-08T07:30:00Z",
	})
}

// 2026-11-01: New York repeats 01:00-02:00 (EDT then EST), so a 01:00-03:00
// window is three real hours long.
func TestAvailableSlotsHandlesFallBackOverlap(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ: loc,
		Now:     mustTime(t, "2026-11-01T00:00:00Z"),
		Rules:   []scheduling.WeeklyRule{weekdayRule(0, "01:00", "03:00")},
		Type:    singleDay(baseMeetingType()),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := startsUTC(slots)
	if len(got) != 6 {
		t.Fatalf("slot count = %d (%v), want 6 across the repeated hour", len(got), got)
	}
	if got[0] != "2026-11-01T05:00:00Z" || got[5] != "2026-11-01T07:30:00Z" {
		t.Fatalf("slots = %v, want 05:00Z through 07:30Z", got)
	}
}

// Seeded property test: whatever faker throws at the calendar, a returned slot
// never collides with a busy interval once buffers are applied.
func TestAvailableSlotsNeverCollideWithFakedBusyIntervals(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	faker := gofakeit.New(20261026)

	for scenario := 0; scenario < 40; scenario++ {
		meetingType := scheduling.MeetingType{
			DurationMinutes:     []int{15, 30, 60}[faker.IntRange(0, 2)],
			BufferBeforeMinutes: faker.IntRange(0, 15),
			BufferAfterMinutes:  faker.IntRange(0, 15),
			MinNoticeHours:      faker.IntRange(0, 12),
			BookingHorizonDays:  14,
		}
		now := time.Date(2026, 10, 26, 4, 0, 0, 0, time.UTC)

		busy := make([]scheduling.Interval, 0, 12)
		for i := 0; i < 12; i++ {
			start := now.Add(time.Duration(faker.IntRange(0, 12*24)) * time.Hour)
			end := start.Add(time.Duration(faker.IntRange(15, 90)) * time.Minute)
			busy = append(busy, scheduling.Interval{Start: start, End: end})
		}

		slots, err := scheduling.AvailableSlots(scheduling.Query{
			OwnerTZ: loc,
			Now:     now,
			Rules: []scheduling.WeeklyRule{
				weekdayRule(1, "09:00", "17:00"),
				weekdayRule(2, "09:00", "17:00"),
				weekdayRule(3, "09:00", "17:00"),
				weekdayRule(4, "09:00", "17:00"),
				weekdayRule(5, "09:00", "17:00"),
			},
			Type: meetingType,
			Busy: busy,
		})
		if err != nil {
			t.Fatalf("scenario %d: %v", scenario, err)
		}

		earliest := now.Add(time.Duration(meetingType.MinNoticeHours) * time.Hour)
		horizonEnd := now.AddDate(0, 0, meetingType.BookingHorizonDays)
		for _, slot := range slots {
			if slot.Start.Before(earliest) {
				t.Fatalf("scenario %d: slot %s before notice cutoff %s", scenario, slot.Start, earliest)
			}
			if slot.End.After(horizonEnd) {
				t.Fatalf("scenario %d: slot %s after horizon %s", scenario, slot.End, horizonEnd)
			}
			if slot.End.Sub(slot.Start) != time.Duration(meetingType.DurationMinutes)*time.Minute {
				t.Fatalf("scenario %d: slot %s has duration %s", scenario, slot.Start, slot.End.Sub(slot.Start))
			}
			occupied := scheduling.Interval{
				Start: slot.Start.Add(-time.Duration(meetingType.BufferBeforeMinutes) * time.Minute),
				End:   slot.End.Add(time.Duration(meetingType.BufferAfterMinutes) * time.Minute),
			}
			for _, block := range busy {
				if occupied.Overlaps(block) {
					t.Fatalf("scenario %d: slot %s (%s) collides with busy %s-%s",
						scenario, slot.Start, occupied.Start.Format(time.RFC3339), block.Start, block.End)
				}
			}
		}
	}
}
