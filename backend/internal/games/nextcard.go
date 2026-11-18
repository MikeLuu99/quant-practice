package games

import (
	"encoding/json"
	"fmt"
	"math"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Next Card Betting.
//
// A shared deck cycle reveals reference card(s); players predict
// higher/lower (or inside/outside) and size a stake. Even-money payout:
// win +stake, loss -stake, push returns the stake. Skill scoring follows
// appendix A1: 3 x decision fraction + 7 x Kelly-sizing closeness.
// ---------------------------------------------------------------------------

type ncConfig struct {
	Suits           int     `json:"suits"` // 1..4
	AceHigh         bool    `json:"ace_high"`
	Mode            string  `json:"mode"` // higher_lower | inside_outside | alternating
	StartingBalance float64 `json:"starting_balance"`
	BettingTimerMS  int64   `json:"betting_timer_ms"`
	ResultMS        int64   `json:"result_ms"`
	Cycles          int     `json:"cycles"`
	MaxRounds       int     `json:"max_rounds"`
}

type ncCard struct {
	Rank int `json:"rank"` // 1..13
	Suit int `json:"suit"` // 0..3
}

func (c ncCard) val(aceHigh bool) int {
	if c.Rank == 1 && aceHigh {
		return 14
	}
	return c.Rank
}

func (c ncCard) label() string {
	ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	suits := []string{"S", "H", "D", "C"}
	return ranks[c.Rank-1] + suits[c.Suit]
}

type ncRound struct {
	Mode  string  `json:"mode"`
	Cycle int     `json:"cycle"`
	Ref1  ncCard  `json:"ref1"`
	Ref2  *ncCard `json:"ref2,omitempty"`
	Draw  ncCard  `json:"draw"`
}

type ncTape struct {
	PRNG   string     `json:"prng"`
	Config ncConfig   `json:"config"`
	Decks  [][]ncCard `json:"decks"` // shuffled order per cycle
	Rounds []ncRound  `json:"rounds"`
}

type ncRoundEval struct {
	PW     float64 `json:"p_win"`
	PL     float64 `json:"p_loss"`
	PP     float64 `json:"p_push"`
	Target string  `json:"target"` // "" when no positive edge
	KellyF float64 `json:"kelly_fraction"`
}

type ncPlayerResult struct {
	Round           int         `json:"round"`
	Outcome         string      `json:"outcome"` // WIN | LOSS | PUSH | MISS
	Delta           float64     `json:"delta"`
	Balance         float64     `json:"balance"`
	Feedback        ncRoundEval `json:"feedback"`
	Bet             string      `json:"bet,omitempty"` // prediction made
	Stake           float64     `json:"stake,omitempty"`
	DecisionCorrect bool        `json:"decision_correct"`
	E               float64     `json:"sizing_score"`
}

type ncPlayer struct {
	balance    float64
	acted      bool
	prediction string
	stake      float64
	results    []ncPlayerResult
	rounds     int
	correct    int
	oppSum     float64
	oppCount   int
}

type ncGame struct {
	ctx          *engine.Ctx
	cfg          ncConfig
	tape         ncTape
	round        int
	phase        string // BETTING | RESULT | DONE
	phaseStartMS int64
	players      map[string]*ncPlayer
	done         bool
}

func newNextCard(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := ncConfig{
		Suits: 4, AceHigh: false, Mode: "higher_lower", StartingBalance: 100,
		BettingTimerMS: 20_000, ResultMS: 6_000, Cycles: 1, MaxRounds: 0,
	}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	switch cfg.Mode {
	case "higher_lower", "inside_outside", "alternating":
	default:
		return nil, fmt.Errorf("invalid nextcard mode %q", cfg.Mode)
	}
	if cfg.Suits < 1 || cfg.Suits > 4 {
		return nil, fmt.Errorf("nextcard suits must be 1..4")
	}

	rng := engine.NewRNG(ctx.Seed + 0xCAB0)
	g := &ncGame{ctx: ctx, cfg: cfg, phase: "BETTING", players: map[string]*ncPlayer{}}
	g.tape.PRNG = "splitmix64-v1"
	g.tape.Config = cfg
	for _, p := range ctx.Players {
		g.players[p] = &ncPlayer{balance: cfg.StartingBalance}
	}

	// Build cycles: full deck order per cycle, consuming 2 or 3 cards per round.
	for cycle := 0; cycle < cfg.Cycles; cycle++ {
		deck := make([]ncCard, 0, cfg.Suits*13)
		for s := 0; s < cfg.Suits; s++ {
			for r := 1; r <= 13; r++ {
				deck = append(deck, ncCard{Rank: r, Suit: s})
			}
		}
		rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		g.tape.Decks = append(g.tape.Decks, append([]ncCard(nil), deck...))
		i := 0
		for i < len(deck) {
			if cfg.MaxRounds > 0 && len(g.tape.Rounds) >= cfg.MaxRounds {
				break
			}
			mode := cfg.Mode
			if mode == "alternating" {
				if len(g.tape.Rounds)%2 == 0 {
					mode = "higher_lower"
				} else {
					mode = "inside_outside"
				}
			}
			needed := 2
			if mode == "inside_outside" {
				needed = 3
			}
			if i+needed > len(deck) {
				break
			}
			rd := ncRound{Mode: mode, Cycle: cycle, Ref1: deck[i], Draw: deck[i+needed-1]}
			if needed == 3 {
				c := deck[i+1]
				rd.Ref2 = &c
				if rd.Ref1.val(cfg.AceHigh) == c.val(cfg.AceHigh) {
					// Equal boundaries are disallowed as a reference range;
					// skip this slot (draw cards still consumed).
					i += needed
					continue
				}
			}
			g.tape.Rounds = append(g.tape.Rounds, rd)
			i += needed
		}
	}
	if len(g.tape.Rounds) == 0 {
		return nil, fmt.Errorf("nextcard produced no rounds")
	}
	return g, nil
}

func (g *ncGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "nextcard", Name: "Next Card Betting", Access: "premium", MinPlayers: 1, MaxPlayers: 300,
		DurationMS:  int64(len(g.tape.Rounds)) * (g.cfg.BettingTimerMS + g.cfg.ResultMS),
		Description: "Predict the next card (higher/lower, inside/outside) and size your stake Kelly-style.",
		Rules:       "Even-money payout: correct +stake, wrong -stake, equality pushes. Probabilities count remaining unseen cards. Kelly target f* = max(0, (pW-pL)/(pW+pL)). Skill = 3 x decision fraction + 7 x average sizing closeness (appendix A1). Deck resets each cycle with a recorded new order. One bet per round; no action by the deadline is an implicit pass.",
	}
}

