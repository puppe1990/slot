package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/scheduling"
)

const dateKeyLayout = "2006-01-02"

type timezoneOption struct {
	Value    string
	Label    string
	Selected bool
}

type slotView struct {
	StartISO   string
	EndISO     string
	GuestClock string
	GuestRange string
	HostClock  string
	HostRange  string
}

type slotDay struct {
	Date       string
	Weekday    string
	DayNumber  string
	HasSlots   bool
	IsSelected bool
	IsPast     bool
	Slots      []slotView
}

type slotPageData struct {
	MeetingType     models.MeetingType
	Owner           models.User
	GuestTimezone   string
	GuestTZLabel    string
	HostTZLabel     string
	Days            []slotDay
	Selected        slotDay
	SelectedDate    string
	DateLabel       string
	PrevDate        string
	NextDate        string
	TimezoneOptions []timezoneOption
	TotalAvailable  int
}

func (b base) buildSlots(ctx context.Context, meetingType models.MeetingType, guestTimezone, requestedDate string, excludeBookingID int64) (slotPageData, error) {
	owner, err := b.deps.Store.GetUser(meetingType.UserID)
	if err != nil {
		return slotPageData{}, fmt.Errorf("load owner: %w", err)
	}
	ownerLocation, err := time.LoadLocation(owner.Timezone)
	if err != nil {
		ownerLocation = time.UTC
	}
	guestTimezone = normalizeTimezone(guestTimezone, owner.Timezone)
	guestLocation, err := time.LoadLocation(guestTimezone)
	if err != nil {
		guestLocation = ownerLocation
	}

	now := b.now()
	horizonEnd := now.AddDate(0, 0, meetingType.BookingHorizonDays)
	rules, err := b.deps.Store.ListAvailabilityRules(owner.ID)
	if err != nil {
		return slotPageData{}, fmt.Errorf("load availability: %w", err)
	}
	overrides, err := b.deps.Store.ListDateOverrides(owner.ID,
		now.In(ownerLocation).Format(dateKeyLayout),
		horizonEnd.In(ownerLocation).AddDate(0, 0, 1).Format(dateKeyLayout))
	if err != nil {
		return slotPageData{}, fmt.Errorf("load overrides: %w", err)
	}
	blocks, err := b.deps.Store.ListActiveIntervalsExcluding(owner.ID, now, horizonEnd, now, excludeBookingID)
	if err != nil {
		return slotPageData{}, fmt.Errorf("load bookings: %w", err)
	}
	busy, err := b.deps.Calendar.ListBusyIntervals(ctx, now, horizonEnd)
	if err != nil {
		return slotPageData{}, fmt.Errorf("load busy intervals: %w", err)
	}

	slots, err := scheduling.AvailableSlots(scheduling.Query{
		OwnerTZ:   ownerLocation,
		Now:       now,
		Rules:     toWeeklyRules(rules),
		Overrides: toDateOverrides(overrides),
		Busy:      busy,
		Bookings:  blocks,
		Type: scheduling.MeetingType{
			DurationMinutes:     meetingType.DurationMinutes,
			BufferBeforeMinutes: meetingType.BufferBeforeMinutes,
			BufferAfterMinutes:  meetingType.BufferAfterMinutes,
			MinNoticeHours:      meetingType.MinNoticeHours,
			BookingHorizonDays:  meetingType.BookingHorizonDays,
		},
	})
	if err != nil {
		return slotPageData{}, fmt.Errorf("compute slots: %w", err)
	}

	byDate := map[string][]slotView{}
	for _, slot := range slots {
		key := formatDateKey(slot.Start, ownerLocation)
		byDate[key] = append(byDate[key], slotView{
			StartISO:   slot.Start.Format(time.RFC3339),
			EndISO:     slot.End.Format(time.RFC3339),
			GuestClock: formatClock(slot.Start, guestLocation),
			GuestRange: formatRange(slot.Start, slot.End, guestLocation),
			HostClock:  formatClock(slot.Start, ownerLocation),
			HostRange:  formatRange(slot.Start, slot.End, ownerLocation),
		})
	}

	today := time.Date(now.In(ownerLocation).Year(), now.In(ownerLocation).Month(), now.In(ownerLocation).Day(), 0, 0, 0, 0, ownerLocation)
	selectedDate := chooseDate(requestedDate, byDate, today, meetingType.BookingHorizonDays)

	selected := slotDay{Date: selectedDate, IsSelected: true, Slots: byDate[selectedDate], HasSlots: len(byDate[selectedDate]) > 0}
	selectedTime, _ := time.ParseInLocation(dateKeyLayout, selectedDate, ownerLocation)
	selected.Weekday = formatWeekday(selectedTime, ownerLocation)
	selected.DayNumber = formatDayNumber(selectedTime, ownerLocation)

	stripStart := selectedTime.AddDate(0, 0, -3)
	if stripStart.Before(today) {
		stripStart = today
	}
	days := make([]slotDay, 0, 7)
	for i := 0; i < 7; i++ {
		date := stripStart.AddDate(0, 0, i)
		if date.After(horizonEnd.In(ownerLocation)) {
			break
		}
		key := date.Format(dateKeyLayout)
		days = append(days, slotDay{
			Date:       key,
			Weekday:    formatWeekday(date, ownerLocation),
			DayNumber:  formatDayNumber(date, ownerLocation),
			HasSlots:   len(byDate[key]) > 0,
			IsSelected: key == selectedDate,
			IsPast:     date.Before(today),
			Slots:      byDate[key],
		})
	}

	prevDate := stripStart.AddDate(0, 0, -7)
	if prevDate.Before(today) {
		prevDate = today
	}
	nextDate := stripStart.AddDate(0, 0, 7)
	if nextDate.After(horizonEnd.In(ownerLocation)) {
		nextDate = selectedTime
	}

	return slotPageData{
		MeetingType:     meetingType,
		Owner:           owner,
		GuestTimezone:   guestTimezone,
		GuestTZLabel:    formatTimezone(now, guestLocation),
		HostTZLabel:     formatTimezone(now, ownerLocation),
		Days:            days,
		Selected:        selected,
		SelectedDate:    selectedDate,
		DateLabel:       formatLongDate(selectedTime, ownerLocation),
		PrevDate:        prevDate.Format(dateKeyLayout),
		NextDate:        nextDate.Format(dateKeyLayout),
		TimezoneOptions: buildTimezoneOptions(guestTimezone),
		TotalAvailable:  len(slots),
	}, nil
}

