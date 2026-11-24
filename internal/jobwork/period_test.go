package jobwork

import "testing"

func TestQuarter(t *testing.T) {
	cases := []struct {
		fyStart         int
		q               int
		from, to, label string
	}{
		{2024, 1, "2024-04-01", "2024-06-30", "Q1 FY2024-25 (Apr-Jun 2024)"},
		{2024, 2, "2024-07-01", "2024-09-30", "Q2 FY2024-25 (Jul-Sep 2024)"},
		{2024, 3, "2024-10-01", "2024-12-31", "Q3 FY2024-25 (Oct-Dec 2024)"},
		{2024, 4, "2025-01-01", "2025-03-31", "Q4 FY2024-25 (Jan-Mar 2025)"},
		{2023, 4, "2024-01-01", "2024-03-31", "Q4 FY2023-24 (Jan-Mar 2024)"}, // ends in a leap year
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			p, err := Quarter(c.fyStart, c.q)
			if err != nil {
				t.Fatal(err)
			}
			if p.From.String() != c.from || p.To.String() != c.to {
				t.Errorf("Quarter(%d, %d) = %s..%s, want %s..%s", c.fyStart, c.q, p.From, p.To, c.from, c.to)
			}
			if p.Label != c.label {
				t.Errorf("label = %q, want %q", p.Label, c.label)
			}
			if p.Frequency != FreqQuarterly {
				t.Errorf("frequency = %q, want %q", p.Frequency, FreqQuarterly)
			}
		})
	}

	for _, bad := range []int{0, 5, -1} {
		if _, err := Quarter(2024, bad); err == nil {
			t.Errorf("Quarter(2024, %d) succeeded, want an error", bad)
		}
	}
}

func TestHalfYear(t *testing.T) {
	cases := []struct {
		fyStart         int
		h               int
		from, to, label string
	}{
		{2024, 1, "2024-04-01", "2024-09-30", "H1 FY2024-25 (Apr-Sep 2024)"},
		{2024, 2, "2024-10-01", "2025-03-31", "H2 FY2024-25 (Oct 2024-Mar 2025)"},
		{2023, 2, "2023-10-01", "2024-03-31", "H2 FY2023-24 (Oct 2023-Mar 2024)"},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			p, err := HalfYear(c.fyStart, c.h)
			if err != nil {
				t.Fatal(err)
			}
			if p.From.String() != c.from || p.To.String() != c.to {
				t.Errorf("HalfYear(%d, %d) = %s..%s, want %s..%s", c.fyStart, c.h, p.From, p.To, c.from, c.to)
			}
			if p.Label != c.label {
				t.Errorf("label = %q, want %q", p.Label, c.label)
			}
		})
	}

	for _, bad := range []int{0, 3} {
		if _, err := HalfYear(2024, bad); err == nil {
			t.Errorf("HalfYear(2024, %d) succeeded, want an error", bad)
		}
	}
}

func TestFinancialYear(t *testing.T) {
	p := FinancialYear(2024)
	if p.From.String() != "2024-04-01" || p.To.String() != "2025-03-31" {
		t.Errorf("FinancialYear(2024) = %s..%s, want 2024-04-01..2025-03-31", p.From, p.To)
	}
	if p.Label != "FY2024-25 (Apr 2024-Mar 2025)" {
		t.Errorf("label = %q", p.Label)
	}
	if p.Frequency != FreqAnnual {
		t.Errorf("frequency = %q, want %q", p.Frequency, FreqAnnual)
	}

	leap := FinancialYear(2023)
	if leap.To.String() != "2024-03-31" {
		t.Errorf("FinancialYear(2023) ends %s, want 2024-03-31", leap.To)
	}
}

func TestFinancialYearStartAndLabel(t *testing.T) {
	cases := []struct {
		date  string
		want  int
		label string
	}{
		{"2025-04-01", 2025, "FY2025-26"},
		{"2025-03-31", 2024, "FY2024-25"},
		{"2025-01-15", 2024, "FY2024-25"},
		{"2024-12-31", 2024, "FY2024-25"},
		{"2024-04-01", 2024, "FY2024-25"},
		{"2024-03-01", 2023, "FY2023-24"},
	}
	for _, c := range cases {
		t.Run(c.date, func(t *testing.T) {
			got := FinancialYearStart(MustDate(c.date))
			if got != c.want {
				t.Errorf("FinancialYearStart(%s) = %d, want %d", c.date, got, c.want)
			}
			if label := FinancialYearLabel(got); label != c.label {
				t.Errorf("FinancialYearLabel(%d) = %q, want %q", got, label, c.label)
			}
		})
	}
}

