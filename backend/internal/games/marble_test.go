package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func marbleForTest(t *testing.T, cfg string, seed uint64, players ...string) (*marbleGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newMarble(cfgRaw, fc.ctx(players, seed))
	if err != nil {
		t.Fatalf("newMarble: %v", err)
	}
	return g.(*marbleGame), fc
}

func marbleCfg() string {
	return `{"sim_duration_ms":600000,"playback_x":100,"history_matches":2,"side_count":3,"window_step_ms":1000,"window_len_ms":800,"main_quote_step_ms":500}`
}

func TestMarbleTapeShapeAndDeterminism(t *testing.T) {
	a, _ := marbleForTest(t, marbleCfg(), 3, "p1")
	b, _ := marbleForTest(t, marbleCfg(), 3, "p1")
	ta, _ := a.Tape()
	tb, _ := b.Tape()
	if string(ta) != string(tb) {
		t.Fatal("same seed must produce identical tape")
	}
	var tape marbleTape
	_ = json.Unmarshal(ta, &tape)
	for _, r := range tape.Rates {
		if r < 0.3 || r > 1.2 {
			t.Fatalf("rate %f out of range", r)
		}
	}
	if len(tape.History) != 4 { // 2 matches per team
		t.Fatalf("history %d, want 4", len(tape.History))
	}
	for i := 1; i < len(tape.Goals); i++ {
		if tape.Goals[i].TSimMS < tape.Goals[i-1].TSimMS {
			t.Fatal("goal tape not sorted by simulation time")
		}
	}
	if tape.Goals[len(tape.Goals)-1].TSimMS >= tape.Config.SimDurationMS {
		t.Fatal("goal beyond full time")
	}
}

func TestMarbleTotalsMatchGoalTape(t *testing.T) {
	g, _ := marbleForTest(t, marbleCfg(), 4, "p1")
	end := g.cfg.SimDurationMS
	aOr, aRd, bOr, bRd := g.totalsAt(end)
	total := aOr + aRd + bOr + bRd
	if total != len(g.tape.Goals) {
		t.Fatalf("totals %d != goal count %d", total, len(g.tape.Goals))
	}
	// Monotonicity.
	a2, _, _, _ := g.totalsAt(0)
	if a2 != 0 {
		t.Fatal("totals at t=0 must be 0")
	}
}

func TestMarbleTradeWindowsAndStale(t *testing.T) {
	g, fc := marbleForTest(t, marbleCfg(), 5, "p1")
	fc.advance(100) // inside window 0 [0,800)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	sides := s["sides"].([]any)
	first := sides[0].(map[string]any)
	if first["status"] != "OPEN" {
		t.Fatalf("side 0 should be OPEN, got %v", first["status"])
	}
	ver := int(s["market_version"].(float64))
	// Buy the side contract inside its window.
	out := g.Apply(engine.ActionRequest{ActionID: "x1", PlayerID: "p1", MarketVersion: ver, Action: "BUY",
		Payload: engine.MustJSON(map[string]any{"contract_id": first["id"], "quantity": 1})}, 1)
	if !out.Accepted {
		t.Fatalf("side BUY rejected: %s", out.Reason)
	}
	var ex map[string]any
	_ = json.Unmarshal(out.Execution, &ex)
	if int(ex["edge_points"].(float64)) > 4 || int(ex["edge_points"].(float64)) < -4 {
		t.Fatalf("edge points out of bins: %v", ex["edge_points"])
	}
	// Stale version rejected.
	out = g.Apply(engine.ActionRequest{ActionID: "x2", PlayerID: "p1", MarketVersion: ver - 1, Action: "BUY",
		Payload: engine.MustJSON(map[string]any{"contract_id": "MAIN", "quantity": 1})}, 1)
	if out.Accepted || out.Reason != engine.ReasonStaleMarket {
		t.Fatalf("stale version must fail, got %+v", out)
	}
	// After the window closes, that contract is not tradable.
	fc.advance(800) // t=900 > close at 800
	out = g.Apply(engine.ActionRequest{ActionID: "x3", PlayerID: "p1", MarketVersion: 0, Action: "BUY",
		Payload: engine.MustJSON(map[string]any{"contract_id": first["id"], "quantity": 1})}, 1)
	if out.Accepted || out.Reason != engine.ReasonMarketClosed {
		t.Fatalf("closed window must fail MARKET_CLOSED, got %+v", out)
	}
}

