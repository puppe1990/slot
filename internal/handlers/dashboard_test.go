package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

func TestDashboardHandlerRendersBookingsAndStats(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	dashboard := NewDashboardHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = session.WithUserID(req, env.ownerID)
	rr := httptest.NewRecorder()
	dashboard.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Executive schedule", "Alex Rivera", "/book/30min", "No bookings yet"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestDashboardHandlerIncludesFlash(t *testing.T) {
	env := setupTestEnv(t)
	dashboard := NewDashboardHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = session.WithUserID(req, env.ownerID)
	req = flash.WithMessage(req, flash.Message{Kind: "notice", Message: "Welcome back!"})
	rr := httptest.NewRecorder()
	dashboard.ServeHTTP(rr, req)

	if !strings.Contains(rr.Body.String(), "Welcome back!") {
		t.Errorf("missing flash notice")
	}
}

func TestDashboardHandlerSignedInShowsChrome(t *testing.T) {
	env := setupTestEnv(t)
	dashboard := NewDashboardHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = session.WithUserID(req, env.ownerID)
	rr := httptest.NewRecorder()
	dashboard.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `action="/logout"`) {
		t.Error("signed-in page should offer sign-out")
	}
	if !strings.Contains(body, `href="/setup"`) {
		t.Error("signed-in page should link to availability")
	}
}

func TestDashboardOwnerCancelFreesBooking(t *testing.T) {
	env := setupTestEnv(t)
	meetingType := env.seedMeetingType(t, "30min", 30)
	bookingHandler := NewBookingHandler(env.deps)

	holdReq := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"America/New_York"},
	}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	bookingHandler.Hold(holdRec, holdReq)
	confirmReq := postForm("/book/30min/confirm", url.Values{
		"name":  {"Sarah Jenkins"},
		"email": {"sarah@example.com"},
		"tz":    {"America/New_York"},
	}, holdCookie(holdRec))
	confirmReq.SetPathValue("slug", "30min")
	confirmRec := httptest.NewRecorder()
	bookingHandler.Confirm(confirmRec, confirmReq)

	dashboard := NewDashboardHandler(env.deps)
	cancelReq := postForm("/bookings/1/cancel", url.Values{"reason": {"Conflict"}}, nil)
	cancelReq.SetPathValue("id", "1")
	cancelReq = session.WithUserID(cancelReq, env.ownerID)
	cancelRec := httptest.NewRecorder()
	dashboard.Cancel(cancelRec, cancelReq)

	if cancelRec.Code != http.StatusSeeOther || cancelRec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("cancel = %d %q, want 303 /dashboard", cancelRec.Code, cancelRec.Header().Get("Location"))
	}
	cancelled, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("status = %q, want CANCELLED", cancelled.Status)
	}
	if cancelled.MeetingTypeID != meetingType.ID {
		t.Fatalf("wrong booking cancelled: %+v", cancelled)
	}
}

func TestNotFoundRendersHTML(t *testing.T) {
	env := setupTestEnv(t)
	notFound := NewNotFoundHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	notFound.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "This page is not here") {
		t.Fatalf("404 body is not the rendered page: %s", rec.Body.String())
	}
}