// remainingRanks returns per-rank card counts among unseen cards of the
// current cycle as of the start of round roundIdx (refs revealed, draw not).
func (g *ncGame) remainingRanks(roundIdx int) [15]int {
	rem := [15]int{}
	if roundIdx < 0 || roundIdx >= len(g.tape.Rounds) {
		return rem
	}
	cyc := g.tape.Rounds[roundIdx].Cycle
	for i := 0; i < roundIdx; i++ {
		if g.tape.Rounds[i].Cycle != cyc {
			continue
		}
		rd := g.tape.Rounds[i]
		rem[rd.Ref1.Rank]++
		if rd.Ref2 != nil {
			rem[rd.Ref2.Rank]++
		}
		rem[rd.Draw.Rank]++
	}
	rd := g.tape.Rounds[roundIdx]
	rem[rd.Ref1.Rank]++
	if rd.Ref2 != nil {
		rem[rd.Ref2.Rank]++
	}
	for r := 1; r <= 13; r++ {
		rem[r] = g.cfg.Suits - rem[r]
	}
	return rem
}

func (g *ncGame) permuted(mode string, rem [15]int, ref1 ncCard, ref2 *ncCard) (pW, pL, pP float64) {
	r1, r2 := ref1.val(g.cfg.AceHigh), 0
	if ref2 != nil {
		r2 = ref2.val(g.cfg.AceHigh)
	}
	total := 0.0
	for r := 1; r <= 13; r++ {
		if rem[r] <= 0 {
			continue
		}
		total += float64(rem[r])
		var wins bool
		var push bool
		switch mode {
		case "HIGHER":
			wins, push = r > r1, r == r1
		case "LOWER":
			wins, push = r < r1, r == r1
		case "INSIDE":
			wins, push = r > r1 && r < r2, r == r1 || r == r2
		case "OUTSIDE":
			wins, push = r < r1 || r > r2, r == r1 || r == r2
		}
		switch {
		case push:
			pP += float64(rem[r])
		case wins:
			pW += float64(rem[r])
		default:
			pL += float64(rem[r])
		}
	}
	if total > 0 {
		pW, pL, pP = pW/total, pL/total, pP/total
	}
	return
}

