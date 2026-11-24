// Package jobwork holds the rules a principal needs when goods are sent out for
// job work: when each delivery challan falls due, how much of it is still with
// the job worker, when an unreturned quantity becomes a deemed supply, and what
// a Form ITC-04 filing period adds up to.
//
// Everything here is a pure function over records. Nothing in this package
// reads a file, opens a socket or looks at the clock except Today.
package jobwork

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// GoodsType separates the two kinds of goods a principal may send out, because
// they carry different return periods.
type GoodsType string

// The two goods types recognised on a job-work challan.
const (
	GoodsInputs  GoodsType = "inputs"
	GoodsCapital GoodsType = "capital"
)

// Valid reports whether the goods type is one of the two recognised values.
func (g GoodsType) Valid() bool { return g == GoodsInputs || g == GoodsCapital }

// Label is the goods type written for a person to read.
func (g GoodsType) Label() string {
	switch g {
	case GoodsInputs:
		return "Inputs"
	case GoodsCapital:
		return "Capital goods"
	}
	return string(g)
}

// Direction says whether a challan records goods going out to a job worker or
// coming back from one.
type Direction string

// The two directions a challan can record.
const (
	DirectionSent     Direction = "sent"
	DirectionReturned Direction = "returned"
)

// Valid reports whether the direction is one of the two recognised values.
func (d Direction) Valid() bool { return d == DirectionSent || d == DirectionReturned }

// qtyEpsilon is the tolerance used when comparing quantities, so that a
// quantity returned in several parts still settles exactly against the
// quantity sent despite binary floating point.
const qtyEpsilon = 1e-9

