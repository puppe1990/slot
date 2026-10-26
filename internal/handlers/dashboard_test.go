package handlers

import (
	"net/http"
	"net/http/httptest"
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
