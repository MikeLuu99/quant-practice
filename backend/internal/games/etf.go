package games

import (
	"encoding/json"
	"fmt"
	"math"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// ETF Trading Challenge.
//
// Each ETF holds unit weights of 2-4 stocks. BUY_ETF buys the ETF and sells
// the basket atomically; SELL_ETF does the reverse. After any fill, quotes
// reset to the next tape snapshot and the shared market version advances.
// ---------------------------------------------------------------------------

type etfQuote struct {
	Bid int `json:"bid"`
	Ask int `json:"ask"`
}

type etfSnap struct {
	T      int64      `json:"t_ms"`
	Stocks []etfQuote `json:"stocks"`
	ETF    etfQuote   `json:"etf"`
	Tag    string     `json:"tag"` // none | news | normal
}

type etfTape struct {
	PRNG   string     `json:"prng"`
	Config etfConfig  `json:"config"`
	Snaps  []etfSnap  `json:"snaps"`
}

type etfConfig struct {
	DurationMS      int64   `json:"duration_ms"`
	StockCount      int     `json:"stock_count"` // 2..4
	Fee             int     `json:"fee"`
	QuoteStepMS     int64   `json:"quote_step_ms"`
	ArbProbability  float64 `json:"arb_probability"`
	NewsProbability float64 `json:"news_probability"`
}

type etfLedger struct {
	Seq     int     `json:"seq"`
	Side    string  `json:"side"`
	Profit  float64 `json:"profit"`
	Tag     string  `json:"tag"`
	Legs    []map[string]any `json:"legs"`
}

type etfPlayer struct {
	profit     float64
	newsProfit float64
	normProfit float64
	trades     int
	profitable int
	ledger     []etfLedger
}

type etfGame struct {
	ctx    *engine.Ctx
	cfg    etfConfig
	tape   etfTape
	snap   int // index of the active snapshot == market_version
	stocks []int // weights: unit
	players map[string]*etfPlayer
	done   bool
}

func newETF(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := etfConfig{
		DurationMS: 300_000, StockCount: 3, Fee: 0, QuoteStepMS: 5_000,
		ArbProbability: 0.25, NewsProbability: 0.35,
	}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.StockCount < 2 || cfg.StockCount > 4 || cfg.QuoteStepMS < 500 {
		return nil, fmt.Errorf("invalid etf config")
	}
	rng := engine.NewRNG(ctx.Seed + 0xE17)
	g := &etfGame{ctx: ctx, cfg: cfg, players: map[string]*etfPlayer{}, snap: 0}
	g.tape.PRNG = "splitmix64-v1"
	g.tape.Config = cfg
	for _, p := range ctx.Players {
		g.players[p] = &etfPlayer{}
	}

	// Random walk with persistent drift per stock.
	mids := make([]int, cfg.StockCount)
	drift := make([]int, cfg.StockCount)
	for i := range mids {
		mids[i] = rng.Range(40, 60)
		drift[i] = rng.Range(-1, 1)
	}
	nSnaps := int(cfg.DurationMS/cfg.QuoteStepMS) + 1
	for s := 0; s < nSnaps; s++ {
		snap := etfSnap{T: int64(s) * cfg.QuoteStepMS}
		for i := range mids {
			step := rng.Range(-2, 2) + drift[i]
			mids[i] = int(math.Max(5, float64(mids[i]+step)))
			snap.Stocks = append(snap.Stocks, etfQuote{Bid: mids[i] - 1, Ask: mids[i] + 1})
		}
		basketBid, basketAsk := 0, 0
		for _, q := range snap.Stocks {
			basketBid += q.Bid
			basketAsk += q.Ask
		}
		// Occasionally misprice the ETF to create an instant arbitrage cycle.
		switch roll := rng.Float(); {
		case roll < cfg.ArbProbability:
			dir := rng.Int(2)
			if dir == 0 { // buy arb: ETF ask below basket bid
				snap.ETF.Ask = basketBid - 1
				snap.ETF.Bid = snap.ETF.Ask - 2
			} else { // sell arb: ETF bid above basket ask
				snap.ETF.Bid = basketAsk + 1
				snap.ETF.Ask = snap.ETF.Bid + 2
			}
			if rng.Float() < cfg.NewsProbability {
				snap.Tag = "news"
			} else {
				snap.Tag = "normal"
			}
		default:
			mid := (basketBid + basketAsk) / 2
			mid += rng.Range(-2, 2)
			snap.ETF = etfQuote{Bid: mid - 1, Ask: mid + 1}
			snap.Tag = "none"
		}
		g.tape.Snaps = append(g.tape.Snaps, snap)
	}
	return g, nil
}

func (g *etfGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "etf", Name: "ETF Trading Challenge", Access: "premium", MinPlayers: 1, MaxPlayers: 50,
		DurationMS: g.cfg.DurationMS,
		Description: "ETFs track stock baskets. Buy the cheap side, sell the expensive side, atomically.",
		Rules: "BUY_ETF buys 1 ETF at its ask and instantly sells its basket at the stock bids; SELL_ETF reverses it. Instant profit = sum(stock bids) - ETF ask (minus fees) for buys, ETF bid - sum(stock asks) for sells. Execution is atomic at the validated snapshot; after a fill, quotes reset to the next tape snapshot and the shared market version advances, so stale clicks fail with STALE_MARKET. Fills are tagged news or normal from the tape.",
	}
}

