package jobwork

import (
	"strings"
	"testing"
)

// sentChallan is a valid challan for goods sent out, for tests to vary.
func sentChallan() Challan {
	return Challan{
		No:             "C/001",
		Date:           MustDate("2024-10-05"),
		JobWorkerGSTIN: "29ABCDE1234F1Z5",
		JobWorkerName:  "Shree Plating Works",
		ItemDesc:       "MS brackets for plating",
		HSN:            "7308",
		Qty:            100,
		UQC:            "KGS",
		TaxableValue:   50000,
		GoodsType:      GoodsInputs,
		Direction:      DirectionSent,
	}
}

// returnChallan is a valid challan for goods received back against C/001.
func returnChallan(no, date string, qty float64) Challan {
	return Challan{
		No:               no,
		Date:             MustDate(date),
		JobWorkerGSTIN:   "29ABCDE1234F1Z5",
		JobWorkerName:    "Shree Plating Works",
		ItemDesc:         "MS brackets, plated",
		HSN:              "7308",
		Qty:              qty,
		UQC:              "KGS",
		TaxableValue:     0,
		GoodsType:        GoodsInputs,
		Direction:        DirectionReturned,
		AgainstChallanNo: "C/001",
	}
}

func TestChallanValidate(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Challan)
		wants []string // substrings every one of which must appear in the error
	}{
		{"a complete challan for goods sent out", func(*Challan) {}, nil},
		{"capital goods", func(c *Challan) { c.GoodsType = GoodsCapital }, nil},
		{"no HSN is allowed", func(c *Challan) { c.HSN = "" }, nil},
		{"an eight-digit HSN", func(c *Challan) { c.HSN = "73082000" }, nil},
		{"a fractional quantity", func(c *Challan) { c.Qty = 12.5 }, nil},
		{"zero taxable value", func(c *Challan) { c.TaxableValue = 0 }, nil},

		{"missing challan number", func(c *Challan) { c.No = "  " }, []string{"challan number is required"}},
		{"missing date", func(c *Challan) { c.Date = Date{} }, []string{"challan date is required"}},
		{"missing GSTIN", func(c *Challan) { c.JobWorkerGSTIN = "" }, []string{"GSTIN is required"}},
		{"GSTIN too short", func(c *Challan) { c.JobWorkerGSTIN = "29ABCDE1234F1Z" }, []string{"GSTIN shape"}},
		{"GSTIN with a letter where the state code goes", func(c *Challan) { c.JobWorkerGSTIN = "2XABCDE1234F1Z5" }, []string{"GSTIN shape"}},
		{"GSTIN with digits in the PAN letters", func(c *Challan) { c.JobWorkerGSTIN = "29ABC1E1234F1Z5" }, []string{"GSTIN shape"}},
		{"missing job worker name", func(c *Challan) { c.JobWorkerName = "" }, []string{"job worker name is required"}},
		{"missing description", func(c *Challan) { c.ItemDesc = "" }, []string{"description of goods is required"}},
		{"HSN too short", func(c *Challan) { c.HSN = "73" }, []string{"HSN"}},
		{"HSN with letters", func(c *Challan) { c.HSN = "73O8" }, []string{"HSN"}},
		{"zero quantity", func(c *Challan) { c.Qty = 0 }, []string{"quantity must be greater than zero"}},
		{"negative quantity", func(c *Challan) { c.Qty = -5 }, []string{"quantity must be greater than zero"}},
		{"missing unit", func(c *Challan) { c.UQC = "" }, []string{"unit of measure"}},
		{"negative taxable value", func(c *Challan) { c.TaxableValue = -1 }, []string{"taxable value cannot be negative"}},
		{"unknown goods type", func(c *Challan) { c.GoodsType = "tools" }, []string{"goods type must be"}},
		{"unknown direction", func(c *Challan) { c.Direction = "moved" }, []string{"direction must be"}},
		{"goods sent out cannot be against another challan", func(c *Challan) { c.AgainstChallanNo = "C/000" }, []string{"cannot itself be against"}},
		{
			"several problems are all reported",
			func(c *Challan) { c.No = ""; c.Qty = 0; c.UQC = ""; c.GoodsType = "" },
			[]string{"challan number is required", "quantity must be greater than zero", "unit of measure", "goods type must be"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ch := sentChallan()
			c.edit(&ch)
			err := ch.Validate()
			if len(c.wants) == 0 {
				if err != nil {
					t.Fatalf("Validate() = %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %v", c.wants)
			}
			for _, want := range c.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Validate() = %q, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestChallanValidateReturn(t *testing.T) {
	t.Run("a complete return", func(t *testing.T) {
		if err := returnChallan("R/001", "2024-11-10", 60).Validate(); err != nil {
			t.Fatalf("Validate() = %v, want no error", err)
		}
	})
	t.Run("a return must name the challan it settles", func(t *testing.T) {
		r := returnChallan("R/001", "2024-11-10", 60)
		r.AgainstChallanNo = ""
		err := r.Validate()
		if err == nil || !strings.Contains(err.Error(), "must name the challan") {
			t.Fatalf("Validate() = %v, want it to require the original challan number", err)
		}
	})
}

func TestNormaliseAndKey(t *testing.T) {
	c := Challan{
		No:               "  c/001 ",
		JobWorkerGSTIN:   " 29abcde1234f1z5 ",
		UQC:              " kgs ",
		HSN:              " 7308 ",
		GoodsType:        " Inputs ",
		Direction:        " Returned ",
		AgainstChallanNo: " C/000 ",
		JobWorkerName:    "  Shree Plating Works ",
		ItemDesc:         " brackets ",
	}.Normalise()

	if c.No != "c/001" {
		t.Errorf("No = %q", c.No)
	}
	if c.JobWorkerGSTIN != "29ABCDE1234F1Z5" {
		t.Errorf("GSTIN = %q", c.JobWorkerGSTIN)
	}
	if c.UQC != "KGS" {
		t.Errorf("UQC = %q", c.UQC)
	}
	if c.GoodsType != GoodsInputs {
		t.Errorf("GoodsType = %q", c.GoodsType)
	}
	if c.Direction != DirectionReturned {
		t.Errorf("Direction = %q", c.Direction)
	}
	if c.JobWorkerName != "Shree Plating Works" || c.ItemDesc != "brackets" || c.HSN != "7308" {
		t.Errorf("text fields not trimmed: %+v", c)
	}

	keyCases := []struct{ in, want string }{
		{"C/001", "C/001"},
		{" c/001 ", "C/001"},
		{"c / 001", "C/001"},
		{"DC\t0012", "DC0012"},
	}
	for _, k := range keyCases {
		if got := Key(k.in); got != k.want {
			t.Errorf("Key(%q) = %q, want %q", k.in, got, k.want)
		}
	}
}

func TestValidateAgainst(t *testing.T) {
	sent := sentChallan()
	cases := []struct {
		name string
		edit func(*Challan)
		want string // "" means no error
	}{
		{"a matching return", func(*Challan) {}, ""},
		{"goods back on the day they went out", func(r *Challan) { r.Date = sent.Date }, ""},
		{"goods back before they went out", func(r *Challan) { r.Date = MustDate("2024-10-04") }, "cannot come back"},
		{"a different job worker", func(r *Challan) { r.JobWorkerGSTIN = "27PQRSX5678K1ZV" }, "was sent to"},
		{"a different kind of goods", func(r *Challan) { r.GoodsType = GoodsCapital }, "records Inputs"},
		{"not a return at all", func(r *Challan) { r.Direction = DirectionSent }, "only a return"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ret := returnChallan("R/001", "2024-11-10", 60)
			c.edit(&ret)
			err := ValidateAgainst(ret, sent)
			if c.want == "" {
				if err != nil {
					t.Fatalf("ValidateAgainst() = %v, want no error", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidateAgainst() = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestSplitAndOrphanReturns(t *testing.T) {
	orphan := returnChallan("R/009", "2024-12-20", 5)
	orphan.AgainstChallanNo = "C/999"

	register := []Challan{
		sentChallan(),
		returnChallan("R/001", "2024-11-10", 60),
		returnChallan("R/002", "2024-11-20", 10),
		orphan,
	}

	sent, returns := Split(register)
	if len(sent) != 1 || len(returns) != 3 {
		t.Fatalf("Split gave %d sent and %d returns, want 1 and 3", len(sent), len(returns))
	}

	grouped := ReturnsBySentChallan(returns)
	if got := len(grouped[Key("C/001")]); got != 2 {
		t.Errorf("C/001 has %d returns grouped against it, want 2", got)
	}

	orphans := OrphanReturns(register)
	if len(orphans) != 1 || orphans[0].No != "R/009" {
		t.Fatalf("OrphanReturns = %+v, want only R/009", orphans)
	}

	t.Run("a register with nothing in it", func(t *testing.T) {
		if s, r := Split(nil); s != nil || r != nil {
			t.Errorf("Split(nil) = %v, %v, want nothing", s, r)
		}
		if got := OrphanReturns(nil); got != nil {
			t.Errorf("OrphanReturns(nil) = %v, want nothing", got)
		}
	})
}

func TestRounding(t *testing.T) {
	moneyCases := []struct {
		in   float64
		want float64
	}{
		{33.333333, 33.33},
		{33.336, 33.34},
		{0.005, 0.01},
		{-0.005, -0.01},
		{1250.5, 1250.5},
		{0, 0},
	}
	for _, c := range moneyCases {
		if got := round2(c.in); got != c.want {
			t.Errorf("round2(%v) = %v, want %v", c.in, got, c.want)
		}
	}

	qtyCases := []struct {
		in   float64
		want float64
	}{
		{12.3456, 12.346},
		{0.0004, 0},
		{1.0000000000000002, 1},
		{100, 100},
	}
	for _, c := range qtyCases {
		if got := round3(c.in); got != c.want {
			t.Errorf("round3(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
