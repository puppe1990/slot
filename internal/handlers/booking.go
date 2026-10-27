package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/calendar"
	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
	"github.com/puppe1990/slot/internal/store"
)

const holdCookieName = "slot_hold"

type BookingHandler struct {
	base
}

func NewBookingHandler(deps Deps) *BookingHandler {
	return &BookingHandler{base: newBase(deps)}
}

func (h *BookingHandler) Show(w http.ResponseWriter, r *http.Request) {
	meetingType, ok := h.meetingType(w, r)
	if !ok {
		return
	}
	h.renderBooking(w, r, meetingType, nil, 0)
}

// Hold turns a chosen start into a ten-minute reservation and parks the raw
// token in an HttpOnly cookie: the token never lands in a URL, referrer or log.
func (h *BookingHandler) Hold(w http.ResponseWriter, r *http.Request) {
	meetingType, ok := h.meetingType(w, r)
	if !ok {
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start, err := time.Parse(time.RFC3339, strings.TrimSpace(r.FormValue("slot")))
	if err != nil {
		h.renderBooking(w, r, meetingType, validate.FieldErrors{"slot": h.localeFor(r).t("book.error_slot")}, http.StatusUnprocessableEntity)
		return
	}
	owner, err := h.deps.Store.GetUser(meetingType.UserID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	guestTimezone := normalizeTimezone(r.FormValue("tz"), owner.Timezone)

	now := h.now()
	_, holdToken, err := h.deps.Store.CreateHold(store.HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         start.UTC(),
		End:           start.UTC().Add(time.Duration(meetingType.DurationMinutes) * time.Minute),
		GuestTimezone: guestTimezone,
		ExpiresAt:     now.Add(10 * time.Minute),
		Now:           now,
	})
	if err != nil {
		if errors.Is(err, store.ErrSlotUnavailable) {
			h.renderBooking(w, r, meetingType, validate.FieldErrors{
				"slot": h.localeFor(r).t("book.error_taken"),
			}, http.StatusConflict)
			return
		}
		h.serverError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     holdCookieName,
		Value:    holdToken,
		Path:     "/book",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.deps.Config.CookieSecure(),
		MaxAge:   int((10 * time.Minute).Seconds()),
	})
	http.Redirect(w, r, "/book/"+meetingType.Slug+"/confirm?tz="+guestTimezone, http.StatusSeeOther)
}

