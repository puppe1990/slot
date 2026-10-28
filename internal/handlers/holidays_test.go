package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/holidays"
	"github.com/puppe1990/slot/internal/models"
)

func holidaysListForTest() []holidays.Day {
	return holidays.List()
}

func TestHolidayClosuresRespectTogglesAndManualOverrides(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, loc)
	horizon := now.AddDate(0, 0, 30)

	closed := holidayClosures(nil, map[string]bool{}, now, horizon, loc)
	if len(closed) != 2 { // Finados (2 Nov) and República (15 Nov)
		t.Fatalf("closures = %+v, want 2", closed)
	}
	dates := map[string]bool{}
	for _, override := range closed {
		if !override.IsUnavailable {
			t.Errorf("%s should be closed", override.Date)
		}
		dates[override.Date] = true
	}
	if !dates["2026-11-02"] || !dates["2026-11-15"] {
		t.Fatalf("dates = %v", dates)
	}

	off := holidayClosures(nil, map[string]bool{"all_souls": false}, now, horizon, loc)
	for _, override := range off {
		if override.Date == "2026-11-02" {
			t.Fatal("all_souls was turned off but still closed")
		}
	}

	// The owner opened the holiday manually: the explicit override wins.
	explicit := []models.DateOverride{{Date: "2026-11-15", IsUnavailable: false, CustomStartTime: "09:00", CustomEndTime: "12:00"}}
	kept := holidayClosures(explicit, map[string]bool{}, now, horizon, loc)
	for _, override := range kept {
		if override.Date == "2026-11-15" && override.IsUnavailable {
			t.Fatal("manual override was overwritten by the holiday")
		}
	}
}

func TestHolidaysCloseTheDayInTheAPI(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	api := NewAdminAPI(env.deps)

	read := func() map[string]int {
		t.Helper()
		rec := httptest.NewRecorder()
		api.Slots(rec, adminRequest(http.MethodGet, "/api/admin/slots?format=30min&days=30", "", "secret"))
		if rec.Code != http.StatusOK {
			t.Fatalf("slots = %d: %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Slots []struct {
				Date string `json:"date"`
			} `json:"slots"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		byDate := map[string]int{}
		for _, slot := range payload.Slots {
			byDate[slot.Date]++
		}
		return byDate
	}

	before := read()
	if before["2026-11-02"] != 0 || before["2026-11-15"] != 0 {
		t.Fatalf("holidays still bookable: %v", before)
	}
	if before["2026-11-03"] == 0 {
		t.Fatal("the day after Finados lost its slots")
	}

	if err := env.store.SaveHolidayPrefs(env.ownerID, map[string]bool{"all_souls": false}); err != nil {
		t.Fatal(err)
	}
	after := read()
	if after["2026-11-02"] == 0 {
		t.Fatal("turning the holiday off did not reopen the day")
	}
	if after["2026-11-15"] != 0 {
		t.Fatal("república should stay closed")
	}
}

func TestSetupSavesHolidayToggles(t *testing.T) {
	env := setupTestEnv(t)
	setup := NewSetupHandler(env.deps)

	req := postForm("/setup/holidays", url.Values{"holiday": {"christmas", "all_souls"}}, nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	setup.SaveHolidays(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	prefs, err := env.store.ListHolidayPrefs(env.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs) != len(holidaysListForTest()) {
		t.Fatalf("prefs = %v, want every holiday stored", prefs)
	}
	if !prefs["christmas"] || prefs["new_year"] {
		t.Fatalf("prefs = %v", prefs)
	}

	page := httptest.NewRequest(http.MethodGet, "/setup", nil)
	page = session.WithUserID(page, env.ownerID)
	pageRec := httptest.NewRecorder()
	setup.Show(pageRec, page)
	body := pageRec.Body.String()
	for _, want := range []string{"Christmas", "New Year&#39;s Day", "Next:"} {
		if !strings.Contains(body, want) {
			t.Errorf("setup missing %q", want)
		}
	}
	if body == "" {
		t.Fatal("empty page")
	}
}
