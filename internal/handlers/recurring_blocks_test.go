package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/models"
)

func strconv64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func modelsRecurringBlock(userID int64, anchor string) models.RecurringBlock {
	return models.RecurringBlock{
		UserID:        userID,
		Title:         "All Mondays",
		Weekdays:      []int{1},
		StartTime:     "00:00",
		EndTime:       "23:59",
		IntervalWeeks: 1,
		AnchorDate:    anchor,
	}
}

func TestSetupAddsAndRemovesRecurringBlock(t *testing.T) {
	env := setupTestEnv(t)
	setup := NewSetupHandler(env.deps)

	add := postForm("/setup/blocks", url.Values{
		"title":          {"Morning Icaro Time"},
		"weekdays":       {"1", "2", "3", "4", "5"},
		"start_time":     {"07:00"},
		"end_time":       {"11:00"},
		"interval_weeks": {"2"},
		"anchor_date":    {"2026-10-26"},
	}, nil)
	add = session.WithUserID(add, env.ownerID)
	addRec := httptest.NewRecorder()
	setup.AddBlock(addRec, add)
	if addRec.Code != http.StatusSeeOther {
		t.Fatalf("add status = %d: %s", addRec.Code, addRec.Body.String())
	}

	blocks, err := env.store.ListRecurringBlocks(env.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks = %+v", blocks)
	}
	if blocks[0].Title != "Morning Icaro Time" || blocks[0].IntervalWeeks != 2 || len(blocks[0].Weekdays) != 5 {
		t.Fatalf("block = %+v", blocks[0])
	}

	page := httptest.NewRequest(http.MethodGet, "/setup", nil)
	page = session.WithUserID(page, env.ownerID)
	pageRec := httptest.NewRecorder()
	setup.Show(pageRec, page)
	if !strings.Contains(pageRec.Body.String(), "Morning Icaro Time") {
		t.Fatal("setup page does not list the block")
	}

	remove := postForm("/setup/blocks/"+strconv64(blocks[0].ID)+"/delete", nil, nil)
	remove = session.WithUserID(remove, env.ownerID)
	removeRec := httptest.NewRecorder()
	setup.DeleteBlock(removeRec, remove)
	if removeRec.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d", removeRec.Code)
	}
	blocks, _ = env.store.ListRecurringBlocks(env.ownerID)
	if len(blocks) != 0 {
		t.Fatalf("blocks after delete = %+v", blocks)
	}
}

func TestSetupRejectsRecurringBlockWithoutDays(t *testing.T) {
	env := setupTestEnv(t)
	setup := NewSetupHandler(env.deps)

	req := postForm("/setup/blocks", url.Values{
		"start_time":     {"07:00"},
		"end_time":       {"11:00"},
		"interval_weeks": {"2"},
		"anchor_date":    {"2026-10-26"},
	}, nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	setup.AddBlock(rec, req)

	blocks, _ := env.store.ListRecurringBlocks(env.ownerID)
	if len(blocks) != 0 {
		t.Fatalf("invalid block was stored: %+v", blocks)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
}

// A recurring block must reach the slot engine, not just the settings table.
func TestAdminAPISlotsSkipsRecurringBlock(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	if _, err := env.store.CreateRecurringBlock(modelsRecurringBlock(env.ownerID, "2026-11-02")); err != nil {
		t.Fatal(err)
	}
	api := NewAdminAPI(env.deps)

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
	if len(payload.Slots) == 0 {
		t.Fatal("no slots at all")
	}
	byDate := map[string]int{}
	for _, slot := range payload.Slots {
		byDate[slot.Date]++
	}
	if byDate["2026-11-02"] != 0 {
		t.Fatalf("blocked Monday still offers %d slots", byDate["2026-11-02"])
	}
	if byDate["2026-10-27"] == 0 || byDate["2026-11-03"] == 0 {
		t.Fatalf("other days lost their slots: %v", byDate)
	}
}
