package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChooseDateStaysOnTodayWhenLaterDaysHaveSlots(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 10, 28, 0, 0, 0, 0, loc)

	got := chooseDate("", today, 30)
	if got != "2026-10-28" {
		t.Fatalf("chooseDate = %q, want today 2026-10-28 even though only Thursday has slots", got)
	}
}

func TestChooseDateHonorsARequestedDateWithoutSlots(t *testing.T) {
	today := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	got := chooseDate("2026-10-30", today, 30)
	if got != "2026-10-30" {
		t.Fatalf("chooseDate = %q, want the requested date 2026-10-30", got)
	}
}

func TestBookPageOpensOnTodayWhenNoticeClearsTheRestOfTheDay(t *testing.T) {
	env := setupTestEnv(t)
	// Wednesday 16:00 America/New_York (EDT) = 20:00 UTC. A 24h notice window
	// eats the rest of today and lands the first free slot on Thursday 16:00.
	env.deps.Clock = func() time.Time {
		return time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	}
	meetingType := env.seedMeetingType(t, "30min", 30)
	meetingType.MinNoticeHours = 24
	if err := env.store.UpdateMeetingType(meetingType); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/book/30min?tz=America/New_York", nil)
	req.SetPathValue("slug", "30min")
	rec := httptest.NewRecorder()
	NewBookingHandler(env.deps).Show(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, `name="date" value="2026-10-28"`) {
		t.Fatalf("page jumped off today; want hidden date=2026-10-28\n%s", body)
	}
	if strings.Contains(body, `name="date" value="2026-10-29"`) {
		t.Fatal("page selected Thursday, the first day with slots")
	}
	if !strings.Contains(body, "Wednesday, October 28, 2026") {
		t.Fatal("heading should name today, Wednesday 28")
	}
	if !strings.Contains(body, `date=2026-10-28"`) && !strings.Contains(body, "date=2026-10-28") {
		t.Fatal("today's chip should stay a link so the guest can come back to it")
	}
}
