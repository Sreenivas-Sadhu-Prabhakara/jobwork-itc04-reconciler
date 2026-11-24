package jobwork

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatQty writes a quantity with no trailing zeros and no thousands
// separator, so that a spreadsheet reads it back as a number.
func FormatQty(v float64) string {
	return strconv.FormatFloat(round3(v), 'f', -1, 64)
}

// FormatMoney writes an amount in rupees and paise with no thousands
// separator, so that a spreadsheet reads it back as a number.
func FormatMoney(v float64) string {
	return strconv.FormatFloat(round2(v), 'f', 2, 64)
}

// ChallanCSVHeader is the header row of the challan register export. Importing
// this file back gives the same register, so it doubles as the import template.
func ChallanCSVHeader() []string {
	return []string{
		"Challan No", "Date", "Direction", "Against Challan No",
		"Job Worker GSTIN", "Job Worker Name", "Description", "HSN",
		"Qty", "UQC", "Taxable Value", "Goods Type",
	}
}

// ChallanCSVRows writes the whole register, one row per challan.
func ChallanCSVRows(all []Challan) [][]string {
	rows := make([][]string, 0, len(all))
	for _, c := range all {
		c = c.Normalise()
		rows = append(rows, []string{
			c.No, c.Date.String(), string(c.Direction), c.AgainstChallanNo,
			c.JobWorkerGSTIN, c.JobWorkerName, c.ItemDesc, c.HSN,
			FormatQty(c.Qty), c.UQC, FormatMoney(c.TaxableValue), string(c.GoodsType),
		})
	}
	return rows
}

// FilingCSVHeader is the header row of the ITC-04 filing export: one row per
// challan of goods sent out in the period, with its position as at the review
// date.
func FilingCSVHeader() []string {
	return []string{
		"Job Worker GSTIN", "Job Worker Name", "Challan No", "Challan Date",
		"Goods Type", "Description", "HSN", "UQC",
		"Qty Sent", "Qty Returned", "Qty Outstanding",
		"Taxable Value", "Return Deadline", "Status",
		"Days Left", "Days Overdue", "Deemed Supply Value",
	}
}

// FilingCSVRows writes the filing period's challans of goods sent out, in
// job-worker order, which is the order a filing is keyed in.
func FilingCSVRows(f Filing) [][]string {
	var rows [][]string
	for _, w := range f.Workers {
		for _, a := range w.Lines {
			daysLeft, daysOverdue := "", ""
			if a.State == StatePending {
				daysLeft = strconv.Itoa(a.DaysLeft)
			}
			if a.State == StateOverdue {
				daysOverdue = strconv.Itoa(a.DaysOverdue)
			}
			rows = append(rows, []string{
				w.JobWorkerGSTIN, w.JobWorkerName, a.ChallanNo, a.SentDate.String(),
				a.GoodsType.Label(), a.ItemDesc, a.HSN, a.UQC,
				FormatQty(a.SentQty), FormatQty(a.ReturnedQty), FormatQty(a.OutstandingQty),
				FormatMoney(a.TaxableValue), a.Deadline.String(), string(a.State),
				daysLeft, daysOverdue, FormatMoney(a.DeemedSupplyValue),
			})
		}
	}
	return rows
}

// RowError is one unusable row of an imported file, named by its line number in
// that file so it can be found and fixed.
type RowError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Error makes a RowError usable as an error value.
func (e RowError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Message) }

// headerKey reduces a column heading to the form headings are matched on, so
// that "Challan No.", "challan_no" and "ChallanNo" are the same column.
func headerKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// columnAliases are the headings accepted for each field of a challan.
var columnAliases = map[string][]string{
	"no":        {"challanno", "challannumber", "challan", "no", "challanno1"},
	"date":      {"date", "challandate", "dateofchallan", "sentdate"},
	"direction": {"direction", "sentorreturned", "movement", "inout"},
	"against":   {"againstchallanno", "againstchallan", "against", "originalchallanno", "originalchallan", "parentchallanno"},
	"gstin":     {"jobworkergstin", "gstin", "jwgstin", "gstinofjobworker"},
	"name":      {"jobworkername", "jobworker", "name", "nameofjobworker"},
	"desc":      {"description", "descriptionofgoods", "itemdesc", "itemdescription", "item", "particulars"},
	"hsn":       {"hsn", "hsncode", "hsnsac"},
	"qty":       {"qty", "quantity", "qtysent"},
	"uqc":       {"uqc", "unit", "uom", "unitofmeasure", "units"},
	"value":     {"taxablevalue", "value", "amount", "taxableamount"},
	"goodstype": {"goodstype", "typeofgoods", "type", "inputsorcapitalgoods"},
}

// mapHeader works out which column holds which field.
func mapHeader(header []string) map[string]int {
	byKey := map[string]int{}
	for i, h := range header {
		byKey[headerKey(h)] = i
	}
	cols := map[string]int{}
	for field, aliases := range columnAliases {
		for _, alias := range aliases {
			if i, ok := byKey[alias]; ok {
				cols[field] = i
				break
			}
		}
	}
	return cols
}

