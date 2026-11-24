package jobwork

import (
	"testing"
)

func TestDeadline(t *testing.T) {
	cases := []struct {
		name      string
		sent      string
		goodsType GoodsType
		want      string
	}{
		{"inputs get one year", "2024-10-05", GoodsInputs, "2025-10-05"},
		{"capital goods get three years", "2024-10-05", GoodsCapital, "2027-10-05"},
		{"inputs sent on a leap day fall due on 1 March", "2024-02-29", GoodsInputs, "2025-03-01"},
		{"capital goods sent on a leap day fall due on 1 March", "2024-02-29", GoodsCapital, "2027-03-01"},
		{"inputs sent on 28 February into a leap year", "2023-02-28", GoodsInputs, "2024-02-28"},
		{"capital goods sent on 28 February into a leap year", "2021-02-28", GoodsCapital, "2024-02-28"},
		{"inputs sent on 1 March after a leap day", "2024-03-01", GoodsInputs, "2025-03-01"},
		{"inputs sent on 31 December", "2024-12-31", GoodsInputs, "2025-12-31"},
		{"an unknown goods type is treated as inputs", "2024-10-05", "", "2025-10-05"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Deadline(MustDate(c.sent), c.goodsType); got.String() != c.want {
				t.Errorf("Deadline(%s, %s) = %s, want %s", c.sent, c.goodsType, got, c.want)
			}
		})
	}

	t.Run("a challan with no date has no deadline", func(t *testing.T) {
		if got := Deadline(Date{}, GoodsInputs); !got.IsZero() {
			t.Errorf("Deadline(zero date) = %s, want the zero date", got)
		}
	})

	t.Run("the return periods can be overridden", func(t *testing.T) {
		r := Rules{InputsReturnYears: 2, CapitalGoodsReturnYears: 5}.Normalise()
		if got := r.Deadline(MustDate("2024-10-05"), GoodsInputs); got.String() != "2026-10-05" {
			t.Errorf("inputs with a two-year period = %s, want 2026-10-05", got)
		}
		if got := r.Deadline(MustDate("2024-10-05"), GoodsCapital); got.String() != "2029-10-05" {
			t.Errorf("capital goods with a five-year period = %s, want 2029-10-05", got)
		}
	})

	t.Run("zero settings fall back to the statutory periods", func(t *testing.T) {
		r := Rules{}.Normalise()
		if r.InputsReturnYears != InputsReturnYears || r.CapitalGoodsReturnYears != CapitalGoodsReturnYears {
			t.Errorf("Normalise gave %+v, want the statutory periods", r)
		}
		if r.TurnoverThresholdINR != HalfYearlyFilingTurnoverThresholdINR || r.FilingDueDayOfMonth != FilingDueDayOfMonth {
			t.Errorf("Normalise gave %+v, want the statutory figures", r)
		}
	})

	t.Run("the statutory periods are one and three years", func(t *testing.T) {
		if InputsReturnYears != 1 {
			t.Errorf("InputsReturnYears = %d, want 1", InputsReturnYears)
		}
		if CapitalGoodsReturnYears != 3 {
			t.Errorf("CapitalGoodsReturnYears = %d, want 3", CapitalGoodsReturnYears)
		}
	})
}