// gstinShape is the shape of a GSTIN: a two-digit state code, the ten-character
// PAN of the holder, an entity code, a fixed letter and a check character.
// Only the shape is checked here — the check digit is not recomputed, so a
// GSTIN that passes may still not exist. Confirm it on the GST portal.
var gstinShape = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][0-9A-Z]{3}$`)

// hsnShape is the shape of an HSN code: four to eight digits.
var hsnShape = regexp.MustCompile(`^[0-9]{4,8}$`)

// LooksLikeGSTIN reports whether a string has the shape of a GSTIN. The check
// digit is not recomputed, so a string that passes may still not be a GSTIN
// that exists: confirm it on the GST portal.
func LooksLikeGSTIN(s string) bool {
	return gstinShape.MatchString(strings.ToUpper(strings.TrimSpace(s)))
}

// Challan is one delivery challan line: goods sent out to a job worker, or
// goods received back from one.
type Challan struct {
	No               string    `json:"no"`
	Date             Date      `json:"date"`
	JobWorkerGSTIN   string    `json:"jobWorkerGstin"`
	JobWorkerName    string    `json:"jobWorkerName"`
	ItemDesc         string    `json:"itemDesc"`
	HSN              string    `json:"hsn"`
	Qty              float64   `json:"qty"`
	UQC              string    `json:"uqc"`
	TaxableValue     float64   `json:"taxableValue"`
	GoodsType        GoodsType `json:"goodsType"`
	Direction        Direction `json:"direction"`
	AgainstChallanNo string    `json:"againstChallanNo,omitempty"`
}

// Normalise trims the stray spaces a typed-in or imported record collects and
// upper-cases the fields that are codes. It does not change any number.
func (c Challan) Normalise() Challan {
	c.No = strings.TrimSpace(c.No)
	c.JobWorkerGSTIN = strings.ToUpper(strings.TrimSpace(c.JobWorkerGSTIN))
	c.JobWorkerName = strings.TrimSpace(c.JobWorkerName)
	c.ItemDesc = strings.TrimSpace(c.ItemDesc)
	c.HSN = strings.TrimSpace(c.HSN)
	c.UQC = strings.ToUpper(strings.TrimSpace(c.UQC))
	c.GoodsType = GoodsType(strings.ToLower(strings.TrimSpace(string(c.GoodsType))))
	c.Direction = Direction(strings.ToLower(strings.TrimSpace(string(c.Direction))))
	c.AgainstChallanNo = strings.TrimSpace(c.AgainstChallanNo)
	return c
}

// Key is the form of a challan number used for matching. Challan numbers get
// typed with different spacing and case in different registers, so returns are
// matched against the sent challan case-insensitively and space-insensitively.
func Key(challanNo string) string {
	return strings.ToUpper(strings.Join(strings.Fields(challanNo), ""))
}

// Key is the matching form of this challan's own number.
func (c Challan) Key() string { return Key(c.No) }

// AgainstKey is the matching form of the challan number a return refers to.
func (c Challan) AgainstKey() string { return Key(c.AgainstChallanNo) }

// Validate reports everything wrong with a single challan at once, so a form or
// an imported row can show all of its problems in one pass. It returns nil when
// the record is fit to store.
func (c Challan) Validate() error {
	c = c.Normalise()
	var problems []error
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	if c.No == "" {
		add("challan number is required")
	}
	if c.Date.IsZero() {
		add("challan date is required")
	}
	if c.JobWorkerGSTIN == "" {
		add("job worker GSTIN is required")
	} else if !LooksLikeGSTIN(c.JobWorkerGSTIN) {
		add("job worker GSTIN %q is not in the 15-character GSTIN shape", c.JobWorkerGSTIN)
	}
	if c.JobWorkerName == "" {
		add("job worker name is required")
	}
	if c.ItemDesc == "" {
		add("description of goods is required")
	}
	if c.HSN != "" && !hsnShape.MatchString(c.HSN) {
		add("HSN %q should be 4 to 8 digits", c.HSN)
	}
	if math.IsNaN(c.Qty) || math.IsInf(c.Qty, 0) || c.Qty <= 0 {
		add("quantity must be greater than zero")
	}
	if c.UQC == "" {
		add("unit of measure (UQC) is required")
	}
	if math.IsNaN(c.TaxableValue) || math.IsInf(c.TaxableValue, 0) || c.TaxableValue < 0 {
		add("taxable value cannot be negative")
	}
	if !c.GoodsType.Valid() {
		add("goods type must be %q or %q", GoodsInputs, GoodsCapital)
	}
	switch {
	case !c.Direction.Valid():
		add("direction must be %q or %q", DirectionSent, DirectionReturned)
	case c.Direction == DirectionReturned && c.AgainstChallanNo == "":
		add("a return must name the challan it comes back against")
	case c.Direction == DirectionSent && c.AgainstChallanNo != "":
		add("a challan for goods sent out cannot itself be against another challan")
	}
	return errors.Join(problems...)
}

// ValidateAgainst checks a return against the sent challan it refers to: the
// two must agree about the job worker and the kind of goods, and goods cannot
// come back before they went out.
func ValidateAgainst(ret, sent Challan) error {
	ret, sent = ret.Normalise(), sent.Normalise()
	var problems []error
	if ret.Direction != DirectionReturned {
		problems = append(problems, errors.New("only a return can be recorded against a challan"))
	}
	if sent.Direction != DirectionSent {
		problems = append(problems, fmt.Errorf("challan %s does not record goods sent out", sent.No))
	}
	if ret.Date.Before(sent.Date) {
		problems = append(problems, fmt.Errorf("goods cannot come back on %s, before challan %s was issued on %s",
			ret.Date, sent.No, sent.Date))
	}
	if ret.JobWorkerGSTIN != "" && ret.JobWorkerGSTIN != sent.JobWorkerGSTIN {
		problems = append(problems, fmt.Errorf("challan %s was sent to %s, not %s",
			sent.No, sent.JobWorkerGSTIN, ret.JobWorkerGSTIN))
	}
	if ret.GoodsType != "" && ret.GoodsType != sent.GoodsType {
		problems = append(problems, fmt.Errorf("challan %s records %s, not %s",
			sent.No, sent.GoodsType.Label(), ret.GoodsType.Label()))
	}
	return errors.Join(problems...)
}

// Split separates a register into the challans that sent goods out and the
// challans that recorded goods coming back.
func Split(all []Challan) (sent, returns []Challan) {
	for _, c := range all {
		switch c.Normalise().Direction {
		case DirectionSent:
			sent = append(sent, c)
		case DirectionReturned:
			returns = append(returns, c)
		}
	}
	return sent, returns
}

// ReturnsBySentChallan groups returns by the matching key of the challan they
// come back against.
func ReturnsBySentChallan(returns []Challan) map[string][]Challan {
	byKey := make(map[string][]Challan, len(returns))
	for _, r := range returns {
		if r.Direction != DirectionReturned {
			continue
		}
		k := r.AgainstKey()
		byKey[k] = append(byKey[k], r)
	}
	return byKey
}

// OrphanReturns lists the returns whose challan number matches nothing in the
// register — usually a typo in the challan number, and always worth fixing
// before a filing, because the quantity is not reducing anything.
func OrphanReturns(all []Challan) []Challan {
	sent, returns := Split(all)
	known := make(map[string]bool, len(sent))
	for _, s := range sent {
		known[s.Key()] = true
	}
	var orphans []Challan
	for _, r := range returns {
		if !known[r.AgainstKey()] {
			orphans = append(orphans, r)
		}
	}
	return orphans
}

// round2 rounds money to two decimals (paise), half away from zero.
func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

// round3 rounds a quantity to three decimals, which is the precision ITC-04
// quantities are kept at here, half away from zero.
func round3(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1000) / 1000
}
