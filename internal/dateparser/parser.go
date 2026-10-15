// Package dateparser turns natural-language date hints into [time.Time] values.
package dateparser

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidDate = errors.New("invalid date hint")

var (
	reISO      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reISOTime  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{1,2}:\d{2}$`)
	reRelative = regexp.MustCompile(`^in\s+(\d+)\s*([hmdw])(?:\s+(?:at\s+)?(.+))?$`)
	reTime     = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	reTimeAMPM = regexp.MustCompile(`^(\d{1,2})\s*(am|pm)$`)
)

const (
	daysPerWeek = 7

	wordToday    = "today"
	wordTomorrow = "tomorrow"
)

// Parse converts a date hint into a concrete time. It checks ISO dates and
// relative durations first, then accepts a bare time or a day/weekday with an
// optional time. Bare times in the past roll to tomorrow; a past time attached
// to today or the current weekday rolls to the next week. Missing times use
// defaultTime (HH:MM). Relative days/weeks keep the current time unless an
// explicit clock is supplied; relative hours/minutes are elapsed offsets.
// now and defaultTime are injected for determinism.
func Parse(s string, now time.Time, defaultTime string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, invalid(s)
	}
	if t, matched, err := parseExact(s, now, defaultTime); matched || err != nil {
		return t, err
	}
	if t, matched, err := parseRelative(s, now); matched || err != nil {
		return t, err
	}

	// Bare time: today at that time, rolled to tomorrow if already past.
	lower := strings.ToLower(s)
	if hm, err := parseHM(lower); err == nil {
		return timeOnly(hm, now), nil
	}
	return parseDayAndTime(s, now, defaultTime)
}

func parseExact(s string, now time.Time, defaultTime string) (time.Time, bool, error) {
	if reISO.MatchString(s) {
		t, err := withDefaultTime(s, "2006-01-02", defaultTime, now)
		return t, true, err
	}
	if reISOTime.MatchString(s) {
		t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		if err != nil {
			return time.Time{}, true, invalid(s)
		}
		return t, true, nil
	}
	return time.Time{}, false, nil
}

func parseRelative(s string, now time.Time) (time.Time, bool, error) {
	m := reRelative.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return time.Time{}, false, nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return time.Time{}, true, invalid(s)
	}
	if m[3] != "" && (m[2] == "h" || m[2] == "m") {
		return time.Time{}, true, invalid(s)
	}
	var days int
	switch m[2] {
	case "h":
		return now.Add(time.Duration(n) * time.Hour), true, nil
	case "m":
		return now.Add(time.Duration(n) * time.Minute), true, nil
	case "d":
		days = n
	case "w":
		days = daysPerWeek * n
	default:
		return time.Time{}, true, invalid(s)
	}
	if m[3] == "" {
		return now.AddDate(0, 0, days), true, nil
	}
	hm, err := parseHM(strings.ReplaceAll(m[3], " ", ""))
	if err != nil {
		return time.Time{}, true, invalid(s)
	}
	return time.Date(now.Year(), now.Month(), now.Day()+days, hm[0], hm[1], 0, 0, now.Location()), true, nil
}

func parseDayAndTime(s string, now time.Time, defaultTime string) (time.Time, error) {
	fields := strings.Fields(s)
	head := strings.ToLower(fields[0])

	var day time.Time
	haveDay := false
	switch head {
	case wordToday:
		day = midnight(now)
		haveDay = true
	case wordTomorrow:
		day = midnight(now).AddDate(0, 0, 1)
		haveDay = true
	default:
		if wd, ok := weekday(head); ok {
			// Go keeps the sign of a negative remainder; add a week before
			// taking modulo so earlier weekdays wrap into the next week.
			delta := (int(wd) - int(now.Weekday()) + daysPerWeek) % daysPerWeek
			day = midnight(now).AddDate(0, 0, delta)
			haveDay = true
		}
	}
	if !haveDay {
		return time.Time{}, invalid(s)
	}

	rest := fields[1:]
	if len(rest) > 0 && strings.EqualFold(rest[0], "at") {
		rest = rest[1:]
	}
	timeStr := defaultTime
	if len(rest) > 0 {
		timeStr = strings.ToLower(strings.Join(rest, ""))
	}
	hm, err := parseHM(timeStr)
	if err != nil {
		return time.Time{}, invalid(s)
	}
	t := time.Date(day.Year(), day.Month(), day.Day(), hm[0], hm[1], 0, 0, now.Location())

	// "today" (or same-weekday) with a time that's already past → next week.
	if day.Equal(midnight(now)) && !t.After(now) && len(rest) > 0 {
		t = t.AddDate(0, 0, daysPerWeek)
	}
	return t, nil
}

func weekday(name string) (time.Weekday, bool) {
	switch name {
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	case "sun", "sunday":
		return time.Sunday, true
	}
	return 0, false
}

func midnight(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func timeOnly(hm [2]int, now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hm[0], hm[1], 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func parseHM(s string) ([2]int, error) {
	if m := reTime.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		minute, _ := strconv.Atoi(m[2])
		if h > 23 || minute > 59 {
			return [2]int{}, fmt.Errorf("bad time")
		}
		return [2]int{h, minute}, nil
	}
	if m := reTimeAMPM.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		if h == 0 || h > 12 {
			return [2]int{}, fmt.Errorf("bad time")
		}
		if m[2] == "pm" && h != 12 {
			h += 12
		}
		if m[2] == "am" && h == 12 {
			h = 0
		}
		return [2]int{h, 0}, nil
	}
	return [2]int{}, fmt.Errorf("bad time")
}

func withDefaultTime(s, layout, defaultTime string, now time.Time) (time.Time, error) {
	t, err := time.ParseInLocation(layout, s, time.Local)
	if err != nil {
		return time.Time{}, invalid(s)
	}
	hm, err := parseHM(defaultTime)
	if err != nil {
		return time.Time{}, invalid(s)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), hm[0], hm[1], 0, 0, now.Location()), nil
}

func invalid(s string) error {
	return fmt.Errorf("%w: %q", ErrInvalidDate, s)
}
