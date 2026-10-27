// Package call issues the video room for a booking. The app only needs a URL,
// so the interface stays tiny and a new vendor is an adapter plus one env var.
package call

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	ProviderNone  = "none"
	ProviderDaily = "daily"
	ProviderJitsi = "jitsi"
)

type Room struct {
	URL      string
	Provider string
}

type Input struct {
	BookingID int64
	Title     string
	Start     time.Time
	End       time.Time
}

type Provider interface {
	Provider() string
	CreateRoom(ctx context.Context, input Input) (Room, error)
	// EndRoom is best effort: a vendor that has no room lifecycle (Jitsi)
	// simply returns nil.
	EndRoom(ctx context.Context, room Room) error
}

type Config struct {
	Provider     string
	DailyAPIKey  string
	DailyBaseURL string
	JitsiBaseURL string
}

// New picks the adapter. An unknown or empty provider is the no-op, so a
// deployment without credentials keeps working with just the modality label.
func New(cfg Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ProviderDaily:
		if strings.TrimSpace(cfg.DailyAPIKey) == "" {
			return nil, fmt.Errorf("CALL_PROVIDER=daily requires DAILY_API_KEY")
		}
		return NewDaily(cfg.DailyAPIKey, cfg.DailyBaseURL)
	case ProviderJitsi:
		return NewJitsi(cfg.JitsiBaseURL)
	default:
		return None{}, nil
	}
}

// None is the deployment without video credentials.
type None struct{}

func (None) Provider() string { return ProviderNone }

func (None) CreateRoom(context.Context, Input) (Room, error) {
	return Room{}, nil
}

func (None) EndRoom(context.Context, Room) error { return nil }
