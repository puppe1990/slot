package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/models"
)

// SettingsHandler owns the account itself: name, home timezone and password.
// Availability (hours, overrides, calendar) stays in /setup, so the two pages
// never edit the same field.
type SettingsHandler struct {
	base
}

func NewSettingsHandler(deps Deps) *SettingsHandler {
	return &SettingsHandler{base: newBase(deps)}
}

func (h *SettingsHandler) Show(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	h.render(w, r, owner, nil, 0)
}

func (h *SettingsHandler) Save(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	timezone := normalizeTimezone(r.FormValue("timezone"), owner.User.Timezone)
	if name == "" {
		h.render(w, r, owner, validate.FieldErrors{"name": h.localeFor(r).t("settings.error_name")}, http.StatusUnprocessableEntity)
		return
	}
	if err := h.deps.Store.UpdateOwnerProfile(owner.User.ID, name, timezone); err != nil {
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", h.localeFor(r).t("settings.saved"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *SettingsHandler) Password(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	l := h.localeFor(r)
	current := r.FormValue("current_password")
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirmation")

	var errs validate.FieldErrors
	if !session.VerifyPassword(owner.User.PasswordHash, current) {
		errs.Add("current_password", l.t("settings.password_wrong"))
	}
	if err := validate.MinLength(password, 8); err != nil {
		errs.Add("password", l.t("auth.password_too_short"))
	}
	if password != confirm {
		errs.Add("password_confirmation", l.t("auth.password_mismatch"))
	}
	if errs.Any() {
		h.render(w, r, owner, errs, http.StatusUnprocessableEntity)
		return
	}

	hash, err := session.HashPassword(password)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.deps.Store.UpdatePassword(owner.User.ID, hash); err != nil {
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", l.t("settings.password_saved"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *SettingsHandler) render(w http.ResponseWriter, r *http.Request, owner ownerUser, errs validate.FieldErrors, status int) {
	rules, err := h.deps.Store.ListAvailabilityRules(owner.User.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	now := h.now()
	h.page(w, r, "app", "settings", map[string]any{
		"Title":             "Settings",
		"ActiveNav":         "settings",
		"Owner":             owner.User,
		"TimezoneOptions":   buildTimezoneOptions(normalizeTimezone(owner.User.Timezone, "UTC")),
		"TimezoneAbbrevNow": zoneOffset(now, mustLocation(owner.User.Timezone)),
		"Errors":            errs,
		// A brand-new account has no rules yet: the timezone hint may preselect
		// the browser zone without overwriting a deliberate choice.
		"FirstVisit": len(rules) == 0,
	}, status)
}

type profileView struct {
	Name         string
	Email        string
	Timezone     string
	TimezoneHint string
}

func presentProfile(user models.User) profileView {
	return profileView{
		Name:         user.DisplayName(),
		Email:        user.Email,
		Timezone:     user.Timezone,
		TimezoneHint: formatTimezone(time.Now().UTC(), mustLocation(user.Timezone)),
	}
}
