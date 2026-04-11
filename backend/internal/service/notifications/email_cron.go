package notifications

import (
	"strconv"
	"strings"
	"time"
)

func emailCronMatches(expression string, ts time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(expression))
	if len(fields) != 5 {
		return false
	}

	ts = ts.In(time.Local)
	return matchEmailCronField(fields[0], ts.Minute(), 0, 59, false) &&
		matchEmailCronField(fields[1], ts.Hour(), 0, 23, false) &&
		matchEmailCronField(fields[2], ts.Day(), 1, 31, false) &&
		matchEmailCronField(fields[3], int(ts.Month()), 1, 12, false) &&
		matchEmailCronField(fields[4], int(ts.Weekday()), 0, 6, true)
}

func matchEmailCronField(field string, value int, min int, max int, weekday bool) bool {
	for _, part := range strings.Split(field, ",") {
		if emailCronPartMatches(strings.TrimSpace(part), value, min, max, weekday) {
			return true
		}
	}
	return false
}

func emailCronPartMatches(part string, value int, min int, max int, weekday bool) bool {
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
		parsedStart, err := parseEmailCronNumber(rangeParts[0], min, max, weekday)
		if err != nil {
			return false
		}
		parsedEnd, err := parseEmailCronNumber(rangeParts[1], min, max, weekday)
		if err != nil {
			return false
		}
		start = parsedStart
		end = parsedEnd
	default:
		single, err := parseEmailCronNumber(base, min, max, weekday)
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
		if normalizeEmailCronValue(candidate, weekday) == value {
			return true
		}
	}
	return false
}

func parseEmailCronNumber(value string, min int, max int, weekday bool) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	parsed = normalizeEmailCronValue(parsed, weekday)
	if parsed < min || parsed > max {
		return 0, strconv.ErrSyntax
	}
	return parsed, nil
}

func normalizeEmailCronValue(value int, weekday bool) int {
	if weekday && value == 7 {
		return 0
	}
	return value
}
