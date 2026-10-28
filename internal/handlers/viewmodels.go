package handlers

import (
	"time"

	"github.com/puppe1990/slot/internal/models"
)

type bookingView struct {
	ID            int64
	GuestName     string
	GuestEmail    string
	GuestNotes    string
	Title         string
	Status        string
	StatusLabel   string
	DateLabel     string
	GuestRange    string
	HostRange     string
	GuestTZLabel  string
	HostTZLabel   string
	LocationLabel string
	MeetingURL    string
	Attendance    string
	GuestWhatsApp string
	WhatsAppLink  string
	DurationLabel string
	Start         time.Time
	IsUpcoming    bool
	Cancelled     bool
}

func presentBooking(booking models.Booking, ownerLocation, guestLocation *time.Location, now time.Time, l localeText) bookingView {
	if guestLocation == nil {
		guestLocation = ownerLocation
	}
	return bookingView{
		ID:            booking.ID,
		GuestName:     booking.GuestName,
		GuestEmail:    booking.GuestEmail,
		GuestNotes:    booking.GuestNotes,
		Title:         booking.MeetingTypeTitle,
		Status:        string(booking.Status),
		StatusLabel:   l.statusLabel(booking.Status),
		DateLabel:     l.longDate(booking.StartUTC, ownerLocation),
		GuestRange:    l.clockRange(booking.StartUTC, booking.EndUTC, guestLocation),
		HostRange:     l.clockRange(booking.StartUTC, booking.EndUTC, ownerLocation),
		GuestTZLabel:  formatTimezone(booking.StartUTC, guestLocation),
		HostTZLabel:   formatTimezone(booking.StartUTC, ownerLocation),
		LocationLabel: l.locationLabel(booking.MeetingTypeLocation),
		MeetingURL:    booking.MeetingURL,
		Attendance:    booking.Attendance,
		GuestWhatsApp: booking.GuestWhatsApp,
		WhatsAppLink:  whatsAppLink(booking.GuestWhatsApp),
		DurationLabel: l.durationUpper(booking.MeetingTypeDuration),
		Start:         booking.StartUTC,
		IsUpcoming:    booking.StartUTC.After(now) && booking.Active(),
		Cancelled:     booking.Status == models.BookingStatusCancelled,
	}
}

type meetingTypeView struct {
	models.MeetingType
	PublicPath      string
	DurationLabel   string
	BufferLabel     string
	NoticeLabel     string
	HorizonLabel    string
	LocationLabel   string
	LocationDetails string
}

var noticeHourChoices = []int{0, 1, 2, 4, 12, 24, 48}

type noticeHourOption struct {
	Hours    int
	Label    string
	Selected bool
}

func noticeHourLabel(hours int, l localeText) string {
	if hours == 0 {
		return l.t("formats.notice_none")
	}
	return l.t("unit.hours", hours)
}

func noticeHourOptions(selected int, l localeText) []noticeHourOption {
	out := make([]noticeHourOption, 0, len(noticeHourChoices)+1)
	seen := false
	for _, hours := range noticeHourChoices {
		out = append(out, noticeHourOption{
			Hours:    hours,
			Label:    noticeHourLabel(hours, l),
			Selected: hours == selected,
		})
		if hours == selected {
			seen = true
		}
	}
	if !seen {
		out = append(out, noticeHourOption{
			Hours:    selected,
			Label:    noticeHourLabel(selected, l),
			Selected: true,
		})
	}
	return out
}

func presentMeetingType(meetingType models.MeetingType, l localeText) meetingTypeView {
	return meetingTypeView{
		MeetingType:     meetingType,
		PublicPath:      "/book/" + meetingType.Slug,
		DurationLabel:   l.durationUpper(meetingType.DurationMinutes),
		BufferLabel:     l.duration(meetingType.BufferBeforeMinutes) + " / " + l.duration(meetingType.BufferAfterMinutes),
		NoticeLabel:     l.notice(meetingType.MinNoticeHours),
		HorizonLabel:    l.horizon(meetingType.BookingHorizonDays),
		LocationLabel:   l.locationLabel(meetingType.LocationType),
		LocationDetails: meetingType.LocationDetails,
	}
}

// locationOptions is the modality picker. Only the values are stable; the
// labels are translated in the template through the location.* catalog keys.
var locationOptions = []selectOption{
	{Value: models.LocationGoogleMeet, Key: "location.meet"},
	{Value: models.LocationPhone, Key: "location.phone"},
	{Value: models.LocationCustom, Key: "location.custom"},
}

type selectOption struct {
	Value string
	Key   string
}
