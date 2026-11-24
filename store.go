package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sreenivas-sadhu-prabhakara/jobwork-itc04-reconciler/internal/jobwork"
)

// fileFormatVersion is written into the data file so a later version of the
// tool can tell what it is reading.
const fileFormatVersion = 1

// defaultHorizonDays is how far ahead the dashboard looks for challans about to
// fall due, until the user changes it.
const defaultHorizonDays = 30

// settings are the principal's own details and the statutory figures in force.
type settings struct {
	PrincipalName  string        `json:"principalName"`
	PrincipalGSTIN string        `json:"principalGstin"`
	TurnoverINR    float64       `json:"annualAggregateTurnoverInr"`
	HorizonDays    int           `json:"horizonDays"`
	Rules          jobwork.Rules `json:"rules"`
}

// normalise fills in anything missing with a sensible default.
func (s settings) normalise() settings {
	s.PrincipalName = strings.TrimSpace(s.PrincipalName)
	s.PrincipalGSTIN = strings.ToUpper(strings.TrimSpace(s.PrincipalGSTIN))
	if s.HorizonDays <= 0 {
		s.HorizonDays = defaultHorizonDays
	}
	if s.TurnoverINR < 0 {
		s.TurnoverINR = 0
	}
	s.Rules = s.Rules.Normalise()
	return s
}

// fileData is the whole contents of the data file.
type fileData struct {
	Version  int               `json:"version"`
	Settings settings          `json:"settings"`
	Challans []jobwork.Challan `json:"challans"`
}

// store keeps the register in memory and writes the whole file out after every
// change. A job-work register is small — a few thousand challans at most — so
// rewriting it is both simpler and safer than updating it in place.
type store struct {
	mu   sync.RWMutex
	path string
	data fileData
}

// openStore reads the data file, or starts an empty register if there is no
// file yet. It does not create the file until something is saved.
func openStore(path string) (*store, error) {
	s := &store{
		path: path,
		data: fileData{Version: fileFormatVersion, Settings: settings{}.normalise()},
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return s, nil
	}

	var loaded fileData
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, fmt.Errorf("reading %s: %w (move the file aside to start again)", path, err)
	}
	if loaded.Version > fileFormatVersion {
		return nil, fmt.Errorf("%s was written by a newer version of this tool (format %d, this build reads %d)",
			path, loaded.Version, fileFormatVersion)
	}
	loaded.Version = fileFormatVersion
	loaded.Settings = loaded.Settings.normalise()
	for i, c := range loaded.Challans {
		loaded.Challans[i] = c.Normalise()
	}
	s.data = loaded
	return s, nil
}

// snapshot is a copy of the register that a reader can use without holding the
// lock.
func (s *store) snapshot() fileData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.data
	out.Challans = append([]jobwork.Challan(nil), s.data.Challans...)
	return out
}

// change applies a change to the register and saves the file. If saving fails
// the change is rolled back, so what is in memory always matches what is on
// disk.
func (s *store) change(apply func(d *fileData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := append([]jobwork.Challan(nil), s.data.Challans...)
	beforeSettings := s.data.Settings

	if err := apply(&s.data); err != nil {
		s.data.Challans = before
		s.data.Settings = beforeSettings
		return err
	}
	sortChallans(s.data.Challans)
	if err := s.save(); err != nil {
		s.data.Challans = before
		s.data.Settings = beforeSettings
		return err
	}
	return nil
}

// save writes the whole register to the data file atomically: a temporary file
// in the same directory, flushed to disk, then renamed over the old one. A
// crash therefore leaves either the old file or the new one, never half of
// either.
func (s *store) save() error {
	s.data.Version = fileFormatVersion

	body, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("preparing the register: %w", err)
	}
	body = append(body, '\n')

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // a no-op once the rename has succeeded

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replacing %s: %w", s.path, err)
	}
	return nil
}

// sortChallans keeps the register in a stable order: newest challan first, and
// within a date by challan number, so the file and every list read the same way
// every time.
func sortChallans(all []jobwork.Challan) {
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Date.Equal(all[j].Date) {
			return all[i].Date.After(all[j].Date)
		}
		return all[i].No < all[j].No
	})
}

// find returns the position of a challan by number, or -1.
func find(all []jobwork.Challan, no string) int {
	key := jobwork.Key(no)
	for i, c := range all {
		if c.Key() == key {
			return i
		}
	}
	return -1
}

