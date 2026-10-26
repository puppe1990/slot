package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

// The list renders one <.form> per format inside a range. A component override
// that reads a field the row does not have 500s here instead of at boot.
func TestMeetingTypesListRendersAfterCreate(t *testing.T) {
	env := setupTestEnv(t)
	userID, err := env.store.CreateUser("formats@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewMeetingTypeHandler(env.deps)

	form := url.Values{"title": {"30-Minute Strategy Advisory"}, "duration_minutes": {"30"}}
	createReq := httptest.NewRequest(http.MethodPost, "/meeting-types", strings.NewReader(form.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq = session.WithUserID(createReq, userID)
	createRec := httptest.NewRecorder()
	handler.Create(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d: %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/meeting-types", nil)
	listReq = session.WithUserID(listReq, userID)
	listRec := httptest.NewRecorder()
	handler.Index(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listRec.Code, listRec.Body.String())
	}
	body := listRec.Body.String()
	for _, want := range []string{"30-Minute Strategy Advisory", "/book/30min", "Pause"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
}
