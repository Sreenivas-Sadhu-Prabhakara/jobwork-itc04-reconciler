package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sreenivas-sadhu-prabhakara/jobwork-itc04-reconciler/internal/jobwork"
)

// api serves the JSON interface the page talks to. Every call is same-origin
// and nothing leaves the machine.
type api struct {
	store *store
}

// routes registers every endpoint.
func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/state", a.getState)
	mux.HandleFunc("POST /api/challans", a.postChallan)
	mux.HandleFunc("POST /api/returns", a.postReturn)
	mux.HandleFunc("DELETE /api/challans", a.deleteChallan)
	mux.HandleFunc("PUT /api/settings", a.putSettings)
	mux.HandleFunc("GET /api/itc04", a.getFiling)
	mux.HandleFunc("GET /api/itc04.json", a.getFilingJSONFile)
	mux.HandleFunc("GET /api/itc04.csv", a.getFilingCSV)
	mux.HandleFunc("GET /api/challans.csv", a.getRegisterCSV)
	mux.HandleFunc("GET /api/template.csv", a.getTemplateCSV)
	mux.HandleFunc("POST /api/import", a.postImport)
}

// statutoryFigures are the figures in force, sent to the page so that it can
// state them rather than hard-coding them a second time.
type statutoryFigures struct {
	InputsReturnYears       int     `json:"inputsReturnYears"`
	CapitalGoodsReturnYears int     `json:"capitalGoodsReturnYears"`
	TurnoverThresholdINR    float64 `json:"turnoverThresholdInr"`
	FilingDueDayOfMonth     int     `json:"filingDueDayOfMonth"`
	StatutoryDefaults       bool    `json:"statutoryDefaults"`
}

// periodOption is one choice in the filing-period picker.
type periodOption struct {
	Frequency jobwork.Frequency `json:"frequency"`
	Index     int               `json:"index"`
	FYStart   int               `json:"fyStart"`
	Label     string            `json:"label"`
	From      jobwork.Date      `json:"from"`
	To        jobwork.Date      `json:"to"`
	DueDate   jobwork.Date      `json:"dueDate"`
}

// statePayload is everything the page needs to draw itself.
type statePayload struct {
	Today              jobwork.Date         `json:"today"`
	AsOf               jobwork.Date         `json:"asOf"`
	Settings           settings             `json:"settings"`
	Statutory          statutoryFigures     `json:"statutory"`
	SuggestedFrequency jobwork.Frequency    `json:"suggestedFrequency"`
	Challans           []jobwork.Challan    `json:"challans"`
	Assessments        []jobwork.Assessment `json:"assessments"`
	Dashboard          jobwork.Dashboard    `json:"dashboard"`
	Periods            []periodOption       `json:"periods"`
	DataFile           string               `json:"dataFile"`
}

// getState answers with the whole register as judged on a date.
func (a *api) getState(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()
	asOf, err := dateParam(r, "asOf", jobwork.Today())
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	horizon := data.Settings.HorizonDays
	if v := strings.TrimSpace(r.URL.Query().Get("horizon")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest, errors.New("horizon must be a number of days"))
			return
		}
		horizon = n
	}

	rules := data.Settings.Rules.Normalise()
	writeJSON(w, http.StatusOK, statePayload{
		Today:              jobwork.Today(),
		AsOf:               asOf,
		Settings:           data.Settings,
		Statutory:          figures(rules),
		SuggestedFrequency: rules.FilingFrequency(data.Settings.TurnoverINR),
		Challans:           data.Challans,
		Assessments:        rules.Assess(data.Challans, asOf),
		Dashboard:          rules.Review(data.Challans, asOf, horizon),
		Periods:            periodOptions(rules, asOf),
		DataFile:           a.store.path,
	})
}

// postChallan records goods sent out to a job worker.
func (a *api) postChallan(w http.ResponseWriter, r *http.Request) {
	var in jobwork.Challan
	if !readJSON(w, r, &in) {
		return
	}
	in.Direction = jobwork.DirectionSent
	in.AgainstChallanNo = ""

	saved, err := a.store.addChallan(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"challan": saved})
}

