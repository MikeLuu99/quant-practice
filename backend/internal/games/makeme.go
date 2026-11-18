package games

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"quantgame/backend/internal/engine"
)

// ---------------------------------------------------------------------------
// Make Me a Market.
//
// A factual question has a frozen positive answer V (evaluator-only). In
// multiplayer, players lock a private estimate, rotate as maker quoting
// bid/ask with sizes, and others trade one lot or pass. In solo, an informed
// computer exploits quotes that cross V. Afterwards every player answers
// personalized book questions (position, worst-case loss over the public
// settlement domain, break-even, profit at a shown z); revealing the trade
// history halves the points for that question. Scores follow appendix A2.
// ---------------------------------------------------------------------------

type mmConfig struct {
	Rounds       int     `json:"rounds"`
	EstimateMS   int64   `json:"estimate_timer_ms"`
	QuoteMS      int64   `json:"quote_timer_ms"`
	RespondMS    int64   `json:"respond_timer_ms"`
	QuestionMS   int64   `json:"question_timer_ms"`
	SpreadPct    float64 `json:"spread_pct"`
	MinLot       int     `json:"min_lot"`
	MaxLot       int     `json:"max_lot"`
	RevealFactor float64 `json:"reveal_factor"`
	SoloPauseMS  int64   `json:"solo_pause_ms"`
}

type mmQuestion struct {
	MarketQ
	Z int `json:"z"` // settlement value asked about
}

type mmPlayerState struct {
	estimate    float64
	estLocked   bool
	C           float64
	Q           int
	quotePoints float64
	bookPoints  float64
	qIdx        int
	answered    [4]bool
	revealed    [4]bool
	qStartMS    int64
	finished    bool
}

type mmQuote struct {
	Bid, Ask, BidSize, AskSize int
	BidLeft, AskLeft           int
	Set                        bool
}

type mmGame struct {
	ctx          *engine.Ctx
	cfg          mmConfig
	q            mmQuestion
	maxSpread    int
	phase        string // ESTIMATE | QUOTE | RESPONSE | SOLO_WAIT | BOOK | DONE
	phaseStartMS int64
	turn         int
	quote        mmQuote
	responded    map[string]bool
	review       []map[string]any
	players      map[string]*mmPlayerState
	playerOrder  []string
	solo         bool
	soloMids     []float64
	soloBotLog   []map[string]any
	done         bool
}

func newMakeMe(cfgRaw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	cfg := mmConfig{
		Rounds: 3, EstimateMS: 30_000, QuoteMS: 20_000, RespondMS: 12_000,
		QuestionMS: 45_000, SpreadPct: 0.15, MinLot: 1, MaxLot: 10, RevealFactor: 0.5, SoloPauseMS: 1_500,
	}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.Rounds < 1 || len(ctx.Players) < 1 {
		return nil, fmt.Errorf("invalid makeme config")
	}
	rng := engine.NewRNG(ctx.Seed + 0xA6E7)
	g := &mmGame{ctx: ctx, cfg: cfg, players: map[string]*mmPlayerState{}, responded: map[string]bool{}}
	g.q = mmQuestion{MarketQ: marketBank[rng.Int(len(marketBank))]}
	g.q.Z = int(math.Round(g.q.DomainUB * (0.3 + rng.Float()*0.4)))
	g.maxSpread = int(math.Ceil(g.q.Answer * cfg.SpreadPct))
	g.solo = len(ctx.Players) == 1
	for _, p := range ctx.Players {
		g.players[p] = &mmPlayerState{}
	}
	g.playerOrder = append([]string(nil), ctx.Players...)
	sort.Strings(g.playerOrder)
	if g.solo {
		g.phase = "QUOTE"
	} else {
		g.phase = "ESTIMATE"
	}
	g.phaseStartMS = 0
	return g, nil
}

