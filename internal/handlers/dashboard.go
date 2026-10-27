package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"

	"github.com/puppe1990/slot/internal/call"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

type DashboardHandler struct {
	base
}

func NewDashboardHandler(deps Deps) *DashboardHandler {
	return &DashboardHandler{base: newBase(deps)}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	now := h.now()
	filter := parseDashboardFilter(r)
	bookings, err := h.deps.Store.ListBookings(owner.User.ID, filter.storeFilter(now))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	// KPIs stay global: the filter changes the list, not the desk summary.
	stats, err := h.deps.Store.ListBookings(owner.User.ID, store.BookingFilter{
		Statuses: []models.BookingStatus{models.BookingStatusConfirmed, models.BookingStatusRescheduled},
		From:     now,
		To:       now.AddDate(0, 0, 90),
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	types, err := h.deps.Store.ListMeetingTypes(owner.User.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	l := h.localeFor(r)
	views := make([]bookingView, 0, len(bookings))
	for _, booking := range bookings {
		views = append(views, presentBooking(booking, owner.Location, owner.Location, now, l))
	}

	var minutes int
	var next *bookingView
	upcomingCount := 0
	for _, booking := range stats {
		minutes += booking.MeetingTypeDuration
		upcomingCount++
		if next == nil {
			candidate := presentBooking(booking, owner.Location, owner.Location, now, l)
			next = &candidate
		}
	}

	published := firstActiveMeetingType(types)
	data := map[string]any{
		"Title":          "Dashboard",
		"ActiveNav":      "dashboard",
		"Owner":          owner.User,
		"Bookings":       views,
		"ListCount":      len(views),
		"MeetingTypes":   presentMeetingTypes(types, l),
		"UpcomingCount":  upcomingCount,
		"UpcomingHours":  l.duration(minutes),
		"NextBooking":    next,
		"PublicPath":     publicPath(published),
		"HasMeetingType": published.ID != 0,
		"Filter":         filter,
		"StatusOptions":  filter.statusOptions(),
		"DayOptions":     filter.dayOptions(),
		"ExportURL":      filterExportURL(filter),
		"Now":            now,
	}
	h.page(w, r, "app", "dashboard", data, 0)
}

// Cancel is the owner-side kill switch the guest link does not cover: the
// owner can drop a booking from the dashboard without opening the guest token.
func (h *DashboardHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	bookingID, ok := intParam(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	booking, err := h.deps.Store.GetBooking(owner.User.ID, bookingID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if err := h.deps.Store.CancelBooking(booking.ID, models.ActorOwner, reason); err != nil {
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
	h.sendCancelled(r, booking, "", reason)
	h.sendOwnerCancelled(r, booking, reason)
	flash.Set(w, "notice", h.localeFor(r).t("dashboard.cancelled_flash"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func firstActiveMeetingType(types []models.MeetingType) models.MeetingType {
	for _, meetingType := range types {
		if meetingType.IsActive {
			return meetingType
		}
	}
	return models.MeetingType{}
}

func presentMeetingTypes(types []models.MeetingType, l localeText) []meetingTypeView {
	out := make([]meetingTypeView, 0, len(types))
	for _, meetingType := range types {
		out = append(out, presentMeetingType(meetingType, l))
	}
	return out
}

func publicPath(meetingType models.MeetingType) string {
	if meetingType.Slug == "" {
		return ""
	}
	return "/book/" + meetingType.Slug
}
