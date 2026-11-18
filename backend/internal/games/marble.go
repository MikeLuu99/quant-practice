package games

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// The Marble Olympics.
//
// A simulated marble football match. Hidden team quality is Poisson scoring
// rates per team and goal colour; earlier tournament scores give evidence.
// Contracts: the MAIN contract pays (final A total) x (final B total); side
// contracts pay 100 x indicator(event). Quotes are generated from an
// omniscient evaluator with seeded mispricing; edge points are awarded at
// trade time by standardized mispricing bins (appendix contract).
// ---------------------------------------------------------------------------

type marbleConfig struct {
	SimDurationMS  int64   `json:"sim_duration_ms"` // 90 game-minutes
	PlaybackX      float64 `json:"playback_x"`      // sim-ms per real-ms
	HistoryMatches int     `json:"history_matches"`
	SideCount      int     `json:"side_count"`
	WindowStepMS   int64   `json:"window_step_ms"`
	WindowLenMS    int64   `json:"window_len_ms"`
	MainQuoteStep  int64   `json:"main_quote_step_ms"`
}

type marbleGoal struct {
	TSimMS int64  `json:"t_sim_ms"`
	Team   string `json:"team"` // A | B
	Color  string `json:"color"` // orange | red
}

type marbleHistoryMatch struct {
	Team int `json:"team"`
	Opp  int `json:"opponent"`
}

type marbleSideSpec struct {
	ID   string `json:"id"`
	Desc string `json:"desc"`
	Kind string `json:"kind"` // A_TOTAL_GE | B_TOTAL_GE | A_ORANGE_GE | B_ORANGE_GE | TOTAL_GE
	Arg  int    `json:"arg"`
}

type marbleSideMarket struct {
	Spec       marbleSideSpec `json:"spec"`
	OpenRealMS int64          `json:"open_real_ms"`
	CloseRealMS int64         `json:"close_real_ms"`
	Bid        int            `json:"bid"`
	Ask        int            `json:"ask"`
}

type marbleTape struct {
	PRNG    string             `json:"prng"`
	Config  marbleConfig       `json:"config"`
	Rates   [4]float64         `json:"rates"` // hidden: [A_or, A_rd, B_or, B_rd] per game-minute
	Goals   []marbleGoal       `json:"goals"`
	History [][2]marbleHistoryMatch `json:"history"` // [A..., B...]
	MainQuotes []struct {
		TRealMS int64 `json:"t_real_ms"`
		Bid     int   `json:"bid"`
		Ask     int   `json:"ask"`
	} `json:"main_quotes"`
	Sides []marbleSideMarket `json:"sides"`
}

type marbleBook struct {
	C float64
	Q int
}

type marbleTradeRec struct {
	Contract string  `json:"contract"`
	Side     string  `json:"side"`
	Price    int     `json:"price"`
	EdgePts  int     `json:"edge_points"`
}

type marblePlayer struct {
	books    map[string]*marbleBook // contract -> C,Q
	edgeSum  int
	trades   []marbleTradeRec
}

type marbleGame struct {
	ctx    *engine.Ctx
	cfg    marbleConfig
	tape   marbleTape
	qIdx   int // main quote cursor
	players map[string]*marblePlayer
	done   bool
}

var marbleSideCatalog = []marbleSideSpec{
	{ID: "A_GE_7", Desc: "Team A finishes with at least 7 goals", Kind: "A_TOTAL_GE", Arg: 7},
	{ID: "A_GE_9", Desc: "Team A finishes with at least 9 goals", Kind: "A_TOTAL_GE", Arg: 9},
	{ID: "B_GE_6", Desc: "Team B finishes with at least 6 goals", Kind: "B_TOTAL_GE", Arg: 6},
	{ID: "B_GE_8", Desc: "Team B finishes with at least 8 goals", Kind: "B_TOTAL_GE", Arg: 8},
	{ID: "A_OR_GE_3", Desc: "Team A scores at least 3 orange goals", Kind: "A_ORANGE_GE", Arg: 3},
	{ID: "TOT_GE_13", Desc: "Combined final totals reach at least 13", Kind: "TOTAL_GE", Arg: 13},
}

func poissonSample(rng *engine.RNG, mu float64) int {
	L := math.Exp(-mu)
	k, p := 0, 1.0
	for {
		p *= rng.Float()
		if p <= L {
			return k
		}
		k++
	}
}

