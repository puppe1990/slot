package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

type createFormatRequest struct {
	Slug                string `json:"slug"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	DurationMinutes     int    `json:"duration_minutes"`
	BufferBeforeMinutes int    `json:"buffer_before_minutes"`
	BufferAfterMinutes  int    `json:"buffer_after_minutes"`
	MinNoticeHours      *int   `json:"min_notice_hours"`
	BookingHorizonDays  int    `json:"booking_horizon_days"`
	LocationType        string `json:"location_type"`
}

func (h *AdminAPI) CreateFormat(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w)
	if !ok {
		return
	}
	var input createFormatRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	l := h.localeFor(r)
	var errs validate.FieldErrors
	title := strings.TrimSpace(input.Title)
	if title == "" {
		errs.Add("title", l.t("formats.error_title"))
	}
	duration := input.DurationMinutes
	if duration == 0 {
		duration = 30
	}
	if duration < 5 || duration > 480 {
		errs.Add("duration_minutes", l.t("formats.error_duration"))
	}
	notice := intPointerOr(input.MinNoticeHours, 24)
	if notice < 0 || notice > 720 {
		errs.Add("min_notice_hours", l.t("formats.error_notice"))
	}
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	if slug != "" && !validSlug(slug) {
		errs.Add("slug", "slug must be lowercase letters, digits and dashes")
	}
	if errs.Any() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": errs})
		return
	}

	meetingType := models.MeetingType{
		UserID:              owner.ID,
		Slug:                slug,
		Title:               title,
		Description:         strings.TrimSpace(input.Description),
		DurationMinutes:     duration,
		BufferBeforeMinutes: orInt(input.BufferBeforeMinutes, 10),
		BufferAfterMinutes:  orInt(input.BufferAfterMinutes, 10),
		MinNoticeHours:      notice,
		BookingHorizonDays:  orInt(input.BookingHorizonDays, 30),
		LocationType:        models.LocationGoogleMeet,
		IsActive:            true,
	}
	if strings.TrimSpace(input.LocationType) != "" {
		meetingType.LocationType = strings.ToUpper(strings.TrimSpace(input.LocationType))
	}
	if meetingType.Slug == "" {
		meetingType.Slug = NewMeetingTypeHandler(h.deps).freeSlug(duration)
	}

	id, err := h.deps.Store.CreateMeetingType(meetingType)
	if err != nil {
		if errors.Is(err, store.ErrSlugTaken) {
			writeJSONError(w, http.StatusConflict, "slug already used: "+meetingType.Slug)
			return
		}
		h.serverError(w, r, err)
		return
	}
	created, err := h.deps.Store.GetMeetingType(owner.ID, id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"format": h.presentFormat(r, created)})
}

func (h *AdminAPI) UpdateFormat(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w)
	if !ok {
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		writeJSONError(w, http.StatusBadRequest, "slug is required")
		return
	}
	meetingType, err := h.deps.Store.GetMeetingTypeBySlug(slug)
	if err != nil || meetingType.UserID != owner.ID {
		writeJSONError(w, http.StatusNotFound, "format not found: "+slug)
		return
	}
	var input createFormatRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	l := h.localeFor(r)
	applyFormatPatch(&meetingType, input)
	var errs validate.FieldErrors
	if meetingType.DurationMinutes < 5 || meetingType.DurationMinutes > 480 {
		errs.Add("duration_minutes", l.t("formats.error_duration"))
	}
	if meetingType.MinNoticeHours < 0 || meetingType.MinNoticeHours > 720 {
		errs.Add("min_notice_hours", l.t("formats.error_notice"))
	}
	if errs.Any() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": errs})
		return
	}
	if err := h.deps.Store.UpdateMeetingType(meetingType); err != nil {
		h.serverError(w, r, err)
		return
	}
	updated, err := h.deps.Store.GetMeetingType(owner.ID, meetingType.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"format": h.presentFormat(r, updated)})
}

func applyFormatPatch(meetingType *models.MeetingType, input createFormatRequest) {
	if title := strings.TrimSpace(input.Title); title != "" {
		meetingType.Title = title
	}
	if strings.TrimSpace(input.Description) != "" {
		meetingType.Description = strings.TrimSpace(input.Description)
	}
	if input.DurationMinutes != 0 {
		meetingType.DurationMinutes = input.DurationMinutes
	}
	if input.MinNoticeHours != nil {
		meetingType.MinNoticeHours = *input.MinNoticeHours
	}
	if input.BufferBeforeMinutes > 0 {
		meetingType.BufferBeforeMinutes = input.BufferBeforeMinutes
	}
	if input.BufferAfterMinutes > 0 {
		meetingType.BufferAfterMinutes = input.BufferAfterMinutes
	}
	if input.BookingHorizonDays > 0 {
		meetingType.BookingHorizonDays = input.BookingHorizonDays
	}
	if strings.TrimSpace(input.LocationType) != "" {
		meetingType.LocationType = strings.ToUpper(strings.TrimSpace(input.LocationType))
	}
}

func (h *AdminAPI) presentFormat(r *http.Request, meetingType models.MeetingType) map[string]any {
	return map[string]any{
		"id":               meetingType.ID,
		"slug":             meetingType.Slug,
		"title":            meetingType.Title,
		"duration_minutes": meetingType.DurationMinutes,
		"min_notice_hours": meetingType.MinNoticeHours,
		"is_active":        meetingType.IsActive,
		"public_url":       h.absoluteURL(r, "/book/"+meetingType.Slug),
	}
}

// validSlug keeps the public path predictable: /book/<slug>.
func validSlug(slug string) bool {
	if len(slug) < 2 || len(slug) > 48 {
		return false
	}
	for _, char := range slug {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return !strings.HasPrefix(slug, "-") && !strings.HasSuffix(slug, "-")
}

func orInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func intPointerOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
