package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

type NotFoundHandler struct {
	base
}

func NewNotFoundHandler(deps Deps) *NotFoundHandler {
	return &NotFoundHandler{base: newBase(deps)}
}

// ServeHTTP owns both a missing route and a path param that failed to parse.
// The layout follows the session so an owner keeps the app chrome.
func (h *NotFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	layout := "booking"
	if _, ok := session.UserID(r); ok {
		layout = "app"
	}
	h.page(w, r, layout, "not_found", map[string]any{
		"Title": "Not found",
		"Path":  r.URL.Path,
	}, http.StatusNotFound)
}
