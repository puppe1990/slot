package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
)

// Email copy lives here so a subject change never drifts between the confirm,
// the manage page and the dashboard. Bodies follow the request locale, which is
// the language the guest booked in.
func (b base) sendConfirmed(r *http.Request, booking models.Booking, manageURL string) {
	l := b.localeFor(r)
	loc := mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone))
	b.send(r, notify.Message{
		Kind:    "booking.confirmed",
		To:      booking.GuestEmail,
		Subject: l.t("email.confirmed.subject", booking.MeetingTypeTitle),
		Body: b.bookingEmailBody(l, booking, loc,
			l.t("email.confirmed.intro", booking.MeetingTypeTitle, booking.OwnerName), manageURL),
	})
}

func (b base) sendOwnerNew(r *http.Request, booking models.Booking) {
	l := b.localeFor(r)
	loc := mustLocation(booking.OwnerTimezone)
	b.send(r, notify.Message{
		Kind:    "booking.owner_new",
		To:      booking.OwnerEmail,
		Subject: l.t("email.owner_new.subject", booking.GuestName),
		Body: b.bookingEmailBody(l, booking, loc,
			l.t("email.owner_new.intro", booking.GuestName, booking.GuestEmail, booking.MeetingTypeTitle), ""),
	})
}

func (b base) sendMoved(r *http.Request, booking models.Booking, manageURL string) {
	l := b.localeFor(r)
	loc := mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone))
	b.send(r, notify.Message{
		Kind:    "booking.moved",
		To:      booking.GuestEmail,
		Subject: l.t("email.moved.subject", booking.MeetingTypeTitle),
		Body: b.bookingEmailBody(l, booking, loc,
			l.t("email.moved.intro", booking.MeetingTypeTitle), manageURL),
	})
}

func (b base) sendOwnerMoved(r *http.Request, booking models.Booking) {
	l := b.localeFor(r)
	loc := mustLocation(booking.OwnerTimezone)
	b.send(r, notify.Message{
		Kind:    "booking.owner_moved",
		To:      booking.OwnerEmail,
		Subject: l.t("email.owner_moved.subject", booking.GuestName),
		Body: b.bookingEmailBody(l, booking, loc,
			l.t("email.owner_moved.intro", booking.GuestName, booking.MeetingTypeTitle), ""),
	})
}

func (b base) sendCancelled(r *http.Request, booking models.Booking, manageURL, reason string) {
	l := b.localeFor(r)
	loc := mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone))
	b.send(r, notify.Message{
		Kind:    "booking.cancelled",
		To:      booking.GuestEmail,
		Subject: l.t("email.cancelled.subject", booking.MeetingTypeTitle),
		Body: b.bookingEmailBody(l, booking, loc,
			l.t("email.cancelled.intro", booking.MeetingTypeTitle), manageURL) +
			reasonLine(l, reason),
	})
}

func (b base) sendOwnerCancelled(r *http.Request, booking models.Booking, reason string) {
	l := b.localeFor(r)
	loc := mustLocation(booking.OwnerTimezone)
	b.send(r, notify.Message{
		Kind:    "booking.owner_cancelled",
		To:      booking.OwnerEmail,
		Subject: l.t("email.owner_cancelled.subject", booking.GuestName),
		Body: b.bookingEmailBody(l, booking, loc,
			l.t("email.owner_cancelled.intro", booking.GuestName, booking.MeetingTypeTitle), "") +
			reasonLine(l, reason),
	})
}

func (b base) bookingEmailBody(l localeText, booking models.Booking, loc *time.Location, intro, manageURL string) string {
	lines := []string{
		intro,
		"",
		l.longDate(booking.StartUTC, loc) + " · " + l.clockRange(booking.StartUTC, booking.EndUTC, loc) +
			" (" + formatTimezone(booking.StartUTC, loc) + ")",
	}
	if booking.MeetingURL != "" {
		lines = append(lines, l.t("email.join")+": "+booking.MeetingURL)
	}
	if manageURL != "" {
		lines = append(lines, l.t("email.manage")+": "+manageURL)
	}
	return strings.Join(lines, "\n")
}

func reasonLine(l localeText, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	return "\n" + l.t("email.reason") + ": " + reason
}

// send never fails a request: the booking is already committed and a mail
// outage must not turn a confirmation into a 500.
func (b base) send(r *http.Request, message notify.Message) {
	if strings.TrimSpace(message.To) == "" {
		return
	}
	if err := b.deps.Notifier.Send(r.Context(), message); err != nil {
		log.Printf("slot: notify %s on %s: %v", message.Kind, r.URL.Path, err)
	}
}
