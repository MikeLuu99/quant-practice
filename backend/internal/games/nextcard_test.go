package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func ncForTest(t *testing.T, cfg string, players ...string) (*ncGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newNextCard(cfgRaw, fc.ctx(players, 9))
	if err != nil {
		t.Fatalf("newNextCard: %v", err)
	}
	return g.(*ncGame), fc
}

func ncBet(g *ncGame, p, pred string, stake float64) engine.Output {
	return g.Apply(engine.ActionRequest{ActionID: p + pred, PlayerID: p, Action: "BET",
		Payload: engine.MustJSON(map[string]any{"prediction": pred, "stake": stake})}, 1)
}

func TestNextCardWorkedExampleKellyAndBalance(t *testing.T) {
	g, fc := ncForTest(t, `{"suits":1,"starting_balance":90,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 9, Suit: 0}, Draw: ncCard{Rank: 5, Suit: 0}}}

	eval := g.evaluateRound(0)
	ev := eval["LOWER"]
	if ev.PW != 2.0/3.0 || ev.PL != 1.0/3.0 {
		t.Fatalf("LOWER probs wrong: %+v (want 2/3, 1/3)", ev)
	}
	if ev.KellyF != 1.0/3.0 || ev.Target != "LOWER" {
		t.Fatalf("Kelly fraction %v, want 1/3 targeting LOWER", ev)
	}

	out := ncBet(g, "p1", "LOWER", 30)
	if !out.Accepted {
		t.Fatalf("BET rejected: %s", out.Reason)
	}
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["balance"].(float64) != 120 {
		t.Fatalf("balance after win %v, want 120", s["balance"])
	}
	mr := s["my_result"].(map[string]any)
	if mr["outcome"] != "WIN" || mr["sizing_score"].(float64) != 1 {
		t.Fatalf("result wrong: %+v", mr)
	}
}

func TestNextCardLossAndPush(t *testing.T) {
	// Loss: same setup but the draw is high.
	g, fc := ncForTest(t, `{"suits":1,"starting_balance":90,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 9, Suit: 0}, Draw: ncCard{Rank: 12, Suit: 0}}}
	_ = ncBet(g, "p1", "LOWER", 30)
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["balance"].(float64) != 60 {
		t.Fatalf("balance after loss %v, want 60", s["balance"])
	}

	// Push: four suits allow a second nine; push returns the stake.
	g2, fc2 := ncForTest(t, `{"suits":4,"starting_balance":90,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	g2.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 9, Suit: 0}, Draw: ncCard{Rank: 9, Suit: 1}}}
	_ = ncBet(g2, "p1", "LOWER", 30)
	fc2.advance(1100)
	snap, _ = g2.Snapshot("p1")
	_ = json.Unmarshal(snap, &s)
	if s["balance"].(float64) != 90 {
		t.Fatalf("push must return the stake, got %v", s["balance"])
	}
	mr := s["my_result"].(map[string]any)
	if mr["outcome"] != "PUSH" {
		t.Fatalf("outcome %v, want PUSH", mr["outcome"])
	}
}

func TestNextCardDeckConservation(t *testing.T) {
	g, _ := ncForTest(t, `{"suits":3,"cycles":2,"mode":"alternating"}`, "p1")
	// Rounds must consume cards sequentially from their cycle's recorded deck
	// order, without overlap, and every round's cards must be distinct.
	for cycle, deck := range g.tape.Decks {
		pos := 0
		for _, rd := range g.tape.Rounds {
			if rd.Cycle != cycle {
				continue
			}
			need := 2
			if rd.Mode == "inside_outside" {
				need = 3
			}
			if pos+need > len(deck) {
				t.Fatalf("cycle %d over-consumed: pos %d + %d > %d", cycle, pos, need, len(deck))
			}
			slot := map[int]bool{}
			for _, c := range []ncCard{rd.Ref1, rd.Draw} {
				key := c.Rank*4 + c.Suit
				if slot[key] {
					t.Fatalf("duplicate card %d within a round", key)
				}
				slot[key] = true
			}
			if rd.Ref2 != nil {
				key := rd.Ref2.Rank*4 + rd.Ref2.Suit
				if slot[key] {
					t.Fatalf("duplicate card %d within a round", key)
				}
				slot[key] = true
				if rd.Ref1.val(false) == rd.Ref2.val(false) {
					t.Fatal("inside/outside round has equal boundaries")
				}
			}
			pos += need
		}
	}
}

func TestNextCardSkillScorePerfectSizing(t *testing.T) {
	g, fc := ncForTest(t, `{"suits":1,"starting_balance":90,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 9, Suit: 0}, Draw: ncCard{Rank: 5, Suit: 0}}}
	_ = ncBet(g, "p1", "LOWER", 30) // f = 30/90 = 1/3 = f* -> e = 1
	fc.advance(1100)
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	p := r["players"].([]any)[0].(map[string]any)
	if p["skill"].(float64) != 10 {
		t.Fatalf("perfect decision+sizing must give skill 10, got %v", p["skill"])
	}
	if p["decision_fraction"].(float64) != 1 || p["sizing_average"].(float64) != 1 {
		t.Fatalf("components wrong: %+v", p)
	}
}

