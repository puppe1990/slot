package calendar

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/puppe1990/slot/internal/scheduling"
)

var errEventNotFound = errors.New("calendar event not found")

// Fake is the deterministic in-memory calendar. It is safe for concurrent
// handlers and never talks to the network.
type Fake struct {
	mu     sync.Mutex
	events map[string]Event
	order  []string
	nextID int
}

func NewFake() *Fake {
	return &Fake{events: make(map[string]Event)}
}

func (f *Fake) Provider() string { return ProviderFake }

// SeedBusy pins blocks that came from outside the app (commute, focus time) so
// the public page can show a realistic "prep buffer booked" state.
func (f *Fake) SeedBusy(intervals []scheduling.Interval) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, interval := range intervals {
		f.nextID++
		id := fmt.Sprintf("busy-%d", f.nextID)
		f.events[id] = Event{
			ID:      id,
			Summary: "Busy",
			Start:   interval.Start.UTC(),
			End:     interval.End.UTC(),
		}
		f.order = append(f.order, id)
	}
}

func (f *Fake) ListBusyIntervals(_ context.Context, from, to time.Time) ([]scheduling.Interval, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	window := scheduling.Interval{Start: from, End: to}
	var busy []scheduling.Interval
	for _, id := range f.order {
		event := f.events[id]
		interval := scheduling.Interval{Start: event.Start, End: event.End}
		if interval.Overlaps(window) || interval.Start.Equal(from) || interval.End.Equal(to) {
			busy = append(busy, interval)
		}
	}
	return busy, nil
}

func (f *Fake) CreateEvent(_ context.Context, event Event) (Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	event.ID = fmt.Sprintf("evt-%d", f.nextID)
	event.Start = event.Start.UTC()
	event.End = event.End.UTC()
	f.events[event.ID] = event
	f.order = append(f.order, event.ID)
	return event, nil
}

func (f *Fake) UpdateEvent(_ context.Context, id string, start, end time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	event, ok := f.events[id]
	if !ok {
		return fmt.Errorf("%w: %s", errEventNotFound, id)
	}
	event.Start = start.UTC()
	event.End = end.UTC()
	f.events[id] = event
	return nil
}

func (f *Fake) DeleteEvent(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.events[id]; !ok {
		return fmt.Errorf("%w: %s", errEventNotFound, id)
	}
	delete(f.events, id)
	for i, existing := range f.order {
		if existing == id {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	return nil
}
