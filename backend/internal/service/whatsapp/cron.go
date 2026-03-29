package whatsapp

import (
	"strconv"
	"strings"
	"time"
)

func cronMatches(expression string, ts time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(expression))
	if len(fields) != 5 {
		return false
	}

	ts = ts.In(time.Local)
	return matchCronField(fields[0], ts.Minute(), 0, 59, false) &&
		matchCronField(fields[1], ts.Hour(), 0, 23, false) &&
		matchCronField(fields[2], ts.Day(), 1, 31, false) &&
		matchCronField(fields[3], int(ts.Month()), 1, 12, false) &&
		matchCronField(fields[4], int(ts.Weekday()), 0, 6, true)
}

func nextCronOccurrence(expression string, after time.Time) *time.Time {
	if strings.TrimSpace(expression) == "" {
		return nil
	}

	candidate := after.In(time.Local).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if cronMatches(expression, candidate) {
			next := candidate
			return &next
		}
		candidate = candidate.Add(time.Minute)
	}

	return nil
}

func matchCronField(field string, value int, min int, max int, weekday bool) bool {
	for _, part := range strings.Split(field, ",") {
		if cronPartMatches(strings.TrimSpace(part), value, min, max, weekday) {
			return true
		}
	}
	return false
}

func cronPartMatches(part string, value int, min int, max int, weekday bool) bool {
	if part == "" {
		return false
	}

	step := 1
	base := part
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		if len(pieces) != 2 {
			return false
		}
		base = pieces[0]
		parsedStep, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
		if err != nil || parsedStep <= 0 {
			return false
		}
		step = parsedStep
	}

	start := min
	end := max
	switch {
	case base == "" || base == "*":
	case strings.Contains(base, "-"):
		rangeParts := strings.SplitN(base, "-", 2)
		if len(rangeParts) != 2 {
			return false
		}
		parsedStart, err := parseCronNumber(rangeParts[0], min, max, weekday)
		if err != nil {
			return false
		}
		parsedEnd, err := parseCronNumber(rangeParts[1], min, max, weekday)
		if err != nil {
			return false
		}
		start = parsedStart
		end = parsedEnd
	default:
		single, err := parseCronNumber(base, min, max, weekday)
		if err != nil {
			return false
		}
		start = single
		end = single
	}

	if start > end {
		return false
	}

	for candidate := start; candidate <= end; candidate += step {
		if normalizeCronValue(candidate, weekday) == value {
			return true
		}
	}
	return false
}

func parseCronNumber(value string, min int, max int, weekday bool) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}

	parsed = normalizeCronValue(parsed, weekday)
	if parsed < min || parsed > max {
		return 0, strconv.ErrSyntax
	}
	return parsed, nil
}

func normalizeCronValue(value int, weekday bool) int {
	if weekday && value == 7 {
		return 0
	}
	return value
}
