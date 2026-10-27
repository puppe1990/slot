// Package slotcli is the HTTP client shared by the `slot` CLI and the MCP
// server. Both talk to the admin API with a Bearer token, so neither of them
// touches the database.
package slotcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

type Booking struct {
	ID              int64  `json:"id"`
	Status          string `json:"status"`
	Title           string `json:"title"`
	Slug            string `json:"slug"`
	DurationMinutes int    `json:"duration_minutes"`
	GuestName       string `json:"guest_name"`
	GuestEmail      string `json:"guest_email"`
	GuestWhatsApp   string `json:"guest_whatsapp"`
	GuestNotes      string `json:"guest_notes"`
	GuestTimezone   string `json:"guest_timezone"`
	StartUTC        string `json:"start_utc"`
	EndUTC          string `json:"end_utc"`
	Attendance      string `json:"attendance"`
	MeetingURL      string `json:"meeting_url"`
	PublicURL       string `json:"public_url"`
	JoinURL         string `json:"join_url"`
}

func (b Booking) When() string {
	start, err := time.Parse(time.RFC3339, b.StartUTC)
	if err != nil {
		return b.StartUTC
	}
	return start.Format("2006-01-02 15:04 MST")
}

type Format struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	DurationMinutes int    `json:"duration_minutes"`
	IsActive        bool   `json:"is_active"`
	PublicURL       string `json:"public_url"`
	MinNoticeHours  int    `json:"min_notice_hours"`
	HorizonDays     int    `json:"horizon_days"`
}

type Slot struct {
	Date       string `json:"date"`
	StartUTC   string `json:"start_utc"`
	EndUTC     string `json:"end_utc"`
	GuestClock string `json:"guest_clock"`
	GuestRange string `json:"guest_range"`
	HostRange  string `json:"host_range"`
	Timezone   string `json:"timezone"`
}

type CreateBookingInput struct {
	Format   string `json:"format"`
	Slot     string `json:"slot"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	WhatsApp string `json:"whatsapp,omitempty"`
	Notes    string `json:"notes,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type CreatedBooking struct {
	Booking   Booking `json:"booking"`
	ManageURL string  `json:"manage_url"`
}

func (c *Client) Bookings(ctx context.Context, status string, days int) ([]Booking, error) {
	query := "?status=" + urlQuery(status, "upcoming") + "&days=" + strconv.Itoa(orDefault(days, 90))
	var payload struct {
		Bookings []Booking `json:"bookings"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/admin/bookings"+query, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Bookings, nil
}

func (c *Client) Formats(ctx context.Context) ([]Format, error) {
	var payload struct {
		Formats []Format `json:"formats"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/admin/formats", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Formats, nil
}

func (c *Client) Slots(ctx context.Context, format, timezone string, days int) ([]Slot, error) {
	query := "?format=" + urlQuery(format, "") + "&days=" + strconv.Itoa(days)
	if timezone != "" {
		query += "&tz=" + urlQuery(timezone, "")
	}
	var payload struct {
		Slots []Slot `json:"slots"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/admin/slots"+query, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Slots, nil
}

func (c *Client) CreateBooking(ctx context.Context, input CreateBookingInput) (CreatedBooking, error) {
	var payload CreatedBooking
	if err := c.do(ctx, http.MethodPost, "/api/admin/bookings", input, &payload); err != nil {
		return CreatedBooking{}, err
	}
	return payload, nil
}

func (c *Client) Cancel(ctx context.Context, id int64, reason string) (Booking, error) {
	var payload struct {
		Booking Booking `json:"booking"`
	}
	body := map[string]string{"reason": reason}
	if err := c.do(ctx, http.MethodPost, "/api/admin/bookings/"+strconv.FormatInt(id, 10)+"/cancel", body, &payload); err != nil {
		return Booking{}, err
	}
	return payload.Booking, nil
}

func (c *Client) Reschedule(ctx context.Context, id int64, slot string) (Booking, error) {
	var payload struct {
		Booking Booking `json:"booking"`
	}
	body := map[string]string{"slot": slot}
	if err := c.do(ctx, http.MethodPost, "/api/admin/bookings/"+strconv.FormatInt(id, 10)+"/reschedule", body, &payload); err != nil {
		return Booking{}, err
	}
	return payload.Booking, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, payload)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}

	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("call %s%s: %w", c.BaseURL, path, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		var apiError struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(snippet, &apiError) == nil && apiError.Error != "" {
			return fmt.Errorf("%d: %s", response.StatusCode, apiError.Error)
		}
		return fmt.Errorf("%d: %s", response.StatusCode, strings.TrimSpace(string(snippet)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func urlQuery(value, fallback string) string {
	if value == "" {
		value = fallback
	}
	return strings.ReplaceAll(value, " ", "%20")
}

func orDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

type Account struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	AppURL   string `json:"app_url"`
}

type Block struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Weekdays      []int  `json:"weekdays"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	IntervalWeeks int    `json:"interval_weeks"`
	AnchorDate    string `json:"anchor_date"`
}

type CreateBlockInput struct {
	Title         string `json:"title"`
	Weekdays      []int  `json:"weekdays"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	IntervalWeeks int    `json:"interval_weeks"`
	AnchorDate    string `json:"anchor_date,omitempty"`
}

func (c *Client) Me(ctx context.Context) (Account, error) {
	var account Account
	if err := c.do(ctx, http.MethodGet, "/api/admin/me", nil, &account); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (c *Client) Blocks(ctx context.Context) ([]Block, error) {
	var payload struct {
		Blocks []Block `json:"blocks"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/admin/blocks", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Blocks, nil
}

func (c *Client) CreateBlock(ctx context.Context, input CreateBlockInput) (Block, error) {
	var payload struct {
		Block Block `json:"block"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/admin/blocks", input, &payload); err != nil {
		return Block{}, err
	}
	return payload.Block, nil
}

func (c *Client) DeleteBlock(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodPost, "/api/admin/blocks/"+strconv.FormatInt(id, 10)+"/delete", map[string]string{}, nil)
}
