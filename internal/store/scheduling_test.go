package store

import (
	"errors"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
)

var fixedNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func at(clock string) time.Time {
	ts, err := time.Parse(time.RFC3339, clock)
	if err != nil {
		panic(err)
	}
	return ts
}

func seedOwner(t *testing.T, s *SQLiteStore) int64 {
	t.Helper()
	id, err := s.CreateUser("owner@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateOwnerProfile(id, "Alex Rivera", "America/New_York"); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedMeetingType(t *testing.T, s *SQLiteStore, userID int64, duration int) models.MeetingType {
	t.Helper()
	return seedMeetingTypeWithBuffers(t, s, userID, duration, 10, 10)
}

func seedMeetingTypeWithBuffers(t *testing.T, s *SQLiteStore, userID int64, duration, before, after int) models.MeetingType {
	t.Helper()
	meetingType := models.MeetingType{
		UserID:              userID,
		Slug:                "30min",
		Title:               "30-Minute Strategy Advisory",
		Description:         "Focused tactical review.",
		DurationMinutes:     duration,
		BufferBeforeMinutes: before,
		BufferAfterMinutes:  after,
		MinNoticeHours:      24,
		BookingHorizonDays:  30,
		LocationType:        models.LocationGoogleMeet,
		IsActive:            true,
	}
	id, err := s.CreateMeetingType(meetingType)
	if err != nil {
		t.Fatal(err)
	}
	meetingType.ID = id
	return meetingType
}

func TestStoreMeetingTypeCRUD(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	faker := gofakeit.New(11)

	created := seedMeetingType(t, s, ownerID, 30)
	if created.ID == 0 {
		t.Fatal("CreateMeetingType returned id 0")
	}

	bySlug, err := s.GetMeetingTypeBySlug("30min")
	if err != nil {
		t.Fatal(err)
	}
	if bySlug.ID != created.ID || bySlug.Title != created.Title {
		t.Fatalf("GetMeetingTypeBySlug = %+v, want id %d title %q", bySlug, created.ID, created.Title)
	}

	bySlug.Title = faker.Sentence(4)
	bySlug.DurationMinutes = 60
	if err := s.UpdateMeetingType(bySlug); err != nil {
		t.Fatal(err)
	}
	reread, err := s.GetMeetingType(ownerID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Title != bySlug.Title || reread.DurationMinutes != 60 {
		t.Fatalf("after update = %+v", reread)
	}

	if err := s.SetMeetingTypeActive(ownerID, created.ID, false); err != nil {
		t.Fatal(err)
	}
	reread, err = s.GetMeetingType(ownerID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.IsActive {
		t.Fatal("SetMeetingTypeActive(false) left the type active")
	}

	listed, err := s.ListMeetingTypes(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListMeetingTypes = %d rows, want 1", len(listed))
	}
}

func TestStoreSlugIsGloballyUnique(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	seedMeetingType(t, s, ownerID, 30)

	second := models.MeetingType{
		UserID:             ownerID,
		Slug:               "30min",
		Title:              "Duplicate",
		DurationMinutes:    30,
		BookingHorizonDays: 30,
		LocationType:       models.LocationPhone,
	}
	if _, err := s.CreateMeetingType(second); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug err = %v, want ErrSlugTaken", err)
	}
}

func TestStoreAvailabilityRulesReplace(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)

	rules := []models.AvailabilityRule{
		{DayOfWeek: 1, StartTime: "09:00", EndTime: "12:30"},
		{DayOfWeek: 1, StartTime: "13:30", EndTime: "17:00"},
		{DayOfWeek: 3, StartTime: "10:00", EndTime: "15:00"},
	}
	if err := s.ReplaceAvailabilityRules(ownerID, rules); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListAvailabilityRules(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ListAvailabilityRules = %d, want 3", len(got))
	}

	if err := s.ReplaceAvailabilityRules(ownerID, rules[:1]); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListAvailabilityRules(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EndTime != "12:30" {
		t.Fatalf("after replace = %+v, want a single 09:00-12:30 rule", got)
	}
}

func TestStoreDateOverrides(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)

	id, err := s.UpsertDateOverride(ownerID, models.DateOverride{Date: "2026-10-30", IsUnavailable: true})
	if err != nil {
		t.Fatal(err)
	}
	sameID, err := s.UpsertDateOverride(ownerID, models.DateOverride{Date: "2026-10-30", CustomStartTime: "10:00", CustomEndTime: "12:00"})
	if err != nil {
		t.Fatal(err)
	}
	if sameID != id {
		t.Fatalf("UpsertDateOverride changed the id: %d -> %d", id, sameID)
	}

	list, err := s.ListDateOverrides(ownerID, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].IsUnavailable || list[0].CustomStartTime != "10:00" {
		t.Fatalf("ListDateOverrides = %+v", list)
	}

	if err := s.DeleteDateOverride(ownerID, id); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListDateOverrides(ownerID, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("after delete = %+v, want none", list)
	}
}

func TestStoreRejectsOverlappingHold(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingTypeWithBuffers(t, s, ownerID, 30, 0, 0)
	faker := gofakeit.New(3)

	hold := HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    faker.Email(),
		GuestTimezone: "Europe/London",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	}
	first, _, err := s.CreateHold(hold)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != models.BookingStatusHold {
		t.Fatalf("hold status = %q, want HOLD", first.Status)
	}

	overlapping := hold
	overlapping.Start = at("2026-10-26T13:15:00Z")
	overlapping.End = at("2026-10-26T13:45:00Z")
	if _, _, err := s.CreateHold(overlapping); !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("overlapping hold err = %v, want ErrSlotUnavailable", err)
	}

	adjacent := hold
	adjacent.Start = at("2026-10-26T13:30:00Z")
	adjacent.End = at("2026-10-26T14:00:00Z")
	if _, _, err := s.CreateHold(adjacent); err != nil {
		t.Fatalf("adjacent hold: %v", err)
	}
}

