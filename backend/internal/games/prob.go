package games

import (
	"encoding/json"
	"fmt"
	"math"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Probability Betting.
//
// Rounds present shared dice / coin / card events with quoted decimal odds.
// Players submit stakes atomically; outcomes settle from a pre-generated
// sample tape. Correlated predicates in a round share one sampled primitive.
// Kelly sizing follows f* = max(0, (p*b - (1-p)) / b) with b = D - 1.
// ---------------------------------------------------------------------------

type probOutcome struct {
	Desc string  `json:"desc"`
	P    float64 `json:"p"` // exact probability weight
}

type probPrimitive struct {
	Type     string        `json:"type"` // dice2 | die1 | coin3 | card
	Outcomes []probOutcome `json:"outcomes"`
}

type probPredicate struct {
	ID          string  `json:"event_id"`
	Text        string  `json:"text"`
	Kind        string  `json:"kind"`
	Arg         int     `json:"arg,omitempty"`
	P           float64 `json:"p"`            // exact probability (evaluator)
	FairOdds    float64 `json:"fair_odds"`    // 1/p
	OfferedOdds float64 `json:"offered_odds"` // decimal odds D, stake returned included
	EV          float64 `json:"ev_per_unit"`  // p*b - (1-p), b = D-1
	Kelly       float64 `json:"kelly_fraction"`
	ScaledKelly float64 `json:"scaled_kelly"` // after bankroll scaling
}

type probRound struct {
	Primitive  probPrimitive   `json:"primitive"`
	Predicates []probPredicate `json:"predicates"`
	SampleIdx  int             `json:"sample_idx"` // tape: sampled outcome index
	SampleDesc string          `json:"sample_desc"`
}

type probTape struct {
	SamplerVersion string      `json:"sampler_version"`
	PRNG           string      `json:"prng"`
	Config         probConfig  `json:"config"`
	Rounds         []probRound `json:"rounds"`
}

type probConfig struct {
	Rounds    int     `json:"rounds"`
	Bankroll  float64 `json:"bankroll"`
	BettingMS int64   `json:"betting_timer_ms"`
	ResultMS  int64   `json:"result_timer_ms"`
}

type probBet struct {
	EventID string  `json:"event_id"`
	Stake   float64 `json:"stake"`
}

type probSettled struct {
	EventID         string  `json:"event_id"`
	Stake           float64 `json:"stake"`
	Outcome         string  `json:"outcome"` // WIN | LOSS
	Profit          float64 `json:"profit"`
	Balance         float64 `json:"balance"`
	DecisionCorrect bool    `json:"decision_correct"`
	E               float64 `json:"sizing_score"`
	TargetF         float64 `json:"target_fraction"`
}

type probPlayer struct {
	balance   float64
	acted     bool
	bets      map[string]float64
	bankAtBet float64
	settled   [][]probSettled // per round
	decisions int
	correct   int
	oppSum    float64
	oppCount  int
}

type probGame struct {
	ctx          *engine.Ctx
	cfg          probConfig
	tape         probTape
	round        int
	phase        string // BETTING | RESULT | DONE
	phaseStartMS int64
	players      map[string]*probPlayer
	done         bool
}

func buildPrimitive(rng *engine.RNG) probPrimitive {
	switch t := []string{"dice2", "die1", "coin3", "card"}[rng.Int(4)]; t {
	case "dice2":
		counts := []int{1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1}
		p := probPrimitive{Type: "dice2"}
		for i, c := range counts {
			p.Outcomes = append(p.Outcomes, probOutcome{Desc: fmt.Sprintf("sum %d", i+2), P: float64(c) / 36})
		}
		return p
	case "die1":
		p := probPrimitive{Type: "die1"}
		for i := 1; i <= 6; i++ {
			p.Outcomes = append(p.Outcomes, probOutcome{Desc: fmt.Sprintf("face %d", i), P: 1.0 / 6})
		}
		return p
	case "coin3":
		p := probPrimitive{Type: "coin3"}
		for h := 0; h <= 3; h++ {
			p.Outcomes = append(p.Outcomes, probOutcome{Desc: fmt.Sprintf("%d heads", h), P: []float64{1, 3, 3, 1}[h] / 8})
		}
		return p
	default:
		p := probPrimitive{Type: "card"}
		for r := 1; r <= 13; r++ {
			p.Outcomes = append(p.Outcomes, probOutcome{Desc: fmt.Sprintf("rank %d", r), P: 4.0 / 52})
		}
		return p
	}
}

func predProb(pr probPrimitive, kind string, arg int) float64 {
	total := 0.0
	for _, o := range pr.Outcomes {
		if matchesPredicate(pr.Type, o.Desc, kind, arg) {
			total += o.P
		}
	}
	return total
}

func matchesPredicate(pType, desc, kind string, arg int) bool {
	var n int
	if _, err := fmt.Sscanf(desc, "sum %d", &n); err == nil {
		switch kind {
		case "SUM_GE":
			return n >= arg
		case "SUM_LE":
			return n <= arg
		case "SUM_EQ":
			return n == arg
		case "EVEN":
			return n%2 == 0
		case "ODD":
			return n%2 == 1
		}
	}
	if _, err := fmt.Sscanf(desc, "face %d", &n); err == nil {
		switch kind {
		case "FACE_GE":
			return n >= arg
		case "FACE_EQ":
			return n == arg
		case "FACE_ODD":
			return n%2 == 1
		}
	}
	if _, err := fmt.Sscanf(desc, "%d heads", &n); err == nil {
		switch kind {
		case "HEADS_EQ":
			return n == arg
		case "HEADS_GE":
			return n >= arg
		case "HEADS_EQ0":
			return n == 0
		}
	}
	if _, err := fmt.Sscanf(desc, "rank %d", &n); err == nil {
		switch kind {
		case "RANK_GE":
			return n >= arg
		case "RANK_EQ":
			return n == arg
		case "RANK_FACE": // J, Q, K
			return n >= 11
		}
	}
	return false
}

func predText(pType, kind string, arg int) string {
	switch kind {
	case "SUM_GE":
		return fmt.Sprintf("Two dice: the sum is at least %d", arg)
	case "SUM_LE":
		return fmt.Sprintf("Two dice: the sum is at most %d", arg)
	case "SUM_EQ":
		return fmt.Sprintf("Two dice: the sum is exactly %d", arg)
	case "EVEN":
		return "Two dice: the sum is even"
	case "ODD":
		return "Two dice: the sum is odd"
	case "FACE_GE":
		return fmt.Sprintf("One die: the face is at least %d", arg)
	case "FACE_EQ":
		return fmt.Sprintf("One die: the face is exactly %d", arg)
	case "FACE_ODD":
		return "One die: the face is odd"
	case "HEADS_EQ":
		return fmt.Sprintf("Three coin flips: exactly %d heads", arg)
	case "HEADS_GE":
		return fmt.Sprintf("Three coin flips: at least %d heads", arg)
	case "HEADS_EQ0":
		return "Three coin flips: zero heads"
	case "RANK_GE":
		return fmt.Sprintf("Card draw: rank at least %d (A=1)", arg)
	case "RANK_EQ":
		return fmt.Sprintf("Card draw: rank exactly %d (A=1)", arg)
	case "RANK_FACE":
		return "Card draw: face card (J, Q or K)"
	}
	return kind
}

func makePredicate(rng *engine.RNG, pr probPrimitive, id string) probPredicate {
	kind := ""
	arg := 0
	switch pr.Type {
	case "dice2":
		switch rng.Int(5) {
		case 0:
			kind, arg = "SUM_GE", rng.Range(5, 10)
		case 1:
			kind, arg = "SUM_LE", rng.Range(3, 8)
		case 2:
			kind, arg = "SUM_EQ", rng.Range(4, 10)
		case 3:
			kind = "EVEN"
		default:
			kind = "ODD"
		}
	case "die1":
		switch rng.Int(3) {
		case 0:
			kind, arg = "FACE_GE", rng.Range(3, 6)
		case 1:
			kind, arg = "FACE_EQ", rng.Range(2, 6)
		default:
			kind = "FACE_ODD"
		}
	case "coin3":
		switch rng.Int(3) {
		case 0:
			kind, arg = "HEADS_EQ", rng.Range(1, 2)
		case 1:
			kind, arg = "HEADS_GE", rng.Range(1, 2)
		default:
			kind = "HEADS_EQ0"
		}
	default:
		switch rng.Int(3) {
		case 0:
			kind, arg = "RANK_GE", rng.Range(8, 12)
		case 1:
			kind = "RANK_FACE"
		default:
			kind, arg = "RANK_EQ", rng.Range(2, 13)
		}
	}
	p := predProb(pr, kind, arg)
	fair := 0.0
	if p > 0 {
		fair = 1 / p
	}
	bias := 0.7 + rng.Float()*0.7 // 0.70 .. 1.40 of fair odds
	D := math.Max(1.05, engine.Round2(fair*bias))
	if math.IsInf(D, 0) || math.IsNaN(D) {
		D = 20
	}
	b := D - 1
	ev := p*b - (1 - p)
	kelly := 0.0
	if b > 0 {
		kelly = math.Max(0, (p*b-(1-p))/b)
	}
	return probPredicate{
		ID: id, Text: predText(pr.Type, kind, arg), Kind: kind, Arg: arg,
		P: p, FairOdds: engine.Round2(fair), OfferedOdds: D, EV: ev, Kelly: kelly,
	}
}

func newProb(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := probConfig{Rounds: 8, Bankroll: 100, BettingMS: 30_000, ResultMS: 8_000}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.Rounds < 1 || cfg.Bankroll <= 0 {
		return nil, fmt.Errorf("invalid prob config")
	}
	rng := engine.NewRNG(ctx.Seed + 0xF00B)
	g := &probGame{ctx: ctx, cfg: cfg, phase: "BETTING", players: map[string]*probPlayer{}}
	g.tape.PRNG = "splitmix64-v1"
	g.tape.SamplerVersion = "categorical-cdf-v1"
	g.tape.Config = cfg
	for _, p := range ctx.Players {
		g.players[p] = &probPlayer{balance: cfg.Bankroll, bets: map[string]float64{}}
	}
	for i := 0; i < cfg.Rounds; i++ {
		pr := buildPrimitive(rng)
		round := probRound{Primitive: pr}
		nPreds := 1 + rng.Int(2) // one or two correlated predicates
		for j := 0; j < nPreds; j++ {
			round.Predicates = append(round.Predicates, makePredicate(rng, pr, fmt.Sprintf("r%d-e%d", i, j)))
		}
		round.SampleIdx = samplePrimitive(rng, pr)
		round.SampleDesc = pr.Outcomes[round.SampleIdx].Desc
		g.tape.Rounds = append(g.tape.Rounds, round)
	}
	return g, nil
}

func (g *probGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "prob", Name: "Probability Betting", Access: "premium", MinPlayers: 1, MaxPlayers: 300,
		DurationMS:  int64(g.cfg.Rounds) * (g.cfg.BettingMS + g.cfg.ResultMS),
		Description: "Find favorable odds on dice, coin and card events, and size stakes Kelly-style.",
		Rules:       "Decimal odds include stake return: a win adds stake x (D-1), a loss subtracts the stake. Exact probabilities are enumerated from the sample space; fair odds are 1/p. Kelly f* = max(0, (p*b-(1-p))/b), b = D-1; if target fractions in a round sum above 1 they scale proportionally. Correlated predicates in a round share one sampled outcome. Submissions lock once accepted; skill = 3 x decision fraction + 7 x sizing closeness (appendix A1), balance breaks ties.",
	}
}

