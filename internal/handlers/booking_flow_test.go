package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/slot/internal/models"
)

func postForm(path string, values url.Values, cookie *http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

func holdCookie(recorder *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == holdCookieName && cookie.Value != "" {
			return cookie
		}
	}
	return nil
}

func TestBookingFlowHoldConfirmManageCancel(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	booking := NewBookingHandler(env.deps)
	manage := NewManageHandler(env.deps)

	// 1. Public page renders the grid.
	showReq := httptest.NewRequest(http.MethodGet, "/book/30min?tz=Europe/London", nil)
	showReq.SetPathValue("slug", "30min")
	showRec := httptest.NewRecorder()
	booking.Show(showRec, showReq)
	if showRec.Code != http.StatusOK {
		t.Fatalf("show status = %d: %s", showRec.Code, showRec.Body.String())
	}
	if !strings.Contains(showRec.Body.String(), "30-Minute Strategy Advisory") {
		t.Fatal("public page is missing the meeting title")
	}

	// 2. Hold the Monday 14:00Z slot (10:00 in New York).
	holdReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"Europe/London"},
	}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	booking.Hold(holdRec, holdReq)
	if holdRec.Code != http.StatusSeeOther {
		t.Fatalf("hold status = %d: %s", holdRec.Code, holdRec.Body.String())
	}
	if location := holdRec.Header().Get("Location"); !strings.HasPrefix(location, "/book/30min/confirm") {
		t.Fatalf("hold Location = %q", location)
	}
	cookie := holdCookie(holdRec)
	if cookie == nil {
		t.Fatal("hold did not set the slot_hold cookie")
	}

	// 3. A second hold for the same instant collides.
	secondReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"Europe/London"},
	}, nil)
	secondReq.SetPathValue("slug", "30min")
	secondRec := httptest.NewRecorder()
	booking.Hold(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("duplicate hold status = %d, want 409", secondRec.Code)
	}

	// 4. Confirm page shows the held window.
	confirmPageReq := httptest.NewRequest(http.MethodGet, "/book/30min/confirm?tz=Europe/London", nil)
	confirmPageReq.SetPathValue("slug", "30min")
	confirmPageReq.AddCookie(cookie)
	confirmPageRec := httptest.NewRecorder()
	booking.ConfirmPage(confirmPageRec, confirmPageReq)
	if confirmPageRec.Code != http.StatusOK {
		t.Fatalf("confirm page status = %d: %s", confirmPageRec.Code, confirmPageRec.Body.String())
	}
	if !strings.Contains(confirmPageRec.Body.String(), "Slot held for you") {
		t.Fatal("confirm page is missing the hold banner")
	}

	// 5. Validate the guest form before booking.
	invalidReq := postForm("/book/30min/confirm", url.Values{
		"consent": {"1"},
		"name":    {"Sarah Jenkins"},
		"email":   {"not-an-email"},
	}, cookie)
	invalidReq.SetPathValue("slug", "30min")
	invalidRec := httptest.NewRecorder()
	booking.Confirm(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid email status = %d, want 422", invalidRec.Code)
	}

	// 6. Confirm the reservation.
	confirmReq := postForm("/book/30min/confirm", url.Values{
		"consent": {"1"},
		"name":    {"Sarah Jenkins"},
		"email":   {"sarah@example.com"},
		"notes":   {"Reviewing the migration plan."},
		"tz":      {"Europe/London"},
	}, cookie)
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	booking.Confirm(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusSeeOther {
		t.Fatalf("confirm status = %d: %s", confirmRec.Code, confirmRec.Body.String())
	}
	location := confirmRec.Header().Get("Location")
	if !strings.HasPrefix(location, "/manage/") {
		t.Fatalf("confirm Location = %q, want /manage/<token>", location)
	}
	token := strings.TrimPrefix(location, "/manage/")

	// 7. Guest notifications went out.
	if _, ok := env.outbox.LastFor("sarah@example.com"); !ok {
		t.Fatal("guest confirmation was not queued")
	}
	if _, ok := env.outbox.LastFor("owner@example.com"); !ok {
		t.Fatal("owner notice was not queued")
	}

	// 8. Manage page renders the booking.
	manageReq := httptest.NewRequest(http.MethodGet, "/manage/"+token, nil)
	manageReq.SetPathValue("token", token)
	manageRec := httptest.NewRecorder()
	manage.Show(manageRec, manageReq)
	if manageRec.Code != http.StatusOK {
		t.Fatalf("manage status = %d: %s", manageRec.Code, manageRec.Body.String())
	}
	if !strings.Contains(manageRec.Body.String(), "Sarah Jenkins") {
		t.Fatal("manage page is missing the guest name")
	}

	// 9. The ICS download is a calendar file.
	icsReq := httptest.NewRequest(http.MethodGet, "/manage/"+token+"/invite.ics", nil)
	icsReq.SetPathValue("token", token)
	icsRec := httptest.NewRecorder()
	manage.Invite(icsRec, icsReq)
	if icsRec.Code != http.StatusOK {
		t.Fatalf("ics status = %d", icsRec.Code)
	}
	if !strings.Contains(icsRec.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("ics Content-Type = %q", icsRec.Header().Get("Content-Type"))
	}

	// 10. Cancel, and the slot is free again.
	cancelReq := postForm("/manage/"+token+"/cancel", url.Values{"reason": {"Conflict"}}, nil)
	cancelReq.SetPathValue("token", token)
	cancelRec := httptest.NewRecorder()
	manage.Cancel(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusSeeOther {
		t.Fatalf("cancel status = %d: %s", cancelRec.Code, cancelRec.Body.String())
	}

	rebookReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"Europe/London"},
	}, nil)
	rebookReq.SetPathValue("slug", "30min")
	rebookRec := httptest.NewRecorder()
	booking.Hold(rebookRec, rebookReq)
	if rebookRec.Code != http.StatusSeeOther {
		t.Fatalf("rebook status = %d after cancel; the slot was not freed", rebookRec.Code)
	}
}

