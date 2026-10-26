package handlers

import (
	"fmt"
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
	DurationLabel string
	Start         time.Time
	IsUpcoming    bool
	Cancelled     bool
}

func presentBooking(booking models.Booking, ownerLocation, guestLocation *time.Location, now time.Time) bookingView {
	if guestLocation == nil {
		guestLocation = ownerLocation
	}
	status := string(booking.Status)
	label := "Confirmed"
	switch booking.Status {
	case models.BookingStatusCancelled:
		label = "Cancelled"
	case models.BookingStatusRescheduled:
		label = "Rescheduled"
	case models.BookingStatusHold:
		label = "Held"
	}
	return bookingView{
		ID:            booking.ID,
		GuestName:     booking.GuestName,
		GuestEmail:    booking.GuestEmail,
		GuestNotes:    booking.GuestNotes,
		Title:         booking.MeetingTypeTitle,
		Status:        status,
		StatusLabel:   label,
		DateLabel:     formatLongDate(booking.StartUTC, ownerLocation),
		GuestRange:    formatRange(booking.StartUTC, booking.EndUTC, guestLocation),
		HostRange:     formatRange(booking.StartUTC, booking.EndUTC, ownerLocation),
		GuestTZLabel:  formatTimezone(booking.StartUTC, guestLocation),
		HostTZLabel:   formatTimezone(booking.StartUTC, ownerLocation),
		LocationLabel: locationLabel(booking.MeetingTypeLocation),
		DurationLabel: formatDurationShort(booking.MeetingTypeDuration),
		Start:         booking.StartUTC,
		IsUpcoming:    booking.StartUTC.After(now) && booking.Active(),
		Cancelled:     booking.Status == models.BookingStatusCancelled,
	}
}

func locationLabel(locationType string) string {
	switch locationType {
	case models.LocationGoogleMeet:
		return "Google Meet"
	case models.LocationPhone:
		return "Phone call"
	case models.LocationCustom:
		return "Custom location"
	default:
		return "To be confirmed"
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

func presentMeetingType(meetingType models.MeetingType) meetingTypeView {
	return meetingTypeView{
		MeetingType:     meetingType,
		PublicPath:      "/book/" + meetingType.Slug,
		DurationLabel:   formatDurationShort(meetingType.DurationMinutes),
		BufferLabel:     formatDuration(meetingType.BufferBeforeMinutes) + " / " + formatDuration(meetingType.BufferAfterMinutes),
		NoticeLabel:     formatDuration(meetingType.MinNoticeHours*60) + " notice",
		HorizonLabel:    formatHorizon(meetingType.BookingHorizonDays),
		LocationLabel:   locationLabel(meetingType.LocationType),
		LocationDetails: meetingType.LocationDetails,
	}
}

var locationOptions = []selectOption{
	{Value: models.LocationGoogleMeet, Label: "Google Meet"},
	{Value: models.LocationPhone, Label: "Phone call"},
	{Value: models.LocationCustom, Label: "Custom location"},
}

type selectOption struct {
	Value string
	Label string
}

func formatHorizon(days int) string {
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