func (g *mmGame) Meta() engine.GameMeta {
	return engine.GameMeta{
		Slug: "makeme", Name: "Make Me a Market", Access: "premium", MinPlayers: 1, MaxPlayers: 300,
		DurationMS:  0, // phase-driven; computed below
		Description: "Quote a market on a hidden factual quantity, then reconstruct your own book.",
		Rules:       "A factual answer V is frozen and hidden. Multiplayer: lock a private estimate, then rotate as maker quoting within the spread cap (timely +2, missed -1); others trade one lot first-come within capacity or pass. Solo: an informed computer sells into bids above V and lifts asks below V. Then every player answers personalized book questions over the public domain [0, U] - position, worst-case loss, break-even, profit at a shown z. Revealing your history halves that question's points. Estimation bonus follows appendix A2.",
	}
}

// book questions in fixed order.
var mmBookQuestions = []string{"position", "worst_case_loss", "break_even", "profit_at_z"}

func (g *mmGame) questionPoints(p *mmPlayerState) float64 {
	base := 1.0
	if !g.solo {
		base = 2
	}
	if p.revealed[p.qIdx] {
		return base * g.cfg.RevealFactor
	}
	return base
}

// truths returns the four book answers for a player's book state.
func (g *mmGame) truths(p *mmPlayerState) []any {
	U := g.q.DomainUB
	be := any("NONE")
	if p.Q != 0 {
		b := -p.C / float64(p.Q)
		if b >= 0 && b <= U {
			be = b
		}
	} else if p.C == 0 {
		be = "ANY"
	}
	worst := math.Max(0, -math.Min(p.C, p.C+float64(p.Q)*U))
	return []any{float64(p.Q), worst, be, p.C + float64(p.Q)*float64(g.q.Z)}
}

func (g *mmGame) prompts(p *mmPlayerState) []string {
	U := int(g.q.DomainUB)
	return []string{
		"What is your current signed position in lots? (positive = long)",
		fmt.Sprintf("Over the settlement domain [0, %d], what is your maximum guaranteed loss in points? (0 if none)", U),
		"At what settlement value does your book break even? Answer with a number, NONE (no break-even in the domain), or ANY (every value breaks even).",
		fmt.Sprintf("If the contract settles at z = %d, what is your profit in points?", g.q.Z),
	}
}

func (g *mmGame) maker() string {
	if g.turn >= len(g.playerOrder)*g.cfg.Rounds {
		return ""
	}
	return g.playerOrder[g.turn%len(g.playerOrder)]
}

func (g *mmGame) Tick() { g.tick() }

func (g *mmGame) tick() {
	if g.done {
		return
	}
	now := g.ctx.ElapsedMS()
	switch g.phase {
	case "ESTIMATE":
		if now-g.phaseStartMS >= g.cfg.EstimateMS || g.allEstimatesLocked() {
			g.phase = "QUOTE"
			g.phaseStartMS = now
		}
	case "QUOTE":
		if now-g.phaseStartMS >= g.cfg.QuoteMS {
			if !g.solo && !g.quote.Set {
				g.players[g.maker()].quotePoints -= 1 // missed turn
				g.endMarket(now)
			} else if g.solo {
				g.nextSoloRound(now) // missed solo quote
			}
		}
	case "RESPONSE":
		if now-g.phaseStartMS >= g.cfg.RespondMS || g.allResponded() {
			g.endMarket(now)
		}
	case "SOLO_WAIT":
		if now-g.phaseStartMS >= g.cfg.SoloPauseMS {
			g.nextSoloRound(now)
		}
	case "BOOK":
		for _, p := range g.playerOrder {
			st := g.players[p]
			if st.finished {
				continue
			}
			if now-st.qStartMS >= g.cfg.QuestionMS {
				g.advanceBook(st, now) // timeout: zero points for that question
			}
		}
		allDone := true
		for _, p := range g.playerOrder {
			if !g.players[p].finished {
				allDone = false
			}
		}
		if allDone {
			g.phase = "DONE"
			g.done = true
		}
	}
}

func (g *mmGame) allEstimatesLocked() bool {
	for _, p := range g.playerOrder {
		if !g.players[p].estLocked {
			return false
		}
	}
	return true
}

func (g *mmGame) allResponded() bool {
	for _, p := range g.playerOrder {
		if p == g.maker() {
			continue
		}
		if !g.responded[p] {
			return false
		}
	}
	return true
}

