# Job-work challan tracker and ITC-04 reconciler for manufacturers

A free, single-binary tool for small manufacturers in **India** who send goods out for
job work. It keeps the delivery-challan register, works out when each challan falls due,
shows what is still with each job worker, flags the quantities that have become a
**deemed supply**, and prepares the figures for a **Form GST ITC-04** filing period.

Part of the **Open MSME** family of free tools for micro and small businesses in India
and the Philippines.

## What it does

- Records every delivery challan for goods **sent out**, and every quantity **received
  back** against it — partial returns, in as many instalments as it takes.
- Works out each challan's **return deadline**: one year for inputs, three years for
  capital goods, by calendar arithmetic from the challan date.
- Judges each challan as at a date you choose: **Returned**, **Pending** with days left,
  or **Overdue** with days overdue and the taxable value now at risk.
- Pro-rates the challan's taxable value by the quantity still out, giving the
  **deemed supply value** — the value tax would fall on.
- Shows **outstanding quantity per job worker** and a work list of everything overdue or
  falling due inside a window you set, worst first.
- Rolls a **filing period** (quarter, half year or financial year) up by job worker and
  exports it as **JSON and CSV** for Form ITC-04, with the due date.
- **Imports and exports** the register as CSV: columns in any order, dates as
  `2024-10-05` or `05/10/2024`, amounts with rupee signs and lakh commas. A row that
  cannot be used is reported by its line number and the rest still come in.

## Who it's for

Small manufacturers who send goods out for job-work: fabricators, machine shops, textile
and garment units, plating and heat-treatment shops, printers, food processors — anyone
who moves material to another GSTIN under a delivery challan and has to report it. It is
for the **principal**, not the job worker: the return deadlines and the ITC-04 obligation
sit with whoever sent the goods out.

## The rule it implements

**Section 143 of the CGST Act, 2017** lets a registered person (the principal) send
inputs or capital goods to a job worker without paying tax, provided they are brought
back — or supplied from the job worker's premises — within **one year** for inputs and
**three years** for capital goods (section 143(1)(a)). If they are not, sections 143(3)
and 143(4) treat the goods as having been **supplied to the job worker on the day they
were sent out**, and tax falls due from that date with interest. The proviso to section
143(1) puts **moulds and dies, jigs and fixtures, and tools** outside that time limit.

**Rule 45 of the CGST Rules, 2017** requires the goods to move under a **delivery
challan** and the challans of a period to be reported in **Form GST ITC-04**. Rule 45(3),
as substituted with effect from **1 October 2021**, sets the period by turnover:

| Annual aggregate turnover (preceding FY) | ITC-04 period | Due |
| --- | --- | --- |
| above ₹5 crore | half-yearly (Apr–Sep, Oct–Mar) | 25th of the following month |
| up to ₹5 crore | once a financial year | 25 April following |

Before 1 October 2021 every principal filed **quarterly**, which is why a quarterly period
is still offered — it is also a useful way to review a long one.

### The statutory figures, in one place

All four live as named constants in `internal/jobwork/rules.go` and are editable under
**Settings**, so the tool survives the next amendment:

| Constant | Value | Source |
| --- | --- | --- |
| `InputsReturnYears` | 1 year | CGST Act s.143(1)(a) |
| `CapitalGoodsReturnYears` | 3 years | CGST Act s.143(1)(a) |
| `HalfYearlyFilingTurnoverThresholdINR` | ₹5,00,00,000 | CGST Rules r.45(3) |
| `FilingDueDayOfMonth` | 25th | CGST Rules r.45(3) |

**Confirm these against the current CGST Act and Rules, a CBIC notification, or your own
tax adviser before you rely on them.** Periods, thresholds and due dates change, and a
figure that was right when this was written may not be right today.

## Install and run

One binary, the Go standard library and nothing else — no database, no third-party
modules, no build step for the interface.

```sh
git clone https://github.com/sreenivas-sadhu-prabhakara/jobwork-itc04-reconciler.git
cd jobwork-itc04-reconciler
go build -o jobwork-itc04-reconciler .
./jobwork-itc04-reconciler
```

Then open **http://127.0.0.1:8080**. Or run it without building, with `go run .`.

| Flag | Default | What it is |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | address to serve the interface on |
| `-data` | `./jobwork-itc04-reconciler-data.json` | file to keep the challan register in |

```sh
./jobwork-itc04-reconciler -addr 127.0.0.1:9000 -data ~/accounts/job-work-2024-25.json
```

It binds to `127.0.0.1` — the machine it runs on, not the network. Stop it with Ctrl-C.

## Where the data is stored

One JSON file on your own machine: the challans, your own details and the statutory
figures in force. Nothing is sent anywhere — no outbound network calls, no telemetry, no
fonts or scripts from the internet, and no connection to the GST portal.

The file is rewritten whole after every change, atomically: a temporary file beside the
register, flushed, then renamed over the old one, so an interrupted save leaves the
previous register intact rather than half of one. It is written readable only by you, in
plain indented JSON, and the CSV export round-trips back through the importer — so you
are never locked in. Back it up by copying the file.

## The rules, and their tests

The rules live in `internal/jobwork` as pure functions over records, with table-driven
tests — including the cases that are easy to get wrong:

```sh
go test ./... -count=1
```

- `Deadline(sentDate, goodsType)` — calendar arithmetic, tested on leap days
  (29 February 2024 plus one year is 1 March 2025) and on 28 February into a leap year.
- `Outstanding(sent, returns)` — partial returns in instalments, instalments that only
  add up exactly in decimal, returns against another challan, and over-return (held at
  zero and flagged, never negative).
- `Status(challan, returns, asOf)` — Returned / Pending with days left / Overdue with
  days overdue, tested on the boundary: the deadline date itself is still within time.
- `DeemedSupplyValue` — pro-rating by quantity, rounded to paise.
- Period arithmetic — Indian financial-year quarters and halves, the quarter that ends in
  December falling due on 25 January, filing frequency at and either side of the ₹5 crore
  threshold.

## Scope and accuracy

This is a register and a calculator. It is honest about what it does not do:

- It **does not file anything** and has no portal integration. It prepares the figures.
- It computes the **value** a deemed supply would fall on, not the **tax**: no rate is
  applied, no interest is computed, and no place of supply is decided.
- It applies the three-year period to everything marked as capital goods. Moulds and
  dies, jigs and fixtures, and tools have no time limit — keep them out of the register.
- The job worker's **waste and scrap** (section 143(5)) is a supply in its own right and
  is outside this register.
- GSTINs are checked for the 15-character **shape** only; the check digit is not
  recomputed, so one that passes may still not exist.
- Quantities are added only within a unit of measure; where a total would mix units, the
  tool says so rather than implying that kilograms and pieces add up.
- Form ITC-04's own table numbering is not reproduced, because it has differed between
  versions of the form. Read the exported figures across onto whichever version the
  portal is serving.

It cannot know what you did not record. Check every figure against your challans before
you file.

MIT licensed.
