package games

import (
	"encoding/json"
	"fmt"
	"math"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Fruit Market.
//
// Two bags of apples/oranges; value V = m * (a1+a2) * (o1+o2). Players buy at
// the ask or sell at the bid; trades settle immediately against current V.
// Final score = cumulative profit * first-click accuracy. The exogenous event
// tape (fruit increments, resets, value events) is generated once from the
// seed and stored verbatim for reproducibility.
// ---------------------------------------------------------------------------

type fruitUpdate struct {
	T          int64 `json:"t_ms"`
	Bag        int   `json:"bag"`
	Fruit      int   `json:"fruit"` // 0 apples, 1 oranges
	Add        int   `json:"add"`
	Reset      bool  `json:"reset,omitempty"`      // bag exceeded max_count
	NewApples  int   `json:"new_apples,omitempty"` // post-reset counts
	NewOranges int   `json:"new_oranges,omitempty"`
}

type fruitEvent struct {
	T          int64  `json:"t_ms"`
	Type       string `json:"type"` // DOUBLE | HALF | NULLIFY
	Bag        int    `json:"bag,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

type fruitTape struct {
	PRNG         string        `json:"prng"`
	Updates      []fruitUpdate `json:"updates"`
	Events       []fruitEvent  `json:"events"`
	QuoteOffsets []float64     `json:"quote_offsets"` // mispricing as fraction of V per market version
}

type fruitConfig struct {
	DurationMS         int64 `json:"duration_ms"`
	MaxCount           int   `json:"max_count"`
	MinTradeIntervalMS int64 `json:"min_trade_interval_ms"`
	TradeQuantity      int   `json:"trade_quantity"`
	EventDurationMS    int64 `json:"event_duration_ms"`
	SpreadBP           int   `json:"spread_bp"` // quote spread in basis points of V
}

type fruitPlayer struct {
	profit      float64
	trades      int
	fcTried     int
	fcCorrect   int
	lastTradeMS int64
	fcUsed      map[int]bool // market_version -> first click already taken
}

type fruitGame struct {
	ctx  *engine.Ctx
	cfg  fruitConfig
	tape fruitTape
	rng  *engine.RNG // construction stream only; live state follows tape

	bags     [2][2]int // bag -> [apples, oranges]
	mult     float64
	nullify  [2]int64 // bag -> nullify end time (0 = none)
	version  int
	bid, ask int
	qIdx     int // next tape quote offset to consume
	nextU    int // index into tape.Updates
	nextE    int // index into tape.Events

	players map[string]*fruitPlayer
	done    bool
}

func newFruit(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := fruitConfig{
		DurationMS:         300_000,
		MaxCount:           12,
		MinTradeIntervalMS: 400,
		TradeQuantity:      1,
		EventDurationMS:    30_000,
		SpreadBP:           400,
	}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.DurationMS <= 0 || cfg.MaxCount < 8 || cfg.TradeQuantity < 1 {
		return nil, fmt.Errorf("invalid fruit config")
	}

	g := &fruitGame{ctx: ctx, cfg: cfg, players: map[string]*fruitPlayer{}}
	for _, p := range ctx.Players {
		g.players[p] = &fruitPlayer{fcUsed: map[int]bool{}, lastTradeMS: -cfg.MinTradeIntervalMS}
	}

	rng := engine.NewRNG(ctx.Seed + 0xF00D)
	g.rng = rng
	g.tape.PRNG = "splitmix64-v1"

	// Initial counts: uniform integers 3..8 per fruit per bag (independent).
	for b := 0; b < 2; b++ {
		g.bags[b][0] = rng.Range(3, 8)
		g.bags[b][1] = rng.Range(3, 8)
	}

	// Four independent update streams (bag x fruit), intervals 15-20s, adds 0-3.
	for b := 0; b < 2; b++ {
		for f := 0; f < 2; f++ {
			t := int64(rng.Range(15, 20)) * 1000
			counts := [2][2]int{g.bags[0], g.bags[1]}
			for t < cfg.DurationMS {
				add := rng.Range(0, 3)
				u := fruitUpdate{T: t, Bag: b, Fruit: f, Add: add}
				ca, co := counts[b][0], counts[b][1]
				if f == 0 {
					ca += add
				} else {
					co += add
				}
				if ca > cfg.MaxCount || co > cfg.MaxCount {
					u.Reset = true
					ca, co = rng.Range(3, 8), rng.Range(3, 8)
					u.NewApples, u.NewOranges = ca, co
				}
				counts[b][0], counts[b][1] = ca, co
				g.tape.Updates = append(g.tape.Updates, u)
				t += int64(rng.Range(15, 20)) * 1000
			}
		}
	}

	// 2-3 value events, spaced apart so they never overlap.
	n := rng.Range(2, 3)
	t := int64(rng.Range(40, 70)) * 1000
	for i := 0; i < n && t+cfg.EventDurationMS < cfg.DurationMS; i++ {
		types := []string{"DOUBLE", "HALF", "NULLIFY"}
		ev := fruitEvent{T: t, Type: types[rng.Int(3)], DurationMS: cfg.EventDurationMS}
		if ev.Type == "NULLIFY" {
			ev.Bag = rng.Int(2)
		}
		g.tape.Events = append(g.tape.Events, ev)
		t += cfg.EventDurationMS + int64(rng.Range(50, 90))*1000
	}

	// Quote mispricing offsets: uniform fraction of V in [-0.04, 0.04].
	for i := 0; i < 1024; i++ {
		g.tape.QuoteOffsets = append(g.tape.QuoteOffsets, rng.Float()*0.08-0.04)
	}

	// Start with a live market: multiplier 1 and initial quotes.
	g.mult = 1
	g.applyQuotes()
	return g, nil
}

func (g *fruitGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "fruit", Name: "Fruit Market", Access: "free", MinPlayers: 1, MaxPlayers: 300,
		DurationMS:  g.cfg.DurationMS,
		Description: "Value apples x oranges across two visible bags; buy at the ask, sell at the bid.",
		Rules:       "V = multiplier x total apples x total oranges. BUY reward = V - ask, SELL reward = bid - V, settled immediately. Quotes are V plus a seeded mispricing offset from the tape plus a half-spread. First-click accuracy multiplies raw profit at the end. Fruit updates arrive every 15-20s (+0..3 each); exceeding the cap resets the bag. DOUBLE/HALF/NULLIFY events come from the event tape.",
	}
}

// tick applies all tape entries due at the current elapsed time.
func (g *fruitGame) tick() {
	now := g.ctx.ElapsedMS()
	changed := false
	for g.nextE < len(g.tape.Events) && g.tape.Events[g.nextE].T <= now {
		ev := g.tape.Events[g.nextE]
		g.version++
		if ev.Type == "NULLIFY" {
			g.nullify[ev.Bag] = ev.T + ev.DurationMS
		} else if ev.Type == "DOUBLE" {
			g.mult *= 2
		} else {
			g.mult *= 0.5
		}
		g.nextE++
		changed = true
	}
	// Event expiry (nullify lift) also bumps the version.
	for b := 0; b < 2; b++ {
		if g.nullify[b] != 0 && now >= g.nullify[b] {
			g.nullify[b] = 0
			g.version++
			changed = true
		}
	}
	for g.nextU < len(g.tape.Updates) && g.tape.Updates[g.nextU].T <= now {
		u := g.tape.Updates[g.nextU]
		if g.nullify[u.Bag] == 0 { // nullified bags freeze
			if u.Reset {
				g.bags[u.Bag][0] = u.NewApples
				g.bags[u.Bag][1] = u.NewOranges
			} else {
				g.bags[u.Bag][u.Fruit] += u.Add
			}
			g.version++
			changed = true
		}
		g.nextU++
	}
	if changed {
		g.applyQuotes()
	}
	if now >= g.cfg.DurationMS {
		g.done = true
	}
}

func (g *fruitGame) value() (int, [2][2]int) {
	eff := [2][2]int{}
	sumA, sumO := 0, 0
	for b := 0; b < 2; b++ {
		a, o := g.bags[b][0], g.bags[b][1]
		if g.nullify[b] != 0 && g.ctx.ElapsedMS() < g.nullify[b] {
			a, o = 0, 0
		} else {
			g.nullify[b] = 0
		}
		eff[b] = [2]int{a, o}
		sumA += a
		sumO += o
	}
	v := g.mult * float64(sumA) * float64(sumO)
	return int(math.Round(v)), eff
}

func (g *fruitGame) applyQuotes() {
	v, _ := g.value()
	h := int(math.Max(1, math.Round(float64(v)*0.01))) // half spread
	off := 0.0
	if g.qIdx < len(g.tape.QuoteOffsets) {
		off = g.tape.QuoteOffsets[g.qIdx]
		g.qIdx++
	}
	mid := math.Round(float64(v) + off*float64(v))
	g.version++
	g.bid, g.ask = int(mid)-h, int(mid)+h
}

func (g *fruitGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	p, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	_, eff := g.value()
	acc := 0.0
	if p.fcTried > 0 {
		acc = float64(p.fcCorrect) / float64(p.fcTried)
	}
	remaining := g.cfg.DurationMS - g.ctx.ElapsedMS()
	if remaining < 0 {
		remaining = 0
	}
	ev := map[string]any{"type": "NONE", "multiplier": g.mult, "nullified": []int{}}
	for b := 0; b < 2; b++ {
		if g.nullify[b] != 0 {
			ev["type"] = "NULLIFY"
			ev["nullified"] = append(ev["nullified"].([]int), b)
		}
	}
	if g.mult >= 2 {
		ev["type"] = "DOUBLE"
	} else if g.mult <= 0.5 {
		ev["type"] = "HALF"
	}
	return engine.MustJSON(map[string]any{
		"bags":              []map[string]int{{"apples": eff[0][0], "oranges": eff[0][1]}, {"apples": eff[1][0], "oranges": eff[1][1]}},
		"event":             ev,
		"bid":               g.bid,
		"ask":               g.ask,
		"market_version":    g.version,
		"time_remaining_ms": remaining,
		"finished":          g.done,
		"me": map[string]any{
			"profit":               engine.Round2(p.profit),
			"trades":               p.trades,
			"first_clicks_correct": p.fcCorrect,
			"first_clicks_tried":   p.fcTried,
			"accuracy":             engine.Round2(acc),
		},
	}), nil
}

func (g *fruitGame) applyAction(p *fruitPlayer, side string, qty, version int, now int64) engine.Output {
	if g.done {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	if now-p.lastTradeMS < g.cfg.MinTradeIntervalMS {
		return engine.Output{Accepted: false, Reason: engine.ReasonRateLimit}
	}
	if version != 0 && version != g.version {
		return engine.Output{Accepted: false, Reason: engine.ReasonStaleMarket}
	}
	if qty != g.cfg.TradeQuantity {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	v, _ := g.value()
	var reward float64
	var price int
	if side == "BUY" {
		price = g.ask
		reward = float64(v - price)
	} else {
		price = g.bid
		reward = float64(price - v)
	}
	p.profit += reward
	p.trades++
	p.lastTradeMS = now
	first := !p.fcUsed[g.version]
	var fc string
	if first {
		p.fcUsed[g.version] = true
		p.fcTried++
		if reward > 0 {
			p.fcCorrect++
			fc = "CORRECT"
		} else {
			fc = "WRONG"
		}
	}
	acc := 0.0
	if p.fcTried > 0 {
		acc = float64(p.fcCorrect) / float64(p.fcTried)
	}
	return engine.Output{
		Accepted:    true,
		RoundReward: reward,
		Execution:   engine.MustJSON(map[string]any{"side": side, "price": price, "quantity": qty, "value_hidden": true}),
		PrivateChanges: engine.MustJSON(map[string]any{
			"cumulative_profit":    engine.Round2(p.profit),
			"first_click":          fc,
			"first_clicks_correct": p.fcCorrect,
			"first_clicks_tried":   p.fcTried,
			"accuracy":             engine.Round2(acc),
		}),
	}
}

func (g *fruitGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	p, ok := g.players[req.PlayerID]
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	now := g.ctx.ElapsedMS()
	switch req.Action {
	case "BUY", "SELL":
		qty := g.cfg.TradeQuantity
		if len(req.Payload) > 0 {
			var pl struct {
				Quantity int `json:"quantity"`
			}
			_ = json.Unmarshal(req.Payload, &pl)
			if pl.Quantity != 0 {
				qty = pl.Quantity
			}
		}
		return g.applyAction(p, req.Action, qty, req.MarketVersion, now)
	case "NOOP":
		return engine.Output{Accepted: true}
	}
	return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
}

func (g *fruitGame) Finished() bool { g.tick(); return g.done }

func (g *fruitGame) Results() (json.RawMessage, error) {
	g.tick()
	type row struct {
		Player       string  `json:"player"`
		Profit       float64 `json:"raw_profit"`
		FCCorrect    int     `json:"first_clicks_correct"`
		FCTried      int     `json:"first_clicks_tried"`
		Accuracy     float64 `json:"accuracy"`
		FinalScore   float64 `json:"final_score"`
		Rank         int     `json:"rank"`
		GlobalPoints int     `json:"global_points"`
	}
	v, eff := g.value()
	rows := make([]row, 0, len(g.players))
	for name, p := range g.players {
		acc := 0.0
		if p.fcTried > 0 {
			acc = float64(p.fcCorrect) / float64(p.fcTried)
		}
		rows = append(rows, row{Player: name, Profit: engine.Round2(p.profit), FCCorrect: p.fcCorrect, FCTried: p.fcTried, Accuracy: engine.Round2(acc), FinalScore: engine.Round2(p.profit * acc)})
	}
	// Shared ranks belong on the returned rows, not a temporary sorted copy.
	for i := range rows {
		rows[i].Rank = 1
		for j := range rows {
			if rows[j].FinalScore > rows[i].FinalScore {
				rows[i].Rank++
			}
		}
	}
	solo := len(rows) <= 1
	for i := range rows {
		var outranked int
		for j := range rows {
			if rows[j].FinalScore < rows[i].FinalScore {
				outranked++
			}
		}
		mult := 0
		switch {
		case rows[i].Accuracy >= 0.8:
			mult = 2
		case rows[i].Accuracy >= 0.6:
			mult = 1
		}
		if solo {
			rows[i].GlobalPoints = mult
		} else {
			rows[i].GlobalPoints = outranked * mult
		}
	}
	return engine.MustJSON(map[string]any{
		"finished":    true,
		"final_bags":  []map[string]int{{"apples": eff[0][0], "oranges": eff[0][1]}, {"apples": eff[1][0], "oranges": eff[1][1]}},
		"final_value": v,
		"players":     rows,
	}), nil
}

func (g *fruitGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }
