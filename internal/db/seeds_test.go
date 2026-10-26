package db_test

import (
	"testing"

	"github.com/puppe1990/slot/internal/db"
	"github.com/puppe1990/slot/internal/store"
)

func newSeedStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRunSeedsCreatesDemoDesk(t *testing.T) {
	s := newSeedStore(t)
	ownerID, err := s.CreateUser("demo@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.RunSeeds(s); err != nil {
		t.Fatal(err)
	}

	owner, err := s.GetUser(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name == "" || owner.Timezone == "" {
		t.Fatalf("owner profile not seeded: %+v", owner)
	}

	types, err := s.ListMeetingTypes(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 {
		t.Fatalf("meeting types = %d, want 2", len(types))
	}

	rules, err := s.ListAvailabilityRules(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 5 {
		t.Fatalf("availability rules = %d, want 5 weekdays", len(rules))
	}

	bookings, err := s.ListBookings(ownerID, store.BookingFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bookings) == 0 {
		t.Fatal("demo bookings were not created")
	}
}

func TestRunSeedsIsIdempotent(t *testing.T) {
	s := newSeedStore(t)
	ownerID, err := s.CreateUser("demo@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.RunSeeds(s); err != nil {
		t.Fatal(err)
	}
	first, err := s.ListBookings(ownerID, store.BookingFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RunSeeds(s); err != nil {
		t.Fatal(err)
	}
	second, err := s.ListBookings(ownerID, store.BookingFilter{})
	if err != nil {
		t.Fatal(err)
	}
	types, err := s.ListMeetingTypes(ownerID)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != len(second) {
		t.Fatalf("second RunSeeds duplicated bookings: %d -> %d", len(first), len(second))
	}
	if len(types) != 2 {
		t.Fatalf("second RunSeeds duplicated formats: %d", len(types))
	}
}

func TestRunSeedsWithoutDemoUserIsNoop(t *testing.T) {
	s := newSeedStore(t)
	if err := db.RunSeeds(s); err != nil {
		t.Fatalf("RunSeeds on an empty database: %v", err)
	}
}