func kellyFrac(pW, pL float64) float64 {
	if pW+pL <= 0 {
		return 0
	}
	return math.Max(0, (pW-pL)/(pW+pL))
}

// evaluateRound computes pre-draw probabilities and the Kelly target for the
// permitted predictions of a round.
func (g *ncGame) evaluateRound(roundIdx int) map[string]ncRoundEval {
	rd := g.tape.Rounds[roundIdx]
	rem := g.remainingRanks(roundIdx)
	out := map[string]ncRoundEval{}
	permitted := []string{"HIGHER", "LOWER"}
	if rd.Mode == "inside_outside" {
		permitted = []string{"INSIDE", "OUTSIDE"}
	}
	for _, p := range permitted {
		pW, pL, pP := g.permuted(p, rem, rd.Ref1, rd.Ref2)
		f := kellyFrac(pW, pL)
		out[p] = ncRoundEval{PW: pW, PL: pL, PP: pP, Target: p, KellyF: f}
	}
	// Target = the permitted prediction with the highest positive edge.
	// Raw precision: rounding happens only for display.
	best := ""
	bestF := 0.0
	for _, p := range permitted {
		if f := out[p].KellyF; f > bestF {
			bestF, best = f, p
		}
	}
	for _, p := range permitted {
		ev := out[p]
		if p != best {
			ev.Target = ""
			ev.KellyF = 0 // only the chosen target carries a target fraction
		}
		out[p] = ev
	}
	return out
}

func (g *ncGame) outcomeOf(rd ncRound, prediction string) string {
	d := rd.Draw.val(g.cfg.AceHigh)
	r1 := rd.Ref1.val(g.cfg.AceHigh)
	r2 := 0
	if rd.Ref2 != nil {
		r2 = rd.Ref2.val(g.cfg.AceHigh)
	}
	switch prediction {
	case "HIGHER":
		if d > r1 {
			return "WIN"
		} else if d == r1 {
			return "PUSH"
		}
		return "LOSS"
	case "LOWER":
		if d < r1 {
			return "WIN"
		} else if d == r1 {
			return "PUSH"
		}
		return "LOSS"
	case "INSIDE":
		if d == r1 || d == r2 {
			return "PUSH"
		} else if d > r1 && d < r2 {
			return "WIN"
		}
		return "LOSS"
	case "OUTSIDE":
		if d == r1 || d == r2 {
			return "PUSH"
		} else if d < r1 || d > r2 {
			return "WIN"
		}
		return "LOSS"
	}
	return "LOSS"
}

func (g *ncGame) Tick() { g.tick() }

func (g *ncGame) tick() {
	if g.done || g.round >= len(g.tape.Rounds) {
		if g.round >= len(g.tape.Rounds) {
			g.done = true
		}
		return
	}
	now := g.ctx.ElapsedMS()
	switch g.phase {
	case "BETTING":
		if now-g.phaseStartMS >= g.cfg.BettingTimerMS || g.allActed() {
			g.settle(now)
		}
	case "RESULT":
		if now-g.phaseStartMS >= g.cfg.ResultMS {
			g.round++
			if g.round >= len(g.tape.Rounds) {
				g.done = true
				g.phase = "DONE"
				return
			}
			g.phase = "BETTING"
			g.phaseStartMS = now
			for _, p := range g.players {
				p.acted = false
				p.prediction = ""
				p.stake = 0
			}
		}
	}
}

func (g *ncGame) allActed() bool {
	for _, p := range g.players {
		if !p.acted {
			return false
		}
	}
	return true
}

