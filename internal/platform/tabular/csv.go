package tabular

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
)

// csvParser reads CSV via the stdlib. FieldsPerRecord is set to -1 so ragged
// rows parse rather than abort — raggedness is surfaced by the importer's
// per-row validation, not treated as a parse failure (ADR-0015).
type csvParser struct{}

func (csvParser) Parse(r io.Reader) (Sheet, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return Sheet{}, ErrEmpty
	}

	if err != nil {
		return Sheet{}, fmt.Errorf("tabular: read csv header: %w", err)
	}

	rows, err := reader.ReadAll()
	if err != nil {
		return Sheet{}, fmt.Errorf("tabular: read csv rows: %w", err)
	}

	return Sheet{Header: header, Rows: rows}, nil
}
