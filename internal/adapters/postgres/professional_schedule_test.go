package postgres

import (
	"crypto/rand"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestProfessionalScheduleMaintenancePreservesBookedVisits(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	customer, professional, booked := bookedCancellationFixture(t, ctx, store)
	schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 60, HorizonDays: 7}
	for day := 1; day <= 7; day++ {
		schedule.Week = append(schedule.Week, workflow.WorkingRange{Weekday: day, Start: "08:00", End: "17:00"})
	}
	view, err := store.SaveProfessionalSchedule(ctx, professional, schedule)
	if err != nil || view.Added == 0 {
		t.Fatalf("save schedule: %+v %v", view, err)
	}
	view, err = store.SaveProfessionalSchedule(ctx, professional, schedule)
	if err != nil || view.Added != 0 || view.Removed != 0 {
		t.Fatalf("repeat materialization changed slots: %+v %v", view, err)
	}
	zone := loadTimezone(schedule.Timezone)
	bookedDay := mustParseUTC(booked.Job.Appointment.StartUTC).In(zone).Format("2006-01-02")
	preview, err := store.PreviewProfessionalDaysOff(ctx, professional, workflow.ProfessionalDayOff{FirstDay: bookedDay, LastDay: bookedDay})
	if err != nil || len(preview.BookedVisits) != 1 || preview.BookedVisits[0].JobID != booked.Job.ID {
		t.Fatalf("days off preview lost booking: %+v %v", preview, err)
	}
	day, removed, err := store.AddProfessionalDaysOff(ctx, professional, workflow.ProfessionalDayOff{FirstDay: bookedDay, LastDay: bookedDay, Note: "Holiday"})
	if err != nil || removed == 0 {
		t.Fatalf("days off did not remove free times: %d %v", removed, err)
	}
	if err := store.RemoveProfessionalAvailability(ctx, professional, booked.Job.Appointment.SlotID); err == nil {
		t.Fatal("booked time was removed")
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", booked.WorkspaceID)
	if err != nil || loaded.Job.Appointment == nil || loaded.Job.Appointment.StartUTC != booked.Job.Appointment.StartUTC {
		t.Fatalf("days off moved booked visit: %v", err)
	}
	if _, err := store.RemoveProfessionalDaysOff(ctx, professional, day.ID); err != nil {
		t.Fatal(err)
	}
	window, err := store.ProfessionalAvailabilityWindow(ctx, professional, time.Now().In(zone).AddDate(0, 0, 1).Format("2006-01-02"), 6)
	if err != nil {
		t.Fatal(err)
	}
	var free workflow.ScheduledAvailabilitySlot
	for _, slot := range window.Slots {
		if slot.Status == "available" && slot.Source == "schedule" {
			free = slot
			break
		}
	}
	if free.SlotID == "" {
		t.Fatal("no generated time to remove")
	}
	if err := store.RemoveProfessionalAvailability(ctx, professional, free.SlotID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	window, err = store.ProfessionalAvailabilityWindow(ctx, professional, mustParseUTC(free.StartUTC).In(zone).Format("2006-01-02"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Removed) != 1 || window.Removed[0].StartUTC != free.StartUTC {
		t.Fatalf("single-time exclusion did not survive regeneration: %+v", window.Removed)
	}
	if err := store.RestoreProfessionalAvailability(ctx, professional, free.StartUTC); err != nil {
		t.Fatal(err)
	}
	window, err = store.ProfessionalAvailabilityWindow(ctx, professional, mustParseUTC(free.StartUTC).In(zone).Format("2006-01-02"), 1)
	if err != nil || len(window.Removed) != 0 {
		t.Fatalf("restore failed: %+v %v", window, err)
	}
	found := false
	for _, slot := range window.Slots {
		found = found || slot.StartUTC == free.StartUTC && slot.Status == "available"
	}
	if !found {
		t.Fatal("restored generated time is not bookable")
	}
	manualStart := time.Now().In(zone).AddDate(0, 0, 4)
	manualStart = time.Date(manualStart.Year(), manualStart.Month(), manualStart.Day(), 22, 0, 0, 0, zone)
	manual, err := store.AddProfessionalAvailability(ctx, professional, workflow.Appointment{StartUTC: manualStart.UTC().Format(time.RFC3339), EndUTC: manualStart.Add(time.Hour).UTC().Format(time.RFC3339), Timezone: zone.String()})
	if err != nil {
		t.Fatal(err)
	}
	schedule.Week = []workflow.WorkingRange{}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	slots, err := store.ListProfessionalAvailability(ctx, professional)
	if err != nil {
		t.Fatal(err)
	}
	manualFound, bookedFound := false, false
	for _, slot := range slots {
		manualFound = manualFound || slot.SlotID == manual
		bookedFound = bookedFound || slot.SlotID == booked.Job.Appointment.SlotID && slot.Status == "booked"
	}
	if !manualFound || !bookedFound {
		t.Fatalf("schedule edit lost manual/booked time: %+v", slots)
	}
}

func TestGeneratedTimeRacingHoursSaveNeverDeletesABooking(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	customer, professional, original := bookedCancellationFixture(t, ctx, store)
	schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 60, HorizonDays: 7}
	for day := 1; day <= 7; day++ {
		schedule.Week = append(schedule.Week, workflow.WorkingRange{Weekday: day, Start: "08:00", End: "17:00"})
	}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	slots, err := store.AvailableProfessionalSlots(ctx, professional, "")
	if err != nil || len(slots) == 0 {
		t.Fatalf("no generated time: %v", err)
	}
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	state.Intake = &workflow.Intake{Messages: []workflow.Message{}}
	state.Job = original.Job
	state.Job.ID = "schedule-race-" + rand.Text()
	state.Job.QuoteApproved = false
	state.Job.Appointment = nil
	state.Job.AppointmentConfirmation = nil
	raw, _ := json.Marshal(state)
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	var booked workflow.State
	var bookingErr, saveErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		booked, bookingErr = store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "approve-and-book", Value: slots[0].SlotID})
	}()
	go func() {
		defer wg.Done()
		empty := schedule
		empty.Week = []workflow.WorkingRange{}
		_, saveErr = store.SaveProfessionalSchedule(ctx, professional, empty)
	}()
	wg.Wait()
	if saveErr != nil {
		t.Fatal(saveErr)
	}
	listed, err := store.ListProfessionalAvailability(ctx, professional)
	if err != nil {
		t.Fatal(err)
	}
	if bookingErr == nil {
		found := false
		for _, slot := range listed {
			found = found || slot.SlotID == booked.Job.Appointment.SlotID && slot.Status == "booked"
		}
		if !found {
			t.Fatal("hours save deleted a slot that won the booking race")
		}
		loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
		if err != nil || loaded.Job.Appointment == nil || loaded.Job.Appointment.SlotID != slots[0].SlotID {
			t.Fatalf("successful booking disappeared: %v", err)
		}
	} else {
		loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
		if err != nil || loaded.Job.Appointment != nil {
			t.Fatalf("failed booking partially reserved a removed slot: %v", err)
		}
	}
}

