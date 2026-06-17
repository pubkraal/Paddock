// Package tabular reads operator-supplied roster files (entry lists,
// accreditation rosters) into a uniform Sheet, behind a Parser interface with
// CSV and XLSX adapters selected by file extension (ADR-0015). Importers depend
// only on Parser and Sheet, never on the underlying csv/excelize libraries, so
// new formats are additive. The Column/Mapping helpers resolve a parsed header
// against required logical columns via synonym lists.
package tabular

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
)

// ErrUnsupportedFormat is returned by ParserFor for a filename whose extension
// matches no known adapter.
var ErrUnsupportedFormat = errors.New("tabular: unsupported file format")

// ErrEmpty is returned by Parse when the source has no header row.
var ErrEmpty = errors.New("tabular: no rows")

// Sheet is a parsed tabular file: the first row as Header, the remainder as
// Rows. Cells are untyped strings; rows may be ragged (shorter or longer than
// the header), which the importer's per-row validation handles, not the parser.
type Sheet struct {
	Header []string
	Rows   [][]string
}

// Parser turns the bytes of one tabular file into a Sheet.
type Parser interface {
	Parse(r io.Reader) (Sheet, error)
}

// ParserFor selects a Parser from a filename's extension: .csv and .xlsx are
// supported; anything else yields ErrUnsupportedFormat. CSV is the guaranteed
// path (ADR-0015).
func ParserFor(filename string) (Parser, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		return csvParser{}, nil
	case ".xlsx":
		return newXLSXParser(), nil
	default:
		return nil, ErrUnsupportedFormat
	}
}
