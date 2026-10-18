package workflow

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TimeWindow is a stretch of local time a person said they are free: some days (by date range and
// weekday) and a part of the day, in minutes after midnight.
type TimeWindow struct {
	FromDate string `json:"fromDate,omitempty"` // YYYY-MM-DD, inclusive; empty = no lower bound
	ToDate   string `json:"toDate,omitempty"`   // YYYY-MM-DD, inclusive; empty = no upper bound
	Weekdays []int  `json:"weekdays,omitempty"` // ISO 1..7; empty = any day
	From     int    `json:"from"`
	To       int    `json:"to"`
}

const (
	dayStart = 8 * 60
	dayEnd   = 20 * 60
)

var (
	weekdayWords = map[string]int{
		"monday": 1, "mon": 1, "maandag": 1, "tuesday": 2, "tue": 2, "tues": 2, "dinsdag": 2, "wednesday": 3, "wed": 3, "woensdag": 3,
		"thursday": 4, "thu": 4, "thur": 4, "thurs": 4, "donderdag": 4, "friday": 5, "fri": 5, "vrijdag": 5,
		"saturday": 6, "sat": 6, "zaterdag": 6, "sunday": 7, "sun": 7, "zondag": 7,
	}
	monthWords = map[string]int{
		"january": 1, "jan": 1, "januari": 1, "february": 2, "feb": 2, "februari": 2, "march": 3, "mar": 3, "maart": 3, "mrt": 3,
		"april": 4, "apr": 4, "may": 5, "mei": 5, "june": 6, "jun": 6, "juni": 6, "july": 7, "jul": 7, "juli": 7,
		"august": 8, "aug": 8, "augustus": 8, "september": 9, "sep": 9, "sept": 9, "october": 10, "oct": 10, "oktober": 10, "okt": 10,
		"november": 11, "nov": 11, "december": 12, "dec": 12,
	}
	wordSplit     = regexp.MustCompile(`[a-zà-ÿ]+`)
	clauseSplit   = regexp.MustCompile(`\s*(?:[,;/]|\bor\b|\bof\b|\band\b|\ben\b|\bplus\b)\s*`)
	isoDate       = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	slashDate     = regexp.MustCompile(`\b(\d{1,2})[/-](\d{1,2})\b`)
	dayMonth      = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)?\s+(?:of\s+)?([a-z]+)\b`)
	monthDay      = regexp.MustCompile(`\b([a-z]+)\s+(\d{1,2})(?:st|nd|rd|th)?\b`)
	clock         = `(\d{1,2})(?:[:.h](\d{2}))?\s*(am|pm|uur)?`
	betweenRange  = regexp.MustCompile(`(?:between|from|tussen|van|vanaf)?\s*` + clock + `\s*(?:-|–|to|until|till|and|en|tot)\s*` + clock)
	dutchCompound = regexp.MustCompile(`\b(maandag|dinsdag|woensdag|donderdag|vrijdag|zaterdag|zondag)(s)?(ochtend|ochtends|middag|middags|avond|avonds)\b`)
	betweenAnd    = regexp.MustCompile(`\b(between|tussen)\s+(\d{1,2}(?:[:.h]\d{2})?\s*(?:am|pm)?)\s*(and|en)\s*(\d{1,2}(?:[:.h]\d{2})?\s*(?:am|pm)?)`)
	afterClock    = regexp.MustCompile(`(?:after|from|na|vanaf|starting)\s+` + clock)
	beforeClock   = regexp.MustCompile(`(?:before|until|till|voor|tot)\s+` + clock)
)

func clockMinutes(hour, minute, suffix string) (int, bool) {
	h, err := strconv.Atoi(hour)
	if err != nil || h > 24 {
		return 0, false
	}
	m := 0
	if minute != "" {
		if m, err = strconv.Atoi(minute); err != nil || m > 59 {
			return 0, false
		}
	}
	switch suffix {
	case "pm":
		if h < 12 {
			h += 12
		}
	case "am":
		if h == 12 {
			h = 0
		}
	}
	return h*60 + m, true
}

// ParseAvailability reads the days and times a person gave in the chat, in English or Dutch: "Tuesday
// afternoon or Thursday after 5", "weekday evenings", "next week", "21 oct 10-12", "any time". It
// returns false when it found nothing it can use, so the agent can ask again.
func ParseAvailability(text string, now time.Time, zone *time.Location) ([]TimeWindow, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return nil, false
	}
	lower = betweenAnd.ReplaceAllString(lower, "$1 $2-$4")
	lower = dutchCompound.ReplaceAllString(lower, "$1 $3") // "dinsdagavond" is "dinsdag avond"
	// "Not on Wednesday", "except Friday", "nooit op woensdag": remove those days instead of adding them.
	var excluded []int
	lower = negation.ReplaceAllStringFunc(lower, func(segment string) string {
		for _, word := range wordSplit.FindAllString(segment, -1) {
			if day, ok := weekdayDay(word); ok && !slices.Contains(excluded, day) {
				excluded = append(excluded, day)
			}
		}
		return ""
	})
	today := now.In(zone)
	dateOf := func(t time.Time) string { return t.Format("2006-01-02") }
	var windows []TimeWindow
	var pendingDayOnly []int // indexes of windows given without a time of day

	for _, clause := range clauseSplit.Split(lower, -1) {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		w := TimeWindow{From: dayStart, To: dayEnd}
		hasDay, hasTime := false, false
		words := wordSplit.FindAllString(clause, -1)
		has := func(options ...string) bool {
			for _, option := range options {
				if slices.Contains(words, option) || strings.Contains(clause, option) && strings.Contains(option, " ") {
					return true
				}
			}
			return false
		}
		for _, word := range words {
			if day, ok := weekdayDay(word); ok {
				w.Weekdays = append(w.Weekdays, day)
				hasDay = true
			}
		}
		switch {
		case has("weekdays", "weekday", "doordeweeks", "werkdagen"):
			w.Weekdays, hasDay = []int{1, 2, 3, 4, 5}, true
		case has("weekend", "weekends", "weekeinde"):
			w.Weekdays, hasDay = []int{6, 7}, true
		}
		switch {
		case has("today", "vandaag"):
			w.FromDate, w.ToDate, hasDay = dateOf(today), dateOf(today), true
		case has("tomorrow", "morgen"):
			w.FromDate, w.ToDate, hasDay = dateOf(today.AddDate(0, 0, 1)), dateOf(today.AddDate(0, 0, 1)), true
		case has("this week", "deze week"):
			end := today.AddDate(0, 0, (7-int(today.Weekday()))%7)
			w.FromDate, w.ToDate, hasDay = dateOf(today), dateOf(end), true
		case has("next week", "volgende week"):
			start := today.AddDate(0, 0, (8-int(today.Weekday()))%7+map[bool]int{true: 7, false: 0}[today.Weekday() == time.Monday])
			w.FromDate, w.ToDate, hasDay = dateOf(start), dateOf(start.AddDate(0, 0, 6)), true
		case has("any time", "anytime", "whenever", "flexible", "flexibel", "altijd", "maakt niet uit", "any day"):
			hasDay = true
			w.From, w.To, hasTime = 0, 24*60, true
		}
		if m := isoDate.FindStringSubmatch(clause); m != nil {
			y, _ := strconv.Atoi(m[1])
			mo, _ := strconv.Atoi(m[2])
			d, _ := strconv.Atoi(m[3])
			w.FromDate = time.Date(y, time.Month(mo), d, 0, 0, 0, 0, zone).Format("2006-01-02")
			w.ToDate, hasDay = w.FromDate, true
		} else if m := dayMonth.FindStringSubmatch(clause); m != nil && monthWords[m[2]] > 0 {
			d, _ := strconv.Atoi(m[1])
			w.FromDate, w.ToDate, hasDay = nextDate(today, monthWords[m[2]], d, zone), nextDate(today, monthWords[m[2]], d, zone), true
		} else if m := monthDay.FindStringSubmatch(clause); m != nil && monthWords[m[1]] > 0 {
			d, _ := strconv.Atoi(m[2])
			w.FromDate, w.ToDate, hasDay = nextDate(today, monthWords[m[1]], d, zone), nextDate(today, monthWords[m[1]], d, zone), true
		} else if m := slashDate.FindStringSubmatch(clause); m != nil && !betweenRange.MatchString(clause) {
			d, _ := strconv.Atoi(m[1])
			mo, _ := strconv.Atoi(m[2])
			if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
				w.FromDate, w.ToDate, hasDay = nextDate(today, mo, d, zone), nextDate(today, mo, d, zone), true
			}
		}
		// Time of day.
		switch {
		case has("morning", "mornings", "ochtend", "ochtends", "voormiddag"):
			w.From, w.To, hasTime = 8*60, 12*60, true
		case has("afternoon", "afternoons", "middag", "middags", "namiddag"):
			w.From, w.To, hasTime = 12*60, 17*60, true
		case has("evening", "evenings", "avond", "avonds", "after work", "na het werk"):
			w.From, w.To, hasTime = 17*60, 21*60, true
		}
		if m := betweenRange.FindStringSubmatch(clause); m != nil {
			from, ok1 := clockMinutes(m[1], m[2], m[3])
			to, ok2 := clockMinutes(m[4], m[5], m[6])
			if ok1 && ok2 && to > from {
				w.From, w.To, hasTime = from, to, true
			}
		} else if m := afterClock.FindStringSubmatch(clause); m != nil {
			if from, ok := clockMinutes(m[1], m[2], m[3]); ok {
				if from < 8*60 && m[3] == "" {
					from += 12 * 60 // "after 5" means five in the afternoon
				}
				w.From, w.To, hasTime = from, 24*60, true
			}
		} else if m := beforeClock.FindStringSubmatch(clause); m != nil {
			if to, ok := clockMinutes(m[1], m[2], m[3]); ok {
				w.From, w.To, hasTime = 0, to, true
			}
		}
		if w.To <= w.From {
			continue
		}
		switch {
		case hasDay:
			windows = append(windows, w)
			if !hasTime {
				pendingDayOnly = append(pendingDayOnly, len(windows)-1)
			}
		case hasTime:
			// "Tuesday or Thursday, afternoons": the time of day belongs to the days named before it.
			if len(pendingDayOnly) > 0 {
				for _, i := range pendingDayOnly {
					windows[i].From, windows[i].To = w.From, w.To
				}
				pendingDayOnly = nil
			} else {
				windows = append(windows, w)
			}
		}
	}
	if len(excluded) > 0 {
		for i := range windows {
			days := windows[i].Weekdays
			if len(days) == 0 {
				days = []int{1, 2, 3, 4, 5, 6, 7}
			}
			windows[i].Weekdays = slices.DeleteFunc(slices.Clone(days), func(d int) bool { return slices.Contains(excluded, d) })
		}
		windows = slices.DeleteFunc(windows, func(w TimeWindow) bool { return len(w.Weekdays) == 0 })
	}
	return windows, len(windows) > 0
}

func nextDate(today time.Time, month, day int, zone *time.Location) string {
	candidate := time.Date(today.Year(), time.Month(month), day, 0, 0, 0, 0, zone)
	if candidate.Before(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, zone)) {
		candidate = candidate.AddDate(1, 0, 0)
	}
	return candidate.Format("2006-01-02")
}

// Allows reports whether a visit lies inside one of the windows.
func WindowsAllow(windows []TimeWindow, visit Appointment, zone *time.Location) bool {
	start, err1 := time.Parse(time.RFC3339, visit.StartUTC)
	end, err2 := time.Parse(time.RFC3339, visit.EndUTC)
	if err1 != nil || err2 != nil {
		return false
	}
	s, e := start.In(zone), end.In(zone)
	if e.Format("2006-01-02") != s.Format("2006-01-02") {
		return false
	}
	date := s.Format("2006-01-02")
	weekday := int(s.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	from, to := s.Hour()*60+s.Minute(), e.Hour()*60+e.Minute()
	for _, w := range windows {
		if w.FromDate != "" && date < w.FromDate || w.ToDate != "" && date > w.ToDate {
			continue
		}
		if len(w.Weekdays) > 0 && !slices.Contains(w.Weekdays, weekday) {
			continue
		}
		if from >= w.From && to <= w.To {
			return true
		}
	}
	return false
}

var negation = regexp.MustCompile(`\b(?:never|not|except|excluding|cannot|can't|cant|behalve|niet|nooit|geen)\b[^,;.]*`)

// weekdayDay reads "saturday", "saturdays" and Dutch "zaterdags" as the same day.
func weekdayDay(word string) (int, bool) {
	if day, ok := weekdayWords[word]; ok {
		return day, true
	}
	if len(word) > 5 {
		for _, suffix := range []string{"s", "en"} {
			if day, ok := weekdayWords[strings.TrimSuffix(word, suffix)]; ok && strings.HasSuffix(word, suffix) {
				return day, true
			}
		}
	}
	return 0, false
}
