package jobwork

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"iso", "2025-07-02", "2025-07-02", false},
		{"iso with spaces", "  2025-07-02 ", "2025-07-02", false},
		{"day first with slashes", "02/07/2025", "2025-07-02", false},
		{"day first with hyphens", "02-07-2025", "2025-07-02", false},
		{"day first beyond the twelfth", "28/02/2025", "2025-02-28", false},
		{"year first with slashes", "2025/07/02", "2025-07-02", false},
		{"leap day", "2024-02-29", "2024-02-29", false},
		{"empty is the zero date", "", "", false},
		{"spaces only is the zero date", "   ", "", false},
		{"day that does not exist", "2025-02-30", "", true},
		{"month that does not exist", "2025-13-01", "", true},
		{"leap day in a common year", "2025-02-29", "", true},
		{"words", "tomorrow", "", true},
		{"short year", "25-07-02", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseDate(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseDate(%q) = %v, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDate(%q) returned %v", c.in, err)
			}
			if got.String() != c.want {
				t.Errorf("ParseDate(%q) = %q, want %q", c.in, got.String(), c.want)
			}
		})
	}
}

func TestAddYears(t *testing.T) {
	cases := []struct {
		name  string
		from  string
		years int
		want  string
	}{
		{"ordinary date plus one year", "2025-07-02", 1, "2026-07-02"},
		{"ordinary date plus three years", "2025-07-02", 3, "2028-07-02"},
		{"leap day plus one year rolls to 1 March", "2024-02-29", 1, "2025-03-01"},
		{"leap day plus three years rolls to 1 March", "2024-02-29", 3, "2027-03-01"},
		{"leap day plus four years lands on a leap day", "2024-02-29", 4, "2028-02-29"},
		{"28 February into a leap year", "2023-02-28", 1, "2024-02-28"},
		{"28 February plus three years into a leap year", "2021-02-28", 3, "2024-02-28"},
		{"31 March keeps the day", "2024-03-31", 1, "2025-03-31"},
		{"31 December keeps the day", "2024-12-31", 1, "2025-12-31"},
		{"1 March after a leap day", "2024-03-01", 1, "2025-03-01"},
		{"zero years is the same date", "2024-02-29", 0, "2024-02-29"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MustDate(c.from).AddYears(c.years)
			if got.String() != c.want {
				t.Errorf("%s plus %d year(s) = %s, want %s", c.from, c.years, got, c.want)
			}
		})
	}

	t.Run("the zero date stays the zero date", func(t *testing.T) {
		if got := (Date{}).AddYears(1); !got.IsZero() {
			t.Errorf("zero date plus a year = %s, want the zero date", got)
		}
	})
}

func TestDaysBetween(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"same day", "2025-07-02", "2025-07-02", 0},
		{"next day", "2025-07-02", "2025-07-03", 1},
		{"backwards is negative", "2025-07-03", "2025-07-02", -1},
		{"across a leap day", "2024-02-28", "2024-03-01", 2},
		{"across 28 February in a common year", "2025-02-28", "2025-03-01", 1},
		{"a leap year is 366 days", "2024-01-01", "2025-01-01", 366},
		{"a common year is 365 days", "2025-01-01", "2026-01-01", 365},
		{"a year of job work from a leap day", "2024-02-29", "2025-03-01", 366},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DaysBetween(MustDate(c.a), MustDate(c.b)); got != c.want {
				t.Errorf("DaysBetween(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestAddDays(t *testing.T) {
	cases := []struct{ from, want string }{
		{"2024-02-28", "2024-02-29"},
		{"2025-02-28", "2025-03-01"},
		{"2024-12-31", "2025-01-01"},
	}
	for _, c := range cases {
		if got := MustDate(c.from).AddDays(1); got.String() != c.want {
			t.Errorf("%s plus a day = %s, want %s", c.from, got, c.want)
		}
	}
}

func TestDateOrdering(t *testing.T) {
	early, late := MustDate("2025-01-01"), MustDate("2025-12-31")
	if !early.Before(late) || late.Before(early) {
		t.Error("Before does not order the two dates")
	}
	if !late.After(early) || early.After(late) {
		t.Error("After does not order the two dates")
	}
	if !early.Equal(MustDate("2025-01-01")) {
		t.Error("Equal says two identical dates differ")
	}
	if !early.OnOrBefore(early) || !early.OnOrBefore(late) || late.OnOrBefore(early) {
		t.Error("OnOrBefore is wrong at or around equality")
	}
}

func TestNewDateNormalises(t *testing.T) {
	cases := []struct {
		name string
		y    int
		m    time.Month
		d    int
		want string
	}{
		{"an ordinary date", 2025, time.July, 2, "2025-07-02"},
		{"30 February rolls over", 2025, time.February, 30, "2025-03-02"},
		{"month thirteen is next January", 2024, time.Month(13), 1, "2025-01-01"},
		{"month sixteen is next April", 2024, time.Month(16), 1, "2025-04-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NewDate(c.y, c.m, c.d); got.String() != c.want {
				t.Errorf("NewDate(%d, %d, %d) = %s, want %s", c.y, c.m, c.d, got, c.want)
			}
		})
	}
}

func TestDateJSON(t *testing.T) {
	type holder struct {
		D Date `json:"d"`
	}

	t.Run("round trip", func(t *testing.T) {
		b, err := json.Marshal(holder{D: MustDate("2024-02-29")})
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != `{"d":"2024-02-29"}` {
			t.Fatalf("marshalled to %s", b)
		}
		var back holder
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if back.D.String() != "2024-02-29" {
			t.Errorf("round trip gave %s", back.D)
		}
	})

	t.Run("the zero date marshals as null", func(t *testing.T) {
		b, err := json.Marshal(holder{})
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != `{"d":null}` {
			t.Errorf("marshalled to %s, want null", b)
		}
	})

	t.Run("null and empty unmarshal to the zero date", func(t *testing.T) {
		for _, in := range []string{`{"d":null}`, `{"d":""}`} {
			var h holder
			if err := json.Unmarshal([]byte(in), &h); err != nil {
				t.Fatalf("%s: %v", in, err)
			}
			if !h.D.IsZero() {
				t.Errorf("%s gave %s, want the zero date", in, h.D)
			}
		}
	})

	t.Run("a bad date is rejected", func(t *testing.T) {
		var h holder
		if err := json.Unmarshal([]byte(`{"d":"2025-02-30"}`), &h); err == nil {
			t.Error("unmarshalling 2025-02-30 succeeded, want an error")
		}
		if err := json.Unmarshal([]byte(`{"d":7}`), &h); err == nil {
			t.Error("unmarshalling a number succeeded, want an error")
		}
	})
}
