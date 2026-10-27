package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

// AdminAPI is the JSON surface shared by the CLI and the MCP server. It runs
// behind middleware.AdminAuth (Bearer ADMIN_TOKEN) and acts as the single
// owner, so both clients stay thin and never touch the database.
type AdminAPI struct {
	base
}

func NewAdminAPI(deps Deps) *AdminAPI {
	return &AdminAPI{base: newBase(deps)}
}

func (h *AdminAPI) owner(w http.ResponseWriter) (models.User, bool) {
	owner, err := h.deps.Store.FirstOwner()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "no owner account yet")
		return models.User{}, false
	}
	return owner, true
}

func (h *AdminAPI) Bookings(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w)
	if !ok {
		return
	}
	filter := parseDashboardFilter(r)
	bookings, err := h.deps.Store.ListBookings(owner.ID, filter.storeFilter(h.now()))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	out := make([]adminBooking, 0, len(bookings))
	for _, booking := range bookings {
		out = append(out, h.presentAdminBooking(r, booking))
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": out})
}

func (h *AdminAPI) Formats(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w)
	if !ok {
		return
	}
	types, err := h.deps.Store.ListMeetingTypes(owner.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(types))
	for _, meetingType := range types {
		out = append(out, map[string]any{
			"slug":              meetingType.Slug,
			"title":             meetingType.Title,
			"duration_minutes":  meetingType.DurationMinutes,
			"is_active":         meetingType.IsActive,
			"public_url":        h.absoluteURL(r, "/book/"+meetingType.Slug),
			"min_notice_hours":  meetingType.MinNoticeHours,
			"horizon_days":      meetingType.BookingHorizonDays,
			"location_type":     meetingType.LocationType,
			"buffer_before_min": meetingType.BufferBeforeMinutes,
			"buffer_after_min":  meetingType.BufferAfterMinutes,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"formats": out})
}

func (h *AdminAPI) Slots(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w)
	if !ok {
		return
	}
	slug := strings.TrimSpace(r.URL.Query().Get("format"))
	if slug == "" {
		writeJSONError(w, http.StatusBadRequest, "format is required")
		return
	}
	meetingType, err := h.deps.Store.GetMeetingTypeBySlug(slug)
	if err != nil || !meetingType.IsActive {
		writeJSONError(w, http.StatusNotFound, "format not found or paused: "+slug)
		return
	}
	guestTimezone := normalizeTimezone(r.URL.Query().Get("tz"), owner.Timezone)
	data, err := h.buildSlots(r.Context(), meetingType, guestTimezone, r.URL.Query().Get("date"), 0, h.localeFor(r))
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	days := formInt(r, "days", 0)
	limit := h.now().AddDate(0, 0, days)
	out := make([]map[string]any, 0, len(data.All))
	for _, slot := range data.All {
		start, err := time.Parse(time.RFC3339, slot.StartISO)
		if err != nil || (days > 0 && start.After(limit)) {
			continue
		}
		out = append(out, map[string]any{
			"date":        slot.Date,
			"start_utc":   slot.StartISO,
			"end_utc":     slot.EndISO,
			"guest_clock": slot.GuestClock,
			"guest_range": slot.GuestRange,
			"host_range":  slot.HostRange,
			"timezone":    data.GuestTimezone,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"format":   meetingType.Slug,
		"title":    meetingType.Title,
		"timezone": data.GuestTimezone,
		"slots":    out,
	})
}

