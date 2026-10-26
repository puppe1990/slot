package db

import (
	"fmt"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/store"
)

const (
	demoOwnerEmail    = "demo@example.com"
	demoOwnerName     = "Alex Rivera"
	demoOwnerTimezone = "America/New_York"
)

// RunSeeds populates a believable consultant desk. Safe to run repeatedly:
// every step checks what already exists first.
func RunSeeds(s store.Store) error {
	owner, err := s.FindUserByEmail(demoOwnerEmail)
	if err != nil {
		// Production has no demo user; seeding is a development affordance.
		return nil

	}
	if err := s.UpdateOwnerProfile(owner.ID, demoOwnerName, demoOwnerTimezone); err != nil {
		return fmt.Errorf("seed profile: %w", err)
	}
	if err := s.ReplaceAvailabilityRules(owner.ID, demoWeeklyHours()); err != nil {
		return fmt.Errorf("seed availability: %w", err)
	}
	if err := s.SaveCalendarConnection(models.CalendarConnection{
		UserID:       owner.ID,
		Provider:     models.CalendarProviderFake,
		AccountEmail: owner.Email,
		SyncStatus:   "ACTIVE",
		LastSyncedAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("seed calendar connection: %w", err)
	}

	types, err := s.ListMeetingTypes(owner.ID)
	if err != nil {
		return fmt.Errorf("seed meeting types: %w", err)
	}
	if len(types) == 0 {
		types, err = createDemoMeetingTypes(s, owner.ID)
		if err != nil {
			return err
		}
	}

	bookings, err := s.ListBookings(owner.ID, store.BookingFilter{})
	if err != nil {
		return fmt.Errorf("seed bookings: %w", err)
	}
	if len(bookings) > 0 {
		return nil
	}
	return createDemoBookings(s, types)
}

func demoWeeklyHours() []models.AvailabilityRule {
	rules := make([]models.AvailabilityRule, 0, 5)
	for day := 1; day <= 5; day++ {
		rules = append(rules, models.AvailabilityRule{
			DayOfWeek: day,
			StartTime: "09:00",
			EndTime:   "17:00",
			IsActive:  true,
		})
	}
	return rules
}

func createDemoMeetingTypes(s store.Store, ownerID int64) ([]models.MeetingType, error) {
	specs := []models.MeetingType{
		{
			UserID:              ownerID,
			Slug:                "30min",
			Title:               "30-Minute Strategy Advisory",
			Description:         "Focused tactical review of product architecture, team topology, and engineering roadmaps.",
			DurationMinutes:     30,
			BufferBeforeMinutes: 10,
			BufferAfterMinutes:  10,
			MinNoticeHours:      12,
			BookingHorizonDays:  30,
			LocationType:        models.LocationGoogleMeet,
			IsActive:            true,
		},
		{
			UserID:              ownerID,
			Slug:                "60min",
			Title:               "60-Minute Deep Dive",
			Description:         "A full hour for architecture teardown, hiring calibration, or a go-to-market working session.",
			DurationMinutes:     60,
			BufferBeforeMinutes: 15,
			BufferAfterMinutes:  15,
			MinNoticeHours:      24,
			BookingHorizonDays:  45,
			LocationType:        models.LocationGoogleMeet,
			IsActive:            true,
		},
	}

	out := make([]models.MeetingType, 0, len(specs))
	for _, spec := range specs {
		id, err := s.CreateMeetingType(spec)
		if err != nil {
			return nil, fmt.Errorf("seed meeting type %q: %w", spec.Slug, err)
		}
		created, err := s.GetMeetingType(ownerID, id)
		if err != nil {
			return nil, fmt.Errorf("seed meeting type %q read back: %w", spec.Slug, err)
		}
		out = append(out, created)
	}
	return out, nil
}

func createDemoBookings(s store.Store, types []models.MeetingType) error {
	if len(types) == 0 {
		return nil
	}
	faker := gofakeit.New(20261026)
	now := time.Now().UTC()
	location, err := time.LoadLocation(demoOwnerTimezone)
	if err != nil {
		location = time.UTC
	}

	slots := []struct {
		weekday time.Weekday
		hour    int
		typeIdx int
	}{
		{time.Tuesday, 10, 0},
		{time.Wednesday, 14, 0},
		{time.Thursday, 11, 1},
	}

	for _, slot := range slots {
		meetingType := types[slot.typeIdx%len(types)]
		start := nextWeekday(now, location, slot.weekday, slot.hour)
		email := faker.Email()
		_, holdToken, err := s.CreateHold(store.HoldInput{
			MeetingTypeID: meetingType.ID,
			Start:         start,
			End:           start.Add(time.Duration(meetingType.DurationMinutes) * time.Minute),
			GuestEmail:    email,
			GuestTimezone: "Europe/London",
			ExpiresAt:     now.Add(30 * time.Minute),
			Now:           now,
		})
		if err != nil {
			return fmt.Errorf("seed hold: %w", err)
		}
		if _, err := s.ConfirmHold(holdToken, store.GuestInput{
			Name:     faker.Name(),
			Email:    email,
			Notes:    faker.Sentence(8),
			Timezone: "Europe/London",
		}, guesttoken.Hash(guesttoken.RandomToken()), now); err != nil {
			return fmt.Errorf("seed confirm: %w", err)
		}
	}
	return nil
}

func nextWeekday(from time.Time, location *time.Location, weekday time.Weekday, hour int) time.Time {
	local := from.In(location)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, location)
	for candidate.Weekday() != weekday || candidate.Before(local) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate.UTC()
}
