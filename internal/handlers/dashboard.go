package handlers

import (
	"net/http"

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
