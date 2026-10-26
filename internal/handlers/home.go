package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

type HomeHandler struct {
	base
}

func NewHomeHandler(deps Deps) *HomeHandler {
	return &HomeHandler{base: newBase(deps)}
}

// "/" is a router, not a page: the owner goes to the dashboard and a visitor
// lands directly on the first published format.
func (h *HomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := session.UserID(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	meetingType, err := h.deps.Store.DefaultMeetingType()
	if err == nil && meetingType.Slug != "" {
		http.Redirect(w, r, "/book/"+meetingType.Slug, http.StatusSeeOther)
		return
	}
	h.page(w, r, "booking", "landing", map[string]any{
		"Title": "Slot",
	}, 0)
}