func TestMarbleSettlementMainPayout(t *testing.T) {
	g, fc := marbleForTest(t, marbleCfg(), 6, "p1", "p2")
	fc.advance(100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	ver := int(s["market_version"].(float64))
	ask := int(s["main"].(map[string]any)["ask"].(float64))
	out := g.Apply(engine.ActionRequest{ActionID: "b1", PlayerID: "p1", MarketVersion: ver, Action: "BUY",
		Payload: engine.MustJSON(map[string]any{"contract_id": "MAIN", "quantity": 1})}, 1)
	if !out.Accepted {
		t.Fatalf("MAIN buy rejected: %s", out.Reason)
	}
	if got := g.players["p1"].books["MAIN"]; got.C != float64(-ask) || got.Q != 1 {
		t.Fatalf("book wrong: %+v", got)
	}
	fc.advance(7000) // full time (real duration 6000)
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	ft := r["final_totals"].(map[string]any)
	finalA := int(ft["A"].(map[string]any)["total"].(float64))
	finalB := int(ft["B"].(map[string]any)["total"].(float64))
	if int(r["payouts"].(map[string]any)["MAIN"].(float64)) != finalA*finalB {
		t.Fatal("MAIN payout must be finalA x finalB")
	}
	// Side payouts must match the stated conditions.
	pays := r["payouts"].(map[string]any)
	ge := func(id string, cond bool) {
		if cond && int(pays[id].(float64)) != 100 {
			t.Fatalf("%s should pay 100", id)
		}
		if !cond && int(pays[id].(float64)) != 0 {
			t.Fatalf("%s should pay 0", id)
		}
	}
	ge("A_GE_7", finalA >= 7)
	ge("A_GE_9", finalA >= 9)
	ge("B_GE_6", finalB >= 6)
	// Player profit from the held MAIN position.
	var p1 map[string]any
	for _, row := range r["players"].([]any) {
		m := row.(map[string]any)
		if m["player"] == "p1" {
			p1 = m
		}
	}
	want := -float64(ask) + float64(finalA*finalB)
	if p1["profit"].(float64) != want {
		t.Fatalf("profit %v, want %v", p1["profit"], want)
	}
}

func TestMarbleRankScoreFormula(t *testing.T) {
	g, _ := marbleForTest(t, marbleCfg(), 7, "p1", "p2", "p3")
	// Direct rank-score check via controlled books/edge.
	g.players["p1"].edgeSum = 10
	g.players["p2"].edgeSum = 5
	g.players["p3"].edgeSum = 5
	g.players["p1"].books["MAIN"] = &marbleBook{C: -10, Q: 1}
	g.players["p2"].books["MAIN"] = &marbleBook{C: -20, Q: 1}
	g.players["p3"].books["MAIN"] = &marbleBook{C: -20, Q: 1}
	// Same MAIN payout for all: profit p1 = P-10, p2 = p3 = P-20.
	// Rank score: p1 gets 2+2 for edge (both others fewer) and 1+1 for
	// profit (both lower) = 6. p2/p3: no opponents with fewer edge or lower
	// profit among their peers with equal edge/profit -> 0.
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	var p1 map[string]any
	for _, row := range r["players"].([]any) {
		m := row.(map[string]any)
		if m["player"] == "p1" {
			p1 = m
		}
	}
	if p1 == nil {
		t.Fatal("p1 row missing")
	}
	if int(p1["rank_score"].(float64)) != 6 {
		t.Fatalf("p1 rank score %v, want 6", p1["rank_score"])
	}
	if int(p1["rank"].(float64)) != 1 {
		t.Fatalf("p1 rank %v, want 1", p1["rank"])
	}
}
