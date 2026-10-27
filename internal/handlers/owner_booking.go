package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/ics"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
	"github.com/puppe1990/slot/internal/store"
)

type filterOption struct {
	Value    string
	LabelKey string
	Selected bool
}

// dashboardFilter is the owner's view of the list: which statuses and how wide
// the window. It is parsed from GET params so a filtered list is a shareable
// URL and the export reuses exactly the same query.
type dashboardFilter struct {
	Status string
	Days   int
}

func parseDashboardFilter(r *http.Request) dashboardFilter {
	filter := dashboardFilter{Status: "upcoming", Days: 90}
	switch r.URL.Query().Get("status") {
	case "past":
		filter.Status = "past"
	case "cancelled":
		filter.Status = "cancelled"
	case "all":
		filter.Status = "all"
	}
	switch r.URL.Query().Get("days") {
	case "7":
		filter.Days = 7
	case "30":
		filter.Days = 30
	}
	return filter
}

func (f dashboardFilter) storeFilter(now time.Time) store.BookingFilter {
	active := []models.BookingStatus{models.BookingStatusConfirmed, models.BookingStatusRescheduled}
	switch f.Status {
	case "past":
		return store.BookingFilter{
			Statuses: append(active, models.BookingStatusCancelled),
			From:     now.AddDate(0, 0, -f.Days),
			To:       now,
			Limit:    200,
		}
	case "cancelled":
		return store.BookingFilter{
			Statuses: []models.BookingStatus{models.BookingStatusCancelled},
			From:     now.AddDate(0, 0, -f.Days),
			Limit:    200,
		}
	case "all":
		return store.BookingFilter{
			Statuses: append(active, models.BookingStatusCancelled),
			From:     now.AddDate(0, 0, -f.Days),
			To:       now.AddDate(0, 0, f.Days),
			Limit:    200,
		}
	default:
		return store.BookingFilter{
			Statuses: active,
			From:     now,
			To:       now.AddDate(0, 0, f.Days),
			Limit:    200,
		}
	}
}

func (f dashboardFilter) statusOptions() []filterOption {
	options := []filterOption{
		{Value: "upcoming", LabelKey: "dashboard.status_upcoming"},
		{Value: "past", LabelKey: "dashboard.status_past"},
		{Value: "cancelled", LabelKey: "dashboard.status_cancelled"},
		{Value: "all", LabelKey: "dashboard.status_all"},
	}
	for i := range options {
		options[i].Selected = options[i].Value == f.Status
	}
	return options
}

func (f dashboardFilter) dayOptions() []filterOption {
	options := []filterOption{
		{Value: "7", LabelKey: "dashboard.days_7"},
		{Value: "30", LabelKey: "dashboard.days_30"},
		{Value: "90", LabelKey: "dashboard.days_90"},
	}
	for i := range options {
		options[i].Selected = options[i].Value == strconv.Itoa(f.Days)
	}
	return options
}

// Invite serves one booking as a calendar file for the owner.
func (h *DashboardHandler) Invite(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	booking, ok := h.ownerBooking(w, r, owner.User.ID)
	if !ok {
		return
	}
	h.writeCalendar(w, h.localeFor(r), []models.Booking{booking}, "slot-booking.ics")
}

// Export downloads the current filter as a single VCALENDAR, so the owner can
// import the week into Google/Apple without one file per booking.
func (h *DashboardHandler) Export(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	filter := parseDashboardFilter(r)
	bookings, err := h.deps.Store.ListBookings(owner.User.ID, filter.storeFilter(h.now()))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeCalendar(w, h.localeFor(r), bookings, "slot-bookings.ics")
}

func (h *DashboardHandler) writeCalendar(w http.ResponseWriter, l localeText, bookings []models.Booking, filename string) {
	events := make([]ics.Event, 0, len(bookings))
	for _, booking := range bookings {
		if booking.Status == models.BookingStatusCancelled {
			continue
		}
		events = append(events, ics.Event{
			UID:            "booking-" + strconv.FormatInt(booking.ID, 10) + "@slot",
			Start:          booking.StartUTC,
			End:            booking.EndUTC,
			Summary:        booking.MeetingTypeTitle + " · " + booking.GuestName,
			Description:    bookingNotes(booking),
			Location:       meetingLocation(booking, l),
			OrganizerName:  booking.OwnerName,
			OrganizerEmail: booking.OwnerEmail,
			AttendeeName:   booking.GuestName,
			AttendeeEmail:  booking.GuestEmail,
		})
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ics.BuildCalendar(events))
}

