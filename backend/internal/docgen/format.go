package docgen

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Formatting helpers for template values. Templates receive every number and
// date as an already formatted string (kamus_variabel.json, "format_angka").

var monthsID = [12]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

var monthsEN = [12]string{
	"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

var romanMonths = [12]string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII"}

// FormatDateID formats t as "1 Oktober 2026".
func FormatDateID(t time.Time) string {
	return strconv.Itoa(t.Day()) + " " + monthsID[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// FormatDateEN formats t as "1 October 2026".
func FormatDateEN(t time.Time) string {
	return strconv.Itoa(t.Day()) + " " + monthsEN[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// FormatDMY formats t as "14-02-1998".
func FormatDMY(t time.Time) string {
	return t.Format("02-01-2006")
}

// PeriodID formats the month of t as "Oktober 2026".
func PeriodID(t time.Time) string {
	return monthsID[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// PeriodEN formats the month of t as "October 2026".
func PeriodEN(t time.Time) string {
	return monthsEN[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// RomanMonth returns the Roman numeral used in document numbers
// (021/PKWT/CTN/X/2026). It returns "" for an invalid month.
func RomanMonth(m time.Month) string {
	if m < time.January || m > time.December {
		return ""
	}
	return romanMonths[m-1]
}

// Thousands formats n with '.' thousand separators and no currency symbol:
// 8000000 -> "8.000.000", -1500 -> "-1.500".
func Thousands(n int64) string {
	neg := n < 0
	u := uint64(n)
	if neg {
		u = -u // two's complement also handles math.MinInt64
	}
	digits := strconv.FormatUint(u, 10)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte('.')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

var terbilangDigits = [10]string{"", "satu", "dua", "tiga", "empat", "lima", "enam", "tujuh", "delapan", "sembilan"}

var terbilangScales = []string{"", "ribu", "juta", "miliar", "triliun", "kuadriliun", "kuintiliun"}

// Terbilang spells n in Indonesian words: 0 -> "nol", 11 -> "sebelas",
// 1000 -> "seribu", 9300000 -> "sembilan juta tiga ratus ribu".
func Terbilang(n int64) string {
	if n == 0 {
		return "nol"
	}
	neg := n < 0
	u := uint64(n)
	if neg {
		u = -u
	}
	var groups []uint64
	for u > 0 {
		groups = append(groups, u%1000)
		u /= 1000
	}
	var parts []string
	for i := len(groups) - 1; i >= 0; i-- {
		g := groups[i]
		if g == 0 {
			continue
		}
		if i == 1 && g == 1 {
			parts = append(parts, "seribu")
			continue
		}
		parts = append(parts, terbilangBelowThousand(g))
		if i > 0 {
			parts = append(parts, terbilangScales[i])
		}
	}
	s := strings.Join(parts, " ")
	if neg {
		s = "minus " + s
	}
	return s
}

// TerbilangRupiah is Terbilang followed by "rupiah", as the templates print
// amounts ("delapan juta rupiah").
func TerbilangRupiah(n int64) string {
	return Terbilang(n) + " rupiah"
}

func terbilangBelowThousand(g uint64) string {
	var parts []string
	h, r := g/100, g%100
	switch {
	case h == 1:
		parts = append(parts, "seratus")
	case h > 1:
		parts = append(parts, terbilangDigits[h], "ratus")
	}
	switch {
	case r == 0:
	case r < 10:
		parts = append(parts, terbilangDigits[r])
	case r == 10:
		parts = append(parts, "sepuluh")
	case r == 11:
		parts = append(parts, "sebelas")
	case r < 20:
		parts = append(parts, terbilangDigits[r-10], "belas")
	default:
		parts = append(parts, terbilangDigits[r/10], "puluh")
		if r%10 != 0 {
			parts = append(parts, terbilangDigits[r%10])
		}
	}
	return strings.Join(parts, " ")
}

// MaskAccount hides a bank account number except its last four digits, with a
// fixed-width mask so the length is not revealed: "1234567890" ->
// "******7890". Spaces, dots and dashes are ignored; inputs with four or fewer
// characters are fully masked.
func MaskAccount(account string) string {
	var digits []rune
	for _, r := range account {
		if r == ' ' || r == '-' || r == '.' {
			continue
		}
		digits = append(digits, r)
	}
	if len(digits) <= 4 {
		if len(digits) == 0 {
			return ""
		}
		return "******"
	}
	return "******" + string(digits[len(digits)-4:])
}

// SingleLine turns any line break, tab or other control character (including
// U+000B, U+2028 and U+2029) into a space, collapses runs of whitespace and
// trims the result. Every template value is single line.
func SingleLine(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == ' ' || r == ' ' {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// PrevWorkday returns t when it falls on Monday-Friday, otherwise the Friday
// before it. Public holidays are not considered.
func PrevWorkday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, -1)
	case time.Sunday:
		return t.AddDate(0, 0, -2)
	}
	return t
}
