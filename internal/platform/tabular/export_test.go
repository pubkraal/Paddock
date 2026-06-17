package tabular

import "github.com/xuri/excelize/v2"

// NewXLSXParserWithFailingRows builds an xlsx parser whose row read always
// fails, so the read-error path is covered without excelize cooperation (it
// never surfaces a read error for an openable file). The reader still must be a
// valid xlsx so OpenReader succeeds and the seam is reached.
func NewXLSXParserWithFailingRows(err error) Parser {
	return xlsxParser{
		getRows: func(*excelize.File, string) ([][]string, error) { return nil, err },
	}
}
