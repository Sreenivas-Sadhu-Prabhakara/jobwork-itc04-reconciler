package jobwork

import (
	"sort"
	"strings"
)

// The statutory figures this tool applies, in one place.
//
// Section 143 of the CGST Act, 2017 lets a registered person (the principal)
// send inputs or capital goods to a job worker without paying tax, provided
// they are brought back within the period below. If they are not, section
// 143(3) and 143(4) treat the goods as having been supplied to the job worker
// on the day they were sent out, and tax falls due from that day.
//
// Rule 45 of the CGST Rules, 2017 requires the goods to move under a delivery
// challan and the challans of a period to be reported in Form ITC-04.
//
// These figures were correct when this tool was written. Confirm them against
// the current CGST Act and Rules, and override them in the tool's settings if
// they change.
const (
	// InputsReturnYears is the period within which inputs sent out for job work
	// must be received back — one year, under section 143(1)(a).
	InputsReturnYears = 1

	// CapitalGoodsReturnYears is the period within which capital goods sent out
	// for job work must be received back — three years, under section
	// 143(1)(a). The period does not apply to moulds and dies, jigs and
	// fixtures, or tools sent out to a job worker: see the proviso to section
	// 143(1). This tool applies the three-year period to everything marked as
	// capital goods, so keep those four kinds of items out of the register.
	CapitalGoodsReturnYears = 3

	// HalfYearlyFilingTurnoverThresholdINR is the annual aggregate turnover, in
	// rupees, above which Form ITC-04 is filed half-yearly rather than once a
	// financial year — five crore rupees, under rule 45(3) as substituted with
	// effect from 1 October 2021.
	HalfYearlyFilingTurnoverThresholdINR = 50_000_000 // ₹5,00,00,000

	// FilingDueDayOfMonth is the day of the month succeeding a filing period by
	// which Form ITC-04 is due — the twenty-fifth, under rule 45(3), unless the
	// Commissioner extends it.
	FilingDueDayOfMonth = 25
)

// Rules carries the statutory figures so that a user can override them when the
// law changes without the tool needing a new build. A zero field falls back to
// the constant above.
type Rules struct {
	InputsReturnYears       int     `json:"inputsReturnYears"`
	CapitalGoodsReturnYears int     `json:"capitalGoodsReturnYears"`
	TurnoverThresholdINR    float64 `json:"halfYearlyTurnoverThresholdInr"`
	FilingDueDayOfMonth     int     `json:"filingDueDayOfMonth"`
}

// DefaultRules are the statutory figures as stated in the constants above.
func DefaultRules() Rules {
	return Rules{
		InputsReturnYears:       InputsReturnYears,
		CapitalGoodsReturnYears: CapitalGoodsReturnYears,
		TurnoverThresholdINR:    HalfYearlyFilingTurnoverThresholdINR,
		FilingDueDayOfMonth:     FilingDueDayOfMonth,
	}
}

// Normalise fills any figure left at zero with the statutory default, so a
// partly-filled settings file still produces correct arithmetic.
func (r Rules) Normalise() Rules {
	d := DefaultRules()
	if r.InputsReturnYears <= 0 {
		r.InputsReturnYears = d.InputsReturnYears
	}
	if r.CapitalGoodsReturnYears <= 0 {
		r.CapitalGoodsReturnYears = d.CapitalGoodsReturnYears
	}
	if r.TurnoverThresholdINR <= 0 {
		r.TurnoverThresholdINR = d.TurnoverThresholdINR
	}
	if r.FilingDueDayOfMonth <= 0 || r.FilingDueDayOfMonth > 28 {
		r.FilingDueDayOfMonth = d.FilingDueDayOfMonth
	}
	return r
}

// ReturnYears is the number of years within which goods of this type must come
// back: one for inputs, three for capital goods.
func (r Rules) ReturnYears(g GoodsType) int {
	r = r.Normalise()
	if g == GoodsCapital {
		return r.CapitalGoodsReturnYears
	}
	return r.InputsReturnYears
}

// Deadline is the last date on which goods sent out on sentDate can be received
// back without becoming a deemed supply. It is calendar arithmetic, so a
// challan dated 29 February falls due on 1 March of the target year, that
// calendar date not existing there.
func (r Rules) Deadline(sentDate Date, goodsType GoodsType) Date {
	if sentDate.IsZero() {
		return Date{}
	}
	return sentDate.AddYears(r.ReturnYears(goodsType))
}

// Deadline is Rules.Deadline with the statutory figures.
func Deadline(sentDate Date, goodsType GoodsType) Date {
	return DefaultRules().Deadline(sentDate, goodsType)
}

