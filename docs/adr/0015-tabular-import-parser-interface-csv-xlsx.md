# ADR-0015: Tabular Import Behind a Parser Interface — CSV and XLSX Adapters

**Status**: Accepted
**Date**: 2026-06-17

## Context

Phase 2 imports two kinds of operator-supplied file: a race **entry list** (Car № → Team → Driver lineup → Class → livery refs) and an **accreditation** roster (person → tier → validity window → credential ref). These arrive in whatever the championship's existing tooling exports — most commonly CSV, very commonly `.xlsx` straight out of Excel or Google Sheets. The acceptance demo imports a 40-car entry list and a 120-person accreditation file *in the formats the operator already has* (PLAN §6), and open decision O5 explicitly defers the exact Excel scope to this phase, noting "CSV is the guaranteed path; xlsx via a maintained Go lib."

The import flow — preview, column mapping, per-row validation, error report, persist — is identical regardless of file format. Only the bytes-to-rows step differs. We do not want the entry-list and accreditation importers to know or care whether a file was CSV or XLSX.

## Decision

Introduce one platform package, `internal/platform/tabular`, with a single abstraction:

```go
type Sheet struct {
    Header []string
    Rows   [][]string
}

type Parser interface {
    Parse(io.Reader) (Sheet, error)
}

func ParserFor(filename string) (Parser, error) // dispatch by extension
```

Two adapters implement `Parser`:

- **`csvParser`** over the stdlib `encoding/csv`, with `FieldsPerRecord = -1` so ragged rows parse rather than abort — raggedness is a *row-level* concern surfaced by the importer's validation, not a parse failure.
- **`xlsxParser`** over `github.com/xuri/excelize/v2` — the maintained Go xlsx library (O5) — reading the first worksheet's rows.

`ParserFor` selects the adapter by file extension (`.csv` → CSV, `.xlsx` → XLSX; anything else → `ErrUnsupportedFormat`). The importers in `internal/catalog` depend only on `tabular.Parser` and `tabular.Sheet`; they never import excelize or `encoding/csv`. A `Mapping` helper in the same package resolves a parsed header against required logical columns via synonym lists (e.g. `car_no` ← `{"car", "№", "no", "number", "car №"}`) and reports any unmapped required column — this backs the wizard's "Car № → col A · mapped" panel and the accreditation column-mapping step.

CSV remains the guaranteed path: it has no third-party dependency and is the format we test most exhaustively. XLSX is a convenience adapter behind the same seam.

## Alternatives Considered

### CSV only in Phase 2, defer XLSX entirely

**Pros:**
- Zero new dependencies; smallest surface to test to 100%.

**Cons:**
- Operators overwhelmingly hand over `.xlsx`; forcing a "save as CSV" step undercuts the same-day-onboarding promise that is the entire point of the phase.

**Why rejected**: the product decision this session is to accept both formats now. The `Parser` seam makes XLSX cheap to add and keeps it isolated, so the cost is contained.

### A single parser that sniffs and branches internally

**Pros:**
- One type, no dispatch function.

**Cons:**
- Mixes two unrelated parsing libraries in one type, violates single-responsibility, and makes each format harder to test in isolation.

**Why rejected**: two small adapters behind an interface is the cleaner, more testable shape and matches the project's "accept interfaces, return structs / interface at the consumer" conventions.

## Consequences

### Positive

- Entry-list and accreditation importers are format-agnostic; adding `.xls`, ODS, or a Google-Sheets pull later is an additive adapter plus a `ParserFor` case — no importer change.
- Each adapter is unit-tested in isolation: CSV happy/ragged/empty/quoted; XLSX via an in-memory excelize fixture built in the test (no disk), so no testdata binaries are committed.
- excelize is the only new third-party dependency, confined to one file behind the interface.

### Negative

- excelize is a non-trivial dependency (it parses the full OOXML zip container); it lives behind the seam so it can be swapped if it ever becomes a liability.
- The `Sheet` abstraction is intentionally lossy (strings only, first worksheet, no formulas/types) — sufficient for roster imports, not a general spreadsheet model.

### Neutral

- Format is determined by filename extension at upload, consistent with how the HTMX upload widget already labels the chosen file.