func (g *ncGame) settle(now int64) {
	rd := g.tape.Rounds[g.round]
	eval := g.evaluateRound(g.round)
	var target string
	var targetF float64
	for p, ev := range eval {
		if ev.Target != "" {
			target = p
			targetF = ev.KellyF
		}
	}
	for _, pl := range g.players {
		res := ncPlayerResult{Round: g.round, Feedback: ncRoundEval{}}
		if target != "" {
			// Opportunity exists; compute player's participation and sizing.
			balBefore := pl.balance
			if pl.acted && pl.prediction == target {
				f := pl.stake / math.Max(1, balBefore)
				e := math.Max(0, 1-math.Abs(f-targetF)/targetF)
				pl.oppSum += e
				res.E = engine.Round2(e)
			}
			pl.oppCount++
		}
		hasBet := pl.acted && pl.prediction != "" && pl.stake > 0
		// An explicit pass is a locked decision, but is not a zero-stake bet.
		if target != "" {
			res.DecisionCorrect = pl.acted && pl.prediction == target
		} else {
			res.DecisionCorrect = !hasBet // explicit and implicit passes agree
		}
		pl.rounds++
		if res.DecisionCorrect {
			pl.correct++
		}
		// Balance application for those who bet.
		if hasBet {
			outcome := g.outcomeOf(rd, pl.prediction)
			switch outcome {
			case "WIN":
				pl.balance += pl.stake
				res.Delta = pl.stake
			case "LOSS":
				pl.balance -= pl.stake
				res.Delta = -pl.stake
			case "PUSH":
				res.Delta = 0
			}
			res.Outcome = outcome
			res.Bet = pl.prediction
			res.Stake = pl.stake
		} else {
			res.Outcome = "MISS"
		}
		ev := eval[pl.prediction]
		if ev.Target != "" || pl.prediction == "" {
			res.Feedback = ncRoundEval{PW: ev.PW, PL: ev.PL, PP: ev.PP, Target: target, KellyF: targetF}
		} else {
			// Bet on a non-target side: show that side's probabilities.
			evTarget := eval[pl.prediction]
			res.Feedback = ncRoundEval{PW: evTarget.PW, PL: evTarget.PL, PP: evTarget.PP, Target: target, KellyF: targetF}
		}
		pl.results = append(pl.results, res)
	}
	g.phase = "RESULT"
	g.phaseStartMS = now
}

func (g *ncGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	p, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	history := make([]map[string]any, 0, g.round)
	for i := 0; i < g.round; i++ {
		rd := g.tape.Rounds[i]
		entry := map[string]any{
			"round": i + 1, "mode": rd.Mode,
			"ref1": map[string]any{"label": rd.Ref1.label(), "rank": rd.Ref1.Rank},
			"draw": map[string]any{"label": rd.Draw.label(), "rank": rd.Draw.Rank},
		}
		if rd.Ref2 != nil {
			entry["ref2"] = map[string]any{"label": rd.Ref2.label(), "rank": rd.Ref2.Rank}
		}
		if i < len(p.results) {
			entry["my_result"] = p.results[i]
		}
		history = append(history, entry)
	}
	// After the final reveal the cursor is past the tape. Keep the last
	// reference visible so the client can fetch the completed session.
	current := g.round
	if current >= len(g.tape.Rounds) {
		current = len(g.tape.Rounds) - 1
	}
	cur := g.tape.Rounds[current]
	remaining := g.cfg.BettingTimerMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	if g.phase == "RESULT" {
		remaining = g.cfg.ResultMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	}
	if remaining < 0 {
		remaining = 0
	}
	ref1 := map[string]any{"label": cur.Ref1.label(), "rank": cur.Ref1.Rank}
	snap := map[string]any{
		"round":             current + 1,
		"total_rounds":      len(g.tape.Rounds),
		"phase":             g.phase,
		"mode":              cur.Mode,
		"ref1":              ref1,
		"time_remaining_ms": remaining,
		"balance":           engine.Round2(p.balance),
		"starting_balance":  g.cfg.StartingBalance,
		"acted":             p.acted,
		"history":           history,
		"finished":          g.done,
	}
	if cur.Ref2 != nil {
		snap["ref2"] = map[string]any{"label": cur.Ref2.label(), "rank": cur.Ref2.Rank}
	}
	if (g.phase == "RESULT" || g.done) && len(p.results) > current {
		rd := g.tape.Rounds[current]
		snap["draw"] = map[string]any{"label": rd.Draw.label(), "rank": rd.Draw.Rank}
		snap["my_result"] = p.results[current]
	}
	return engine.MustJSON(snap), nil
}

