package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/slot/internal/calendar"
	"github.com/puppe1990/slot/internal/guesttoken"
	appi18n "github.com/puppe1990/slot/internal/i18n"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
	"github.com/puppe1990/slot/internal/store"
)

// fixedTestNow is a Monday noon UTC: 08:00 in New York, so the 09:00 window is
// still ahead of the notice cutoff unless a test says otherwise.
var fixedTestNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

type testEnv struct {
	deps     Deps
	store    store.Store
	calendar *calendar.Fake
	outbox   *notify.Outbox
	signer   *guesttoken.Signer
	ownerID  int64
}

func testSite() meta.Site {
	return meta.Site{AppName: "slot", AppURL: "https://slot.example.com"}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func setupTestViews(t *testing.T) *view.Renderer {
	t.Helper()
	root := projectRoot(t)
	fsys := os.DirFS(filepath.Join(root, "web", "templates"))
	r, err := view.Load(fsys, appi18n.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func setupTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s := setupTestStore(t)
	fakeCalendar := calendar.NewFake()
	outbox := notify.NewOutbox()
	signer, err := guesttoken.New("test-secret-at-least-sixteen-bytes")
	if err != nil {
		t.Fatal(err)
	}
	env := &testEnv{
		store:    s,
		calendar: fakeCalendar,
		outbox:   outbox,
		signer:   signer,
		deps: Deps{
			Views:    setupTestViews(t),
			Store:    s,
			Site:     testSite(),
			Catalog:  appi18n.DefaultCatalog(),
			Catalogs: map[string]*i18n.Catalog{"en": appi18n.DefaultCatalog(), "pt": appi18n.NewCatalog("pt")},
			Config:   cais.Config{Env: "test"},
			Calendar: fakeCalendar,
			Notifier: outbox,
			Signer:   signer,
			Clock:    func() time.Time { return fixedTestNow },
		},
	}
	env.ownerID = env.seedOwner(t)
	return env
}

// seedOwner mirrors the /setup submit: a profile plus a Monday-Friday 09:00-17:00
// grid in New York.
func (e *testEnv) seedOwner(t *testing.T) int64 {
	t.Helper()
	userID, err := e.store.CreateUser("owner@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.UpdateOwnerProfile(userID, "Alex Rivera", "America/New_York"); err != nil {
		t.Fatal(err)
	}
	rules := make([]models.AvailabilityRule, 0, 5)
	for day := 1; day <= 5; day++ {
		rules = append(rules, models.AvailabilityRule{
			DayOfWeek: day,
			StartTime: "09:00",
			EndTime:   "17:00",
			IsActive:  true,
		})
	}
	if err := e.store.ReplaceAvailabilityRules(userID, rules); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SaveCalendarConnection(models.CalendarConnection{
		UserID:       userID,
		Provider:     models.CalendarProviderFake,
		AccountEmail: "owner@example.com",
		SyncStatus:   "ACTIVE",
		LastSyncedAt: fixedTestNow,
	}); err != nil {
		t.Fatal(err)
	}
	return userID
}

func (e *testEnv) seedMeetingType(t *testing.T, slug string, duration int) models.MeetingType {
	t.Helper()
	id, err := e.store.CreateMeetingType(models.MeetingType{
		UserID:              e.ownerID,
		Slug:                slug,
		Title:               "30-Minute Strategy Advisory",
		Description:         "Focused tactical review of architecture and roadmap.",
		DurationMinutes:     duration,
		BufferBeforeMinutes: 0,
		BufferAfterMinutes:  0,
		MinNoticeHours:      0,
		BookingHorizonDays:  30,
		LocationType:        models.LocationGoogleMeet,
		IsActive:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	meetingType, err := e.store.GetMeetingType(e.ownerID, id)
	if err != nil {
		t.Fatal(err)
	}
	return meetingType
}
