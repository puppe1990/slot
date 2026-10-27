package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/call"
	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/ics"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

type ManageHandler struct {
	base
}

func NewManageHandler(deps Deps) *ManageHandler {
	return &ManageHandler{base: newBase(deps)}
}

func (h *ManageHandler) Show(w http.ResponseWriter, r *http.Request) {
	booking, ok := h.load(w, r)
	if !ok {
		return
	}
	h.renderManage(w, r, booking, nil, 0)
}

func (h *ManageHandler) Reschedule(w http.ResponseWriter, r *http.Request) {
	booking, ok := h.load(w, r)
	if !ok {
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !booking.Active() {
		h.renderManage(w, r, booking, validate.FieldErrors{
			"slot": h.localeFor(r).t("manage.error_cancelled"),
		}, http.StatusConflict)
		return
	}

	start, err := time.Parse(time.RFC3339, strings.TrimSpace(r.FormValue("slot")))
	if err != nil {
		h.renderManage(w, r, booking, validate.FieldErrors{"slot": h.localeFor(r).t("manage.error_pick")}, http.StatusUnprocessableEntity)
		return
	}
	end := start.Add(time.Duration(booking.MeetingTypeDuration) * time.Minute)
	if err := h.deps.Store.RescheduleBooking(booking.ID, start.UTC(), end.UTC(), models.ActorGuest, "Guest rescheduled", h.now()); err != nil {
		if errors.Is(err, store.ErrSlotUnavailable) {
			h.renderManage(w, r, booking, validate.FieldErrors{
				"slot": h.localeFor(r).t("manage.error_taken"),
			}, http.StatusConflict)
			return
		}
		h.serverError(w, r, err)
		return
	}

	if booking.CalendarEventID != "" {
		if err := h.deps.Calendar.UpdateEvent(r.Context(), booking.CalendarEventID, start.UTC(), end.UTC()); err != nil {
			log.Printf("slot: update calendar event for booking %d: %v", booking.ID, err)
		}
	}
	h.sendMoved(r, booking, h.absoluteURL(r, r.URL.Path))
	h.sendOwnerMoved(r, booking)
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}

func (h *ManageHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	booking, ok := h.load(w, r)
	if !ok {
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if err := h.deps.Store.CancelBooking(booking.ID, models.ActorGuest, reason); err != nil {
		if errors.Is(err, store.ErrBookingNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, r, err)
		return
	}
	if booking.CalendarEventID != "" {
		if err := h.deps.Calendar.DeleteEvent(r.Context(), booking.CalendarEventID); err != nil {
			log.Printf("slot: delete calendar event for booking %d: %v", booking.ID, err)
		}
	}
	if booking.MeetingURL != "" {
		if err := h.deps.Call.EndRoom(r.Context(), call.Room{URL: booking.MeetingURL}); err != nil {
			log.Printf("slot: end room for booking %d: %v", booking.ID, err)
		}
	}
	h.sendCancelled(r, booking, h.absoluteURL(r, r.URL.Path), reason)
	h.sendOwnerCancelled(r, booking, reason)
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}

// Join issues a meeting token for the guest and redirects into the room. The
// email only carries this page's signed link, so a forwarded invite cannot be
// used to walk into someone else's call.
func (h *ManageHandler) Join(w http.ResponseWriter, r *http.Request) {
	booking, ok := h.load(w, r)
	if !ok {
		return
	}
	if booking.MeetingURL == "" {
		flash.Set(w, "error", h.localeFor(r).t("call.unavailable"), h.deps.Config.CookieSecure())
		http.Redirect(w, r, managePathFrom(r, "/join"), http.StatusSeeOther)
		return
	}
	roomURL, err := h.deps.Call.RoomURL(r.Context(), call.Room{URL: booking.MeetingURL}, call.Viewer{
		Name:      booking.GuestName,
		ExpiresAt: booking.EndUTC.Add(30 * time.Minute),
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if roomURL == "" {
		flash.Set(w, "error", h.localeFor(r).t("call.unavailable"), h.deps.Config.CookieSecure())
		http.Redirect(w, r, managePathFrom(r, "/join"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, roomURL, http.StatusSeeOther)
}

// RSVP is a POST on purpose: a GET in an email body gets prefetched by mail
// scanners, which would confirm attendance without the guest.
func (h *ManageHandler) RSVP(w http.ResponseWriter, r *http.Request) {
	booking, ok := h.load(w, r)
	if !ok {
		return
	}
	l := h.localeFor(r)
	if !booking.Active() {
		h.renderManage(w, r, booking, validate.FieldErrors{"slot": l.t("manage.error_cancelled")}, http.StatusConflict)
		return
	}
	if err := h.deps.Store.SetAttendance(booking.ID, models.AttendanceGoing); err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.deps.Store.RecordAudit(models.AuditLog{
		BookingID: booking.ID,
		Action:    "ATTENDANCE_CONFIRMED",
		Actor:     models.ActorGuest,
	}); err != nil {
		log.Printf("slot: audit rsvp for booking %d: %v", booking.ID, err)
	}
	flash.Set(w, "notice", l.t("rsvp.flash"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, managePathFrom(r, "/rsvp"), http.StatusSeeOther)
}

func (h *ManageHandler) Invite(w http.ResponseWriter, r *http.Request) {
	l := h.localeFor(r)
	booking, ok := h.load(w, r)
	if !ok {
		return
	}
	document := ics.Build(ics.Event{
		UID:            "booking-" + strconv.FormatInt(booking.ID, 10) + "@slot",
		Start:          booking.StartUTC,
		End:            booking.EndUTC,
		Summary:        booking.MeetingTypeTitle + " · " + booking.OwnerName,
		Description:    bookingNotes(booking),
		Location:       meetingLocation(booking, l),
		URL:            h.absoluteURL(r, strings.TrimSuffix(r.URL.Path, "/invite.ics")+"/join"),
		OrganizerName:  booking.OwnerName,
		OrganizerEmail: booking.OwnerEmail,
		AttendeeName:   booking.GuestName,
		AttendeeEmail:  booking.GuestEmail,
	})
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="slot-booking.ics"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

// managePathFrom strips the action suffix so an error or a successful action
// lands on the page that owns it instead of re-running the action.
func managePathFrom(r *http.Request, suffix string) string {
	return strings.TrimSuffix(r.URL.Path, suffix)
}

func (h *ManageHandler) renderManage(w http.ResponseWriter, r *http.Request, booking models.Booking, errs validate.FieldErrors, status int) {
	meetingType, err := h.deps.Store.GetMeetingTypeBySlug(booking.MeetingTypeSlug)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	l := h.localeFor(r)
	ownerLocation := mustLocation(booking.OwnerTimezone)
	guestTimezone := normalizeTimezone(r.URL.Query().Get("tz"), booking.GuestTimezone)
	guestLocation := mustLocation(guestTimezone)

	var slots slotPageData
	if booking.Active() {
		slots, err = h.buildSlots(r.Context(), meetingType, guestTimezone, r.URL.Query().Get("date"), booking.ID, l)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
	}

	h.page(w, r, "booking", "manage", map[string]any{
		"Title":        "Your booking",
		"Owner":        models.User{Name: booking.OwnerName, Email: booking.OwnerEmail, Timezone: booking.OwnerTimezone},
		"Booking":      presentBooking(booking, ownerLocation, guestLocation, h.now(), l),
		"MeetingType":  presentMeetingType(meetingType, l),
		"Slots":        slots,
		"Errors":       errs,
		"ManagePath":   r.URL.Path,
		"ICSURL":       r.URL.Path + "/invite.ics",
		"GuestTZ":      guestTimezone,
		"GuestOptions": buildTimezoneOptions(guestTimezone),
		"GuestRange":   l.clockRange(booking.StartUTC, booking.EndUTC, guestLocation),
		"HostRange":    l.clockRange(booking.StartUTC, booking.EndUTC, ownerLocation),
		"GuestLabel":   formatTimezone(booking.StartUTC, guestLocation),
		"HostLabel":    formatTimezone(booking.StartUTC, ownerLocation),
		"DateLabel":    l.longDate(booking.StartUTC, guestLocation),
	}, status)
}

// load resolves the signed token twice: the signature proves it was issued by
// this app, and the hash lookup proves it still belongs to a live booking.
func (h *ManageHandler) load(w http.ResponseWriter, r *http.Request) (models.Booking, bool) {
	token := r.PathValue("token")
	bookingID, err := h.deps.Signer.Parse(token)
	if err != nil {
		http.NotFound(w, r)
		return models.Booking{}, false
	}
	booking, err := h.deps.Store.GetBookingByGuestTokenHash(guesttoken.Hash(token))
	if err != nil || booking.ID != bookingID {
		http.NotFound(w, r)
		return models.Booking{}, false
	}
	return booking, true
}