// addChallan puts one challan into the register, refusing a number that is
// already there and, for a return, one that settles against a challan that is
// not.
func (s *store) addChallan(c jobwork.Challan) (jobwork.Challan, error) {
	c = c.Normalise()
	if err := c.Validate(); err != nil {
		return c, err
	}
	err := s.change(func(d *fileData) error {
		if find(d.Challans, c.No) >= 0 {
			return fmt.Errorf("challan %s is already in the register", c.No)
		}
		if c.Direction == jobwork.DirectionReturned {
			i := find(d.Challans, c.AgainstChallanNo)
			if i < 0 {
				return fmt.Errorf("there is no challan %s in the register to settle against", c.AgainstChallanNo)
			}
			if err := jobwork.ValidateAgainst(c, d.Challans[i]); err != nil {
				return err
			}
		}
		d.Challans = append(d.Challans, c)
		return nil
	})
	return c, err
}

// deleteChallan removes a challan. A challan that goods have come back against
// is only removed with cascade set, which removes those returns too, so a
// delete can never leave a return settling against nothing.
func (s *store) deleteChallan(no string, cascade bool) (int, error) {
	removed := 0
	err := s.change(func(d *fileData) error {
		i := find(d.Challans, no)
		if i < 0 {
			return fmt.Errorf("there is no challan %s in the register", no)
		}
		target := d.Challans[i]
		key := target.Key()

		var dependents []string
		if target.Direction == jobwork.DirectionSent {
			for _, c := range d.Challans {
				if c.Direction == jobwork.DirectionReturned && c.AgainstKey() == key {
					dependents = append(dependents, c.No)
				}
			}
		}
		if len(dependents) > 0 && !cascade {
			return fmt.Errorf("challan %s has %d return(s) against it (%s) — remove those first, or confirm removing them with it",
				target.No, len(dependents), strings.Join(dependents, ", "))
		}

		kept := make([]jobwork.Challan, 0, len(d.Challans))
		for _, c := range d.Challans {
			if c.Key() == key || (cascade && c.Direction == jobwork.DirectionReturned && c.AgainstKey() == key) {
				removed++
				continue
			}
			kept = append(kept, c)
		}
		d.Challans = kept
		return nil
	})
	return removed, err
}

// putSettings replaces the principal's details and the statutory figures.
func (s *store) putSettings(next settings) (settings, error) {
	next = next.normalise()
	if next.PrincipalGSTIN != "" && !jobwork.LooksLikeGSTIN(next.PrincipalGSTIN) {
		return next, fmt.Errorf("your own GSTIN %q is not in the 15-character GSTIN shape", next.PrincipalGSTIN)
	}
	err := s.change(func(d *fileData) error {
		d.Settings = next
		return nil
	})
	return next, err
}

// importResult reports what an imported file did to the register.
type importResult struct {
	Added    int                `json:"added"`
	Skipped  int                `json:"skipped"`
	Errors   []jobwork.RowError `json:"errors"`
	Warnings []jobwork.RowError `json:"warnings"`
}

// importChallans adds the rows of a parsed CSV file to the register. A row that
// cannot be used is reported and the rest are still added. A return whose
// challan number matches nothing is added with a warning rather than dropped,
// so nothing is lost silently — the dashboard then lists it as unmatched.
func (s *store) importChallans(rows []jobwork.ParsedRow, rowErrors []jobwork.RowError) (importResult, error) {
	result := importResult{Errors: rowErrors}

	err := s.change(func(d *fileData) error {
		existing := make(map[string]bool, len(d.Challans)+len(rows))
		sentKeys := make(map[string]bool, len(d.Challans)+len(rows))
		for _, c := range d.Challans {
			existing[c.Key()] = true
			if c.Direction == jobwork.DirectionSent {
				sentKeys[c.Key()] = true
			}
		}
		for _, row := range rows {
			if row.Challan.Direction == jobwork.DirectionSent {
				sentKeys[row.Challan.Key()] = true
			}
		}

		for _, row := range rows {
			c := row.Challan
			if existing[c.Key()] {
				result.Skipped++
				result.Errors = append(result.Errors, jobwork.RowError{
					Line:    row.Line,
					Message: fmt.Sprintf("challan %s is already in the register", c.No),
				})
				continue
			}
			if c.Direction == jobwork.DirectionReturned && !sentKeys[c.AgainstKey()] {
				result.Warnings = append(result.Warnings, jobwork.RowError{
					Line:    row.Line,
					Message: fmt.Sprintf("%s settles against challan %s, which is not in the register — it will reduce nothing until that challan is added", c.No, c.AgainstChallanNo),
				})
			}
			existing[c.Key()] = true
			d.Challans = append(d.Challans, c)
			result.Added++
		}
		return nil
	})
	return result, err
}
