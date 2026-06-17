package catalog

// SessionSpec is one session a template scaffolds: its typed kind and display
// name. Ordinals are assigned positionally when the event is materialised.
type SessionSpec struct {
	Type SessionType
	Name string
}

// OnboardingTemplate is a named event blueprint (ADR-0014): picking it scaffolds
// the listed sessions on event creation. Definitions live in code, not the
// database — they are behaviour, table-test-covered and single-sourced.
type OnboardingTemplate struct {
	Key      string
	Label    string
	Blurb    string
	Sessions []SessionSpec
}

// templates is the ordered registry rendered as selectable cards in the wizard.
// The order is intentional (endurance is the design's default centrepiece, but
// sprint reads first as the simplest); tests pin each set.
var templates = []OnboardingTemplate{
	{
		Key:   "sprint",
		Label: "Sprint weekend",
		Blurb: "FP · Quali · 2 races · podium",
		Sessions: []SessionSpec{
			{SessionPractice, "Free Practice"},
			{SessionQualifying, "Qualifying"},
			{SessionRace, "Race 1"},
			{SessionRace, "Race 2"},
			{SessionPodium, "Podium"},
		},
	},
	{
		Key:   "endurance",
		Label: "Endurance",
		Blurb: "6 sessions · stint windows · multi-driver captioning ready",
		Sessions: []SessionSpec{
			{SessionQualifying, "Pre-Qualifying"},
			{SessionQualifying, "Top-30 Qualifying"},
			{SessionWarmup, "Warm-Up"},
			{SessionRace, "Race · 24h"},
			{SessionPodium, "Podium"},
			{SessionPaddock, "Paddock / Atmosphere"},
		},
	},
	{
		Key:   "rally",
		Label: "Rally",
		Blurb: "Stage-based · road sections (placeholder)",
		Sessions: []SessionSpec{
			{SessionRace, "Stage 1"},
		},
	},
}

// Templates returns the onboarding templates in display order.
func Templates() []OnboardingTemplate {
	return templates
}

// TemplateByKey returns the template with the given key, or ErrUnknownTemplate.
func TemplateByKey(key string) (OnboardingTemplate, error) {
	for _, t := range templates {
		if t.Key == key {
			return t, nil
		}
	}

	return OnboardingTemplate{}, ErrUnknownTemplate
}
