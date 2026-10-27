package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/slot/internal/calendar"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/scheduling"
	"github.com/puppe1990/slot/internal/store"
)

var weekdayLabels = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

type weekdayRow struct {
	Index int
	Label string
	Start string
	End   string
	Open  bool
}

type SetupHandler struct {
	base
}

func NewSetupHandler(deps Deps) *SetupHandler {
	return &SetupHandler{base: newBase(deps)}
}

func (h *SetupHandler) Show(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	rules, err := h.deps.Store.ListAvailabilityRules(owner.User.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	connection, err := h.deps.Store.GetCalendarConnection(owner.User.ID)
	if err != nil && !errors.Is(err, store.ErrConnectionNotFound) {
		h.serverError(w, r, err)
		return
	}
	blocks, err := h.deps.Store.ListRecurringBlocks(owner.User.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, owner.User, rules, blocks, connection, nil, 0)
}

func (h *SetupHandler) Save(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rules := h.readWeeklyRules(r)

	var errs validate.FieldErrors
	if len(rules) == 0 {
		errs.Add("availability", h.localeFor(r).t("setup.error_window"))
	}
	if errs.Any() {
		connection, _ := h.deps.Store.GetCalendarConnection(owner.User.ID)
		blocks, _ := h.deps.Store.ListRecurringBlocks(owner.User.ID)
		h.render(w, r, owner.User, rules, blocks, connection, errs, http.StatusUnprocessableEntity)
		return
	}

	if err := h.deps.Store.ReplaceAvailabilityRules(owner.User.ID, rules); err != nil {
		h.serverError(w, r, err)
		return
	}
	// google is a stored preference only when the binary actually runs the
	// Google adapter; otherwise the radio would promise a sync that never happens.
	provider := models.CalendarProviderFake
	if strings.TrimSpace(r.FormValue("calendar_provider")) == models.CalendarProviderGoogle &&
		h.deps.Calendar.Provider() == calendar.ProviderGoogle {
		provider = models.CalendarProviderGoogle
	}
	if err := h.deps.Store.SaveCalendarConnection(models.CalendarConnection{
		UserID:       owner.User.ID,
		Provider:     provider,
		AccountEmail: owner.User.Email,
		SyncStatus:   "ACTIVE",
		LastSyncedAt: h.now(),
	}); err != nil {
		h.serverError(w, r, err)
		return
	}

	flash.Set(w, "notice", h.localeFor(r).t("setup.saved"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (h *SetupHandler) AddOverride(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	date := strings.TrimSpace(r.FormValue("date"))
	if _, err := time.Parse(dateKeyLayout, date); err != nil {
		flash.Set(w, "error", h.localeFor(r).t("setup.error_date"), h.deps.Config.CookieSecure())
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	override := models.DateOverride{
		Date:          date,
		IsUnavailable: r.FormValue("is_unavailable") != "",
	}
	if !override.IsUnavailable {
		override.CustomStartTime = strings.TrimSpace(r.FormValue("custom_start_time"))
		override.CustomEndTime = strings.TrimSpace(r.FormValue("custom_end_time"))
		if _, _, err := scheduling.ParseClock(override.CustomStartTime); err != nil {
			flash.Set(w, "error", h.localeFor(r).t("setup.error_start"), h.deps.Config.CookieSecure())
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		if _, _, err := scheduling.ParseClock(override.CustomEndTime); err != nil {
			flash.Set(w, "error", h.localeFor(r).t("setup.error_end"), h.deps.Config.CookieSecure())
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
	}
	if _, err := h.deps.Store.UpsertDateOverride(owner.User.ID, override); err != nil {
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", h.localeFor(r).t("setup.override_saved"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (h *SetupHandler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
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
	if err := h.deps.Store.DeleteDateOverride(owner.User.ID, id); err != nil {
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", h.localeFor(r).t("setup.override_removed"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (h *SetupHandler) AddBlock(w http.ResponseWriter, r *http.Request) {
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
	cookieSecure := h.deps.Config.CookieSecure()

	fail := func(message string) {
		flash.Set(w, "error", message, cookieSecure)
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
	}

	block, errs := buildRecurringBlock(
		owner.User.ID,
		r.FormValue("title"),
		models.ParseWeekdays(strings.Join(r.Form["weekdays"], ",")),
		strings.TrimSpace(r.FormValue("start_time")),
		strings.TrimSpace(r.FormValue("end_time")),
		formInt(r, "interval_weeks", 2),
		strings.TrimSpace(r.FormValue("anchor_date")),
		h.now().In(mustLocation(owner.User.Timezone)).Format(dateKeyLayout),
		l,
	)
	if errs.Any() {
		fail(firstError(errs))
		return
	}
	if _, err := h.deps.Store.CreateRecurringBlock(block); err != nil {
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", l.t("setup.recurring_saved"), cookieSecure)
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (h *SetupHandler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
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
	if err := h.deps.Store.DeleteRecurringBlock(owner.User.ID, id); err != nil {
		if errors.Is(err, store.ErrRecurringBlockNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, r, err)
		return
	}
	flash.Set(w, "notice", h.localeFor(r).t("setup.recurring_removed"), h.deps.Config.CookieSecure())
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (h *SetupHandler) readWeeklyRules(r *http.Request) []models.AvailabilityRule {
	rules := make([]models.AvailabilityRule, 0, 7)
	for day := 0; day < 7; day++ {
		start := strings.TrimSpace(r.FormValue(fmt.Sprintf("start_%d", day)))
		end := strings.TrimSpace(r.FormValue(fmt.Sprintf("end_%d", day)))
		if start == "" || end == "" {
			continue
		}
		rules = append(rules, models.AvailabilityRule{
			DayOfWeek: day,
			StartTime: start,
			EndTime:   end,
			IsActive:  true,
		})
	}
	return rules
}

func (h *SetupHandler) render(w http.ResponseWriter, r *http.Request, owner models.User, rules []models.AvailabilityRule, blocks []models.RecurringBlock, connection models.CalendarConnection, errs validate.FieldErrors, status int) {
	now := h.now()
	from := now.Format(dateKeyLayout)
	to := now.AddDate(0, 0, 120).Format(dateKeyLayout)
	overrides, err := h.deps.Store.ListDateOverrides(owner.ID, from, to)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	published, _ := h.deps.Store.DefaultMeetingType()
	h.page(w, r, "app", "setup", map[string]any{
		"Title":           "Availability",
		"PublicPath":      publicPath(published),
		"ActiveNav":       "setup",
		"Owner":           owner,
		"Profile":         presentProfile(owner),
		"Weekdays":        buildWeekdayRows(rules),
		"Connection":      connection,
		"Overrides":       overrides,
		"RecurringBlocks": blocks,
		"WeekdayIndexes":  []int{0, 1, 2, 3, 4, 5, 6},
		"Today":           now.In(mustLocation(owner.Timezone)).Format(dateKeyLayout),
		"Errors":          errs,
		"GoogleAvailable": h.deps.Calendar.Provider() == calendar.ProviderGoogle,
	}, status)
}

func buildWeekdayRows(rules []models.AvailabilityRule) []weekdayRow {
	// A brand-new owner has no rules yet. Prefill the work week so the first
	// visit to /setup is one click instead of seven time inputs.
	if len(rules) == 0 {
		return defaultWeekdayRows()
	}

	byDay := map[int]models.AvailabilityRule{}
	for _, rule := range rules {
		if _, exists := byDay[rule.DayOfWeek]; !exists {
			byDay[rule.DayOfWeek] = rule
		}
	}
	rows := make([]weekdayRow, 0, 7)
	for day := 0; day < 7; day++ {
		row := weekdayRow{Index: day, Label: weekdayLabels[day]}
		if rule, ok := byDay[day]; ok {
			row.Start = rule.StartTime
			row.End = rule.EndTime
			row.Open = true
		}
		rows = append(rows, row)
	}
	return rows
}

func defaultWeekdayRows() []weekdayRow {
	rows := make([]weekdayRow, 0, 7)
	for day := 0; day < 7; day++ {
		row := weekdayRow{Index: day, Label: weekdayLabels[day]}
		if day >= 1 && day <= 5 {
			row.Start = "09:00"
			row.End = "17:00"
			row.Open = true
		}
		rows = append(rows, row)
	}
	return rows
}
