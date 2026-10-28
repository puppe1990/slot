package calendar_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/slot/internal/calendar"
)

func newGoogleTestProvider(t *testing.T, handler http.HandlerFunc) *calendar.Google {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	provider, err := calendar.NewGoogle("primary", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	provider.BaseURL = server.URL
	provider.AccessToken = func(context.Context) (string, error) { return "test-token", nil }
	return provider
}

func TestGoogleListBusyIntervals(t *testing.T) {
	provider := newGoogleTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/freeBusy" {
			t.Errorf("%s %s, want POST /freeBusy", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"calendars": map[string]any{
				"primary": map[string]any{
					"busy": []map[string]string{
						{"start": "2026-10-27T14:00:00Z", "end": "2026-10-27T14:30:00Z"},
					},
				},
			},
		})
	})

	busy, err := provider.ListBusyIntervals(context.Background(), at("2026-10-27T00:00:00Z"), at("2026-10-28T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 || !busy[0].Start.Equal(at("2026-10-27T14:00:00Z")) {
		t.Fatalf("busy = %+v", busy)
	}
}

func TestGoogleCreateEventReturnsID(t *testing.T) {
	provider := newGoogleTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendars/primary/events" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["summary"] != "Advisory" {
			t.Errorf("summary = %v", body["summary"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "evt-42"})
	})

	event, err := provider.CreateEvent(context.Background(), calendar.Event{
		Summary: "Advisory",
		Start:   at("2026-10-27T14:00:00Z"),
		End:     at("2026-10-27T14:30:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "evt-42" {
		t.Fatalf("event.ID = %q, want evt-42", event.ID)
	}
}

func TestGoogleUpdateAndDeleteEvent(t *testing.T) {
	seen := map[string]string{}
	provider := newGoogleTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method] = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	if err := provider.UpdateEvent(context.Background(), "evt-7", at("2026-10-27T15:00:00Z"), at("2026-10-27T15:30:00Z")); err != nil {
		t.Fatal(err)
	}
	if err := provider.DeleteEvent(context.Background(), "evt-7"); err != nil {
		t.Fatal(err)
	}
	if seen[http.MethodPatch] != "/calendars/primary/events/evt-7" {
		t.Fatalf("patch path = %q", seen[http.MethodPatch])
	}
	if seen[http.MethodDelete] != "/calendars/primary/events/evt-7" {
		t.Fatalf("delete path = %q", seen[http.MethodDelete])
	}
}

func TestGoogleRequiresTokenSource(t *testing.T) {
	provider, err := calendar.NewGoogle("primary", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ListBusyIntervals(context.Background(), at("2026-10-27T00:00:00Z"), at("2026-10-28T00:00:00Z")); err == nil {
		t.Fatal("ListBusyIntervals worked without a token source")
	}
}

func TestGoogleFreeBusySurfacesUnreadableCalendar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"calendars":{"owner@example.com":{"errors":[{"domain":"global","reason":"notFound"}],"busy":[]}}}`))
	}))
	defer server.Close()

	provider := &calendar.Google{
		CalendarID:  "owner@example.com",
		BaseURL:     server.URL,
		HTTPClient:  server.Client(),
		AccessToken: func(context.Context) (string, error) { return "token", nil },
	}
	_, err := provider.ListBusyIntervals(context.Background(), time.Now(), time.Now().Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "notFound") {
		t.Fatalf("err = %v, want notFound surfaced", err)
	}
}

func TestGoogleAttendeesFollowTheCredential(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"id":"evt"}`))
	}))
	defer server.Close()

	event := calendar.Event{
		Summary:   "Purple Stock · Marina",
		Start:     time.Now(),
		End:       time.Now().Add(30 * time.Minute),
		Attendees: []string{"marina@example.com", "owner@example.com"},
	}

	serviceAccount := &calendar.Google{
		CalendarID: "owner@example.com", BaseURL: server.URL, HTTPClient: server.Client(),
		AccessToken:     func(context.Context) (string, error) { return "t", nil },
		InviteAttendees: false,
	}
	if _, err := serviceAccount.CreateEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if _, ok := bodies[0]["attendees"]; ok {
		t.Fatal("service account sent attendees; Google answers 403 forbiddenForServiceAccounts")
	}

	oauth := &calendar.Google{
		CalendarID: "owner@example.com", BaseURL: server.URL, HTTPClient: server.Client(),
		AccessToken:     func(context.Context) (string, error) { return "t", nil },
		InviteAttendees: true,
	}
	if _, err := oauth.CreateEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if _, ok := bodies[1]["attendees"]; !ok {
		t.Fatal("oauth flow dropped the attendees")
	}
}
