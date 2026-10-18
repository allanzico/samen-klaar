package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestWorkingHoursProduceThePublishedWeeklyExamples(t *testing.T) {
	for _, example := range []struct{ visit, gap, saturdayEnd, want int }{{90, 30, 13, 22}, {60, 30, 13, 33}, {60, 30, 0, 30}, {60, 0, 0, 45}, {60, 60, 0, 25}} {
		schedule := ProfessionalSchedule{Timezone: "Europe/Amsterdam", VisitMinutes: example.visit, GapMinutes: example.gap, NoticeMinutes: 1440, HorizonDays: 7}
		for day := 1; day <= 5; day++ {
			schedule.Week = append(schedule.Week, WorkingRange{Weekday: day, Start: "08:00", End: "17:00"})
		}
		if example.saturdayEnd > 0 {
			schedule.Week = append(schedule.Week, WorkingRange{Weekday: 6, Start: "09:00", End: "13:00"})
		}
		preview, err := PreviewProfessionalSchedule(schedule)
		if err != nil || preview.WeeklyCount != example.want {
			t.Fatalf("visit=%d gap=%d: count=%d want=%d err=%v", example.visit, example.gap, preview.WeeklyCount, example.want, err)
		}
	}
}

func TestDaysOffValidateLocalDatesRangeAndNote(t *testing.T) {
	now := time.Date(2026, 10, 17, 22, 30, 0, 0, time.UTC) // Already 18 October in Amsterdam.
	for _, example := range []struct {
		day     ProfessionalDayOff
		message string
	}{
		{ProfessionalDayOff{FirstDay: "2026-10-17", LastDay: "2026-10-18"}, "Choose a first day off from today."},
		{ProfessionalDayOff{FirstDay: "2026-10-19", LastDay: "2026-10-18"}, "The last day off must be on or after the first."},
		{ProfessionalDayOff{FirstDay: "2026-10-18", LastDay: "2027-10-19"}, "Days off can cover at most a year."},
		{ProfessionalDayOff{FirstDay: "2026-10-18", LastDay: "2026-10-18", Note: strings.Repeat("é", 81)}, "Keep the days-off note to 80 characters."},
	} {
		if err := ValidateProfessionalDayOff(&example.day, "Europe/Amsterdam", now); err == nil || err.Error() != example.message {
			t.Fatalf("days-off validation: %v want %q", err, example.message)
		}
	}
	valid := ProfessionalDayOff{FirstDay: "2026-10-18", LastDay: "2027-10-18", Note: strings.Repeat("é", 80)}
	if err := ValidateProfessionalDayOff(&valid, "Europe/Amsterdam", now); err != nil {
		t.Fatalf("366 local days and 80 characters should be valid: %v", err)
	}
}

func TestWorkingHoursRespectDaysOffExclusionsAndDaylightSaving(t *testing.T) {
	schedule := ProfessionalSchedule{Timezone: "Europe/Amsterdam", VisitMinutes: 30, GapMinutes: 0, NoticeMinutes: 60, HorizonDays: 7, Week: []WorkingRange{{Weekday: 7, Start: "02:00", End: "04:00"}}}
	for _, example := range []struct {
		day   string
		count int
		first string
	}{{"2026-03-29", 2, "2026-03-29T01:00:00Z"}, {"2026-10-25", 4, "2026-10-25T00:00:00Z"}} {
		now, _ := time.Parse(time.RFC3339, example.day+"T00:00:00Z")
		slots, err := GenerateProfessionalAvailability(schedule, now, nil, nil)
		if err != nil || len(slots) != example.count || slots[0].StartUTC != example.first {
			t.Fatalf("%s: slots=%+v err=%v", example.day, slots, err)
		}
		slots, err = GenerateProfessionalAvailability(schedule, now, []ProfessionalDayOff{{FirstDay: example.day, LastDay: example.day}}, nil)
		if err != nil || len(slots) != 0 {
			t.Fatalf("day off generated slots: %+v %v", slots, err)
		}
		slots, err = GenerateProfessionalAvailability(schedule, now, nil, map[string]bool{example.first: true})
		if err != nil || len(slots) != example.count-1 {
			t.Fatalf("exclusion ignored: %+v %v", slots, err)
		}
	}
}

func TestLongerServiceDoesNotInventTimesInAnExistingShortDay(t *testing.T) {
	schedule := ProfessionalSchedule{Timezone: "Europe/Amsterdam", VisitMinutes: 90, GapMinutes: 30, NoticeMinutes: 60, HorizonDays: 7, Week: []WorkingRange{{Weekday: 1, Start: "08:00", End: "09:00"}}}
	if err := ValidateProfessionalSchedule(&schedule); err == nil {
		t.Fatal("new short working hours were accepted")
	}
	slots, err := GenerateProfessionalAvailability(schedule, time.Now(), nil, nil)
	if err != nil || len(slots) != 0 {
		t.Fatalf("an existing day shorter than the edited service should generate no free times: %+v %v", slots, err)
	}
}
