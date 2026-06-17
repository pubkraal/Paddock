package tabular

import (
	"fmt"
	"strings"
)

// Column is a required logical column and the header labels that may denote it.
// Synonyms are matched case-insensitively against trimmed header cells.
type Column struct {
	Key      string
	Synonyms []string
}

// MissingColumnsError reports required columns absent from a header. Keys is in
// the order the columns were requested.
type MissingColumnsError struct {
	Keys []string
}

func (e *MissingColumnsError) Error() string {
	return fmt.Sprintf("tabular: missing required columns: %s", strings.Join(e.Keys, ", "))
}

// Mapping resolves logical column keys to header indices for one file.
type Mapping struct {
	indices map[string]int
}

// Map resolves each requested Column against the header, returning a Mapping or
// a *MissingColumnsError listing every required column that no header matched.
// When a synonym matches multiple header cells the first (leftmost) wins.
func Map(header []string, cols []Column) (Mapping, error) {
	normalized := make([]string, len(header))
	for i, h := range header {
		normalized[i] = strings.ToLower(strings.TrimSpace(h))
	}

	indices := make(map[string]int, len(cols))

	var missing []string

	for _, col := range cols {
		idx, ok := matchColumn(normalized, col.Synonyms)
		if !ok {
			missing = append(missing, col.Key)

			continue
		}

		indices[col.Key] = idx
	}

	if len(missing) > 0 {
		return Mapping{}, &MissingColumnsError{Keys: missing}
	}

	return Mapping{indices: indices}, nil
}

func matchColumn(normalizedHeader, synonyms []string) (int, bool) {
	for i, h := range normalizedHeader {
		for _, syn := range synonyms {
			if h == strings.ToLower(strings.TrimSpace(syn)) {
				return i, true
			}
		}
	}

	return -1, false
}

// Index returns the header index a logical key resolved to.
func (m Mapping) Index(key string) int {
	return m.indices[key]
}

// Value returns the cell for a logical key in a row, or "" if the row is too
// short (ragged) to reach that column.
func (m Mapping) Value(row []string, key string) string {
	idx := m.indices[key]
	if idx < 0 || idx >= len(row) {
		return ""
	}

	return strings.TrimSpace(row[idx])
}
