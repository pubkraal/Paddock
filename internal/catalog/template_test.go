package catalog_test

import (
	"errors"
	"testing"

	"github.com/pubkraal/paddock/internal/catalog"
)

func TestTemplates_AreThreeKnownKeys(t *testing.T) {
	t.Parallel()

	got := catalog.Templates()

	wantKeys := []string{"sprint", "endurance", "rally"}
	if len(got) != len(wantKeys) {
		t.Fatalf("Templates() returned %d, want %d", len(got), len(wantKeys))
	}

	for i, key := range wantKeys {
		if got[i].Key != key {
			t.Errorf("Templates()[%d].Key = %q, want %q", i, got[i].Key, key)
		}

		if got[i].Label == "" || got[i].Blurb == "" {
			t.Errorf("template %q has empty Label/Blurb", key)
		}
	}
}

func TestTemplateByKey_SessionSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want []catalog.SessionType
	}{
		{
			key: "sprint",
			want: []catalog.SessionType{
				catalog.SessionPractice,
				catalog.SessionQualifying,
				catalog.SessionRace,
				catalog.SessionRace,
				catalog.SessionPodium,
			},
		},
		{
			key: "endurance",
			want: []catalog.SessionType{
				catalog.SessionQualifying,
				catalog.SessionQualifying,
				catalog.SessionWarmup,
				catalog.SessionRace,
				catalog.SessionPodium,
				catalog.SessionPaddock,
			},
		},
		{
			key:  "rally",
			want: []catalog.SessionType{catalog.SessionRace},
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			tmpl, err := catalog.TemplateByKey(tt.key)
			if err != nil {
				t.Fatalf("TemplateByKey(%q): %v", tt.key, err)
			}

			if len(tmpl.Sessions) != len(tt.want) {
				t.Fatalf("%s has %d sessions, want %d", tt.key, len(tmpl.Sessions), len(tt.want))
			}

			for i, spec := range tmpl.Sessions {
				if spec.Type != tt.want[i] {
					t.Errorf("%s session[%d].Type = %q, want %q", tt.key, i, spec.Type, tt.want[i])
				}

				if spec.Name == "" {
					t.Errorf("%s session[%d] has empty Name", tt.key, i)
				}

				if !spec.Type.Valid() {
					t.Errorf("%s session[%d].Type %q is not valid", tt.key, i, spec.Type)
				}
			}
		})
	}
}

func TestTemplateByKey_Unknown(t *testing.T) {
	t.Parallel()

	if _, err := catalog.TemplateByKey("le-mans"); !errors.Is(err, catalog.ErrUnknownTemplate) {
		t.Fatalf("TemplateByKey(unknown) err = %v, want ErrUnknownTemplate", err)
	}
}