func (g *mmGame) endMarket(now int64) {
	if g.quote.Set {
		g.review = append(g.review, map[string]any{
			"maker": g.maker(), "bid": g.quote.Bid, "ask": g.quote.Ask,
			"bid_size": g.quote.BidSize, "ask_size": g.quote.AskSize,
			"bid_left": g.quote.BidLeft, "ask_left": g.quote.AskLeft,
			"responded": g.responded,
		})
	}
	g.quote = mmQuote{}
	g.responded = map[string]bool{}
	g.turn++
	if g.turn >= len(g.playerOrder)*g.cfg.Rounds {
		g.startBook(now)
		return
	}
	g.phase = "QUOTE"
	g.phaseStartMS = now
}

func (g *mmGame) nextSoloRound(now int64) {
	if g.quote.Set {
		mid := float64(g.quote.Bid+g.quote.Ask) / 2
		g.soloMids = append(g.soloMids, mid)
	}
	g.quote = mmQuote{}
	if len(g.soloMids) >= g.cfg.Rounds {
		g.startBook(now)
		return
	}
	g.phase = "QUOTE"
	g.phaseStartMS = now
}

func (g *mmGame) soloRound() int {
	return len(g.soloMids)
}

func (g *mmGame) startBook(now int64) {
	g.phase = "BOOK"
	g.phaseStartMS = now
	for _, p := range g.playerOrder {
		g.players[p].qStartMS = now
	}
}

func (g *mmGame) advanceBook(st *mmPlayerState, now int64) {
	st.qIdx++
	if st.qIdx >= len(mmBookQuestions) {
		st.finished = true
		return
	}
	st.qStartMS = now
}

// informedBot exploits solo quotes that cross the true value V.
func (g *mmGame) informedBot(now int64) {
	st := g.players[g.playerOrder[0]]
	if !g.quote.Set {
		return
	}
	if float64(g.quote.Bid) > g.q.Answer {
		// The computer sells one lot into the player's rich bid.
		st.C -= float64(g.quote.Bid)
		st.Q++
		g.soloBotLog = append(g.soloBotLog, map[string]any{"action": "SELL_TO_PLAYER", "price": g.quote.Bid})
	} else if float64(g.quote.Ask) < g.q.Answer {
		// The computer buys one lot from the player's cheap ask.
		st.C += float64(g.quote.Ask)
		st.Q--
		g.soloBotLog = append(g.soloBotLog, map[string]any{"action": "BUY_FROM_PLAYER", "price": g.quote.Ask})
	} else {
		g.soloBotLog = append(g.soloBotLog, map[string]any{"action": "PASS"})
	}
}

func (g *mmGame) estBonus(p string) float64 {
	st := g.players[p]
	var r float64
	if g.solo {
		if len(g.soloMids) == 0 {
			return -5
		}
		mid := g.soloMids[len(g.soloMids)-1]
		r = math.Max(mid/g.q.Answer, g.q.Answer/mid)
		return engine.Clamp(2-3*math.Log2(r), -5, 2)
	}
	if !st.estLocked || st.estimate <= 0 {
		return -8
	}
	r = math.Max(st.estimate/g.q.Answer, g.q.Answer/st.estimate)
	return engine.Clamp(3-4*math.Log2(r), -8, 3)
}