func poissonPMF(mu float64, maxK int) []float64 {
	pmf := make([]float64, maxK+1)
	l := -mu
	sum := 0.0
	for k := 0; k <= maxK; k++ {
		if k > 0 {
			l += math.Log(mu) - math.Log(float64(k))
		}
		p := math.Exp(l)
		pmf[k] = p
		sum += p
	}
	for k := range pmf {
		pmf[k] /= sum // normalize tail
	}
	return pmf
}

func newMarble(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := marbleConfig{
		SimDurationMS: 5_400_000, PlaybackX: 90, HistoryMatches: 5,
		SideCount: 6, WindowStepMS: 8_000, WindowLenMS: 6_000, MainQuoteStep: 5_000,
	}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.PlaybackX <= 0 || cfg.SimDurationMS < 60_000 {
		return nil, fmt.Errorf("invalid marble config")
	}
	rng := engine.NewRNG(ctx.Seed + 0xB7A1)
	g := &marbleGame{ctx: ctx, cfg: cfg, players: map[string]*marblePlayer{}}
	g.tape.PRNG = "splitmix64-v1"
	g.tape.Config = cfg
	for _, p := range ctx.Players {
		g.players[p] = &marblePlayer{books: map[string]*marbleBook{}}
	}

	// Hidden rates per game-minute.
	for i := range g.tape.Rates {
		g.tape.Rates[i] = 0.3 + rng.Float()*0.9 // 0.3 .. 1.2
	}
	lAOr, lARd, lBOr, lBRd := g.tape.Rates[0], g.tape.Rates[1], g.tape.Rates[2], g.tape.Rates[3]

	// Goal tape: independent Poisson processes per team/colour.
	type stream struct {
		team, color string
		lambda      float64
	}
	for _, s := range []stream{{"A", "orange", lAOr}, {"A", "red", lARd}, {"B", "orange", lBOr}, {"B", "red", lBRd}} {
		t := rng.Exp(60_000 / s.lambda)
		for t < float64(cfg.SimDurationMS) {
			g.tape.Goals = append(g.tape.Goals, marbleGoal{TSimMS: int64(t), Team: s.team, Color: s.color})
			t += rng.Exp(60_000 / s.lambda)
		}
	}
	sort.SliceStable(g.tape.Goals, func(i, j int) bool { return g.tape.Goals[i].TSimMS < g.tape.Goals[j].TSimMS })

	// Tournament history under the same team rates vs a 0.75/min opponent.
	for _, team := range []string{"A", "B"} {
		var lambda float64
		if team == "A" {
			lambda = lAOr + lARd
		} else {
			lambda = lBOr + lBRd
		}
		for m := 0; m < cfg.HistoryMatches; m++ {
			g.tape.History = append(g.tape.History, [2]marbleHistoryMatch{
				{Team: poissonSample(rng, lambda*90), Opp: poissonSample(rng, 0.75*90)},
			})
		}
	}

	// Side markets: sequential windows with quotes from the omniscient
	// evaluator at open time, plus seeded mispricing.
	realDur := cfg.simRealDurationMS()
	for i := 0; i < cfg.SideCount && i < len(marbleSideCatalog); i++ {
		spec := marbleSideCatalog[i]
		open := int64(i) * cfg.WindowStepMS
		if open+cfg.WindowLenMS > realDur {
			break
		}
		m := marbleSideMarket{Spec: spec, OpenRealMS: open, CloseRealMS: open + cfg.WindowLenMS}
		simOpen := int64(float64(open) * cfg.PlaybackX)
		aOr, aRd, bOr, bRd := g.totalsAt(simOpen)
		p := g.sideProb(spec, aOr, aRd, bOr, bRd, simOpen)
		E := 100 * p
		sd := 100 * math.Sqrt(p*(1-p))
		off := (rng.Float()*0.7 - 0.35) * sd
		spread := math.Max(2, math.Round(sd*0.1))
		mid := E + off
		m.Bid = int(math.Max(0, math.Round(mid-spread/2)))
		m.Ask = int(math.Min(100, math.Round(mid+spread/2)))
		g.tape.Sides = append(g.tape.Sides, m)
	}

	// MAIN quotes every MainQuoteStep of real time.
	for t := int64(0); t < realDur; t += cfg.MainQuoteStep {
		simT := int64(float64(t) * cfg.PlaybackX)
		aOr, aRd, bOr, bRd := g.totalsAt(simT)
		muA := (lAOr + lARd) * float64(cfg.SimDurationMS-simT) / 60_000
		muB := (lBOr + lBRd) * float64(cfg.SimDurationMS-simT) / 60_000
		fair := (float64(aOr+aRd) + muA) * (float64(bOr+bRd) + muB)
		k := math.Max(2, fair*0.06)
		off := (rng.Float()*2 - 1) * k
		mid := fair + off
		q := struct {
			TRealMS int64 `json:"t_real_ms"`
			Bid     int   `json:"bid"`
			Ask     int   `json:"ask"`
		}{TRealMS: t, Bid: int(math.Max(1, math.Round(mid-1))), Ask: int(math.Max(3, math.Round(mid + 1)))}
		g.tape.MainQuotes = append(g.tape.MainQuotes, q)
	}
	return g, nil
}

