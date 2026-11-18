package games

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"quantgame/backend/internal/engine"
)

func trainerForTest(t *testing.T, id, mode string) (*trainerGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	game, err := newTrainer(engine.MustJSON(map[string]any{"trainer_id": id, "mode": mode}), fc.ctx([]string{"alice"}, 42))
	if err != nil {
		t.Fatal(err)
	}
	g := game.(*trainerGame)
	trainerAction(t, g, fc, "BEGIN", nil)
	return g, fc
}
func trainerAction(t *testing.T, g *trainerGame, fc *fakeClock, action string, p map[string]any) engine.Output {
	t.Helper()
	if p == nil {
		p = map[string]any{}
	}
	p["elapsed_ms"] = g.ctx.ElapsedMS() - g.startedAt
	if g.current() != nil {
		p["trial_id"] = g.current().ID
	}
	out := g.Apply(engine.ActionRequest{ActionID: fmt.Sprintf("a-%d", len(g.events)), PlayerID: "alice", Action: action, Payload: engine.MustJSON(p)}, int64(len(g.events)+1))
	if !out.Accepted {
		t.Fatalf("%s %s/%s phase%s: %+v", action, g.cfg.TrainerID, g.cfg.Mode, g.phase, out)
	}
	return out
}
func trainerPresent(t *testing.T, g *trainerGame, fc *fakeClock) {
	if g.phase == "FEEDBACK" {
		trainerAction(t, g, fc, "NEXT", nil)
	}
	if g.done {
		return
	}
	trainerAction(t, g, fc, "PRESENT", nil)
	q := g.current()
	if q.EncodeMS+q.MaskMS > 0 {
		fc.advance(q.EncodeMS + q.MaskMS)
		trainerAction(t, g, fc, "ADVANCE_TIME", nil)
	}
}
func trainerSolve(t *testing.T, g *trainerGame, fc *fakeClock) {
	t.Helper()
	trial := g.current()
	fc.advance(20)
	if trial.Question != nil {
		trainerAction(t, g, fc, "ANSWER", map[string]any{"answer": trial.Question.Answer})
		return
	}
	switch trial.Kind {
	case "task-switch":
		trainerAction(t, g, fc, "RESPOND", map[string]any{"yes": trial.Secret["yes"], "reaction_time_ms": 20})
	case "response-conflict":
		trainerAction(t, g, fc, "RESPOND", map[string]any{"side": trial.Secret["side"], "reaction_time_ms": 20})
	case "pincode":
		trainerAction(t, g, fc, "RESPOND", map[string]any{"answer": trial.Secret["answer"]})
	case "code-match":
		trainerAction(t, g, fc, "RESPOND", map[string]any{"choice": trial.Secret["answer"]})
	case "grid-recall":
		trainerAction(t, g, fc, "RECALL", map[string]any{"cells": trial.Secret["targets"]})
	case "hold-fire":
		if trial.Secret["target"] == true {
			trainerAction(t, g, fc, "PRESS", nil)
		} else {
			fc.advance(trial.ResponseMS)
			trainerAction(t, g, fc, "ADVANCE_TIME", nil)
		}
	case "intercept":
		cross := trainerDecode[[]map[string]float64](trial.Public["crossings"])[0]
		wait := int64(math.Round(cross["ideal_fire_s"]*1000)) - (g.ctx.ElapsedMS() - g.startedAt - g.onset)
		fc.advance(wait)
		trainerAction(t, g, fc, "FIRE", map[string]any{"cannon": 0})
		fc.advance(int64(math.Ceil(cross["travel_s"]*1000)) + 2)
		trainerAction(t, g, fc, "ADVANCE_TIME", nil)
	case "target-number":
		for _, step := range trainerDecode[[]numberStep](trial.Secret["solution"]) {
			trainerAction(t, g, fc, "COMBINE", map[string]any{"left": step.Left, "right": step.Right, "operator": step.Operator})
		}
	case "skyline":
		for _, step := range trainerDecode[[]skylineMove](trial.Secret["solution"]) {
			trainerAction(t, g, fc, "MOVE", map[string]any{"from": step.From, "to": step.To})
		}
	case "mystery":
		trainerAction(t, g, fc, "GUESS", map[string]any{"values": trial.Secret["secret"]})
	case "pump":
		if g.cfg.Mode == "manual" {
			trainerAction(t, g, fc, "CASH_IN", nil)
		} else {
			trainerAction(t, g, fc, "COMMIT_TARGET", map[string]any{"target": 0})
		}
	case "sweet-spot":
		d := g.dials[0]
		angle := trainerSignedAngle(d.Target - d.Theta)
		if d.Omega > 0 && angle < 0 {
			angle += 360
		}
		if d.Omega < 0 && angle > 0 {
			angle -= 360
		}
		at := d.Spawn + int64(math.Round(angle/d.Omega*1000))
		if at < g.ctx.ElapsedMS()-g.startedAt {
			at += int64(math.Round(360 / math.Abs(d.Omega) * 1000))
		}
		fc.advance(at - (g.ctx.ElapsedMS() - g.startedAt))
		trainerAction(t, g, fc, "CLICK", map[string]any{"x": d.X, "y": d.Y})
	default:
		if g.cfg.Mode == "case" {
			fair := trainerFloat(trial.Secret["fair"])
			half := trainerFloat(trial.Public["max_spread"]) / 4
			trainerAction(t, g, fc, "QUOTE", map[string]any{"bid": fmt.Sprint(fair - half), "ask": fmt.Sprint(fair + half), "size": 1})
		} else {
			t.Fatalf("unhandled %s", trial.Kind)
		}
	}
}
func TestTrainerAll22AndEveryModeReplay(t *testing.T) {
	if len(TrainerProfiles()) != 22 {
		t.Fatal("catalogue count")
	}
	for _, profile := range TrainerProfiles() {
		for _, mode := range profile.Modes {
			t.Run(profile.ID+"/"+mode, func(t *testing.T) {
				g, fc := trainerForTest(t, profile.ID, mode)
				steps := g.cfg.Rounds
				if steps == 0 {
					steps = 2
				}
				if profile.ID == "sweet-spot" {
					steps = 1
				}
				for i := 0; i < steps && !g.done; i++ {
					trainerPresent(t, g, fc)
					trainerSolve(t, g, fc)
				}
				if !g.done {
					trainerAction(t, g, fc, "END_SESSION", nil)
				}
				result, err := g.Results()
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(result, []byte("NaN")) {
					t.Fatal("invalid number")
				}
				tape, err := g.Tape()
				if err != nil {
					t.Fatal(err)
				}
				replayed, err := ReplayTrainer(tape)
				if err != nil {
					t.Fatalf("replay: %v", err)
				}
				if !bytes.Equal(result, replayed) {
					t.Fatal("different results")
				}
				if profile.ID == "hold-fire" {
					var r map[string]any
					_ = json.Unmarshal(result, &r)
					if r["balanced_accuracy"] != float64(1) {
						t.Fatalf("balanced accuracy %v", r["balanced_accuracy"])
					}
				}
			})
		}
	}
}
func TestTrainerMemoryIsolationAndInputPhases(t *testing.T) {
	for _, id := range []string{"pincode", "grid-recall"} {
		t.Run(id, func(t *testing.T) {
			g, fc := trainerForTest(t, id, "")
			trainerAction(t, g, fc, "PRESENT", nil)
			trial := g.current()
			if trial.EncodeMS <= 0 {
				t.Fatal("missing encode")
			}
			fc.advance(trial.EncodeMS + trial.MaskMS)
			trainerAction(t, g, fc, "ADVANCE_TIME", nil)
			raw, _ := g.Snapshot("alice")
			var state map[string]any
			_ = json.Unmarshal(raw, &state)
			scene := state["scene"].(map[string]any)
			for _, key := range []string{"digits", "answer", "blue", "red", "targets", "decoys"} {
				if _, ok := scene[key]; ok {
					t.Fatalf("memory leak %s %s", key, raw)
				}
			}
			if bytes.Contains(raw, []byte(`"secret"`)) {
				t.Fatal("secret envelope exposed")
			}
			trainerSolve(t, g, fc)
		})
	}
}
func TestTrainerRoundingAndPuzzleWitnesses(t *testing.T) {
	if roundedFractionDigits(5, 12, 5) != "41667" || roundedFractionDigits(19999, 20000, 4) != "10000" || roundedFractionDigits(1, 8, 2) != "13" {
		t.Fatal("half up rounding")
	}
	for _, mode := range []string{"open", "capped"} {
		g, fc := trainerForTest(t, "skyline", mode)
		for i := 0; i < 10; i++ {
			trainerPresent(t, g, fc)
			opt := trainerInt(g.current().Public["minimum_moves"])
			if opt < 2 || opt > 7 {
				t.Fatal("distance")
			}
			trainerSolve(t, g, fc)
			if g.feedback["excess_moves"] != 0 {
				t.Fatal("witness not optimal")
			}
		}
	}
	g, fc := trainerForTest(t, "target-number", "")
	for i := 0; i < 10; i++ {
		trainerPresent(t, g, fc)
		trainerSolve(t, g, fc)
		if g.feedback["correct"] != true {
			t.Fatal("invalid witness")
		}
	}
}
func TestTrainerPumpCensoringAndStakesAccounting(t *testing.T) {
	g, fc := trainerForTest(t, "pump", "manual")
	trainerPresent(t, g, fc)
	trainerAction(t, g, fc, "CASH_IN", nil)
	raw, _ := g.Snapshot("alice")
	if bytes.Contains(raw, []byte(`"threshold"`)) || bytes.Contains(raw, []byte(`"solution"`)) {
		t.Fatalf("cashed threshold leaked: %s", raw)
	}
	g, fc = trainerForTest(t, "pump", "stakes")
	trainerPresent(t, g, fc)
	k := trainerInt(g.current().Secret["threshold"])
	trainerAction(t, g, fc, "COMMIT_TARGET", map[string]any{"target": 127})
	if k < 128 && g.bank != -(k-1)*10 {
		t.Fatal("pop penalty")
	}
}
func TestTrainerHoldFirePressAllAndNone(t *testing.T) {
	for _, press := range []bool{true, false} {
		g, fc := trainerForTest(t, "hold-fire", "standard")
		for !g.done {
			trainerPresent(t, g, fc)
			if press {
				trainerAction(t, g, fc, "PRESS", nil)
			} else {
				fc.advance(g.current().ResponseMS)
				trainerAction(t, g, fc, "ADVANCE_TIME", nil)
			}
		}
		raw, _ := g.Results()
		var r map[string]any
		_ = json.Unmarshal(raw, &r)
		if r["balanced_accuracy"] != .5 {
			t.Fatal(string(raw))
		}
	}
}
func TestTrainerExpiryAndTamperedTape(t *testing.T) {
	g, fc := trainerForTest(t, "math", "")
	trainerPresent(t, g, fc)
	fc.advance(g.cfg.DurationMS)
	out := g.Apply(engine.ActionRequest{PlayerID: "alice", Action: "ANSWER", Payload: engine.MustJSON(map[string]any{"trial_id": g.current().ID, "answer": g.current().Question.Answer, "elapsed_ms": g.cfg.DurationMS})}, 3)
	if out.Accepted || g.metrics["correct"] != 0 {
		t.Fatal("late answer accepted")
	}
	tape, _ := g.Tape()
	var parsed trainerTape
	_ = json.Unmarshal(tape, &parsed)
	parsed.Trials[0].Question.Answer = "999999"
	if _, err := ReplayTrainer(engine.MustJSON(parsed)); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatal("tamper accepted", err)
	}
}

func TestTrainerDialsFrameIndependenceAndFullExpiryReplay(t *testing.T) {
	var results [][]byte
	for _, stepped := range []bool{false, true} {
		g, fc := trainerForTest(t, "sweet-spot", "")
		trainerPresent(t, g, fc)
		if stepped {
			for i := 0; i < 24; i++ {
				fc.advance(5000)
				if fc.ms >= 120000 {
					g.Tick()
				} else {
					trainerAction(t, g, fc, "ADVANCE_TIME", nil)
				}
				if i == 19 && len(g.dials) != 6 {
					t.Fatalf("expected six final-stage dials, got%d", len(g.dials))
				}
			}
		} else {
			fc.advance(120000)
			g.Tick()
		}
		if !g.done || g.score >= 0 || len(g.dials) != 0 {
			t.Fatal("expiry accounting")
		}
		raw, _ := g.Results()
		results = append(results, raw)
		tape, _ := g.Tape()
		if _, err := ReplayTrainer(tape); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(results[0], results[1]) {
		t.Fatal("frame cadence changed the result")
	}
}