// Release deletes the hold so the slot returns to the pool right away instead
// of waiting out the ten-minute TTL.
func (h *BookingHandler) Release(w http.ResponseWriter, r *http.Request) {
	meetingType, ok := h.meetingType(w, r)
	if !ok {
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if token := holdTokenFromCookie(r); token != "" {
		if err := h.deps.Store.ReleaseHold(token); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.clearHoldCookie(w)
	target := "/book/" + meetingType.Slug
	if guestTimezone := strings.TrimSpace(r.FormValue("tz")); guestTimezone != "" {
		target += "?tz=" + guestTimezone
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *BookingHandler) ConfirmPage(w http.ResponseWriter, r *http.Request) {
	meetingType, ok := h.meetingType(w, r)
	if !ok {
		return
	}
	hold, ok := h.liveHold(w, r, meetingType)
	if !ok {
		return
	}
	h.renderConfirm(w, r, meetingType, hold, nil, 0)
}

func (h *BookingHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	meetingType, ok := h.meetingType(w, r)
	if !ok {
		return
	}
	hold, ok := h.liveHold(w, r, meetingType)
	if !ok {
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	guestTimezone := normalizeTimezone(r.FormValue("tz"), hold.GuestTimezone)
	l := h.localeFor(r)

	var errs validate.FieldErrors
	if err := validate.Required(map[string]string{"name": name}, "name"); err != nil {
		errs.Add("name", l.t("confirm.error_name"))
	}
	if err := validate.Email(email); err != nil {
		errs.Add("email", l.t("confirm.error_email"))
	}
	if errs.Any() {
		h.renderConfirm(w, r, meetingType, hold, errs, http.StatusUnprocessableEntity)
		return
	}

	guestToken := h.deps.Signer.Sign(hold.ID)
	booking, err := h.deps.Store.ConfirmHold(holdTokenFromCookie(r), store.GuestInput{
		Name:     name,
		Email:    email,
		Notes:    notes,
		Timezone: guestTimezone,
	}, guesttoken.Hash(guestToken), h.now())
	if err != nil {
		if errors.Is(err, store.ErrHoldExpired) || errors.Is(err, store.ErrSlotUnavailable) {
			h.clearHoldCookie(w)
			h.renderBooking(w, r, meetingType, validate.FieldErrors{
				"slot": h.localeFor(r).t("book.error_expired"),
			}, http.StatusConflict)
			return
		}
		h.serverError(w, r, err)
		return
	}

	event, err := h.deps.Calendar.CreateEvent(r.Context(), calendar.Event{
		Summary:   booking.MeetingTypeTitle + " · " + name,
		Details:   bookingNotes(booking),
		Start:     booking.StartUTC,
		End:       booking.EndUTC,
		Attendees: []string{email, booking.OwnerEmail},
		Location:  meetingLocation(booking, h.localeFor(r)),
	})
	if err != nil {
		// The slot is already committed; a calendar hiccup must not lose the
		// guest's confirmation. The owner still sees the booking in the app.
		log.Printf("slot: create calendar event for booking %d: %v", booking.ID, err)
	} else if err := h.deps.Store.AttachCalendarEvent(booking.ID, event.ID); err != nil {
		log.Printf("slot: attach calendar event %s to booking %d: %v", event.ID, booking.ID, err)
	}

	manageURL := h.absoluteURL(r, "/manage/"+guestToken)
	h.notifyGuest(r, booking, manageURL)
	h.notifyOwner(r, booking, manageURL)

	h.clearHoldCookie(w)
	http.Redirect(w, r, "/manage/"+guestToken, http.StatusSeeOther)
}

func (h *BookingHandler) renderBooking(w http.ResponseWriter, r *http.Request, meetingType models.MeetingType, errs validate.FieldErrors, status int) {
	guestTimezone := r.URL.Query().Get("tz")
	if guestTimezone == "" {
		guestTimezone = r.FormValue("tz")
	}
	l := h.localeFor(r)
	data, err := h.buildSlots(r.Context(), meetingType, guestTimezone, r.URL.Query().Get("date"), 0, l)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	ownerConnection, _ := h.deps.Store.GetCalendarConnection(meetingType.UserID)
	h.page(w, r, "booking", "book", map[string]any{
		"Title":        meetingType.Title,
		"MeetingType":  presentMeetingType(meetingType, l),
		"Owner":        data.Owner,
		"Slots":        data,
		"Errors":       errs,
		"Connection":   ownerConnection,
		"PublicPath":   "/book/" + meetingType.Slug,
		"Timezone":     data.GuestTimezone,
		"GuestOptions": data.TimezoneOptions,
	}, status)
}

func (h *BookingHandler) renderConfirm(w http.ResponseWriter, r *http.Request, meetingType models.MeetingType, hold models.Booking, errs validate.FieldErrors, status int) {
	l := h.localeFor(r)
	ownerLocation := mustLocation(hold.OwnerTimezone)
	guestTimezone := normalizeTimezone(r.URL.Query().Get("tz"), hold.GuestTimezone)
	guestLocation := mustLocation(guestTimezone)
	remaining := int(hold.HoldExpiresAt.Sub(h.now()).Seconds())
	if remaining < 0 {
		remaining = 0
	}

	h.page(w, r, "booking", "book_confirm", map[string]any{
		"Title":       "Confirm · " + meetingType.Title,
		"MeetingType": presentMeetingType(meetingType, l),
		"Owner":       models.User{Name: hold.OwnerName, Email: hold.OwnerEmail, Timezone: hold.OwnerTimezone},
		"Hold":        presentBooking(hold, ownerLocation, guestLocation, h.now(), l),
		"Errors":      errs,
		"GuestTZ":     guestTimezone,
		"GuestRange":  l.clockRange(hold.StartUTC, hold.EndUTC, guestLocation),
		"HostRange":   l.clockRange(hold.StartUTC, hold.EndUTC, ownerLocation),
		"GuestLabel":  formatTimezone(hold.StartUTC, guestLocation),
		"HostLabel":   formatTimezone(hold.StartUTC, ownerLocation),
		"DateLabel":   l.longDate(hold.StartUTC, guestLocation),
		"Remaining":   remaining,
		"PublicPath":  "/book/" + meetingType.Slug,
	}, status)
}

func (h *BookingHandler) meetingType(w http.ResponseWriter, r *http.Request) (models.MeetingType, bool) {
	slug := r.PathValue("slug")
	meetingType, err := h.deps.Store.GetMeetingTypeBySlug(slug)
	if err != nil || !meetingType.IsActive {
		http.NotFound(w, r)
		return models.MeetingType{}, false
	}
	return meetingType, true
}

func (h *BookingHandler) liveHold(w http.ResponseWriter, r *http.Request, meetingType models.MeetingType) (models.Booking, bool) {
	token := holdTokenFromCookie(r)
	if token == "" {
		http.Redirect(w, r, "/book/"+meetingType.Slug, http.StatusSeeOther)
		return models.Booking{}, false
	}
	hold, err := h.deps.Store.GetHold(token, h.now())
	if err != nil || hold.MeetingTypeSlug != meetingType.Slug {
		h.clearHoldCookie(w)
		h.renderBooking(w, r, meetingType, validate.FieldErrors{
			"slot": h.localeFor(r).t("book.error_expired"),
		}, http.StatusConflict)
		return models.Booking{}, false
	}
	return hold, true
}

func (h *BookingHandler) clearHoldCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     holdCookieName,
		Value:    "",
		Path:     "/book",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.deps.Config.CookieSecure(),
		MaxAge:   -1,
	})
}

func holdTokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(holdCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func bookingNotes(booking models.Booking) string {
	notes := "Booked through Slot."
	if booking.GuestNotes != "" {
		notes += "\n\nGuest notes: " + booking.GuestNotes
	}
	return notes
}

func meetingLocation(booking models.Booking, l localeText) string {
	return l.locationLabel(booking.MeetingTypeLocation)
}

func (h *BookingHandler) notifyGuest(r *http.Request, booking models.Booking, manageURL string) {
	if booking.GuestEmail == "" {
		return
	}
	_ = h.deps.Notifier.Send(r.Context(), notify.Message{
		Kind:    "booking.confirmed",
		To:      booking.GuestEmail,
		Subject: "Confirmed: " + booking.MeetingTypeTitle,
		Body: "You are booked for " + booking.MeetingTypeTitle + " with " +
			booking.OwnerName + " on " + booking.StartUTC.Format(time.RFC1123) + ".\n\n" +
			"Reschedule or cancel: " + manageURL,
	})
}

func (h *BookingHandler) notifyOwner(r *http.Request, booking models.Booking, manageURL string) {
	if booking.OwnerEmail == "" {
		return
	}
	_ = h.deps.Notifier.Send(r.Context(), notify.Message{
		Kind:    "booking.owner_notice",
		To:      booking.OwnerEmail,
		Subject: "New booking: " + booking.GuestName,
		Body: booking.GuestName + " (" + booking.GuestEmail + ") booked " +
			booking.MeetingTypeTitle + " on " + booking.StartUTC.Format(time.RFC1123) + ".\n\n" +
			"Guest management link: " + manageURL,
	})
}