// chooseDate prefers the guest's pick, then the first day that actually has a
// slot, then today — never a date outside the booking horizon.
func chooseDate(requested string, byDate map[string][]slotView, today time.Time, horizonDays int) string {
	horizonEnd := today.AddDate(0, 0, horizonDays)
	if parsed, err := time.ParseInLocation(dateKeyLayout, requested, today.Location()); err == nil {
		if !parsed.Before(today) && !parsed.After(horizonEnd) {
			return requested
		}
	}
	for day := 0; day <= horizonDays; day++ {
		key := today.AddDate(0, 0, day).Format(dateKeyLayout)
		if len(byDate[key]) > 0 {
			return key
		}
	}
	return today.Format(dateKeyLayout)
}

func buildTimezoneOptions(selected string) []timezoneOption {
	options := make([]timezoneOption, 0, len(timezoneOptions)+1)
	known := false
	for _, name := range timezoneOptions {
		known = known || name == selected
	}
	// A browser-detected zone outside the curated list still has to render,
	// otherwise the select would silently fall back to the host clock.
	if selected != "" && !known {
		options = append(options, timezoneOption{Value: selected, Label: selected, Selected: true})
	}
	for _, name := range timezoneOptions {
		options = append(options, timezoneOption{Value: name, Label: name, Selected: name == selected})
	}
	return options
}

func toWeeklyRules(rules []models.AvailabilityRule) []scheduling.WeeklyRule {
	out := make([]scheduling.WeeklyRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.IsActive {
			continue
		}
		out = append(out, scheduling.WeeklyRule{
			DayOfWeek: rule.DayOfWeek,
			StartTime: rule.StartTime,
			EndTime:   rule.EndTime,
		})
	}
	return out
}

func toDateOverrides(overrides []models.DateOverride) []scheduling.DateOverride {
	out := make([]scheduling.DateOverride, 0, len(overrides))
	for _, override := range overrides {
		out = append(out, scheduling.DateOverride{
			Date:          override.Date,
			IsUnavailable: override.IsUnavailable,
			CustomStart:   override.CustomStartTime,
			CustomEnd:     override.CustomEndTime,
		})
	}
	return out
}