func (h *DashboardHandler) ReschedulePage(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	booking, ok := h.ownerBooking(w, r, owner.User.ID)
	if !ok {
		return
	}
	h.renderReschedule(w, r, owner.User.ID, booking, nil, 0)
}

func (h *DashboardHandler) Reschedule(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	booking, ok := h.ownerBooking(w, r, owner.User.ID)
	if !ok {
		return
	}
	l := h.localeFor(r)
	if !booking.Active() {
		h.renderReschedule(w, r, owner.User.ID, booking,
			validate.FieldErrors{"slot": l.t("owner_reschedule.cancelled")}, http.StatusConflict)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start, err := time.Parse(time.RFC3339, strings.TrimSpace(r.FormValue("slot")))
	if err != nil {
		h.renderReschedule(w, r, owner.User.ID, booking,
			validate.FieldErrors{"slot": l.t("owner_reschedule.error")}, http.StatusUnprocessableEntity)
		return
	}
	end := start.Add(time.Duration(booking.MeetingTypeDuration) * time.Minute)
	if err := h.deps.Store.RescheduleBooking(booking.ID, start.UTC(), end.UTC(), models.ActorOwner, "Owner rescheduled", h.now()); err != nil {
		if errors.Is(err, store.ErrSlotUnavailable) {
			h.renderReschedule(w, r, owner.User.ID, booking,
				validate.FieldErrors{"slot": l.t("owner_reschedule.unavailable")}, http.StatusConflict)
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
	if booking.GuestEmail != "" {
		_ = h.deps.Notifier.Send(r.Context(), notify.Message{
			Kind:    "booking.changed",
			To:      booking.GuestEmail,
			Subject: l.t("owner_reschedule.subject") + ": " + booking.MeetingTypeTitle,
			Body:    l.t("owner_reschedule.body", start.UTC().Format(time.RFC1123)),
		})
	}
	flash.Set(w, "notice", l.t("owner_reschedule.saved"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *DashboardHandler) renderReschedule(w http.ResponseWriter, r *http.Request, ownerID int64, booking models.Booking, errs validate.FieldErrors, status int) {
	meetingType, err := h.deps.Store.GetMeetingTypeBySlug(booking.MeetingTypeSlug)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	l := h.localeFor(r)
	guestTimezone := normalizeTimezone(r.URL.Query().Get("tz"), booking.GuestTimezone)
	slots, err := h.buildSlots(r.Context(), meetingType, guestTimezone, r.URL.Query().Get("date"), booking.ID, l)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.page(w, r, "app", "booking_reschedule", map[string]any{
		"Title":        l.t("owner_reschedule.eyebrow"),
		"ActiveNav":    "dashboard",
		"Owner":        models.User{Name: booking.OwnerName, Email: booking.OwnerEmail, Timezone: booking.OwnerTimezone},
		"Booking":      presentBooking(booking, mustLocation(booking.OwnerTimezone), mustLocation(guestTimezone), h.now(), l),
		"MeetingType":  presentMeetingType(meetingType, l),
		"Slots":        slots,
		"Errors":       errs,
		"GuestTZ":      guestTimezone,
		"GuestOptions": buildTimezoneOptions(guestTimezone),
		"FormAction":   "/bookings/" + strconv.FormatInt(booking.ID, 10) + "/reschedule",
		"PagePath":     "/bookings/" + strconv.FormatInt(booking.ID, 10) + "/reschedule",
	}, status)
}

func (h *DashboardHandler) ownerBooking(w http.ResponseWriter, r *http.Request, ownerID int64) (models.Booking, bool) {
	id, ok := intParam(r, "id")
	if !ok {
		http.NotFound(w, r)
		return models.Booking{}, false
	}
	booking, err := h.deps.Store.GetBooking(ownerID, id)
	if err != nil {
		http.NotFound(w, r)
		return models.Booking{}, false
	}
	return booking, true
}

// filterFormURL keeps the export link in sync with the visible filter.
func filterExportURL(filter dashboardFilter) string {
	values := url.Values{"status": {filter.Status}, "days": {strconv.Itoa(filter.Days)}}
	return "/dashboard/export.ics?" + values.Encode()
}
