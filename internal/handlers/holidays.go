package handlers

import (
	"time"

	"github.com/puppe1990/slot/internal/holidays"
	"github.com/puppe1990/slot/internal/models"
)

type holidayView struct {
	Key     string
	Name    string
	Enabled bool
	Next    string
}

// presentHolidays merges the fixed calendar with the owner's toggles. A key
// with no stored preference is on: the calendar starts closed.
func presentHolidays(prefs map[string]bool, now time.Time, loc *time.Location, l localeText) []holidayView {
	list := holidays.List()
	views := make([]holidayView, 0, len(list))
	thisYear := now.In(loc).Year()
	for _, day := range list {
		enabled := true
		if value, set := prefs[day.Key]; set {
			enabled = value
		}
		next := day.NextOnOrAfter(now, loc)
		label := l.shortDate(next, loc)
		if next.Year() != thisYear {
			label += " " + itoa(next.Year())
		}
		views = append(views, holidayView{
			Key:     day.Key,
			Name:    l.t("holiday." + day.Key),
			Enabled: enabled,
			Next:    label,
		})
	}
	return views
}

// holidayClosures turns enabled holidays inside the horizon into closed dates.
// A manual date override for the same day wins: the owner deliberately chose
// to open it.
func holidayClosures(overrides []models.DateOverride, prefs map[string]bool, now, horizonEnd time.Time, loc *time.Location) []models.DateOverride {
	explicit := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		explicit[override.Date] = struct{}{}
	}

	from := now.In(loc).Format(dateKeyLayout)
	to := horizonEnd.In(loc).Format(dateKeyLayout)
	out := overrides
	for _, day := range holidays.List() {
		if enabled, set := prefs[day.Key]; set && !enabled {
			continue
		}
		for year := now.In(loc).Year(); year <= horizonEnd.In(loc).Year(); year++ {
			key := day.Occurrence(year, loc).Format(dateKeyLayout)
			if key < from || key > to {
				continue
			}
			if _, taken := explicit[key]; taken {
				continue
			}
			out = append(out, models.DateOverride{Date: key, IsUnavailable: true})
		}
	}
	return out
}
