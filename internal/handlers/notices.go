package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
)

// bookingEmail is one notice, in both flavours. Email copy lives here so a
// subject change never drifts between confirm, manage and dashboard, and the
// body follows the locale the guest booked in.
type bookingEmail struct {
	Kind      string
	To        string
	Subject   string
	Heading   string
	Tone      string
	Intro     string
	Location  *time.Location
	CTAURL    string
	CTALabel  string
	ManageURL string
	Reason    string
	WhatsApp  string
	// Guest block: owner-facing notices show who is coming, how to reach them
	// and what they asked for.
	GuestEmail    string
	GuestWhatsApp string
	GuestNotes    string
}

func (b base) sendConfirmed(r *http.Request, booking models.Booking, manageURL string) {
	l := b.localeFor(r)
	b.send(r, b.bookingEmailMessage(r, l, booking, bookingEmail{
		Kind:      "booking.confirmed",
		To:        booking.GuestEmail,
		Subject:   l.t("email.confirmed.subject", booking.MeetingTypeTitle),
		Heading:   l.t("email.heading.confirmed"),
		Tone:      "pine",
		Intro:     l.t("email.confirmed.intro", booking.MeetingTypeTitle, booking.OwnerName),
		Location:  mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone)),
		CTAURL:    joinURL(manageURL, booking.MeetingURL),
		CTALabel:  l.t("call.join"),
		ManageURL: manageURL,
	}))
}

func (b base) sendOwnerNew(r *http.Request, booking models.Booking) {
	l := b.localeFor(r)
	b.send(r, b.bookingEmailMessage(r, l, booking, bookingEmail{
		Kind:          "booking.owner_new",
		To:            booking.OwnerEmail,
		Subject:       l.t("email.owner_new.subject", booking.GuestName),
		Heading:       l.t("email.heading.owner_new"),
		Tone:          "pine",
		Intro:         l.t("email.owner_new.intro", booking.GuestName, booking.MeetingTypeTitle),
		Location:      mustLocation(booking.OwnerTimezone),
		WhatsApp:      booking.GuestWhatsApp,
		GuestEmail:    booking.GuestEmail,
		GuestWhatsApp: booking.GuestWhatsApp,
		GuestNotes:    booking.GuestNotes,
		CTAURL:        b.absoluteURL(r, "/dashboard"),
		CTALabel:      l.t("email.cta.dashboard"),
	}))
}

func (b base) sendMoved(r *http.Request, booking models.Booking, manageURL string) {
	l := b.localeFor(r)
	b.send(r, b.bookingEmailMessage(r, l, booking, bookingEmail{
		Kind:      "booking.moved",
		To:        booking.GuestEmail,
		Subject:   l.t("email.moved.subject", booking.MeetingTypeTitle),
		Heading:   l.t("email.heading.moved"),
		Tone:      "slate",
		Intro:     l.t("email.moved.intro", booking.MeetingTypeTitle),
		Location:  mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone)),
		CTAURL:    joinURL(manageURL, booking.MeetingURL),
		CTALabel:  l.t("call.join"),
		ManageURL: manageURL,
	}))
}

func (b base) sendOwnerMoved(r *http.Request, booking models.Booking) {
	l := b.localeFor(r)
	b.send(r, b.bookingEmailMessage(r, l, booking, bookingEmail{
		Kind:     "booking.owner_moved",
		To:       booking.OwnerEmail,
		Subject:  l.t("email.owner_moved.subject", booking.GuestName),
		Heading:  l.t("email.heading.owner_moved"),
		Tone:     "slate",
		Intro:    l.t("email.owner_moved.intro", booking.GuestName, booking.MeetingTypeTitle),
		Location: mustLocation(booking.OwnerTimezone),
		CTAURL:   b.absoluteURL(r, "/dashboard"),
		CTALabel: l.t("email.cta.dashboard"),
	}))
}

