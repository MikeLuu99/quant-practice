package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func mmForTest(t *testing.T, cfg string, seed uint64, players ...string) (*mmGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newMakeMe(cfgRaw, fc.ctx(players, seed))
	if err != nil {
		t.Fatalf("newMakeMe: %v", err)
	}
	return g.(*mmGame), fc
}

func TestMakemeWorkedExampleBookMath(t *testing.T) {
	// The published worked example: buy 2 at 40 and sell 1 at 55 gives
	// C = -25, q = 1; profit at 50 is 25; break-even 25; worst-case loss
	// over [0,100] is 25.
	g, _ := mmForTest(t, `{"rounds":1}`, 1, "p1")
	st := g.players["p1"]
	st.C, st.Q = 0, 0
	st.C -= 40 * 2
	st.Q += 2
	st.C += 55
	st.Q--
	if st.C != -25 || st.Q != 1 {
		t.Fatalf("book %v/%v, want -25/1", st.C, st.Q)
	}
	g.q.Z = 50
	g.q.DomainUB = 100
	truths := g.truths(st)
	if truths[0] != 1.0 {
		t.Fatalf("position %v, want 1", truths[0])
	}
	if truths[1] != 25.0 {
		t.Fatalf("worst-case loss %v, want 25", truths[1])
	}
	if truths[2] != 25.0 {
		t.Fatalf("break-even %v, want 25", truths[2])
	}
	if truths[3] != 25.0 {
		t.Fatalf("profit at z %v, want 25", truths[3])
	}
}

func TestMakemeBreakEvenEdgeCases(t *testing.T) {
	g, _ := mmForTest(t, `{"rounds":1}`, 1, "p1")
	g.q.DomainUB = 100
	// Flat book with no cash: every value breaks even.
	st := &mmPlayerState{}
	if g.truths(st)[2] != "ANY" {
		t.Fatalf("q=0,C=0 must be ANY, got %v", g.truths(st)[2])
	}
	// Flat book with cash: no break-even.
	st = &mmPlayerState{C: 10}
	if g.truths(st)[2] != "NONE" {
		t.Fatalf("q=0,C!=0 must be NONE, got %v", g.truths(st)[2])
	}
	// Short 5 with C=100: BE = -100/-5 = 20 in domain.
	st = &mmPlayerState{C: 100, Q: -5}
	be, _ := g.truths(st)[2].(float64)
	if be != 20 {
		t.Fatalf("break-even %v, want 20", be)
	}
	// Worst-case loss for a short book at the domain upper bound.
	// profit(U) = 100 - 5*100 = -400 -> loss 400.
	if g.truths(st)[1] != 400.0 {
		t.Fatalf("worst case %v, want 400", g.truths(st)[1])
	}
	// Break-even outside the domain: C=-10, Q=1 -> BE=10 in [0,100] ok;
	// C=-500, Q=1 -> BE=500 outside -> NONE.
	st = &mmPlayerState{C: -500, Q: 1}
	if g.truths(st)[2] != "NONE" {
		t.Fatalf("out-of-domain break-even must be NONE, got %v", g.truths(st)[2])
	}
}

func TestMakemeAnswerChecking(t *testing.T) {
	cases := []struct {
		ans   string
		truth any
		want  bool
	}{
		{"1", 1.0, true},
		{"1.0", 1.0, true},
		{"25.2", 25.0, true}, // within 1% tolerance (0.25)
		{"25.3", 25.0, false}, // 0.3 off exceeds the 0.25 tolerance
		{"none", "NONE", true},
		{"NONE", "NONE", true},
		{"any", "ANY", true},
		{"12", "ANY", false},
		{"abc", 5.0, false},
	}
	for _, c := range cases {
		if got := checkBookAnswer(c.ans, c.truth); got != c.want {
			t.Fatalf("checkBookAnswer(%q, %v) = %v, want %v", c.ans, c.truth, got, c.want)
		}
	}
	// Tolerance boundary: |v - want| <= max(0.02, 1% of |want|).
	if !checkBookAnswer("25.25", 25.0) {
		t.Fatal("within 1% tolerance should pass")
	}
	if checkBookAnswer("25.3", 25.0) {
		t.Fatal("0.3 off on 25 exceeds 1% tolerance")
	}
}

