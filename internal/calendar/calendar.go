// Package calendar isolates the external calendar behind one interface. The
// application always talks to a Provider; tests and local development use
// Fake, and Google is only constructed when OAuth credentials exist.
package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/scheduling"
)

const (
	ProviderFake   = "fake"
	ProviderGoogle = "google"
)

type Event struct {
	ID        string
	Summary   string
	Details   string
	Start     time.Time
	End       time.Time
	Attendees []string
	Location  string
}

type Provider interface {
	Provider() string
	ListBusyIntervals(ctx context.Context, from, to time.Time) ([]scheduling.Interval, error)
	CreateEvent(ctx context.Context, event Event) (Event, error)
	UpdateEvent(ctx context.Context, id string, start, end time.Time) error
	DeleteEvent(ctx context.Context, id string) error
}

type Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	CalendarID         string
	// GoogleServiceAccount is the credential JSON of a service account with
	// access to CalendarID. It replaces the OAuth flow entirely.
	GoogleServiceAccount string
	// TokenSource is supplied by the OAuth connection. Without it the app
	// stays on the fake, so a half-configured deployment never 500s a booking.
	TokenSource func(ctx context.Context) (string, error)
}

// New picks the concrete adapter. Unset Google credentials are the normal
// local case: the deterministic fake keeps slot math and tests working.
func New(cfg Config) (Provider, error) {
	// A service account needs no OAuth consent and no refresh token, so it wins
	// when both are configured.
	if strings.TrimSpace(cfg.GoogleServiceAccount) != "" {
		token, err := ServiceAccountToken(cfg.GoogleServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("google service account: %w", err)
		}
		provider, err := NewGoogle(cfg.CalendarID, nil)
		if err != nil {
			return nil, fmt.Errorf("google calendar: %w", err)
		}
		provider.AccessToken = token
		// A service account on a personal calendar cannot add attendees.
		provider.InviteAttendees = false
		return provider, nil
	}
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" || cfg.TokenSource == nil {
		return NewFake(), nil
	}
	provider, err := NewGoogle(cfg.CalendarID, nil)
	if err != nil {
		return nil, fmt.Errorf("google calendar: %w", err)
	}
	provider.AccessToken = cfg.TokenSource
	return provider, nil
}
