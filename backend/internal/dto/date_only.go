package dto

import (
	"fmt"
	"strings"
	"time"
)

const DateOnlyLayout = "2006-01-02"

type DateOnly string

func ParseDateOnly(value DateOnly) (time.Time, error) {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}

	parsed, err := time.Parse(DateOnlyLayout, trimmed)
	if err == nil {
		return parsed.UTC(), nil
	}

	timestamp, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q", value)
	}

	return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, time.UTC), nil
}