func TestMakemeSoloInformedBot(t *testing.T) {
	g, fc := mmForTest(t, `{"rounds":2,"quote_timer_ms":1000,"solo_pause_ms":100,"question_timer_ms":500}`, 2, "p1")
	// Force the hidden answer so bot behavior is predictable.
	V := 100
	g.q.MarketQ.Answer = float64(V)
	spread := g.maxSpread
	if spread > 20 {
		spread = 20
	}
	// Round 1: bid above V -> the computer sells one lot into the player's bid.
	if out := mmDoQuote(g, "p1", V+10, V+10+spread); !out.Accepted {
		t.Fatalf("solo quote rejected: %s", out.Reason)
	}
	if got := g.players["p1"]; got.C != float64(-(V + 10)) || got.Q != 1 {
		t.Fatalf("bot should buy at the player's rich bid %d: %+v", V+10, got)
	}
	fc.advance(200) // SOLO_WAIT passes -> round 2
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["round"].(float64) != 2 {
		t.Fatalf("round %v, want 2", s["round"])
	}
	// Round 2: ask below V -> the computer lifts the player's cheap ask.
	ask := V - 10
	bid := ask - spread
	if bid < 1 {
		bid = 1
	}
	if out := mmDoQuote(g, "p1", bid, ask); !out.Accepted {
		t.Fatalf("solo quote 2 rejected: %s", out.Reason)
	}
	if got := g.players["p1"]; got.C != float64(-(V+10)+ask) || got.Q != 0 {
		t.Fatalf("bot should sell back at ask %d: %+v", ask, got)
	}
}

func mmDoQuote(g *mmGame, p string, bid, ask int) engine.Output {
	return g.Apply(engine.ActionRequest{ActionID: p + "qq" + string(rune(bid)), PlayerID: p, Action: "QUOTE",
		Payload: engine.MustJSON(map[string]any{"bid": bid, "ask": ask, "bid_size": 1, "ask_size": 1})}, 1)
}

func TestMakemeSoloFlowToEnd(t *testing.T) {
	g, fc := mmForTest(t, `{"rounds":2,"quote_timer_ms":1000,"solo_pause_ms":100,"question_timer_ms":500}`, 3, "p1")
	g.q.MarketQ.Answer = 100
	// Two neutral quotes: mid 100 exactly -> estimation bonus 2.
	_ = mmDoQuote(g, "p1", 95, 105)
	fc.advance(200)
	_ = mmDoQuote(g, "p1", 98, 102)
	fc.advance(200)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "BOOK" {
		t.Fatalf("phase %v, want BOOK", s["phase"])
	}
	// Answer all four questions correctly using known book state (flat).
	keys := []string{"position", "worst_case_loss", "break_even", "profit_at_z"}
	answers := []string{"0", "0", "ANY", "0"}
	for i, k := range keys {
		bq := s["book_question"].(map[string]any)
		if bq["key"] != k {
			t.Fatalf("question %d: %v, want %v", i, bq["key"], k)
		}
		out := g.Apply(engine.ActionRequest{ActionID: "a" + k, PlayerID: "p1", Action: "ANSWER_BOOK_QUESTION",
			Payload: engine.MustJSON(map[string]any{"answer": answers[i]})}, 1)
		if !out.Accepted {
			t.Fatalf("answer rejected: %s", out.Reason)
		}
		var ex map[string]any
		_ = json.Unmarshal(out.Execution, &ex)
		if ex["correct"] != true {
			t.Fatalf("answer %q should be correct", answers[i])
		}
		if i < len(keys)-1 {
			snap, _ = g.Snapshot("p1")
			_ = json.Unmarshal(snap, &s)
		}
	}
	if !g.Finished() {
		t.Fatal("game should be over after the last book question")
	}
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	p := r["players"].([]any)[0].(map[string]any)
	// Solo: 4 correct x 1 point + bonus 2 = 6.
	if p["base_score"].(float64) != 6 {
		t.Fatalf("base score %v, want 6 (4 book + 2 est bonus)", p["base_score"])
	}
	// Answer and explanation must be revealed.
	if r["answer"].(map[string]any)["explanation"] == "" {
		t.Fatal("answer explanation missing")
	}
}

func TestMakemeHistoryRevealHalvesPoints(t *testing.T) {
	g, fc := mmForTest(t, `{"rounds":1,"quote_timer_ms":1000,"solo_pause_ms":100,"question_timer_ms":5000}`, 4, "p1")
	g.q.MarketQ.Answer = 100
	_ = mmDoQuote(g, "p1", 95, 105) // flat book
	fc.advance(200)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "BOOK" {
		t.Fatalf("phase %v, want BOOK", s["phase"])
	}
	// First question: reveal history first -> half points.
	out := g.Apply(engine.ActionRequest{ActionID: "rev", PlayerID: "p1", Action: "REVEAL_HISTORY"}, 1)
	if !out.Accepted {
		t.Fatalf("reveal rejected: %s", out.Reason)
	}
	var pc map[string]any
	_ = json.Unmarshal(out.PrivateChanges, &pc)
	if pc["points_now"].(float64) != 0.5 {
		t.Fatalf("points after reveal %v, want 0.5", pc["points_now"])
	}
	if pc["my_book"] == nil {
		t.Fatal("history reveal must expose the book")
	}
	out = g.Apply(engine.ActionRequest{ActionID: "a1", PlayerID: "p1", Action: "ANSWER_BOOK_QUESTION",
		Payload: engine.MustJSON(map[string]any{"answer": "0"})}, 1)
	if !out.Accepted || out.RoundReward != 0.5 {
		t.Fatalf("halved reward %v, want 0.5", out.RoundReward)
	}
	// Second question without reveal: full 1 point.
	out = g.Apply(engine.ActionRequest{ActionID: "a2", PlayerID: "p1", Action: "ANSWER_BOOK_QUESTION",
		Payload: engine.MustJSON(map[string]any{"answer": "0"})}, 1)
	if !out.Accepted || out.RoundReward != 1 {
		t.Fatalf("full reward %v, want 1", out.RoundReward)
	}
}