func TestStoreExpiredHoldDoesNotBlock(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	stale := HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    "stale@example.com",
		GuestTimezone: "UTC",
		ExpiresAt:     fixedNow.Add(-time.Minute),
		Now:           fixedNow.Add(-2 * time.Minute),
	}
	if _, _, err := s.CreateHold(stale); err != nil {
		t.Fatal(err)
	}

	fresh := stale
	fresh.ExpiresAt = fixedNow.Add(10 * time.Minute)
	fresh.Now = fixedNow
	if _, _, err := s.CreateHold(fresh); err != nil {
		t.Fatalf("expired hold blocked a new reservation: %v", err)
	}
}

func TestStoreConfirmHoldIssuesBooking(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)
	faker := gofakeit.New(19)

	hold, holdToken, err := s.CreateHold(HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    faker.Email(),
		GuestTimezone: "Europe/London",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	booking, err := s.ConfirmHold(
		holdToken,
		GuestInput{Name: faker.Name(), Email: faker.Email(), Notes: faker.Sentence(6), Timezone: "Europe/London"},
		"guest-token-hash",
		fixedNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	if booking.ID != hold.ID {
		t.Fatalf("confirmed booking id = %d, want the hold row %d", booking.ID, hold.ID)
	}
	if booking.Status != models.BookingStatusConfirmed {
		t.Fatalf("status = %q, want CONFIRMED", booking.Status)
	}
	if booking.GuestName == "" {
		t.Fatal("guest name was not persisted")
	}

	if _, err := s.ConfirmHold(holdToken, GuestInput{Name: "Again"}, "other-hash", fixedNow); !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("second confirm err = %v, want ErrHoldExpired", err)
	}

	found, err := s.GetBookingByGuestTokenHash("guest-token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != booking.ID {
		t.Fatalf("GetBookingByGuestTokenHash = %d, want %d", found.ID, booking.ID)
	}
}

func TestStoreConfirmRejectsExpiredHold(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	_, holdToken, err := s.CreateHold(HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    "late@example.com",
		GuestTimezone: "UTC",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.ConfirmHold(holdToken, GuestInput{Name: "Late"}, "guest-hash", fixedNow.Add(11*time.Minute))
	if !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("expired confirm err = %v, want ErrHoldExpired", err)
	}
}

