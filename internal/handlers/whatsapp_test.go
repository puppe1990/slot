package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/models"
)

func holdFor(t *testing.T, env *testEnv) *http.Cookie {
	t.Helper()
	env.seedMeetingType(t, "30min", 30)
	booking := NewBookingHandler(env.deps)
	req := postForm("/book/30min/hold", url.Values{
		"slot": {"2026-10-26T14:00:00Z"},
		"tz":   {"America/New_York"},
	}, nil)
	req.SetPathValue("slug", "30min")
	rec := httptest.NewRecorder()
	booking.Hold(rec, req)
	cookie := holdCookie(rec)
	if cookie == nil {
		t.Fatalf("hold failed: %d %s", rec.Code, rec.Body.String())
	}
	return cookie
}

func TestConfirmStoresNormalizedWhatsApp(t *testing.T) {
	env := setupTestEnv(t)
	cookie := holdFor(t, env)
	booking := NewBookingHandler(env.deps)

	req := postForm("/book/30min/confirm", url.Values{
		"name":     {"Marina Costa"},
		"email":    {"marina@example.com"},
		"whatsapp": {"+55 (11) 99999-8888"},
		"tz":       {"America/New_York"},
	}, cookie)
	req.SetPathValue("slug", "30min")
	rec := httptest.NewRecorder()
	booking.Confirm(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("confirm = %d: %s", rec.Code, rec.Body.String())
	}

	stored, err := env.store.GetBooking(env.ownerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GuestWhatsApp != "+5511999998888" {
		t.Fatalf("whatsapp = %q, want +5511999998888", stored.GuestWhatsApp)
	}

	dashReq := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	dashReq = session.WithUserID(dashReq, env.ownerID)
	dashRec := httptest.NewRecorder()
	NewDashboardHandler(env.deps).ServeHTTP(dashRec, dashReq)
	if !strings.Contains(dashRec.Body.String(), "https://wa.me/5511999998888") {
		t.Error("dashboard is missing the wa.me link")
	}

	owner, ok := env.outbox.LastFor("owner@example.com")
	if !ok {
		t.Fatal("owner email was not queued")
	}
	if !strings.Contains(owner.Body, "+5511999998888") {
		t.Errorf("owner email missing the number: %s", owner.Body)
	}
}

func TestConfirmRejectsMalformedWhatsApp(t *testing.T) {
	env := setupTestEnv(t)
	cookie := holdFor(t, env)
	booking := NewBookingHandler(env.deps)

	req := postForm("/book/30min/confirm", url.Values{
		"name":     {"Marina Costa"},
		"email":    {"marina@example.com"},
		"whatsapp": {"123"},
		"tz":       {"America/New_York"},
	}, cookie)
	req.SetPathValue("slug", "30min")
	rec := httptest.NewRecorder()
	booking.Confirm(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "8 to 15 digits") {
		t.Error("form is missing the whatsapp error")
	}
	// Validation failure keeps the hold alive for a retry, but must not
	// confirm anything.
	if stored, err := env.store.GetBooking(env.ownerID, 1); err == nil && stored.Status != models.BookingStatusHold {
		t.Fatalf("status = %q, want the row to stay HOLD", stored.Status)
	}
}
