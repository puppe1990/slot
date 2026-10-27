package call_test

import (
	"context"
	"encoding/hex"
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
	if seenBody["privacy"] != "private" {
		t.Fatalf("privacy = %v, want private", seenBody["privacy"])
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
	// The suffix is random hex, so a booking id can appear by chance; assert
	// the shape instead of scanning for digits.
	suffix := strings.TrimPrefix(first.URL, "https://meet.jit.si/slot-")
	if len(suffix) != 24 {
		t.Fatalf("suffix = %q, want 24 hex chars", suffix)
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Fatalf("suffix %q is not hex: %v", suffix, err)
	}
}

func TestDailyRoomURLIssuesGuestToken(t *testing.T) {
	var seenPath string
	var seenBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "jwt-token"})
	}))
	t.Cleanup(server.Close)

	provider, err := call.NewDaily("key", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	room := call.Room{URL: "https://team.daily.co/slot-9-abcd"}
	got, err := provider.RoomURL(context.Background(), room, call.Viewer{
		Name:      "Marina Costa",
		ExpiresAt: testInput().End,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://team.daily.co/slot-9-abcd?t=jwt-token" {
		t.Fatalf("url = %q", got)
	}
	if seenPath != "POST /meeting-tokens" {
		t.Fatalf("request = %q", seenPath)
	}
	properties, _ := seenBody["properties"].(map[string]any)
	if properties["room_name"] != "slot-9-abcd" || properties["user_name"] != "Marina Costa" {
		t.Fatalf("properties = %v", properties)
	}
	if properties["is_owner"] != false {
		t.Fatalf("is_owner = %v, want false", properties["is_owner"])
	}
	exp, _ := properties["exp"].(float64)
	if int64(exp) != testInput().End.Unix() {
		t.Fatalf("exp = %v", properties["exp"])
	}
}

func TestDailyRoomURLMarksTheOwner(t *testing.T) {
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "owner-token"})
	}))
	t.Cleanup(server.Close)

	provider, err := call.NewDaily("key", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.RoomURL(context.Background(), call.Room{URL: "https://team.daily.co/slot-9-abcd"},
		call.Viewer{Name: "Alex", Owner: true}); err != nil {
		t.Fatal(err)
	}
	properties, _ := seenBody["properties"].(map[string]any)
	if properties["is_owner"] != true {
		t.Fatalf("is_owner = %v, want true", properties["is_owner"])
	}
}

func TestJitsiRoomURLIsTheRoom(t *testing.T) {
	provider, err := call.NewJitsi("")
	if err != nil {
		t.Fatal(err)
	}
	room, err := provider.CreateRoom(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}
	got, err := provider.RoomURL(context.Background(), room, call.Viewer{Name: "Guest"})
	if err != nil {
		t.Fatal(err)
	}
	if got != room.URL {
		t.Fatalf("url = %q, want %q", got, room.URL)
	}
}