// cell reads a column from a row, or "" when the row is short.
func cell(row []string, cols map[string]int, field string) string {
	i, ok := cols[field]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// ParseNumber reads a number as it is typed in a register: with thousands
// separators, a currency symbol, or a trailing unit. An empty cell is zero.
func ParseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	trimmed := s
	for _, prefix := range []string{"₹", "Rs.", "RS.", "rs.", "Rs", "INR"} {
		if rest, found := strings.CutPrefix(trimmed, prefix); found {
			trimmed = strings.TrimSpace(rest)
			break
		}
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= '0' && r <= '9', r == '.', r == '-', r == '+':
			b.WriteRune(r)
		case r == ',', r == ' ', r == ' ':
			// thousands separators and spaces are ignored
		default:
			return 0, fmt.Errorf("%q is not a number", s)
		}
	}
	v, err := strconv.ParseFloat(b.String(), 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return v, nil
}

// ParseDirection reads the direction column, accepting the words a register
// actually uses. An empty value is read from whether the row names a challan to
// come back against.
func ParseDirection(s, against string) (Direction, error) {
	switch headerKey(s) {
	case "":
		if strings.TrimSpace(against) != "" {
			return DirectionReturned, nil
		}
		return DirectionSent, nil
	case "sent", "out", "outward", "dispatch", "dispatched", "issue", "issued", "send":
		return DirectionSent, nil
	case "returned", "return", "in", "inward", "received", "receivedback", "back", "receipt":
		return DirectionReturned, nil
	}
	return "", fmt.Errorf("direction %q should be sent or returned", s)
}

// ParseGoodsType reads the goods-type column. An empty value is read as inputs,
// which is what most job-work movement is.
func ParseGoodsType(s string) (GoodsType, error) {
	switch headerKey(s) {
	case "":
		return GoodsInputs, nil
	case "inputs", "input", "raw", "rawmaterial", "rawmaterials", "i":
		return GoodsInputs, nil
	case "capital", "capitalgoods", "cg", "capitalgood", "c":
		return GoodsCapital, nil
	}
	return "", fmt.Errorf("goods type %q should be inputs or capital", s)
}

// ParsedRow is one usable row of an imported file, with the line of the file it
// came from so that anything else to say about it can point at that line.
type ParsedRow struct {
	Line    int     `json:"line"`
	Challan Challan `json:"challan"`
}

// ParseChallanCSV reads rows of a CSV export or template into challans. The
// first row must be a header; columns may be in any order and extra columns are
// ignored. Every row that cannot be used is reported with its line number, and
// the rows that can be used are still returned, so one bad row does not lose
// the file.
func ParseChallanCSV(rows [][]string) ([]ParsedRow, []RowError) {
	if len(rows) == 0 {
		return nil, []RowError{{Line: 0, Message: "the file is empty"}}
	}
	cols := mapHeader(rows[0])
	if _, ok := cols["no"]; !ok {
		return nil, []RowError{{Line: 1, Message: "no \"Challan No\" column found — the first row must be a header"}}
	}
	if _, ok := cols["qty"]; !ok {
		return nil, []RowError{{Line: 1, Message: "no \"Qty\" column found — the first row must be a header"}}
	}

	var out []ParsedRow
	var errs []RowError
	for i, row := range rows[1:] {
		line := i + 2
		if isBlankRow(row) {
			continue
		}
		c, err := challanFromRow(row, cols)
		if err != nil {
			errs = append(errs, RowError{Line: line, Message: err.Error()})
			continue
		}
		if err := c.Validate(); err != nil {
			errs = append(errs, RowError{Line: line, Message: flattenError(err)})
			continue
		}
		out = append(out, ParsedRow{Line: line, Challan: c})
	}
	return out, errs
}

// challanFromRow turns one CSV row into a challan.
func challanFromRow(row []string, cols map[string]int) (Challan, error) {
	date, err := ParseDate(cell(row, cols, "date"))
	if err != nil {
		return Challan{}, fmt.Errorf("date %q: %w", cell(row, cols, "date"), err)
	}
	qty, err := ParseNumber(cell(row, cols, "qty"))
	if err != nil {
		return Challan{}, fmt.Errorf("quantity: %w", err)
	}
	value, err := ParseNumber(cell(row, cols, "value"))
	if err != nil {
		return Challan{}, fmt.Errorf("taxable value: %w", err)
	}
	against := cell(row, cols, "against")
	direction, err := ParseDirection(cell(row, cols, "direction"), against)
	if err != nil {
		return Challan{}, err
	}
	goodsType, err := ParseGoodsType(cell(row, cols, "goodstype"))
	if err != nil {
		return Challan{}, err
	}
	c := Challan{
		No:               cell(row, cols, "no"),
		Date:             date,
		JobWorkerGSTIN:   cell(row, cols, "gstin"),
		JobWorkerName:    cell(row, cols, "name"),
		ItemDesc:         cell(row, cols, "desc"),
		HSN:              cell(row, cols, "hsn"),
		Qty:              qty,
		UQC:              cell(row, cols, "uqc"),
		TaxableValue:     value,
		GoodsType:        goodsType,
		Direction:        direction,
		AgainstChallanNo: against,
	}
	return c.Normalise(), nil
}

// isBlankRow reports whether a row holds nothing, which is what a trailing
// newline or a spacer row in a spreadsheet looks like.
func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// flattenError writes a joined error on one line, for a per-row message.
func flattenError(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join(strings.Split(err.Error(), "\n"), "; ")
}