// ReturnPosition is how much of one sent challan has come back.
type ReturnPosition struct {
	SentQty        float64 `json:"sentQty"`
	ReturnedQty    float64 `json:"returnedQty"`
	OutstandingQty float64 `json:"outstandingQty"`
	ExcessQty      float64 `json:"excessQty"`
	OverReturned   bool    `json:"overReturned"`
	ReturnCount    int     `json:"returnCount"`
}

// Outstanding works out how much of a sent challan is still with the job
// worker. Returns may be partial and may arrive in any number of instalments;
// only returns naming this challan are counted. If more comes back than went
// out — a mis-keyed challan number, or a quantity entered twice — the
// outstanding quantity is clamped at zero and the excess is reported, because a
// negative outstanding quantity would quietly cancel out a real exposure
// elsewhere.
func Outstanding(sent Challan, returns []Challan) ReturnPosition {
	sent = sent.Normalise()
	key := sent.Key()

	pos := ReturnPosition{SentQty: round3(sent.Qty)}
	for _, ret := range returns {
		ret = ret.Normalise()
		if ret.Direction != DirectionReturned || ret.AgainstKey() != key {
			continue
		}
		pos.ReturnedQty += ret.Qty
		pos.ReturnCount++
	}
	pos.ReturnedQty = round3(pos.ReturnedQty)

	outstanding := round3(pos.SentQty - pos.ReturnedQty)
	if outstanding < -qtyEpsilon {
		pos.OverReturned = true
		pos.ExcessQty = round3(-outstanding)
		outstanding = 0
	} else if outstanding < qtyEpsilon {
		outstanding = 0
	}
	pos.OutstandingQty = outstanding
	return pos
}

// DeemedSupplyValue is the taxable value attributable to a quantity still with
// the job worker: the challan's taxable value pro-rated by quantity. It is the
// value on which tax falls due once the challan is overdue, rounded to paise.
// A challan with no quantity carries no value.
func DeemedSupplyValue(taxableValue, sentQty, outstandingQty float64) float64 {
	if sentQty <= 0 || outstandingQty <= 0 || taxableValue <= 0 {
		return 0
	}
	if outstandingQty >= sentQty {
		return round2(taxableValue)
	}
	return round2(taxableValue * outstandingQty / sentQty)
}

// State is where a sent challan stands on a given date.
type State string

// The three states a sent challan can be in.
const (
	// StateReturned means everything that went out has come back.
	StateReturned State = "Returned"
	// StatePending means something is still out, and the deadline has not passed.
	StatePending State = "Pending"
	// StateOverdue means the deadline has passed with goods still out, so the
	// outstanding quantity is a deemed supply.
	StateOverdue State = "Overdue"
)

// Assessment is one sent challan judged as at a date.
type Assessment struct {
	ChallanNo      string    `json:"challanNo"`
	SentDate       Date      `json:"sentDate"`
	JobWorkerGSTIN string    `json:"jobWorkerGstin"`
	JobWorkerName  string    `json:"jobWorkerName"`
	ItemDesc       string    `json:"itemDesc"`
	HSN            string    `json:"hsn"`
	UQC            string    `json:"uqc"`
	GoodsType      GoodsType `json:"goodsType"`

	SentQty        float64 `json:"sentQty"`
	ReturnedQty    float64 `json:"returnedQty"`
	OutstandingQty float64 `json:"outstandingQty"`
	ExcessQty      float64 `json:"excessQty"`
	OverReturned   bool    `json:"overReturned"`
	ReturnCount    int     `json:"returnCount"`

	TaxableValue      float64 `json:"taxableValue"`
	DeemedSupplyValue float64 `json:"deemedSupplyValue"`

	Deadline    Date  `json:"deadline"`
	AsOf        Date  `json:"asOf"`
	State       State `json:"state"`
	DaysLeft    int   `json:"daysLeft"`
	DaysOverdue int   `json:"daysOverdue"`
}

// Status judges one sent challan as at asOf: returned in full, still pending
// with days left, or overdue — in which case the outstanding quantity is a
// deemed supply and the value at risk is reported. The deadline date itself is
// still within time.
func (r Rules) Status(sent Challan, returns []Challan, asOf Date) Assessment {
	sent = sent.Normalise()
	pos := Outstanding(sent, returns)
	deadline := r.Deadline(sent.Date, sent.GoodsType)

	a := Assessment{
		ChallanNo:      sent.No,
		SentDate:       sent.Date,
		JobWorkerGSTIN: sent.JobWorkerGSTIN,
		JobWorkerName:  sent.JobWorkerName,
		ItemDesc:       sent.ItemDesc,
		HSN:            sent.HSN,
		UQC:            sent.UQC,
		GoodsType:      sent.GoodsType,

		SentQty:        pos.SentQty,
		ReturnedQty:    pos.ReturnedQty,
		OutstandingQty: pos.OutstandingQty,
		ExcessQty:      pos.ExcessQty,
		OverReturned:   pos.OverReturned,
		ReturnCount:    pos.ReturnCount,

		TaxableValue: round2(sent.TaxableValue),
		Deadline:     deadline,
		AsOf:         asOf,
	}

	switch {
	case pos.OutstandingQty <= 0:
		a.State = StateReturned
	case asOf.IsZero() || deadline.IsZero() || asOf.OnOrBefore(deadline):
		a.State = StatePending
		a.DaysLeft = DaysBetween(asOf, deadline)
		if a.DaysLeft < 0 {
			a.DaysLeft = 0
		}
		a.DeemedSupplyValue = DeemedSupplyValue(sent.TaxableValue, pos.SentQty, pos.OutstandingQty)
	default:
		a.State = StateOverdue
		a.DaysOverdue = DaysBetween(deadline, asOf)
		a.DeemedSupplyValue = DeemedSupplyValue(sent.TaxableValue, pos.SentQty, pos.OutstandingQty)
	}
	return a
}

