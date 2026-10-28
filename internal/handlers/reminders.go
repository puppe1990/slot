package handlers

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
)

// ReminderService sends the "your session is coming up" email. It runs in the
// server process (one binary on this deploy) and is idempotent: the booking is
// stamped before the send, so a restart or a second tick never duplicates it.
type ReminderService struct {
	base
}

func NewReminderService(deps Deps) *ReminderService {
	return &ReminderService{base: newBase(deps)}
}

// RunOnce delivers every reminder that is due and returns how many went out.
func (h *ReminderService) RunOnce(ctx context.Context) (int, error) {
	now := h.now()
	due, err := h.deps.Store.ListBookingsNeedingReminder(now, now.Add(reminderLead()))
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, booking := range due {
		claimed, err := h.deps.Store.ClaimReminder(booking.ID, now)
		if err != nil {
			return sent, err
		}
		if !claimed {
			continue
		}
		if strings.TrimSpace(booking.GuestEmail) == "" {
			continue
		}

		message := h.reminderMessage(booking)
		if err := h.deps.Notifier.Send(ctx, message); err != nil {
			// Clear the stamp so the next tick retries instead of losing it.
			log.Printf("slot: reminder for booking %d: %v", booking.ID, err)
			_ = h.deps.Store.ReleaseReminder(booking.ID)
			continue
		}
		sent++
	}
	return sent, nil
}

// Start ticks until ctx is cancelled. The first pass runs right away so a
// restart does not push due reminders to the next interval.
func (h *ReminderService) Start(ctx context.Context) {
	ticker := time.NewTicker(reminderInterval())
	defer ticker.Stop()

	if count, err := h.RunOnce(ctx); err != nil {
		log.Printf("slot: reminder pass: %v", err)
	} else if count > 0 {
		log.Printf("slot: %d reminder(s) sent", count)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if count, err := h.RunOnce(ctx); err != nil {
				log.Printf("slot: reminder pass: %v", err)
			} else if count > 0 {
				log.Printf("slot: %d reminder(s) sent", count)
			}
		}
	}
}

func (h *ReminderService) reminderMessage(booking models.Booking) notify.Message {
	catalog := h.deps.Catalog
	if localized, ok := h.deps.Catalogs[booking.GuestLocale]; ok && localized != nil {
		catalog = localized
	}
	l := localeText{tag: catalog.Locale(), catalog: catalog}
	manageURL := strings.TrimRight(h.deps.Site.AppURL, "/") + "/manage/" + h.deps.Signer.Sign(booking.ID)
	return h.bookingEmailMessage(l, booking, bookingEmail{
		Kind:      "booking.reminder",
		To:        booking.GuestEmail,
		Subject:   l.t("email.reminder.subject", booking.MeetingTypeTitle),
		Heading:   l.t("email.heading.reminder"),
		Tone:      "pine",
		Intro:     l.t("email.reminder.intro", booking.MeetingTypeTitle, booking.OwnerName),
		Location:  mustLocation(normalizeTimezone(booking.GuestTimezone, booking.OwnerTimezone)),
		CTAURL:    joinURL(manageURL, booking.MeetingURL),
		CTALabel:  l.t("call.join"),
		ManageURL: manageURL,
	})
}

// RemindersEnabled is off only when REMINDERS is explicitly false-ish.
func RemindersEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REMINDERS"))) {
	case "off", "0", "false", "no":
		return false
	default:
		return true
	}
}

// reminderLead is how far ahead the reminder goes out.
func reminderLead() time.Duration {
	hours := 24
	if raw := strings.TrimSpace(os.Getenv("REMINDER_LEAD_HOURS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 168 {
			hours = parsed
		}
	}
	return time.Duration(hours) * time.Hour
}

// reminderInterval keeps the tick cheap; the lead window is what controls when
// a reminder actually goes out.
func reminderInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("REMINDER_INTERVAL_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 1 && seconds <= 3600 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 5 * time.Minute
}
