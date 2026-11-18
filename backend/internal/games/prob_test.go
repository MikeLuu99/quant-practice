package games

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"quantgame/backend/internal/engine"
)

func probForTest(t *testing.T, cfg string, seed uint64, players ...string) (*probGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newProb(cfgRaw, fc.ctx(players, seed))
	if err != nil {
		t.Fatalf("newProb: %v", err)
	}
	return g.(*probGame), fc
}

func probSubmit(g *probGame, p string, bets map[string]float64) engine.Output {
	list := make([]probBet, 0, len(bets))
	for id, s := range bets {
		list = append(list, probBet{EventID: id, Stake: s})
	}
	return g.Apply(engine.ActionRequest{ActionID: p + "sb", PlayerID: p, Action: "SUBMIT_BETS",
		Payload: engine.MustJSON(map[string]any{"bets": list})}, 1)
}

// The worked example: two dice, P(sum>=8) = 15/36 = 5/12; offered odds 3.0
// give EV = 1/4 per unit and Kelly 1/8; bankroll 80 with stake 10.
func probWorkedRound() probRound {
	counts := []int{1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1}
	pr := probPrimitive{Type: "dice2"}
	for i, c := range counts {
		pr.Outcomes = append(pr.Outcomes, probOutcome{Desc: fmt.Sprintf("sum %d", i+2), P: float64(c) / 36})
	}
	p := 15.0 / 36
	D := 3.0
	b := D - 1
	ev := p*b - (1 - p)
	return probRound{
		Primitive:  pr,
		Predicates: []probPredicate{{ID: "e0", Text: "sum >= 8", Kind: "SUM_GE", Arg: 8, P: p, FairOdds: 1 / p, OfferedOdds: D, EV: ev, Kelly: ev / b}},
		SampleIdx:  6, // sum 8 -> win
		SampleDesc: "sum 8",
	}
}

func TestProbWorkedExampleOddsAndKelly(t *testing.T) {
	g, fc := probForTest(t, `{"rounds":1,"bankroll":80,"betting_timer_ms":1000,"result_timer_ms":100}`, 1, "p1")
	g.tape.Rounds = []probRound{probWorkedRound()}
	rd := g.tape.Rounds[0]
	ev := rd.Predicates[0]
	if ev.P != 5.0/12 {
		t.Fatalf("p = %v, want 5/12", ev.P)
	}
	if ev.FairOdds != engine.Round2(2.4) {
		t.Fatalf("fair odds %v, want 2.4", ev.FairOdds)
	}
	if math.Abs(ev.EV-0.25) > 1e-9 {
		t.Fatalf("EV per unit %v, want 0.25", ev.EV)
	}
	if math.Abs(ev.Kelly-0.125) > 1e-9 {
		t.Fatalf("Kelly %v, want 1/8", ev.Kelly)
	}
	out := probSubmit(g, "p1", map[string]float64{"e0": 10})
	if !out.Accepted {
		t.Fatalf("submit rejected: %s", out.Reason)
	}
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["balance"].(float64) != 100 { // win adds stake x (D-1) = 20
		t.Fatalf("balance after win %v, want 100", s["balance"])
	}
	res := s["my_results"].([]any)[0].(map[string]any)
	if res["outcome"] != "WIN" || res["profit"].(float64) != 20 {
		t.Fatalf("settlement wrong: %+v", res)
	}
}

