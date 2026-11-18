package games

import (
	_ "embed"
	"encoding/json"
)

//go:embed assessment_profiles.json
var assessmentProfileJSON []byte

type AssessmentProfile struct {
	SpecID            string   `json:"spec_id"`
	ID                string   `json:"id"`
	CompanyID         string   `json:"company_id"`
	Company           string   `json:"company"`
	Title             string   `json:"title"`
	Variant           string   `json:"variant"`
	QuestionCount     int      `json:"question_count"`
	DurationMS        int64    `json:"duration_ms"`
	PreviewCount      int      `json:"preview_count"`
	PreviewDurationMS int64    `json:"preview_duration_ms"`
	ResponseMode      string   `json:"response_mode"`
	Navigation        string   `json:"navigation"`
	TimerMode         string   `json:"timer_mode"`
	ItemTimeMS        int64    `json:"item_time_ms"`
	Families          []string `json:"families"`
	Engines           []string `json:"engines"`
	Overlap           []string `json:"overlap"`
	ScoreOrigin       string   `json:"score_origin"`
	NavigationOrigin  string   `json:"navigation_origin"`
	ContentOrigin     string   `json:"content_origin"`
	Notes             string   `json:"notes"`
	Aids              string   `json:"aids"`
	Proposed          []string `json:"proposed"`
	Unknown           []string `json:"unknown"`
	Source            string   `json:"source"`
	ReviewedOn        string   `json:"reviewed_on"`
	ManifestVersion   string   `json:"manifest_version"`
}

func AssessmentProfiles() []AssessmentProfile {
	var profiles []AssessmentProfile
	if err := json.Unmarshal(assessmentProfileJSON, &profiles); err != nil {
		panic(err)
	}
	return profiles
}

func assessmentProfile(id string) (AssessmentProfile, bool) {
	for _, p := range AssessmentProfiles() {
		if p.ID == id {
			return p, true
		}
	}
	return AssessmentProfile{}, false
}
