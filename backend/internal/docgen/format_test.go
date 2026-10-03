package docgen

import (
	"math"
	"testing"
	"time"
)

func TestFormatDates(t *testing.T) {
	d := time.Date(2026, time.October, 1, 15, 4, 0, 0, time.UTC)
	if got := FormatDateID(d); got != "1 Oktober 2026" {
		t.Errorf("FormatDateID = %q", got)
	}
	if got := FormatDateEN(d); got != "1 October 2026" {
		t.Errorf("FormatDateEN = %q", got)
	}
	if got := FormatDMY(time.Date(1998, time.February, 14, 0, 0, 0, 0, time.UTC)); got != "14-02-1998" {
		t.Errorf("FormatDMY = %q", got)
	}
	if got := PeriodID(d); got != "Oktober 2026" {
		t.Errorf("PeriodID = %q", got)
	}
	if got := PeriodEN(d); got != "October 2026" {
		t.Errorf("PeriodEN = %q", got)
	}
	months := map[time.Month][2]string{
		time.January: {"Januari", "January"}, time.February: {"Februari", "February"},
		time.March: {"Maret", "March"}, time.May: {"Mei", "May"}, time.August: {"Agustus", "August"},
		time.December: {"Desember", "December"},
	}
	for m, names := range months {
		day := time.Date(2027, m, 1, 0, 0, 0, 0, time.UTC)
		if got := FormatDateID(day); got != "1 "+names[0]+" 2027" {
			t.Errorf("FormatDateID(%v) = %q", m, got)
		}
		if got := FormatDateEN(day); got != "1 "+names[1]+" 2027" {
			t.Errorf("FormatDateEN(%v) = %q", m, got)
		}
	}
}

func TestRomanMonth(t *testing.T) {
	want := []string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII"}
	for i, w := range want {
		if got := RomanMonth(time.Month(i + 1)); got != w {
			t.Errorf("RomanMonth(%d) = %q, want %q", i+1, got, w)
		}
	}
	if RomanMonth(0) != "" || RomanMonth(13) != "" {
		t.Error("invalid months must return empty")
	}
}

func TestThousands(t *testing.T) {
	cases := map[int64]string{
		0:             "0",
		7:             "7",
		999:           "999",
		1000:          "1.000",
		185000:        "185.000",
		8000000:       "8.000.000",
		9905000:       "9.905.000",
		1234567890:    "1.234.567.890",
		-1500:         "-1.500",
		math.MaxInt64: "9.223.372.036.854.775.807",
		math.MinInt64: "-9.223.372.036.854.775.808",
	}
	for n, want := range cases {
		if got := Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTerbilang(t *testing.T) {
	cases := map[int64]string{
		0:                   "nol",
		1:                   "satu",
		10:                  "sepuluh",
		11:                  "sebelas",
		12:                  "dua belas",
		19:                  "sembilan belas",
		20:                  "dua puluh",
		21:                  "dua puluh satu",
		100:                 "seratus",
		101:                 "seratus satu",
		111:                 "seratus sebelas",
		250:                 "dua ratus lima puluh",
		999:                 "sembilan ratus sembilan puluh sembilan",
		1000:                "seribu",
		1001:                "seribu satu",
		1100:                "seribu seratus",
		2000:                "dua ribu",
		11000:               "sebelas ribu",
		100000:              "seratus ribu",
		101000:              "seratus satu ribu",
		1000000:             "satu juta",
		8000000:             "delapan juta",
		9905000:             "sembilan juta sembilan ratus lima ribu",
		10835000:            "sepuluh juta delapan ratus tiga puluh lima ribu",
		1000001000:          "satu miliar seribu",
		2500000000:          "dua miliar lima ratus juta",
		1000000000000:       "satu triliun",
		1000000000000000:    "satu kuadriliun",
		1000000000000000000: "satu kuintiliun",
		-2500:               "minus dua ribu lima ratus",
		math.MaxInt64: "sembilan kuintiliun dua ratus dua puluh tiga kuadriliun tiga ratus tujuh puluh dua triliun " +
			"tiga puluh enam miliar delapan ratus lima puluh empat juta tujuh ratus tujuh puluh lima ribu " +
			"delapan ratus tujuh",
	}
	for n, want := range cases {
		if got := Terbilang(n); got != want {
			t.Errorf("Terbilang(%d) = %q, want %q", n, got, want)
		}
	}
	if got := TerbilangRupiah(8000000); got != "delapan juta rupiah" {
		t.Errorf("TerbilangRupiah = %q", got)
	}
	if got := Terbilang(math.MinInt64); got[:6] != "minus " {
		t.Errorf("Terbilang(MinInt64) = %q", got)
	}
}

func TestMaskAccount(t *testing.T) {
	cases := map[string]string{
		"1234567890":          "******7890",
		"123-456-7890":        "******7890",
		"1234 5678 9012 3456": "******3456",
		"12345":               "******2345",
		"1234":                "******",
		"":                    "",
	}
	for in, want := range cases {
		if got := MaskAccount(in); got != want {
			t.Errorf("MaskAccount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSingleLine(t *testing.T) {
	cases := map[string]string{
		"plain":                       "plain",
		"  a\r\nb\tc  ":               "a b c",
		"x\vy\fz":                     "x y z",
		"line sep para":               "line sep para",
		"nul\x00byte":                 "nul byte",
		"Senin–Jumat / Monday–Friday": "Senin–Jumat / Monday–Friday",
	}
	for in, want := range cases {
		if got := SingleLine(in); got != want {
			t.Errorf("SingleLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrevWorkday(t *testing.T) {
	cases := map[string]string{
		"2026-09-25": "2026-09-25", // Friday
		"2026-10-24": "2026-10-23", // Saturday -> Friday
		"2026-10-25": "2026-10-23", // Sunday -> Friday
		"2026-10-26": "2026-10-26", // Monday
		"2026-11-01": "2026-10-30", // Sunday across a month boundary
	}
	for in, want := range cases {
		d, _ := time.Parse("2006-01-02", in)
		if got := PrevWorkday(d).Format("2006-01-02"); got != want {
			t.Errorf("PrevWorkday(%s) = %s, want %s", in, got, want)
		}
	}
}