// postReturn records goods received back against a challan.
func (a *api) postReturn(w http.ResponseWriter, r *http.Request) {
	var in jobwork.Challan
	if !readJSON(w, r, &in) {
		return
	}
	in.Direction = jobwork.DirectionReturned

	if strings.TrimSpace(in.AgainstChallanNo) == "" {
		writeProblem(w, http.StatusBadRequest, errors.New("a return must name the challan it comes back against"))
		return
	}

	// A return inherits the job worker and the kind of goods from the challan
	// it settles against, so the form only has to ask what came back and when.
	// Without that challan there is nothing to inherit and nothing to reduce,
	// so say so rather than complaining about the fields it would have filled.
	data := a.store.snapshot()
	i := find(data.Challans, in.AgainstChallanNo)
	if i < 0 {
		writeProblem(w, http.StatusBadRequest, fmt.Errorf(
			"there is no challan %s in the register to settle against", in.AgainstChallanNo))
		return
	}
	orig := data.Challans[i]
	if in.JobWorkerGSTIN == "" {
		in.JobWorkerGSTIN = orig.JobWorkerGSTIN
	}
	if in.JobWorkerName == "" {
		in.JobWorkerName = orig.JobWorkerName
	}
	if in.ItemDesc == "" {
		in.ItemDesc = orig.ItemDesc
	}
	if in.HSN == "" {
		in.HSN = orig.HSN
	}
	if in.UQC == "" {
		in.UQC = orig.UQC
	}
	if in.GoodsType == "" {
		in.GoodsType = orig.GoodsType
	}

	saved, err := a.store.addChallan(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}

	// Recording more than went out is allowed — it may be what happened — but
	// it is almost always a keying mistake, so say so.
	var warnings []string
	after := a.store.snapshot()
	if i := find(after.Challans, saved.AgainstChallanNo); i >= 0 {
		_, returns := jobwork.Split(after.Challans)
		if pos := jobwork.Outstanding(after.Challans[i], returns); pos.OverReturned {
			warnings = append(warnings, fmt.Sprintf(
				"%s %s more has now come back against challan %s than went out on it — check the quantities",
				jobwork.FormatQty(pos.ExcessQty), after.Challans[i].UQC, after.Challans[i].No))
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"challan": saved, "warnings": warnings})
}

// deleteChallan removes a challan from the register.
func (a *api) deleteChallan(w http.ResponseWriter, r *http.Request) {
	no := strings.TrimSpace(r.URL.Query().Get("no"))
	if no == "" {
		writeProblem(w, http.StatusBadRequest, errors.New("which challan? pass ?no=<challan number>"))
		return
	}
	cascade := r.URL.Query().Get("cascade") == "1"

	removed, err := a.store.deleteChallan(no, cascade)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

// putSettings saves the principal's details and the statutory figures.
func (a *api) putSettings(w http.ResponseWriter, r *http.Request) {
	var in settings
	if !readJSON(w, r, &in) {
		return
	}
	saved, err := a.store.putSettings(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": saved})
}

// getFiling answers with the figures for one filing period.
func (a *api) getFiling(w http.ResponseWriter, r *http.Request) {
	filing, _, err := a.filing(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, filing)
}

// getFilingJSONFile answers with the whole period — every line, every return
// and the roll-up — as a JSON file to keep with the filing.
func (a *api) getFilingJSONFile(w http.ResponseWriter, r *http.Request) {
	filing, _, err := a.filing(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	body, err := json.MarshalIndent(filing, "", "  ")
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err)
		return
	}
	name := "itc04-" + safeFileName(filing.Period.Label) + ".json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	_, _ = w.Write(append(body, '\n'))
}

// getFilingCSV answers with the same figures as a CSV file, one row per challan
// of goods sent out in the period.
func (a *api) getFilingCSV(w http.ResponseWriter, r *http.Request) {
	filing, _, err := a.filing(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	name := "itc04-" + safeFileName(filing.Period.Label) + ".csv"
	writeCSV(w, name, jobwork.FilingCSVHeader(), jobwork.FilingCSVRows(filing))
}

// getRegisterCSV answers with the whole register as a CSV file. Importing that
// file back gives the same register, so it is also the import template.
func (a *api) getRegisterCSV(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()
	writeCSV(w, "job-work-challans.csv", jobwork.ChallanCSVHeader(), jobwork.ChallanCSVRows(data.Challans))
}

// getTemplateCSV answers with an empty file carrying only the column headings.
func (a *api) getTemplateCSV(w http.ResponseWriter, r *http.Request) {
	writeCSV(w, "job-work-challans-template.csv", jobwork.ChallanCSVHeader(), nil)
}

// postImport adds the rows of a CSV file to the register.
func (a *api) postImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBody)
	defer r.Body.Close()

	reader := csv.NewReader(r.Body)
	reader.FieldsPerRecord = -1 // rows may be ragged; short rows are reported per row
	reader.TrimLeadingSpace = true

	rows, err := reader.ReadAll()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Errorf("this does not read as a CSV file: %w", err))
		return
	}

	parsed, rowErrors := jobwork.ParseChallanCSV(rows)
	result, err := a.store.importChallans(parsed, rowErrors)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// filing resolves the period asked for and prepares its figures.