func (g *ncGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	p, ok := g.players[req.PlayerID]
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if g.done || g.phase != "BETTING" {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	if p.acted {
		return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
	}
	rd := g.tape.Rounds[g.round]
	switch req.Action {
	case "PASS":
		p.acted = true
		p.prediction = ""
		p.stake = 0
		return engine.Output{Accepted: true, PrivateChanges: engine.MustJSON(map[string]any{"acted": true})}
	case "BET":
		var pl struct {
			Prediction string  `json:"prediction"`
			Stake      float64 `json:"stake"`
		}
		if err := json.Unmarshal(req.Payload, &pl); err != nil {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		permitted := map[string]bool{"HIGHER": true, "LOWER": true}
		if rd.Mode == "inside_outside" {
			permitted = map[string]bool{"INSIDE": true, "OUTSIDE": true}
		}
		if !permitted[pl.Prediction] {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		stake := engine.Round2(pl.Stake)
		if stake <= 0 || math.IsNaN(stake) || math.IsInf(stake, 0) {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		if stake > p.balance {
			return engine.Output{Accepted: false, Reason: engine.ReasonInsufficient}
		}
		p.acted = true
		p.prediction = pl.Prediction
		p.stake = stake
		return engine.Output{
			Accepted:       true,
			Execution:      engine.MustJSON(map[string]any{"prediction": pl.Prediction, "stake": stake}),
			PrivateChanges: engine.MustJSON(map[string]any{"acted": true, "stake": stake}),
		}
	}
	return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
}

func (g *ncGame) Finished() bool { g.tick(); return g.done }

func (g *ncGame) skill(p *ncPlayer) (d, e float64) {
	if p.rounds > 0 {
		d = float64(p.correct) / float64(p.rounds)
	}
	if p.oppCount > 0 {
		e = p.oppSum / float64(p.oppCount)
	}
	return d, e
}

func (g *ncGame) Results() (json.RawMessage, error) {
	g.tick()
	type row struct {
		Player       string  `json:"player"`
		Skill        float64 `json:"skill"`
		DecisionFrac float64 `json:"decision_fraction"`
		SizingAvg    float64 `json:"sizing_average"`
		Balance      float64 `json:"final_balance"`
		Rank         int     `json:"rank"`
		GlobalPoints int     `json:"global_points"`
		SuitBonus    int     `json:"suit_bonus"`
	}
	rows := make([]row, 0, len(g.players))
	for name, p := range g.players {
		d, e := g.skill(p)
		rows = append(rows, row{
			Player: name, Skill: engine.Round2(3*d + 7*e),
			DecisionFrac: engine.Round2(d), SizingAvg: engine.Round2(e),
			Balance: engine.Round2(p.balance), SuitBonus: g.cfg.Suits - 1,
		})
	}
	for i := range rows {
		r := 1
		for j := range rows {
			if rows[j].Skill > rows[i].Skill ||
				(rows[j].Skill == rows[i].Skill && rows[j].Balance > rows[i].Balance) {
				r++
			}
		}
		rows[i].Rank = r
	}
	solo := len(rows) <= 1
	for i := range rows {
		if rows[i].Balance <= 0 {
			rows[i].GlobalPoints = 0 // bankrupt players earn no global points
			continue
		}
		if solo {
			rows[i].GlobalPoints = rows[i].SuitBonus
			continue
		}
		outranked := 0
		for j := range rows {
			if rows[j].Skill < rows[i].Skill ||
				(rows[j].Skill == rows[i].Skill && rows[j].Balance < rows[i].Balance) {
				outranked++
			}
		}
		rows[i].GlobalPoints = outranked + rows[i].SuitBonus
	}
	return engine.MustJSON(map[string]any{"finished": true, "players": rows}), nil
}

func (g *ncGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }
