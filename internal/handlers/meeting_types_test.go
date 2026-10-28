package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

// The list renders one <.form> per format inside a range. A component override
// that reads a field the row does not have 500s here instead of at boot.
func TestMeetingTypesListOffersMinNoticeHours(t *testing.T) {
	env := setupTestEnv(t)
	handler := NewMeetingTypeHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/meeting-types", nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	handler.Index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="min_notice_hours"`) {
		t.Fatal("create form is missing min_notice_hours")
	}
	if !strings.Contains(body, `value="0"`) {
		t.Fatal("create form should offer 0 hours (same-day booking)")
	}
}

func TestCreateFormatHonorsZeroMinNotice(t *testing.T) {
	env := setupTestEnv(t)
	handler := NewMeetingTypeHandler(env.deps)

	form := url.Values{
		"title":            {"Same-day advisory"},
		"duration_minutes": {"30"},
		"min_notice_hours": {"0"},
	}
	req := httptest.NewRequest(http.MethodPost, "/meeting-types", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}

	created, err := env.store.GetMeetingTypeBySlug("30min")
	if err != nil {
		t.Fatal(err)
	}
	if created.MinNoticeHours != 0 {
		t.Fatalf("min_notice_hours = %d, want 0 so remaining times today stay bookable", created.MinNoticeHours)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/meeting-types", nil)
	listReq = session.WithUserID(listReq, env.ownerID)
	listRec := httptest.NewRecorder()
	handler.Index(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", listRec.Code, listRec.Body.String())
	}
	listBody := listRec.Body.String()
	if strings.Contains(listBody, `text-foam">0h notice`) {
		t.Fatal("format card labeled 0h as 0h notice")
	}
	if !strings.Contains(listBody, `text-foam">Same day (0h)`) {
		t.Fatal("format card should label 0h as same-day")
	}
}

func TestCreateFormatKeepsZeroNoticeOnError(t *testing.T) {
	env := setupTestEnv(t)
	handler := NewMeetingTypeHandler(env.deps)

	form := url.Values{
		"title":            {""},
		"duration_minutes": {"30"},
		"min_notice_hours": {"0"},
	}
	req := httptest.NewRequest(http.MethodPost, "/meeting-types", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `value="0" selected`) {
		t.Fatal("validation error should keep 0h selected, not snap back to 24h")
	}
}

func TestUpdateFormatHonorsZeroMinNotice(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	meetingType.MinNoticeHours = 24
	if err := env.store.UpdateMeetingType(meetingType); err != nil {
		t.Fatal(err)
	}
	handler := NewMeetingTypeHandler(env.deps)

	form := url.Values{
		"title":                {meetingType.Title},
		"duration_minutes":     {"30"},
		"min_notice_hours":     {"0"},
		"booking_horizon_days": {"30"},
		"location_type":        {meetingType.LocationType},
		"is_active":            {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/meeting-types/%d", meetingType.ID), strings.NewReader(form.Encode()))
	req.SetPathValue("id", fmt.Sprintf("%d", meetingType.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := env.store.GetMeetingType(env.ownerID, meetingType.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MinNoticeHours != 0 {
		t.Fatalf("min_notice_hours = %d, want 0", updated.MinNoticeHours)
	}
}

func TestMeetingTypesListRendersAfterCreate(t *testing.T) {
	env := setupTestEnv(t)
	userID, err := env.store.CreateUser("formats@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewMeetingTypeHandler(env.deps)

	form := url.Values{"title": {"30-Minute Strategy Advisory"}, "duration_minutes": {"30"}}
	createReq := httptest.NewRequest(http.MethodPost, "/meeting-types", strings.NewReader(form.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq = session.WithUserID(createReq, userID)
	createRec := httptest.NewRecorder()
	handler.Create(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d: %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/meeting-types", nil)
	listReq = session.WithUserID(listReq, userID)
	listRec := httptest.NewRecorder()
	handler.Index(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listRec.Code, listRec.Body.String())
	}
	body := listRec.Body.String()
	for _, want := range []string{"30-Minute Strategy Advisory", "/book/30min", "Pause"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
}

// The editor renders a <select> per modality from locationOptions; a mismatch
// between the Go struct and the template fields breaks the page at render time
// (it shipped once as "can't evaluate field Value in type string").
func TestMeetingTypeEditRenders(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	handler := NewMeetingTypeHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/meeting-types/1/edit", nil)
	req.SetPathValue("id", "1")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("edit = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		meetingType.Title,
		`name="min_notice_hours"`,
		`value="GOOGLE_MEET"`,
		`value="PHONE"`,
		`value="CUSTOM"`,
		"Save format",
		"0 keeps remaining times today",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit page missing %q", want)
		}
	}
	if strings.Contains(body, "can't evaluate field") {
		t.Error("edit page leaked a template error")
	}
}
