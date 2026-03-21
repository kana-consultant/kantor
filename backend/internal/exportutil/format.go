package exportutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func FormatIDR(amount int64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	raw := strconv.FormatInt(amount, 10)
	parts := make([]string, 0, len(raw)/3+1)
	for len(raw) > 3 {
		parts = append([]string{raw[len(raw)-3:]}, parts...)
		raw = raw[:len(raw)-3]
	}
	parts = append([]string{raw}, parts...)
	return sign + "Rp " + strings.Join(parts, ".")
}

func FormatDate(value time.Time) string {
	return value.Format("02/01/2006")
}

func FormatDateTime(value time.Time) string {
	return value.Format("02/01/2006 15:04")
}

func OptionalString(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func Filename(prefix, format string) string {
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s-%s.%s", prefix, timestamp, strings.ToLower(strings.TrimSpace(format)))
}