func TestOutstanding(t *testing.T) {
	cases := []struct {
		name            string
		sentQty         float64
		returns         []Challan
		wantReturned    float64
		wantOutstanding float64
		wantExcess      float64
		wantOver        bool
		wantCount       int
	}{
		{
			name:            "nothing has come back",
			sentQty:         100,
			returns:         nil,
			wantOutstanding: 100,
		},
		{
			name:            "one full return settles the challan",
			sentQty:         100,
			returns:         []Challan{returnChallan("R/001", "2024-11-10", 100)},
			wantReturned:    100,
			wantOutstanding: 0,
			wantCount:       1,
		},
		{
			name:            "a partial return",
			sentQty:         100,
			returns:         []Challan{returnChallan("R/001", "2024-11-10", 60)},
			wantReturned:    60,
			wantOutstanding: 40,
			wantCount:       1,
		},
		{
			name:    "partial returns in instalments",
			sentQty: 100,
			returns: []Challan{
				returnChallan("R/001", "2024-11-10", 40),
				returnChallan("R/002", "2024-11-20", 25),
				returnChallan("R/003", "2024-12-01", 10),
			},
			wantReturned:    75,
			wantOutstanding: 25,
			wantCount:       3,
		},
		{
			name:    "instalments that only add up exactly in decimal",
			sentQty: 1,
			returns: []Challan{
				returnChallan("R/001", "2024-11-10", 0.1),
				returnChallan("R/002", "2024-11-20", 0.2),
				returnChallan("R/003", "2024-12-01", 0.7),
			},
			wantReturned:    1,
			wantOutstanding: 0,
			wantCount:       3,
		},
		{
			name:            "more has come back than went out",
			sentQty:         100,
			returns:         []Challan{returnChallan("R/001", "2024-11-10", 120)},
			wantReturned:    120,
			wantOutstanding: 0,
			wantExcess:      20,
			wantOver:        true,
			wantCount:       1,
		},
		{
			name:    "a return against another challan is not counted",
			sentQty: 100,
			returns: func() []Challan {
				other := returnChallan("R/001", "2024-11-10", 60)
				other.AgainstChallanNo = "C/002"
				return []Challan{other}
			}(),
			wantOutstanding: 100,
		},
		{
			name:    "a challan for goods sent out is not counted as a return",
			sentQty: 100,
			returns: func() []Challan {
				notAReturn := returnChallan("C/002", "2024-11-10", 60)
				notAReturn.Direction = DirectionSent
				return []Challan{notAReturn}
			}(),
			wantOutstanding: 100,
		},
		{
			name:    "the challan number is matched whatever the case and spacing",
			sentQty: 100,
			returns: func() []Challan {
				messy := returnChallan("R/001", "2024-11-10", 60)
				messy.AgainstChallanNo = " c / 001 "
				return []Challan{messy}
			}(),
			wantReturned:    60,
			wantOutstanding: 40,
			wantCount:       1,
		},
		{
			name:            "a fractional quantity",
			sentQty:         12.5,
			returns:         []Challan{returnChallan("R/001", "2024-11-10", 4.25)},
			wantReturned:    4.25,
			wantOutstanding: 8.25,
			wantCount:       1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sent := sentChallan()
			sent.Qty = c.sentQty
			got := Outstanding(sent, c.returns)

			if got.SentQty != c.sentQty {
				t.Errorf("SentQty = %v, want %v", got.SentQty, c.sentQty)
			}
			if got.ReturnedQty != c.wantReturned {
				t.Errorf("ReturnedQty = %v, want %v", got.ReturnedQty, c.wantReturned)
			}
			if got.OutstandingQty != c.wantOutstanding {
				t.Errorf("OutstandingQty = %v, want %v", got.OutstandingQty, c.wantOutstanding)
			}
			if got.ExcessQty != c.wantExcess {
				t.Errorf("ExcessQty = %v, want %v", got.ExcessQty, c.wantExcess)
			}
			if got.OverReturned != c.wantOver {
				t.Errorf("OverReturned = %v, want %v", got.OverReturned, c.wantOver)
			}
			if got.ReturnCount != c.wantCount {
				t.Errorf("ReturnCount = %d, want %d", got.ReturnCount, c.wantCount)
			}
			if got.OutstandingQty < 0 {
				t.Errorf("OutstandingQty = %v, which must never be negative", got.OutstandingQty)
			}
		})
	}
}

