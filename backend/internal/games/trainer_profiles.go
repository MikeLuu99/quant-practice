package games

import (
	_ "embed"
	"encoding/json"
)

//go:embed trainer_profiles.json
var trainerProfilesJSON []byte

type TrainerProfile struct {
	SpecID        string   `json:"spec_id"`
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Modes         []string `json:"modes"`
	DurationMS    int64    `json:"duration_ms"`
	Rounds        int      `json:"rounds"`
	ScoreKind     string   `json:"score_kind"`
	Games         []string `json:"games"`
	Assessments   []string `json:"assessments"`
	Source        string   `json:"source"`
	Description   string   `json:"description"`
	RulesVersion  string   `json:"rules_version"`
	ContentOrigin string   `json:"content_origin"`
	Proposed      string   `json:"proposed"`
	ReviewedOn    string   `json:"reviewed_on"`
}

func TrainerProfiles() []TrainerProfile {
	var out []TrainerProfile
	if err := json.Unmarshal(trainerProfilesJSON, &out); err != nil {
		panic(err)
	}
	return out
}
func trainerProfile(id string) (TrainerProfile, bool) {
	for _, p := range TrainerProfiles() {
		if p.ID == id {
			return p, true
		}
	}
	return TrainerProfile{}, false
}

// TrainerFermiLibrary exposes prompts and units, never the evaluation reference.
func TrainerFermiLibrary() []map[string]any {
	out := []map[string]any{}
	for _, q := range fermiBank {
		out = append(out, map[string]any{"id": q.ID, "prompt": q.Prompt, "units": q.Units})
	}
	return out
}