func (a *api) filing(r *http.Request) (jobwork.Filing, settings, error) {
	data := a.store.snapshot()
	rules := data.Settings.Rules.Normalise()

	period, err := resolvePeriod(r, rules)
	if err != nil {
		return jobwork.Filing{}, data.Settings, err
	}
	asOf, err := dateParam(r, "asOf", jobwork.Today())
	if err != nil {
		return jobwork.Filing{}, data.Settings, err
	}
	return rules.Filing(data.Challans, period, asOf, data.Settings.TurnoverINR), data.Settings, nil
}

// resolvePeriod reads a filing period from the query: either a statutory period
// by frequency, financial year and index, or a span of dates.
func resolvePeriod(r *http.Request, rules jobwork.Rules) (jobwork.Period, error) {
	q := r.URL.Query()

	if from := strings.TrimSpace(q.Get("from")); from != "" {
		fromDate, err := jobwork.ParseDate(from)
		if err != nil {
			return jobwork.Period{}, fmt.Errorf("from: %w", err)
		}
		toDate, err := jobwork.ParseDate(q.Get("to"))
		if err != nil {
			return jobwork.Period{}, fmt.Errorf("to: %w", err)
		}
		return jobwork.CustomPeriod(fromDate, toDate)
	}

	freq := jobwork.Frequency(strings.TrimSpace(q.Get("freq")))
	if freq == "" {
		freq = jobwork.FreqQuarterly
	}
	fyStart := jobwork.FinancialYearStart(jobwork.Today())
	if v := strings.TrimSpace(q.Get("fy")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2017 || n > 2100 {
			return jobwork.Period{}, fmt.Errorf("fy %q should be the year a financial year starts in, such as 2024", v)
		}
		fyStart = n
	}
	index := 1
	if v := strings.TrimSpace(q.Get("index")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return jobwork.Period{}, fmt.Errorf("index %q should be a number", v)
		}
		index = n
	}
	return jobwork.PeriodFor(freq, fyStart, index)
}

// periodOptions lists the periods a filing can be prepared for: this financial
// year and the four before it, at each frequency.
func periodOptions(rules jobwork.Rules, asOf jobwork.Date) []periodOption {
	current := jobwork.FinancialYearStart(asOf)
	var out []periodOption
	for fy := current; fy > current-5; fy-- {
		for _, freq := range []jobwork.Frequency{jobwork.FreqQuarterly, jobwork.FreqHalfYearly, jobwork.FreqAnnual} {
			for _, p := range jobwork.PeriodsIn(freq, fy) {
				out = append(out, periodOption{
					Frequency: p.Frequency,
					Index:     p.Index,
					FYStart:   fy,
					Label:     p.Label,
					From:      p.From,
					To:        p.To,
					DueDate:   rules.DueDate(p),
				})
			}
		}
	}
	return out
}

// figures reports the statutory figures in force and whether they are still the
// ones this build ships with.
func figures(rules jobwork.Rules) statutoryFigures {
	return statutoryFigures{
		InputsReturnYears:       rules.InputsReturnYears,
		CapitalGoodsReturnYears: rules.CapitalGoodsReturnYears,
		TurnoverThresholdINR:    rules.TurnoverThresholdINR,
		FilingDueDayOfMonth:     rules.FilingDueDayOfMonth,
		StatutoryDefaults:       rules == jobwork.DefaultRules(),
	}
}

// dateParam reads a date from the query, falling back to a default when it is
// not given.
func dateParam(r *http.Request, name string, fallback jobwork.Date) (jobwork.Date, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return fallback, nil
	}
	d, err := jobwork.ParseDate(v)
	if err != nil {
		return fallback, fmt.Errorf("%s: %w", name, err)
	}
	if d.IsZero() {
		return fallback, nil
	}
	return d, nil
}

// readJSON reads a JSON request body, answering with a problem and reporting
// false when it cannot.
func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Errorf("this request did not read as JSON: %w", err))
		return false
	}
	return true
}

// writeJSON answers with a JSON body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "could not prepare the answer", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeProblem answers with an error, splitting the several problems a form can
// have into a list the page can show against the fields.
func writeProblem(w http.ResponseWriter, status int, err error) {
	problems := strings.Split(err.Error(), "\n")
	writeJSON(w, status, map[string]any{
		"error":    strings.Join(problems, "; "),
		"problems": problems,
	})
}

// writeCSV answers with a CSV file for the browser to save.
func writeCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	out := csv.NewWriter(w)
	if err := out.Write(header); err != nil {
		return
	}
	for _, row := range rows {
		if err := out.Write(row); err != nil {
			return
		}
	}
	out.Flush()
}

// safeFileName reduces a period label to something safe to offer as a file
// name on any system.
func safeFileName(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "period"
	}
	return out
}