func TestDeemedSupplyValue(t *testing.T) {
	cases := []struct {
		name                          string
		taxableValue, sentQty, outQty float64
		want                          float64
	}{
		{"nothing came back, so the whole value is at risk", 50000, 100, 100, 50000},
		{"pro-rated to the part still out", 50000, 100, 40, 20000},
		{"a third of the value, rounded to paise", 100, 3, 1, 33.33},
		{"two thirds of the value, rounded to paise", 100, 3, 2, 66.67},
		{"nothing outstanding carries no value", 50000, 100, 0, 0},
		{"a challan with no quantity carries no value", 50000, 0, 10, 0},
		{"a challan with no value carries no value", 0, 100, 40, 0},
		{"an outstanding quantity above the quantity sent is capped", 50000, 100, 150, 50000},
		{"a negative outstanding quantity carries no value", 50000, 100, -5, 0},
		{"paise are kept", 1234.56, 2, 1, 617.28},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DeemedSupplyValue(c.taxableValue, c.sentQty, c.outQty); got != c.want {
				t.Errorf("DeemedSupplyValue(%v, %v, %v) = %v, want %v",
					c.taxableValue, c.sentQty, c.outQty, got, c.want)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	cases := []struct {
		name            string
		goodsType       GoodsType
		returns         []Challan
		asOf            string
		wantState       State
		wantDeadline    string
		wantDaysLeft    int
		wantDaysOverdue int
		wantDeemed      float64
	}{
		{
			name:         "nothing back, well within time",
			goodsType:    GoodsInputs,
			asOf:         "2024-11-01",
			wantState:    StatePending,
			wantDeadline: "2025-10-05",
			wantDaysLeft: 338,
			wantDeemed:   50000,
		},
		{
			name:         "part back, still within time",
			goodsType:    GoodsInputs,
			returns:      []Challan{returnChallan("R/001", "2024-11-10", 60)},
			asOf:         "2025-10-04",
			wantState:    StatePending,
			wantDeadline: "2025-10-05",
			wantDaysLeft: 1,
			wantDeemed:   20000,
		},
		{
			name:         "on the deadline itself it is still within time",
			goodsType:    GoodsInputs,
			asOf:         "2025-10-05",
			wantState:    StatePending,
			wantDeadline: "2025-10-05",
			wantDaysLeft: 0,
			wantDeemed:   50000,
		},
		{
			name:            "the day after the deadline it is a deemed supply",
			goodsType:       GoodsInputs,
			asOf:            "2025-10-06",
			wantState:       StateOverdue,
			wantDeadline:    "2025-10-05",
			wantDaysOverdue: 1,
			wantDeemed:      50000,
		},
		{
			name:            "only the part still out is a deemed supply",
			goodsType:       GoodsInputs,
			returns:         []Challan{returnChallan("R/001", "2024-11-10", 60)},
			asOf:            "2025-12-05",
			wantState:       StateOverdue,
			wantDeadline:    "2025-10-05",
			wantDaysOverdue: 61,
			wantDeemed:      20000,
		},
		{
			name:         "everything back, so nothing is at risk",
			goodsType:    GoodsInputs,
			returns:      []Challan{returnChallan("R/001", "2024-11-10", 100)},
			asOf:         "2030-01-01",
			wantState:    StateReturned,
			wantDeadline: "2025-10-05",
			wantDeemed:   0,
		},
		{
			name:         "everything back in instalments",
			goodsType:    GoodsInputs,
			returns:      []Challan{returnChallan("R/001", "2024-11-10", 55), returnChallan("R/002", "2024-12-10", 45)},
			asOf:         "2030-01-01",
			wantState:    StateReturned,
			wantDeadline: "2025-10-05",
		},
		{
			name:         "more back than went out still counts as returned",
			goodsType:    GoodsInputs,
			returns:      []Challan{returnChallan("R/001", "2024-11-10", 101)},
			asOf:         "2030-01-01",
			wantState:    StateReturned,
			wantDeadline: "2025-10-05",
		},
		{
			name:         "capital goods have three years",
			goodsType:    GoodsCapital,
			asOf:         "2025-10-06",
			wantState:    StatePending,
			wantDeadline: "2027-10-05",
			wantDaysLeft: 729,
			wantDeemed:   50000,
		},
		{
			name:            "capital goods after three years",
			goodsType:       GoodsCapital,
			asOf:            "2027-10-06",
			wantState:       StateOverdue,
			wantDeadline:    "2027-10-05",
			wantDaysOverdue: 1,
			wantDeemed:      50000,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sent := sentChallan()
			sent.GoodsType = c.goodsType
			got := Status(sent, c.returns, MustDate(c.asOf))

			if got.State != c.wantState {
				t.Errorf("State = %q, want %q", got.State, c.wantState)
			}
			if got.Deadline.String() != c.wantDeadline {
				t.Errorf("Deadline = %s, want %s", got.Deadline, c.wantDeadline)
			}
			if got.DaysLeft != c.wantDaysLeft {
				t.Errorf("DaysLeft = %d, want %d", got.DaysLeft, c.wantDaysLeft)
			}
			if got.DaysOverdue != c.wantDaysOverdue {
				t.Errorf("DaysOverdue = %d, want %d", got.DaysOverdue, c.wantDaysOverdue)
			}
			if got.DeemedSupplyValue != c.wantDeemed {
				t.Errorf("DeemedSupplyValue = %v, want %v", got.DeemedSupplyValue, c.wantDeemed)
			}
			if got.ChallanNo != "C/001" || got.JobWorkerGSTIN != "29ABCDE1234F1Z5" || got.UQC != "KGS" {
				t.Errorf("the challan's own details did not carry through: %+v", got)
			}
			if got.AsOf.String() != c.asOf {
				t.Errorf("AsOf = %s, want %s", got.AsOf, c.asOf)
			}
		})
	}

	t.Run("a challan sent on a leap day falls due on 1 March", func(t *testing.T) {
		sent := sentChallan()
		sent.Date = MustDate("2024-02-29")
		if got := Status(sent, nil, MustDate("2025-03-01")); got.State != StatePending || got.DaysLeft != 0 {
			t.Errorf("on 2025-03-01 the challan is %q with %d days left, want Pending with 0", got.State, got.DaysLeft)
		}
		if got := Status(sent, nil, MustDate("2025-03-02")); got.State != StateOverdue || got.DaysOverdue != 1 {
			t.Errorf("on 2025-03-02 the challan is %q, %d days overdue, want Overdue by 1", got.State, got.DaysOverdue)
		}
	})
}

// register is the worked example the roll-up tests share: two job workers, one
// challan from an earlier year that has gone overdue, partial returns, and a
// return whose challan number matches nothing.
func register() []Challan {
	c1 := sentChallan() // C/001, 2024-10-05, inputs, 100 KGS, ₹50,000, worker A
	c2 := Challan{
		No:             "C/002",
		Date:           MustDate("2024-11-20"),
		JobWorkerGSTIN: "29ABCDE1234F1Z5",
		JobWorkerName:  "Shree Plating Works",
		ItemDesc:       "Press tool",
		HSN:            "8207",
		Qty:            2,
		UQC:            "NOS",
		TaxableValue:   400000,
		GoodsType:      GoodsCapital,
		Direction:      DirectionSent,
	}
	c3 := Challan{
		No:             "C/003",
		Date:           MustDate("2023-06-01"),
		JobWorkerGSTIN: "27PQRSX5678K1ZV",
		JobWorkerName:  "Mahesh Fabricators",
		ItemDesc:       "Steel pipe for bending",
		HSN:            "7306",
		Qty:            500,
		UQC:            "MTR",
		TaxableValue:   250000,
		GoodsType:      GoodsInputs,
		Direction:      DirectionSent,
	}

	r1 := returnChallan("R/001", "2024-11-10", 60) // 60 of C/001
	r2 := Challan{
		No:               "R/002",
		Date:             MustDate("2024-12-15"),
		JobWorkerGSTIN:   "27PQRSX5678K1ZV",
		JobWorkerName:    "Mahesh Fabricators",
		ItemDesc:         "Steel pipe, bent",
		HSN:              "7306",
		Qty:              200,
		UQC:              "MTR",
		GoodsType:        GoodsInputs,
		Direction:        DirectionReturned,
		AgainstChallanNo: "C/003",
	}
	r3 := returnChallan("R/003", "2024-12-20", 5)
	r3.AgainstChallanNo = "C/999" // a mis-keyed challan number

	return []Challan{c1, c2, c3, r1, r2, r3}
}

func TestAssessAndReview(t *testing.T) {
	asOf := MustDate("2025-01-15")
	d := Review(register(), asOf, 30)

	t.Run("totals", func(t *testing.T) {
		want := Totals{
			Challans:         3,
			Returned:         0,
			Pending:          2,
			Overdue:          1,
			QtySent:          602,
			QtyReturned:      260,
			QtyOutstanding:   342,
			TaxableValueSent: 700000,
			ValueOutstanding: 570000, // 20,000 + 400,000 + 150,000
			ValueOverdue:     150000, // 300 of 500 metres at ₹250,000
			JobWorkers:       2,
			MixedUnits:       true,
		}
		if d.Totals != want {
			t.Errorf("Totals =\n%+v\nwant\n%+v", d.Totals, want)
		}
	})

	t.Run("the overdue challan comes first", func(t *testing.T) {
		if len(d.AtRisk) != 1 {
			t.Fatalf("AtRisk has %d challans, want only the overdue one: %+v", len(d.AtRisk), d.AtRisk)
		}
		got := d.AtRisk[0]
		if got.ChallanNo != "C/003" || got.State != StateOverdue {
			t.Fatalf("AtRisk[0] = %s (%s), want C/003 Overdue", got.ChallanNo, got.State)
		}
		if got.DaysOverdue != 228 {
			t.Errorf("C/003 is %d days overdue, want 228", got.DaysOverdue)
		}
		if got.OutstandingQty != 300 || got.DeemedSupplyValue != 150000 {
			t.Errorf("C/003 has %v outstanding worth %v, want 300 and 150000",
				got.OutstandingQty, got.DeemedSupplyValue)
		}
	})

	t.Run("per job worker", func(t *testing.T) {
		if len(d.Workers) != 2 {
			t.Fatalf("Workers = %d, want 2", len(d.Workers))
		}
		// The worker with an overdue challan is listed first.
		first, second := d.Workers[0], d.Workers[1]
		if first.JobWorkerGSTIN != "27PQRSX5678K1ZV" {
			t.Errorf("first worker = %s, want the one with the overdue challan", first.JobWorkerGSTIN)
		}
		if first.OverdueChallans != 1 || first.ValueOverdue != 150000 || first.QtyOutstanding != 300 {
			t.Errorf("Mahesh Fabricators = %+v", first)
		}
		if first.EarliestDeadline.String() != "2024-06-01" {
			t.Errorf("earliest deadline = %s, want 2024-06-01", first.EarliestDeadline)
		}
		if len(first.UQCs) != 1 || first.UQCs[0] != "MTR" {
			t.Errorf("units = %v, want only MTR", first.UQCs)
		}

		if second.Challans != 2 || second.OpenChallans != 2 || second.OverdueChallans != 0 {
			t.Errorf("Shree Plating Works = %+v", second)
		}
		if second.QtyOutstanding != 42 || second.ValueOutstanding != 420000 {
			t.Errorf("Shree Plating Works outstanding = %v qty, %v value, want 42 and 420000",
				second.QtyOutstanding, second.ValueOutstanding)
		}
		if len(second.UQCs) != 2 || second.UQCs[0] != "KGS" || second.UQCs[1] != "NOS" {
			t.Errorf("units = %v, want KGS and NOS", second.UQCs)
		}
	})

	t.Run("the mis-keyed return is flagged", func(t *testing.T) {
		if len(d.Orphans) != 1 || d.Orphans[0].No != "R/003" {
			t.Errorf("Orphans = %+v, want only R/003", d.Orphans)
		}
	})

	t.Run("a wider horizon pulls in the challans falling due", func(t *testing.T) {
		wide := Review(register(), asOf, 400)
		if len(wide.AtRisk) != 2 {
			t.Fatalf("AtRisk over 400 days = %d challans, want 2", len(wide.AtRisk))
		}
		if wide.AtRisk[0].ChallanNo != "C/003" || wide.AtRisk[1].ChallanNo != "C/001" {
			t.Errorf("AtRisk order = %s, %s; want C/003 then C/001",
				wide.AtRisk[0].ChallanNo, wide.AtRisk[1].ChallanNo)
		}
	})

	t.Run("an empty register", func(t *testing.T) {
		empty := Review(nil, asOf, 30)
		if empty.Totals != (Totals{}) {
			t.Errorf("Totals = %+v, want everything at zero", empty.Totals)
		}
		if len(empty.Workers) != 0 || len(empty.AtRisk) != 0 {
			t.Errorf("an empty register produced %d workers and %d at-risk challans",
				len(empty.Workers), len(empty.AtRisk))
		}
	})
}

func TestSortByUrgency(t *testing.T) {
	list := []Assessment{
		{ChallanNo: "B", State: StateReturned},
		{ChallanNo: "C", State: StatePending, DaysLeft: 200},
		{ChallanNo: "D", State: StatePending, DaysLeft: 5},
		{ChallanNo: "E", State: StateOverdue, DaysOverdue: 10},
		{ChallanNo: "F", State: StateOverdue, DaysOverdue: 400},
		{ChallanNo: "A", State: StateReturned},
	}
	SortByUrgency(list)

	want := []string{"F", "E", "D", "C", "A", "B"}
	for i, no := range want {
		if list[i].ChallanNo != no {
			got := make([]string, len(list))
			for j, a := range list {
				got[j] = a.ChallanNo
			}
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestAtRiskHorizon(t *testing.T) {
	list := []Assessment{
		{ChallanNo: "overdue", State: StateOverdue, DaysOverdue: 3},
		{ChallanNo: "due in 10", State: StatePending, DaysLeft: 10},
		{ChallanNo: "due in 45", State: StatePending, DaysLeft: 45},
		{ChallanNo: "settled", State: StateReturned},
	}
	cases := []struct {
		horizon int
		want    []string
	}{
		{0, []string{"overdue"}},
		{10, []string{"overdue", "due in 10"}},
		{45, []string{"overdue", "due in 10", "due in 45"}},
	}
	for _, c := range cases {
		got := AtRisk(list, c.horizon)
		if len(got) != len(c.want) {
			t.Fatalf("horizon %d gave %d challans, want %d", c.horizon, len(got), len(c.want))
		}
		for i, no := range c.want {
			if got[i].ChallanNo != no {
				t.Errorf("horizon %d: position %d = %q, want %q", c.horizon, i, got[i].ChallanNo, no)
			}
		}
	}
}
