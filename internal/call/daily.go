package call

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dailyBaseURL = "https://api.daily.co/v1"

// Daily creates one room per booking through the REST API. The room name is
// ours (unguessable and idempotent per booking), the API key never leaves the
// server, and the room is ejected after the session ends.
type Daily struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewDaily(apiKey, baseURL string) (*Daily, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("daily api key is required")
	}
	if baseURL == "" {
		baseURL = dailyBaseURL
	}
	return &Daily{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), httpClient: http.DefaultClient}, nil
}

func (d *Daily) Provider() string { return ProviderDaily }

func (d *Daily) CreateRoom(ctx context.Context, input Input) (Room, error) {
	name := roomName(input.BookingID)
	exp := input.End.Add(time.Hour).Unix()
	body := map[string]any{
		"name": name,
		"properties": map[string]any{
			"exp":                exp,
			"eject_at_room_exp":  true,
			"enable_prejoin_ui":  true,
			"enable_chat":        false,
			"enable_screenshare": true,
		},
	}

	var response struct {
		URL string `json:"url"`
	}
	if err := d.do(ctx, http.MethodPost, "/rooms", body, &response); err != nil {
		return Room{}, fmt.Errorf("daily create room: %w", err)
	}
	if response.URL == "" {
		return Room{}, fmt.Errorf("daily create room %q returned no url", name)
	}
	return Room{URL: response.URL, Provider: ProviderDaily}, nil
}

// EndRoom deletes the room. The name lives in the URL we stored, so the
// bookings table needs no second identifier for the vendor.
func (d *Daily) EndRoom(ctx context.Context, room Room) error {
	name := roomNameFromURL(room.URL)
	if name == "" {
		return nil
	}
	if err := d.do(ctx, http.MethodDelete, "/rooms/"+url.PathEscape(name), nil, nil); err != nil && !isNotFound(err) {
		return fmt.Errorf("daily end room %q: %w", name, err)
	}
	return nil
}

func (d *Daily) do(ctx context.Context, method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode daily request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, d.baseURL+path, payload)
	if err != nil {
		return fmt.Errorf("build daily request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+d.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := d.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call daily: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		// Never include the request headers here: they carry the API key.
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 256))
		return statusError{code: response.StatusCode, message: strings.TrimSpace(string(snippet))}
	}
	if out == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("decode daily response: %w", err)
	}
	return nil
}

type statusError struct {
	code    int
	message string
}

func (e statusError) Error() string {
	return fmt.Sprintf("daily returned %d: %s", e.code, e.message)
}

func isNotFound(err error) bool {
	status, ok := err.(statusError)
	return ok && status.code == http.StatusNotFound
}

func roomNameFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return ""
	}
	name := strings.Trim(parsed.Path, "/")
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return name
}