func TestProbLossAndAtomicSubmission(t *testing.T) {
	g, fc := probForTest(t, `{"rounds":1,"bankroll":80,"betting_timer_ms":1000,"result_timer_ms":100}`, 1, "p1", "p2")
	round := probWorkedRound()
	round.SampleIdx = 1 // sum 3 -> loss
	round.SampleDesc = "sum 3"
	g.tape.Rounds = []probRound{round}
	// Atomic: total stakes above bankroll reject the WHOLE submission.
	out := probSubmit(g, "p1", map[string]float64{"e0": 90})
	if out.Accepted || out.Reason != engine.ReasonInsufficientStakes {
		t.Fatalf("want INSUFFICIENT_STAKES, got %+v", out)
	}
	if g.players["p1"].acted {
		t.Fatal("rejected submission must not lock the player")
	}
	// Unknown event id rejects the whole submission.
	out = probSubmit(g, "p1", map[string]float64{"eX": 10})
	if out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("unknown event must fail INVALID_VALUE, got %+v", out)
	}
	out = probSubmit(g, "p1", map[string]float64{"e0": 10})
	if !out.Accepted {
		t.Fatalf("valid submit rejected: %s", out.Reason)
	}
	if out := probSubmit(g, "p1", map[string]float64{"e0": 10}); out.Accepted || out.Reason != engine.ReasonAlreadyActed {
		t.Fatalf("resubmission must fail ALREADY_ACTED, got %+v", out)
	}
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["balance"].(float64) != 70 { // loss subtracts the stake
		t.Fatalf("balance after loss %v, want 70", s["balance"])
	}
}

func TestProbScaledKellyTargets(t *testing.T) {
	g, fc := probForTest(t, `{"rounds":1,"bankroll":100,"betting_timer_ms":1000,"result_timer_ms":100}`, 1, "p1")
	// Two positive-edge events, each Kelly 0.75 -> scaled to 0.5 each.
	pr := probPrimitive{Type: "die1"}
	for i := 1; i <= 6; i++ {
		pr.Outcomes = append(pr.Outcomes, probOutcome{Desc: fmt.Sprintf("face %d", i), P: 1.0 / 6})
	}
	p := 0.5
	b := 1.0 // odds 2.0
	ev := p*b - (1 - p)
	kelly := ev / b // 0.25? no: ev = 0 -> careful
	_ = kelly
	// Use odds 3.0 on p=0.5: ev = 0.5*2 - 0.5 = 0.5, kelly = 0.25... need 0.75.
	// Choose odds 5.0: b=4, ev = 0.5*4 - 0.5 = 1.5, kelly = 1.5/4 = 0.375.
	// Two such events sum to 0.75 (<1) - instead force targets via direct tape edit below.
	round := probRound{
		Primitive: pr,
		Predicates: []probPredicate{
			{ID: "a", Text: "odd", Kind: "FACE_ODD", P: 0.5, FairOdds: 2, OfferedOdds: 5, EV: 1.5, Kelly: 0.75},
			{ID: "b", Text: "face >= 4", Kind: "FACE_GE", Arg: 4, P: 0.5, FairOdds: 2, OfferedOdds: 5, EV: 1.5, Kelly: 0.75},
		},
		SampleIdx: 1, SampleDesc: "face 1", // odd wins, >=4 loses
	}
	g.tape.Rounds = []probRound{round}
	out := probSubmit(g, "p1", map[string]float64{"a": 50, "b": 50})
	if !out.Accepted {
		t.Fatalf("submit rejected: %s", out.Reason)
	}
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	// Scaled targets 0.5 each; f = 50/100 = 0.5 -> e = 1 for both.
	for _, r := range s["my_results"].([]any) {
		res := r.(map[string]any)
		if res["target_fraction"].(float64) != 0.5 {
			t.Fatalf("target not scaled: %+v", res)
		}
		if res["sizing_score"].(float64) != 1 {
			t.Fatalf("sizing score %v, want 1", res["sizing_score"])
		}
		if res["decision_correct"] != true {
			t.Fatalf("decision should be correct: %+v", res)
		}
	}
	// face 1: 'a' wins (+50x4=200), 'b' loses (-50) -> balance 250.
	if s["balance"].(float64) != 250 {
		t.Fatalf("balance %v, want 250", s["balance"])
	}
}

