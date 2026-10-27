package handlers

import (
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/scheduling"
)

// buildRecurringBlock validates the raw form/JSON fields once, so the setup
// page and the admin API cannot disagree about what a valid block is.
func buildRecurringBlock(userID int64, title string, weekdays []int, start, end string, interval int, anchor, today string, l localeText) (models.RecurringBlock, validate.FieldErrors) {
	block := models.RecurringBlock{
		UserID:        userID,
		Title:         strings.TrimSpace(title),
		Weekdays:      weekdays,
		StartTime:     start,
		EndTime:       end,
		IntervalWeeks: interval,
		AnchorDate:    anchor,
	}
	var errs validate.FieldErrors

	if len(weekdays) == 0 {
		errs.Add("weekdays", l.t("setup.error_weekday"))
	}
	startHour, startMinute, startErr := scheduling.ParseClock(start)
	endHour, endMinute, endErr := scheduling.ParseClock(end)
	if startErr != nil {
		errs.Add("start_time", l.t("setup.error_start"))
	}
	if endErr != nil {
		errs.Add("end_time", l.t("setup.error_end"))
	}
	if startErr == nil && endErr == nil && endHour*60+endMinute <= startHour*60+startMinute {
		errs.Add("end_time", l.t("setup.error_clock"))
	}
	if interval < 1 || interval > 8 {
		errs.Add("interval_weeks", l.t("setup.error_interval"))
	}
	if anchor == "" {
		anchor = today
		block.AnchorDate = today
	}
	if _, err := time.Parse(dateKeyLayout, anchor); err != nil {
		errs.Add("anchor_date", l.t("setup.error_anchor"))
	}
	return block, errs
}

// firstError is what the flash cookie needs: one message, in the user's locale.
func firstError(errs validate.FieldErrors) string {
	for _, message := range errs {
		return message
	}
	return ""
}
