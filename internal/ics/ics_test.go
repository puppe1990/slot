package ics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/ics"
)

func TestBuildContainsCalendarEnvelope(t *testing.T) {
	faker := gofakeit.New(5)
	document := string(ics.Build(ics.Event{
		UID:            "booking-42@slot.local",
		Start:          time.Date(2026, 10, 27, 14, 0, 0, 0, time.UTC),
		End:            time.Date(2026, 10, 27, 14, 30, 0, 0, time.UTC),
		Summary:        faker.Sentence(4),
		Description:    "Agenda, context and notes.",
		Location:       "Google Meet",
		OrganizerName:  "Alex Rivera",
		OrganizerEmail: "owner@example.com",
		AttendeeName:   "Sarah Jenkins",
		AttendeeEmail:  "sarah@example.com",
	}))

	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"DTSTART:20261027T140000Z",
		"DTEND:20261027T143000Z",
		"UID:booking-42@slot.local",
		"ORGANIZER;CN=Alex Rivera:mailto:owner@example.com",
		"ATTENDEE;CN=Sarah Jenkins;RSVP=TRUE:mailto:sarah@example.com",
		"END:VEVENT",
		"END:VCALENDAR",
	} {
		if !strings.Contains(document, want) {
			t.Fatalf("document missing %q:\n%s", want, document)
		}
	}
}

func TestBuildEscapesTextAndUsesCRLF(t *testing.T) {
	document := string(ics.Build(ics.Event{
		UID:         "booking-1@slot.local",
		Start:       time.Date(2026, 10, 27, 14, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 10, 27, 14, 30, 0, 0, time.UTC),
		Summary:     "Advisory, strategy; review",
		Description: "Line one\nLine two",
	}))

	if !strings.Contains(document, `SUMMARY:Advisory\, strategy\; review`) {
		t.Fatalf("summary was not escaped:\n%s", document)
	}
	if !strings.Contains(document, `DESCRIPTION:Line one\nLine two`) {
		t.Fatalf("description newline was not escaped:\n%s", document)
	}
	if !strings.Contains(document, "\r\n") {
		t.Fatal("document does not use CRLF line endings")
	}
	if strings.Contains(document, "\n\n") && !strings.Contains(document, "\r\n\r\n") {
		t.Fatal("document mixes LF and CRLF")
	}
}

func TestBuildFoldsLongLines(t *testing.T) {
	long := strings.Repeat("architecture ", 12)
	document := string(ics.Build(ics.Event{
		UID:         "booking-2@slot.local",
		Start:       time.Date(2026, 10, 27, 14, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 10, 27, 14, 30, 0, 0, time.UTC),
		Summary:     "Advisory",
		Description: long,
	}))

	for _, line := range strings.Split(document, "\r\n") {
		if len(line) > 75 && !strings.HasPrefix(line, " ") {
			t.Fatalf("line longer than 75 octets without folding: %q", line)
		}
	}
}
