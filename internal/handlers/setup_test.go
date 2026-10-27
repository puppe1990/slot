package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/models"
)

// A brand-new owner has no calendar connection row yet. /setup must render the
// form so they can save one — GetCalendarConnection returns a wrapped
// ErrConnectionNotFound, so the check has to be errors.Is.
func TestSetupRendersForOwnerWithoutCalendarConnection(t *testing.T) {
	env := setupTestEnv(t)
	userID, err := env.store.CreateUser("fresh@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.UpdateOwnerProfile(userID, "Fresh Owner", "America/New_York"); err != nil {
		t.Fatal(err)
	}

	setup := NewSetupHandler(env.deps)
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req = session.WithUserID(req, userID)
	rec := httptest.NewRecorder()
	setup.Show(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Hours, buffers and calendar", "Weekly hours", "Calendar sync"} {
		if !strings.Contains(body, want) {
			t.Errorf("setup page missing %q", want)
		}
	}
}

func TestBuildWeekdayRowsPrefillsWorkWeekForNewOwner(t *testing.T) {
	rows := buildWeekdayRows(nil)
	if len(rows) != 7 {
		t.Fatalf("rows = %d, want 7", len(rows))
	}
	for _, row := range rows {
		workday := row.Index >= 1 && row.Index <= 5
		if row.Open != workday {
			t.Errorf("%s open = %v, want %v", row.Label, row.Open, workday)
		}
		if workday && (row.Start != "09:00" || row.End != "17:00") {
			t.Errorf("%s = %s-%s, want 09:00-17:00", row.Label, row.Start, row.End)
		}
	}
}

func TestBuildWeekdayRowsKeepsASavedPartialWeek(t *testing.T) {
	rows := buildWeekdayRows([]models.AvailabilityRule{
		{DayOfWeek: 2, StartTime: "10:00", EndTime: "15:00", IsActive: true},
	})
	if rows[2].Start != "10:00" || rows[2].End != "15:00" || !rows[2].Open {
		t.Fatalf("Tuesday = %+v, want the saved 10:00-15:00 window", rows[2])
	}
	if rows[1].Open || rows[1].Start != "" {
		t.Fatalf("Monday = %+v, want it closed: defaults must not reappear after a save", rows[1])
	}
}

// A brand-new owner has no rules; /settings marks that so the client script may
// preselect the browser zone. After the first save the marker must be false,
// otherwise every visit would overwrite the owner's choice.
func TestSettingsMarksFirstVisitOnlyWithoutRules(t *testing.T) {
	env := setupTestEnv(t)
	userID, err := env.store.CreateUser("tz@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	settings := NewSettingsHandler(env.deps)

	render := func() string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		req = session.WithUserID(req, userID)
		rec := httptest.NewRecorder()
		settings.Show(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if body := render(); !strings.Contains(body, `data-first-visit="true"`) {
		t.Error("new owner should be marked as a first visit")
	}
	if body := render(); !strings.Contains(body, `id="tz-hint"`) {
		t.Error("settings is missing the timezone hint")
	}

	if err := env.store.ReplaceAvailabilityRules(userID, []models.AvailabilityRule{
		{DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00", IsActive: true},
	}); err != nil {
		t.Fatal(err)
	}
	if body := render(); !strings.Contains(body, `data-first-visit="false"`) {
		t.Error("owner with saved rules must not be treated as a first visit")
	}
}