func (g *probGame) Tick() { g.tick() }

func (g *probGame) tick() {
	if g.done || g.round >= len(g.tape.Rounds) {
		if g.round >= len(g.tape.Rounds) {
			g.done = true
		}
		return
	}
	now := g.ctx.ElapsedMS()
	switch g.phase {
	case "BETTING":
		if now-g.phaseStartMS >= g.cfg.BettingMS || g.allActed() {
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
				p.bets = map[string]float64{}
			}
		}
	}
}

func (g *probGame) allActed() bool {
	for _, p := range g.players {
		if !p.acted {
			return false
		}
	}
	return true
}

func (g *probGame) settle(now int64) {
	rd := g.tape.Rounds[g.round]
	// Scaled Kelly targets: if positive-edge target fractions exceed 1 in a
	// round, scale them proportionally (declared approximation of A1).
	sum := 0.0
	for _, pr := range rd.Predicates {
		if pr.EV > 0 && pr.Kelly > 0 {
			sum += pr.Kelly
		}
	}
	scale := 1.0
	if sum > 1 {
		scale = 1 / sum
	}
	for i := range rd.Predicates {
		pr := &g.tape.Rounds[g.round].Predicates[i]
		if pr.EV > 0 && pr.Kelly > 0 {
			pr.ScaledKelly = pr.Kelly * scale
		}
	}
	sample := rd.SampleDesc
	for _, pl := range g.players {
		row := make([]probSettled, 0, len(rd.Predicates))
		for _, pr := range rd.Predicates {
			stake := pl.bets[pr.ID]
			won := matchesPredicate(rd.Primitive.Type, sample, pr.Kind, pr.Arg)
			res := probSettled{EventID: pr.ID, Stake: stake, TargetF: pr.ScaledKelly}
			opportunity := pr.EV > 0 && pr.Kelly > 0
			staked := stake > 0
			res.DecisionCorrect = (opportunity && staked) || (!opportunity && !staked)
			pl.decisions++
			if res.DecisionCorrect {
				pl.correct++
			}
			if opportunity {
				pl.oppCount++
				if staked {
					f := stake / math.Max(1, pl.bankAtBet)
					e := math.Max(0, 1-math.Abs(f-pr.ScaledKelly)/pr.ScaledKelly)
					pl.oppSum += e
					res.E = engine.Round2(e)
				}
			}
			if staked {
				if won {
					profit := stake * (pr.OfferedOdds - 1)
					pl.balance += profit
					res.Outcome = "WIN"
					res.Profit = engine.Round2(profit)
				} else {
					pl.balance -= stake
					res.Outcome = "LOSS"
					res.Profit = -stake
				}
			}
			res.Balance = engine.Round2(pl.balance)
			row = append(row, res)
		}
		pl.settled = append(pl.settled, row)
	}
	g.phase = "RESULT"
	g.phaseStartMS = now
}