func (c marbleConfig) simRealDurationMS() int64 {
	return int64(float64(c.SimDurationMS) / c.PlaybackX)
}

func (g *marbleGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "marble", Name: "The Marble Olympics", Access: "premium", MinPlayers: 1, MaxPlayers: 300,
		DurationMS: g.cfg.simRealDurationMS(),
		Description: "Trade final-count contracts during a simulated marble football final.",
		Rules: "MAIN pays (final A) x (final B); side contracts pay 100 x indicator. Historical tournament scores hint at hidden Poisson scoring rates. Side markets open in sequential windows. Edge points at trade time: sign(mispricing) x min(4, ceil(|d|/sd)) per side trade. Pausing freezes the simulation clock and market windows together.",
	}
}

// totalsAt counts goals up to simNow.
func (g *marbleGame) totalsAt(simNow int64) (aOr, aRd, bOr, bRd int) {
	for _, gl := range g.tape.Goals {
		if gl.TSimMS > simNow {
			break
		}
		switch {
		case gl.Team == "A" && gl.Color == "orange":
			aOr++
		case gl.Team == "A":
			aRd++
		case gl.Color == "orange":
			bOr++
		default:
			bRd++
		}
	}
	return
}

func (g *marbleGame) simNow() int64 {
	return int64(float64(g.ctx.ElapsedMS()) * g.cfg.PlaybackX)
}

func (g *marbleGame) sideProb(spec marbleSideSpec, aOr, aRd, bOr, bRd int, simNow int64) float64 {
	remMin := float64(g.cfg.SimDurationMS-simNow) / 60_000
	lAOr, lARd, lBOr, lBRd := g.tape.Rates[0], g.tape.Rates[1], g.tape.Rates[2], g.tape.Rates[3]
	pmf := func(mu float64, cur int, target int, ge bool) float64 {
		dist := poissonPMF(mu, 40)
		p := 0.0
		for k, pk := range dist {
			final := cur + k
			if (ge && final >= target) || (!ge && final >= target) {
				p += pk
			}
		}
		return p
	}
	switch spec.Kind {
	case "A_TOTAL_GE":
		return pmf((lAOr+lARd)*remMin, aOr+aRd, spec.Arg, true)
	case "B_TOTAL_GE":
		return pmf((lBOr+lBRd)*remMin, bOr+bRd, spec.Arg, true)
	case "A_ORANGE_GE":
		return pmf(lAOr*remMin, aOr, spec.Arg, true)
	case "B_ORANGE_GE":
		return pmf(lBOr*remMin, bOr, spec.Arg, true)
	case "TOTAL_GE":
		return pmf((lAOr+lARd+lBOr+lBRd)*remMin, aOr+aRd+bOr+bRd, spec.Arg, true)
	}
	return 0
}

func (g *marbleGame) version() int {
	// Main quote step count + 2 per fully elapsed side window + 1 per open one.
	v := 1 + g.qIdx
	now := g.ctx.ElapsedMS()
	for _, m := range g.tape.Sides {
		if now >= m.OpenRealMS {
			v++
		}
		if now >= m.CloseRealMS {
			v++
		}
	}
	return v
}

func (g *marbleGame) mainQuote() (int, int) {
	idx := int(g.ctx.ElapsedMS() / g.cfg.MainQuoteStep)
	if idx >= len(g.tape.MainQuotes) {
		idx = len(g.tape.MainQuotes) - 1
	}
	if g.qIdx < idx {
		g.qIdx = idx
	}
	q := g.tape.MainQuotes[idx]
	return q.Bid, q.Ask
}