func TestDueDate(t *testing.T) {
	cases := []struct {
		name string
		make func() Period
		want string
	}{
		{"quarter ending in June", func() Period { p, _ := Quarter(2024, 1); return p }, "2024-07-25"},
		{"quarter ending in December rolls into the next year", func() Period { p, _ := Quarter(2024, 3); return p }, "2025-01-25"},
		{"quarter ending in March", func() Period { p, _ := Quarter(2024, 4); return p }, "2025-04-25"},
		{"half year ending in September", func() Period { p, _ := HalfYear(2024, 1); return p }, "2024-10-25"},
		{"half year ending in March", func() Period { p, _ := HalfYear(2024, 2); return p }, "2025-04-25"},
		{"financial year", func() Period { return FinancialYear(2024) }, "2025-04-25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DueDate(c.make()); got.String() != c.want {
				t.Errorf("DueDate = %s, want %s", got, c.want)
			}
		})
	}

	t.Run("a period with no end has no due date", func(t *testing.T) {
		if got := DueDate(Period{}); !got.IsZero() {
			t.Errorf("DueDate = %s, want the zero date", got)
		}
	})

	t.Run("the due day can be overridden", func(t *testing.T) {
		p, _ := Quarter(2024, 1)
		r := Rules{FilingDueDayOfMonth: 20}
		if got := r.DueDate(p); got.String() != "2024-07-20" {
			t.Errorf("DueDate with a 20th due day = %s, want 2024-07-20", got)
		}
	})
}

func TestFilingFrequency(t *testing.T) {
	cases := []struct {
		name     string
		turnover float64
		want     Frequency
	}{
		{"well below the threshold", 1_000_000, FreqAnnual},
		{"just below the threshold", 49_999_999, FreqAnnual},
		{"exactly at the threshold", 50_000_000, FreqAnnual},
		{"a rupee above the threshold", 50_000_001, FreqHalfYearly},
		{"well above the threshold", 250_000_000, FreqHalfYearly},
		{"not stated, so the shorter period is assumed", 0, FreqHalfYearly},
		{"a negative figure is treated as not stated", -1, FreqHalfYearly},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FilingFrequency(c.turnover); got != c.want {
				t.Errorf("FilingFrequency(%v) = %q, want %q", c.turnover, got, c.want)
			}
		})
	}

	t.Run("the threshold is five crore rupees", func(t *testing.T) {
		if HalfYearlyFilingTurnoverThresholdINR != 50000000 {
			t.Errorf("threshold = %d, want 50000000", HalfYearlyFilingTurnoverThresholdINR)
		}
		if FilingDueDayOfMonth != 25 {
			t.Errorf("due day = %d, want 25", FilingDueDayOfMonth)
		}
	})

	t.Run("the threshold can be overridden", func(t *testing.T) {
		r := Rules{TurnoverThresholdINR: 10_000_000}
		if got := r.FilingFrequency(20_000_000); got != FreqHalfYearly {
			t.Errorf("above an overridden threshold = %q, want half-yearly", got)
		}
		if got := r.FilingFrequency(5_000_000); got != FreqAnnual {
			t.Errorf("below an overridden threshold = %q, want annual", got)
		}
	})
}

func TestPeriodContains(t *testing.T) {
	p, _ := Quarter(2024, 3) // 1 October to 31 December 2024
	cases := []struct {
		date string
		want bool
	}{
		{"2024-09-30", false},
		{"2024-10-01", true},
		{"2024-11-15", true},
		{"2024-12-31", true},
		{"2025-01-01", false},
	}
	for _, c := range cases {
		if got := p.Contains(MustDate(c.date)); got != c.want {
			t.Errorf("%s contains %s = %v, want %v", p.Label, c.date, got, c.want)
		}
	}
	if p.Contains(Date{}) {
		t.Error("a period should not contain the zero date")
	}
}

