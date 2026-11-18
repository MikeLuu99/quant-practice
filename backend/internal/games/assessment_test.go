package games

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"quantgame/backend/internal/engine"
)

func assessmentForTest(t *testing.T, profile string, overrides map[string]any) (*assessmentGame, *fakeClock) {
	t.Helper()
	cfg := map[string]any{"profile_id": profile, "mode": "preview"}
	for k, v := range overrides {
		cfg[k] = v
	}
	fc := &fakeClock{}
	game, err := newAssessment(engine.MustJSON(cfg), fc.ctx([]string{"alice"}, 42))
	if err != nil {
		t.Fatal(err)
	}
	return game.(*assessmentGame), fc
}
func assessmentDo(g *assessmentGame, action string, payload any) engine.Output {
	return g.Apply(engine.ActionRequest{ActionID: fmt.Sprintf("action-%d", len(g.events)+1), PlayerID: "alice", Action: action, Payload: engine.MustJSON(payload)}, int64(len(g.events)+1))
}
func assessmentAnswerResponse(q assessmentQuestion) assessmentResponse {
	if q.ResponseType == "interval" {
		y, _ := parseExactNumber(q.Answer)
		return assessmentResponse{Low: canon(new(big.Rat).Mul(y, big.NewRat(9, 10))), High: canon(new(big.Rat).Mul(y, big.NewRat(11, 10))), Reasoning: "Original interval test"}
	}
	return assessmentResponse{Answer: q.Answer}
}
func TestAssessmentCatalogAndEveryProfilePlaythrough(t *testing.T) {
	profiles := AssessmentProfiles()
	if len(profiles) != 25 {
		t.Fatalf("profiles=%d", len(profiles))
	}
	firms := map[string]bool{}
	ids := map[string]bool{}
	for _, p := range profiles {
		if ids[p.ID] {
			t.Fatalf("duplicate profile %s", p.ID)
		}
		ids[p.ID] = true
		firms[p.CompanyID] = true
		for _, mode := range []string{"preview", "full"} {
			t.Run(p.ID+"/"+mode, func(t *testing.T) {
				g, fc := assessmentForTest(t, p.ID, map[string]any{"mode": mode})
				expectedCount := p.PreviewCount
				if mode == "full" {
					expectedCount = p.QuestionCount
				}
				if len(g.questions) != expectedCount {
					t.Fatal("wrong count")
				}
				for i, q := range g.questions {
					if g.index != i {
						t.Fatalf("cursor %d != %d", g.index, i)
					}
					fc.advance(11)
					if out := assessmentDo(g, "SAVE_ANSWER", map[string]any{"question_id": q.ID, "response": assessmentAnswerResponse(q)}); !out.Accepted {
						t.Fatalf("save %s: %+v", q.ID, out)
					}
					if i+1 == len(g.questions) && g.cfg.Navigation == "revisable" {
						if out := assessmentDo(g, "SUBMIT_TEST", nil); !out.Accepted {
							t.Fatal(out)
						}
					} else if out := assessmentDo(g, "NEXT", map[string]string{"question_id": q.ID}); !out.Accepted {
						t.Fatal(out)
					}
				}
				if !g.Finished() {
					t.Fatal("playthrough did not finish")
				}
				results, err := g.Results()
				if err != nil {
					t.Fatal(err)
				}
				var result map[string]any
				if err := json.Unmarshal(results, &result); err != nil {
					t.Fatal(err)
				}
				if p.ResponseMode == "interval" {
					if result["coverage_rate"] != float64(1) || result["raw_score"] != nil {
						t.Fatalf("interval result %s", results)
					}
				} else if result["raw_score"] != float64(len(g.questions)) {
					t.Fatalf("all correct score: %s", results)
				}
				tape, err := g.Tape()
				if err != nil {
					t.Fatal(err)
				}
				replayed, err := ReplayAssessment(tape)
				if err != nil {
					t.Fatalf("replay: %v", err)
				}
				if !bytes.Equal(results, replayed) {
					t.Fatal("replay bytes differ")
				}
			})
		}
	}
	if len(firms) != 11 {
		t.Fatalf("firms=%d", len(firms))
	}
}
func TestAssessmentPrivacyAndInvalidAnswers(t *testing.T) {
	for _, p := range AssessmentProfiles() {
		t.Run(p.ID, func(t *testing.T) {
			g, _ := assessmentForTest(t, p.ID, nil)
			raw, _ := g.Snapshot("alice")
			var s map[string]any
			_ = json.Unmarshal(raw, &s)
			q := s["question"].(map[string]any)
			for _, hidden := range []string{"answer", "model", "explanation", "correct_answer", "absolute_tolerance", "relative_tolerance"} {
				if _, ok := q[hidden]; ok {
					t.Fatalf("leaked %s", hidden)
				}
			}
			if _, err := g.Results(); !errors.Is(err, engine.ErrNotFinished) {
				t.Fatal("live results exposed")
			}
			if _, err := g.Tape(); !errors.Is(err, engine.ErrNotFinished) {
				t.Fatal("live tape exposed")
			}
			id := g.questions[0].ID
			out := assessmentDo(g, "SAVE_ANSWER", map[string]any{"question_id": id, "response": assessmentResponse{Answer: "<script>oops</script>"}})
			if out.Accepted || out.Reason != "INVALID_FORMAT" || len(g.responses) != 0 {
				t.Fatalf("invalid answer changed attempt: %+v", out)
			}
			response := assessmentAnswerResponse(g.questions[0])
			out = assessmentDo(g, "SAVE_ANSWER", map[string]any{"question_id": id, "response": response})
			if !out.Accepted || len(out.Execution) != 0 || out.RoundReward != 0 {
				t.Fatalf("receipt leaked correctness: %+v", out)
			}
		})
	}
}
func TestAssessmentRevisableEditsAndForwardLocks(t *testing.T) {
	g, _ := assessmentForTest(t, "drw-assessment", nil)
	q := g.questions[0]
	correct := assessmentAnswerResponse(q)
	wrong := correct
	for _, option := range q.Choices {
		if option.ID != q.Answer {
			wrong.Answer = option.ID
			break
		}
	}
	for _, r := range []assessmentResponse{wrong, correct, wrong, correct} {
		if out := assessmentDo(g, "SAVE_ANSWER", map[string]any{"question_id": q.ID, "response": r}); !out.Accepted {
			t.Fatal(out)
		}
	}
	_ = assessmentDo(g, "NEXT", map[string]string{"question_id": q.ID})
	second := g.questions[1]
	if out := assessmentDo(g, "PREVIOUS", map[string]string{"question_id": second.ID}); !out.Accepted {
		t.Fatal(out)
	}
	if g.responses[q.ID].Answer != q.Answer {
		t.Fatal("revision not restored")
	}
	_ = assessmentDo(g, "SUBMIT_TEST", nil)
	result, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(result, &r)
	if r["correct"] != float64(1) || r["blank"] != float64(2) {
		t.Fatalf("graded a revised question more than once: %s", result)
	}
	f, _ := assessmentForTest(t, "da-vinci-numerical", nil)
	first := f.questions[0]
	_ = assessmentDo(f, "NEXT", map[string]string{"question_id": first.ID})
	out := assessmentDo(f, "PREVIOUS", map[string]string{"question_id": f.questions[1].ID})
	if out.Accepted || out.Reason != "NAVIGATION_LOCKED" {
		t.Fatal(out)
	}
	out = assessmentDo(f, "SAVE_ANSWER", map[string]any{"question_id": first.ID, "response": assessmentAnswerResponse(first)})
	if out.Accepted || out.Reason != "STALE_QUESTION" {
		t.Fatal(out)
	}
}
func TestAssessmentGlobalDeadlineAndBlankScoring(t *testing.T) {
	for _, ms := range []int64{999, 1000, 1001} {
		t.Run(fmt.Sprint(ms), func(t *testing.T) {
			g, fc := assessmentForTest(t, "maven-numerical-new", map[string]any{"mode": "custom", "duration_ms": 1000, "question_count": 2})
			q := g.questions[0]
			fc.advance(ms)
			out := assessmentDo(g, "SAVE_ANSWER", map[string]any{"question_id": q.ID, "response": assessmentAnswerResponse(q)})
			if out.Accepted != (ms < 1000) {
				t.Fatalf("deadline: %+v", out)
			}
			if ms >= 1000 && out.Reason != engine.ReasonLate {
				t.Fatal(out)
			}
			fc.advance(10)
			g.Tick()
			result, err := g.Results()
			if err != nil {
				t.Fatal(err)
			}
			var r map[string]any
			_ = json.Unmarshal(result, &r)
			if r["elapsed_ms"] != float64(1000) {
				t.Fatalf("deadline drift: %s", result)
			}
			tape, _ := g.Tape()
			if _, err := ReplayAssessment(tape); err != nil {
				t.Fatal(err)
			}
		})
	}
	g, fc := assessmentForTest(t, "mako-codility", nil)
	fc.advance(g.cfg.DurationMS)
	g.Tick()
	result, _ := g.Results()
	var r map[string]any
	_ = json.Unmarshal(result, &r)
	if r["accuracy_attempted"] != float64(0) || r["raw_score"] != float64(0) || r["blank"] != float64(g.cfg.QuestionCount) {
		t.Fatalf("blank score: %s", result)
	}
}
func TestAssessmentPerItemTimersCatchUpWithoutDrift(t *testing.T) {
	g, fc := assessmentForTest(t, "akuna-grid", map[string]any{"mode": "custom", "question_count": 3, "item_time_ms": 1000})
	fc.advance(1000)
	g.Tick()
	if g.index != 1 || g.itemStart != 1000 {
		t.Fatal("first timeout")
	}
	fc.advance(2000)
	g.Tick()
	if !g.Finished() || g.finishedAt != 3000 {
		t.Fatalf("catchup: %+v", g.transitions)
	}
	result, _ := g.Results()
	tape, _ := g.Tape()
	replayed, err := ReplayAssessment(tape)
	if err != nil || !bytes.Equal(result, replayed) {
		t.Fatalf("timeout replay: %v", err)
	}
	h, hc := assessmentForTest(t, "akuna-grid", map[string]any{"mode": "custom", "question_count": 2, "item_time_ms": 1000})
	hc.advance(100)
	_ = assessmentDo(h, "NEXT", map[string]string{"question_id": h.questions[0].ID})
	if h.itemStart != 100 {
		t.Fatal("unused time carried")
	}
	hc.advance(999)
	h.Tick()
	if h.Finished() {
		t.Fatal("early item timeout")
	}
	hc.advance(1)
	h.Tick()
	if !h.Finished() || h.finishedAt != 1100 {
		t.Fatal("wrong explicit-advance deadline")
	}
}
func TestAssessmentExactRationalsAndPerItemTolerance(t *testing.T) {
	q := assessmentQuestion{ResponseType: "numeric", Answer: "3/4", AbsTolerance: "0", RelTolerance: "0"}
	for _, raw := range []string{"3/4", "0.75", "6/8", "7.5e-1"} {
		r, ok := normalizeAssessmentAnswer(q, assessmentResponse{Answer: raw})
		if !ok || !assessmentCorrect(q, r) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"1/0", "NaN", "Inf", "0xff", "1,000", "2+2", "1e99"} {
		if _, ok := normalizeAssessmentAnswer(q, assessmentResponse{Answer: raw}); ok {
			t.Fatalf("accepted %s", raw)
		}
	}
	q.Answer = "2/3"
	q.AbsTolerance = "1/200"
	r, _ := normalizeAssessmentAnswer(q, assessmentResponse{Answer: "0.67"})
	if !assessmentCorrect(q, r) {
		t.Fatal("explicit item tolerance not applied")
	}
	q.Answer = "1/1000000"
	q.AbsTolerance = "0"
	if assessmentCorrect(q, assessmentResponse{Answer: "0"}) {
		t.Fatal("tiny probability rounded to zero without permission")
	}
}
func TestAssessmentIntervalLossAndBlankInterval(t *testing.T) {
	for _, test := range []struct {
		low, high string
		loss      float64
		covered   bool
	}{{"80", "120", 40, true}, {"105", "115", 60, false}, {"90", "95", 55, false}} {
		t.Run(test.low, func(t *testing.T) {
			g, _ := assessmentForTest(t, "maven-interval", nil)
			g.questions[0].Answer = "100"
			out := assessmentDo(g, "SAVE_ANSWER", map[string]any{"question_id": g.questions[0].ID, "response": assessmentResponse{Low: test.low, High: test.high}})
			if !out.Accepted {
				t.Fatal(out)
			}
			_ = assessmentDo(g, "SUBMIT_TEST", nil)
			raw, _ := g.Results()
			var result map[string]any
			_ = json.Unmarshal(raw, &result)
			review := result["review"].([]any)[0].(map[string]any)
			if review["interval_loss"] != test.loss || review["covered"] != test.covered {
				t.Fatalf("bad loss: %s", raw)
			}
		})
	}
	g, fc := assessmentForTest(t, "maven-interval", nil)
	fc.advance(g.cfg.DurationMS)
	raw, _ := g.Results()
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	if result["mean_interval_loss"] != nil || result["intervals_answered"] != float64(0) {
		t.Fatal("blank interval treated as perfect")
	}
}
func TestAssessmentCubeProperRotationsAndGridEnumeration(t *testing.T) {
	for seed := uint64(1); seed <= 50; seed++ {
		q := cubeTask(engine.NewRNG(seed))
		model := q.Model.(map[string]any)
		frames := model["folded_frames"].(map[string]cubeFrame)
		if model["proper_rotation_count"] != 24 {
			t.Fatal("cube must have 24 rotations")
		}
		asset := q.Assets.(map[string]any)
		valid := 0
		for _, c := range asset["candidates"].([]cubeCandidate) {
			if validCubeCandidate(frames, c) {
				valid++
				if c.ID != q.Answer {
					t.Fatal("wrong cube key")
				}
			}
		}
		if valid != 1 {
			t.Fatalf("cube has %d valid choices", valid)
		}
		grid := gridTask(engine.NewRNG(seed))
		a := grid.Assets.(map[string]any)
		m := grid.Model.(map[string]any)
		rows := a["rows"].([][2]string)
		bets := a["bets"].([]gridBet)
		weights := m["weights"].([]int)
		evs := m["expected_net_payouts"].([]string)
		for i, bet := range bets {
			total, numerator := 0, 0
			for r, w := range weights {
				total += w
				payoff := 0
				for _, c := range bet.Components {
					hit := rows[r][0] == c.Letter || rows[r][1] == c.Letter
					if hit {
						payoff += 12 * c.Sign
					} else {
						payoff -= c.Sign
					}
				}
				numerator += w * payoff
			}
			expected := big.NewRat(int64(numerator), int64(total))
			cost, _ := parseExactNumber(bet.Cost)
			expected.Sub(expected, cost)
			if canon(expected) != evs[i] {
				t.Fatal("grid EV differs from independent row enumeration")
			}
		}
	}
}
func TestAssessmentConfigAndReplayCorruption(t *testing.T) {
	invalid := []map[string]any{{"mode": "full", "question_count": 1}, {"mode": "custom", "question_count": 301}, {"mode": "custom", "duration_ms": -1}, {"mode": "custom", "navigation": "rewind"}, {"mode": "custom", "response_mode": "interval"}, {"mode": "custom", "family": "not-a-family"}}
	for _, values := range invalid {
		values["profile_id"] = "maven-numerical-new"
		if _, _, err := resolveAssessmentConfig(engine.MustJSON(values)); err == nil {
			t.Fatalf("accepted %+v", values)
		}
	}
	g, _ := assessmentForTest(t, "mako-aptitude", nil)
	_ = assessmentDo(g, "SUBMIT_TEST", nil)
	raw, _ := g.Tape()
	var tape assessmentTape
	_ = json.Unmarshal(raw, &tape)
	tape.Questions[0].Prompt += " tampered"
	if _, err := ReplayAssessment(engine.MustJSON(tape)); err == nil {
		t.Fatal("corrupted question hash accepted")
	}
}