func (g *marbleGame) openSide() *marbleSideMarket {
	now := g.ctx.ElapsedMS()
	for i := range g.tape.Sides {
		if now >= g.tape.Sides[i].OpenRealMS && now < g.tape.Sides[i].CloseRealMS {
			return &g.tape.Sides[i]
		}
	}
	return nil
}

func (g *marbleGame) Tick() { g.tick() }

func (g *marbleGame) tick() {
	if !g.done && g.ctx.ElapsedMS() >= g.cfg.simRealDurationMS() {
		g.done = true
	}
}

func (g *marbleGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	p, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	simNow := g.simNow()
	aOr, aRd, bOr, bRd := g.totalsAt(simNow)
	mb, ma := g.mainQuote()
	remaining := g.cfg.simRealDurationMS() - g.ctx.ElapsedMS()
	if remaining < 0 {
		remaining = 0
	}
	hist := make([]map[string]any, 0, len(g.tape.History))
	for _, h := range g.tape.History {
		hist = append(hist, map[string]any{"team": h[0].Team, "opponent": h[0].Opp})
	}
	sides := make([]map[string]any, 0, len(g.tape.Sides))
	now := g.ctx.ElapsedMS()
	for _, m := range g.tape.Sides {
		status := "PENDING"
		if now >= m.CloseRealMS {
			status = "CLOSED"
		} else if now >= m.OpenRealMS {
			status = "OPEN"
		}
		entry := map[string]any{"id": m.Spec.ID, "desc": m.Spec.Desc, "status": status}
		if status == "OPEN" {
			entry["bid"] = m.Bid
			entry["ask"] = m.Ask
			entry["closes_in_ms"] = m.CloseRealMS - now
		}
		sides = append(sides, entry)
	}
	book := map[string]any{}
	for id, b := range p.books {
		book[id] = map[string]any{"cash": engine.Round2(b.C), "position": b.Q}
	}
	return engine.MustJSON(map[string]any{
		"match_clock_game_min": float64(simNow) / 60_000,
		"time_remaining_ms": remaining,
		"history": hist,
		"totals":  map[string]any{"A": map[string]int{"orange": aOr, "red": aRd}, "B": map[string]int{"orange": bOr, "red": bRd}},
		"main":    map[string]any{"id": "MAIN", "desc": "Pays (final A total) x (final B total)", "bid": mb, "ask": ma},
		"sides":   sides,
		"market_version": g.version(),
		"my_book": book,
		"my_edge_points": p.edgeSum,
		"my_trades": p.trades,
		"finished": g.done,
	}), nil
}

func (g *marbleGame) edgePointsFor(spec marbleSideSpec, side string, price int, simNow int64) int {
	aOr, aRd, bOr, bRd := g.totalsAt(simNow)
	p := g.sideProb(spec, aOr, aRd, bOr, bRd, simNow)
	E := 100 * p
	sd := 100 * math.Sqrt(p*(1-p))
	var d float64
	if side == "BUY" {
		d = E - float64(price)
	} else {
		d = float64(price) - E
	}
	if sd <= 0 {
		if d > 0 {
			return 4
		} else if d < 0 {
			return -4
		}
		return 0
	}
	sign := 1
	if d < 0 {
		sign = -1
	}
	return sign * int(math.Min(4, math.Ceil(math.Abs(d)/sd)))
}