func TestNextCardPassCorrectWithNoEdge(t *testing.T) {
	g, fc := ncForTest(t, `{"suits":1,"starting_balance":100,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	// Ref 7 in one suit: 6 lower, 6 higher -> zero edge both ways.
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 7, Suit: 0}, Draw: ncCard{Rank: 3, Suit: 0}}}
	eval := g.evaluateRound(0)
	if eval["HIGHER"].Target != "" || eval["LOWER"].Target != "" {
		t.Fatalf("zero-edge round must have no target: %+v", eval)
	}
	fc.advance(1100) // implicit pass
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	mr := s["my_result"].(map[string]any)
	if mr["outcome"] != "MISS" || mr["decision_correct"] != true {
		t.Fatalf("implicit pass with no edge should be correct: %+v", mr)
	}
	if s["balance"].(float64) != 100 {
		t.Fatalf("pass must not change balance, got %v", s["balance"])
	}
}

func TestNextCardWrongSideBet(t *testing.T) {
	g, fc := ncForTest(t, `{"suits":1,"starting_balance":90,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 9, Suit: 0}, Draw: ncCard{Rank: 5, Suit: 0}}}
	_ = ncBet(g, "p1", "HIGHER", 30) // target is LOWER
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	mr := s["my_result"].(map[string]any)
	if mr["decision_correct"] != false {
		t.Fatalf("betting the wrong side must be incorrect: %+v", mr)
	}
	if mr["sizing_score"].(float64) != 0 {
		t.Fatalf("wrong-side sizing score must be 0, got %v", mr["sizing_score"])
	}
}

func TestNextCardValidation(t *testing.T) {
	g, _ := ncForTest(t, `{"suits":1,"starting_balance":50}`, "p1", "p2")
	if out := ncBet(g, "p1", "LOWER", 60); out.Accepted || out.Reason != engine.ReasonInsufficient {
		t.Fatalf("overbalance stake must fail INSUFFICIENT_BALANCE, got %+v", out)
	}
	if out := ncBet(g, "p1", "LOWER", 0); out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("zero stake must fail INVALID_VALUE, got %+v", out)
	}
	if out := ncBet(g, "p1", "INSIDE", 10); out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("prediction not permitted in higher_lower mode must fail, got %+v", out)
	}
	out := ncBet(g, "p1", "LOWER", 10)
	if !out.Accepted {
		t.Fatalf("valid bet rejected: %s", out.Reason)
	}
	if out := ncBet(g, "p1", "HIGHER", 10); out.Accepted || out.Reason != engine.ReasonAlreadyActed {
		t.Fatalf("second bet in round must fail ALREADY_ACTED, got %+v", out)
	}
}

func TestNextCardBankruptNoGlobalPoints(t *testing.T) {
	g, fc := ncForTest(t, `{"suits":1,"starting_balance":10,"betting_timer_ms":1000,"result_ms":100}`, "p1")
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 9, Suit: 0}, Draw: ncCard{Rank: 12, Suit: 0}}}
	_ = ncBet(g, "p1", "LOWER", 10) // all-in loss
	fc.advance(1100)
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	p := r["players"].([]any)[0].(map[string]any)
	if p["final_balance"].(float64) != 0 {
		t.Fatalf("balance %v, want 0", p["final_balance"])
	}
	if p["global_points"].(float64) != 0 {
		t.Fatalf("bankrupt player must earn no global points, got %v", p["global_points"])
	}
}

func TestNextCardAceHigh(t *testing.T) {
	g, _ := ncForTest(t, `{"suits":1,"ace_high":true}`, "p1")
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 13, Suit: 0}, Draw: ncCard{Rank: 1, Suit: 0}}}
	eval := g.evaluateRound(0)
	// With ace high, only lower cards remain below K -> LOWER has edge.
	if eval["LOWER"].Target != "LOWER" {
		t.Fatalf("ace-high must make LOWER the target vs K: %+v", eval)
	}
	if g.outcomeOf(g.tape.Rounds[0], "HIGHER") != "WIN" {
		t.Fatal("ace must rank above king with ace_high")
	}
}
