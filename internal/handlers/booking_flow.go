package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/calendar"
	"github.com/puppe1990/slot/internal/call"
	"github.com/puppe1990/slot/internal/models"
)

// finalizeBooking runs the side effects of a confirmed booking: calendar event,
// video room, guest and owner notices. It never fails the request — the slot is
// already committed, so a vendor hiccup is logged and the booking stands.
func (b base) finalizeBooking(r *http.Request, booking models.Booking, guestToken string) {
	// Room first: the calendar event carries the signed join link, so it needs
	// to know whether a room exists.
	room, err := b.deps.Call.CreateRoom(r.Context(), call.Input{
		BookingID: booking.ID,
		Title:     booking.MeetingTypeTitle,
		Start:     booking.StartUTC,
		End:       booking.EndUTC,
	})
	if err != nil {
		log.Printf("slot: create room for booking %d: %v", booking.ID, err)
	} else if room.URL != "" {
		if err := b.deps.Store.AttachMeetingURL(booking.ID, room.URL); err != nil {
			log.Printf("slot: attach room to booking %d: %v", booking.ID, err)
		}
		booking.MeetingURL = room.URL
	}

	event, err := b.deps.Calendar.CreateEvent(r.Context(), b.calendarEvent(booking, b.localeFor(r)))
	if err != nil {
		log.Printf("slot: create calendar event for booking %d: %v", booking.ID, err)
	} else if err := b.deps.Store.AttachCalendarEvent(booking.ID, event.ID); err != nil {
		log.Printf("slot: attach calendar event %s to booking %d: %v", event.ID, booking.ID, err)
	}

	manageURL := b.absoluteURL(r, "/manage/"+guestToken)
	b.sendConfirmed(r, booking, manageURL)
	b.sendOwnerNew(r, booking)
}

// calendarEvent is the single description of what a Slot booking looks like in
// the owner's calendar.
func (b base) calendarEvent(booking models.Booking, l localeText) calendar.Event {
	details := bookingNotes(booking)
	location := meetingLocation(booking, l)

	// The room URL alone is useless (private room, token required), so the
	// event carries the signed join route — the same one the e-mail sends.
	if root := strings.TrimRight(b.deps.Site.AppURL, "/"); root != "" && b.deps.Signer != nil {
		manageURL := root + "/manage/" + b.deps.Signer.Sign(booking.ID)
		if join := joinURL(manageURL, booking.MeetingURL); join != "" {
			details += "\n\n" + l.t("call.join") + ": " + join
			location = join
		}
		details += "\n" + l.t("email.cta.manage") + ": " + manageURL
	}

	return calendar.Event{
		Summary:   booking.MeetingTypeTitle + " · " + booking.GuestName,
		Details:   details,
		Start:     booking.StartUTC,
		End:       booking.EndUTC,
		Attendees: []string{booking.GuestEmail, booking.OwnerEmail},
		Location:  location,
	}
}

// cancelBooking applies the cancellation and its side effects. The caller owns
// authorization (owner session or signed guest link).
func (b base) cancelBooking(r *http.Request, booking models.Booking, actor, reason, manageURL string) error {
	if err := b.deps.Store.CancelBooking(booking.ID, actor, reason); err != nil {
		return err
	}
	if booking.CalendarEventID != "" {
		if err := b.deps.Calendar.DeleteEvent(r.Context(), booking.CalendarEventID); err != nil {
			log.Printf("slot: delete calendar event for booking %d: %v", booking.ID, err)
		}
	}
	if booking.MeetingURL != "" {
		if err := b.deps.Call.EndRoom(r.Context(), call.Room{URL: booking.MeetingURL}); err != nil {
			log.Printf("slot: end room for booking %d: %v", booking.ID, err)
		}
	}
	b.sendCancelled(r, booking, manageURL, reason)
	b.sendOwnerCancelled(r, booking, reason)
	return nil
}

// rescheduleBooking moves the session, keeping the same side effects no matter
// who asked (owner dashboard, guest link or CLI).
func (b base) rescheduleBooking(r *http.Request, booking models.Booking, start time.Time, actor, reason, manageURL string) error {
	end := start.Add(time.Duration(booking.MeetingTypeDuration) * time.Minute)
	if err := b.deps.Store.RescheduleBooking(booking.ID, start.UTC(), end.UTC(), actor, reason, b.now()); err != nil {
		return err
	}
	if booking.CalendarEventID != "" {
		if err := b.deps.Calendar.UpdateEvent(r.Context(), booking.CalendarEventID, start.UTC(), end.UTC()); err != nil {
			log.Printf("slot: update calendar event for booking %d: %v", booking.ID, err)
		}
	} else {
		// The confirmation may have failed to write the event (calendar outage
		// or a rejected credential). A reschedule heals it instead of leaving
		// the session invisible on the calendar forever.
		moved := booking
		moved.StartUTC = start.UTC()
		moved.EndUTC = end.UTC()
		event, err := b.deps.Calendar.CreateEvent(r.Context(), b.calendarEvent(moved, b.localeFor(r)))
		if err != nil {
			log.Printf("slot: create missing calendar event for booking %d: %v", booking.ID, err)
		} else if err := b.deps.Store.AttachCalendarEvent(booking.ID, event.ID); err != nil {
			log.Printf("slot: attach calendar event %s to booking %d: %v", event.ID, booking.ID, err)
		}
	}
	b.sendMoved(r, booking, manageURL)
	b.sendOwnerMoved(r, booking)
	return nil
}
