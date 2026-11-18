package games

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"quantgame/backend/internal/engine"
)

// fakeClock lets tests drive simulated time deterministically.
type fakeClock struct{ ms int64 }

func (f *fakeClock) ctx(players []string, seed uint64) *engine.Ctx {
	return &engine.Ctx{
		Players: players,
		Seed:    seed,
		Start:   time.Unix(0, 0),
		NowFn:   func() time.Time { return time.Unix(0, f.ms*int64(time.Millisecond)) },
	}
}

func (f *fakeClock) advance(ms int64) { f.ms += ms }

// ---------------------------------------------------------------------------
// Fruit Market
// ---------------------------------------------------------------------------

func fruitForTest(t *testing.T, seed uint64, cfg string, players ...string) (*fruitGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newFruit(cfgRaw, fc.ctx(players, seed))
	if err != nil {
		t.Fatalf("newFruit: %v", err)
	}
	return g.(*fruitGame), fc
}

func fruitAction(g *fruitGame, p, action string, version int, payload string) engine.Output {
	var pl json.RawMessage
	if payload != "" {
		pl = json.RawMessage(payload)
	}
	return g.Apply(engine.ActionRequest{ActionID: fmt.Sprintf("%s-%s-%d", p, action, g.version), PlayerID: p, MarketVersion: version, Action: action, Payload: pl}, 1)
}

func TestFruitInitialSnapshotHidesFairValue(t *testing.T) {
	g, _ := fruitForTest(t, 1, "", "p1")
	snap, err := g.Snapshot("p1")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(snap, &s); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"fair", "value", "V"} {
		if _, ok := s[forbidden]; ok {
			t.Fatalf("snapshot leaked fair value field %q", forbidden)
		}
	}
	bags := s["bags"].([]any)
	if len(bags) != 2 {
		t.Fatal("want 2 bags")
	}
	for _, b := range bags {
		bag := b.(map[string]any)
		for _, k := range []string{"apples", "oranges"} {
			c := int(bag[k].(float64))
			if c < 3 || c > 8 {
				t.Fatalf("initial %s = %d, want in [3,8]", k, c)
			}
		}
	}
}

func TestFruitBuySellRewardsSettleImmediately(t *testing.T) {
	g, fc := fruitForTest(t, 2, "", "p1")
	g.mult = 1
	g.bags[0] = [2]int{4, 5}
	g.bags[1] = [2]int{3, 6}
	g.tape.QuoteOffsets[0] = -0.03 // force ask below V so BUY is profitable
	g.qIdx = 0                     // tests consumed offsets at construction
	g.applyQuotes()                // V = (4+3)*(5+6) = 77
	v, _ := g.value()
	if v != 77 {
		t.Fatalf("V = %d, want 77", v)
	}
	if g.ask <= g.bid {
		t.Fatalf("crossed quotes %d/%d", g.bid, g.ask)
	}

	out := fruitAction(g, "p1", "BUY", g.version, "")
	if !out.Accepted {
		t.Fatalf("BUY rejected: %s", out.Reason)
	}
	askAtBuy := g.ask
	if out.RoundReward != float64(v-askAtBuy) || out.RoundReward <= 0 {
		t.Fatalf("BUY reward %f, want V-ask = %d (positive)", out.RoundReward, v-askAtBuy)
	}
	// SELL requires a fresh market version and a fresh trade interval.
	fc.advance(500)
	vOld := g.version
	g.tape.QuoteOffsets[1] = 0.03 // force bid above V so SELL is profitable
	g.qIdx = 1
	g.applyQuotes()
	out = fruitAction(g, "p1", "SELL", vOld, "") // stale on purpose
	if out.Accepted || out.Reason != engine.ReasonStaleMarket {
		t.Fatalf("stale SELL should fail with STALE_MARKET, got %+v", out)
	}
	out = fruitAction(g, "p1", "SELL", g.version, "")
	if !out.Accepted {
		t.Fatalf("SELL rejected: %s", out.Reason)
	}
	if out.RoundReward != float64(g.bid-v) || out.RoundReward <= 0 {
		t.Fatalf("SELL reward %f, want bid-V = %d (positive)", out.RoundReward, g.bid-v)
	}
	p := g.players["p1"]
	if p.profit != float64(v-askAtBuy)+float64(g.bid-v) {
		t.Fatalf("cumulative profit mismatch: %f", p.profit)
	}
}

