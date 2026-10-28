package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

type MeetingTypeHandler struct {
	base
}

func NewMeetingTypeHandler(deps Deps) *MeetingTypeHandler {
	return &MeetingTypeHandler{base: newBase(deps)}
}

func (h *MeetingTypeHandler) Index(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	types, err := h.deps.Store.ListMeetingTypes(owner.User.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.renderList(w, r, owner.User, types, nil, 24, 0)
}

func (h *MeetingTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	duration := formInt(r, "duration_minutes", 30)
	notice := formInt(r, "min_notice_hours", 24)
	var errs validate.FieldErrors
	if title == "" {
		errs.Add("title", h.localeFor(r).t("formats.error_title"))
	}
	if duration < 5 || duration > 480 {
		errs.Add("duration_minutes", h.localeFor(r).t("formats.error_duration"))
	}
	if notice < 0 || notice > 720 {
		errs.Add("min_notice_hours", h.localeFor(r).t("formats.error_notice"))
	}
	if errs.Any() {
		types, _ := h.deps.Store.ListMeetingTypes(owner.User.ID)
		h.renderList(w, r, owner.User, types, errs, notice, http.StatusUnprocessableEntity)
		return
	}

	meetingType := models.MeetingType{
		UserID:              owner.User.ID,
		Slug:                h.freeSlug(duration),
		Title:               title,
		Description:         strings.TrimSpace(r.FormValue("description")),
		DurationMinutes:     duration,
		BufferBeforeMinutes: 10,
		BufferAfterMinutes:  10,
		MinNoticeHours:      notice,
		BookingHorizonDays:  30,
		LocationType:        models.LocationGoogleMeet,
		IsActive:            true,
	}
	if _, err := h.deps.Store.CreateMeetingType(meetingType); err != nil {
		if errors.Is(err, store.ErrSlugTaken) {
			meetingType.Slug = h.freeSlug(duration)
			if _, retryErr := h.deps.Store.CreateMeetingType(meetingType); retryErr == nil {
				flash.Set(w, "notice", h.localeFor(r).t("formats.created"), h.deps.Config.CookieSecure())
				http.Redirect(w, r, "/meeting-types", http.StatusSeeOther)
				return
			}
		}
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", h.localeFor(r).t("formats.created"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/meeting-types", http.StatusSeeOther)
}

func (h *MeetingTypeHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id, ok := intParam(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	meetingType, err := h.deps.Store.GetMeetingType(owner.User.ID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.deps.Store.SetMeetingTypeActive(owner.User.ID, id, !meetingType.IsActive); err != nil {
		h.serverError(w, r, err)
		return
	}
	target := "/meeting-types"
	if r.FormValue("from") == "edit" {
		target = fmt.Sprintf("/meeting-types/%d/edit", id)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *MeetingTypeHandler) Edit(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id, ok := intParam(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	meetingType, err := h.deps.Store.GetMeetingType(owner.User.ID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderEdit(w, r, owner.User, meetingType, nil, 0)
}

func (h *MeetingTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id, ok := intParam(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	meetingType, err := h.deps.Store.GetMeetingType(owner.User.ID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	meetingType.Title = strings.TrimSpace(r.FormValue("title"))
	meetingType.Description = strings.TrimSpace(r.FormValue("description"))
	meetingType.DurationMinutes = formInt(r, "duration_minutes", meetingType.DurationMinutes)
	meetingType.BufferBeforeMinutes = formInt(r, "buffer_before_minutes", meetingType.BufferBeforeMinutes)
	meetingType.BufferAfterMinutes = formInt(r, "buffer_after_minutes", meetingType.BufferAfterMinutes)
	meetingType.MinNoticeHours = formInt(r, "min_notice_hours", meetingType.MinNoticeHours)
	meetingType.BookingHorizonDays = formInt(r, "booking_horizon_days", meetingType.BookingHorizonDays)
	meetingType.LocationType = strings.TrimSpace(r.FormValue("location_type"))
	meetingType.LocationDetails = strings.TrimSpace(r.FormValue("location_details"))
	meetingType.IsActive = r.FormValue("is_active") != ""

	var errs validate.FieldErrors
	if meetingType.Title == "" {
		errs.Add("title", h.localeFor(r).t("formats.error_title"))
	}
	if meetingType.DurationMinutes < 5 || meetingType.DurationMinutes > 480 {
		errs.Add("duration_minutes", h.localeFor(r).t("formats.error_duration"))
	}
	if meetingType.MinNoticeHours < 0 || meetingType.MinNoticeHours > 720 {
		errs.Add("min_notice_hours", h.localeFor(r).t("formats.error_notice"))
	}
	if errs.Any() {
		h.renderEdit(w, r, owner.User, meetingType, errs, http.StatusUnprocessableEntity)
		return
	}
	if err := h.deps.Store.UpdateMeetingType(meetingType); err != nil {
		if errors.Is(err, store.ErrSlugTaken) {
			h.renderEdit(w, r, owner.User, meetingType,
				validate.FieldErrors{"slug": h.localeFor(r).t("formats.error_slug")}, http.StatusUnprocessableEntity)
			return
		}
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", h.localeFor(r).t("formats.saved"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/meeting-types", http.StatusSeeOther)
}

func (h *MeetingTypeHandler) renderList(w http.ResponseWriter, r *http.Request, owner models.User, types []models.MeetingType, errs validate.FieldErrors, selectedNotice, status int) {
	l := h.localeFor(r)
	h.page(w, r, "app", "meeting_types", map[string]any{
		"Title":        "Session formats",
		"PublicPath":   publicPath(firstActiveMeetingType(types)),
		"ActiveNav":    "meeting-types",
		"Owner":        owner,
		"MeetingTypes": presentMeetingTypes(types, l),
		"NoticeHours":  noticeHourOptions(selectedNotice, l),
		"Errors":       errs,
	}, status)
}

func (h *MeetingTypeHandler) renderEdit(w http.ResponseWriter, r *http.Request, owner models.User, meetingType models.MeetingType, errs validate.FieldErrors, status int) {
	h.page(w, r, "app", "meeting_type_edit", map[string]any{
		"Title":           meetingType.Title,
		"PublicPath":      publicPath(meetingType),
		"ActiveNav":       "meeting-types",
		"Owner":           owner,
		"MeetingType":     presentMeetingType(meetingType, h.localeFor(r)),
		"Errors":          errs,
		"LocationOptions": locationOptions,
	}, status)
}

// freeSlug keeps public links short and readable: 30min, 30min-2, 30min-3.
func (h *MeetingTypeHandler) freeSlug(durationMinutes int) string {
	base := strconv.Itoa(durationMinutes) + "min"
	candidate := base
	for attempt := 2; attempt < 20; attempt++ {
		if _, err := h.deps.Store.GetMeetingTypeBySlug(candidate); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, attempt)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func formInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.FormValue(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// numberInPath finds the id segment when the router is not in the way, which
// is how handlers are exercised in tests.
func numberInPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if _, err := strconv.ParseInt(parts[i], 10, 64); err == nil {
			return parts[i]
		}
	}
	return ""
}

func intParam(r *http.Request, key string) (int64, bool) {
	raw := r.PathValue(key)
	if raw == "" {
		raw = r.FormValue(key)
	}
	if raw == "" {
		raw = numberInPath(r.URL.Path)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}
