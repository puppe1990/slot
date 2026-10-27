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
var locationOptions = []string{
	models.LocationGoogleMeet,
	models.LocationPhone,
	models.LocationCustom,
}
