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
	case "whoami":
		return runWhoami(args[1:], out)
	case "blocks":
		return runBlocks(args[1:], out)
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
  slot whoami [--json]
  slot blocks [--json]
  slot blocks add --title "..." --days 1,2,3,4,5 --start 07:00 --end 11:00
                 [--every 2] [--anchor 2026-11-02]
  slot blocks rm <id>
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

func runWhoami(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("whoami", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	account, err := newClient(*url, *token).Me(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, account)
	}
	_, _ = fmt.Fprintf(out, "%s <%s>\n%s\n%s\n", account.Name, account.Email, account.Timezone, account.AppURL)
	return nil
}

func runBlocks(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "add" {
		return runAddBlock(args[1:], out)
	}
	if len(args) > 0 && (args[0] == "rm" || args[0] == "remove") {
		return runRemoveBlock(args[1:], out)
	}

	fs := flag.NewFlagSet("blocks", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	blocks, err := newClient(*url, *token).Blocks(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, blocks)
	}
	if len(blocks) == 0 {
		_, _ = fmt.Fprintln(out, "no recurring blocks")
		return nil
	}
	for _, block := range blocks {
		_, _ = fmt.Fprintf(out, "%-4d %-24s %-10s %s–%s  every %d week(s), week of %s\n",
			block.ID, block.Title, weekdayNames(block.Weekdays), block.StartTime, block.EndTime,
			block.IntervalWeeks, block.AnchorDate)
	}
	return nil
}

func runAddBlock(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("blocks add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	title := fs.String("title", "", "label")
	days := fs.String("days", "1,2,3,4,5", "weekday numbers, 0 = Sunday")
	start := fs.String("start", "07:00", "block start")
	end := fs.String("end", "11:00", "block end")
	every := fs.Int("every", 2, "weeks between repeats")
	anchor := fs.String("anchor", "", "first week starts on (default today)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	weekdays, err := parseDays(*days)
	if err != nil {
		return err
	}
	block, err := newClient(*url, *token).CreateBlock(context.Background(), slotcli.CreateBlockInput{
		Title:         *title,
		Weekdays:      weekdays,
		StartTime:     *start,
		EndTime:       *end,
		IntervalWeeks: *every,
		AnchorDate:    *anchor,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, block)
	}
	_, _ = fmt.Fprintf(out, "block #%d %s %s–%s every %d week(s), week of %s\n",
		block.ID, weekdayNames(block.Weekdays), block.StartTime, block.EndTime,
		block.IntervalWeeks, block.AnchorDate)
	return nil
}

func runRemoveBlock(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("blocks rm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	url, token, asJSON := addClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := parseID(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := newClient(*url, *token).DeleteBlock(context.Background(), id); err != nil {
		return err
	}
	if *asJSON {
		return printJSON(out, map[string]int64{"deleted": id})
	}
	_, _ = fmt.Fprintf(out, "removed block #%d\n", id)
	return nil
}

func parseDays(raw string) ([]int, error) {
	days := []int{}
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 0 || value > 6 {
			return nil, fmt.Errorf("weekday %q must be 0..6", part)
		}
		days = append(days, value)
	}
	return days, nil
}

func weekdayNames(days []int) string {
	names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	out := make([]string, 0, len(days))
	for _, day := range days {
		if day >= 0 && day < len(names) {
			out = append(out, names[day])
		}
	}
	return strings.Join(out, ",")
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
