// Package ics renders a single-event calendar file. It stays dependency-free
// and emits RFC 5545 CRLF lines so Apple Calendar, Google and Outlook all
// accept the download.
package ics

import (
	"strings"
	"time"
)

const maxLineOctets = 75

type Event struct {
	UID            string
	Start          time.Time
	End            time.Time
	Summary        string
	Description    string
	Location       string
	OrganizerName  string
	OrganizerEmail string
	AttendeeName   string
	AttendeeEmail  string
	URL            string
}

func Build(event Event) []byte {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Slot//Scheduling//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + text(event.UID),
		"DTSTAMP:" + stamp(time.Now().UTC()),
		"DTSTART:" + stamp(event.Start),
		"DTEND:" + stamp(event.End),
		"SUMMARY:" + text(event.Summary),
		"STATUS:CONFIRMED",
	}
	if event.Description != "" {
		lines = append(lines, "DESCRIPTION:"+text(event.Description))
	}
	if event.Location != "" {
		lines = append(lines, "LOCATION:"+text(event.Location))
	}
	if event.OrganizerEmail != "" {
		lines = append(lines, "ORGANIZER;CN="+text(event.OrganizerName)+":mailto:"+event.OrganizerEmail)
	}
	if event.AttendeeEmail != "" {
		lines = append(lines, "ATTENDEE;CN="+text(event.AttendeeName)+";RSVP=TRUE:mailto:"+event.AttendeeEmail)
	}
	if event.URL != "" {
		lines = append(lines, "URL:"+event.URL)
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")

	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(fold(line))
		builder.WriteString("\r\n")
	}
	return []byte(builder.String())
}

func stamp(ts time.Time) string {
	return ts.UTC().Format("20060102T150405Z")
}

// text escapes the four characters RFC 5545 reserves inside a TEXT value.
func text(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", `\;`,
		",", `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
	)
	return replacer.Replace(value)
}

// fold keeps every physical line under 75 octets; continuation lines start
// with a single space.
func fold(line string) string {
	if len(line) <= maxLineOctets {
		return line
	}
	var builder strings.Builder
	limit := maxLineOctets
	for len(line) > limit {
		builder.WriteString(line[:limit])
		builder.WriteString("\r\n ")
		line = line[limit:]
		limit = maxLineOctets - 1
	}
	builder.WriteString(line)
	return builder.String()
}
