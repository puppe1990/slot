package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/slot/internal/notify"
)

func TestResendSendPostsMessage(t *testing.T) {
	var seenAuth, seenPath string
	var seenBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "abc-123"})
	}))
	t.Cleanup(server.Close)

	notifier, err := notify.NewResend("re_test_key", "Slot <slot@example.com>", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = notifier.Send(context.Background(), notify.Message{
		Kind:    "booking.confirmed",
		To:      "guest@example.com",
		Subject: "Confirmed: Advisory",
		Body:    "See you there",
	})
	if err != nil {
		t.Fatal(err)
	}

	if seenAuth != "Bearer re_test_key" {
		t.Fatalf("Authorization = %q", seenAuth)
	}
	if seenPath != "POST /emails" {
		t.Fatalf("request = %q", seenPath)
	}
	if seenBody["from"] != "Slot <slot@example.com>" {
		t.Fatalf("from = %v", seenBody["from"])
	}
	if seenBody["subject"] != "Confirmed: Advisory" || seenBody["text"] != "See you there" {
		t.Fatalf("body = %v", seenBody)
	}
	to, _ := seenBody["to"].([]any)
	if len(to) != 1 || to[0] != "guest@example.com" {
		t.Fatalf("to = %v", seenBody["to"])
	}
}

func TestResendErrorDoesNotLeakKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"API key is invalid"}`))
	}))
	t.Cleanup(server.Close)

	notifier, err := notify.NewResend("re_secret_key", "Slot <slot@example.com>", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = notifier.Send(context.Background(), notify.Message{To: "guest@example.com", Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v, want the status", err)
	}
	if strings.Contains(err.Error(), "re_secret_key") {
		t.Fatal("error leaked the api key")
	}
}

func TestResendSkipsEmptyRecipient(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(server.Close)

	notifier, err := notify.NewResend("re_key", "Slot <slot@example.com>", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := notifier.Send(context.Background(), notify.Message{To: "  "}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("empty recipient should not hit the API")
	}
}
