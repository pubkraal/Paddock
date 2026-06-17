package tabular_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/pubkraal/paddock/internal/platform/tabular"
	"github.com/xuri/excelize/v2"
)

func TestParserFor_DispatchesByExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		wantErr  error
	}{
		{"csv lower", "entrylist.csv", nil},
		{"csv upper", "ENTRYLIST.CSV", nil},
		{"xlsx", "entrylist.xlsx", nil},
		{"xlsx mixed case", "Roster.XLSX", nil},
		{"path with dirs", "/tmp/uploads/roster.csv", nil},
		{"unsupported xls", "legacy.xls", tabular.ErrUnsupportedFormat},
		{"unsupported none", "noext", tabular.ErrUnsupportedFormat},
		{"unsupported txt", "notes.txt", tabular.ErrUnsupportedFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := tabular.ParserFor(tt.filename)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParserFor(%q) err = %v, want %v", tt.filename, err, tt.wantErr)
			}

			if tt.wantErr == nil && p == nil {
				t.Fatalf("ParserFor(%q) returned nil parser without error", tt.filename)
			}
		})
	}
}

func TestParse_CSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         string
		wantHeader []string
		wantRows   [][]string
		wantErr    bool
	}{
		{
			name:       "happy path",
			in:         "Car,Team,Class\n72,AMG Landgraf,SP9\n27,Lionspeed,GT3",
			wantHeader: []string{"Car", "Team", "Class"},
			wantRows:   [][]string{{"72", "AMG Landgraf", "SP9"}, {"27", "Lionspeed", "GT3"}},
		},
		{
			name:       "ragged rows tolerated",
			in:         "Car,Team,Class\n72,AMG Landgraf\n27,Lionspeed,GT3,extra",
			wantHeader: []string{"Car", "Team", "Class"},
			wantRows:   [][]string{{"72", "AMG Landgraf"}, {"27", "Lionspeed", "GT3", "extra"}},
		},
		{
			name:       "quoted fields with commas",
			in:         "Car,Team\n72,\"AMG Landgraf, Team A\"",
			wantHeader: []string{"Car", "Team"},
			wantRows:   [][]string{{"72", "AMG Landgraf, Team A"}},
		},
		{
			name:       "header only",
			in:         "Car,Team,Class",
			wantHeader: []string{"Car", "Team", "Class"},
			wantRows:   nil,
		},
		{
			name:    "empty input",
			in:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := tabular.ParserFor("file.csv")
			if err != nil {
				t.Fatalf("ParserFor: %v", err)
			}

			sheet, err := p.Parse(strings.NewReader(tt.in))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = nil error, want error", tt.in)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}

			assertStrings(t, "header", sheet.Header, tt.wantHeader)
			assertRows(t, sheet.Rows, tt.wantRows)
		})
	}
}

func TestParse_XLSX(t *testing.T) {
	t.Parallel()

	data := newXLSX(t, "Sheet1", [][]string{
		{"Name", "Tier", "Email"},
		{"S. Bauer", "media", "s.bauer@nls-media.de"},
		{"P. Iredi", "sponsor", "p.iredi@pirelli.de"},
	})

	p, err := tabular.ParserFor("roster.xlsx")
	if err != nil {
		t.Fatalf("ParserFor: %v", err)
	}

	sheet, err := p.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	assertStrings(t, "header", sheet.Header, []string{"Name", "Tier", "Email"})
	assertRows(t, sheet.Rows, [][]string{
		{"S. Bauer", "media", "s.bauer@nls-media.de"},
		{"P. Iredi", "sponsor", "p.iredi@pirelli.de"},
	})
}

func TestParse_XLSX_Empty(t *testing.T) {
	t.Parallel()

	data := newXLSX(t, "Sheet1", nil)

	p, err := tabular.ParserFor("empty.xlsx")
	if err != nil {
		t.Fatalf("ParserFor: %v", err)
	}

	if _, err := p.Parse(bytes.NewReader(data)); err == nil {
		t.Fatal("Parse(empty xlsx) = nil error, want error")
	}
}

func TestParse_CSV_MalformedBodyRow(t *testing.T) {
	t.Parallel()

	// Header parses; a body row with a bare quote inside an unquoted field is a
	// hard CSV parse error (distinct from raggedness, which is tolerated).
	p, err := tabular.ParserFor("file.csv")
	if err != nil {
		t.Fatalf("ParserFor: %v", err)
	}

	if _, err := p.Parse(strings.NewReader("Car,Team\n72,AMG \"Landgraf\" Team")); err == nil {
		t.Fatal("Parse(malformed body) = nil error, want error")
	}
}

func TestParse_XLSX_RowsError(t *testing.T) {
	t.Parallel()

	// The workbook opens cleanly; the row read fails (seam) — exercises the
	// read-error path excelize itself will not surface for an openable file.
	data := newXLSX(t, "Sheet1", [][]string{{"a"}})

	p := tabular.NewXLSXParserWithFailingRows(errBoom)

	if _, err := p.Parse(bytes.NewReader(data)); err == nil {
		t.Fatal("Parse(rows error) = nil error, want error")
	}
}

func TestParse_XLSX_Corrupt(t *testing.T) {
	t.Parallel()

	p, err := tabular.ParserFor("bad.xlsx")
	if err != nil {
		t.Fatalf("ParserFor: %v", err)
	}

	if _, err := p.Parse(strings.NewReader("not a real xlsx file")); err == nil {
		t.Fatal("Parse(corrupt xlsx) = nil error, want error")
	}
}

func TestParse_CSV_ReadError(t *testing.T) {
	t.Parallel()

	p, err := tabular.ParserFor("file.csv")
	if err != nil {
		t.Fatalf("ParserFor: %v", err)
	}

	if _, err := p.Parse(errReader{}); err == nil {
		t.Fatal("Parse(errReader) = nil error, want error")
	}
}

func TestParse_XLSX_ReadError(t *testing.T) {
	t.Parallel()

	p, err := tabular.ParserFor("file.xlsx")
	if err != nil {
		t.Fatalf("ParserFor: %v", err)
	}

	if _, err := p.Parse(errReader{}); err == nil {
		t.Fatal("Parse(errReader) = nil error, want error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }

var errBoom = errors.New("boom")

func newXLSX(t *testing.T, sheet string, rows [][]string) []byte {
	t.Helper()

	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })

	if sheet != "Sheet1" {
		if _, err := f.NewSheet(sheet); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
	}

	for r, row := range rows {
		for c, val := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}

			if err := f.SetCellStr(sheet, cell, val); err != nil {
				t.Fatalf("SetCellStr: %v", err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	return buf.Bytes()
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s len = %d, want %d (%q vs %q)", label, len(got), len(want), got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

func assertRows(t *testing.T, got, want [][]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("rows len = %d, want %d (%v vs %v)", len(got), len(want), got, want)
	}

	for i := range want {
		assertStrings(t, "row", got[i], want[i])
	}
}