func TestFruitFirstClickAccuracy(t *testing.T) {
	g, fc := fruitForTest(t, 3, "", "p1")
	g.mult, g.bags[0], g.bags[1] = 1, [2]int{4, 5}, [2]int{3, 6}

	// Market version 1: first click (counts), second click same version (ignored).
	g.tape.QuoteOffsets[0] = -0.03
	g.qIdx = 0
	g.applyQuotes()
	v1 := g.version
	out := fruitAction(g, "p1", "BUY", v1, "")
	if !out.Accepted || out.RoundReward <= 0 {
		t.Fatalf("expected profitable first click, got %+v", out)
	}
	fc.advance(500)
	out = fruitAction(g, "p1", "BUY", g.version, "")
	if !out.Accepted {
		t.Fatalf("second trade rejected: %s", out.Reason)
	}
	if g.players["p1"].fcTried != 1 {
		t.Fatalf("fcTried=%d, want 1 (same market version)", g.players["p1"].fcTried)
	}

	// New market version -> new first-click opportunity.
	fc.advance(500)
	g.applyQuotes()
	out = fruitAction(g, "p1", "SELL", g.version, "")
	if !out.Accepted {
		t.Fatalf("third trade rejected: %s", out.Reason)
	}
	p := g.players["p1"]
	if p.fcTried != 2 {
		t.Fatalf("fcTried=%d, want 2", p.fcTried)
	}
	wantAcc := float64(p.fcCorrect) / 2
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	row := r["players"].([]any)[0].(map[string]any)
	if row["accuracy"].(float64) != wantAcc {
		t.Fatalf("accuracy %v, want %f", row["accuracy"], wantAcc)
	}
	if row["final_score"].(float64) != row["raw_profit"].(float64)*wantAcc {
		t.Fatalf("final score != profit * accuracy: %+v", row)
	}
}

func TestFruitRejectionsDoNotChangeState(t *testing.T) {
	g, fc := fruitForTest(t, 4, `{"min_trade_interval_ms":10000}`, "p1")
	out := fruitAction(g, "p1", "BUY", g.version, "")
	if !out.Accepted {
		t.Fatalf("first BUY rejected: %s", out.Reason)
	}
	afterGood := g.players["p1"].profit
	// Rate limit.
	out = fruitAction(g, "p1", "BUY", g.version, "")
	if out.Accepted || out.Reason != engine.ReasonRateLimit {
		t.Fatalf("want RATE_LIMIT, got %+v", out)
	}
	// Wrong quantity.
	fc.advance(11000)
	g.applyQuotes()
	out = fruitAction(g, "p1", "BUY", g.version, `{"quantity":5}`)
	if out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("want INVALID_VALUE, got %+v", out)
	}
	if g.players["p1"].profit != afterGood {
		t.Fatalf("rejected actions changed profit: %f -> %f", afterGood, g.players["p1"].profit)
	}
	if g.players["p1"].trades != 1 {
		t.Fatalf("rejected actions counted as trades: %d", g.players["p1"].trades)
	}
}

func valueOf(t *testing.T, g *fruitGame) int {
	t.Helper()
	v, _ := g.value()
	return v
}

func TestFruitEventsDoubleHalfNullify(t *testing.T) {
	g, _ := fruitForTest(t, 5, "", "p1")
	g.mult, g.bags[0], g.bags[1] = 1, [2]int{4, 5}, [2]int{3, 6}
	g.applyQuotes()
	base, _ := g.value()

	// DOUBLE
	g.mult = 2
	g.applyQuotes()
	if v := valueOf(t, g); v != 2*base {
		t.Fatalf("double: V=%d want %d", v, 2*base)
	}
	// HALF (77/2 = 38.5 -> rounds to 39)
	g.mult = 0.5
	g.applyQuotes()
	if v := valueOf(t, g); v != 39 {
		t.Fatalf("half: V=%d want 39", v)
	}
	// NULLIFY bag 0: V = (0 + a2) * (0 + o2) = 3*6 = 18 (times mult 0.5 -> 9)
	g.nullify[0] = 1 << 40
	if v := valueOf(t, g); v != 9 {
		t.Fatalf("nullify: V=%d want 9", v)
	}
}

