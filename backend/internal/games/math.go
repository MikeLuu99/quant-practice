package games

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Math Challenge.
//
// A timed arithmetic test over an ordered question tape shared by all
// competitors. Every participant starts at item zero; faster players reach
// more items. Answers are exact rationals: integers, finite decimals and
// numerator/denominator strings are all accepted and compared exactly.
// ---------------------------------------------------------------------------

type mathQuestion struct {
	ID         string `json:"question_id"`
	Mode       string `json:"mode"` // integer | decimal | fraction
	Expression string `json:"expression"`
	AnswerNum  string `json:"answer"` // exact rational, canonical form
}

type mathTape struct {
	PRNG      string         `json:"prng"`
	Config    map[string]any `json:"config"`
	Questions []mathQuestion `json:"questions"`
}

type mathConfig struct {
	DurationMS int64  `json:"duration_ms"`
	Mode       string `json:"mode"` // integer | decimal | fraction | mixed
	Count      int    `json:"count"`
}

type mathAttempt struct {
	QuestionID string `json:"question_id"`
	Expression string `json:"expression"`
	Given      string `json:"given"`
	Correct    bool   `json:"correct"`
	LatencyMS  int64  `json:"latency_ms"`
}

type mathPlayer struct {
	index     int
	score     int
	correct   int
	incorrect int
	attempts  []mathAttempt
	lastANS   int64
}

type mathGame struct {
	ctx     *engine.Ctx
	cfg     mathConfig
	tape    mathTape
	players map[string]*mathPlayer
	done    bool
}

func newMath(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := mathConfig{DurationMS: 300_000, Mode: "integer", Count: 200}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	switch cfg.Mode {
	case "integer", "decimal", "fraction", "mixed":
	default:
		return nil, fmt.Errorf("invalid math mode %q", cfg.Mode)
	}
	if cfg.Count < 1 || cfg.DurationMS < 1 {
		return nil, fmt.Errorf("count and duration_ms must be positive")
	}
	g := &mathGame{ctx: ctx, cfg: cfg, players: map[string]*mathPlayer{}}
	for _, id := range ctx.Players {
		g.players[id] = &mathPlayer{}
	}
	rng := engine.NewRNG(ctx.Seed + 0xCA1C)
	g.tape.PRNG = "splitmix64-v1"
	g.tape.Config = map[string]any{"mode": cfg.Mode, "count": cfg.Count, "two_operands": true}
	for i := 0; i < cfg.Count; i++ {
		mode := cfg.Mode
		if mode == "mixed" {
			mode = []string{"integer", "decimal", "fraction"}[rng.Int(3)]
		}
		q := genMathQuestion(rng, i, mode)
		g.tape.Questions = append(g.tape.Questions, q)
	}
	return g, nil
}