// OutcomeAt returns the sampled primitive outcome description for a round.
func (r probRound) OutcomeAt(idx int) string {
	if idx < len(r.Primitive.Outcomes) {
		return r.Primitive.Outcomes[idx].Desc
	}
	return ""
}

func (g *probGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	p, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	remaining := g.cfg.BettingMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	if g.phase == "RESULT" {
		remaining = g.cfg.ResultMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	}
	if remaining < 0 {
		remaining = 0
	}
	snap := map[string]any{
		"round":             g.round + 1,
		"total_rounds":      len(g.tape.Rounds),
		"phase":             g.phase,
		"time_remaining_ms": remaining,
		"balance":           engine.Round2(p.balance),
		"starting_bankroll": g.cfg.Bankroll,
		"acted":             p.acted,
		"finished":          g.done,
	}
	if g.phase == "BETTING" {
		rd := g.tape.Rounds[g.round]
		events := make([]map[string]any, 0, len(rd.Predicates))
		for _, pr := range rd.Predicates {
			// Evaluator values (p, fair odds, EV, Kelly) stay hidden while betting.
			events = append(events, map[string]any{
				"event_id": pr.ID, "text": pr.Text, "offered_odds": pr.OfferedOdds,
			})
		}
		snap["events"] = events
	} else if g.phase == "RESULT" {
		rd := g.tape.Rounds[g.round]
		snap["sample"] = rd.SampleDesc
		if g.round < len(p.settled) {
			snap["my_results"] = p.settled[g.round]
			// Evaluator feedback: true p, fair odds, EV and scaled target.
			fb := make([]map[string]any, 0, len(rd.Predicates))
			for _, pr := range rd.Predicates {
				fb = append(fb, map[string]any{
					"event_id": pr.ID, "p": pr.P, "fair_odds": pr.FairOdds,
					"offered_odds": pr.OfferedOdds, "ev_per_unit": pr.EV,
					"kelly_fraction": pr.Kelly, "scaled_target": pr.ScaledKelly,
				})
			}
			snap["feedback"] = fb
		}
	}
	history := make([]map[string]any, 0, g.round)
	for i := 0; i < g.round; i++ {
		history = append(history, map[string]any{"round": i + 1, "sample": g.tape.Rounds[i].SampleDesc, "results": p.settled[i]})
	}
	snap["history"] = history
	return engine.MustJSON(snap), nil
}

func (g *probGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
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
	switch req.Action {
	case "PASS":
		p.acted = true
		return engine.Output{Accepted: true, PrivateChanges: engine.MustJSON(map[string]any{"acted": true})}
	case "SUBMIT_BETS":
		var pl struct {
			Bets []probBet `json:"bets"`
		}
		if err := json.Unmarshal(req.Payload, &pl); err != nil {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		// Atomic validation: every event id must exist in the round and the
		// total of reserved stakes cannot exceed the round's bankroll.
		round := g.tape.Rounds[g.round]
		valid := map[string]float64{}
		total := 0.0
		for _, b := range pl.Bets {
			var found *probPredicate
			for i := range round.Predicates {
				if round.Predicates[i].ID == b.EventID {
					found = &round.Predicates[i]
					break
				}
			}
			if found == nil {
				return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
			}
			if math.IsNaN(b.Stake) || math.IsInf(b.Stake, 0) || b.Stake <= 0 {
				return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
			}
			if _, dup := valid[b.EventID]; dup {
				return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
			}
			valid[b.EventID] = engine.Round2(b.Stake)
			total += engine.Round2(b.Stake)
		}
		if total > p.balance+1e-9 {
			return engine.Output{Accepted: false, Reason: engine.ReasonInsufficientStakes}
		}
		p.acted = true
		p.bets = valid
		p.bankAtBet = p.balance
		return engine.Output{
			Accepted:       true,
			Execution:      engine.MustJSON(map[string]any{"bets": pl.Bets, "total": engine.Round2(total)}),
			PrivateChanges: engine.MustJSON(map[string]any{"acted": true, "reserved": engine.Round2(total)}),
		}
	}
	return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
}

func (g *probGame) Finished() bool { g.tick(); return g.done }

func (g *probGame) lengthBonus() float64 {
	switch {
	case g.cfg.Rounds >= 9 && g.cfg.Rounds <= 10:
		return 1.5
	case g.cfg.Rounds >= 7 && g.cfg.Rounds <= 8:
		return 1
	case g.cfg.Rounds >= 5 && g.cfg.Rounds <= 6:
		return 0.5
	}
	return 0
}

func (g *probGame) Results() (json.RawMessage, error) {
	g.tick()
	type row struct {
		Player       string  `json:"player"`
		Skill        float64 `json:"skill"`
		DecisionFrac float64 `json:"decision_fraction"`
		SizingAvg    float64 `json:"sizing_average"`
		Balance      float64 `json:"final_balance"`
		Rank         int     `json:"rank"`
		GlobalPoints float64 `json:"global_points"`
		LengthBonus  float64 `json:"length_bonus"`
	}
	rows := make([]row, 0, len(g.players))
	for name, p := range g.players {
		d, e := 0.0, 0.0
		if p.decisions > 0 {
			d = float64(p.correct) / float64(p.decisions)
		}
		if p.oppCount > 0 {
			e = p.oppSum / float64(p.oppCount)
		}
		rows = append(rows, row{
			Player: name, Skill: engine.Round2(3*d + 7*e),
			DecisionFrac: engine.Round2(d), SizingAvg: engine.Round2(e),
			Balance: engine.Round2(p.balance), LengthBonus: g.lengthBonus(),
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
		if solo {
			rows[i].GlobalPoints = rows[i].LengthBonus
			continue
		}
		outranked := 0
		for j := range rows {
			if rows[j].Skill < rows[i].Skill ||
				(rows[j].Skill == rows[i].Skill && rows[j].Balance < rows[i].Balance) {
				outranked++
			}
		}
		rows[i].GlobalPoints = float64(outranked) + rows[i].LengthBonus
	}
	return engine.MustJSON(map[string]any{"finished": true, "players": rows}), nil
}

func (g *probGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }

// Aggregated sums/head-counts are not equally likely. Sample their declared
// mass rather than drawing uniformly from the list of distinct descriptions.
func samplePrimitive(rng *engine.RNG, primitive probPrimitive) int {
	draw, cumulative := rng.Float(), 0.0
	for i, outcome := range primitive.Outcomes {
		cumulative += outcome.P
		if draw < cumulative {
			return i
		}
	}
	return len(primitive.Outcomes) - 1
}
