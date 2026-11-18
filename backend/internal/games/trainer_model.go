package games

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"slices"

	"quantgame/backend/internal/engine"
)

const trainerVersion = "trainer-runtime-v1"
const trainerBankVersion = "trainer-scenarios-v1"

type trainerConfig struct {
	TrainerID     string   `json:"trainer_id"`
	Mode          string   `json:"mode"`
	DurationMS    int64    `json:"duration_ms"`
	Rounds        int      `json:"rounds"`
	AnswerFormat  string   `json:"answer_format"`
	Difficulty    int      `json:"difficulty"`
	Family        string   `json:"family"`
	Operations    []string `json:"operations"`
	Digits        int      `json:"digits"`
	Places        int      `json:"places"`
	FixedSpan     int      `json:"fixed_span"`
	MaxSpan       int      `json:"max_span"`
	FaceStyle     string   `json:"face_style"`
	PointerDevice string   `json:"pointer_device"`
	ItemID        string   `json:"item_id,omitempty"`
}
type trainerTrial struct {
	ID         string              `json:"trial_id"`
	Index      int                 `json:"index"`
	Level      int                 `json:"level"`
	Kind       string              `json:"kind"`
	Public     map[string]any      `json:"public"`
	Secret     map[string]any      `json:"secret"`
	Question   *assessmentQuestion `json:"question,omitempty"`
	EncodeMS   int64               `json:"encode_ms"`
	MaskMS     int64               `json:"mask_ms"`
	ResponseMS int64               `json:"response_ms"`
}
type trainerTile struct {
	ID         string   `json:"id"`
	Value      int      `json:"value"`
	Expression string   `json:"expression"`
	Sources    []string `json:"sources"`
}
type trainerDial struct {
	ID      string  `json:"id"`
	Slot    int     `json:"slot"`
	Spawn   int64   `json:"spawn_ms"`
	Expires int64   `json:"expires_ms"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Radius  float64 `json:"radius"`
	Theta   float64 `json:"theta0"`
	Omega   float64 `json:"omega"`
	Target  float64 `json:"target"`
	Width   float64 `json:"width"`
}
type trainerBook struct {
	Cash       float64          `json:"cash"`
	Position   float64          `json:"position"`
	Shares     float64          `json:"shares"`
	Delta      float64          `json:"delta"`
	Multiplier float64          `json:"multiplier"`
	Fair       float64          `json:"fair"`
	Spot       float64          `json:"spot"`
	Ledger     []map[string]any `json:"ledger"`
}
type trainerMoveState struct {
	Tiles       []trainerTile `json:"tiles,omitempty"`
	Stacks      [][]string    `json:"stacks,omitempty"`
	Moves       int           `json:"moves"`
	Guesses     [][]string    `json:"guesses,omitempty"`
	Counts      []int         `json:"counts,omitempty"`
	FirstMoveMS *int64        `json:"first_move_ms"`
	Pumps       int           `json:"pumps"`
	Hints       int           `json:"hints"`
	Reasoning   string        `json:"reasoning"`
}

func resolveTrainerConfig(raw json.RawMessage) (trainerConfig, TrainerProfile, error) {
	var c trainerConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, TrainerProfile{}, err
	}
	p, ok := trainerProfile(c.TrainerID)
	if !ok {
		return c, p, fmt.Errorf("unknown trainer")
	}
	if c.Mode == "" {
		c.Mode = p.Modes[0]
	}
	if !slices.Contains(p.Modes, c.Mode) {
		return c, p, fmt.Errorf("unknown trainer mode")
	}
	if c.AnswerFormat == "" {
		c.AnswerFormat = "open"
	}
	if c.AnswerFormat != "open" && c.AnswerFormat != "choice" {
		return c, p, fmt.Errorf("answer_format must be open or choice")
	}
	if c.Difficulty == 0 {
		c.Difficulty = 2
	}
	if c.Difficulty < 1 || c.Difficulty > 3 {
		return c, p, fmt.Errorf("difficulty must be 1..3")
	}
	if c.Digits == 0 {
		c.Digits = c.Difficulty
	}
	if c.Digits < 1 || c.Digits > 4 {
		return c, p, fmt.Errorf("digits must be 1..4")
	}
	if c.Family == "" {
		c.Family = "mixed"
	}
	if !slices.Contains([]string{"mixed", "integer", "decimal", "fraction", "percentage", "counting", "bayes", "expectation"}, c.Family) {
		return c, p, fmt.Errorf("invalid topic family")
	}
	if len(c.Operations) == 0 {
		c.Operations = []string{"+", "-", "*", "/", "%"}
	}
	for _, op := range c.Operations {
		if !slices.Contains([]string{"+", "-", "*", "/", "%"}, op) {
			return c, p, fmt.Errorf("invalid operation")
		}
	}
	if c.MaxSpan == 0 {
		c.MaxSpan = 8
	}
	if c.MaxSpan < 4 || c.MaxSpan > 8 || c.FixedSpan < 0 || c.FixedSpan > c.MaxSpan || (c.FixedSpan > 0 && c.FixedSpan < 4) {
		return c, p, fmt.Errorf("digit span must be 4..8; fixed 0 means adaptive")
	}
	if c.Places == 0 {
		c.Places = 4
	}
	if c.Places < 2 || c.Places > 5 {
		return c, p, fmt.Errorf("decimal places must be 2..5")
	}
	if c.FaceStyle == "" {
		c.FaceStyle = "mixed"
	}
	if !slices.Contains([]string{"mixed", "colors", "shapes", "dice", "numbers", "letters", "icons", "patterns"}, c.FaceStyle) {
		return c, p, fmt.Errorf("unknown face style")
	}
	if c.DurationMS == 0 {
		c.DurationMS = p.DurationMS
	}
	if c.Rounds == 0 {
		c.Rounds = p.Rounds
	}
	switch c.TrainerID {
	case "math", "task-switch", "response-conflict", "pincode", "cube", "sequence", "probability":
		maxMinutes := int64(10)
		if c.TrainerID == "sequence" || c.TrainerID == "probability" {
			maxMinutes = 15
		}
		if c.DurationMS < 60000 || c.DurationMS > maxMinutes*60000 {
			return c, p, fmt.Errorf("timer outside published 1..%d minute range", maxMinutes)
		}
		c.Rounds = 0
	case "fractions":
		if c.Mode == "test" {
			c.Places = 5
			if c.DurationMS == 0 {
				c.DurationMS = 120000
			}
			if !slices.Contains([]int64{60000, 120000, 300000, 600000}, c.DurationMS) {
				return c, p, fmt.Errorf("test durations are 1,2,5 or 10 minutes")
			}
			c.Rounds = 0
		} else {
			c.DurationMS = 0
		}
	case "etf", "options", "futures", "prediction":
		if c.Mode == "case" {
			c.Rounds = 9
			c.DurationMS = 2700000
		} else {
			c.Rounds = 10
			c.DurationMS = 0
		}
	case "pump":
		c.DurationMS = 0
		if c.Mode == "stakes" {
			c.Rounds = 20
		} else {
			c.Rounds = 30
		}
	case "sweet-spot":
		c.DurationMS = 120000
		c.Rounds = 0
	case "fermi":
		if c.ItemID != "" {
			found := false
			for _, q := range fermiBank {
				if q.ID == c.ItemID {
					found = true
				}
			}
			if !found {
				return c, p, fmt.Errorf("unknown Fermi question")
			}
		}
		c.DurationMS = 0
		c.Rounds = 1
	default:
		c.DurationMS = 0
	}
	if c.Rounds < 0 || c.Rounds > 128 {
		return c, p, fmt.Errorf("round count must be 1..128 for finite sessions")
	}
	if p.Rounds > 0 && c.Rounds < 1 && c.DurationMS == 0 {
		return c, p, fmt.Errorf("finite sessions require rounds")
	}
	return c, p, nil
}
func trainerRNG(seed uint64, id string, index, level int) *engine.RNG {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return engine.NewRNG(seed ^ h.Sum64() ^ uint64(index+1)*0x9E3779B97F4A7C15 ^ uint64(level+17)*0xBF58476D1CE4E5B9)
}
func trainerClone[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copy T
	_ = json.Unmarshal(raw, &copy)
	return copy
}
func trainerInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		x, _ := n.Int64()
		return int(x)
	}
	return 0
}
func trainerFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case json.Number:
		x, _ := n.Float64()
		return x
	}
	return 0
}
func trainerString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
func trainerDecode[T any](v any) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func trainerSignedAngle(deg float64) float64 { return math.Mod(math.Mod(deg+180, 360)+360, 360) - 180 }