func (g *mmGame) Snapshot(playerID string) (json.RawMessage, error) {
	g.tick()
	st, ok := g.players[playerID]
	if !ok {
		return nil, fmt.Errorf("unknown player %q", playerID)
	}
	remaining := int64(0)
	switch g.phase {
	case "ESTIMATE":
		remaining = g.cfg.EstimateMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	case "QUOTE":
		remaining = g.cfg.QuoteMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	case "RESPONSE":
		remaining = g.cfg.RespondMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	case "SOLO_WAIT":
		remaining = g.cfg.SoloPauseMS - (g.ctx.ElapsedMS() - g.phaseStartMS)
	case "BOOK":
		remaining = g.cfg.QuestionMS - (g.ctx.ElapsedMS() - st.qStartMS)
	}
	if remaining < 0 {
		remaining = 0
	}
	snap := map[string]any{
		"phase":             g.phase,
		"time_remaining_ms": remaining,
		"finished":          g.done,
		"question":          map[string]any{"prompt": g.q.Prompt, "units": g.q.Units, "domain": []int{0, int(g.q.DomainUB)}},
		"spread_limit":      g.maxSpread,
		"lot_range":         []int{g.cfg.MinLot, g.cfg.MaxLot},
		"my_quote_points":   st.quotePoints,
		"my_book_points":    st.bookPoints,
		"rounds":            g.cfg.Rounds,
	}
	if g.solo {
		snap["mode"] = "solo"
		snap["round"] = g.soloRound() + 1
	} else {
		snap["mode"] = "multiplayer"
		snap["turn"] = g.turn + 1
		snap["total_turns"] = len(g.playerOrder) * g.cfg.Rounds
		snap["maker"] = g.maker()
	}
	if g.phase == "ESTIMATE" {
		snap["estimate_locked"] = st.estLocked
	}
	if g.phase == "QUOTE" || g.phase == "RESPONSE" {
		// Own book is visible while trading but never during book questions.
		snap["my_book"] = map[string]any{"cash": st.C, "position": st.Q}
		if g.quote.Set {
			snap["quote"] = map[string]any{
				"bid": g.quote.Bid, "ask": g.quote.Ask,
				"bid_size": g.quote.BidSize, "ask_size": g.quote.AskSize,
			}
		}
		if g.phase == "RESPONSE" {
			snap["i_responded"] = g.responded[playerID]
		}
	}
	if g.phase == "SOLO_WAIT" && len(g.soloBotLog) > 0 {
		snap["bot_action"] = g.soloBotLog[len(g.soloBotLog)-1]
	}
	if g.phase == "BOOK" && !st.finished {
		if st.revealed[st.qIdx] {
			snap["my_book"] = map[string]any{"cash": st.C, "position": st.Q}
		}
		q := mmBookQuestions[st.qIdx]
		snap["book_question"] = map[string]any{
			"index": st.qIdx, "key": q, "prompt": g.prompts(st)[st.qIdx],
			"points": g.questionPoints(st), "history_revealed": st.revealed[st.qIdx],
			"question_index": st.qIdx + 1, "question_total": len(mmBookQuestions),
		}
	}
	return engine.MustJSON(snap), nil
}

