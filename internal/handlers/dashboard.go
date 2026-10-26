package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
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
	upcoming, err := h.deps.Store.ListBookings(owner.User.ID, store.BookingFilter{
		From:  now,
		To:    now.AddDate(0, 0, 90),
		Limit: 60,
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

	views := make([]bookingView, 0, len(upcoming))
	var minutes int
	var next *bookingView
	for _, booking := range upcoming {
		if booking.Status == models.BookingStatusCancelled {
			continue
		}
		view := presentBooking(booking, owner.Location, owner.Location, now)
		views = append(views, view)
		if view.IsUpcoming {
			minutes += booking.MeetingTypeDuration
			if next == nil {
				candidate := view
				next = &candidate
			}
		}
	}

	published := firstActiveMeetingType(types)
	data := map[string]any{
		"Title":          "Dashboard",
		"ActiveNav":      "dashboard",
		"Owner":          owner.User,
		"Bookings":       views,
		"MeetingTypes":   presentMeetingTypes(types),
		"UpcomingCount":  countUpcoming(views),
		"UpcomingHours":  formatDuration(minutes),
		"NextBooking":    next,
		"PublicPath":     publicPath(published),
		"HasMeetingType": published.ID != 0,
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
	if booking.GuestEmail != "" {
		_ = h.deps.Notifier.Send(r.Context(), notify.Message{
			Kind:    "booking.cancelled",
			To:      booking.GuestEmail,
			Subject: "Cancelled: " + booking.MeetingTypeTitle,
			Body:    "The host cancelled " + booking.MeetingTypeTitle + " that was set for " + booking.StartUTC.Format(time.RFC1123) + ".",
		})
	}
	flash.Set(w, "notice", "Booking cancelled and the guest was notified.", h.deps.Config.CookieSecure())
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

func presentMeetingTypes(types []models.MeetingType) []meetingTypeView {
	out := make([]meetingTypeView, 0, len(types))
	for _, meetingType := range types {
		out = append(out, presentMeetingType(meetingType))
	}
	return out
}

func countUpcoming(views []bookingView) int {
	count := 0
	for _, view := range views {
		if view.IsUpcoming {
			count++
		}
	}
	return count
}

func publicPath(meetingType models.MeetingType) string {
	if meetingType.Slug == "" {
		return ""
	}
	return "/book/" + meetingType.Slug
}
