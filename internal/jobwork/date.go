package jobwork

import (
	"errors"
	"strings"
	"time"
)

// DateLayout is the canonical form a date is written and stored in: YYYY-MM-DD.
const DateLayout = "2006-01-02"

// inputLayouts are the forms ParseDate accepts. The canonical ISO form is tried
// first; the day-first forms are the ones hand-written challan books use in
// India, so "02/07/2025" is read as 2 July 2025, never as 7 February.
var inputLayouts = []string{DateLayout, "02/01/2006", "02-01-2006", "2006/01/02"}

// ErrBadDate is returned when a string is not a date in any accepted form.
var ErrBadDate = errors.New("date must be written as YYYY-MM-DD (or DD/MM/YYYY)")

// Date is a calendar date with no time of day and no time zone: the only thing
// a delivery challan carries. It is held at midnight UTC so that subtracting
// two dates gives a whole number of days.
type Date struct {
	t time.Time
}

// NewDate builds a date from its parts. Out-of-range parts are normalised the
// way the calendar does it, so NewDate(2025, time.February, 30) is 2 March 2025.
func NewDate(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// MustDate is NewDate for fixed dates written into code and tests.
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// ParseDate reads a date written in any of the accepted forms. An empty string
// is the zero date, which is how an absent date travels.
func ParseDate(s string) (Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Date{}, nil
	}
	for _, layout := range inputLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return Date{t: t}, nil
		}
	}
	return Date{}, ErrBadDate
}

// Today is the current date in the machine's own time zone.
func Today() Date {
	now := time.Now()
	return NewDate(now.Year(), now.Month(), now.Day())
}

// IsZero reports whether the date is absent.
func (d Date) IsZero() bool { return d.t.IsZero() }

// String writes the date in the canonical YYYY-MM-DD form; the zero date is "".
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(DateLayout)
}

// Time exposes the underlying instant (midnight UTC) for formatting.
func (d Date) Time() time.Time { return d.t }

// Year returns the calendar year.
func (d Date) Year() int { return d.t.Year() }

// Month returns the calendar month.
func (d Date) Month() time.Month { return d.t.Month() }

// Day returns the day of the month.
func (d Date) Day() int { return d.t.Day() }

// AddYears moves the date on by whole calendar years. It uses calendar
// arithmetic, so the day and month are kept where they exist: 2025-07-02 plus
// one year is 2026-07-02. Where the day does not exist in the target year the
// calendar carries it over, so 2024-02-29 plus one year is 2025-03-01 — a
// challan sent on a leap day therefore falls due on 1 March.
func (d Date) AddYears(n int) Date {
	if d.IsZero() {
		return d
	}
	return Date{t: d.t.AddDate(n, 0, 0)}
}

// AddDays moves the date on by whole days.
func (d Date) AddDays(n int) Date {
	if d.IsZero() {
		return d
	}
	return Date{t: d.t.AddDate(0, 0, n)}
}

// Before reports whether d falls earlier than o.
func (d Date) Before(o Date) bool { return d.t.Before(o.t) }

// After reports whether d falls later than o.
func (d Date) After(o Date) bool { return d.t.After(o.t) }

// Equal reports whether both dates are the same calendar day.
func (d Date) Equal(o Date) bool { return d.t.Equal(o.t) }

// OnOrBefore reports whether d is the same day as o or earlier.
func (d Date) OnOrBefore(o Date) bool { return !d.t.After(o.t) }

// DaysBetween counts whole days from a to b. It is negative when b is earlier
// than a, and counts calendar days, so it is unaffected by daylight saving.
func DaysBetween(a, b Date) int {
	return int(b.t.Sub(a.t) / (24 * time.Hour))
}

// MarshalJSON writes the date as a "YYYY-MM-DD" string, or null when absent.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.String() + `"`), nil
}

// UnmarshalJSON reads a date from a JSON string; null and "" give the zero date.
func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*d = Date{}
		return nil
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return ErrBadDate
	}
	parsed, err := ParseDate(s[1 : len(s)-1])
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
