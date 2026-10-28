package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/scheduling"
)

const googleBaseURL = "https://www.googleapis.com/calendar/v3"

// Google speaks the Calendar REST API with a caller-supplied access token.
// OAuth exchange lives outside this package so the adapter stays testable
// against an httptest server.
type Google struct {
	CalendarID  string
	BaseURL     string
	HTTPClient  *http.Client
	AccessToken func(ctx context.Context) (string, error)
}

func NewGoogle(calendarID string, client *http.Client) (*Google, error) {
	if strings.TrimSpace(calendarID) == "" {
		calendarID = "primary"
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Google{CalendarID: calendarID, BaseURL: googleBaseURL, HTTPClient: client}, nil
}

func (g *Google) Provider() string { return ProviderGoogle }

func (g *Google) ListBusyIntervals(ctx context.Context, from, to time.Time) ([]scheduling.Interval, error) {
	body := map[string]any{
		"timeMin": from.UTC().Format(time.RFC3339),
		"timeMax": to.UTC().Format(time.RFC3339),
		"items":   []map[string]string{{"id": g.CalendarID}},
	}
	var response struct {
		Calendars map[string]struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
			Busy []struct {
				Start time.Time `json:"start"`
				End   time.Time `json:"end"`
			} `json:"busy"`
		} `json:"calendars"`
	}
	if err := g.do(ctx, http.MethodPost, "/freeBusy", body, &response); err != nil {
		return nil, fmt.Errorf("google freeBusy: %w", err)
	}

	// A calendar the credential cannot read comes back as notFound with an
	// empty busy list. Returning it as "free" would double-book the owner
	// silently, so it is an error the owner can see.
	if problems := response.Calendars[g.CalendarID].Errors; len(problems) > 0 {
		return nil, fmt.Errorf("google freeBusy for %q: %s", g.CalendarID, problems[0].Reason)
	}

	busy := response.Calendars[g.CalendarID].Busy
	intervals := make([]scheduling.Interval, 0, len(busy))
	for _, block := range busy {
		intervals = append(intervals, scheduling.Interval{Start: block.Start.UTC(), End: block.End.UTC()})
	}
	return intervals, nil
}

func (g *Google) CreateEvent(ctx context.Context, event Event) (Event, error) {
	attendees := make([]map[string]string, 0, len(event.Attendees))
	for _, email := range event.Attendees {
		attendees = append(attendees, map[string]string{"email": email})
	}
	body := map[string]any{
		"summary":     event.Summary,
		"description": event.Details,
		"location":    event.Location,
		"start":       map[string]string{"dateTime": event.Start.UTC().Format(time.RFC3339)},
		"end":         map[string]string{"dateTime": event.End.UTC().Format(time.RFC3339)},
		"attendees":   attendees,
	}
	var created struct {
		ID string `json:"id"`
	}
	path := "/calendars/" + url.PathEscape(g.CalendarID) + "/events"
	if err := g.do(ctx, http.MethodPost, path, body, &created); err != nil {
		return Event{}, fmt.Errorf("google create event: %w", err)
	}
	event.ID = created.ID
	return event, nil
}

func (g *Google) UpdateEvent(ctx context.Context, id string, start, end time.Time) error {
	body := map[string]any{
		"start": map[string]string{"dateTime": start.UTC().Format(time.RFC3339)},
		"end":   map[string]string{"dateTime": end.UTC().Format(time.RFC3339)},
	}
	path := "/calendars/" + url.PathEscape(g.CalendarID) + "/events/" + url.PathEscape(id)
	if err := g.do(ctx, http.MethodPatch, path, body, nil); err != nil {
		return fmt.Errorf("google update event %s: %w", id, err)
	}
	return nil
}

func (g *Google) DeleteEvent(ctx context.Context, id string) error {
	path := "/calendars/" + url.PathEscape(g.CalendarID) + "/events/" + url.PathEscape(id)
	if err := g.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("google delete event %s: %w", id, err)
	}
	return nil
}

func (g *Google) do(ctx context.Context, method, path string, body, out any) error {
	if g.AccessToken == nil {
		return errors.New("google calendar has no access token source; connect the account first")
	}
	token, err := g.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("google access token: %w", err)
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode google request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, g.BaseURL+path, payload)
	if err != nil {
		return fmt.Errorf("build google request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	response, err := g.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("call google calendar: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("google calendar returned %d: %s", response.StatusCode, strings.TrimSpace(string(snippet)))
	}
	if out == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("decode google response: %w", err)
	}
	return nil
}
