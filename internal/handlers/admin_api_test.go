package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/middleware"

	"github.com/puppe1990/slot/internal/call"
)

func adminRequest(method, path, body, token string) *http.Request {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestAdminAPIRequiresBearerToken(t *testing.T) {
	env := setupTestEnv(t)
	api := NewAdminAPI(env.deps)
	cfg := cais.Config{Env: "test", AdminToken: "secret-token"}
	handler := middleware.AdminAuth(cfg)(http.HandlerFunc(api.Bookings))

	anon := httptest.NewRecorder()
	handler.ServeHTTP(anon, adminRequest(http.MethodGet, "/api/admin/bookings", "", ""))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", anon.Code)
	}

	wrong := httptest.NewRecorder()
	handler.ServeHTTP(wrong, adminRequest(http.MethodGet, "/api/admin/bookings", "", "nope"))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d, want 401", wrong.Code)
	}

	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, adminRequest(http.MethodGet, "/api/admin/bookings", "", "secret-token"))
	if ok.Code != http.StatusOK {
		t.Fatalf("valid token = %d: %s", ok.Code, ok.Body.String())
	}
}

func TestAdminAPIBooksListsAndCancels(t *testing.T) {
	env := setupTestEnv(t)
	env.deps.Call = call.None{}
	env.seedMeetingType(t, "30min", 30)
	api := NewAdminAPI(env.deps)

	booked := httptest.NewRecorder()
	api.CreateBooking(booked, adminRequest(http.MethodPost, "/api/admin/bookings", `{
		"format":"30min","slot":"2026-10-26T14:00:00Z",
		"name":"Marina Costa","email":"marina@example.com",
		"whatsapp":"+55 11 99999-8888","notes":"Review the plan.",
		"timezone":"America/Sao_Paulo"
	}`, "secret"))
	if booked.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", booked.Code, booked.Body.String())
	}
	var created struct {
		Booking struct {
			ID            int64  `json:"id"`
			Status        string `json:"status"`
			GuestWhatsApp string `json:"guest_whatsapp"`
			JoinURL       string `json:"join_url"`
		} `json:"booking"`
		ManageURL string `json:"manage_url"`
	}
	if err := json.Unmarshal(booked.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Booking.Status != "CONFIRMED" || created.Booking.GuestWhatsApp != "+5511999998888" {
		t.Fatalf("booking = %+v", created.Booking)
	}
	if !strings.Contains(created.ManageURL, "/manage/") || !strings.Contains(created.Booking.JoinURL, "/join") {
		t.Fatalf("links = %q / %q", created.ManageURL, created.Booking.JoinURL)
	}

	list := httptest.NewRecorder()
	api.Bookings(list, adminRequest(http.MethodGet, "/api/admin/bookings?status=upcoming&days=30", "", "secret"))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Marina Costa") {
		t.Fatalf("list = %d: %s", list.Code, list.Body.String())
	}

	cancelPath := "/api/admin/bookings/" + strconv.FormatInt(created.Booking.ID, 10) + "/cancel"
	cancelled := httptest.NewRecorder()
	api.CancelBooking(cancelled, adminRequest(http.MethodPost, cancelPath, `{"reason":"conflict"}`, "secret"))
	if cancelled.Code != http.StatusOK || !strings.Contains(cancelled.Body.String(), "CANCELLED") {
		t.Fatalf("cancel = %d: %s", cancelled.Code, cancelled.Body.String())
	}
}

