package tabular

import (
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

// xlsxParser reads the first worksheet of an .xlsx file via excelize. Cells are
// returned as their displayed string values, matching the CSV adapter. getRows
// is a seam: it defaults to (*excelize.File).GetRows and is overridden in tests
// to exercise the read-error path, which excelize never surfaces for an
// otherwise-openable file.
type xlsxParser struct {
	getRows func(f *excelize.File, sheet string) ([][]string, error)
}

func newXLSXParser() xlsxParser {
	return xlsxParser{
		getRows: func(f *excelize.File, sheet string) ([][]string, error) {
			return f.GetRows(sheet)
		},
	}
}

func (p xlsxParser) Parse(r io.Reader) (Sheet, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return Sheet{}, fmt.Errorf("tabular: open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	rows, err := p.getRows(f, f.GetSheetName(0))
	if err != nil {
		return Sheet{}, fmt.Errorf("tabular: read xlsx rows: %w", err)
	}

	if len(rows) == 0 {
		return Sheet{}, ErrEmpty
	}

	return Sheet{Header: rows[0], Rows: rows[1:]}, nil
}
