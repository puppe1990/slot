package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/call"
	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/ics"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
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
	h.notifyBoth(r, booking)
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
	h.notifyBoth(r, booking)
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}

func (h *ManageHandler) Invite(w http.ResponseWriter, r *http.Request) {
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
		Location:       meetingLocation(booking, h.localeFor(r)),
		URL:            booking.MeetingURL,
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

func (h *ManageHandler) notifyBoth(r *http.Request, booking models.Booking) {
	manageURL := strings.TrimRight(h.deps.Site.AppURL, "/") + r.URL.Path
	if booking.GuestEmail != "" {
		_ = h.deps.Notifier.Send(r.Context(), notify.Message{
			Kind:    "booking.changed",
			To:      booking.GuestEmail,
			Subject: booking.MeetingTypeTitle + " updated",
			Body:    booking.MeetingTypeTitle + " · " + manageURL,
		})
	}
	if booking.OwnerEmail != "" {
		_ = h.deps.Notifier.Send(r.Context(), notify.Message{
			Kind:    "booking.owner_notice",
			To:      booking.OwnerEmail,
			Subject: "Booking changed: " + booking.GuestName,
			Body:    booking.MeetingTypeTitle + " · " + manageURL,
		})
	}
}
