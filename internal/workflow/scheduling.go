package workflow

import (
	"errors"
	"time"
)

const (
	confirmationWindow = 2 * time.Hour
	businessOpenHour   = 9
	businessCloseHour  = 17
)

// AppointmentConfirmationDeadline uses a two-business-hour response window,
// Monday to Friday, 09:00–17:00 Europe/Amsterdam. The appointment timezone is
// validated for display but cannot change the operational response clock. A request made
// outside those hours starts counting at the next opening time.
func AppointmentConfirmationDeadline(from time.Time, timezone string) (time.Time, error) {
	if _, err := time.LoadLocation(timezone); err != nil {
		return time.Time{}, errors.New("appointment timezone is invalid")
	}
	location, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		return time.Time{}, err
	}
	current := from.In(location)
	remaining := confirmationWindow
	for range 370 {
		current = nextBusinessOpening(current, location)
		closing := time.Date(current.Year(), current.Month(), current.Day(), businessCloseHour, 0, 0, 0, location)
		available := closing.Sub(current)
		if remaining <= available {
			return current.Add(remaining), nil
		}
		remaining -= available
		current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, location)
	}
	return time.Time{}, errors.New("could not calculate appointment confirmation deadline")
}

func nextBusinessOpening(current time.Time, location *time.Location) time.Time {
	current = current.In(location)
	for current.Weekday() == time.Saturday || current.Weekday() == time.Sunday {
		current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, location)
	}
	opening := time.Date(current.Year(), current.Month(), current.Day(), businessOpenHour, 0, 0, 0, location)
	closing := time.Date(current.Year(), current.Month(), current.Day(), businessCloseHour, 0, 0, 0, location)
	if current.Before(opening) {
		return opening
	}
	if !current.Before(closing) {
		current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, location)
		return nextBusinessOpening(current, location)
	}
	return current
}
