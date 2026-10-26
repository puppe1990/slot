package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

func TestHomeHandlerSendsVisitorToPublishedFormat(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	home := NewHomeHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	home.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	if location := rr.Header().Get("Location"); location != "/book/30min" {
		t.Fatalf("Location = %q, want /book/30min", location)
	}
}

func TestHomeHandlerShowsLandingWithoutPublishedFormat(t *testing.T) {
	env := setupTestEnv(t)
	home := NewHomeHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	home.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "No public format yet") {
		t.Errorf("landing missing empty state")
	}
	if strings.Contains(body, `action="/logout"`) {
		t.Error("anonymous landing must not offer sign-out")
	}
}

func TestHomeHandlerSendsOwnerToDashboard(t *testing.T) {
	env := setupTestEnv(t)
	home := NewHomeHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = session.WithUserID(req, env.ownerID)
	rr := httptest.NewRecorder()
	home.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/dashboard" {
		t.Fatalf("status = %d location = %q, want 303 /dashboard", rr.Code, rr.Header().Get("Location"))
	}
}
