package jobwork

import (
	"fmt"
	"sort"
	"time"
)

// Frequency is how often Form ITC-04 is filed.
type Frequency string

// The filing frequencies this tool can prepare a period for. Rule 45(3), as
// substituted with effect from 1 October 2021, puts a principal on one of the
// first two; the quarterly period is kept because it was the frequency for
// every principal before that date, and because a shorter period is a useful
// way to review a long one.
const (
	FreqQuarterly  Frequency = "quarterly"
	FreqHalfYearly Frequency = "half-yearly"
	FreqAnnual     Frequency = "annual"

	// FreqCustom marks a span of dates that is not a statutory period.
	FreqCustom Frequency = "custom"
)

// FilingFrequency is the frequency rule 45(3) puts a principal on, given their
// annual aggregate turnover in the preceding financial year: half-yearly above
// the threshold, once a financial year at or below it. A turnover of zero means
// "not stated", and the stricter of the two is returned so that nothing is
// understated by leaving the figure blank.
func (r Rules) FilingFrequency(annualAggregateTurnoverINR float64) Frequency {
	r = r.Normalise()
	if annualAggregateTurnoverINR <= 0 {
		return FreqHalfYearly
	}
	if annualAggregateTurnoverINR > r.TurnoverThresholdINR {
		return FreqHalfYearly
	}
	return FreqAnnual
}

// FilingFrequency is Rules.FilingFrequency with the statutory threshold.
func FilingFrequency(annualAggregateTurnoverINR float64) Frequency {
	return DefaultRules().FilingFrequency(annualAggregateTurnoverINR)
}

// FinancialYearStartMonth is the month an Indian financial year begins in.
const FinancialYearStartMonth = time.April

// Period is a filing period: the span of dates whose challans go into one
// Form ITC-04.
type Period struct {
	Frequency     Frequency `json:"frequency"`
	Label         string    `json:"label"`
	From          Date      `json:"from"`
	To            Date      `json:"to"`
	FinancialYear string    `json:"financialYear"`
	Index         int       `json:"index"`
}

// FinancialYearStart is the calendar year in which the Indian financial year
// containing d began: 1 March 2025 belongs to the year that began in 2024.
func FinancialYearStart(d Date) int {
	if d.Month() < FinancialYearStartMonth {
		return d.Year() - 1
	}
	return d.Year()
}

// FinancialYearLabel writes a financial year the way it is written on a
// return: the year it starts in, then the last two digits of the next.
func FinancialYearLabel(fyStart int) string {
	return fmt.Sprintf("FY%d-%02d", fyStart, (fyStart+1)%100)
}

// monthsLabel writes a span of months for a period label.
func monthsLabel(from, to Date) string {
	if from.Year() == to.Year() {
		return fmt.Sprintf("%s-%s %d", from.Month().String()[:3], to.Month().String()[:3], from.Year())
	}
	return fmt.Sprintf("%s %d-%s %d", from.Month().String()[:3], from.Year(), to.Month().String()[:3], to.Year())
}

// Quarter is quarter q of the financial year beginning in fyStart, counted the
// Indian way: Q1 is April to June. q must be 1 to 4.
func Quarter(fyStart, q int) (Period, error) {
	if q < 1 || q > 4 {
		return Period{}, fmt.Errorf("quarter must be 1 to 4, got %d", q)
	}
	from := NewDate(fyStart, FinancialYearStartMonth+time.Month(3*(q-1)), 1)
	to := NewDate(from.Year(), from.Month()+3, 1).AddDays(-1)
	return Period{
		Frequency:     FreqQuarterly,
		Label:         fmt.Sprintf("Q%d %s (%s)", q, FinancialYearLabel(fyStart), monthsLabel(from, to)),
		From:          from,
		To:            to,
		FinancialYear: FinancialYearLabel(fyStart),
		Index:         q,
	}, nil
}

// HalfYear is half h of the financial year beginning in fyStart: H1 is April to
// September, H2 is October to March. h must be 1 or 2.
func HalfYear(fyStart, h int) (Period, error) {
	if h < 1 || h > 2 {
		return Period{}, fmt.Errorf("half must be 1 or 2, got %d", h)
	}
	from := NewDate(fyStart, FinancialYearStartMonth+time.Month(6*(h-1)), 1)
	to := NewDate(from.Year(), from.Month()+6, 1).AddDays(-1)
	return Period{
		Frequency:     FreqHalfYearly,
		Label:         fmt.Sprintf("H%d %s (%s)", h, FinancialYearLabel(fyStart), monthsLabel(from, to)),
		From:          from,
		To:            to,
		FinancialYear: FinancialYearLabel(fyStart),
		Index:         h,
	}, nil
}

// FinancialYear is the whole financial year beginning in fyStart.
func FinancialYear(fyStart int) Period {
	from := NewDate(fyStart, FinancialYearStartMonth, 1)
	to := NewDate(fyStart+1, FinancialYearStartMonth, 1).AddDays(-1)
	return Period{
		Frequency:     FreqAnnual,
		Label:         fmt.Sprintf("%s (%s)", FinancialYearLabel(fyStart), monthsLabel(from, to)),
		From:          from,
		To:            to,
		FinancialYear: FinancialYearLabel(fyStart),
		Index:         1,
	}
}

