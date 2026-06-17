package tabular_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pubkraal/paddock/internal/platform/tabular"
)

func TestMap_ResolvesSynonymsCaseInsensitively(t *testing.T) {
	t.Parallel()

	header := []string{"Car №", "Team Name", "Class", "Drivers"}
	cols := []tabular.Column{
		{Key: "car_no", Synonyms: []string{"car", "no", "number", "car №"}},
		{Key: "team", Synonyms: []string{"team", "team name", "entrant"}},
		{Key: "class", Synonyms: []string{"class", "category"}},
	}

	m, err := tabular.Map(header, cols)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	tests := []struct {
		key  string
		want int
	}{
		{"car_no", 0},
		{"team", 1},
		{"class", 2},
	}

	for _, tt := range tests {
		if got := m.Index(tt.key); got != tt.want {
			t.Errorf("Index(%q) = %d, want %d", tt.key, got, tt.want)
		}
	}
}

func TestMap_ReportsMissingRequiredColumns(t *testing.T) {
	t.Parallel()

	header := []string{"Car", "Team"}
	cols := []tabular.Column{
		{Key: "car_no", Synonyms: []string{"car"}},
		{Key: "team", Synonyms: []string{"team"}},
		{Key: "class", Synonyms: []string{"class", "category"}},
		{Key: "tier", Synonyms: []string{"tier"}},
	}

	_, err := tabular.Map(header, cols)
	if err == nil {
		t.Fatal("Map with missing columns = nil error, want error")
	}

	var missing *tabular.MissingColumnsError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want *MissingColumnsError", err)
	}

	assertStrings(t, "missing", missing.Keys, []string{"class", "tier"})

	if msg := missing.Error(); !strings.Contains(msg, "class") || !strings.Contains(msg, "tier") {
		t.Errorf("Error() = %q, want it to name class and tier", msg)
	}
}

func TestMapping_Value(t *testing.T) {
	t.Parallel()

	header := []string{"Car", "Team", "Class"}
	cols := []tabular.Column{
		{Key: "car_no", Synonyms: []string{"car"}},
		{Key: "team", Synonyms: []string{"team"}},
		{Key: "class", Synonyms: []string{"class"}},
	}

	m, err := tabular.Map(header, cols)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	row := []string{"72", "AMG Landgraf", "SP9"}

	if got := m.Value(row, "team"); got != "AMG Landgraf" {
		t.Errorf("Value(team) = %q, want %q", got, "AMG Landgraf")
	}

	// A short (ragged) row returns empty for a column past its end.
	short := []string{"72"}
	if got := m.Value(short, "class"); got != "" {
		t.Errorf("Value(class) on short row = %q, want empty", got)
	}
}

func TestMap_DuplicateHeaderUsesFirstMatch(t *testing.T) {
	t.Parallel()

	header := []string{"Email", "Email"}
	cols := []tabular.Column{{Key: "email", Synonyms: []string{"email"}}}

	m, err := tabular.Map(header, cols)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	if got := m.Index("email"); got != 0 {
		t.Errorf("Index(email) = %d, want 0 (first match)", got)
	}
}
