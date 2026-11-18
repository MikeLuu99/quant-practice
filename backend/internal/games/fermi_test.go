package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func fermiForTest(t *testing.T, cfg string, players ...string) (*fermiGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newFermi(cfgRaw, fc.ctx(players, 5))
	if err != nil {
		t.Fatalf("newFermi: %v", err)
	}
	return g.(*fermiGame), fc
}

func TestFermiPointTable(t *testing.T) {
	cases := []struct {
		r    float64
		want int
	}{
		{1.1, 10}, {1.1000001, 8}, {1.25, 8}, {1.250001, 6},
		{1.5, 6}, {1.500001, 4}, {2, 4}, {2.000001, 2}, {3, 2}, {3.000001, 0}, {10, 0},
	}
	for _, c := range cases {
		if got := fermiPoints(c.r); got != c.want {
			t.Fatalf("fermiPoints(%f) = %d, want %d", c.r, got, c.want)
		}
	}
}

func TestFermiSymmetricMultiplicativeError(t *testing.T) {
	if multErr(5000, 10000) != 2 || multErr(20000, 10000) != 2 {
		t.Fatal("5000 and 20000 vs 10000 must both give r=2")
	}
	if fermiPoints(multErr(5000, 10000)) != 4 || fermiPoints(multErr(20000, 10000)) != 4 {
		t.Fatal("r=2 must earn 4 points from either side")
	}
	if fermiPoints(multErr(10500, 10000)) != 10 {
		t.Fatal("r=1.05 must earn 10 points")
	}
}

func fermiSetup(t *testing.T, players ...string) (*fermiGame, *fakeClock) {
	t.Helper()
	g, fc := fermiForTest(t, `{"rounds":2,"question_timer_ms":1000,"reveal_ms":400}`, players...)
	// Freeze deterministic questions with answer 10000.
	for i := range g.rounds {
		g.rounds[i] = FermiBankQ{ID: g.rounds[i].ID, Prompt: "test", Units: "u", Answer: 10000}
	}
	return g, fc
}

func TestFermiSubmitLockAndRejects(t *testing.T) {
	g, _ := fermiSetup(t, "p1", "p2")
	q := g.rounds[0]
	// Invalid values never count as submissions.
	for _, bad := range []float64{0, -5, -0.001} {
		out := g.Apply(engine.ActionRequest{ActionID: "bad", PlayerID: "p1", Action: "ESTIMATE",
			Payload: engine.MustJSON(map[string]any{"question_id": q.ID, "value": bad})}, 1)
		if out.Accepted || out.Reason != engine.ReasonInvalidValue {
			t.Fatalf("value %v should be rejected", bad)
		}
		if g.locked["p1"][0] {
			t.Fatalf("invalid value %v locked the player", bad)
		}
	}
	out := g.Apply(engine.ActionRequest{ActionID: "e1", PlayerID: "p1", Action: "ESTIMATE",
		Payload: engine.MustJSON(map[string]any{"question_id": q.ID, "value": 10500})}, 1)
	if !out.Accepted {
		t.Fatalf("valid estimate rejected: %s", out.Reason)
	}
	out = g.Apply(engine.ActionRequest{ActionID: "e2", PlayerID: "p1", Action: "ESTIMATE",
		Payload: engine.MustJSON(map[string]any{"question_id": q.ID, "value": 9000})}, 1)
	if out.Accepted || out.Reason != engine.ReasonAlreadyActed {
		t.Fatalf("second estimate must be rejected with ALREADY_ACTED, got %+v", out)
	}
	if g.estimates["p1"][0] != 10500 {
		t.Fatal("locked estimate changed")
	}
}