func TestScheduleWindowRollsForwardAndServiceDurationRegeneratesOnlyFreeTimes(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	_, professional, booked := bookedCancellationFixture(t, ctx, store)
	schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 60, HorizonDays: 7}
	for day := 1; day <= 7; day++ {
		schedule.Week = append(schedule.Week, workflow.WorkingRange{Weekday: day, Start: "08:00", End: "17:00"})
	}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	// Simulate a worker that was down when the local booking window advanced.
	zone := loadTimezone(schedule.Timezone)
	last := time.Now().In(zone).AddDate(0, 0, 6).Format("2006-01-02")
	if _, err := store.pool.Exec(ctx, `DELETE FROM professional_availability_slots WHERE professional_account_id=$1 AND source='schedule' AND status='available' AND (starts_at AT TIME ZONE $2)::date=$3::date`, professional, schedule.Timezone, last); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE professional_schedules SET materialized_through=current_date-1 WHERE professional_account_id=$1`, professional); err != nil {
		t.Fatal(err)
	}
	if err := store.materializeRecurringAvailability(ctx); err != nil {
		t.Fatal(err)
	}
	window, err := store.ProfessionalAvailabilityWindow(ctx, professional, last, 1)
	if err != nil || len(window.Slots) == 0 {
		t.Fatalf("rolling pass did not restore the horizon: %+v %v", window, err)
	}
	profile, _, err := store.ProfessionalProfile(ctx, professional)
	if err != nil {
		t.Fatal(err)
	}
	profile.DurationMinutes = 90
	if _, err := store.SaveProfessionalProfile(ctx, professional, profile); err != nil {
		t.Fatal(err)
	}
	window, err = store.ProfessionalAvailabilityWindow(ctx, professional, last, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range window.Slots {
		if slot.Source == "schedule" && slot.Status == "available" && mustParseUTC(slot.EndUTC).Sub(mustParseUTC(slot.StartUTC)) != 90*time.Minute {
			t.Fatalf("duration edit left a stale generated time: %+v", slot)
		}
	}
	listed, err := store.ListProfessionalAvailability(ctx, professional)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, slot := range listed {
		found = found || slot.SlotID == booked.Job.Appointment.SlotID && slot.EndUTC == booked.Job.Appointment.EndUTC
	}
	if !found {
		t.Fatal("duration edit changed the booked visit")
	}
}

func TestAgreedVisitHidesGeneratedTimesIncludingTheTravelGap(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	customer, professional, original := bookedCancellationFixture(t, ctx, store)
	schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 60, HorizonDays: 7}
	for day := 1; day <= 7; day++ {
		schedule.Week = append(schedule.Week, workflow.WorkingRange{Weekday: day, Start: "08:00", End: "17:00"})
	}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	slots, err := store.AvailableProfessionalSlots(ctx, professional, "")
	if err != nil || len(slots) == 0 {
		t.Fatalf("no generated times: %v", err)
	}
	neighbor := slots[0]
	start := mustParseUTC(neighbor.EndUTC).Add(15 * time.Minute)
	state, err := store.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	state.Intake = &workflow.Intake{Messages: []workflow.Message{}}
	state.Job = original.Job
	state.Job.ID = "schedule-chat-" + rand.Text()
	state.Job.Appointment = nil
	state.Job.AppointmentConfirmation = nil
	raw, _ := json.Marshal(state)
	if _, err := store.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	proposed, err := store.ApplyWorkspace(ctx, customer, "customer", state.WorkspaceID, workflow.Action{Actor: "customer", Action: "propose-date", ProposedAppointment: &workflow.Appointment{StartUTC: start.Format(time.RFC3339), EndUTC: start.Add(time.Hour).Format(time.RFC3339), Timezone: "Europe/Amsterdam"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyWorkspace(ctx, professional, "professional", state.WorkspaceID, workflow.Action{Actor: "professional", Action: "accept-date", ProposalID: proposed.Job.DateProposal.ID}); err != nil {
		t.Fatal(err)
	}
	slots, err = store.AvailableProfessionalSlots(ctx, professional, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range slots {
		if slot.SlotID == neighbor.SlotID {
			t.Fatal("a generated time inside the travel gap remained bookable")
		}
	}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", state.WorkspaceID)
	if err != nil || loaded.Job.Appointment == nil || loaded.Job.Appointment.StartUTC != start.Format(time.RFC3339) {
		t.Fatalf("materializer changed the agreed visit: %v", err)
	}
}

func TestScheduleNoticeAndProfileEditsPreserveTheAgreedScope(t *testing.T) {
	ctx, store := accountTestStore(t)
	store.stopBackgroundLoops()
	customer, professional, booked := bookedCancellationFixture(t, ctx, store)
	profile, _, err := store.ProfessionalProfile(ctx, professional)
	if err != nil {
		t.Fatal(err)
	}
	profile.PriceMinor += 1000
	updated, err := store.SaveProfessionalProfile(ctx, professional, profile)
	if err != nil || updated.Status != "active" {
		t.Fatalf("price-only update unexpectedly requires review: %+v %v", updated, err)
	}
	schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 20160, HorizonDays: 7, Week: []workflow.WorkingRange{{Weekday: 1, Start: "08:00", End: "17:00"}}}
	if _, err := store.SaveProfessionalSchedule(ctx, professional, schedule); err != nil {
		t.Fatal(err)
	}
	slots, err := store.AvailableProfessionalSlots(ctx, professional, "")
	if err != nil || len(slots) != 0 {
		t.Fatalf("notice was not applied at read time: %+v %v", slots, err)
	}
	profile.ServiceDescription += " New service scope."
	updated, err = store.SaveProfessionalProfile(ctx, professional, profile)
	if err != nil || updated.Status != "pending" {
		t.Fatalf("scope update bypassed review: %+v %v", updated, err)
	}
	loaded, err := store.LoadWorkspace(ctx, customer, "customer", booked.WorkspaceID)
	if err != nil || loaded.Job.Quote.TotalMinor != booked.Job.Quote.TotalMinor || loaded.Job.ScopeVersion != booked.Job.ScopeVersion || loaded.Job.Appointment.StartUTC != booked.Job.Appointment.StartUTC {
		t.Fatalf("profile edit rewrote the booked agreement: %+v %v", loaded.Job, err)
	}
}