// CustomPeriod is any span of dates, for reviewing something the statutory
// periods do not line up with.
func CustomPeriod(from, to Date) (Period, error) {
	if from.IsZero() || to.IsZero() {
		return Period{}, fmt.Errorf("a period needs both a start and an end date")
	}
	if to.Before(from) {
		return Period{}, fmt.Errorf("period ends on %s, before it starts on %s", to, from)
	}
	return Period{
		Frequency:     FreqCustom,
		Label:         fmt.Sprintf("%s to %s", from, to),
		From:          from,
		To:            to,
		FinancialYear: FinancialYearLabel(FinancialYearStart(from)),
	}, nil
}

// PeriodFor builds the period of the given frequency and index within the
// financial year beginning in fyStart.
func PeriodFor(freq Frequency, fyStart, index int) (Period, error) {
	switch freq {
	case FreqQuarterly:
		return Quarter(fyStart, index)
	case FreqHalfYearly:
		return HalfYear(fyStart, index)
	case FreqAnnual:
		return FinancialYear(fyStart), nil
	}
	return Period{}, fmt.Errorf("unknown filing frequency %q", freq)
}

// PeriodsIn lists every period of a frequency in one financial year, for a
// period picker.
func PeriodsIn(freq Frequency, fyStart int) []Period {
	count := map[Frequency]int{FreqQuarterly: 4, FreqHalfYearly: 2, FreqAnnual: 1}[freq]
	out := make([]Period, 0, count)
	for i := 1; i <= count; i++ {
		if p, err := PeriodFor(freq, fyStart, i); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// Contains reports whether a date falls in the period. Both ends are included.
func (p Period) Contains(d Date) bool {
	if d.IsZero() || p.From.IsZero() || p.To.IsZero() {
		return false
	}
	return !d.Before(p.From) && !d.After(p.To)
}

// DueDate is the date Form ITC-04 for this period is due: the twenty-fifth of
// the month after the period ends, unless the Commissioner extends it.
func (r Rules) DueDate(p Period) Date {
	if p.To.IsZero() {
		return Date{}
	}
	r = r.Normalise()
	return NewDate(p.To.Year(), p.To.Month()+1, r.FilingDueDayOfMonth)
}

// DueDate is Rules.DueDate with the statutory day of the month.
func DueDate(p Period) Date { return DefaultRules().DueDate(p) }

// ReturnLine is one quantity received back from a job worker in the period,
// with the sent challan it settles against.
type ReturnLine struct {
	ChallanNo        string    `json:"challanNo"`
	Date             Date      `json:"date"`
	AgainstChallanNo string    `json:"againstChallanNo"`
	AgainstDate      Date      `json:"againstDate"`
	Matched          bool      `json:"matched"`
	JobWorkerGSTIN   string    `json:"jobWorkerGstin"`
	JobWorkerName    string    `json:"jobWorkerName"`
	ItemDesc         string    `json:"itemDesc"`
	HSN              string    `json:"hsn"`
	UQC              string    `json:"uqc"`
	GoodsType        GoodsType `json:"goodsType"`
	Qty              float64   `json:"qty"`
	TaxableValue     float64   `json:"taxableValue"`
}

// WorkerFiling is one job worker's part of a filing period.
type WorkerFiling struct {
	JobWorkerGSTIN      string       `json:"jobWorkerGstin"`
	JobWorkerName       string       `json:"jobWorkerName"`
	ChallansSent        int          `json:"challansSent"`
	QtySent             float64      `json:"qtySent"`
	TaxableValueSent    float64      `json:"taxableValueSent"`
	ReturnsInPeriod     int          `json:"returnsInPeriod"`
	QtyReturnedInPeriod float64      `json:"qtyReturnedInPeriod"`
	QtyOutstanding      float64      `json:"qtyOutstanding"`
	OverdueChallans     int          `json:"overdueChallans"`
	DeemedSupplyValue   float64      `json:"deemedSupplyValue"`
	Lines               []Assessment `json:"lines"`
	Returns             []ReturnLine `json:"returns"`
}

// Filing is everything one Form ITC-04 period adds up to.
type Filing struct {
	Period             Period         `json:"period"`
	AsOf               Date           `json:"asOf"`
	DueDate            Date           `json:"dueDate"`
	SuggestedFrequency Frequency      `json:"suggestedFrequency"`
	TurnoverINR        float64        `json:"annualAggregateTurnoverInr"`
	TurnoverThreshold  float64        `json:"turnoverThresholdInr"`
	Workers            []WorkerFiling `json:"workers"`
	Totals             Totals         `json:"totals"`
	ReturnsInPeriod    int            `json:"returnsInPeriodCount"`
	QtyReturnedInPrd   float64        `json:"qtyReturnedInPeriod"`
	OrphanReturns      []Challan      `json:"orphanReturns"`
}

// Filing gathers a period's challans into the shape a Form ITC-04 filing needs,
// grouped by job worker.
//
// Goods sent out are the challans dated in the period — that is what the
// period reports. Each of those is then judged as at asOf against every return
// in the register, not only the returns dated in the period, because a
// quantity that came back later is still back: outstanding quantity is
// cumulative, not per period. Goods received back in the period are listed
// separately, each against the challan it settles.
func (r Rules) Filing(all []Challan, p Period, asOf Date, annualAggregateTurnoverINR float64) Filing {
	r = r.Normalise()
	sent, returns := Split(all)

	f := Filing{
		Period:             p,
		AsOf:               asOf,
		DueDate:            r.DueDate(p),
		SuggestedFrequency: r.FilingFrequency(annualAggregateTurnoverINR),
		TurnoverINR:        annualAggregateTurnoverINR,
		TurnoverThreshold:  r.TurnoverThresholdINR,
		OrphanReturns:      OrphanReturns(all),
	}

	sentByKey := make(map[string]Challan, len(sent))
	for _, s := range sent {
		sentByKey[s.Key()] = s.Normalise()
	}

	index := map[string]*WorkerFiling{}
	worker := func(gstin, name string) *WorkerFiling {
		w, ok := index[gstin]
		if !ok {
			w = &WorkerFiling{JobWorkerGSTIN: gstin, JobWorkerName: name}
			index[gstin] = w
		}
		if w.JobWorkerName == "" {
			w.JobWorkerName = name
		}
		return w
	}

	var lines []Assessment
	for _, s := range sent {
		s = s.Normalise()
		if !p.Contains(s.Date) {
			continue
		}
		a := r.Status(s, returns, asOf)
		lines = append(lines, a)

		w := worker(a.JobWorkerGSTIN, a.JobWorkerName)
		w.ChallansSent++
		w.QtySent = round3(w.QtySent + a.SentQty)
		w.TaxableValueSent = round2(w.TaxableValueSent + a.TaxableValue)
		w.QtyOutstanding = round3(w.QtyOutstanding + a.OutstandingQty)
		if a.State == StateOverdue {
			w.OverdueChallans++
			w.DeemedSupplyValue = round2(w.DeemedSupplyValue + a.DeemedSupplyValue)
		}
		w.Lines = append(w.Lines, a)
	}

	for _, ret := range returns {
		ret = ret.Normalise()
		if !p.Contains(ret.Date) {
			continue
		}
		line := ReturnLine{
			ChallanNo:        ret.No,
			Date:             ret.Date,
			AgainstChallanNo: ret.AgainstChallanNo,
			JobWorkerGSTIN:   ret.JobWorkerGSTIN,
			JobWorkerName:    ret.JobWorkerName,
			ItemDesc:         ret.ItemDesc,
			HSN:              ret.HSN,
			UQC:              ret.UQC,
			GoodsType:        ret.GoodsType,
			Qty:              round3(ret.Qty),
			TaxableValue:     round2(ret.TaxableValue),
		}
		if orig, ok := sentByKey[ret.AgainstKey()]; ok {
			line.Matched = true
			line.AgainstDate = orig.Date
			if line.JobWorkerGSTIN == "" {
				line.JobWorkerGSTIN = orig.JobWorkerGSTIN
			}
			if line.JobWorkerName == "" {
				line.JobWorkerName = orig.JobWorkerName
			}
		}
		f.ReturnsInPeriod++
		f.QtyReturnedInPrd = round3(f.QtyReturnedInPrd + line.Qty)

		w := worker(line.JobWorkerGSTIN, line.JobWorkerName)
		w.ReturnsInPeriod++
		w.QtyReturnedInPeriod = round3(w.QtyReturnedInPeriod + line.Qty)
		w.Returns = append(w.Returns, line)
	}

	SortByUrgency(lines)
	f.Totals = RollUp(lines)

	out := make([]WorkerFiling, 0, len(index))
	for _, w := range index {
		sort.SliceStable(w.Lines, func(i, j int) bool {
			if !w.Lines[i].SentDate.Equal(w.Lines[j].SentDate) {
				return w.Lines[i].SentDate.Before(w.Lines[j].SentDate)
			}
			return w.Lines[i].ChallanNo < w.Lines[j].ChallanNo
		})
		sort.SliceStable(w.Returns, func(i, j int) bool {
			if !w.Returns[i].Date.Equal(w.Returns[j].Date) {
				return w.Returns[i].Date.Before(w.Returns[j].Date)
			}
			return w.Returns[i].ChallanNo < w.Returns[j].ChallanNo
		})
		out = append(out, *w)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].JobWorkerGSTIN < out[j].JobWorkerGSTIN })
	f.Workers = out
	return f
}

// PrepareFiling is Rules.Filing with the statutory figures.
func PrepareFiling(all []Challan, p Period, asOf Date, annualAggregateTurnoverINR float64) Filing {
	return DefaultRules().Filing(all, p, asOf, annualAggregateTurnoverINR)
}
