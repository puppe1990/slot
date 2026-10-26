package calendar_test

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/calendar"
	"github.com/puppe1990/slot/internal/scheduling"
)

func at(clock string) time.Time {
	ts, err := time.Parse(time.RFC3339, clock)
	if err != nil {
		panic(err)
	}
	return ts
}

func TestFakeProviderLifecycle(t *testing.T) {
	ctx := context.Background()
	faker := gofakeit.New(23)
	provider := calendar.NewFake()

	event, err := provider.CreateEvent(ctx, calendar.Event{
		Summary:   faker.Sentence(4),
		Start:     at("2026-10-27T14:00:00Z"),
		End:       at("2026-10-27T14:30:00Z"),
		Attendees: []string{faker.Email()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == "" {
		t.Fatal("CreateEvent returned an empty id")
	}

	busy, err := provider.ListBusyIntervals(ctx, at("2026-10-27T00:00:00Z"), at("2026-10-28T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 || !busy[0].Start.Equal(at("2026-10-27T14:00:00Z")) {
		t.Fatalf("busy = %+v, want the created event", busy)
	}

	if err := provider.UpdateEvent(ctx, event.ID, at("2026-10-27T15:00:00Z"), at("2026-10-27T15:30:00Z")); err != nil {
		t.Fatal(err)
	}
	busy, err = provider.ListBusyIntervals(ctx, at("2026-10-27T00:00:00Z"), at("2026-10-28T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 || !busy[0].Start.Equal(at("2026-10-27T15:00:00Z")) {
		t.Fatalf("busy after update = %+v", busy)
	}

	if err := provider.DeleteEvent(ctx, event.ID); err != nil {
		t.Fatal(err)
	}
	busy, err = provider.ListBusyIntervals(ctx, at("2026-10-27T00:00:00Z"), at("2026-10-28T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 0 {
		t.Fatalf("busy after delete = %+v, want none", busy)
	}
}

func TestFakeProviderFiltersBusyByRange(t *testing.T) {
	ctx := context.Background()
	provider := calendar.NewFake()
	provider.SeedBusy([]scheduling.Interval{
		{Start: at("2026-10-27T09:00:00Z"), End: at("2026-10-27T10:00:00Z")},
		{Start: at("2026-10-29T09:00:00Z"), End: at("2026-10-29T10:00:00Z")},
	})

	busy, err := provider.ListBusyIntervals(ctx, at("2026-10-27T00:00:00Z"), at("2026-10-28T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 {
		t.Fatalf("busy = %+v, want only the interval inside the range", busy)
	}
}

func TestFakeProviderRejectsUnknownEvent(t *testing.T) {
	ctx := context.Background()
	provider := calendar.NewFake()
	if err := provider.UpdateEvent(ctx, "missing", time.Now(), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("UpdateEvent accepted an unknown id")
	}
	if err := provider.DeleteEvent(ctx, "missing"); err == nil {
		t.Fatal("DeleteEvent accepted an unknown id")
	}
}

func TestNewProviderWithoutCredentialsIsFake(t *testing.T) {
	provider, err := calendar.New(calendar.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Provider() != calendar.ProviderFake {
		t.Fatalf("provider = %q, want fake", provider.Provider())
	}
}

func TestNewProviderWithCredentialsIsGoogle(t *testing.T) {
	provider, err := calendar.New(calendar.Config{
		GoogleClientID:     "client-id",
		GoogleClientSecret: "secret",
		TokenSource:        func(context.Context) (string, error) { return "token", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Provider() != calendar.ProviderGoogle {
		t.Fatalf("provider = %q, want google", provider.Provider())
	}
}
