package games

import (
	"encoding/json"
	"fmt"
	"sort"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Market of Cards.
//
// Each player holds two private cards; center cards are revealed after each
// complete maker rotation. The active maker quotes bid/ask; the first valid
// responder trade takes one unit and closes that market. Settlement value is
// the sum of all dealt card ranks (A=1 .. K=13). Peer-to-peer accounting is
// exactly zero-sum.
// ---------------------------------------------------------------------------

type cardsConfig struct {
	PrepMS      int64 `json:"prep_ms"`
	QuoteMS     int64 `json:"quote_timer_ms"`
	ResponseMS  int64 `json:"response_timer_ms"`
	CenterCount int   `json:"center_count"`
	MaxSpread   int   `json:"max_spread"`
}

type cardT struct {
	Rank int `json:"rank"` // 1..13
	Suit int `json:"suit"` // 0..3
}

func (c cardT) label() string {
	ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	suits := []string{"S", "H", "D", "C"}
	return ranks[c.Rank-1] + suits[c.Suit]
}

type cardsTape struct {
	PRNG   string             `json:"prng"`
	Config cardsConfig        `json:"config"`
	Deck   []cardT            `json:"deck_order"`
	Hands  map[string][]cardT `json:"hands"`
	Center []cardT            `json:"center"`
}

type cardsBook struct {
	C int
	Q int
}

type cardsGame struct {
	ctx          *engine.Ctx
	cfg          cardsConfig
	tape         cardsTape
	turn         int    // global maker turn index
	phase        string // PREP | QUOTE | RESPONSE | DONE
	phaseStartMS int64
	quote        struct {
		Bid, Ask int
		Set      bool
	}
	quoteMaker  string
	filled      bool
	responded   map[string]bool
	respondSeq  map[string]int64
	revealed    int
	books       map[string]*cardsBook
	ledger      []map[string]any
	done        bool
	bots        map[string]bool
	playerOrder []string
}

func newCards(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := cardsConfig{PrepMS: 10_000, QuoteMS: 20_000, ResponseMS: 12_000, CenterCount: 3, MaxSpread: 20}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	n := len(ctx.Players)
	if n < 1 || 2*n+cfg.CenterCount > 52 {
		return nil, fmt.Errorf("need 2N+%d <= 52 players*cards", cfg.CenterCount)
	}
	rng := engine.NewRNG(ctx.Seed + 0xCA12)
	g := &cardsGame{
		ctx: ctx, cfg: cfg, phase: "PREP", phaseStartMS: 0,
		responded: map[string]bool{}, respondSeq: map[string]int64{},
		books: map[string]*cardsBook{}, bots: map[string]bool{},
	}
	g.tape.PRNG = "splitmix64-v1"
	g.tape.Config = cfg

	deck := make([]cardT, 52)
	for i := range deck {
		deck[i] = cardT{Rank: i%13 + 1, Suit: i / 13}
	}
	rng.Shuffle(52, func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	g.tape.Deck = append([]cardT(nil), deck...)

	g.playerOrder = append([]string(nil), ctx.Players...)
	sort.Strings(g.playerOrder)
	// Add bots when a lone human needs counterparties.
	if n == 1 {
		for i := 1; len(g.playerOrder) < 3; i++ {
			name := fmt.Sprintf("bot%d", i)
			if name == ctx.Players[0] {
				continue
			}
			g.playerOrder = append(g.playerOrder, name)
			g.bots[name] = true
		}
	}
	idx := 0
	g.tape.Hands = map[string][]cardT{}
	for _, p := range g.playerOrder {
		g.tape.Hands[p] = []cardT{deck[idx], deck[idx+1]}
		idx += 2
	}
	for k := 0; k < cfg.CenterCount; k++ {
		g.tape.Center = append(g.tape.Center, deck[idx+k])
	}
	for _, p := range g.playerOrder {
		g.books[p] = &cardsBook{}
	}
	return g, nil
}

func (g *cardsGame) Meta() engine.GameMeta {
	n := len(g.playerOrder)
	if n == 0 {
		n = len(g.ctx.Players)
	}
	return engine.GameMeta{
		Slug: "cards", Name: "Market of Cards", Access: "premium", MinPlayers: 1, MaxPlayers: 15,
		DurationMS:  g.cfg.PrepMS + int64(n)*int64(g.cfg.CenterCount)*(g.cfg.QuoteMS+g.cfg.ResponseMS),
		Description: "Quote or trade a hidden card-total contract; center cards reveal progressively.",
		Rules:       "Everyone holds 2 private cards; the contract settles at the sum of ALL dealt card ranks (A=1..K=13). The active maker quotes bid<=ask within the spread cap; the first valid responder trade takes one unit and closes the market; passes do not close it. After each complete maker rotation one center card is revealed. Peer-to-peer settlement is zero-sum. A lone human is joined by two bots.",
	}
}

func (g *cardsGame) handValue(p string) int {
	v := 0
	for _, c := range g.tape.Hands[p] {
		v += c.Rank
	}
	return v
}

func (g *cardsGame) totalDealtValue() int {
	v := 0
	for _, hand := range g.tape.Hands {
		for _, c := range hand {
			v += c.Rank
		}
	}
	for _, c := range g.tape.Center {
		v += c.Rank
	}
	return v
}

func (g *cardsGame) activeMaker() string {
	if g.turn >= len(g.playerOrder)*g.cfg.CenterCount {
		return ""
	}
	return g.playerOrder[g.turn%len(g.playerOrder)]
}

func (g *cardsGame) botEstimate(bot string) int {
	est := g.handValue(bot)
	for i := 0; i < g.revealed; i++ {
		est += g.tape.Center[i].Rank
	}
	unknown := 2*(len(g.playerOrder)-1) + (g.cfg.CenterCount - g.revealed)
	est += 7 * unknown
	return est
}

func (g *cardsGame) Tick() { g.tick() }

func (g *cardsGame) tick() {
	if g.done {
		return
	}
	now := g.ctx.ElapsedMS()
	switch g.phase {
	case "PREP":
		if now-g.phaseStartMS >= g.cfg.PrepMS {
			g.startTurn(now)
		}
	case "QUOTE":
		if g.bots[g.activeMaker()] && !g.quote.Set {
			g.botQuote(now)
		}
		if now-g.phaseStartMS >= g.cfg.QuoteMS {
			if !g.quote.Set {
				g.nextTurn(now) // missed quote: no trade
			}
		}
	case "RESPONSE":
		for _, p := range g.playerOrder {
			if g.bots[p] && p != g.activeMaker() && !g.responded[p] && !g.filled {
				g.botRespond(p, now)
			}
		}
		if now-g.phaseStartMS >= g.cfg.ResponseMS {
			g.nextTurn(now)
		}
	}
}

func (g *cardsGame) startTurn(now int64) {
	g.phase = "QUOTE"
	g.phaseStartMS = now
	g.quote.Set = false
	g.filled = false
	g.responded = map[string]bool{}
	g.respondSeq = map[string]int64{}
}

func (g *cardsGame) nextTurn(now int64) {
	g.turn++
	if g.turn%len(g.playerOrder) == 0 {
		// Rotation complete: reveal a center card.
		g.revealed++
		if g.revealed >= g.cfg.CenterCount {
			g.phase = "DONE"
			g.done = true
			return
		}
	}
	g.startTurn(now)
}

func (g *cardsGame) botQuote(now int64) {
	est := g.botEstimate(g.activeMaker())
	bid := est - 4
	ask := est + 4
	if bid < 1 {
		bid = 1
	}
	if ask-bid > g.cfg.MaxSpread {
		ask = bid + g.cfg.MaxSpread
	}
	g.applyQuote(bid, ask, g.activeMaker(), now)
}

func (g *cardsGame) botRespond(p string, now int64) {
	est := g.botEstimate(p)
	if g.quote.Set && g.quote.Ask < est-1 {
		g.fill(p, "BUY", g.quote.Ask, now)
		return
	}
	if g.quote.Set && g.quote.Bid > est+1 {
		g.fill(p, "SELL", g.quote.Bid, now)
		return
	}
	g.responded[p] = true
}

func (g *cardsGame) applyQuote(bid, ask int, maker string, now int64) engine.Output {
	if bid < 1 || bid > ask || ask-bid > g.cfg.MaxSpread {
		return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
	}
	if g.phase != "QUOTE" || g.activeMaker() != maker || g.quote.Set {
		return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
	}
	g.quote.Bid, g.quote.Ask, g.quote.Set = bid, ask, true
	g.quoteMaker = maker
	g.phase = "RESPONSE"
	g.phaseStartMS = now
	return engine.Output{
		Accepted:      true,
		Execution:     engine.MustJSON(map[string]any{"bid": bid, "ask": ask, "maker": maker}),
		PublicChanges: engine.MustJSON(map[string]any{"phase": "RESPONSE"}),
	}
}

// fill executes the first valid responder trade and closes the market.
func (g *cardsGame) fill(responder, side string, price int, now int64) engine.Output {
	if g.filled || g.phase != "RESPONSE" || !g.quote.Set {
		return engine.Output{Accepted: false, Reason: engine.ReasonNoCapacity}
	}
	if responder == g.quoteMaker {
		return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
	}
	if g.responded[responder] {
		return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
	}
	maker := g.quoteMaker
	mb, rb := g.books[maker], g.books[responder]
	if side == "BUY" { // responder goes long at the ask
		rb.C -= price
		rb.Q++
		mb.C += price
		mb.Q--
	} else { // responder goes short at the bid
		rb.C += price
		rb.Q--
		mb.C -= price
		mb.Q++
	}
	g.filled = true
	g.responded[responder] = true
	g.respondSeq[responder] = now
	g.ledger = append(g.ledger, map[string]any{
		"maker": maker, "responder": responder, "side": side, "price": price, "at_ms": now,
	})
	g.nextTurn(now)
	return engine.Output{
		Accepted:    true,
		RoundReward: 0,
		Execution:   engine.MustJSON(map[string]any{"side": side, "price": price, "quantity": 1}),
		PrivateChanges: engine.MustJSON(map[string]any{
			"cash": rb.C, "position": rb.Q, "market_closed": true,
		}),
		PublicChanges: engine.MustJSON(map[string]any{"trade": g.ledger[len(g.ledger)-1]}),
	}
}

func (g *cardsGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	b, ok := g.books[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	remaining := int64(0)
	switch g.phase {
	case "PREP":
		remaining = g.cfg.PrepMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	case "QUOTE":
		remaining = g.cfg.QuoteMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	case "RESPONSE":
		remaining = g.cfg.ResponseMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	}
	if remaining < 0 {
		remaining = 0
	}
	revealedCards := []map[string]any{}
	for i := 0; i < g.revealed; i++ {
		revealedCards = append(revealedCards, map[string]any{"label": g.tape.Center[i].label(), "rank": g.tape.Center[i].Rank})
	}
	snap := map[string]any{
		"phase":             g.phase,
		"time_remaining_ms": remaining,
		"active_maker":      g.activeMaker(),
		"rotation":          g.turn/len(g.playerOrder) + 1,
		"turn_in_rotation":  g.turn%len(g.playerOrder) + 1,
		"players":           g.playerOrder,
		"center_count":      g.cfg.CenterCount,
		"max_spread":        g.cfg.MaxSpread,
		"responded":         g.responded[playerID],
		"revealed_center":   revealedCards,
		"revealed_count":    g.revealed,
		"market_version":    g.turn*2 + boolToInt(g.quote.Set),
		"finished":          g.done,
		"my_hand":           handJSON(g.tape.Hands[playerID]),
		"my_book":           map[string]any{"cash": b.C, "position": b.Q},
		"ledger":            g.ledger,
	}
	if g.quote.Set {
		snap["quote"] = map[string]any{"bid": g.quote.Bid, "ask": g.quote.Ask, "maker": g.quoteMaker}
	}
	return engine.MustJSON(snap), nil
}

func handJSON(hand []cardT) []map[string]any {
	out := make([]map[string]any, 0, len(hand))
	for _, c := range hand {
		out = append(out, map[string]any{"label": c.label(), "rank": c.Rank})
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (g *cardsGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	if _, ok := g.books[req.PlayerID]; !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if g.done {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	switch req.Action {
	case "QUOTE":
		var pl struct {
			Bid, Ask int
		}
		if err := json.Unmarshal(req.Payload, &pl); err != nil {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		return g.applyQuote(pl.Bid, pl.Ask, req.PlayerID, g.ctx.ElapsedMS())
	case "BUY", "SELL":
		var pl struct {
			Quantity int `json:"quantity"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &pl)
		}
		if pl.Quantity != 1 {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		price := g.quote.Bid
		if req.Action == "BUY" {
			price = g.quote.Ask
		}
		return g.fill(req.PlayerID, req.Action, price, g.ctx.ElapsedMS())
	case "PASS":
		if g.phase != "RESPONSE" || req.PlayerID == g.quoteMaker {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		if g.responded[req.PlayerID] {
			return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
		}
		g.responded[req.PlayerID] = true
		return engine.Output{Accepted: true, PrivateChanges: engine.MustJSON(map[string]any{"passed": true})}
	}
	return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
}

func (g *cardsGame) Finished() bool { g.tick(); return g.done }

func (g *cardsGame) Results() (json.RawMessage, error) {
	g.tick()
	V := g.totalDealtValue()
	type row struct {
		Player   string `json:"player"`
		Cash     int    `json:"cash"`
		Position int    `json:"position"`
		Profit   int    `json:"profit"`
		IsBot    bool   `json:"is_bot"`
		Rank     int    `json:"rank"`
		Global   int    `json:"global_points"`
	}
	rows := make([]row, 0, len(g.books))
	for name, b := range g.books {
		rows = append(rows, row{Player: name, Cash: b.C, Position: b.Q, Profit: b.C + b.Q*V, IsBot: g.bots[name]})
	}
	for i := range rows {
		r := 1
		for j := range rows {
			if rows[j].Profit > rows[i].Profit {
				r++
			}
		}
		rows[i].Rank = r
	}
	// Global: 2 per bot outranked, 3 per human outranked.
	for i := range rows {
		pts := 0
		for j := range rows {
			if rows[j].Profit < rows[i].Profit {
				if rows[j].IsBot {
					pts += 2
				} else {
					pts += 3
				}
			}
		}
		rows[i].Global = pts
	}
	// Zero-sum check: sum of profits must be exactly 0.
	sum := 0
	for _, r := range rows {
		sum += r.Profit
	}
	if sum != 0 {
		// Defensive: never publish a broken ledger.
		return nil, fmt.Errorf("zero-sum violated: %d", sum)
	}
	center := []map[string]any{}
	for _, c := range g.tape.Center {
		center = append(center, map[string]any{"label": c.label(), "rank": c.Rank})
	}
	hands := map[string]any{}
	for p, h := range g.tape.Hands {
		hands[p] = handJSON(h)
	}
	return engine.MustJSON(map[string]any{
		"finished": true,
		"value":    V,
		"hands":    hands,
		"center":   center,
		"players":  rows,
		"ledger":   g.ledger,
	}), nil
}

func (g *cardsGame) Tape() (json.RawMessage, error) { return engine.MustJSON(g.tape), nil }