func (g *etfGame) tick() {
	if !g.done && g.ctx.ElapsedMS() >= g.cfg.DurationMS {
		g.done = true
	}
}

func (g *etfGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	p, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	s := g.tape.Snaps[g.snap]
	stocks := make([]map[string]any, 0, len(s.Stocks))
	for i, q := range s.Stocks {
		stocks = append(stocks, map[string]any{"id": fmt.Sprintf("S%d", i+1), "bid": q.Bid, "ask": q.Ask})
	}
	remaining := g.cfg.DurationMS - g.ctx.ElapsedMS()
	if remaining < 0 {
		remaining = 0
	}
	acc := 0.0
	if p.trades > 0 {
		acc = float64(p.profitable) / float64(p.trades)
	}
	return engine.MustJSON(map[string]any{
		"stocks": stocks,
		"etf":    map[string]any{"id": "E1", "bid": s.ETF.Bid, "ask": s.ETF.Ask, "constituents": len(s.Stocks), "weights": "unit"},
		"news":   s.Tag,
		"market_version": g.snap,
		"time_remaining_ms": remaining,
		"finished": g.done,
		"me": map[string]any{
			"total_profit": engine.Round2(p.profit),
			"news_profit": engine.Round2(p.newsProfit),
			"normal_profit": engine.Round2(p.normProfit),
			"trades": p.trades,
			"profitable_trades": p.profitable,
			"accuracy": engine.Round2(acc),
		},
	}), nil
}

func (g *etfGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	p, ok := g.players[req.PlayerID]
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if g.done {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	if req.Action != "BUY_ETF" && req.Action != "SELL_ETF" {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
	}
	var pl struct {
		ETFID    string `json:"etf_id"`
		Quantity int    `json:"quantity"`
	}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &pl); err != nil {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
	}
	if pl.ETFID != "" && pl.ETFID != "E1" {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	if pl.Quantity != 1 {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	// Stale market version: a price update must never fill an old click.
	if req.MarketVersion != g.snap {
		return engine.Output{Accepted: false, Reason: engine.ReasonStaleMarket}
	}
	s := g.tape.Snaps[g.snap]
	basketBid, basketAsk := 0, 0
	for _, q := range s.Stocks {
		basketBid += q.Bid
		basketAsk += q.Ask
	}
	var profit int
	legs := []map[string]any{}
	if req.Action == "BUY_ETF" {
		profit = basketBid - s.ETF.Ask - g.cfg.Fee
		legs = append(legs, map[string]any{"leg": "BUY", "instrument": "E1", "price": s.ETF.Ask, "quantity": 1})
		for i, q := range s.Stocks {
			legs = append(legs, map[string]any{"leg": "SELL", "instrument": fmt.Sprintf("S%d", i+1), "price": q.Bid, "quantity": 1})
		}
	} else {
		profit = s.ETF.Bid - basketAsk - g.cfg.Fee
		legs = append(legs, map[string]any{"leg": "SELL", "instrument": "E1", "price": s.ETF.Bid, "quantity": 1})
		for i, q := range s.Stocks {
			legs = append(legs, map[string]any{"leg": "BUY", "instrument": fmt.Sprintf("S%d", i+1), "price": q.Ask, "quantity": 1})
		}
	}
	tag := s.Tag
	if tag != "news" {
		tag = "normal"
	}
	p.profit += float64(profit)
	if tag == "news" {
		p.newsProfit += float64(profit)
	} else {
		p.normProfit += float64(profit)
	}
	p.trades++
	if profit > 0 {
		p.profitable++
	}
	// Quote reset: advance to the next tape snapshot for everyone.
	if g.snap+1 < len(g.tape.Snaps) {
		g.snap++
	}
	acc := 0.0
	if p.trades > 0 {
		acc = float64(p.profitable) / float64(p.trades)
	}
	return engine.Output{
		Accepted:    true,
		RoundReward: float64(profit),
		Execution:   engine.MustJSON(map[string]any{"side": req.Action, "profit": profit, "legs": legs, "tag": tag}),
		PrivateChanges: engine.MustJSON(map[string]any{
			"total_profit": engine.Round2(p.profit), "news_profit": engine.Round2(p.newsProfit),
			"normal_profit": engine.Round2(p.normProfit), "trades": p.trades,
			"profitable_trades": p.profitable, "accuracy": engine.Round2(acc),
		}),
		PublicChanges: engine.MustJSON(map[string]any{"market_version": g.snap}),
	}
}

func (g *etfGame) Finished() bool { g.tick(); return g.done }

func (g *etfGame) Results() (json.RawMessage, error) {
	g.tick()
	type row struct {
		Player       string  `json:"player"`
		TotalProfit  float64 `json:"total_profit"`
		NewsProfit   float64 `json:"news_profit"`
		NormalProfit float64 `json:"normal_profit"`
		Trades       int     `json:"trades"`
		Profitable   int     `json:"profitable_trades"`
		Accuracy     float64 `json:"accuracy"`
		Rank         int     `json:"rank"`
		GlobalPoints int     `json:"global_points"`
	}
	rows := make([]row, 0, len(g.players))
	for name, p := range g.players {
		acc := 0.0
		if p.trades > 0 {
			acc = float64(p.profitable) / float64(p.trades)
		}
		rows = append(rows, row{
			Player: name, TotalProfit: engine.Round2(p.profit), NewsProfit: engine.Round2(p.newsProfit),
			NormalProfit: engine.Round2(p.normProfit), Trades: p.trades, Profitable: p.profitable,
			Accuracy: engine.Round2(acc),
		})
	}
	for i := range rows {
		r := 1
		for j := range rows {
			if rows[j].TotalProfit > rows[i].TotalProfit {
				r++
			}
		}
		rows[i].Rank = r
	}
	solo := len(rows) <= 1
	for i := range rows {
		// Zero trades -> accuracy and global points are zero.
		if rows[i].Trades == 0 {
			continue
		}
		if solo {
			continue
		}
		rT, rN, rNorm := 0, 0, 0
		for j := range rows {
			if j == i {
				continue
			}
			if rows[j].TotalProfit < rows[i].TotalProfit {
				rT++
			}
			if rows[j].NewsProfit < rows[i].NewsProfit {
				rN++
			}
			if rows[j].NormalProfit < rows[i].NormalProfit {
				rNorm++
			}
		}
		rows[i].GlobalPoints = int(math.Ceil(float64(rT+rN+2*rNorm) * rows[i].Accuracy))
	}
	return engine.MustJSON(map[string]any{"finished": true, "players": rows}), nil
}

func (g *etfGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }
