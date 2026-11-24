package jobwork

import (
	"strings"
	"testing"
)

func TestParseNumber(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"100", 100, false},
		{"12.5", 12.5, false},
		{"1,250.50", 1250.50, false},
		{"1,00,000", 100000, false},
		{"₹ 50,000", 50000, false},
		{"Rs. 500", 500, false},
		{"INR 500", 500, false},
		{" 42 ", 42, false},
		{"", 0, false},
		{"0", 0, false},
		{"-5", -5, false},
		{"abc", 0, true},
		{"12kg", 0, true},
		{"1.2.3", 0, true},
		{"NaN", 0, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseNumber(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseNumber(%q) = %v, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNumber(%q) returned %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("ParseNumber(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestParseDirection(t *testing.T) {
	cases := []struct {
		in, against string
		want        Direction
		wantErr     bool
	}{
		{"sent", "", DirectionSent, false},
		{"Sent", "", DirectionSent, false},
		{"OUT", "", DirectionSent, false},
		{"outward", "", DirectionSent, false},
		{"dispatched", "", DirectionSent, false},
		{"returned", "C/001", DirectionReturned, false},
		{"Return", "C/001", DirectionReturned, false},
		{"received back", "C/001", DirectionReturned, false},
		{"inward", "C/001", DirectionReturned, false},
		{"", "", DirectionSent, false},          // nothing to come back against
		{"", "C/001", DirectionReturned, false}, // inferred from the column
		{"sideways", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.in+"/"+c.against, func(t *testing.T) {
			got, err := ParseDirection(c.in, c.against)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseDirection(%q, %q) = %q, want an error", c.in, c.against, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDirection(%q, %q) returned %v", c.in, c.against, err)
			}
			if got != c.want {
				t.Errorf("ParseDirection(%q, %q) = %q, want %q", c.in, c.against, got, c.want)
			}
		})
	}
}