func TestManageRejectsTamperedToken(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	manage := NewManageHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/manage/12.forged", nil)
	req.SetPathValue("token", "12.forged")
	rec := httptest.NewRecorder()
	manage.Show(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a forged token", rec.Code)
	}
}

func TestManageReschedulesToNewSlot(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	booking := NewBookingHandler(env.deps)
	manage := NewManageHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"America/New_York"},
	}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	booking.Hold(holdRec, holdReq)
	cookie := holdCookie(holdRec)

	confirmReq := postForm("/book/30min/confirm", url.Values{
		"consent": {"1"},
		"name":    {"Sarah Jenkins"},
		"email":   {"sarah@example.com"},
		"tz":      {"America/New_York"},
	}, cookie)
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	booking.Confirm(confirmRec, confirmReq)
	token := strings.TrimPrefix(confirmRec.Header().Get("Location"), "/manage/")

	rescheduleReq := postForm("/manage/"+token+"/reschedule", url.Values{
		"slot": {"2026-10-27T15:00:00Z"},
	}, nil)
	rescheduleReq.SetPathValue("token", token)
	rescheduleRec := httptest.NewRecorder()
	manage.Reschedule(rescheduleRec, rescheduleReq)
	if rescheduleRec.Code != http.StatusSeeOther {
		t.Fatalf("reschedule status = %d: %s", rescheduleRec.Code, rescheduleRec.Body.String())
	}

	booked, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if booked.Status != models.BookingStatusRescheduled {
		t.Fatalf("status = %q, want RESCHEDULED", booked.Status)
	}
	if booked.StartUTC.Format("2006-01-02T15:04:05Z") != "2026-10-27T15:00:00Z" {
		t.Fatalf("start = %s, want 2026-10-27T15:00:00Z", booked.StartUTC)
	}
	if booked.MeetingTypeID != meetingType.ID {
		t.Fatalf("meeting type = %d, want %d", booked.MeetingTypeID, meetingType.ID)
	}
}

func TestBookingShowIs404ForUnknownSlug(t *testing.T) {
	env := setupTestEnv(t)
	booking := NewBookingHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/book/missing", nil)
	req.SetPathValue("slug", "missing")
	rec := httptest.NewRecorder()
	booking.Show(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestReleaseFreesTheHeldSlot(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	booking := NewBookingHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"America/New_York"},
	}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	booking.Hold(holdRec, holdReq)
	cookie := holdCookie(holdRec)

	releaseReq := postForm("/book/30min/release", url.Values{"tz": {"America/New_York"}}, cookie)
	releaseReq.SetPathValue("slug", "30min")
	releaseRec := httptest.NewRecorder()
	booking.Release(releaseRec, releaseReq)
	if releaseRec.Code != http.StatusSeeOther {
		t.Fatalf("release status = %d: %s", releaseRec.Code, releaseRec.Body.String())
	}
	if location := releaseRec.Header().Get("Location"); !strings.HasPrefix(location, "/book/30min") {
		t.Fatalf("release Location = %q", location)
	}

	again := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"America/New_York"},
	}, nil)
	again.SetPathValue("slug", "30min")
	againRec := httptest.NewRecorder()
	booking.Hold(againRec, again)
	if againRec.Code != http.StatusSeeOther {
		t.Fatalf("slot still held after release: status %d", againRec.Code)
	}
}