func TestMakemeMultiplayerQuoteAndCapacity(t *testing.T) {
	g, fc := mmForTest(t, `{"rounds":1,"estimate_timer_ms":300,"quote_timer_ms":1000,"respond_timer_ms":1000,"question_timer_ms":500}`, 5, "p1", "p2", "p3")
	// Estimates lock quickly.
	for _, p := range []string{"p1", "p2", "p3"} {
		out := g.Apply(engine.ActionRequest{ActionID: "est" + p, PlayerID: p, Action: "LOCK_ESTIMATE",
			Payload: engine.MustJSON(map[string]any{"value": 100})}, 1)
		if !out.Accepted {
			t.Fatalf("estimate %s rejected: %s", p, out.Reason)
		}
	}
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "QUOTE" {
		t.Fatalf("phase %v, want QUOTE", s["phase"])
	}
	maker := s["maker"].(string)
	// Maker quotes at the spread cap (read white-box) with size 2 on each side.
	bid, ask := 90, 90+g.maxSpread
	quote := g.Apply(engine.ActionRequest{ActionID: "mkq", PlayerID: maker, Action: "QUOTE",
		Payload: engine.MustJSON(map[string]any{"bid": bid, "ask": ask, "bid_size": 2, "ask_size": 2})}, 1)
	if !quote.Accepted {
		t.Fatalf("maker quote rejected: %s", quote.Reason)
	}
	// Two different responders can each fill one lot on the ask.
	buyers := []string{}
	for _, p := range g.playerOrder {
		if p == maker {
			continue
		}
		out := g.Apply(engine.ActionRequest{ActionID: "buy" + p, PlayerID: p, Action: "BUY",
			Payload: engine.MustJSON(map[string]any{"quantity": 1})}, 1)
		if out.Accepted {
			buyers = append(buyers, p)
		}
	}
	if len(buyers) != 2 {
		t.Fatalf("ask size 2 must allow exactly 2 fills, got %d", len(buyers))
	}
	// A third attempt has no capacity left.
	for _, p := range g.playerOrder {
		if p == maker {
			continue
		}
		if g.responded[p] {
			continue
		}
		out := g.Apply(engine.ActionRequest{ActionID: "buy2" + p, PlayerID: p, Action: "BUY",
			Payload: engine.MustJSON(map[string]any{"quantity": 1})}, 1)
		if out.Accepted || out.Reason != engine.ReasonNoCapacity {
			t.Fatalf("over-capacity must fail NO_CAPACITY, got %+v", out)
		}
	}
	// Maker's book mirrors two shorts at the ask.
	m := g.players[maker]
	if m.Q != -2 {
		t.Fatalf("maker position %d, want -2", m.Q)
	}
	// Quote points: timely quote +2.
	if m.quotePoints != 2 {
		t.Fatalf("quote points %v, want 2", m.quotePoints)
	}
	fc.advance(2000)
}

func TestMakemeEstimationBonusClipping(t *testing.T) {
	g, _ := mmForTest(t, `{"rounds":1}`, 6, "p1")
	g.solo = false // exercise the multiplayer estimate formula
	g.q.MarketQ.Answer = 100
	// Perfect estimate: r=1 -> bonus 3 (multiplayer).
	g.players["p1"].estimate, g.players["p1"].estLocked = 100, true
	if b := g.estBonus("p1"); b != 3 {
		t.Fatalf("bonus %v, want 3", b)
	}
	// Factor-4 error: log2(4)=2 -> 3-8 = -5 -> clipped to -8? 3-4*2 = -5 -> in range.
	g.players["p1"].estimate = 400
	if b := g.estBonus("p1"); b != -5 {
		t.Fatalf("bonus %v, want -5", b)
	}
	// Huge error clips at -8.
	g.players["p1"].estimate = 1000000 // r = 10000, log2 ~ 13.3 -> 3-53 -> -8
	if b := g.estBonus("p1"); b != -8 {
		t.Fatalf("bonus %v, want -8 (clipped)", b)
	}
	// Missing estimate -> -8.
	delete(g.players, "p1")
	g.players["px"] = &mmPlayerState{}
	if b := g.estBonus("px"); b != -8 {
		t.Fatalf("missing estimate bonus %v, want -8", b)
	}
}