func TestParseGoodsType(t *testing.T) {
	cases := []struct {
		in      string
		want    GoodsType
		wantErr bool
	}{
		{"inputs", GoodsInputs, false},
		{"Input", GoodsInputs, false},
		{"raw material", GoodsInputs, false},
		{"", GoodsInputs, false},
		{"capital", GoodsCapital, false},
		{"Capital Goods", GoodsCapital, false},
		{"CG", GoodsCapital, false},
		{"tools", "", true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseGoodsType(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseGoodsType(%q) = %q, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGoodsType(%q) returned %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("ParseGoodsType(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseChallanCSV(t *testing.T) {
	t.Run("a file in the export's own shape", func(t *testing.T) {
		rows := append([][]string{ChallanCSVHeader()}, ChallanCSVRows(register())...)
		got, errs := ParseChallanCSV(rows)
		if len(errs) != 0 {
			t.Fatalf("errors = %v, want none", errs)
		}
		if len(got) != 6 {
			t.Fatalf("parsed %d challans, want 6", len(got))
		}
		if got[0].Line != 2 {
			t.Errorf("the first challan came from line %d, want 2", got[0].Line)
		}
		for i, want := range register() {
			want = want.Normalise()
			g := got[i].Challan
			if g.No != want.No || !g.Date.Equal(want.Date) || g.Direction != want.Direction ||
				g.AgainstChallanNo != want.AgainstChallanNo || g.JobWorkerGSTIN != want.JobWorkerGSTIN ||
				g.JobWorkerName != want.JobWorkerName || g.ItemDesc != want.ItemDesc || g.HSN != want.HSN ||
				g.Qty != want.Qty || g.UQC != want.UQC || g.TaxableValue != want.TaxableValue ||
				g.GoodsType != want.GoodsType {
				t.Errorf("row %d round-tripped as\n%+v\nwant\n%+v", i, g, want)
			}
		}
	})

	t.Run("columns in another order, with the headings a register uses", func(t *testing.T) {
		rows := [][]string{
			{"Quantity", "Challan No.", "Challan Date", "GSTIN", "Name of job worker", "Particulars", "Unit", "Taxable Amount", "Type of goods", "HSN Code"},
			{"1,250.5", "DC-77", "05/10/2024", "29abcde1234f1z5", "Shree Plating Works", "MS brackets", "KGS", "₹ 2,50,000", "Capital Goods", "7308"},
		}
		got, errs := ParseChallanCSV(rows)
		if len(errs) != 0 {
			t.Fatalf("errors = %v, want none", errs)
		}
		if len(got) != 1 {
			t.Fatalf("parsed %d challans, want 1", len(got))
		}
		c := got[0].Challan
		if c.No != "DC-77" || c.Date.String() != "2024-10-05" || c.Qty != 1250.5 ||
			c.TaxableValue != 250000 || c.UQC != "KGS" || c.GoodsType != GoodsCapital ||
			c.JobWorkerGSTIN != "29ABCDE1234F1Z5" || c.HSN != "7308" || c.Direction != DirectionSent {
			t.Errorf("parsed as %+v", c)
		}
	})

	t.Run("a return with no direction column", func(t *testing.T) {
		rows := [][]string{
			{"Challan No", "Date", "Against Challan No", "GSTIN", "Job Worker Name", "Description", "Qty", "UQC"},
			{"R/010", "2024-11-10", "C/001", "29ABCDE1234F1Z5", "Shree Plating Works", "MS brackets, plated", "60", "KGS"},
		}
		got, errs := ParseChallanCSV(rows)
		if len(errs) != 0 {
			t.Fatalf("errors = %v, want none", errs)
		}
		if got[0].Challan.Direction != DirectionReturned {
			t.Errorf("direction = %q, want it inferred as a return", got[0].Challan.Direction)
		}
		if got[0].Challan.GoodsType != GoodsInputs {
			t.Errorf("goods type = %q, want inputs by default", got[0].Challan.GoodsType)
		}
	})

	t.Run("blank rows are skipped and bad rows are reported by line", func(t *testing.T) {
		rows := [][]string{
			{"Challan No", "Date", "GSTIN", "Job Worker Name", "Description", "Qty", "UQC", "Taxable Value"},
			{"C/010", "2024-10-05", "29ABCDE1234F1Z5", "Shree Plating Works", "MS brackets", "100", "KGS", "50000"},
			{"", "", "", "", "", "", "", ""},
			{"C/011", "2024-13-45", "29ABCDE1234F1Z5", "Shree Plating Works", "MS brackets", "100", "KGS", "50000"},
			{"C/012", "2024-10-06", "29ABCDE1234F1Z5", "Shree Plating Works", "MS brackets", "ten", "KGS", "50000"},
			{"C/013", "2024-10-07", "BADGSTIN", "Shree Plating Works", "MS brackets", "100", "KGS", "50000"},
			{"", "2024-10-08", "29ABCDE1234F1Z5", "Shree Plating Works", "MS brackets", "0", "", "50000"},
			{"C/014", "2024-10-09", "29ABCDE1234F1Z5", "Shree Plating Works", "MS brackets", "25", "KGS", "12500"},
		}
		got, errs := ParseChallanCSV(rows)
		if len(got) != 2 || got[0].Challan.No != "C/010" || got[1].Challan.No != "C/014" {
			t.Fatalf("parsed %+v, want the two usable rows", got)
		}
		if got[0].Line != 2 || got[1].Line != 8 {
			t.Errorf("the usable rows came from lines %d and %d, want 2 and 8", got[0].Line, got[1].Line)
		}
		if len(errs) != 4 {
			t.Fatalf("errors = %v, want 4", errs)
		}
		wantLines := []int{4, 5, 6, 7}
		for i, line := range wantLines {
			if errs[i].Line != line {
				t.Errorf("error %d is on line %d, want %d (%s)", i, errs[i].Line, line, errs[i].Message)
			}
		}
		if !strings.Contains(errs[0].Message, "date") {
			t.Errorf("line 4 should complain about the date: %s", errs[0].Message)
		}
		if !strings.Contains(errs[1].Message, "quantity") {
			t.Errorf("line 5 should complain about the quantity: %s", errs[1].Message)
		}
		if !strings.Contains(errs[2].Message, "GSTIN") {
			t.Errorf("line 6 should complain about the GSTIN: %s", errs[2].Message)
		}
		if !strings.Contains(errs[3].Message, "challan number is required") ||
			!strings.Contains(errs[3].Message, "unit of measure") {
			t.Errorf("line 7 should report every problem at once: %s", errs[3].Message)
		}
		if strings.Contains(errs[3].Message, "\n") {
			t.Errorf("a row's problems should be reported on one line: %q", errs[3].Message)
		}
		if errs[0].Error() != "line 4: "+errs[0].Message {
			t.Errorf("RowError.Error() = %q", errs[0].Error())
		}
	})

	t.Run("a file with no header", func(t *testing.T) {
		rows := [][]string{{"C/010", "2024-10-05", "100"}}
		got, errs := ParseChallanCSV(rows)
		if len(got) != 0 || len(errs) != 1 || !strings.Contains(errs[0].Message, "header") {
			t.Fatalf("got %+v, %v; want a complaint about the header", got, errs)
		}
	})

	t.Run("a header with no quantity column", func(t *testing.T) {
		rows := [][]string{{"Challan No", "Date"}, {"C/010", "2024-10-05"}}
		_, errs := ParseChallanCSV(rows)
		if len(errs) != 1 || !strings.Contains(errs[0].Message, "Qty") {
			t.Fatalf("errors = %v, want a complaint about the missing quantity column", errs)
		}
	})

	t.Run("an empty file", func(t *testing.T) {
		_, errs := ParseChallanCSV(nil)
		if len(errs) != 1 || !strings.Contains(errs[0].Message, "empty") {
			t.Fatalf("errors = %v, want a complaint that the file is empty", errs)
		}
	})

	t.Run("short rows are not a crash", func(t *testing.T) {
		rows := [][]string{ChallanCSVHeader(), {"C/010", "2024-10-05"}}
		got, errs := ParseChallanCSV(rows)
		if len(got) != 0 {
			t.Errorf("parsed %+v, want nothing usable", got)
		}
		if len(errs) != 1 {
			t.Fatalf("errors = %v, want one", errs)
		}
	})
}

func TestChallanCSVHeaderMatchesRowWidth(t *testing.T) {
	header := ChallanCSVHeader()
	for i, row := range ChallanCSVRows(register()) {
		if len(row) != len(header) {
			t.Errorf("row %d has %d cells, header has %d", i, len(row), len(header))
		}
	}
}

func TestFilingCSVRows(t *testing.T) {
	q3, err := Quarter(2024, 3)
	if err != nil {
		t.Fatal(err)
	}
	f := PrepareFiling(register(), q3, MustDate("2025-01-15"), 0)
	header := FilingCSVHeader()
	rows := FilingCSVRows(f)

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per challan sent out in the quarter", len(rows))
	}
	for i, row := range rows {
		if len(row) != len(header) {
			t.Fatalf("row %d has %d cells, header has %d", i, len(row), len(header))
		}
	}

	want := []string{
		"29ABCDE1234F1Z5", "Shree Plating Works", "C/001", "2024-10-05",
		"Inputs", "MS brackets for plating", "7308", "KGS",
		"100", "60", "40",
		"50000.00", "2025-10-05", "Pending",
		"263", "", "20000.00",
	}
	for i, cell := range want {
		if rows[0][i] != cell {
			t.Errorf("row 0 cell %d (%s) = %q, want %q", i, header[i], rows[0][i], cell)
		}
	}

	if rows[1][2] != "C/002" || rows[1][4] != "Capital goods" || rows[1][12] != "2027-11-20" {
		t.Errorf("row 1 = %v", rows[1])
	}

	t.Run("an overdue line fills the days-overdue column instead", func(t *testing.T) {
		fy := FinancialYear(2023)
		g := PrepareFiling(register(), fy, MustDate("2025-01-15"), 0)
		overdue := FilingCSVRows(g)
		if len(overdue) != 1 {
			t.Fatalf("got %d rows, want 1", len(overdue))
		}
		row := overdue[0]
		if row[13] != "Overdue" || row[14] != "" || row[15] != "228" || row[16] != "150000.00" {
			t.Errorf("overdue row = %v", row)
		}
	})
}

func TestFormatting(t *testing.T) {
	qty := []struct {
		in   float64
		want string
	}{
		{100, "100"},
		{12.5, "12.5"},
		{0, "0"},
		{1.0000000000000002, "1"},
		{12.3456, "12.346"},
	}
	for _, c := range qty {
		if got := FormatQty(c.in); got != c.want {
			t.Errorf("FormatQty(%v) = %q, want %q", c.in, got, c.want)
		}
	}

	money := []struct {
		in   float64
		want string
	}{
		{50000, "50000.00"},
		{1250.5, "1250.50"},
		{33.333333, "33.33"},
		{0, "0.00"},
	}
	for _, c := range money {
		if got := FormatMoney(c.in); got != c.want {
			t.Errorf("FormatMoney(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