func (g *mmGame) Apply(req engine.ActionRequest, seq int64) engine.Output {
	g.tick()
	st, ok := g.players[req.PlayerID]
	if !ok {
		return engine.Output{Accepted: false, Reason: engine.ReasonUnknownPlayer}
	}
	if g.done {
		return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
	}
	switch req.Action {
	case "LOCK_ESTIMATE":
		if g.phase != "ESTIMATE" {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		if st.estLocked {
			return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
		}
		var pl struct {
			Value float64 `json:"value"`
		}
		if err := json.Unmarshal(req.Payload, &pl); err != nil || math.IsNaN(pl.Value) || math.IsInf(pl.Value, 0) || pl.Value <= 0 {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		st.estimate, st.estLocked = pl.Value, true
		return engine.Output{Accepted: true, PrivateChanges: engine.MustJSON(map[string]any{"estimate_locked": true})}

	case "QUOTE":
		var pl struct {
			Bid     int `json:"bid"`
			Ask     int `json:"ask"`
			BidSize int `json:"bid_size"`
			AskSize int `json:"ask_size"`
		}
		if err := json.Unmarshal(req.Payload, &pl); err != nil {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		if g.phase != "QUOTE" {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		if g.solo {
			if g.quote.Set {
				return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
			}
		} else if req.PlayerID != g.maker() || g.quote.Set {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		if pl.Bid < 1 || pl.Bid > pl.Ask || pl.Ask-pl.Bid > g.maxSpread {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		if pl.BidSize < g.cfg.MinLot || pl.AskSize < g.cfg.MinLot || pl.BidSize > g.cfg.MaxLot || pl.AskSize > g.cfg.MaxLot {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		g.quote = mmQuote{Bid: pl.Bid, Ask: pl.Ask, BidSize: pl.BidSize, AskSize: pl.AskSize, BidLeft: pl.BidSize, AskLeft: pl.AskSize, Set: true}
		st.quotePoints += 2 // timely quote (multiplayer); solo ignores quote points at scoring time
		if g.solo {
			// The informed computer acts immediately, then a short pause.
			g.informedBot(g.ctx.ElapsedMS())
			g.phase = "SOLO_WAIT"
			g.phaseStartMS = g.ctx.ElapsedMS()
			return engine.Output{
				Accepted: true,
				Execution: engine.MustJSON(map[string]any{
					"bid": pl.Bid, "ask": pl.Ask,
					"bot_action": g.soloBotLog[len(g.soloBotLog)-1],
				}),
				PrivateChanges: engine.MustJSON(map[string]any{"cash": st.C, "position": st.Q}),
			}
		}
		g.phase = "RESPONSE"
		g.phaseStartMS = g.ctx.ElapsedMS()
		return engine.Output{
			Accepted:      true,
			Execution:     engine.MustJSON(map[string]any{"bid": pl.Bid, "ask": pl.Ask, "bid_size": pl.BidSize, "ask_size": pl.AskSize}),
			PublicChanges: engine.MustJSON(map[string]any{"phase": "RESPONSE"}),
		}

	case "BUY", "SELL":
		if g.phase != "RESPONSE" || req.PlayerID == g.maker() {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		if g.responded[req.PlayerID] {
			return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
		}
		if !g.quote.Set {
			return engine.Output{Accepted: false, Reason: engine.ReasonMarketClosed}
		}
		var pl struct {
			Quantity int `json:"quantity"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &pl)
		}
		if pl.Quantity != 1 {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		maker := g.players[g.maker()]
		if req.Action == "BUY" {
			if g.quote.AskLeft <= 0 {
				return engine.Output{Accepted: false, Reason: engine.ReasonNoCapacity}
			}
			g.quote.AskLeft--
			st.C -= float64(g.quote.Ask)
			st.Q++
			maker.C += float64(g.quote.Ask)
			maker.Q--
		} else {
			if g.quote.BidLeft <= 0 {
				return engine.Output{Accepted: false, Reason: engine.ReasonNoCapacity}
			}
			g.quote.BidLeft--
			st.C += float64(g.quote.Bid)
			st.Q--
			maker.C -= float64(g.quote.Bid)
			maker.Q++
		}
		g.responded[req.PlayerID] = true
		return engine.Output{
			Accepted:       true,
			Execution:      engine.MustJSON(map[string]any{"side": req.Action, "price": pickPrice(req.Action, g.quote), "quantity": 1}),
			PrivateChanges: engine.MustJSON(map[string]any{"cash": st.C, "position": st.Q}),
		}

	case "PASS":
		if g.phase != "RESPONSE" || req.PlayerID == g.maker() {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		if g.responded[req.PlayerID] {
			return engine.Output{Accepted: false, Reason: engine.ReasonAlreadyActed}
		}
		g.responded[req.PlayerID] = true
		return engine.Output{Accepted: true, PrivateChanges: engine.MustJSON(map[string]any{"passed": true})}

	case "REVEAL_HISTORY":
		if g.phase != "BOOK" || st.finished || st.revealed[st.qIdx] {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		st.revealed[st.qIdx] = true
		return engine.Output{
			Accepted: true,
			PrivateChanges: engine.MustJSON(map[string]any{
				"history_revealed": true,
				"my_book":          map[string]any{"cash": st.C, "position": st.Q},
				"points_now":       g.questionPoints(st),
			}),
		}

	case "ANSWER_BOOK_QUESTION":
		if g.phase != "BOOK" || st.finished {
			return engine.Output{Accepted: false, Reason: engine.ReasonNotYourTurn}
		}
		var pl struct {
			Answer string `json:"answer"`
		}
		if err := json.Unmarshal(req.Payload, &pl); err != nil {
			return engine.Output{Accepted: false, Reason: engine.ReasonInvalidValue}
		}
		truth := g.truths(st)[st.qIdx]
		correct := checkBookAnswer(strings.TrimSpace(pl.Answer), truth)
		pts := 0.0
		if correct {
			pts = g.questionPoints(st)
			st.bookPoints += pts
		}
		// One answer per question; advance regardless of correctness.
		g.advanceBook(st, g.ctx.ElapsedMS())
		return engine.Output{
			Accepted:       true,
			RoundReward:    pts,
			Execution:      engine.MustJSON(map[string]any{"correct": correct, "question": mmBookQuestions[st.qIdx-1]}),
			PrivateChanges: engine.MustJSON(map[string]any{"book_points": st.bookPoints, "next_index": st.qIdx}),
		}
	}
	return engine.Output{Accepted: false, Reason: engine.ReasonUnknownAction}
}

func pickPrice(side string, q mmQuote) int {
	if side == "BUY" {
		return q.Ask
	}
	return q.Bid
}

// checkBookAnswer validates a book answer against the truth.
func checkBookAnswer(ans string, truth any) bool {
	switch want := truth.(type) {
	case float64:
		v, err := parseNum(ans)
		if err != nil {
			return false
		}
		tol := math.Max(0.02, math.Abs(want)*0.01)
		return math.Abs(v-want) <= tol
	case string:
		up := strings.ToUpper(ans)
		if want == "NONE" {
			return up == "NONE"
		}
		return up == "ANY" // "ANY": every value breaks even
	}
	return false
}

func parseNum(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%g", &v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("not finite")
	}
	return v, nil
}

func (g *mmGame) Finished() bool { g.tick(); return g.done }

func (g *mmGame) Results() (json.RawMessage, error) {
	g.tick()
	type row struct {
		Player      string           `json:"player"`
		Cash        float64          `json:"final_cash"`
		Position    int              `json:"final_position"`
		QuotePoints float64          `json:"quote_points"`
		BookPoints  float64          `json:"book_points"`
		EstBonus    float64          `json:"estimation_bonus"`
		BaseScore   float64          `json:"base_score"`
		Rank        int              `json:"rank"`
		Global      int              `json:"global_points"`
		BookAnswers []map[string]any `json:"book_review"`
	}
	rows := make([]row, 0, len(g.players))
	for name, st := range g.players {
		// Book review: truth per question and whether answered correctly is
		// reconstructed from points; show truths now that the game is over.
		base := st.quotePoints + st.bookPoints + g.estBonus(name)
		if g.solo {
			base = st.bookPoints + g.estBonus(name) // solo has no quote points
		}
		truths := g.truths(st)
		answers := make([]map[string]any, 0, 4)
		for i, q := range mmBookQuestions {
			answers = append(answers, map[string]any{
				"question": q, "truth": truths[i],
			})
		}
		rows = append(rows, row{
			Player: name, Cash: st.C, Position: st.Q,
			QuotePoints: engine.Round2(st.quotePoints), BookPoints: engine.Round2(st.bookPoints),
			EstBonus: engine.Round2(g.estBonus(name)), BaseScore: engine.Round2(base),
			BookAnswers: answers,
		})
	}
	for i := range rows {
		r := 1
		for j := range rows {
			if rows[j].BaseScore > rows[i].BaseScore {
				r++
			}
		}
		rows[i].Rank = r
	}
	if !g.solo {
		for i := range rows {
			pts := 0
			for j := range rows {
				if rows[j].BaseScore < rows[i].BaseScore {
					pts++
				}
			}
			rows[i].Global = pts
		}
	}
	return engine.MustJSON(map[string]any{
		"finished": true,
		"answer":   map[string]any{"value": g.q.Answer, "units": g.q.Units, "explanation": g.q.Explanation},
		"z":        g.q.Z,
		"players":  rows,
		"review":   g.review,
		"bot_log":  g.soloBotLog,
	}), nil
}

func (g *mmGame) Tape() (json.RawMessage, error) {
	return engine.MustJSON(map[string]any{
		"prng":        "splitmix64-v1",
		"question_id": g.q.ID, "answer": g.q.Answer, "z": g.q.Z,
		"config": g.cfg, "max_spread": g.maxSpread,
	}), nil
}
