package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func etfForTest(t *testing.T, cfg string, seed uint64, players ...string) (*etfGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newETF(cfgRaw, fc.ctx(players, seed))
	if err != nil {
		t.Fatalf("newETF: %v", err)
	}
	return g.(*etfGame), fc
}

// The worked example: E1 holds one share each of A and B.
// A 49/50; B 54/55; E1 100/102 -> BUY_ETF earns 49+54-102 = +1.
func etfWorkedTape() []etfSnap {
	return []etfSnap{
		{T: 0, Stocks: []etfQuote{{Bid: 49, Ask: 50}, {Bid: 54, Ask: 55}}, ETF: etfQuote{Bid: 100, Ask: 102}, Tag: "news"},
		{T: 5000, Stocks: []etfQuote{{Bid: 24, Ask: 25}, {Bid: 25, Ask: 26}}, ETF: etfQuote{Bid: 50, Ask: 52}, Tag: "normal"},
	}
}

func etfAction(g *etfGame, p, action string, version int) engine.Output {
	return g.Apply(engine.ActionRequest{ActionID: p + action + string(rune(version)), PlayerID: p, MarketVersion: version, Action: action,
		Payload: engine.MustJSON(map[string]any{"etf_id": "E1", "quantity": 1})}, 1)
}

func TestETFWorkedExampleAtomicLegs(t *testing.T) {
	g, _ := etfForTest(t, `{"duration_ms":10000,"stock_count":2,"quote_step_ms":5000}`, 1, "p1")
	g.tape.Snaps = etfWorkedTape()
	out := etfAction(g, "p1", "BUY_ETF", 0)
	if !out.Accepted {
		t.Fatalf("BUY_ETF rejected: %s", out.Reason)
	}
	if out.RoundReward != 1 {
		t.Fatalf("profit %v, want +1", out.RoundReward)
	}
	var ex struct {
		Side   string           `json:"side"`
		Profit float64          `json:"profit"`
		Legs   []map[string]any `json:"legs"`
		Tag    string           `json:"tag"`
	}
	_ = json.Unmarshal(out.Execution, &ex)
	if len(ex.Legs) != 3 {
		t.Fatalf("want 3 legs (ETF + 2 stocks), got %d", len(ex.Legs))
	}
	if ex.Tag != "news" {
		t.Fatalf("tag %q, want news", ex.Tag)
	}

	// SELL_ETF at the same (pre-reset) quotes would receive 100 and pay
	// 50+55=105 -> -5. Verify via a fresh game with the same snapshot.
	g2, _ := etfForTest(t, `{"duration_ms":10000,"stock_count":2,"quote_step_ms":5000}`, 1, "p1")
	g2.tape.Snaps = etfWorkedTape()
	out2 := etfAction(g2, "p1", "SELL_ETF", 0)
	if !out2.Accepted || out2.RoundReward != -5 {
		t.Fatalf("SELL_ETF %+v, want accepted with -5", out2)
	}
}

func TestETFQuoteResetAndStaleAfterFill(t *testing.T) {
	g, _ := etfForTest(t, `{"duration_ms":10000,"stock_count":2,"quote_step_ms":5000}`, 1, "p1", "p2")
	g.tape.Snaps = etfWorkedTape()
	out := etfAction(g, "p1", "BUY_ETF", 0)
	if !out.Accepted {
		t.Fatal(out.Reason)
	}
	if g.snap != 1 {
		t.Fatalf("snap %d, want 1 after fill (quote reset)", g.snap)
	}
	// The acting player's old version is now stale too.
	out = etfAction(g, "p1", "BUY_ETF", 0)
	if out.Accepted || out.Reason != engine.ReasonStaleMarket {
		t.Fatalf("stale attempt must fail STALE_MARKET, got %+v", out)
	}
	// A concurrent player clicking at the old version loses the race.
	out = etfAction(g, "p2", "BUY_ETF", 0)
	if out.Accepted || out.Reason != engine.ReasonStaleMarket {
		t.Fatalf("racing player must fail STALE_MARKET, got %+v", out)
	}
	// And can trade at the new version.
	out = etfAction(g, "p2", "BUY_ETF", 1)
	if !out.Accepted {
		t.Fatalf("fresh version rejected: %s", out.Reason)
	}
}