func TestProbCorrelatedPredicatesShareSample(t *testing.T) {
	g, fc := probForTest(t, `{"rounds":1,"bankroll":100,"betting_timer_ms":1000,"result_timer_ms":100}`, 1, "p1")
	pr := probPrimitive{Type: "dice2"}
	for i, c := range []int{1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1} {
		pr.Outcomes = append(pr.Outcomes, probOutcome{Desc: fmt.Sprintf("sum %d", i+2), P: float64(c) / 36})
	}
	round := probRound{
		Primitive: pr,
		Predicates: []probPredicate{
			{ID: "ge", Text: "sum >= 8", Kind: "SUM_GE", Arg: 8, P: 15.0 / 36, FairOdds: 2.4, OfferedOdds: 3, EV: 0.25, Kelly: 0.125},
			{ID: "even", Text: "sum even", Kind: "EVEN", P: 0.5, FairOdds: 2, OfferedOdds: 1.5, EV: -0.25, Kelly: 0},
		},
		SampleIdx: 6, SampleDesc: "sum 8", // ge wins, even wins too (8 even)
	}
	g.tape.Rounds = []probRound{round}
	out := probSubmit(g, "p1", map[string]float64{"ge": 10, "even": 10})
	if !out.Accepted {
		t.Fatalf("submit rejected: %s", out.Reason)
	}
	fc.advance(1100)
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["sample"].(string) != "sum 8" {
		t.Fatalf("sample %v", s["sample"])
	}
	results := s["my_results"].([]any)
	r0 := results[0].(map[string]any)
	r1 := results[1].(map[string]any)
	if r0["outcome"] != "WIN" {
		t.Fatalf("correlated ge should win: %+v", r0)
	}
	if r1["outcome"] != "WIN" {
		t.Fatalf("correlated even should win from the same sample: %+v", r1)
	}
	// Balance: +20 (ge) +5 (even) -> 125.
	if s["balance"].(float64) != 125 {
		t.Fatalf("balance %v, want 125", s["balance"])
	}
	// even was negative edge: staking it is an incorrect decision.
	if r1["decision_correct"] != false {
		t.Fatalf("staking a negative-edge event must be incorrect: %+v", r1)
	}
}

func TestProbSkillZeroTradesAndLengthBonus(t *testing.T) {
	g, fc := probForTest(t, `{"rounds":1,"bankroll":100,"betting_timer_ms":1000,"result_timer_ms":100}`, 1, "p1")
	g.tape.Rounds = []probRound{probWorkedRound()}
	fc.advance(1200) // implicit pass, settle, finish
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	p := r["players"].([]any)[0].(map[string]any)
	// With a positive-edge opportunity and no bet, the decision is incorrect;
	// with no opportunities besides that one, E = 0 and skill = 0.
	if p["skill"].(float64) != 0 {
		t.Fatalf("skill %v, want 0", p["skill"])
	}
	if p["final_balance"].(float64) != 100 {
		t.Fatalf("pass must not change balance, got %v", p["final_balance"])
	}
	// Length bonus: 1 round -> 0; craft 8 rounds for +1.
	g.cfg.Rounds = 8
	if g.lengthBonus() != 1 {
		t.Fatalf("length bonus for 8 rounds %v, want 1", g.lengthBonus())
	}
}

func TestProbTapeDeterminism(t *testing.T) {
	a, _ := probForTest(t, `{"rounds":6}`, 7, "p1", "p2")
	b, _ := probForTest(t, `{"rounds":6}`, 7, "p1", "p2")
	ta, _ := a.Tape()
	tb, _ := b.Tape()
	if string(ta) != string(tb) {
		t.Fatal("same seed must produce identical tape")
	}
	// Probabilities must sum to 1 per primitive.
	var tape probTape
	_ = json.Unmarshal(ta, &tape)
	for _, rd := range tape.Rounds {
		sum := 0.0
		for _, o := range rd.Primitive.Outcomes {
			sum += o.P
		}
		if sum < 0.999 || sum > 1.001 {
			t.Fatalf("primitive %s probabilities sum to %f", rd.Primitive.Type, sum)
		}
	}
}
