package holidays

import (
	"testing"
	"time"
)

func TestListIsCompleteAndOrdered(t *testing.T) {
	list := List()
	if len(list) != 8 {
		t.Fatalf("calendar has %d holidays, want 8", len(list))
	}
	for i := 1; i < len(list); i++ {
		previous := list[i-1]
		current := list[i]
		if current.Month < previous.Month || (current.Month == previous.Month && current.Day <= previous.Day) {
			t.Fatalf("%s is out of order after %s", current.Key, previous.Key)
		}
	}
	for _, day := range list {
		if !Exists(day.Key) {
			t.Errorf("%s is missing from Exists", day.Key)
		}
	}
	if Exists("carnival") {
		t.Error("carnival is not part of the fixed calendar")
	}
}

func TestNextOnOrAfter(t *testing.T) {
	loc := time.UTC
	goodFridayEve := time.Date(2026, 10, 27, 15, 0, 0, 0, loc)

	allSouls := Brazil[5]
	if got := allSouls.NextOnOrAfter(goodFridayEve, loc); got.Format("2006-01-02") != "2026-11-02" {
		t.Fatalf("all souls = %s, want 2026-11-02", got)
	}

	newYear := Brazil[0]
	if got := newYear.NextOnOrAfter(goodFridayEve, loc); got.Format("2006-01-02") != "2027-01-01" {
		t.Fatalf("new year = %s, want 2027-01-01", got)
	}

	// On the day itself the holiday still counts as "next".
	onTheDay := time.Date(2026, 12, 25, 9, 0, 0, 0, loc)
	if got := Brazil[7].NextOnOrAfter(onTheDay, loc); got.Format("2006-01-02") != "2026-12-25" {
		t.Fatalf("christmas on the day = %s", got)
	}
}

func TestOccurrenceUsesOwnerLocation(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Skip("no tzdata")
	}
	date := Brazil[5].Occurrence(2026, loc)
	if date.Hour() != 0 || date.Location() != loc {
		t.Fatalf("occurrence = %s in %s", date, date.Location())
	}
}