func TestFermiRevealOnAllSubmittedAndScoring(t *testing.T) {
	g, fc := fermiSetup(t, "p1", "p2")
	q := g.rounds[0]
	_ = g.Apply(engine.ActionRequest{ActionID: "e1", PlayerID: "p1", Action: "ESTIMATE",
		Payload: engine.MustJSON(map[string]any{"question_id": q.ID, "value": 5000})}, 1)
	// Only one submission: still in QUESTION phase.
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "QUESTION" {
		t.Fatal("should still be collecting")
	}
	_ = g.Apply(engine.ActionRequest{ActionID: "e2", PlayerID: "p2", Action: "ESTIMATE",
		Payload: engine.MustJSON(map[string]any{"question_id": q.ID, "value": 20000})}, 1)
	snap, _ = g.Snapshot("p2")
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "REVEAL" {
		t.Fatalf("all submitted should advance to REVEAL, got %v", s["phase"])
	}
	// Both r=2 -> 4 points each.
	rev := s["reveal"].(map[string]any)
	rows := rev["rows"].([]any)
	for _, r := range rows {
		row := r.(map[string]any)
		if int(row["points"].(float64)) != 4 {
			t.Fatalf("r=2 estimate must score 4, got %v", row["points"])
		}
	}
	// Question 2: reveal already started at t=0; round 1 opens at t=500.
	fc.advance(500)
	snap, _ = g.Snapshot("p2")
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "QUESTION" {
		t.Fatalf("round 1 should be collecting, got %v", s["phase"])
	}
	q2 := g.rounds[1]
	out := g.Apply(engine.ActionRequest{ActionID: "e3", PlayerID: "p2", Action: "ESTIMATE",
		Payload: engine.MustJSON(map[string]any{"question_id": q2.ID, "value": 10500})}, 1)
	if !out.Accepted {
		t.Fatalf("q2 estimate rejected: %s", out.Reason)
	}
	fc.advance(1500) // q2 timer expires (p1 misses) -> closes round
	snap, _ = g.Snapshot("p2")
	fc.advance(1000) // reveal completes -> game over
	snap, _ = g.Snapshot("p2")
	_ = json.Unmarshal(snap, &s)
	if s["finished"] != true {
		t.Fatal("last round should finish")
	}
	snap, _ = g.Snapshot("p2")
	_ = json.Unmarshal(snap, &s)
	if s["finished"] != true {
		t.Fatal("last round should finish")
	}
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
	if int(p1["total"].(float64)) != 4-2 {
		t.Fatalf("p1 total %v, want 2 (4 then -2)", p1["total"])
	}
	if int(p2["total"].(float64)) != 4+10 {
		t.Fatalf("p2 total %v, want 14", p2["total"])
	}
	// Shared ranks: p2 rank 1, p1 rank 2.
	if int(p2["rank"].(float64)) != 1 || int(p1["rank"].(float64)) != 2 {
		t.Fatalf("ranks wrong: %v %v", p1["rank"], p2["rank"])
	}
	// Multiplayer global: p2 outranks 1 player; total 14 < 40 so no threshold bonus.
	if int(p2["global_points"].(float64)) != 1 {
		t.Fatalf("p2 global %v, want 1", p2["global_points"])
	}
}

func TestFermiDeadlineMissingPenalty(t *testing.T) {
	g, fc := fermiSetup(t, "p1", "p2")
	fc.advance(1001) // question timer expires, nobody submitted
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	if s["phase"] != "REVEAL" {
		t.Fatalf("timer expiry should close the round, got %v", s["phase"])
	}
	rev := s["reveal"].(map[string]any)
	for _, r := range rev["rows"].([]any) {
		row := r.(map[string]any)
		if int(row["points"].(float64)) != -2 {
			t.Fatalf("missing submission must cost -2, got %v", row["points"])
		}
		if row["value"] != nil {
			t.Fatal("missing row must have nil value")
		}
	}
}

func TestFermiDeterminismAndGlobalThresholds(t *testing.T) {
	a, _ := fermiForTest(t, `{"rounds":10}`, "p1", "p2")
	b, _ := fermiForTest(t, `{"rounds":10}`, "p1", "p2")
	ta, _ := a.Tape()
	tb, _ := b.Tape()
	if string(ta) != string(tb) {
		t.Fatal("same seed must produce identical tape")
	}
	// Threshold bonus boundaries via direct points manipulation.
	g, _ := fermiSetup(t, "p1")
	g.points["p1"] = 80
	g.round = len(g.rounds) - 1
	g.phase = "REVEAL"
	g.phaseStartMS = 0
	g.nextRound(0)
	res, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	p := r["players"].([]any)[0].(map[string]any)
	if int(p["global_points"].(float64)) != 3 {
		t.Fatalf("total 80 solo must give +3, got %v", p["global_points"])
	}
}
