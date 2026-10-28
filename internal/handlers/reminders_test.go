package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	appi18n "github.com/puppe1990/slot/internal/i18n"

	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

// confirmUntil posts the confirm form and returns the recorder, so a test can
// assert both the consent gate and the booking it produces.
func confirmUntil(t *testing.T, env *testEnv, booking *BookingHandler, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	meetingType := env.seedMeetingType(t, "30min", 30)
	start := fixedTestNow.Add(2 * time.Hour)

	holdReq := postForm("/book/30min/hold", url.Values{
		"slot": {start.Format(time.RFC3339)},
		"tz":   {"America/New_York"},
	}, nil)
	holdReq.SetPathValue("slug", "30min")
	holdRec := httptest.NewRecorder()
	booking.Hold(holdRec, holdReq)
	hold := holdCookie(holdRec)
	if hold == nil {
		t.Fatalf("no hold cookie: %d %s", holdRec.Code, holdRec.Body.String())
	}
	if holdRec.Code != http.StatusSeeOther {
		t.Fatalf("hold status = %d: %s", holdRec.Code, holdRec.Body.String())
	}
	_ = meetingType

	confirmReq := postForm("/book/30min/confirm", values, hold)
	confirmReq.SetPathValue("slug", "30min")
	rec := httptest.NewRecorder()
	booking.Confirm(rec, confirmReq)
	return rec
}

func TestConfirmRequiresConsent(t *testing.T) {
	env := setupTestEnv(t)
	booking := NewBookingHandler(env.deps)

	rec := confirmUntil(t, env, booking, url.Values{
		"name":  {"Marina Costa"},
		"email": {"marina@example.com"},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("without consent = %d, want 422", rec.Code)
	}
	bookings, err := env.store.ListBookings(env.ownerID, store.BookingFilter{Status: models.BookingStatusConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if len(bookings) != 0 {
		t.Fatalf("booking was created without consent: %+v", bookings)
	}
}

func TestConfirmRecordsConsent(t *testing.T) {
	env := setupTestEnv(t)
	booking := NewBookingHandler(env.deps)

	rec := confirmUntil(t, env, booking, url.Values{
		"name":    {"Marina Costa"},
		"email":   {"marina@example.com"},
		"consent": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("with consent = %d: %s", rec.Code, rec.Body.String())
	}
	bookings, err := env.store.ListBookings(env.ownerID, store.BookingFilter{Status: models.BookingStatusConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if len(bookings) != 1 {
		t.Fatalf("bookings = %+v", bookings)
	}
	if bookings[0].ConsentAt.IsZero() || !bookings[0].ConsentAt.Equal(fixedTestNow) {
		t.Fatalf("consent_at = %v, want %v", bookings[0].ConsentAt, fixedTestNow)
	}
}

// seedConfirmed books straight through the store, which is what the reminder
// job sees in production.
func seedConfirmed(t *testing.T, env *testEnv, start time.Time) models.Booking {
	t.Helper()
	meetingType, err := env.store.GetMeetingTypeBySlug("30min")
	if err != nil {
		meetingType = env.seedMeetingType(t, "30min", 30)
	}
	hold, holdToken, err := env.store.CreateHold(store.HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         start,
		End:           start.Add(30 * time.Minute),
		GuestEmail:    "marina@example.com",
		GuestTimezone: "America/New_York",
		ExpiresAt:     fixedTestNow.Add(10 * time.Minute),
		Now:           fixedTestNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := env.store.ConfirmHold(holdToken, store.GuestInput{
		Name:      "Marina Costa",
		Email:     "marina@example.com",
		Timezone:  "America/New_York",
		ConsentAt: fixedTestNow,
		Locale:    "pt",
	}, guesttoken.Hash(env.signer.Sign(hold.ID)), fixedTestNow)
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

func TestReminderSendsOnce(t *testing.T) {
	env := setupTestEnv(t)
	seedConfirmed(t, env, fixedTestNow.Add(2*time.Hour))

	service := NewReminderService(env.deps)
	sent, err := service.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("first pass sent %d, want 1", sent)
	}
	if subject := env.outbox.Messages()[0].Subject; !strings.HasPrefix(subject, "Lembrete:") {
		t.Fatalf("reminder did not use the guest locale: %q", subject)
	}
	messages := env.outbox.Messages()
	if len(messages) != 1 || messages[0].Kind != "booking.reminder" {
		t.Fatalf("messages = %+v", messages)
	}
	if !strings.Contains(messages[0].Body, "/manage/") {
		t.Fatalf("reminder has no manage link: %s", messages[0].Body)
	}

	sent, err = service.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 || len(env.outbox.Messages()) != 1 {
		t.Fatalf("second pass duplicated the reminder: sent=%d total=%d", sent, len(env.outbox.Messages()))
	}
}

func TestReminderSkipsFarAndCancelled(t *testing.T) {
	env := setupTestEnv(t)
	seedConfirmed(t, env, fixedTestNow.Add(72*time.Hour))
	cancelled := seedConfirmed(t, env, fixedTestNow.Add(3*time.Hour))
	if err := env.store.CancelBooking(cancelled.ID, models.ActorOwner, "test"); err != nil {
		t.Fatal(err)
	}

	service := NewReminderService(env.deps)
	sent, err := service.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("sent %d reminders, want 0", sent)
	}
}

func TestSettingsStillReachableAfterConsent(t *testing.T) {
	// Guard against the consent gate leaking into owner routes.
	env := setupTestEnv(t)
	settings := NewSettingsHandler(env.deps)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	settings.Show(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings = %d", rec.Code)
	}
}

func TestReminderUsesTheGuestLocale(t *testing.T) {
	env := setupTestEnv(t)
	ptCatalog := appi18n.NewCatalog("pt")
	env.deps.Catalogs = map[string]*i18n.Catalog{"en": appi18n.NewCatalog("en"), "pt": ptCatalog}

	meetingType, err := env.store.GetMeetingTypeBySlug("30min")
	if err != nil {
		meetingType = env.seedMeetingType(t, "30min", 30)
	}
	start := fixedTestNow.Add(2 * time.Hour)
	hold, holdToken, err := env.store.CreateHold(store.HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         start,
		End:           start.Add(30 * time.Minute),
		GuestEmail:    "marina@example.com",
		GuestTimezone: "America/Sao_Paulo",
		ExpiresAt:     fixedTestNow.Add(10 * time.Minute),
		Now:           fixedTestNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.ConfirmHold(holdToken, store.GuestInput{
		Name: "Marina Costa", Email: "marina@example.com", Timezone: "America/Sao_Paulo",
		ConsentAt: fixedTestNow, Locale: "pt",
	}, guesttoken.Hash(env.signer.Sign(hold.ID)), fixedTestNow); err != nil {
		t.Fatal(err)
	}

	service := NewReminderService(env.deps)
	if _, err := service.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if subject := env.outbox.Messages()[0].Subject; !strings.HasPrefix(subject, "Lembrete:") {
		t.Fatalf("subject = %q, want Portuguese", subject)
	}
}