func genMathQuestion(rng *engine.RNG, i int, mode string) mathQuestion {
	q := mathQuestion{ID: fmt.Sprintf("q%d", i), Mode: mode}
	switch mode {
	case "integer":
		a, b := rng.Range(2, 99), rng.Range(2, 99)
		switch op := []string{"+", "-", "*", "/"}[rng.Int(4)]; op {
		case "+":
			q.Expression = fmt.Sprintf("%d + %d", a, b)
			q.AnswerNum = new(big.Int).Add(big.NewInt(int64(a)), big.NewInt(int64(b))).String()
		case "-":
			if a < b {
				a, b = b, a
			}
			q.Expression = fmt.Sprintf("%d - %d", a, b)
			q.AnswerNum = new(big.Int).Sub(big.NewInt(int64(a)), big.NewInt(int64(b))).String()
		case "*":
			x, y := rng.Range(2, 15), rng.Range(2, 15)
			q.Expression = fmt.Sprintf("%d x %d", x, y)
			q.AnswerNum = new(big.Int).Mul(big.NewInt(int64(x)), big.NewInt(int64(y))).String()
		default:
			b = rng.Range(2, 12)
			c := rng.Range(2, 20)
			q.Expression = fmt.Sprintf("%d / %d", b*c, b)
			q.AnswerNum = fmt.Sprintf("%d", c)
		}
	case "decimal":
		a := dec(rng, 2, 99)
		b := dec(rng, 2, 99)
		switch op := []string{"+", "-", "*"}[rng.Int(3)]; op {
		case "+":
			q.Expression = fmt.Sprintf("%s + %s", a, b)
			q.AnswerNum = ratAddSub(a, b, 1)
		case "-":
			if cmpDec(a, b) < 0 {
				a, b = b, a
			}
			q.Expression = fmt.Sprintf("%s - %s", a, b)
			q.AnswerNum = ratAddSub(a, b, -1)
		default:
			x := dec(rng, 2, 12)
			y := dec(rng, 2, 12)
			q.Expression = fmt.Sprintf("%s x %s", x, y)
			ra, _ := new(big.Rat).SetString(x)
			rb, _ := new(big.Rat).SetString(y)
			q.AnswerNum = canon(new(big.Rat).Mul(ra, rb))
		}
	default: // fraction
		a, b := rng.Range(1, 12), rng.Range(2, 12)
		c, d := rng.Range(1, 12), rng.Range(2, 12)
		switch op := []string{"+", "-", "*", "/"}[rng.Int(4)]; op {
		case "+", "-":
			q.Expression = fmt.Sprintf("%d/%d %s %d/%d", a, b, op, c, d)
			ra := ratFrac(a, b)
			rb := ratFrac(c, d)
			if op == "+" {
				q.AnswerNum = canon(ra.Add(ra, rb))
			} else {
				ra.Sub(ra, rb)
				q.AnswerNum = canon(ra)
			}
		case "*":
			q.Expression = fmt.Sprintf("%d/%d x %d/%d", a, b, c, d)
			ra := ratFrac(a, b)
			rb := ratFrac(c, d)
			q.AnswerNum = canon(ra.Mul(ra, rb))
		default:
			q.Expression = fmt.Sprintf("%d/%d / %d/%d", a, b, c, d)
			ra := ratFrac(a, b)
			rb := ratFrac(c, d)
			q.AnswerNum = canon(ra.Quo(ra, rb))
		}
	}
	return q
}

func dec(rng *engine.RNG, lo, hi int) string {
	ip := rng.Range(lo, hi)
	switch rng.Int(3) {
	case 0:
		return fmt.Sprintf("%d", ip)
	default:
		return fmt.Sprintf("%d.%d", ip, rng.Range(1, 9))
	case 2:
		return fmt.Sprintf("%d.%d%d", ip, rng.Range(1, 9), rng.Range(0, 9))
	}
}

func cmpDec(a, b string) int {
	ra, _ := new(big.Rat).SetString(a)
	rb, _ := new(big.Rat).SetString(b)
	return ra.Cmp(rb)
}

func ratAddSub(a, b string, sign int) string {
	ra, _ := new(big.Rat).SetString(a)
	rb, _ := new(big.Rat).SetString(b)
	if sign < 0 {
		rb.Neg(rb)
	}
	return canon(ra.Add(ra, rb))
}

func ratFrac(n, d int) *big.Rat { return new(big.Rat).SetFrac64(int64(n), int64(d)) }

func canon(r *big.Rat) string {
	if r.Denom().Cmp(big.NewInt(1)) == 0 {
		return r.Num().String()
	}
	return fmt.Sprintf("%s/%s", r.Num().String(), r.Denom().String())
}

func (g *mathGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "math", Name: "Math Challenge", Access: "free", MinPlayers: 1, MaxPlayers: 300,
		DurationMS:  g.cfg.DurationMS,
		Description: "Timed arithmetic over integers, decimals and fractions. +1 correct, -1 incorrect.",
		Rules:       "One ordered question tape shared by all players; everyone starts at item zero. Answers accept integers, finite decimals or n/d fractions and are compared as exact rationals (0.75 = 3/4 = 6/8). Correct +1, incorrect -1, malformed input rejected without advancing, one answer per item. At expiry unanswered items contribute zero.",
	}
}

func (g *mathGame) cur(p *mathPlayer) *mathQuestion {
	if p.index < len(g.tape.Questions) {
		return &g.tape.Questions[p.index]
	}
	return nil
}

func (g *mathGame) Snapshot(playerID string) (json.RawMessage, error) {
	p, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	g.checkTime()
	remaining := g.cfg.DurationMS - g.ctx.ElapsedMS()
	if remaining < 0 {
		remaining = 0
	}
	snap := map[string]any{
		"time_remaining_ms": remaining,
		"index":             p.index,
		"total":             len(g.tape.Questions),
		"score":             p.score,
		"correct":           p.correct,
		"incorrect":         p.incorrect,
		"finished":          g.done,
	}
	if q := g.cur(p); q != nil && !g.done {
		snap["question"] = map[string]any{"question_id": q.ID, "expression": q.Expression, "mode": q.Mode}
	}
	return engine.MustJSON(snap), nil
}