func TestPeriodForAndPeriodsIn(t *testing.T) {
	if _, err := PeriodFor("monthly", 2024, 1); err == nil {
		t.Error("PeriodFor with an unknown frequency succeeded, want an error")
	}
	counts := map[Frequency]int{FreqQuarterly: 4, FreqHalfYearly: 2, FreqAnnual: 1}
	for freq, want := range counts {
		if got := len(PeriodsIn(freq, 2024)); got != want {
			t.Errorf("PeriodsIn(%q) gave %d periods, want %d", freq, got, want)
		}
	}
	p, err := PeriodFor(FreqHalfYearly, 2024, 2)
	if err != nil || p.From.String() != "2024-10-01" {
		t.Errorf("PeriodFor(half-yearly, 2024, 2) = %+v, %v", p, err)
	}
}

func TestCustomPeriod(t *testing.T) {
	p, err := CustomPeriod(MustDate("2024-10-01"), MustDate("2024-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Label != "2024-10-01 to 2024-10-31" || p.Frequency != FreqCustom {
		t.Errorf("CustomPeriod = %+v", p)
	}
	if p.FinancialYear != "FY2024-25" {
		t.Errorf("financial year = %q, want FY2024-25", p.FinancialYear)
	}
	if _, err := CustomPeriod(MustDate("2024-10-31"), MustDate("2024-10-01")); err == nil {
		t.Error("a period ending before it starts succeeded, want an error")
	}
	if _, err := CustomPeriod(Date{}, MustDate("2024-10-01")); err == nil {
		t.Error("a period with no start date succeeded, want an error")
	}
}

func TestFiling(t *testing.T) {
	q3, err := Quarter(2024, 3) // 1 October to 31 December 2024
	if err != nil {
		t.Fatal(err)
	}
	asOf := MustDate("2025-01-15")
	f := PrepareFiling(register(), q3, asOf, 0)

	t.Run("the period and its due date", func(t *testing.T) {
		if f.Period.Label != "Q3 FY2024-25 (Oct-Dec 2024)" {
			t.Errorf("period = %q", f.Period.Label)
		}
		if f.DueDate.String() != "2025-01-25" {
			t.Errorf("due date = %s, want 2025-01-25", f.DueDate)
		}
		if f.AsOf.String() != "2025-01-15" {
			t.Errorf("asOf = %s", f.AsOf)
		}
		if f.SuggestedFrequency != FreqHalfYearly {
			t.Errorf("suggested frequency with no turnover stated = %q, want half-yearly", f.SuggestedFrequency)
		}
		if f.TurnoverThreshold != HalfYearlyFilingTurnoverThresholdINR {
			t.Errorf("threshold = %v", f.TurnoverThreshold)
		}
	})

	t.Run("only challans sent out in the period are reported", func(t *testing.T) {
		want := Totals{
			Challans:         2,
			Pending:          2,
			QtySent:          102,
			QtyReturned:      60,
			QtyOutstanding:   42,
			TaxableValueSent: 450000,
			ValueOutstanding: 420000,
			JobWorkers:       1,
			MixedUnits:       true,
		}
		if f.Totals != want {
			t.Errorf("Totals =\n%+v\nwant\n%+v", f.Totals, want)
		}
	})

	t.Run("grouped by job worker in GSTIN order", func(t *testing.T) {
		if len(f.Workers) != 2 {
			t.Fatalf("Workers = %d, want 2", len(f.Workers))
		}
		first, second := f.Workers[0], f.Workers[1]
		if first.JobWorkerGSTIN != "27PQRSX5678K1ZV" || second.JobWorkerGSTIN != "29ABCDE1234F1Z5" {
			t.Fatalf("order = %s, %s", first.JobWorkerGSTIN, second.JobWorkerGSTIN)
		}

		// Mahesh Fabricators sent nothing out this quarter but sent goods back.
		if first.ChallansSent != 0 || len(first.Lines) != 0 {
			t.Errorf("Mahesh Fabricators = %+v, want no challans sent in the period", first)
		}
		if first.ReturnsInPeriod != 1 || first.QtyReturnedInPeriod != 200 {
			t.Errorf("Mahesh Fabricators returns = %d for %v, want 1 for 200",
				first.ReturnsInPeriod, first.QtyReturnedInPeriod)
		}
		if len(first.Returns) != 1 || !first.Returns[0].Matched || first.Returns[0].AgainstDate.String() != "2023-06-01" {
			t.Errorf("the return should settle against C/003 of 2023-06-01: %+v", first.Returns)
		}

		if second.ChallansSent != 2 || second.QtySent != 102 || second.TaxableValueSent != 450000 {
			t.Errorf("Shree Plating Works = %+v", second)
		}
		if second.QtyOutstanding != 42 || second.OverdueChallans != 0 || second.DeemedSupplyValue != 0 {
			t.Errorf("Shree Plating Works position = %+v", second)
		}
		if second.ReturnsInPeriod != 2 || second.QtyReturnedInPeriod != 65 {
			t.Errorf("Shree Plating Works returns = %d for %v, want 2 for 65",
				second.ReturnsInPeriod, second.QtyReturnedInPeriod)
		}
		if len(second.Lines) != 2 || second.Lines[0].ChallanNo != "C/001" || second.Lines[1].ChallanNo != "C/002" {
			t.Errorf("lines should be in challan-date order: %+v", second.Lines)
		}
		// The mis-keyed return is still listed, but marked as settling nothing.
		for _, r := range second.Returns {
			if r.ChallanNo == "R/003" && r.Matched {
				t.Error("R/003 names a challan that does not exist and should not be marked as matched")
			}
		}
	})

	t.Run("goods received back in the period", func(t *testing.T) {
		if f.ReturnsInPeriod != 3 || f.QtyReturnedInPrd != 265 {
			t.Errorf("returns in the period = %d for %v, want 3 for 265", f.ReturnsInPeriod, f.QtyReturnedInPrd)
		}
		if len(f.OrphanReturns) != 1 || f.OrphanReturns[0].No != "R/003" {
			t.Errorf("orphan returns = %+v, want only R/003", f.OrphanReturns)
		}
	})

	t.Run("a period holding the overdue challan", func(t *testing.T) {
		fy, err := PeriodFor(FreqAnnual, 2023, 1)
		if err != nil {
			t.Fatal(err)
		}
		g := PrepareFiling(register(), fy, asOf, 0)
		if g.DueDate.String() != "2024-04-25" {
			t.Errorf("due date = %s, want 2024-04-25", g.DueDate)
		}
		if g.Totals.Challans != 1 || g.Totals.Overdue != 1 {
			t.Fatalf("Totals = %+v, want the one overdue challan", g.Totals)
		}
		if g.Totals.ValueOverdue != 150000 {
			t.Errorf("value overdue = %v, want 150000", g.Totals.ValueOverdue)
		}
		if len(g.Workers) != 1 || g.Workers[0].OverdueChallans != 1 || g.Workers[0].DeemedSupplyValue != 150000 {
			t.Errorf("worker roll-up = %+v", g.Workers)
		}
		if g.ReturnsInPeriod != 0 {
			t.Errorf("returns in FY2023-24 = %d, want 0", g.ReturnsInPeriod)
		}
	})

	t.Run("a period with nothing in it", func(t *testing.T) {
		q1, _ := Quarter(2020, 1)
		empty := PrepareFiling(register(), q1, asOf, 0)
		if empty.Totals.Challans != 0 || len(empty.Workers) != 0 {
			t.Errorf("an empty period gave %+v and %d workers", empty.Totals, len(empty.Workers))
		}
		if empty.DueDate.String() != "2020-07-25" {
			t.Errorf("due date = %s, want 2020-07-25", empty.DueDate)
		}
	})

	t.Run("the turnover decides the suggested frequency", func(t *testing.T) {
		above := PrepareFiling(register(), q3, asOf, 60_000_000)
		if above.SuggestedFrequency != FreqHalfYearly {
			t.Errorf("above the threshold = %q, want half-yearly", above.SuggestedFrequency)
		}
		below := PrepareFiling(register(), q3, asOf, 10_000_000)
		if below.SuggestedFrequency != FreqAnnual {
			t.Errorf("below the threshold = %q, want annual", below.SuggestedFrequency)
		}
		if below.TurnoverINR != 10_000_000 {
			t.Errorf("turnover = %v, want it carried into the filing", below.TurnoverINR)
		}
	})
}