func TestStoreActiveIntervalsIncludeHoldsAndBookings(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	_, holdToken, err := s.CreateHold(HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    "guest@example.com",
		GuestTimezone: "UTC",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmHold(holdToken, GuestInput{Name: "Guest"}, "guest-hash", fixedNow); err != nil {
		t.Fatal(err)
	}

	intervals, err := s.ListActiveIntervals(ownerID, fixedNow, fixedNow.AddDate(0, 0, 7), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(intervals) != 1 {
		t.Fatalf("intervals = %d, want 1", len(intervals))
	}
	if !intervals[0].Start.Equal(at("2026-10-26T12:50:00Z")) {
		t.Fatalf("interval start = %s, want the 10 minute buffer applied", intervals[0].Start)
	}
	if !intervals[0].End.Equal(at("2026-10-26T13:40:00Z")) {
		t.Fatalf("interval end = %s, want the 10 minute buffer applied", intervals[0].End)
	}
}

func TestStoreRescheduleAndCancelFreeTheSlot(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	_, holdToken, err := s.CreateHold(HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    "guest@example.com",
		GuestTimezone: "UTC",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := s.ConfirmHold(holdToken, GuestInput{Name: "Guest"}, "guest-hash", fixedNow)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.RescheduleBooking(booking.ID, at("2026-10-27T14:00:00Z"), at("2026-10-27T14:30:00Z"), models.ActorGuest, "Travel", fixedNow); err != nil {
		t.Fatal(err)
	}
	moved, err := s.GetBooking(ownerID, booking.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status != models.BookingStatusRescheduled {
		t.Fatalf("status = %q, want RESCHEDULED", moved.Status)
	}
	if !moved.StartUTC.Equal(at("2026-10-27T14:00:00Z")) {
		t.Fatalf("start = %s, want 2026-10-27T14:00:00Z", moved.StartUTC)
	}

	intervals, err := s.ListActiveIntervals(ownerID, fixedNow, fixedNow.AddDate(0, 0, 7), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(intervals) != 1 || !intervals[0].Start.Equal(at("2026-10-27T13:50:00Z")) {
		t.Fatalf("intervals after reschedule = %+v, want only the new slot", intervals)
	}

	if err := s.CancelBooking(booking.ID, models.ActorGuest, "Conflict"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := s.GetBooking(ownerID, booking.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != models.BookingStatusCancelled || cancelled.CancellationReason != "Conflict" {
		t.Fatalf("cancelled = %+v", cancelled)
	}

	intervals, err = s.ListActiveIntervals(ownerID, fixedNow, fixedNow.AddDate(0, 0, 7), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(intervals) != 0 {
		t.Fatalf("cancelled booking still blocks %+v", intervals)
	}

	audit, err := s.ListAudit(booking.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) < 4 {
		t.Fatalf("audit rows = %d, want hold + confirm + reschedule + cancel", len(audit))
	}
}

func TestStoreRescheduleRejectsCollision(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	createBooking := func(start, end string, token string) models.Booking {
		t.Helper()
		_, holdToken, err := s.CreateHold(HoldInput{
			MeetingTypeID: meetingType.ID,
			Start:         at(start),
			End:           at(end),
			GuestEmail:    token + "@example.com",
			GuestTimezone: "UTC",
			ExpiresAt:     fixedNow.Add(10 * time.Minute),
			Now:           fixedNow,
		})
		if err != nil {
			t.Fatal(err)
		}
		booking, err := s.ConfirmHold(holdToken, GuestInput{Name: token}, token, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		return booking
	}

	first := createBooking("2026-10-26T13:00:00Z", "2026-10-26T13:30:00Z", "first")
	_ = createBooking("2026-10-27T14:00:00Z", "2026-10-27T14:30:00Z", "second")

	err := s.RescheduleBooking(first.ID, at("2026-10-27T14:15:00Z"), at("2026-10-27T14:45:00Z"), models.ActorGuest, "Travel", fixedNow)
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("colliding reschedule err = %v, want ErrSlotUnavailable", err)
	}
}

func TestStoreCalendarConnectionUpsert(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)

	if _, err := s.GetCalendarConnection(ownerID); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("empty connection err = %v, want ErrConnectionNotFound", err)
	}

	connection := models.CalendarConnection{
		UserID:       ownerID,
		Provider:     models.CalendarProviderFake,
		AccountEmail: "owner@example.com",
		SyncStatus:   "ACTIVE",
		LastSyncedAt: fixedNow,
	}
	if err := s.SaveCalendarConnection(connection); err != nil {
		t.Fatal(err)
	}
	connection.Provider = models.CalendarProviderGoogle
	if err := s.SaveCalendarConnection(connection); err != nil {
		t.Fatal(err)
	}

	stored, err := s.GetCalendarConnection(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Provider != models.CalendarProviderGoogle {
		t.Fatalf("provider = %q, want google", stored.Provider)
	}
}

func TestStoreBookingFilter(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	confirmed := 0
	for i := 0; i < 3; i++ {
		start := fixedNow.Add(time.Duration(i+1) * 24 * time.Hour)
		_, holdToken, err := s.CreateHold(HoldInput{
			MeetingTypeID: meetingType.ID,
			Start:         start,
			End:           start.Add(30 * time.Minute),
			GuestEmail:    "guest@example.com",
			GuestTimezone: "UTC",
			ExpiresAt:     fixedNow.Add(10 * time.Minute),
			Now:           fixedNow,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConfirmHold(holdToken, GuestInput{Name: "Guest"}, guesttoken.Hash(holdToken), fixedNow); err != nil {
			t.Fatal(err)
		}
		confirmed++
	}

	all, err := s.ListBookings(ownerID, BookingFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != confirmed {
		t.Fatalf("ListBookings = %d, want %d", len(all), confirmed)
	}

	windowed, err := s.ListBookings(ownerID, BookingFilter{
		From: fixedNow.Add(36 * time.Hour),
		To:   fixedNow.Add(60 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(windowed) != 1 {
		t.Fatalf("windowed ListBookings = %d, want 1", len(windowed))
	}
	if windowed[0].MeetingTypeTitle == "" {
		t.Fatal("joined meeting type title is empty")
	}
}

func TestStoreDefaultMeetingType(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)

	if _, err := s.DefaultMeetingType(); !errors.Is(err, ErrMeetingTypeNotFound) {
		t.Fatalf("empty DefaultMeetingType err = %v, want ErrMeetingTypeNotFound", err)
	}

	created := seedMeetingType(t, s, ownerID, 30)
	found, err := s.DefaultMeetingType()
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != created.ID {
		t.Fatalf("DefaultMeetingType = %d, want %d", found.ID, created.ID)
	}
}

func TestStoreReleaseHoldFreesTheSlotImmediately(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingTypeWithBuffers(t, s, ownerID, 30, 0, 0)

	_, holdToken, err := s.CreateHold(HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    "guest@example.com",
		GuestTimezone: "UTC",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ReleaseHold(holdToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetHold(holdToken, fixedNow); !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("GetHold after release = %v, want ErrHoldExpired", err)
	}

	_, secondToken, err := s.CreateHold(HoldInput{
		MeetingTypeID: meetingType.ID,
		Start:         at("2026-10-26T13:00:00Z"),
		End:           at("2026-10-26T13:30:00Z"),
		GuestEmail:    "other@example.com",
		GuestTimezone: "UTC",
		ExpiresAt:     fixedNow.Add(10 * time.Minute),
		Now:           fixedNow,
	})
	if err != nil {
		t.Fatalf("slot still blocked after release: %v", err)
	}
	_ = secondToken

	if err := s.ReleaseHold("not-a-real-token"); err != nil {
		t.Fatalf("ReleaseHold on an unknown token = %v, want nil", err)
	}
}

func TestStoreListBookingsFiltersByStatusSet(t *testing.T) {
	s := newTestStore(t)
	ownerID := seedOwner(t, s)
	meetingType := seedMeetingType(t, s, ownerID, 30)

	book := func(start string, token string) models.Booking {
		t.Helper()
		_, holdToken, err := s.CreateHold(HoldInput{
			MeetingTypeID: meetingType.ID,
			Start:         at(start),
			End:           at(start).Add(30 * time.Minute),
			GuestEmail:    token + "@example.com",
			GuestTimezone: "UTC",
			ExpiresAt:     fixedNow.Add(10 * time.Minute),
			Now:           fixedNow,
		})
		if err != nil {
			t.Fatal(err)
		}
		booking, err := s.ConfirmHold(holdToken, GuestInput{Name: token}, guesttoken.Hash(holdToken), fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		return booking
	}

	kept := book("2026-10-27T13:00:00Z", "kept")
	dropped := book("2026-10-28T13:00:00Z", "dropped")
	if err := s.CancelBooking(dropped.ID, models.ActorOwner, "test"); err != nil {
		t.Fatal(err)
	}

	active, err := s.ListBookings(ownerID, BookingFilter{
		Statuses: []models.BookingStatus{models.BookingStatusConfirmed, models.BookingStatusRescheduled},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != kept.ID {
		t.Fatalf("active list = %+v, want only booking %d", active, kept.ID)
	}

	cancelled, err := s.ListBookings(ownerID, BookingFilter{Statuses: []models.BookingStatus{models.BookingStatusCancelled}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 1 || cancelled[0].ID != dropped.ID {
		t.Fatalf("cancelled list = %+v, want only booking %d", cancelled, dropped.ID)
	}
}
