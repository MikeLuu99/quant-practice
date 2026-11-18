package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func TestMathPlayersProgressIndependently(t *testing.T) {
	fc := &fakeClock{}
	game, err := newMath(json.RawMessage(`{"count":2,"duration_ms":10000}`), fc.ctx([]string{"alice", "bob"}, 42))
	if err != nil {
		t.Fatal(err)
	}
	g := game.(*mathGame)
	answer := func(player string, index int) engine.Output {
		q := g.tape.Questions[index]
		return g.Apply(engine.ActionRequest{PlayerID: player, Action: "ANSWER", Payload: engine.MustJSON(map[string]string{"question_id": q.ID, "answer": q.AnswerNum})}, 1)
	}
	if out := answer("alice", 0); !out.Accepted {
		t.Fatal(out)
	}
	raw, _ := g.Snapshot("bob")
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["index"] != float64(0) || snapshot["score"] != float64(0) {
		t.Fatalf("Alice changed Bob's state: %s", raw)
	}
	if out := answer("alice", 1); !out.Accepted || out.GameOver || g.Finished() {
		t.Fatalf("Alice must not finish Bob's test: %+v", out)
	}
	if out := answer("bob", 0); !out.Accepted {
		t.Fatal(out)
	}
	if out := answer("bob", 1); !out.Accepted || !out.GameOver || !g.Finished() {
		t.Fatalf("all players must finish: %+v", out)
	}
	res, err := g.Results()
	if err != nil {
		t.Fatal(err)
	}
	var results struct {
		Players []struct {
			Player string
			Score  int
			Rank   int
		}
	}
	if err := json.Unmarshal(res, &results); err != nil {
		t.Fatal(err)
	}
	if len(results.Players) != 2 {
		t.Fatalf("wrong results: %s", res)
	}
	for _, p := range results.Players {
		if p.Score != 2 || p.Rank != 1 {
			t.Fatalf("wrong player result: %+v", p)
		}
	}
}

func TestMathExhaustionFinishesSoloSession(t *testing.T) {
	g, _ := mathForTest(t, 42, `{"count":1}`)
	out := mathAnswer(g, "q0", g.tape.Questions[0].AnswerNum)
	if !out.Accepted || !out.GameOver || !g.Finished() {
		t.Fatalf("completed tape left session live: %+v", out)
	}
}

func TestNextCardCompletedSessionHasReadableSnapshot(t *testing.T) {
	g, fc := ncForTest(t, `{"suits":1,"max_rounds":1,"result_ms":100}`, "p1")
	out := g.Apply(engine.ActionRequest{PlayerID: "p1", Action: "PASS"}, 1)
	if !out.Accepted {
		t.Fatal(out)
	}
	if _, err := g.Snapshot("p1"); err != nil {
		t.Fatal(err)
	} // settle
	fc.advance(101)
	// Previously Snapshot indexed one past the end of the deck here.
	for range 3 {
		raw, err := g.Snapshot("p1")
		if err != nil {
			t.Fatal(err)
		}
		var snapshot map[string]any
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot["finished"] != true || snapshot["phase"] != "DONE" || snapshot["round"] != float64(1) {
			t.Fatalf("bad final snapshot: %s", raw)
		}
	}
}

func TestMakeMeRevealedBookSurvivesPolling(t *testing.T) {
	g, fc := mmForTest(t, `{"rounds":1,"solo_pause_ms":100}`, 42, "p1")
	if out := mmDoQuote(g, "p1", 1, 1); !out.Accepted {
		t.Fatal(out)
	}
	fc.advance(101)
	_, _ = g.Snapshot("p1")
	if g.phase != "BOOK" {
		t.Fatalf("phase = %s", g.phase)
	}
	before, _ := g.Snapshot("p1")
	var snapshot map[string]any
	_ = json.Unmarshal(before, &snapshot)
	if snapshot["my_book"] != nil {
		t.Fatal("book exposed before reveal")
	}
	out := g.Apply(engine.ActionRequest{PlayerID: "p1", Action: "REVEAL_HISTORY"}, 1)
	if !out.Accepted {
		t.Fatal(out)
	}
	after, _ := g.Snapshot("p1")
	snapshot = nil
	_ = json.Unmarshal(after, &snapshot)
	if snapshot["my_book"] == nil {
		t.Fatal("revealed book lost on next poll")
	}
	out = g.Apply(engine.ActionRequest{PlayerID: "p1", Action: "ANSWER_BOOK_QUESTION", Payload: engine.MustJSON(map[string]string{"answer": "0"})}, 2)
	if !out.Accepted {
		t.Fatal(out)
	}
	next, _ := g.Snapshot("p1")
	snapshot = nil
	_ = json.Unmarshal(next, &snapshot)
	if snapshot["my_book"] != nil {
		t.Fatal("reveal leaked into next question")
	}
}

func TestNextCardExplicitPassIsCorrectWhenNoEdge(t *testing.T) {
	g, _ := ncForTest(t, `{"suits":1,"max_rounds":1}`, "p1")
	// Six remaining ranks on either side of 7: no favorable direction.
	g.tape.Rounds = []ncRound{{Mode: "higher_lower", Cycle: 0, Ref1: ncCard{Rank: 7}, Draw: ncCard{Rank: 8}}}
	out := g.Apply(engine.ActionRequest{PlayerID: "p1", Action: "PASS"}, 1)
	if !out.Accepted {
		t.Fatal(out)
	}
	_, _ = g.Snapshot("p1")
	result := g.players["p1"].results[0]
	if !result.DecisionCorrect || result.Outcome != "MISS" || result.Delta != 0 {
		t.Fatalf("explicit pass misgraded: %+v", result)
	}
}
func TestProbabilitySamplesOutcomeMassRatherThanDescriptionCount(t *testing.T) {
	rng := engine.NewRNG(42)
	primitive := probPrimitive{Type: "test", Outcomes: []probOutcome{{Desc: "rare", P: 0.1}, {Desc: "common", P: 0.9}}}
	rare := 0
	for range 10000 {
		if samplePrimitive(rng, primitive) == 0 {
			rare++
		}
	}
	if rare < 850 || rare > 1150 {
		t.Fatalf("rare outcome frequency=%d/10000, want about 0.1", rare)
	}
}
