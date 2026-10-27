package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

func TestSettingsRendersProfileAndPassword(t *testing.T) {
	env := setupTestEnv(t)
	settings := NewSettingsHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	settings.Show(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Your account", "owner@example.com", "Current password", "Change password", "/setup"} {
		if !strings.Contains(body, want) {
			t.Errorf("settings missing %q", want)
		}
	}
}

func TestSettingsSavesProfile(t *testing.T) {
	env := setupTestEnv(t)
	settings := NewSettingsHandler(env.deps)

	req := postForm("/settings", url.Values{
		"name":     {"Alex Rivera"},
		"timezone": {"America/Sao_Paulo"},
	}, nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	settings.Save(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	owner, err := env.store.GetUser(env.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Alex Rivera" || owner.Timezone != "America/Sao_Paulo" {
		t.Fatalf("owner = %+v", owner)
	}
}

func TestSettingsChangesPassword(t *testing.T) {
	env := setupTestEnv(t)
	// The seeded owner uses "hash" as the password hash; create a real one so
	// VerifyPassword can succeed.
	hash, err := session.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.DB().Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, env.ownerID); err != nil {
		t.Fatal(err)
	}
	settings := NewSettingsHandler(env.deps)

	wrong := postForm("/settings/password", url.Values{
		"current_password":      {"nope"},
		"password":              {"new-password-123"},
		"password_confirmation": {"new-password-123"},
	}, nil)
	wrong = session.WithUserID(wrong, env.ownerID)
	wrongRec := httptest.NewRecorder()
	settings.Password(wrongRec, wrong)
	if wrongRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong current = %d, want 422", wrongRec.Code)
	}

	ok := postForm("/settings/password", url.Values{
		"current_password":      {"correct-horse-battery"},
		"password":              {"new-password-123"},
		"password_confirmation": {"new-password-123"},
	}, nil)
	ok = session.WithUserID(ok, env.ownerID)
	okRec := httptest.NewRecorder()
	settings.Password(okRec, ok)
	if okRec.Code != http.StatusSeeOther {
		t.Fatalf("change = %d: %s", okRec.Code, okRec.Body.String())
	}

	owner, err := env.store.GetUser(env.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if !session.VerifyPassword(owner.PasswordHash, "new-password-123") {
		t.Fatal("password was not updated")
	}
}

// Availability must not touch the profile anymore: hours are saved without a
// name and the account keeps what Settings defined.
func TestSetupSaveLeavesProfileAlone(t *testing.T) {
	env := setupTestEnv(t)
	setup := NewSetupHandler(env.deps)

	req := postForm("/setup", url.Values{
		"name":    {"Should Be Ignored"},
		"start_1": {"10:00"},
		"end_1":   {"16:00"},
	}, nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	setup.Save(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	owner, err := env.store.GetUser(env.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Alex Rivera" {
		t.Fatalf("setup overwrote the profile name: %q", owner.Name)
	}
	rules, err := env.store.ListAvailabilityRules(env.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].StartTime != "10:00" {
		t.Fatalf("rules = %+v", rules)
	}
}

func TestHeaderShowsAccountMenu(t *testing.T) {
	env := setupTestEnv(t)
	env.seedMeetingType(t, "30min", 30)
	dashboard := NewDashboardHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = session.WithUserID(req, env.ownerID)
	rec := httptest.NewRecorder()
	dashboard.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{"data-amarra-dropdown-button", "data-amarra-dropdown-menu", `href="/settings"`} {
		if !strings.Contains(body, want) {
			t.Errorf("header missing %q", want)
		}
	}
}
