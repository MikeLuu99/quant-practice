package games

import (
	"encoding/json"
	"fmt"
	"math"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Fermi Questions Challenge.
//
// Players submit positive numerical estimates under a per-question deadline.
// Accuracy is measured by symmetric multiplicative error r = max(x/y, y/x).
// First valid submission is locked; missing the deadline costs 2 points.
// ---------------------------------------------------------------------------

type fermiConfig struct {
	Rounds          int   `json:"rounds"`
	QuestionTimerMS int64 `json:"question_timer_ms"`
	RevealMS        int64 `json:"reveal_ms"`
}

type fermiRevealRow struct {
	Player   string   `json:"player"`
	Value    *float64 `json:"value"` // nil when missing
	ErrorR   float64  `json:"multiplicative_error,omitempty"`
	Log10Err float64  `json:"log10_error,omitempty"`
	Points   int      `json:"points"`
}

type fermiReveal struct {
	Question FermiBankQ      `json:"question"`
	Rows     []fermiRevealRow `json:"rows"`
}

type fermiGame struct {
	ctx          *engine.Ctx
	cfg          fermiConfig
	rounds       []FermiBankQ
	round        int
	phase        string // QUESTION | REVEAL | DONE
	phaseStartMS int64
	estimates    map[string]map[int]float64
	locked       map[string]map[int]bool
	points       map[string]int
	reveals      []fermiReveal
	done         bool
}

func newFermi(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := fermiConfig{Rounds: 10, QuestionTimerMS: 60_000, RevealMS: 8_000}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.Rounds < 1 || cfg.Rounds > len(fermiBank) {
		return nil, fmt.Errorf("fermi rounds must be 1..%d", len(fermiBank))
	}
	rng := engine.NewRNG(ctx.Seed + 0xFE12)
	idx := make([]int, len(fermiBank))
	for i := range idx {
		idx[i] = i
	}
	rng.Shuffle(len(idx), func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	g := &fermiGame{
		ctx: ctx, cfg: cfg, phase: "QUESTION", phaseStartMS: 0,
		estimates: map[string]map[int]float64{}, locked: map[string]map[int]bool{}, points: map[string]int{},
	}
	for _, p := range ctx.Players {
		g.estimates[p] = map[int]float64{}
		g.locked[p] = map[int]bool{}
		g.points[p] = 0
	}
	for i := 0; i < cfg.Rounds; i++ {
		g.rounds = append(g.rounds, fermiBank[idx[i]])
	}
	return g, nil
}

func (g *fermiGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "fermi", Name: "Fermi Questions Challenge", Access: "premium", MinPlayers: 1, MaxPlayers: 300,
		DurationMS: int64(g.cfg.Rounds) * (g.cfg.QuestionTimerMS + g.cfg.RevealMS),
		Description: "Estimate huge quantities. Scored by multiplicative accuracy, revealed after each round.",
		Rules: "Submit one positive estimate per question; the first valid submission locks. Error r = max(x/y, y/x). Points: r<=1.1 -> 10, <=1.25 -> 8, <=1.5 -> 6, <=2 -> 4, <=3 -> 2, else 0; missing the deadline costs -2. Answers and explanations reveal after each round. Ranking uses shared ranks on total score.",
	}
}

func multErr(x, y float64) float64 { return math.Max(x/y, y/x) }

func fermiPoints(r float64) int {
	switch {
	case r <= 1.1:
		return 10
	case r <= 1.25:
		return 8
	case r <= 1.5:
		return 6
	case r <= 2:
		return 4
	case r <= 3:
		return 2
	}
	return 0
}

func (g *fermiGame) allSubmitted() bool {
	for _, p := range g.ctx.Players {
		if !g.locked[p][g.round] {
			return false
		}
	}
	return true
}

func (g *fermiGame) Tick() { g.tick() }

func (g *fermiGame) tick() {
	if g.done {
		return
	}
	now := g.ctx.ElapsedMS()
	switch g.phase {
	case "QUESTION":
		if now-g.phaseStartMS >= g.cfg.QuestionTimerMS || g.allSubmitted() {
			g.closeRound(now)
		}
	case "REVEAL":
		if now-g.phaseStartMS >= g.cfg.RevealMS {
			g.nextRound(now)
		}
	}
}

func (g *fermiGame) closeRound(now int64) {
	q := g.rounds[g.round]
	rows := make([]fermiRevealRow, 0, len(g.ctx.Players))
	for _, p := range g.ctx.Players {
		if v, ok := g.estimates[p][g.round]; ok {
			r := multErr(v, q.Answer)
			pts := fermiPoints(r)
			g.points[p] += pts
			rows = append(rows, fermiRevealRow{Player: p, Value: &v, ErrorR: engine.Round2(r), Log10Err: engine.Round2(math.Log10(r)), Points: pts})
		} else {
			g.points[p] -= 2
			rows = append(rows, fermiRevealRow{Player: p, Points: -2})
		}
	}
	g.reveals = append(g.reveals, fermiReveal{Question: q, Rows: rows})
	g.phase = "REVEAL"
	g.phaseStartMS = now
}

func (g *fermiGame) nextRound(now int64) {
	if g.round+1 >= g.cfg.Rounds {
		g.done = true
		g.phase = "DONE"
		return
	}
	g.round++
	g.phase = "QUESTION"
	g.phaseStartMS = now
}

func (g *fermiGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	if _, ok := g.points[playerID]; !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	submitted := make([]string, 0, len(g.ctx.Players))
	for _, p := range g.ctx.Players {
		if g.locked[p][g.round] {
			submitted = append(submitted, p)
		}
	}
	remaining := g.cfg.QuestionTimerMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	if g.phase == "REVEAL" {
		remaining = g.cfg.RevealMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	}
	if remaining < 0 {
		remaining = 0
	}
	snap := map[string]any{
		"round":             g.round + 1,
		"total_rounds":      g.cfg.Rounds,
		"phase":             g.phase,
		"time_remaining_ms": remaining,
		"submitted":         submitted,
		"my_points":         g.points[playerID],
		"finished":          g.done,
		"leaderboard":       g.leaderboard(),
	}
	if g.phase == "QUESTION" {
		q := g.rounds[g.round]
		snap["question"] = map[string]any{"question_id": q.ID, "prompt": q.Prompt, "units": q.Units}
	} else if g.phase == "REVEAL" {
		snap["reveal"] = g.reveals[g.round]
	}
	return engine.MustJSON(snap), nil
}

func (g *fermiGame) leaderboard() []map[string]any {
	rows := make([]map[string]any, 0, len(g.points))
	for _, p := range g.ctx.Players {
		rows = append(rows, map[string]any{"player": p, "points": g.points[p]})
	}
	return rows
}

func (g *fermiGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	if _, ok := g.points[req.PlayerID]; !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if req.Action != "ESTIMATE" {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
	}
	if g.phase != "QUESTION" {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	var pl struct {
		QuestionID string  `json:"question_id"`
		Value      float64 `json:"value"`
	}
	if err := json.Unmarshal(req.Payload, &pl); err != nil {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	q := g.rounds[g.round]
	if pl.QuestionID != q.ID {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	if g.locked[req.PlayerID][g.round] {
		return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
	}
	// Reject NaN, +-Inf and non-positive values: invalid input never counts
	// as a submission and never locks the player.
	if math.IsNaN(pl.Value) || math.IsInf(pl.Value, 0) || pl.Value <= 0 {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	g.estimates[req.PlayerID][g.round] = pl.Value
	g.locked[req.PlayerID][g.round] = true
	return engine.Output{
		Accepted:       true,
		Execution:      engine.MustJSON(map[string]any{"locked": true}),
		PrivateChanges: engine.MustJSON(map[string]any{"submitted": true}),
		PublicChanges:  engine.MustJSON(map[string]any{"submitted": []string{req.PlayerID}}),
	}
}

func (g *fermiGame) Finished() bool { g.tick(); return g.done }

func (g *fermiGame) Results() (json.RawMessage, error) {
	g.tick()
	type row struct {
		Player       string `json:"player"`
		Total        int    `json:"total"`
		Rank         int    `json:"rank"`
		GlobalPoints int    `json:"global_points"`
	}
	rows := make([]row, 0, len(g.points))
	for _, p := range g.ctx.Players {
		rows = append(rows, row{Player: p, Total: g.points[p]})
	}
	// Shared ranks by total, descending.
	for i := range rows {
		r := 1
		for j := range rows {
			if rows[j].Total > rows[i].Total {
				r++
			}
		}
		rows[i].Rank = r
	}
	solo := len(rows) <= 1
	for i := range rows {
		var bonus int
		switch {
		case rows[i].Total >= 80:
			bonus = 3
		case rows[i].Total >= 60:
			bonus = 2
		case rows[i].Total >= 40:
			bonus = 1
		}
		if solo {
			rows[i].GlobalPoints = bonus
		} else {
			outranked := 0
			for j := range rows {
				if rows[j].Total < rows[i].Total {
					outranked++
				}
			}
			rows[i].GlobalPoints = bonus + outranked
		}
	}
	return engine.MustJSON(map[string]any{
		"finished": true,
		"players":  rows,
		"reveals":  g.reveals,
	}), nil
}

func (g *fermiGame) Tape() (json.RawMessage, error) {
	return engine.MustJSON(map[string]any{
		"prng":   "splitmix64-v1",
		"rounds": g.rounds,
		"config": g.cfg,
	}), nil
}
