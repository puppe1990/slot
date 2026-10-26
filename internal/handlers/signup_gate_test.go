package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSignUpClosesAfterFirstOwner(t *testing.T) {
	env := setupTestEnv(t)
	auth := NewAuthHandler(env.deps.Views, env.deps.Store, env.deps.Site, env.deps.Store.Sessions(), env.deps.Config, env.deps.Catalog)

	pageReq := httptest.NewRequest(http.MethodGet, "/signup", nil)
	pageRec := httptest.NewRecorder()
	auth.SignUp(pageRec, pageReq)
	if pageRec.Code != http.StatusSeeOther || pageRec.Header().Get("Location") != "/login" {
		t.Fatalf("GET /signup with an owner = %d %q, want 303 /login", pageRec.Code, pageRec.Header().Get("Location"))
	}

	postReq := httptest.NewRequest(http.MethodPost, "/signup",
		strings.NewReader(url.Values{
			"email":                 {"intruder@example.com"},
			"password":              {"password123"},
			"password_confirmation": {"password123"},
		}.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	auth.SignUpPost(postRec, postReq)

	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /signup status = %d, want 303", postRec.Code)
	}
	if _, err := env.store.FindUserByEmail("intruder@example.com"); err == nil {
		t.Fatal("signup created a second account after the owner existed")
	}
}

func TestSignUpOpenBeforeFirstOwner(t *testing.T) {
	env := setupTestEnv(t)
	empty := setupTestStore(t)
	fresh := NewAuthHandler(env.deps.Views, empty, env.deps.Site, empty.Sessions(), env.deps.Config, env.deps.Catalog)

	req := httptest.NewRequest(http.MethodGet, "/signup", nil)
	rec := httptest.NewRecorder()
	fresh.SignUp(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /signup on an empty install = %d, want 200", rec.Code)
	}
}
