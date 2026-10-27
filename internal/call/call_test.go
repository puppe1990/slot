package call_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/slot/internal/call"
)

func testInput() call.Input {
	return call.Input{
		BookingID: 42,
		Title:     "Advisory",
		Start:     time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC),
	}
}

func TestNewRejectsDailyWithoutKey(t *testing.T) {
	if _, err := call.New(call.Config{Provider: "daily"}); err == nil {
		t.Fatal("daily without DAILY_API_KEY should fail")
	}
}

func TestNewFallsBackToNone(t *testing.T) {
	provider, err := call.New(call.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Provider() != call.ProviderNone {
		t.Fatalf("provider = %q, want none", provider.Provider())
	}
	room, err := provider.CreateRoom(context.Background(), testInput())
	if err != nil || room.URL != "" {
		t.Fatalf("none room = %+v, %v", room, err)
	}
}

func TestDailyCreateRoom(t *testing.T) {
	var seenAuth, seenPath string
	var seenBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"url": "https://team.daily.co/slot-42-deadbeef",
		})
	}))
	t.Cleanup(server.Close)

	provider, err := call.NewDaily("test-key", server.URL)
	if err != nil {
		t.Fatal(err)
	}

	room, err := provider.CreateRoom(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}
	if room.URL != "https://team.daily.co/slot-42-deadbeef" || room.Provider != "daily" {
		t.Fatalf("room = %+v", room)
	}
	if seenAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", seenAuth)
	}
	if seenPath != "POST /rooms" {
		t.Fatalf("request = %q", seenPath)
	}

	name, _ := seenBody["name"].(string)
	if !strings.HasPrefix(name, "slot-42-") {
		t.Fatalf("room name = %q, want slot-42-*", name)
	}
	properties, _ := seenBody["properties"].(map[string]any)
	exp, _ := properties["exp"].(float64)
	if int64(exp) != testInput().End.Add(time.Hour).Unix() {
		t.Fatalf("exp = %v, want end + 1h", properties["exp"])
	}
}

func TestDailyCreateRoomSurfacesStatusWithoutKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	t.Cleanup(server.Close)

	provider, err := call.NewDaily("secret-key", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.CreateRoom(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v, want the status", err)
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatal("error leaked the api key")
	}
}

func TestDailyEndRoomDerivesNameFromURL(t *testing.T) {
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	provider, err := call.NewDaily("key", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.EndRoom(context.Background(), call.Room{URL: "https://team.daily.co/slot-7-abcd"}); err != nil {
		t.Fatal(err)
	}
	if seenPath != "DELETE /rooms/slot-7-abcd" {
		t.Fatalf("request = %q", seenPath)
	}
}

func TestDailyEndRoomToleratesMissingRoom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	provider, err := call.NewDaily("key", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.EndRoom(context.Background(), call.Room{URL: "https://team.daily.co/gone"}); err != nil {
		t.Fatalf("404 should be tolerated: %v", err)
	}
}

func TestJitsiRoomIsUniqueAndUnguessable(t *testing.T) {
	provider, err := call.NewJitsi("")
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.CreateRoom(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.CreateRoom(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(first.URL, "https://meet.jit.si/slot-") {
		t.Fatalf("url = %q", first.URL)
	}
	if first.URL == second.URL {
		t.Fatal("two rooms got the same url")
	}
	if strings.Contains(first.URL, "42") {
		t.Fatalf("url %q leaks the booking id", first.URL)
	}
}