func TestETFProfitBucketsAndResults(t *testing.T) {
	g, _ := etfForTest(t, `{"duration_ms":10000,"stock_count":2,"quote_step_ms":5000}`, 1, "p1", "p2")
	g.tape.Snaps = etfWorkedTape()
	// p1: +1 news buy, then at v1: craft a profitable normal buy.
	etfAction(g, "p1", "BUY_ETF", 0)
	g.tape.Snaps[1] = etfSnap{T: 5000, Stocks: []etfQuote{{Bid: 40, Ask: 41}, {Bid: 40, Ask: 41}}, ETF: etfQuote{Bid: 78, Ask: 79}, Tag: "normal"}
	etfAction(g, "p1", "BUY_ETF", 1) // 80 - 79 = +1 normal
	p := g.players["p1"]
	if p.profit != 2 || p.newsProfit != 1 || p.normProfit != 1 || p.profitable != 2 || p.trades != 2 {
		t.Fatalf("buckets wrong: %+v", p)
	}

	// p2: only a losing normal trade (1 trade, 0 profitable).
	g.players["p2"].profit = -3
	g.players["p2"].normProfit = -3
	g.players["p2"].trades = 1
	g.players["p2"].profitable = 0

	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	var p1, p2 map[string]any
	for _, row := range r["players"].([]any) {
		m := row.(map[string]any)
		switch m["player"] {
		case "p1":
			p1 = m
		case "p2":
			p2 = m
		}
	}
	if p1["accuracy"].(float64) != 1 || p2["accuracy"].(float64) != 0 {
		t.Fatalf("accuracy wrong: %v %v", p1["accuracy"], p2["accuracy"])
	}
	// rTotal(p1)=1, rNews(p1)=1, rNormal(p1)=1 -> ceil(1+1+2)*1 = 4.
	if int(p1["global_points"].(float64)) != 4 {
		t.Fatalf("p1 global %v, want 4", p1["global_points"])
	}
	// p2: rTotal=1, rNews=1, rNormal=1, accuracy 0 -> ceil(4*0)=0.
	if int(p2["global_points"].(float64)) != 0 {
		t.Fatalf("p2 global %v, want 0", p2["global_points"])
	}
}

func TestETFZeroTradesZeroAccuracy(t *testing.T) {
	g, fc := etfForTest(t, `{"duration_ms":1000,"stock_count":2,"quote_step_ms":500}`, 1, "p1")
	fc.advance(2000)
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	p := r["players"].([]any)[0].(map[string]any)
	if p["accuracy"].(float64) != 0 || p["global_points"].(float64) != 0 {
		t.Fatalf("zero trades must give accuracy 0 and global 0: %+v", p)
	}
	if out := etfAction(g, "p1", "BUY_ETF", 0); out.Accepted {
		t.Fatal("trading after close must be rejected")
	}
}

func TestETFTapeDeterminismAndShape(t *testing.T) {
	a, _ := etfForTest(t, `{"duration_ms":20000,"stock_count":3,"quote_step_ms":1000}`, 5, "p1")
	b, _ := etfForTest(t, `{"duration_ms":20000,"stock_count":3,"quote_step_ms":1000}`, 5, "p1")
	ta, _ := a.Tape()
	tb, _ := b.Tape()
	if string(ta) != string(tb) {
		t.Fatal("same seed must produce identical tape")
	}
	var tape etfTape
	_ = json.Unmarshal(ta, &tape)
	if len(tape.Snaps) != 21 {
		t.Fatalf("snaps %d, want 21", len(tape.Snaps))
	}
	for _, s := range tape.Snaps {
		if len(s.Stocks) != 3 {
			t.Fatalf("stock count %d", len(s.Stocks))
		}
		if s.ETF.Bid >= s.ETF.Ask {
			t.Fatalf("crossed ETF quotes %d/%d", s.ETF.Bid, s.ETF.Ask)
		}
		for _, q := range s.Stocks {
			if q.Bid >= q.Ask {
				t.Fatalf("crossed stock quotes %d/%d", q.Bid, q.Ask)
			}
		}
		switch s.Tag {
		case "news", "normal", "none":
		default:
			t.Fatalf("bad tag %q", s.Tag)
		}
	}
}
