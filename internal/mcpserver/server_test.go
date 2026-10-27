package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/slot/internal/slotcli"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/formats"):
			_, _ = w.Write([]byte(`{"formats":[{"slug":"30min","title":"Strategy","duration_minutes":30,"is_active":true,"public_url":"https://slot/book/30min"}]}`))
		case strings.HasSuffix(r.URL.Path, "/bookings"):
			_, _ = w.Write([]byte(`{"bookings":[{"id":3,"status":"CONFIRMED","guest_name":"Marina","start_utc":"2026-10-28T17:00:00Z"}]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(api.Close)
	return New(slotcli.New(api.URL, "secret"), "test")
}

func exchange(t *testing.T, server *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := server.Run(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	replies := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var reply map[string]any
		if err := json.Unmarshal([]byte(line), &reply); err != nil {
			t.Fatalf("bad reply %q: %v", line, err)
		}
		replies = append(replies, reply)
	}
	return replies
}

func TestInitializeAndToolsList(t *testing.T) {
	replies := exchange(t, newTestServer(t),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	if len(replies) != 2 {
		t.Fatalf("replies = %d, want 2 (notification must not reply)", len(replies))
	}
	info := replies[0]["result"].(map[string]any)
	if info["protocolVersion"] == "" || info["serverInfo"].(map[string]any)["name"] != "slot" {
		t.Fatalf("initialize = %+v", info)
	}
	tools := replies[1]["result"].(map[string]any)["tools"].([]any)
	names := []string{}
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	for _, want := range []string{"slot_bookings_list", "slot_formats_list", "slot_slots_list", "slot_booking_create", "slot_booking_cancel", "slot_booking_reschedule"} {
		if !contains(names, want) {
			t.Errorf("missing tool %s (have %v)", want, names)
		}
	}
}

func TestCallToolReturnsTextAndMarksFailures(t *testing.T) {
	replies := exchange(t, newTestServer(t),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slot_bookings_list","arguments":{"status":"upcoming"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"slot_nope","arguments":{}}}`,
	)
	first := replies[0]["result"].(map[string]any)
	if first["isError"].(bool) {
		t.Fatalf("call failed: %+v", first)
	}
	text := first["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "Marina") {
		t.Fatalf("text = %q", text)
	}

	second := replies[1]["result"].(map[string]any)
	if !second["isError"].(bool) {
		t.Fatalf("unknown tool should be an error: %+v", second)
	}
}

func TestUnknownMethodIsRPCError(t *testing.T) {
	replies := exchange(t, newTestServer(t), `{"jsonrpc":"2.0","id":9,"method":"resources/list"}`)
	if replies[0]["error"] == nil {
		t.Fatalf("want rpc error, got %+v", replies[0])
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
