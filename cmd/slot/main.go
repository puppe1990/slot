// Command slot is the terminal client for the Slot scheduling app. It talks to
// the admin API with a Bearer token, so it works the same locally and against
// production.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/puppe1990/slot/internal/slotcli"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "slot:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(out)
		return nil
	}

	switch args[0] {
	case "bookings":
		return runBookings(args[1:], out)
	case "formats":
		return runFormats(args[1:], out)
	case "slots":
		return runSlots(args[1:], out)
	case "book":
		return runBook(args[1:], out)
	case "cancel":
		return runCancel(args[1:], out)
	case "reschedule":
		return runReschedule(args[1:], out)
	default:
		usage(out)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(out io.Writer) {
	_, _ = fmt.Fprint(out, `slot — Slot scheduling from the terminal

Usage:
  slot bookings [--status upcoming|past|cancelled|all] [--days 90] [--json]
  slot formats [--json]
  slot slots --format <slug> [--days 7] [--tz America/Sao_Paulo] [--json]
  slot book --format <slug> --slot <RFC3339> --name <name> --email <email>
            [--whatsapp +5511...] [--notes "..."] [--tz ...]
  slot cancel <id> [--reason "..."] [--json]
  slot reschedule <id> --slot <RFC3339> [--json]

Environment:
  SLOT_URL    base URL (default http://localhost:8080)
  SLOT_TOKEN  ADMIN_TOKEN of the app (required)

Every command also accepts --url and --token.
`)
}

func newClient(url, token string) *slotcli.Client {
	if url == "" {
		url = envOr("SLOT_URL", "http://localhost:8080")
	}
	if token == "" {
		token = os.Getenv("SLOT_TOKEN")
	}
	return slotcli.New(url, token)
}

func addClientFlags(fs *flag.FlagSet) (*string, *string, *bool) {
	url := fs.String("url", "", "base URL (default $SLOT_URL)")
	token := fs.String("token", "", "admin token (default $SLOT_TOKEN)")
	asJSON := fs.Bool("json", false, "print raw JSON")
	return url, token, asJSON
}

func runBookings(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("bookings", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	status := fs.String("status", "upcoming", "upcoming, past, cancelled or all")
	days := fs.Int("days", 90, "window in days")
	if err := fs.Parse(args); err != nil {
		return err
	}

	bookings, err := newClient(*url, *token).Bookings(context.Background(), *status, *days)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, bookings)
	}
	if len(bookings) == 0 {
		_, _ = fmt.Fprintln(out, "no bookings")
		return nil
	}
	_, _ = fmt.Fprintf(out, "%-4s %-22s %-12s %-22s %s\n", "ID", "WHEN", "STATUS", "GUEST", "FORMAT")
	for _, booking := range bookings {
		guest := booking.GuestName
		if guest == "" {
			guest = booking.GuestEmail
		}
		_, _ = fmt.Fprintf(out, "%-4d %-22s %-12s %-22s %s\n",
			booking.ID, booking.When(), booking.Status, guest, booking.Title)
	}
	return nil
}

func runFormats(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("formats", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	formats, err := newClient(*url, *token).Formats(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, formats)
	}
	for _, format := range formats {
		state := "paused"
		if format.IsActive {
			state = "live"
		}
		_, _ = fmt.Fprintf(out, "%-10s %-32s %3dmin  %-6s %s\n",
			format.Slug, format.Title, format.DurationMinutes, state, format.PublicURL)
	}
	return nil
}

func runSlots(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("slots", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	format := fs.String("format", "", "meeting type slug (required)")
	days := fs.Int("days", 7, "how many days ahead")
	timezone := fs.String("tz", "", "guest timezone for the labels")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format == "" {
		return fmt.Errorf("--format is required")
	}

	slots, err := newClient(*url, *token).Slots(context.Background(), *format, *timezone, *days)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, slots)
	}
	for _, slot := range slots {
		_, _ = fmt.Fprintf(out, "%s  %-22s host %s\n", slot.StartUTC, slot.GuestRange, slot.HostRange)
	}
	return nil
}

func runBook(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("book", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	format := fs.String("format", "", "meeting type slug (required)")
	slot := fs.String("slot", "", "start in RFC3339 UTC (required)")
	name := fs.String("name", "", "guest name (required)")
	email := fs.String("email", "", "guest email (required)")
	whatsapp := fs.String("whatsapp", "", "guest WhatsApp")
	notes := fs.String("notes", "", "guest notes")
	timezone := fs.String("tz", "", "guest timezone")
	if err := fs.Parse(args); err != nil {
		return err
	}

	created, err := newClient(*url, *token).CreateBooking(context.Background(), slotcli.CreateBookingInput{
		Format: *format, Slot: *slot, Name: *name, Email: *email,
		WhatsApp: *whatsapp, Notes: *notes, Timezone: *timezone,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, created)
	}
	_, _ = fmt.Fprintf(out, "booked #%d %s\n%s\nmanage: %s\njoin:   %s\n",
		created.Booking.ID, created.Booking.When(), created.Booking.Title,
		created.ManageURL, created.Booking.JoinURL)
	return nil
}

func runCancel(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	reason := fs.String("reason", "", "cancellation reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := parseID(fs.Arg(0))
	if err != nil {
		return err
	}

	booking, err := newClient(*url, *token).Cancel(context.Background(), id, *reason)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, booking)
	}
	_, _ = fmt.Fprintf(out, "cancelled #%d (%s)\n", booking.ID, booking.Status)
	return nil
}

func runReschedule(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("reschedule", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	slot := fs.String("slot", "", "new start in RFC3339 UTC (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := parseID(fs.Arg(0))
	if err != nil {
		return err
	}
	if *slot == "" {
		return fmt.Errorf("--slot is required")
	}

	booking, err := newClient(*url, *token).Reschedule(context.Background(), id, *slot)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, booking)
	}
	_, _ = fmt.Fprintf(out, "moved #%d to %s (%s)\n", booking.ID, booking.When(), booking.Status)
	return nil
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("booking id %q is not positive", raw)
	}
	return id, nil
}

func printJSON(out io.Writer, payload any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
