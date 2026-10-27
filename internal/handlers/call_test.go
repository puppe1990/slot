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
		"name":  {"Marina Costa"},
		"email": {"marina@example.com"},
		"tz":    {"America/New_York"},
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
	if !strings.Contains(manageRec.Body.String(), fake.created[0]) {
		t.Error("manage page is missing the join link")
	}

	inviteReq := httptest.NewRequest(http.MethodGet, "/manage/"+token+"/invite.ics", nil)
	inviteReq.SetPathValue("token", token)
	inviteRec := httptest.NewRecorder()
	NewManageHandler(env.deps).Invite(inviteRec, inviteReq)
	if !strings.Contains(inviteRec.Body.String(), "URL:"+fake.created[0]) {
		t.Errorf("ics is missing URL:%s", fake.created[0])
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
		"name":  {"Marina Costa"},
		"email": {"marina@example.com"},
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
		"name":  {"Marina Costa"},
		"email": {"marina@example.com"},
	}, holdCookie(holdRec))
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	bookingHandler.Confirm(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusSeeOther {
		t.Fatalf("confirm = %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	guest, ok := env.outbox.LastFor("marina@example.com")
	if !ok {
		t.Fatal("guest email was not queued")
	}
	for _, want := range []string{fake.created[0], "/manage/", "30-Minute Strategy Advisory"} {
		if !strings.Contains(guest.Body, want) && !strings.Contains(guest.Subject, want) {
			t.Errorf("guest email missing %q: %s", want, guest.Body)
		}
	}

	owner, ok := env.outbox.LastFor("owner@example.com")
	if !ok {
		t.Fatal("owner email was not queued")
	}
	if !strings.Contains(owner.Subject, "Marina Costa") {
		t.Errorf("owner subject = %q, want the guest name", owner.Subject)
	}
}