// Status is Rules.Status with the statutory figures.
func Status(sent Challan, returns []Challan, asOf Date) Assessment {
	return DefaultRules().Status(sent, returns, asOf)
}

// Assess judges every sent challan in a register as at asOf, in urgency order.
func (r Rules) Assess(all []Challan, asOf Date) []Assessment {
	sent, returns := Split(all)
	out := make([]Assessment, 0, len(sent))
	for _, s := range sent {
		out = append(out, r.Status(s, returns, asOf))
	}
	SortByUrgency(out)
	return out
}

// Assess is Rules.Assess with the statutory figures.
func Assess(all []Challan, asOf Date) []Assessment {
	return DefaultRules().Assess(all, asOf)
}

// SortByUrgency orders assessments the way a principal needs to work through
// them: overdue first and longest overdue at the top, then pending with the
// nearest deadline, then everything already returned. Ties break on challan
// number so the order never shifts between runs.
func SortByUrgency(list []Assessment) {
	rank := map[State]int{StateOverdue: 0, StatePending: 1, StateReturned: 2}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if rank[a.State] != rank[b.State] {
			return rank[a.State] < rank[b.State]
		}
		switch a.State {
		case StateOverdue:
			if a.DaysOverdue != b.DaysOverdue {
				return a.DaysOverdue > b.DaysOverdue
			}
			if a.DeemedSupplyValue != b.DeemedSupplyValue {
				return a.DeemedSupplyValue > b.DeemedSupplyValue
			}
		case StatePending:
			if a.DaysLeft != b.DaysLeft {
				return a.DaysLeft < b.DaysLeft
			}
		}
		return strings.Compare(a.ChallanNo, b.ChallanNo) < 0
	})
}

// AtRisk is the work list: every challan already overdue, plus those falling
// due within the next horizonDays, in urgency order. A horizon of zero returns
// the overdue ones alone.
func AtRisk(list []Assessment, horizonDays int) []Assessment {
	out := make([]Assessment, 0, len(list))
	for _, a := range list {
		switch a.State {
		case StateOverdue:
			out = append(out, a)
		case StatePending:
			if a.DaysLeft <= horizonDays {
				out = append(out, a)
			}
		}
	}
	SortByUrgency(out)
	return out
}

// WorkerPosition is one job worker's running position: what is still with them
// and how much of it has gone past its deadline.
type WorkerPosition struct {
	JobWorkerGSTIN   string   `json:"jobWorkerGstin"`
	JobWorkerName    string   `json:"jobWorkerName"`
	Challans         int      `json:"challans"`
	OpenChallans     int      `json:"openChallans"`
	OverdueChallans  int      `json:"overdueChallans"`
	QtySent          float64  `json:"qtySent"`
	QtyReturned      float64  `json:"qtyReturned"`
	QtyOutstanding   float64  `json:"qtyOutstanding"`
	TaxableValueSent float64  `json:"taxableValueSent"`
	ValueOutstanding float64  `json:"valueOutstanding"`
	ValueOverdue     float64  `json:"valueOverdue"`
	EarliestDeadline Date     `json:"earliestDeadline"`
	UQCs             []string `json:"uqcs"`
}

// Totals is a whole-register or whole-period roll-up.
type Totals struct {
	Challans         int     `json:"challans"`
	Returned         int     `json:"returned"`
	Pending          int     `json:"pending"`
	Overdue          int     `json:"overdue"`
	QtySent          float64 `json:"qtySent"`
	QtyReturned      float64 `json:"qtyReturned"`
	QtyOutstanding   float64 `json:"qtyOutstanding"`
	TaxableValueSent float64 `json:"taxableValueSent"`
	ValueOutstanding float64 `json:"valueOutstanding"`
	ValueOverdue     float64 `json:"valueOverdue"`
	JobWorkers       int     `json:"jobWorkers"`
	MixedUnits       bool    `json:"mixedUnits"`
}

