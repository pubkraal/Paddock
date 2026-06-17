package catalog_test

import (
	"errors"
	"testing"

	"github.com/pubkraal/paddock/internal/catalog"
)

func TestParseSessionType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    catalog.SessionType
		wantErr bool
	}{
		{"practice", catalog.SessionPractice, false},
		{"qualifying", catalog.SessionQualifying, false},
		{"race", catalog.SessionRace, false},
		{"warmup", catalog.SessionWarmup, false},
		{"podium", catalog.SessionPodium, false},
		{"paddock", catalog.SessionPaddock, false},
		{"", "", true},
		{"unknown", "", true},
		{"Race", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := catalog.ParseSessionType(tt.in)
			if tt.wantErr {
				if !errors.Is(err, catalog.ErrInvalidSessionType) {
					t.Fatalf("ParseSessionType(%q) err = %v, want ErrInvalidSessionType", tt.in, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseSessionType(%q): %v", tt.in, err)
			}

			if got != tt.want {
				t.Errorf("ParseSessionType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSessionTypeValid(t *testing.T) {
	t.Parallel()

	if !catalog.SessionRace.Valid() {
		t.Error("SessionRace.Valid() = false, want true")
	}

	if catalog.SessionType("nope").Valid() {
		t.Error("invalid SessionType.Valid() = true, want false")
	}
}

func TestParseTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    catalog.Tier
		wantErr bool
	}{
		{"media", catalog.TierMedia, false},
		{"sponsor", catalog.TierSponsor, false},
		{"team", catalog.TierTeam, false},
		{"internal", catalog.TierInternal, false},
		{"MEDIA", catalog.TierMedia, false},
		{" media ", catalog.TierMedia, false},
		{"", "", true},
		{"vip", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := catalog.ParseTier(tt.in)
			if tt.wantErr {
				if !errors.Is(err, catalog.ErrInvalidTier) {
					t.Fatalf("ParseTier(%q) err = %v, want ErrInvalidTier", tt.in, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseTier(%q): %v", tt.in, err)
			}

			if got != tt.want {
				t.Errorf("ParseTier(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEventIsLive(t *testing.T) {
	t.Parallel()

	if (catalog.Event{Status: catalog.EventLive}).IsLive() != true {
		t.Error("live event IsLive() = false, want true")
	}

	if (catalog.Event{Status: catalog.EventDraft}).IsLive() != false {
		t.Error("draft event IsLive() = true, want false")
	}
}
