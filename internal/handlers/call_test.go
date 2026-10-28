package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/call"
	"github.com/puppe1990/slot/internal/models"
)

type fakeCall struct {
	created []string
	ended   []string
}

func (f *fakeCall) Provider() string { return "fake" }

func (f *fakeCall) CreateRoom(_ context.Context, input call.Input) (call.Room, error) {
	roomURL := fmt.Sprintf("https://call.example/%d", input.BookingID)
	f.created = append(f.created, roomURL)
	return call.Room{URL: roomURL, Provider: "fake"}, nil
}

func (f *fakeCall) RoomURL(_ context.Context, room call.Room, viewer call.Viewer) (string, error) {
	if room.URL == "" {
		return "", nil
	}
	if viewer.Owner {
		return room.URL + "?t=owner", nil
	}
	return room.URL + "?t=guest", nil
}

func (f *fakeCall) EndRoom(_ context.Context, room call.Room) error {
	f.ended = append(f.ended, room.URL)
	return nil
}

// The room is issued at confirmation, stored on the booking, shown to the guest
// and released when the booking is cancelled.
func TestConfirmIssuesRoomAndCancelEndsIt(t *testing.T) {
	env := setupTestEnv(t)
	fake := &fakeCall{}
	env.deps.Call = fake
	env.seedMeetingType(t, "30min", 30)
	bookingHandler := NewBookingHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"America/New_York"},
	}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	bookingHandler.Hold(holdRec, holdReq)

	confirmReq := postForm("/book/30min/confirm", url.Values{
		"consent": {"1"},
		"name":    {"Marina Costa"},
		"email":   {"marina@example.com"},
		"tz":      {"America/New_York"},
	}, holdCookie(holdRec))
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	bookingHandler.Confirm(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusSeeOther {
		t.Fatalf("confirm = %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	if len(fake.created) != 1 {
		t.Fatalf("rooms created = %d, want 1", len(fake.created))
	}
	booking, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if booking.MeetingURL != fake.created[0] {
		t.Fatalf("booking url = %q, want %q", booking.MeetingURL, fake.created[0])
	}

	token := strings.TrimPrefix(confirmRec.Header().Get("Location"), "/manage/")
	manageReq := httptest.NewRequest(http.MethodGet, "/manage/"+token, nil)
	manageReq.SetPathValue("token", token)
	manageRec := httptest.NewRecorder()
	NewManageHandler(env.deps).Show(manageRec, manageReq)
	// The page links to the signed join route, never to the private room.
	joinPath := "/manage/" + token + "/join"
	if !strings.Contains(manageRec.Body.String(), joinPath) {
		t.Errorf("manage page is missing %s", joinPath)
	}

	inviteReq := httptest.NewRequest(http.MethodGet, "/manage/"+token+"/invite.ics", nil)
	inviteReq.SetPathValue("token", token)
	inviteRec := httptest.NewRecorder()
	NewManageHandler(env.deps).Invite(inviteRec, inviteReq)
	wantInviteURL := "URL:https://slot.example.com" + joinPath
	if !strings.Contains(inviteRec.Body.String(), wantInviteURL) {
		t.Errorf("ics is missing %s", wantInviteURL)
	}

	cancelReq := postForm("/bookings/1/cancel", url.Values{}, nil)
	cancelReq.SetPathValue("id", "1")
	cancelReq = session.WithUserID(cancelReq, env.ownerID)
	cancelRec := httptest.NewRecorder()
	NewDashboardHandler(env.deps).Cancel(cancelRec, cancelReq)
	if len(fake.ended) != 1 || fake.ended[0] != fake.created[0] {
		t.Fatalf("ended rooms = %v, want the created room", fake.ended)
	}
}

func TestServerWithoutCallProviderStoresNoRoom(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	bookingHandler := NewBookingHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{"slot": {"2026-10-26T14:00:00Z"}}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	bookingHandler.Hold(holdRec, holdReq)

	confirmReq := postForm("/book/30min/confirm", url.Values{
		"consent": {"1"},
		"name":    {"Marina Costa"},
		"email":   {"marina@example.com"},
	}, holdCookie(holdRec))
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	bookingHandler.Confirm(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusSeeOther {
		t.Fatalf("confirm = %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	booking, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if booking.MeetingURL != "" {
		t.Fatalf("meeting url = %q, want empty without a provider", booking.MeetingURL)
	}
}

// The confirmation email must carry the room and the signed link, and the
// owner gets a separate notice so the consultant sees the booking without
// opening the dashboard.
func TestConfirmationEmailsCarryRoomAndManageLink(t *testing.T) {
	env := setupTestEnv(t)
	fake := &fakeCall{}
	env.deps.Call = fake
	env.seedMeetingType(t, "30min", 30)
	bookingHandler := NewBookingHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{"slot": {"2026-10-26T14:00:00Z"}}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	bookingHandler.Hold(holdRec, holdReq)

	confirmReq := postForm("/book/30min/confirm", url.Values{
		"consent":  {"1"},
		"name":     {"Marina Costa"},
		"email":    {"marina@example.com"},
		"whatsapp": {"+55 11 99999-8888"},
		"notes":    {"Review the migration plan."},
	}, holdCookie(holdRec))
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	bookingHandler.Confirm(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusSeeOther {
		t.Fatalf("confirm = %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	joinPath := confirmRec.Header().Get("Location") + "/join"

	guest, ok := env.outbox.LastFor("marina@example.com")
	if !ok {
		t.Fatal("guest email was not queued")
	}
	for _, want := range []string{joinPath, "/manage/", "30-Minute Strategy Advisory"} {
		if !strings.Contains(guest.Body, want) && !strings.Contains(guest.Subject, want) {
			t.Errorf("guest email missing %q: %s", want, guest.Body)
		}
	}

	for _, want := range []string{"#FAF9F6", "#1B382B", "Slot", joinPath} {
		if !strings.Contains(guest.HTML, want) {
			t.Errorf("guest html missing %q", want)
		}
	}
	if guest.HTML == "" {
		t.Fatal("guest email has no html part")
	}

	owner, ok := env.outbox.LastFor("owner@example.com")
	if !ok {
		t.Fatal("owner email was not queued")
	}
	if !strings.Contains(owner.Subject, "Marina Costa") {
		t.Errorf("owner subject = %q, want the guest name", owner.Subject)
	}
	// The owner needs the guest's contacts and brief without opening the app.
	// html/template escapes the leading + as &#43;; the digits are the stable part.
	for _, want := range []string{"marina@example.com", "5511999998888", "Review the migration plan."} {
		if !strings.Contains(owner.Body, want) {
			t.Errorf("owner body missing %q: %s", want, owner.Body)
		}
		if !strings.Contains(owner.HTML, want) {
			t.Errorf("owner html missing %q", want)
		}
	}
	if !strings.Contains(owner.HTML, "Guest") {
		t.Error("owner html is missing the guest block")
	}
}

func bookingForJoin(t *testing.T, env *testEnv) (models.Booking, string) {
	t.Helper()
	meetingType := env.seedMeetingType(t, "30min", 30)
	_ = meetingType
	bookingHandler := NewBookingHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{"slot": {"2026-10-26T14:00:00Z"}}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	bookingHandler.Hold(holdRec, holdReq)

	confirmReq := postForm("/book/30min/confirm", url.Values{
		"consent": {"1"},
		"name":    {"Marina Costa"},
		"email":   {"marina@example.com"},
	}, holdCookie(holdRec))
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	bookingHandler.Confirm(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusSeeOther {
		t.Fatalf("confirm = %d: %s", confirmRec.Code, confirmRec.Body.String())
	}
	booking, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return booking, strings.TrimPrefix(confirmRec.Header().Get("Location"), "/manage/")
}

func TestGuestJoinRedirectsWithToken(t *testing.T) {
	env := setupTestEnv(t)
	env.deps.Call = &fakeCall{}
	_, token := bookingForJoin(t, env)

	req := httptest.NewRequest(http.MethodGet, "/manage/"+token+"/join", nil)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	NewManageHandler(env.deps).Join(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("join = %d: %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "https://call.example/1?t=guest" {
		t.Fatalf("Location = %q", location)
	}
}

func TestOwnerJoinGetsOwnerToken(t *testing.T) {
	env := setupTestEnv(t)
	env.deps.Call = &fakeCall{}
	bookingForJoin(t, env)

	req := httptest.NewRequest(http.MethodGet, "/bookings/1/join", nil)
	req.SetPathValue("id", "1")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	NewDashboardHandler(env.deps).Join(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("join = %d: %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "https://call.example/1?t=owner" {
		t.Fatalf("Location = %q, want the owner token", location)
	}
}

func TestRSVPRecordsAttendance(t *testing.T) {
	env := setupTestEnv(t)
	_, token := bookingForJoin(t, env)

	req := postForm("/manage/"+token+"/rsvp", url.Values{}, nil)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	NewManageHandler(env.deps).RSVP(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/manage/"+token {
		t.Fatalf("rsvp = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	booking, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if booking.Attendance != models.AttendanceGoing {
		t.Fatalf("attendance = %q, want GOING", booking.Attendance)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/manage/"+token, nil)
	pageReq.SetPathValue("token", token)
	pageRec := httptest.NewRecorder()
	NewManageHandler(env.deps).Show(pageRec, pageReq)
	if !strings.Contains(pageRec.Body.String(), "Attendance confirmed") {
		t.Error("manage page does not show the RSVP state")
	}

	dashReq := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	dashReq = session.WithUserID(dashReq, env.ownerID)
	dashRec := httptest.NewRecorder()
	NewDashboardHandler(env.deps).ServeHTTP(dashRec, dashReq)
	if !strings.Contains(dashRec.Body.String(), "Attendance confirmed") {
		t.Error("dashboard does not show the RSVP badge")
	}
}