func TestAdminAPIRejectsTakenSlot(t *testing.T) {
	env := setupTestEnv(t)
	env.deps.Call = call.None{}
	env.seedMeetingType(t, "30min", 30)
	api := NewAdminAPI(env.deps)

	body := `{"format":"30min","slot":"2026-10-26T14:00:00Z","name":"A","email":"a@example.com"}`
	first := httptest.NewRecorder()
	api.CreateBooking(first, adminRequest(http.MethodPost, "/api/admin/bookings", body, "secret"))
	if first.Code != http.StatusCreated {
		t.Fatalf("first = %d: %s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	api.CreateBooking(second, adminRequest(http.MethodPost, "/api/admin/bookings", body, "secret"))
	if second.Code != http.StatusConflict {
		t.Fatalf("second = %d, want 409: %s", second.Code, second.Body.String())
	}
}

func TestAdminAPIRejectsInvalidInput(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	api := NewAdminAPI(env.deps)

	missingEmail := httptest.NewRecorder()
	api.CreateBooking(missingEmail, adminRequest(http.MethodPost, "/api/admin/bookings",
		`{"format":"30min","slot":"2026-10-26T14:00:00Z","name":"A","email":"not-an-email"}`, "secret"))
	if missingEmail.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid email = %d, want 422", missingEmail.Code)
	}

	unknownFormat := httptest.NewRecorder()
	api.CreateBooking(unknownFormat, adminRequest(http.MethodPost, "/api/admin/bookings",
		`{"format":"nope","slot":"2026-10-26T14:00:00Z","name":"A","email":"a@example.com"}`, "secret"))
	if unknownFormat.Code != http.StatusNotFound {
		t.Fatalf("unknown format = %d, want 404", unknownFormat.Code)
	}
}

func TestAdminAPISlots(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	api := NewAdminAPI(env.deps)

	rec := httptest.NewRecorder()
	api.Slots(rec, adminRequest(http.MethodGet, "/api/admin/slots?format=30min&days=2", "", "secret"))
	if rec.Code != http.StatusOK {
		t.Fatalf("slots = %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Slots []struct {
			StartUTC string `json:"start_utc"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Slots) == 0 {
		t.Fatal("no slots returned")
	}
}

func TestAdminAPIBlocksCRUD(t *testing.T) {
	env := setupTestEnv(t)
	api := NewAdminAPI(env.deps)

	me := httptest.NewRecorder()
	api.Me(me, adminRequest(http.MethodGet, "/api/admin/me", "", "secret"))
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), "owner@example.com") {
		t.Fatalf("me = %d: %s", me.Code, me.Body.String())
	}

	created := httptest.NewRecorder()
	api.CreateBlock(created, adminRequest(http.MethodPost, "/api/admin/blocks", `{
		"title":"Morning Icaro Time","weekdays":[1,2,3,4,5],
		"start_time":"07:00","end_time":"11:00","interval_weeks":2,"anchor_date":"2026-11-02"
	}`, "secret"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var payload struct {
		Block struct {
			ID            int64  `json:"id"`
			IntervalWeeks int    `json:"interval_weeks"`
			AnchorDate    string `json:"anchor_date"`
		} `json:"block"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Block.ID == 0 || payload.Block.IntervalWeeks != 2 || payload.Block.AnchorDate != "2026-11-02" {
		t.Fatalf("block = %+v", payload.Block)
	}

	list := httptest.NewRecorder()
	api.Blocks(list, adminRequest(http.MethodGet, "/api/admin/blocks", "", "secret"))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Morning Icaro Time") {
		t.Fatalf("list = %d: %s", list.Code, list.Body.String())
	}

	remove := httptest.NewRecorder()
	api.DeleteBlock(remove, adminRequest(http.MethodPost,
		"/api/admin/blocks/"+strconv.FormatInt(payload.Block.ID, 10)+"/delete", `{}`, "secret"))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", remove.Code, remove.Body.String())
	}
	blocks, _ := env.store.ListRecurringBlocks(env.ownerID)
	if len(blocks) != 0 {
		t.Fatalf("blocks left = %+v", blocks)
	}
}

func TestAdminAPIBlockValidation(t *testing.T) {
	env := setupTestEnv(t)
	api := NewAdminAPI(env.deps)

	rec := httptest.NewRecorder()
	api.CreateBlock(rec, adminRequest(http.MethodPost, "/api/admin/blocks",
		`{"weekdays":[],"start_time":"07:00","end_time":"11:00","interval_weeks":2}`, "secret"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty weekdays = %d, want 422: %s", rec.Code, rec.Body.String())
	}

	backwards := httptest.NewRecorder()
	api.CreateBlock(backwards, adminRequest(http.MethodPost, "/api/admin/blocks",
		`{"weekdays":[1],"start_time":"11:00","end_time":"07:00","interval_weeks":2}`, "secret"))
	if backwards.Code != http.StatusUnprocessableEntity {
		t.Fatalf("backwards clock = %d, want 422", backwards.Code)
	}
}

func TestAdminAPICreateFormat(t *testing.T) {
	env := setupTestEnv(t)
	api := NewAdminAPI(env.deps)

	rec := httptest.NewRecorder()
	api.CreateFormat(rec, adminRequest(http.MethodPost, "/api/admin/formats", `{
		"slug":"hora-solar","title":"Hora Solar","duration_minutes":45,
		"description":"Diagnóstico do homologador","min_notice_hours":12
	}`, "secret"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Format struct {
			Slug      string `json:"slug"`
			Duration  int    `json:"duration_minutes"`
			PublicURL string `json:"public_url"`
		} `json:"format"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Format.Slug != "hora-solar" || payload.Format.Duration != 45 {
		t.Fatalf("format = %+v", payload.Format)
	}
	if !strings.HasSuffix(payload.Format.PublicURL, "/book/hora-solar") {
		t.Fatalf("public url = %q", payload.Format.PublicURL)
	}

	duplicate := httptest.NewRecorder()
	api.CreateFormat(duplicate, adminRequest(http.MethodPost, "/api/admin/formats",
		`{"slug":"hora-solar","title":"Outro","duration_minutes":30}`, "secret"))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", duplicate.Code)
	}

	invalid := httptest.NewRecorder()
	api.CreateFormat(invalid, adminRequest(http.MethodPost, "/api/admin/formats",
		`{"slug":"Hora Solar!","title":"X","duration_minutes":30}`, "secret"))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid slug = %d, want 422", invalid.Code)
	}
}