// Dashboard is the whole register seen as at a date.
type Dashboard struct {
	AsOf        Date             `json:"asOf"`
	HorizonDays int              `json:"horizonDays"`
	Workers     []WorkerPosition `json:"workers"`
	AtRisk      []Assessment     `json:"atRisk"`
	Totals      Totals           `json:"totals"`
	Orphans     []Challan        `json:"orphanReturns"`
}

// Review builds the dashboard: outstanding quantity per job worker, the at-risk
// work list, the roll-up, and any returns whose challan number matches nothing.
func (r Rules) Review(all []Challan, asOf Date, horizonDays int) Dashboard {
	assessments := r.Assess(all, asOf)
	d := Dashboard{
		AsOf:        asOf,
		HorizonDays: horizonDays,
		Workers:     GroupByWorker(assessments),
		AtRisk:      AtRisk(assessments, horizonDays),
		Totals:      RollUp(assessments),
		Orphans:     OrphanReturns(all),
	}
	return d
}

// Review is Rules.Review with the statutory figures.
func Review(all []Challan, asOf Date, horizonDays int) Dashboard {
	return DefaultRules().Review(all, asOf, horizonDays)
}

// GroupByWorker rolls assessments up per job worker, keyed on GSTIN, ordered
// with the largest value still outstanding first.
func GroupByWorker(list []Assessment) []WorkerPosition {
	index := map[string]*WorkerPosition{}
	var order []string
	units := map[string]map[string]bool{}

	for _, a := range list {
		w, ok := index[a.JobWorkerGSTIN]
		if !ok {
			w = &WorkerPosition{JobWorkerGSTIN: a.JobWorkerGSTIN, JobWorkerName: a.JobWorkerName}
			index[a.JobWorkerGSTIN] = w
			order = append(order, a.JobWorkerGSTIN)
			units[a.JobWorkerGSTIN] = map[string]bool{}
		}
		w.Challans++
		w.QtySent = round3(w.QtySent + a.SentQty)
		w.QtyReturned = round3(w.QtyReturned + a.ReturnedQty)
		w.QtyOutstanding = round3(w.QtyOutstanding + a.OutstandingQty)
		w.TaxableValueSent = round2(w.TaxableValueSent + a.TaxableValue)
		if a.UQC != "" {
			units[a.JobWorkerGSTIN][a.UQC] = true
		}
		if a.State != StateReturned {
			w.OpenChallans++
			w.ValueOutstanding = round2(w.ValueOutstanding + a.DeemedSupplyValue)
			if w.EarliestDeadline.IsZero() || a.Deadline.Before(w.EarliestDeadline) {
				w.EarliestDeadline = a.Deadline
			}
		}
		if a.State == StateOverdue {
			w.OverdueChallans++
			w.ValueOverdue = round2(w.ValueOverdue + a.DeemedSupplyValue)
		}
	}

	out := make([]WorkerPosition, 0, len(order))
	for _, gstin := range order {
		w := index[gstin]
		for u := range units[gstin] {
			w.UQCs = append(w.UQCs, u)
		}
		sort.Strings(w.UQCs)
		out = append(out, *w)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ValueOverdue != out[j].ValueOverdue {
			return out[i].ValueOverdue > out[j].ValueOverdue
		}
		if out[i].ValueOutstanding != out[j].ValueOutstanding {
			return out[i].ValueOutstanding > out[j].ValueOutstanding
		}
		return out[i].JobWorkerGSTIN < out[j].JobWorkerGSTIN
	})
	return out
}

// RollUp totals a set of assessments. Quantities are only ever added within the
// same unit of measure; when a set mixes units the total is still returned but
// MixedUnits is set, so a report can say so instead of implying that kilograms
// and pieces add up.
func RollUp(list []Assessment) Totals {
	t := Totals{}
	workers := map[string]bool{}
	units := map[string]bool{}
	for _, a := range list {
		t.Challans++
		switch a.State {
		case StateReturned:
			t.Returned++
		case StatePending:
			t.Pending++
		case StateOverdue:
			t.Overdue++
			t.ValueOverdue = round2(t.ValueOverdue + a.DeemedSupplyValue)
		}
		t.QtySent = round3(t.QtySent + a.SentQty)
		t.QtyReturned = round3(t.QtyReturned + a.ReturnedQty)
		t.QtyOutstanding = round3(t.QtyOutstanding + a.OutstandingQty)
		t.TaxableValueSent = round2(t.TaxableValueSent + a.TaxableValue)
		if a.State != StateReturned {
			t.ValueOutstanding = round2(t.ValueOutstanding + a.DeemedSupplyValue)
		}
		workers[a.JobWorkerGSTIN] = true
		if a.UQC != "" {
			units[a.UQC] = true
		}
	}
	t.JobWorkers = len(workers)
	t.MixedUnits = len(units) > 1
	return t
}