type createBookingRequest struct {
	Format   string `json:"format"`
	Slot     string `json:"slot"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	WhatsApp string `json:"whatsapp"`
	Notes    string `json:"notes"`
	Timezone string `json:"timezone"`
}

func (h *AdminAPI) CreateBooking(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w)
	if !ok {
		return
	}
	var input createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	meetingType, err := h.deps.Store.GetMeetingTypeBySlug(strings.TrimSpace(input.Format))
	if err != nil || !meetingType.IsActive {
		writeJSONError(w, http.StatusNotFound, "format not found or paused: "+input.Format)
		return
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(input.Slot))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "slot must be RFC3339: "+input.Slot)
		return
	}
	var errs validate.FieldErrors
	if err := validate.Required(map[string]string{"name": input.Name}, "name"); err != nil {
		errs.Add("name", "name is required")
	}
	if err := validate.Email(strings.TrimSpace(input.Email)); err != nil {
		errs.Add("email", "email is invalid")
	}
	whatsapp := normalizeWhatsApp(input.WhatsApp)
	if whatsapp != "" && !validWhatsApp(whatsapp) {
		errs.Add("whatsapp", "whatsapp needs 8 to 15 digits")
	}
	if errs.Any() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": errs})
		return
	}
	guestTimezone := normalizeTimezone(input.Timezone, owner.Timezone)

	now := h.now()
	hold, holdToken, err := h.deps.Store.CreateHold(store.HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         start.UTC(),
		End:           start.UTC().Add(time.Duration(meetingType.DurationMinutes) * time.Minute),
		GuestEmail:    strings.TrimSpace(input.Email),
		GuestTimezone: guestTimezone,
		ExpiresAt:     now.Add(10 * time.Minute),
		Now:           now,
	})
	if err != nil {
		if errors.Is(err, store.ErrSlotUnavailable) {
			writeJSONError(w, http.StatusConflict, "slot unavailable: "+input.Slot)
			return
		}
		h.serverError(w, r, err)
		return
	}

	guestToken := h.deps.Signer.Sign(hold.ID)
	booking, err := h.deps.Store.ConfirmHold(holdToken, store.GuestInput{
		Name:     strings.TrimSpace(input.Name),
		Email:    strings.TrimSpace(input.Email),
		WhatsApp: whatsapp,
		Notes:    strings.TrimSpace(input.Notes),
		Timezone: guestTimezone,
	}, guesttoken.Hash(guestToken), now)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.finalizeBooking(r, booking, guestToken)
	stored, err := h.deps.Store.GetBooking(owner.ID, booking.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"booking":    h.presentAdminBooking(r, stored),
		"manage_url": h.absoluteURL(r, "/manage/"+guestToken),
	})
}

func (h *AdminAPI) CancelBooking(w http.ResponseWriter, r *http.Request) {
	owner, booking, ok := h.ownedBooking(w, r)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)

	if err := h.cancelBooking(r, booking, models.ActorOwner, input.Reason, ""); err != nil {
		if errors.Is(err, store.ErrBookingNotFound) {
			writeJSONError(w, http.StatusNotFound, "booking not found")
			return
		}
		h.serverError(w, r, err)
		return
	}
	stored, _ := h.deps.Store.GetBooking(owner.ID, booking.ID)
	writeJSON(w, http.StatusOK, map[string]any{"booking": h.presentAdminBooking(r, stored)})
}

func (h *AdminAPI) RescheduleBooking(w http.ResponseWriter, r *http.Request) {
	owner, booking, ok := h.ownedBooking(w, r)
	if !ok {
		return
	}
	var input struct {
		Slot string `json:"slot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(input.Slot))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "slot must be RFC3339: "+input.Slot)
		return
	}
	if !booking.Active() {
		writeJSONError(w, http.StatusConflict, "booking is cancelled")
		return
	}
	if err := h.rescheduleBooking(r, booking, start, models.ActorOwner, "Owner rescheduled", ""); err != nil {
		if errors.Is(err, store.ErrSlotUnavailable) {
			writeJSONError(w, http.StatusConflict, "slot unavailable: "+input.Slot)
			return
		}
		h.serverError(w, r, err)
		return
	}
	stored, _ := h.deps.Store.GetBooking(owner.ID, booking.ID)
	writeJSON(w, http.StatusOK, map[string]any{"booking": h.presentAdminBooking(r, stored)})
}

func (h *AdminAPI) ownedBooking(w http.ResponseWriter, r *http.Request) (models.User, models.Booking, bool) {
	owner, ok := h.owner(w)
	if !ok {
		return models.User{}, models.Booking{}, false
	}
	rawID := pathBookingID(r)
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, http.StatusBadRequest, "booking id is invalid: "+rawID)
		return models.User{}, models.Booking{}, false
	}
	booking, err := h.deps.Store.GetBooking(owner.ID, id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "booking not found: "+rawID)
		return models.User{}, models.Booking{}, false
	}
	return owner, booking, true
}

func (h *AdminAPI) presentAdminBooking(r *http.Request, booking models.Booking) adminBooking {
	out := adminBooking{
		ID:              booking.ID,
		Status:          string(booking.Status),
		Title:           booking.MeetingTypeTitle,
		Slug:            booking.MeetingTypeSlug,
		DurationMinutes: booking.MeetingTypeDuration,
		GuestName:       booking.GuestName,
		GuestEmail:      booking.GuestEmail,
		GuestWhatsApp:   booking.GuestWhatsApp,
		GuestNotes:      booking.GuestNotes,
		GuestTimezone:   booking.GuestTimezone,
		StartUTC:        booking.StartUTC.Format(time.RFC3339),
		EndUTC:          booking.EndUTC.Format(time.RFC3339),
		Attendance:      booking.Attendance,
		MeetingURL:      booking.MeetingURL,
		PublicURL:       h.absoluteURL(r, "/book/"+booking.MeetingTypeSlug),
		JoinURL:         h.absoluteURL(r, "/bookings/"+strconv.FormatInt(booking.ID, 10)+"/join"),
	}
	if booking.Status == models.BookingStatusCancelled {
		out.CancellationReason = booking.CancellationReason
	}
	return out
}

type adminBooking struct {
	ID                 int64  `json:"id"`
	Status             string `json:"status"`
	Title              string `json:"title"`
	Slug               string `json:"slug"`
	DurationMinutes    int    `json:"duration_minutes"`
	GuestName          string `json:"guest_name"`
	GuestEmail         string `json:"guest_email"`
	GuestWhatsApp      string `json:"guest_whatsapp,omitempty"`
	GuestNotes         string `json:"guest_notes,omitempty"`
	GuestTimezone      string `json:"guest_timezone"`
	StartUTC           string `json:"start_utc"`
	EndUTC             string `json:"end_utc"`
	Attendance         string `json:"attendance,omitempty"`
	MeetingURL         string `json:"meeting_url,omitempty"`
	PublicURL          string `json:"public_url"`
	JoinURL            string `json:"join_url"`
	CancellationReason string `json:"cancellation_reason,omitempty"`
}

// pathBookingID reads the {id} segment. The router fills PathValue; tests call
// the handler directly, so the path is the fallback.
func pathBookingID(r *http.Request) string {
	if id := r.PathValue("id"); id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "bookings" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
