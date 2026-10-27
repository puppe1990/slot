package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	caisi18n "github.com/puppe1990/amarra-cais/pkg/cais/i18n"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	appi18n "github.com/puppe1990/slot/internal/i18n"
)

// localizedEnv parses both catalogs and wraps a handler in LocaleMiddleware, so
// ?lang=pt reaches both the template `t` and the Go-side copy.
func localizedEnv(t *testing.T) (*testEnv, func(http.Handler) http.Handler) {
	t.Helper()
	env := setupTestEnv(t)
	root := projectRoot(t)
	fsys := os.DirFS(filepath.Join(root, "web", "templates"))
	views, err := view.Load(fsys, appi18n.NewCatalog("en"), appi18n.NewCatalog("en"), appi18n.NewCatalog("pt"))
	if err != nil {
		t.Fatal(err)
	}
	env.deps.Views = views
	catalogs := map[string]*caisi18n.Catalog{
		"en": appi18n.NewCatalog("en"),
		"pt": appi18n.NewCatalog("pt"),
	}
	return env, func(next http.Handler) http.Handler {
		return caisi18n.LocaleMiddleware(catalogs, "en")(next)
	}
}

func TestBookPageRendersPortuguese(t *testing.T) {
	env, middleware := localizedEnv(t)
	env.seedMeetingType(t, "30min", 30)
	booking := NewBookingHandler(env.deps)

	req := httptest.NewRequest(http.MethodGet, "/book/30min?lang=pt&tz=America/Sao_Paulo", nil)
	req.SetPathValue("slug", "30min")
	rec := httptest.NewRecorder()
	middleware(http.HandlerFunc(booking.Show)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`lang="pt-BR"`,
		"Horários no seu fuso",
		"Reservar",
		"Cortesia",
		"PT-BR",
		"segunda-feira, 26 de outubro de 2026",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("pt page missing %q", want)
		}
	}
	// 24h clock for pt-BR: the host slot at 10:00 New York renders as 10:00.
	if !strings.Contains(body, "10:00") {
		t.Errorf("pt page should use a 24h clock")
	}
	if strings.Contains(body, "10:00 AM") {
		t.Errorf("pt page still uses an AM/PM clock")
	}
}

func TestLoginRendersPortuguese(t *testing.T) {
	env, middleware := localizedEnv(t)
	auth := NewAuthHandler(env.deps.Views, env.deps.Store, env.deps.Site, env.deps.Store.Sessions(), env.deps.Config, env.deps.Catalog)

	req := httptest.NewRequest(http.MethodGet, "/login?lang=pt", nil)
	rec := httptest.NewRecorder()
	middleware(http.HandlerFunc(auth.Login)).ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{"Entrar", "Senha", "Esqueceu a senha?"} {
		if !strings.Contains(body, want) {
			t.Errorf("pt login missing %q", want)
		}
	}
}

func TestSetupSaveReadsNumericWeekdayFields(t *testing.T) {
	env := setupTestEnv(t)
	userID, err := env.store.CreateUser("hours@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	setup := NewSetupHandler(env.deps)

	form := url.Values{
		"name":     {"Alex Rivera"},
		"timezone": {"America/New_York"},
		"start_1":  {"09:00"},
		"end_1":    {"17:00"},
		"start_3":  {"10:00"},
		"end_3":    {"15:00"},
		"start_2":  {""},
		"end_2":    {""},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, userID)
	rec := httptest.NewRecorder()
	setup.Save(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d: %s", rec.Code, rec.Body.String())
	}
	rules, err := env.store.ListAvailabilityRules(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2 (Monday and Wednesday)", len(rules))
	}
	for _, rule := range rules {
		if rule.DayOfWeek == 1 && (rule.StartTime != "09:00" || rule.EndTime != "17:00") {
			t.Errorf("Monday = %s-%s", rule.StartTime, rule.EndTime)
		}
	}
}
