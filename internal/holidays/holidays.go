// Package holidays holds the national holidays the owner can close. The dates
// are the fixed national ones; movable observances (Carnival, Good Friday,
// Corpus Christi) are deliberately out of scope until someone needs them.
package holidays

import "time"

type Day struct {
	Key   string
	Month time.Month
	Day   int
}

// Brazil is the national calendar, in month order.
var Brazil = []Day{
	{Key: "new_year", Month: time.January, Day: 1},
	{Key: "tiradentes", Month: time.April, Day: 21},
	{Key: "labour", Month: time.May, Day: 1},
	{Key: "independence", Month: time.September, Day: 7},
	{Key: "aparecida", Month: time.October, Day: 12},
	{Key: "all_souls", Month: time.November, Day: 2},
	{Key: "republic", Month: time.November, Day: 15},
	{Key: "christmas", Month: time.December, Day: 25},
}

func List() []Day {
	return Brazil
}

// Exists reports whether the key belongs to the calendar.
func Exists(key string) bool {
	for _, day := range Brazil {
		if day.Key == key {
			return true
		}
	}
	return false
}

// Occurrence is the holiday's date in a given year, at local midnight.
func (d Day) Occurrence(year int, loc *time.Location) time.Time {
	return time.Date(year, d.Month, d.Day, 0, 0, 0, 0, loc)
}

// NextOnOrAfter returns the first occurrence at or after the local date of
// `from`.
func (d Day) NextOnOrAfter(from time.Time, loc *time.Location) time.Time {
	local := from.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	candidate := d.Occurrence(local.Year(), loc)
	if candidate.Before(today) {
		candidate = d.Occurrence(local.Year()+1, loc)
	}
	return candidate
}
