package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

func seedBooking(t *testing.T, env *testEnv, meetingType models.MeetingType, startISO, name, email string) models.Booking {
	t.Helper()
	start, err := time.Parse(time.RFC3339, startISO)
	if err != nil {
		t.Fatal(err)
	}
	_, holdToken, err := env.store.CreateHold(store.HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         start,
		End:           start.Add(time.Duration(meetingType.DurationMinutes) * time.Minute),
		GuestEmail:    email,
		GuestTimezone: "America/New_York",
		ExpiresAt:     fixedTestNow.Add(10 * time.Minute),
		Now:           fixedTestNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := env.store.ConfirmHold(holdToken, store.GuestInput{
		Name:     name,
		Email:    email,
		Timezone: "America/New_York",
	}, guesttoken.Hash(holdToken), fixedTestNow)
	if err != nil {
		t.Fatal(err)
	}
	return booking
}

func TestOwnerRescheduleMovesBooking(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	booking := seedBooking(t, env, meetingType, "2026-10-26T14:00:00Z", "Sarah Jenkins", "sarah@example.com")
	dashboard := NewDashboardHandler(env.deps)

	pageReq := httptest.NewRequest(http.MethodGet, "/bookings/1/reschedule", nil)
	pageReq.SetPathValue("id", "1")
	pageReq = session.WithUserID(pageReq, env.ownerID)
	pageRec := httptest.NewRecorder()
	dashboard.ReschedulePage(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("reschedule page = %d: %s", pageRec.Code, pageRec.Body.String())
	}
	if !strings.Contains(pageRec.Body.String(), "Sarah Jenkins") {
		t.Fatal("reschedule page is missing the guest")
	}

	moveReq := postForm("/bookings/1/reschedule", url.Values{"slot": {"2026-10-27T14:00:00Z"}}, nil)
	moveReq.SetPathValue("id", "1")
	moveReq = session.WithUserID(moveReq, env.ownerID)
	moveRec := httptest.NewRecorder()
	dashboard.Reschedule(moveRec, moveReq)
	if moveRec.Code != http.StatusSeeOther {
		t.Fatalf("reschedule = %d: %s", moveRec.Code, moveRec.Body.String())
	}

	moved, err := env.store.GetBooking(env.ownerID, booking.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status != models.BookingStatusRescheduled {
		t.Fatalf("status = %q, want RESCHEDULED", moved.Status)
	}
	if moved.StartUTC.Format("2006-01-02T15:04:05Z") != "2026-10-27T14:00:00Z" {
		t.Fatalf("start = %s", moved.StartUTC)
	}
	if _, ok := env.outbox.LastFor("sarah@example.com"); !ok {
		t.Fatal("guest was not notified about the move")
	}
}

func TestOwnerRescheduleRejectsCancelledBooking(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	booking := seedBooking(t, env, meetingType, "2026-10-26T14:00:00Z", "Sarah Jenkins", "sarah@example.com")
	if err := env.store.CancelBooking(booking.ID, models.ActorOwner, "test"); err != nil {
		t.Fatal(err)
	}

	dashboard := NewDashboardHandler(env.deps)
	req := postForm("/bookings/1/reschedule", url.Values{"slot": {"2026-10-27T14:00:00Z"}}, nil)
	req.SetPathValue("id", "1")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	dashboard.Reschedule(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestDashboardFilterShowsOnlyRequestedStatus(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	kept := seedBooking(t, env, meetingType, "2026-10-26T14:00:00Z", "Kept Guest", "kept@example.com")
	dropped := seedBooking(t, env, meetingType, "2026-10-27T14:00:00Z", "Dropped Guest", "dropped@example.com")
	if err := env.store.CancelBooking(dropped.ID, models.ActorOwner, "test"); err != nil {
		t.Fatal(err)
	}
	dashboard := NewDashboardHandler(env.deps)
	// The list is identified by its per-row links: the KPI card above also
	// prints the next guest's name, so matching names would be ambiguous.
	activeRow := fmt.Sprintf("/bookings/%d/invite.ics", kept.ID)
	cancelledRow := fmt.Sprintf("/bookings/%d/invite.ics", dropped.ID)

	upcoming := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	upcoming = session.WithUserID(upcoming, env.ownerID)
	upcomingRec := httptest.NewRecorder()
	dashboard.ServeHTTP(upcomingRec, upcoming)
	body := upcomingRec.Body.String()
	if !strings.Contains(body, activeRow) {
		t.Error("upcoming list is missing the active booking row")
	}
	if strings.Contains(body, cancelledRow) {
		t.Error("upcoming list shows a cancelled booking row")
	}

	cancelled := httptest.NewRequest(http.MethodGet, "/dashboard?status=cancelled&days=30", nil)
	cancelled = session.WithUserID(cancelled, env.ownerID)
	cancelledRec := httptest.NewRecorder()
	dashboard.ServeHTTP(cancelledRec, cancelled)
	body = cancelledRec.Body.String()
	if !strings.Contains(body, cancelledRow) {
		t.Error("cancelled filter is missing the cancelled booking row")
	}
	if strings.Contains(body, activeRow) {
		t.Error("cancelled filter shows an active booking row")
	}
}

func TestDashboardExportRendersEveryBooking(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	seedBooking(t, env, meetingType, "2026-10-26T14:00:00Z", "First Guest", "first@example.com")
	seedBooking(t, env, meetingType, "2026-10-27T14:00:00Z", "Second Guest", "second@example.com")
	dashboard := NewDashboardHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/export.ics?status=upcoming&days=30", nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	dashboard.Export(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "slot-bookings.ics") {
		t.Fatalf("Content-Disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	document := rec.Body.String()
	if got := strings.Count(document, "BEGIN:VEVENT"); got != 2 {
		t.Fatalf("VEVENTs = %d, want 2", got)
	}
}

func TestOwnerInviteRendersSingleBooking(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	booking := seedBooking(t, env, meetingType, "2026-10-26T14:00:00Z", "Sarah Jenkins", "sarah@example.com")
	dashboard := NewDashboardHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/bookings/1/invite.ics", nil)
	req.SetPathValue("id", "1")
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	dashboard.Invite(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("invite = %d", rec.Code)
	}
	document := rec.Body.String()
	if got := strings.Count(document, "BEGIN:VEVENT"); got != 1 {
		t.Fatalf("VEVENTs = %d, want 1", got)
	}
	if !strings.Contains(document, "Sarah Jenkins") {
		t.Fatal("invite is missing the guest")
	}
	_ = booking
}