func (b base) sendCancelled(r *http.Request, booking models.Booking, manageURL, reason string) {
	l := b.localeFor(r)
	b.send(r, b.bookingEmailMessage(r, l, booking, bookingEmail{
		Kind:      "booking.cancelled",
		To:        booking.GuestEmail,
		Subject:   l.t("email.cancelled.subject", booking.MeetingTypeTitle),
		Heading:   l.t("email.heading.cancelled"),
		Tone:      "clay",
		Intro:     l.t("email.cancelled.intro", booking.MeetingTypeTitle),
		Location:  mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone)),
		ManageURL: manageURL,
		Reason:    reason,
	}))
}

func (b base) sendOwnerCancelled(r *http.Request, booking models.Booking, reason string) {
	l := b.localeFor(r)
	b.send(r, b.bookingEmailMessage(r, l, booking, bookingEmail{
		Kind:     "booking.owner_cancelled",
		To:       booking.OwnerEmail,
		Subject:  l.t("email.owner_cancelled.subject", booking.GuestName),
		Heading:  l.t("email.heading.owner_cancelled"),
		Tone:     "clay",
		Intro:    l.t("email.owner_cancelled.intro", booking.GuestName, booking.MeetingTypeTitle),
		Location: mustLocation(booking.OwnerTimezone),
		CTAURL:   b.absoluteURL(r, "/dashboard"),
		CTALabel: l.t("email.cta.dashboard"),
		Reason:   reason,
	}))
}

func (b base) bookingEmailMessage(r *http.Request, l localeText, booking models.Booking, email bookingEmail) notify.Message {
	loc := email.Location
	if loc == nil {
		loc = mustLocation(booking.OwnerTimezone)
	}
	reason := strings.TrimSpace(email.Reason)
	when := l.longDate(booking.StartUTC, loc)
	clockRange := l.clockRange(booking.StartUTC, booking.EndUTC, loc)
	zone := formatTimezone(booking.StartUTC, loc)

	lines := []string{email.Intro, "", when + " · " + clockRange + " (" + zone + ")"}
	if email.CTAURL != "" {
		lines = append(lines, "", email.CTALabel+": "+email.CTAURL)
	}
	if email.ManageURL != "" {
		lines = append(lines, l.t("email.cta.manage")+": "+email.ManageURL)
	}
	if email.GuestEmail != "" {
		lines = append(lines, "", l.t("email.guest")+":", email.GuestEmail)
		if email.GuestWhatsApp != "" {
			lines = append(lines, l.t("confirm.whatsapp")+": "+email.GuestWhatsApp)
		}
		if notes := strings.TrimSpace(email.GuestNotes); notes != "" {
			lines = append(lines, l.t("email.message")+": "+notes)
		}
	} else if email.WhatsApp != "" {
		lines = append(lines, l.t("confirm.whatsapp")+": "+email.WhatsApp)
	}
	if reason != "" {
		lines = append(lines, "", l.t("email.reason")+": "+reason)
	}

	html := renderEmail(emailView{
		Lang:          l.tag,
		Eyebrow:       booking.MeetingTypeTitle,
		Heading:       email.Heading,
		Tone:          email.Tone,
		Intro:         email.Intro,
		When:          when,
		Range:         clockRange,
		Zone:          zone,
		ReasonLabel:   l.t("email.reason"),
		Reason:        reason,
		GuestName:     booking.GuestName,
		GuestEmail:    email.GuestEmail,
		GuestWhatsApp: email.GuestWhatsApp,
		GuestNotes:    strings.TrimSpace(email.GuestNotes),
		GuestLabel:    l.t("email.guest"),
		EmailLabel:    l.t("confirm.email"),
		WhatsAppLabel: l.t("confirm.whatsapp"),
		MessageLabel:  l.t("email.message"),
		CTAURL:        email.CTAURL,
		CTALabel:      email.CTALabel,
		ManageURL:     email.ManageURL,
		ManageLabel:   l.t("email.cta.manage"),
		Footer:        l.t("layout.footer"),
		FooterNote:    l.t("layout.footer_note"),
	})

	return notify.Message{
		Kind:    email.Kind,
		To:      email.To,
		Subject: email.Subject,
		Body:    strings.Join(lines, "\n"),
		HTML:    html,
	}
}

// joinURL is the signed route, never the private room.
func joinURL(manageURL, roomURL string) string {
	if roomURL == "" || manageURL == "" {
		return ""
	}
	return strings.TrimRight(manageURL, "/") + "/join"
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
