package handlers

import (
	"fmt"
	"strings"
	"time"
)

// Temporal strings stay in one place so the mono columns line up: clock times
// are always padded, ranges always use an en dash.
func formatClock(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("3:04 PM")
}

func formatRange(start, end time.Time, loc *time.Location) string {
	return formatClock(start, loc) + " – " + formatClock(end, loc)
}

func formatLongDate(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("Monday, January 2, 2006")
}

func formatWeekday(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("Mon")
}

func formatDayNumber(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("2")
}

func formatDateKey(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("2006-01-02")
}

// zoneAbbrev renders "EDT"; zoneOffset renders "UTC-04:00". The pair keeps the
// timezone honest across DST instead of assuming a fixed offset.
func zoneAbbrev(ts time.Time, loc *time.Location) string {
	name, _ := ts.In(loc).Zone()
	return name
}

func zoneOffset(ts time.Time, loc *time.Location) string {
	_, seconds := ts.In(loc).Zone()
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}

func formatTimezone(ts time.Time, loc *time.Location) string {
	return loc.String() + " (" + zoneAbbrev(ts, loc) + ")"
}

func formatDuration(minutes int) string {
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	if minutes > 60 {
		return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%d min", minutes)
}

func formatDurationShort(minutes int) string {
	return fmt.Sprintf("%d MIN", minutes)
}

// timezoneOptions is the curated picker on the public page. Free text would
// accept typos that only explode when a slot is rendered.
var timezoneOptions = []string{
	"America/Los_Angeles",
	"America/Denver",
	"America/Chicago",
	"America/New_York",
	"America/Sao_Paulo",
	"America/Argentina/Buenos_Aires",
	"Atlantic/Azores",
	"Europe/London",
	"Europe/Lisbon",
	"Europe/Madrid",
	"Europe/Paris",
	"Europe/Berlin",
	"Europe/Amsterdam",
	"Europe/Rome",
	"Europe/Warsaw",
	"Europe/Athens",
	"Europe/Istanbul",
	"Europe/Moscow",
	"Africa/Casablanca",
	"Africa/Lagos",
	"Africa/Johannesburg",
	"Asia/Dubai",
	"Asia/Karachi",
	"Asia/Kolkata",
	"Asia/Bangkok",
	"Asia/Singapore",
	"Asia/Shanghai",
	"Asia/Tokyo",
	"Asia/Seoul",
	"Australia/Sydney",
	"Pacific/Auckland",
	"UTC",
}

func isKnownTimezone(name string) bool {
	if name == "" {
		return false
	}
	for _, option := range timezoneOptions {
		if option == name {
			return true
		}
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

func normalizeTimezone(name, fallback string) string {
	name = strings.TrimSpace(name)
	if isKnownTimezone(name) {
		return name
	}
	return fallback
}
