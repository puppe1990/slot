package slotcli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSendsBearerAndDecodes(t *testing.T) {
	var seenAuth, seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(`{"bookings":[{"id":7,"status":"CONFIRMED","guest_name":"Marina","start_utc":"2026-10-28T17:00:00Z"}]}`))
	}))
	defer server.Close()

	bookings, err := New(server.URL, "secret").Bookings(context.Background(), "upcoming", 30)
	if err != nil {
		t.Fatal(err)
	}
	if seenAuth != "Bearer secret" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenPath, "/api/admin/bookings") || !strings.Contains(seenPath, "status=upcoming") {
		t.Fatalf("path = %q", seenPath)
	}
	if len(bookings) != 1 || bookings[0].ID != 7 || bookings[0].GuestName != "Marina" {
		t.Fatalf("bookings = %+v", bookings)
	}
}

func TestClientSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"slot unavailable: 2026-10-28T17:00:00Z"}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "secret").CreateBooking(context.Background(), CreateBookingInput{Format: "30min"})
	if err == nil || !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "slot unavailable") {
		t.Fatalf("err = %v", err)
	}
}