func TestFruitResetOnCapFromTape(t *testing.T) {
	g, fc := fruitForTest(t, 6, "", "p1")
	g.bags[0] = [2]int{7, 7}
	g.tape.Updates = append([]fruitUpdate{}, fruitUpdate{T: 1000, Bag: 0, Fruit: 0, Add: 2, Reset: true, NewApples: 4, NewOranges: 5})
	g.tape.Events = nil
	fc.advance(1500)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	bag0 := s["bags"].([]any)[0].(map[string]any)
	if int(bag0["apples"].(float64)) != 4 || int(bag0["oranges"].(float64)) != 5 {
		t.Fatalf("reset not applied: %v", bag0)
	}
}

func TestFruitDeadlineAndResults(t *testing.T) {
	g, fc := fruitForTest(t, 7, `{"duration_ms":2000}`, "p1")
	fc.advance(3000)
	if !g.Finished() {
		t.Fatal("game should be finished after duration")
	}
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	if r["finished"] != true {
		t.Fatal("results must be marked finished")
	}
	if _, ok := r["players"].([]any)[0].(map[string]any)["global_points"]; !ok {
		t.Fatal("results missing global_points")
	}
}

func TestFruitTapeDeterminism(t *testing.T) {
	a, _ := fruitForTest(t, 99, "", "p1")
	b, _ := fruitForTest(t, 99, "", "p1")
	ta, _ := a.Tape()
	tb, _ := b.Tape()
	if string(ta) != string(tb) {
		t.Fatal("same seed must produce identical tape")
	}
	var tape fruitTape
	if err := json.Unmarshal(ta, &tape); err != nil {
		t.Fatalf("tape not valid JSON: %v", err)
	}
	if tape.PRNG != "splitmix64-v1" {
		t.Fatal("tape must record PRNG version")
	}
	if len(tape.Updates) == 0 {
		t.Fatal("tape must contain updates")
	}
}

// ---------------------------------------------------------------------------
// Math Challenge
// ---------------------------------------------------------------------------

func mathForTest(t *testing.T, seed uint64, cfg string) (*mathGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newMath(cfgRaw, fc.ctx([]string{"p1"}, seed))
	if err != nil {
		t.Fatalf("newMath: %v", err)
	}
	return g.(*mathGame), fc
}

func mathAnswer(g *mathGame, qid, ans string) engine.Output {
	return g.Apply(engine.ActionRequest{ActionID: qid + ans, PlayerID: "p1", Action: "ANSWER", Payload: engine.MustJSON(map[string]string{"question_id": qid, "answer": ans})}, 1)
}

func TestMathExactRationalEquivalence(t *testing.T) {
	for _, ans := range []string{"3/4", "0.75", "6/8", "75/100"} {
		g, _ := mathForTest(t, 1, "")
		g.tape.Questions = []mathQuestion{{ID: "q0", Mode: "fraction", Expression: "1/2 + 1/4", AnswerNum: "3/4"}}
		out := mathAnswer(g, "q0", ans)
		if !out.Accepted {
			t.Fatalf("%q rejected: %s", ans, out.Reason)
		}
		var ex map[string]any
		_ = json.Unmarshal(out.Execution, &ex)
		if ex["correct"] != true {
			t.Fatalf("%q should equal 3/4", ans)
		}
	}
	g, _ := mathForTest(t, 1, "")
	g.tape.Questions = []mathQuestion{{ID: "q0", Mode: "fraction", Expression: "1/2 + 1/4", AnswerNum: "3/4"}}
	out := mathAnswer(g, "q0", "0.76")
	var ex map[string]any
	_ = json.Unmarshal(out.Execution, &ex)
	if ex["correct"] != false {
		t.Fatal("0.76 must be wrong")
	}
}