func (g *mathGame) checkTime() {
	if g.ctx.ElapsedMS() >= g.cfg.DurationMS {
		g.done = true
		return
	}
	for _, p := range g.players {
		if p.index < len(g.tape.Questions) {
			return
		}
	}
	g.done = true
}

func (g *mathGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.checkTime()
	p, ok := g.players[req.PlayerID]
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if req.Action != "ANSWER" {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
	}
	if g.done {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	var pl struct {
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	}
	if err := json.Unmarshal(req.Payload, &pl); err != nil {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	q := g.cur(p)
	if q == nil || pl.QuestionID != q.ID {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	ans := strings.TrimSpace(pl.Answer)
	if ans == "" {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	given, ok := parseExactNumber(ans)
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	truth, _ := new(big.Rat).SetString(q.AnswerNum)
	okAns := given.Cmp(truth) == 0
	delta := -1
	if okAns {
		delta = 1
		p.correct++
	} else {
		p.incorrect++
	}
	p.score += delta
	latency := g.ctx.ElapsedMS() - p.lastANS
	p.lastANS = g.ctx.ElapsedMS()
	p.attempts = append(p.attempts, mathAttempt{QuestionID: q.ID, Expression: q.Expression, Given: ans, Correct: okAns, LatencyMS: latency})
	p.index++
	g.checkTime()
	gameOver := g.done
	return engine.Output{
		Accepted:       true,
		RoundReward:    float64(delta),
		Execution:      engine.MustJSON(map[string]any{"correct": okAns, "correct_answer": q.AnswerNum}),
		PrivateChanges: engine.MustJSON(map[string]any{"score": p.score, "correct": p.correct, "incorrect": p.incorrect}),
		GameOver:       gameOver,
	}
}

func (g *mathGame) Finished() bool { g.checkTime(); return g.done }

func speedPoints(mode string, spp float64) int {
	th := []float64{6, 6.75, 7.5}
	if mode == "integer" {
		th = []float64{3, 3.5, 4.5}
	}
	switch {
	case spp <= th[0]:
		return 3
	case spp <= th[1]:
		return 2
	case spp <= th[2]:
		return 1
	}
	return 0
}

func (g *mathGame) playerResults(p *mathPlayer) map[string]any {
	mode := g.cfg.Mode
	if mode == "mixed" {
		mode = "mixed"
	}
	net := p.score
	res := map[string]any{
		"finished":   g.done,
		"score":      p.score,
		"correct":    p.correct,
		"incorrect":  p.incorrect,
		"attempted":  len(p.attempts),
		"duration_s": g.cfg.DurationMS / 1000,
	}
	if net > 0 {
		spp := float64(g.cfg.DurationMS) / 1000 / float64(net)
		res["seconds_per_point"] = engine.Round2(spp)
		res["speed_points"] = speedPoints(mode, spp)
	} else {
		res["seconds_per_point"] = nil
		res["speed_points"] = 0
	}
	review := make([]map[string]any, 0, len(p.attempts))
	for _, a := range p.attempts {
		review = append(review, map[string]any{
			"question_id": a.QuestionID, "expression": a.Expression, "given": a.Given,
			"correct": a.Correct, "latency_ms": a.LatencyMS,
		})
	}
	res["review"] = review
	return res
}

func (g *mathGame) Results() (json.RawMessage, error) {
	g.checkTime()
	rows := make([]map[string]any, 0, len(g.ctx.Players))
	for _, id := range g.ctx.Players {
		row := g.playerResults(g.players[id])
		row["player"] = id
		rows = append(rows, row)
	}
	for _, row := range rows {
		rank := 1
		for _, other := range rows {
			if other["score"].(int) > row["score"].(int) {
				rank++
			}
		}
		row["rank"] = rank
	}
	res := map[string]any{}
	// Preserve the solo result shape for existing API clients.
	if len(rows) == 1 {
		for key, value := range rows[0] {
			res[key] = value
		}
	}
	res["finished"] = g.done
	res["players"] = rows
	return engine.MustJSON(res), nil
}

func (g *mathGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }
