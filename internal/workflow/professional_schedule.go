package workflow

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type WorkingRange struct {
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type ProfessionalSchedule struct {
	Timezone      string         `json:"timezone"`
	VisitMinutes  int            `json:"visitMinutes"`
	GapMinutes    int            `json:"gapMinutes"`
	NoticeMinutes int            `json:"noticeMinutes"`
	HorizonDays   int            `json:"horizonDays"`
	Week          []WorkingRange `json:"week"`
}

type ProfessionalDayOff struct {
	ID       string `json:"id"`
	FirstDay string `json:"firstDay"`
	LastDay  string `json:"lastDay"`
	Note     string `json:"note"`
}

type SchedulePreviewDay struct {
	Weekday int      `json:"weekday"`
	Starts  []string `json:"starts"`
}

type SchedulePreview struct {
	WeeklyCount int                  `json:"weeklyCount"`
	TypicalWeek []SchedulePreviewDay `json:"typicalWeek"`
}

type ProfessionalScheduleView struct {
	Schedule    *ProfessionalSchedule `json:"schedule"`
	DaysOff     []ProfessionalDayOff  `json:"daysOff"`
	WeeklyCount int                   `json:"weeklyCount"`
	Added       int                   `json:"added"`
	Removed     int                   `json:"removed"`
}

type ScheduledAvailabilitySlot struct {
	AvailabilitySlot
	Source string `json:"source"`
}

type RemovedAvailabilitySlot struct {
	StartUTC string `json:"startUtc"`
	EndUTC   string `json:"endUtc"`
}

type ProfessionalAvailabilityWindow struct {
	Slots   []ScheduledAvailabilitySlot `json:"slots"`
	Removed []RemovedAvailabilitySlot   `json:"removed"`
	DaysOff []ProfessionalDayOff        `json:"daysOff"`
}

type ScheduledBookedVisit struct {
	JobID       string `json:"jobId"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
	StartUTC    string `json:"startUtc"`
	EndUTC      string `json:"endUtc"`
}

type DaysOffPreview struct {
	RemovedCount int                    `json:"removedCount"`
	BookedVisits []ScheduledBookedVisit `json:"bookedVisits"`
}

func scheduleMinute(value string) (int, bool) {
	var hour, minute int
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	if _, err := fmt.Sscanf(value, "%02d:%02d", &hour, &minute); err != nil || hour < 0 || hour > 24 || minute < 0 || minute > 59 || hour == 24 && minute != 0 || minute%15 != 0 {
		return 0, false
	}
	if fmt.Sprintf("%02d:%02d", hour, minute) != value {
		return 0, false
	}
	return hour*60 + minute, true
}

func ValidateProfessionalSchedule(schedule *ProfessionalSchedule) error {
	return validateProfessionalSchedule(schedule, false)
}

func validateProfessionalSchedule(schedule *ProfessionalSchedule, allowShortDay bool) error {
	if schedule.Timezone == "" {
		schedule.Timezone = "Europe/Amsterdam"
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return errors.New("Choose a valid timezone.")
	}
	if schedule.VisitMinutes < 15 || schedule.VisitMinutes > 360 {
		return errors.New("The service duration must be between 15 and 360 minutes.")
	}
	if schedule.GapMinutes < 0 || schedule.GapMinutes > 120 {
		return errors.New("Choose a gap from 0 to 120 minutes.")
	}
	if schedule.NoticeMinutes < 60 || schedule.NoticeMinutes > 20160 {
		return errors.New("Choose notice from 1 hour to 14 days.")
	}
	if schedule.HorizonDays < 7 || schedule.HorizonDays > 90 {
		return errors.New("Choose a booking window from 7 to 90 days.")
	}
	seen := map[int]bool{}
	for _, day := range schedule.Week {
		if day.Weekday < 1 || day.Weekday > 7 || seen[day.Weekday] {
			return errors.New("Choose one time range per day.")
		}
		seen[day.Weekday] = true
		start, validStart := scheduleMinute(day.Start)
		end, validEnd := scheduleMinute(day.End)
		if !validStart || !validEnd {
			return errors.New("Choose times in 15-minute steps.")
		}
		if start >= end {
			return errors.New("The end must be after the start.")
		}
		if end-start < schedule.VisitMinutes && !allowShortDay {
			return errors.New("This day is shorter than one visit.")
		}
	}
	sort.Slice(schedule.Week, func(i, j int) bool { return schedule.Week[i].Weekday < schedule.Week[j].Weekday })
	return nil
}

func PreviewProfessionalSchedule(schedule ProfessionalSchedule) (SchedulePreview, error) {
	preview := SchedulePreview{TypicalWeek: []SchedulePreviewDay{}}
	if err := validateProfessionalSchedule(&schedule, true); err != nil {
		return preview, err
	}
	for weekday := 1; weekday <= 7; weekday++ {
		day := SchedulePreviewDay{Weekday: weekday, Starts: []string{}}
		for _, work := range schedule.Week {
			if work.Weekday != weekday {
				continue
			}
			start, _ := scheduleMinute(work.Start)
			end, _ := scheduleMinute(work.End)
			for minute := start; minute+schedule.VisitMinutes <= end; minute += schedule.VisitMinutes + schedule.GapMinutes {
				day.Starts = append(day.Starts, fmt.Sprintf("%02d:%02d", minute/60, minute%60))
				preview.WeeklyCount++
			}
		}
		preview.TypicalWeek = append(preview.TypicalWeek, day)
	}
	return preview, nil
}

func ValidateProfessionalDayOff(day *ProfessionalDayOff, timezone string, now time.Time) error {
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		return errors.New("Choose a valid timezone.")
	}
	first, err1 := time.ParseInLocation("2006-01-02", day.FirstDay, zone)
	last, err2 := time.ParseInLocation("2006-01-02", day.LastDay, zone)
	if err1 != nil || err2 != nil {
		return errors.New("Choose valid first and last days off.")
	}
	if day.FirstDay < now.In(zone).Format("2006-01-02") {
		return errors.New("Choose a first day off from today.")
	}
	if last.Before(first) {
		return errors.New("The last day off must be on or after the first.")
	}
	if last.After(first.AddDate(0, 0, 365)) {
		return errors.New("Days off can cover at most a year.")
	}
	day.Note = strings.TrimSpace(day.Note)
	if utf8.RuneCountInString(day.Note) > 80 {
		return errors.New("Keep the days-off note to 80 characters.")
	}
	return nil
}

// GenerateProfessionalAvailability uses local dates for the weekly pattern and
// UTC instants for the resulting visits. Notice is deliberately applied at read
// and reservation time, allowing the same generated rows to age naturally.
func GenerateProfessionalAvailability(schedule ProfessionalSchedule, now time.Time, daysOff []ProfessionalDayOff, exclusions map[string]bool) ([]Appointment, error) {
	slots := []Appointment{}
	if err := validateProfessionalSchedule(&schedule, true); err != nil {
		return slots, err
	}
	zone, _ := time.LoadLocation(schedule.Timezone)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	for offset := 0; offset < schedule.HorizonDays; offset++ {
		date := today.AddDate(0, 0, offset)
		label := date.Format("2006-01-02")
		off := false
		for _, day := range daysOff {
			if label >= day.FirstDay && label <= day.LastDay {
				off = true
				break
			}
		}
		if off {
			continue
		}
		weekday := int(date.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		for _, work := range schedule.Week {
			if work.Weekday != weekday {
				continue
			}
			first, _ := scheduleMinute(work.Start)
			last, _ := scheduleMinute(work.End)
			for minute := first; minute+schedule.VisitMinutes <= last; minute += schedule.VisitMinutes + schedule.GapMinutes {
				start := time.Date(date.Year(), date.Month(), date.Day(), minute/60, minute%60, 0, 0, zone)
				if start.In(zone).Format("2006-01-02 15:04") != fmt.Sprintf("%s %02d:%02d", label, minute/60, minute%60) {
					continue
				}
				// Choose the first occurrence of an ambiguous wall clock time.
				for earlier := start.Add(-3 * time.Hour); earlier.Before(start); earlier = earlier.Add(15 * time.Minute) {
					if earlier.In(zone).Format("2006-01-02 15:04") == start.In(zone).Format("2006-01-02 15:04") {
						start = earlier
						break
					}
				}
				key := start.UTC().Format(time.RFC3339)
				if exclusions[key] {
					continue
				}
				end := start.Add(time.Duration(schedule.VisitMinutes) * time.Minute)
				slots = append(slots, Appointment{StartUTC: key, EndUTC: end.UTC().Format(time.RFC3339), Timezone: schedule.Timezone, StartsAt: start.Format("Mon, 2 Jan · 15:04") + "–" + end.In(zone).Format("15:04")})
			}
		}
	}
	return slots, nil
}