func (g *marbleGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	g.mainQuote() // keep the quote cursor and version in sync
	p, ok := g.players[req.PlayerID]
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if g.done {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	if req.Action != "BUY" && req.Action != "SELL" {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
	}
	var pl struct {
		ContractID string `json:"contract_id"`
		Quantity   int    `json:"quantity"`
	}
	if err := json.Unmarshal(req.Payload, &pl); err != nil || pl.Quantity != 1 {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	if req.MarketVersion != 0 && req.MarketVersion != g.version() {
		return engine.Output{Accepted: false, Reason: engine.ReasonStaleMarket}
	}
	simNow := g.simNow()
	var price int
	var spec marbleSideSpec
	isSide := false
	if pl.ContractID == "MAIN" {
		price = g.tape.MainQuotes[g.qIdx].Bid
		if req.Action == "BUY" {
			price = g.tape.MainQuotes[g.qIdx].Ask
		}
	} else {
		m := g.openSide()
		if m == nil || m.Spec.ID != pl.ContractID {
			return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
		}
		isSide = true
		spec = m.Spec
		price = m.Bid
		if req.Action == "BUY" {
			price = m.Ask
		}
	}
	book, okB := p.books[pl.ContractID]
	if !okB {
		book = &marbleBook{}
		p.books[pl.ContractID] = book
	}
	if req.Action == "BUY" {
		book.C -= float64(price)
		book.Q++
	} else {
		book.C += float64(price)
		book.Q--
	}
	edge := 0
	if isSide {
		edge = g.edgePointsFor(spec, req.Action, price, simNow)
		p.edgeSum += edge
	}
	p.trades = append(p.trades, marbleTradeRec{Contract: pl.ContractID, Side: req.Action, Price: price, EdgePts: edge})
	return engine.Output{
		Accepted: true,
		Execution: engine.MustJSON(map[string]any{
			"contract": pl.ContractID, "side": req.Action, "price": price, "edge_points": edge,
		}),
		PrivateChanges: engine.MustJSON(map[string]any{
			"book": map[string]any{"cash": engine.Round2(book.C), "position": book.Q},
			"edge_points": p.edgeSum,
		}),
	}
}

func (g *marbleGame) Finished() bool { g.tick(); return g.done }

func (g *marbleGame) Results() (json.RawMessage, error) {
	g.tick()
	simNow := g.cfg.SimDurationMS
	aOr, aRd, bOr, bRd := g.totalsAt(simNow)
	finalA, finalB := aOr+aRd, bOr+bRd
	payouts := map[string]int{"MAIN": finalA * finalB}
	for _, m := range g.tape.Sides {
		pay := 0
		switch m.Spec.Kind {
		case "A_TOTAL_GE":
			if finalA >= m.Spec.Arg {
				pay = 100
			}
		case "B_TOTAL_GE":
			if finalB >= m.Spec.Arg {
				pay = 100
			}
		case "A_ORANGE_GE":
			if aOr >= m.Spec.Arg {
				pay = 100
			}
		case "B_ORANGE_GE":
			if bOr >= m.Spec.Arg {
				pay = 100
			}
		case "TOTAL_GE":
			if finalA+finalB >= m.Spec.Arg {
				pay = 100
			}
		}
		payouts[m.Spec.ID] = pay
	}
	type row struct {
		Player    string          `json:"player"`
		Profit    float64         `json:"profit"`
		EdgeSum   int             `json:"edge_sum"`
		RankScore int             `json:"rank_score"`
		Rank      int             `json:"rank"`
		Global    int             `json:"global_points"`
		Books     map[string]int  `json:"settled_positions"`
	}
	rows := make([]row, 0, len(g.players))
	for name, pl := range g.players {
		profit := 0.0
		settled := map[string]int{}
		for id, b := range pl.books {
			pay := payouts[id]
			profit += b.C + float64(b.Q)*float64(pay)
			settled[id] = b.Q
		}
		rows = append(rows, row{Player: name, Profit: engine.Round2(profit), EdgeSum: pl.edgeSum, Books: settled})
	}
	// Rank score: 2 per opponent with fewer edge points, 1 with lower profit.
	for i := range rows {
		rs := 0
		for j := range rows {
			if j == i {
				continue
			}
			if rows[j].EdgeSum < rows[i].EdgeSum {
				rs += 2
			}
			if rows[j].Profit < rows[i].Profit {
				rs++
			}
		}
		rows[i].RankScore = rs
	}
	for i := range rows {
		r := 1
		for j := range rows {
			if rows[j].RankScore > rows[i].RankScore ||
				(rows[j].RankScore == rows[i].RankScore && rows[j].EdgeSum > rows[i].EdgeSum) {
				r++
			}
		}
		rows[i].Rank = r
	}
	solo := len(rows) <= 1
	for i := range rows {
		if solo {
			if rows[i].Profit > 0 {
				rows[i].Global = 1
			}
			continue
		}
		outranked := 0
		for j := range rows {
			if rows[j].RankScore < rows[i].RankScore {
				outranked++
			}
		}
		if rows[i].EdgeSum > 0 {
			rows[i].Global = outranked
		}
	}
	return engine.MustJSON(map[string]any{
		"finished": true,
		"final_totals": map[string]any{
			"A": map[string]int{"orange": aOr, "red": aRd, "total": finalA},
			"B": map[string]int{"orange": bOr, "red": bRd, "total": finalB},
		},
		"payouts": payouts,
		"players": rows,
	}), nil
}

func (g *marbleGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }
