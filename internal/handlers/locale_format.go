package handlers

import (
	"net/http"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"

	"github.com/puppe1990/slot/internal/models"
)

// localeText formats everything Go builds at request time: dates, clocks,
// durations, statuses and locations. Static copy lives in the i18n catalogs;
// this type covers the strings that need a value or a date.
type localeText struct {
	tag     string
	catalog *i18n.Catalog
}

func (b base) localeFor(r *http.Request) localeText {
	catalog := i18n.CatalogOr(r, b.deps.Catalog)
	return localeText{tag: catalog.Locale(), catalog: catalog}
}

func (l localeText) t(key string, args ...any) string {
	if l.catalog == nil {
		return key
	}
	return l.catalog.T(key, args...)
}

func (l localeText) isPT() bool {
	return l.tag == "pt"
}

// clock renders 24h for pt-BR and 12h for English. Guests compare the guest
// clock with the host clock, so both use the same convention on a page.
func (l localeText) clock(ts time.Time, loc *time.Location) string {
	if l.isPT() {
		return ts.In(loc).Format("15:04")
	}
	return ts.In(loc).Format("3:04 PM")
}

func (l localeText) clockRange(start, end time.Time, loc *time.Location) string {
	return l.clock(start, loc) + " – " + l.clock(end, loc)
}

func (l localeText) longDate(ts time.Time, loc *time.Location) string {
	if l.isPT() {
		local := ts.In(loc)
		return ptWeekdaysLong[local.Weekday()] + ", " +
			itoa(local.Day()) + " de " + ptMonths[local.Month()-1] + " de " + itoa(local.Year())
	}
	return ts.In(loc).Format("Monday, January 2, 2006")
}

// shortDate is "2 Nov" in English and "2 nov" in Portuguese.
func (l localeText) shortDate(ts time.Time, loc *time.Location) string {
	local := ts.In(loc)
	if l.isPT() {
		return itoa(local.Day()) + " " + ptMonthsShort[local.Month()-1]
	}
	return local.Format("2 Jan")
}

func (l localeText) weekdayShort(ts time.Time, loc *time.Location) string {
	if l.isPT() {
		return ptWeekdaysShort[ts.In(loc).Weekday()]
	}
	return ts.In(loc).Format("Mon")
}

func (l localeText) dayNumber(ts time.Time, loc *time.Location) string {
	return ts.In(loc).Format("2")
}

func (l localeText) duration(minutes int) string {
	switch {
	case minutes%60 == 0:
		return l.t("unit.hours", minutes/60)
	case minutes > 60:
		return l.t("unit.hours_minutes", minutes/60, minutes%60)
	default:
		return l.t("unit.minutes", minutes)
	}
}

func (l localeText) durationUpper(minutes int) string {
	return l.t("unit.minutes_upper", minutes)
}

func (l localeText) horizon(days int) string {
	if days == 1 {
		return l.t("unit.horizon_one")
	}
	return l.t("unit.horizon", days)
}

func (l localeText) notice(hours int) string {
	if hours == 0 {
		return l.t("formats.notice_none")
	}
	return l.t("unit.notice", l.duration(hours*60))
}

func (l localeText) statusLabel(status models.BookingStatus) string {
	switch status {
	case models.BookingStatusCancelled:
		return l.t("status.cancelled")
	case models.BookingStatusRescheduled:
		return l.t("status.rescheduled")
	case models.BookingStatusHold:
		return l.t("status.held")
	default:
		return l.t("status.confirmed")
	}
}

func (l localeText) locationLabel(locationType string) string {
	switch locationType {
	case models.LocationGoogleMeet:
		return l.t("location.meet")
	case models.LocationPhone:
		return l.t("location.phone")
	case models.LocationCustom:
		return l.t("location.custom")
	default:
		return l.t("location.tbc")
	}
}

var ptWeekdaysLong = [7]string{
	"domingo", "segunda-feira", "terça-feira", "quarta-feira",
	"quinta-feira", "sexta-feira", "sábado",
}

var ptWeekdaysShort = [7]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

var ptMonthsShort = [12]string{
	"jan", "fev", "mar", "abr", "mai", "jun",
	"jul", "ago", "set", "out", "nov", "dez",
}

var ptMonths = [12]string{
	"janeiro", "fevereiro", "março", "abril", "maio", "junho",
	"julho", "agosto", "setembro", "outubro", "novembro", "dezembro",
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [4]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