func TestMathScoringAndAdvance(t *testing.T) {
	g, _ := mathForTest(t, 2, "")
	g.tape.Questions = []mathQuestion{
		{ID: "q0", Mode: "integer", Expression: "2 + 2", AnswerNum: "4"},
		{ID: "q1", Mode: "integer", Expression: "3 x 3", AnswerNum: "9"},
	}
	if out := mathAnswer(g, "q0", "4"); !out.Accepted || out.RoundReward != 1 {
		t.Fatalf("correct answer: %+v", out)
	}
	if g.players["p1"].score != 1 || g.players["p1"].index != 1 {
		t.Fatalf("state after correct: score=%d index=%d", g.players["p1"].score, g.players["p1"].index)
	}
	// Re-answering the completed item must fail without side effects.
	out := mathAnswer(g, "q0", "4")
	if out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("double answer must be rejected, got %+v", out)
	}
	if g.players["p1"].score != 1 {
		t.Fatal("rejected answer must not change score")
	}
	out = mathAnswer(g, "q1", "8")
	if !out.Accepted || out.RoundReward != -1 {
		t.Fatalf("wrong answer: %+v", out)
	}
	if g.players["p1"].score != 0 || g.players["p1"].correct != 1 || g.players["p1"].incorrect != 1 {
		t.Fatalf("tally wrong: %+v", g)
	}
	if !out.GameOver {
		t.Fatal("last item answered should end game")
	}
}

func TestMathMalformedRejectedWithoutAdvancing(t *testing.T) {
	g, _ := mathForTest(t, 3, "")
	g.tape.Questions = []mathQuestion{{ID: "q0", Mode: "integer", Expression: "1 + 1", AnswerNum: "2"}}
	for _, bad := range []string{"abc", "", "1.2.3", "1//2", "NaN"} {
		out := mathAnswer(g, "q0", bad)
		if out.Accepted {
			t.Fatalf("%q should be rejected", bad)
		}
		if out.Reason != engine.ReasonInvalidValue {
			t.Fatalf("%q wrong reason: %s", bad, out.Reason)
		}
		if g.players["p1"].index != 0 || g.players["p1"].score != 0 {
			t.Fatalf("malformed answer advanced state on %q", bad)
		}
	}
}

func TestMathSpeedPointsThresholds(t *testing.T) {
	cases := []struct {
		mode string
		spp  float64
		want int
	}{
		{"integer", 3.0, 3}, {"integer", 3.5, 2}, {"integer", 4.5, 1}, {"integer", 4.6, 0},
		{"decimal", 6.0, 3}, {"decimal", 6.75, 2}, {"decimal", 7.5, 1}, {"decimal", 7.6, 0},
		{"fraction", 6.0, 3}, {"mixed", 7.5, 1},
	}
	for _, c := range cases {
		if got := speedPoints(c.mode, c.spp); got != c.want {
			t.Fatalf("speedPoints(%s, %f) = %d, want %d", c.mode, c.spp, got, c.want)
		}
	}
}

func TestMathExpiryUnansweredZero(t *testing.T) {
	g, fc := mathForTest(t, 4, `{"duration_ms":1000,"count":10}`)
	_ = mathAnswer(g, "q0", "999") // deliberately wrong: -1
	fc.advance(2000)
	if !g.Finished() {
		t.Fatal("should finish at expiry")
	}
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	if int(r["score"].(float64)) != -1 {
		t.Fatalf("score %v, want -1 (unanswered items contribute zero)", r["score"])
	}
	if r["seconds_per_point"] != nil {
		t.Fatal("nonpositive net score must yield no seconds-per-point")
	}
	if int(r["speed_points"].(float64)) != 0 {
		t.Fatal("speed points must be 0 for nonpositive net score")
	}
}

func TestMathTapeDeterminism(t *testing.T) {
	a, _ := mathForTest(t, 11, "")
	b, _ := mathForTest(t, 11, "")
	ta, _ := a.Tape()
	tb, _ := b.Tape()
	if string(ta) != string(tb) {
		t.Fatal("same seed must produce identical tape")
	}
}
