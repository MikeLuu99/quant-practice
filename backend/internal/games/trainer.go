package games

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"quantgame/backend/internal/engine"
)

type trainerEvent struct {
	AtMS     int64                `json:"at_ms"`
	Sequence int64                `json:"sequence"`
	Request  engine.ActionRequest `json:"request"`
	Accepted bool                 `json:"accepted"`
	Reason   string               `json:"reason,omitempty"`
}
type trainerGame struct {
	ctx            *engine.Ctx
	cfg            trainerConfig
	profile        TrainerProfile
	phase          string
	started        bool
	done           bool
	startedAt      int64
	simMS          int64
	onset          int64
	responseOnset  int64
	feedbackUntil  int64
	level          int
	streak         int
	lastRule       int
	trials         []trainerTrial
	frozen         []trainerTrial
	history        []map[string]any
	events         []trainerEvent
	move           trainerMoveState
	undo           []trainerMoveState
	score          float64
	bank           int
	book           trainerBook
	feedback       map[string]any
	comparable     bool
	interruptions  []map[string]any
	holdTargets    []bool
	pumpThresholds []int
	dials          []trainerDial
	dialSerial     int
	nextDialStep   int64
	metrics        map[string]float64
	pending        map[string]any
	failure        string
}

func newTrainer(raw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	if len(ctx.Players) != 1 {
		return nil, fmt.Errorf("trainers require one player")
	}
	c, p, err := resolveTrainerConfig(raw)
	if err != nil {
		return nil, err
	}
	g := trainerFromConfig(ctx, c, p, nil)
	return g, nil
}
func trainerFromConfig(ctx *engine.Ctx, c trainerConfig, p TrainerProfile, frozen []trainerTrial) *trainerGame {
	g := &trainerGame{ctx: ctx, cfg: c, profile: p, phase: "INSTRUCTIONS", level: c.Difficulty, trials: []trainerTrial{}, frozen: frozen, history: []map[string]any{}, events: []trainerEvent{}, comparable: true, interruptions: []map[string]any{}, metrics: map[string]float64{}}
	if c.TrainerID == "pincode" {
		g.level = min(5, c.MaxSpan)
		if c.FixedSpan > 0 {
			g.level = c.FixedSpan
		}
	}
	if c.TrainerID == "grid-recall" {
		g.level = 3
	}
	rng := trainerRNG(ctx.Seed, "session-schedule:"+c.TrainerID, 0, 0)
	g.holdTargets = make([]bool, max(60, c.Rounds))
	for i := range g.holdTargets {
		g.holdTargets[i] = i < int(math.Round(.8*float64(len(g.holdTargets))))
	}
	rng.Shuffle(len(g.holdTargets), func(i, j int) { g.holdTargets[i], g.holdTargets[j] = g.holdTargets[j], g.holdTargets[i] })
	g.pumpThresholds = make([]int, 128)
	for i := range g.pumpThresholds {
		g.pumpThresholds[i] = i + 1
	}
	rng.Shuffle(128, func(i, j int) { g.pumpThresholds[i], g.pumpThresholds[j] = g.pumpThresholds[j], g.pumpThresholds[i] })
	g.book = trainerBook{Multiplier: 1, Delta: 1, Spot: 100, Ledger: []map[string]any{}}
	if c.TrainerID == "options" {
		g.book.Multiplier = 100
		g.book.Delta = .6
	}
	return g
}
func (g *trainerGame) Meta() engine.GameMeta {
	return engine.GameMeta{Slug: "trainer", Name: g.profile.Title, Access: "free", MinPlayers: 1, MaxPlayers: 1, DurationMS: g.cfg.DurationMS, Description: g.profile.Description, Rules: "Original practice implementation; preserve mode, timing and raw score components."}
}
func (g *trainerGame) current() *trainerTrial {
	if len(g.trials) == 0 {
		return nil
	}
	return &g.trials[len(g.trials)-1]
}
func (g *trainerGame) nextTrial() {
	if g.cfg.Rounds > 0 && len(g.history) >= g.cfg.Rounds {
		g.finish("completed")
		return
	}
	index := len(g.trials)
	var t trainerTrial
	if g.frozen != nil {
		if index >= len(g.frozen) {
			g.failure = "replay requested a missing scenario"
			g.finish("replay_error")
			return
		}
		t = trainerClone(g.frozen[index])
		if t.Level != g.level {
			g.failure = "adaptive level diverged"
			g.finish("replay_error")
			return
		}
	} else {
		t = g.generateTrial()
	}
	g.trials = append(g.trials, t)
	g.move = trainerMoveState{}
	g.undo = nil
	g.pending = nil
	g.feedback = nil
	g.phase = "READY"
	g.onset = g.simMS
	g.responseOnset = g.simMS
	if t.Kind == "target-number" {
		g.move.Tiles = trainerDecode[[]trainerTile](t.Public["sources"])
	}
	if t.Kind == "skyline" {
		g.move.Stacks = trainerDecode[[][]string](t.Public["start"])
	}
	if t.Kind == "sweet-spot" {
		g.nextDialStep = 24000
		g.fillDials(0)
	}
}
func (g *trainerGame) Tick() {
	if !g.started || g.done || g.cfg.DurationMS == 0 {
		return
	}
	if g.ctx.ElapsedMS()-g.startedAt >= g.cfg.DurationMS {
		g.advanceSim(g.cfg.DurationMS)
		if !g.done {
			g.finish("deadline")
		}
	}
}
func (g *trainerGame) Finished() bool { g.Tick(); return g.done }
func (g *trainerGame) finish(reason string) {
	if g.done {
		return
	}
	if g.cfg.TrainerID == "sweet-spot" {
		g.advanceDials(g.simMS)
		for _, d := range g.dials {
			g.metrics["expiries"]++
			g.history = append(g.history, map[string]any{"dial_id": d.ID, "status": "expired_at_end", "at_ms": g.simMS, "points": -1})
			g.score--
		}
		g.dials = nil
	}
	g.done = true
	g.phase = "DONE"
	g.metrics["finished_at_ms"] = float64(g.simMS)
	if reason != "completed" && reason != "deadline" {
		g.comparable = false
	}
	g.metrics["stop_reason:"+reason] = 1
}
func (g *trainerGame) advanceSim(to int64) {
	if to < g.simMS || g.done {
		return
	}
	if g.cfg.DurationMS > 0 {
		to = min(to, g.cfg.DurationMS)
	}
	g.simMS = to
	if g.cfg.TrainerID == "sweet-spot" && g.started {
		g.advanceDials(to)
		if g.cfg.DurationMS > 0 && to >= g.cfg.DurationMS {
			g.finish("deadline")
		}
		return
	}
	t := g.current()
	if t == nil {
		return
	}
	if g.phase == "ENCODE" && to >= g.onset+t.EncodeMS {
		g.phase = "MASK"
	}
	if g.phase == "MASK" && to >= g.onset+t.EncodeMS+t.MaskMS {
		g.phase = "RESPONSE"
		g.responseOnset = g.onset + t.EncodeMS + t.MaskMS
	}
	if g.phase == "RESPONSE" && t.ResponseMS > 0 && to >= g.responseOnset+t.ResponseMS {
		g.timeoutTrial(g.responseOnset + t.ResponseMS)
	}
	if g.phase == "ANIMATE" && to >= g.feedbackUntil {
		pending := g.pending
		g.pending = nil
		g.completeTrial(pending)
	}
	if g.phase == "FEEDBACK" && g.autoNext() && to >= g.feedbackUntil {
		g.nextTrial()
	}
	if !g.done && g.cfg.DurationMS > 0 && to >= g.cfg.DurationMS {
		g.finish("deadline")
	}
}
func (g *trainerGame) autoNext() bool {
	if g.cfg.Mode == "learn" || g.cfg.TrainerID == "fermi" || g.cfg.TrainerID == "skyline" || g.cfg.TrainerID == "mystery" || g.cfg.TrainerID == "pump" || (g.cfg.TrainerID == "fractions" && g.cfg.Mode == "practice") {
		return false
	}
	return g.cfg.DurationMS > 0 || slices.Contains([]string{"code-match", "grid-recall", "intercept", "hold-fire", "target-number"}, g.cfg.TrainerID)
}
func (g *trainerGame) timeoutTrial(at int64) {
	if g.cfg.TrainerID == "hold-fire" {
		target, _ := g.current().Secret["target"].(bool)
		status := "correct_rejection"
		if target {
			status = "miss"
		}
		g.completeTrial(map[string]any{"correct": !target, "status": status, "points": 0, "at_ms": at})
		return
	}
	g.completeTrial(map[string]any{"correct": false, "status": "timeout", "points": 0, "at_ms": at})
}
func (g *trainerGame) completeTrial(result map[string]any) {
	if g.phase == "FEEDBACK" || g.done {
		return
	}
	t := g.current()
	result["trial_id"] = t.ID
	result["index"] = t.Index
	result["level"] = t.Level
	result["ended_ms"] = g.simMS
	correct, _ := result["correct"].(bool)
	status := trainerString(result["status"])
	points := trainerFloat(result["points"])
	g.score += points
	g.metrics["completed"]++
	if status == "skipped" {
		g.metrics["skipped"]++
	} else if status == "timeout" {
		g.metrics["timeouts"]++
	} else if correct {
		g.metrics["correct"]++
	} else {
		g.metrics["wrong"]++
	}
	if t.Kind == "hold-fire" {
		g.metrics[status]++
		target, _ := t.Secret["target"].(bool)
		if target {
			g.metrics["targets"]++
		} else {
			g.metrics["non_targets"]++
		}
	}
	if t.Kind == "pincode" && g.cfg.FixedSpan == 0 {
		if correct {
			g.level = min(g.cfg.MaxSpan, g.level+1)
		} else {
			g.level = max(4, g.level-1)
		}
	}
	if t.Kind == "grid-recall" {
		if correct {
			g.streak++
			if g.streak >= 2 {
				g.level = min(12, g.level+1)
				g.streak = 0
			}
		} else {
			g.level = max(1, g.level-1)
			g.streak = 0
		}
	}
	if t.Kind == "intercept" || t.Kind == "hold-fire" {
		if correct {
			g.streak++
			if g.streak >= 2 {
				g.level = min(18, g.level+1)
				g.streak = 0
			}
		} else {
			g.level = max(1, g.level-1)
			g.streak = 0
		}
	}
	result["next_level"] = g.level
	result["state"] = trainerClone(g.move)
	if t.Question != nil {
		result["answer"] = assessmentAnswerText(*t.Question)
		result["explanation"] = t.Question.Explanation
	} else {
		if t.Kind != "pump" {
			result["solution"] = t.Secret
		}
	}
	result["stimulus"] = trainerClone(t.Public)
	if t.Question != nil {
		result["question"] = assessmentQuestionPublic(*t.Question)
	}
	result["book"] = trainerClone(g.book)
	g.feedback = result
	g.history = append(g.history, result)
	g.phase = "FEEDBACK"
	g.feedbackUntil = g.simMS + 450
	if g.cfg.Rounds > 0 && len(g.history) >= g.cfg.Rounds {
		g.finish("completed")
	}
}
func (g *trainerGame) visibleScene() map[string]any {
	t := g.current()
	if t == nil {
		return nil
	}
	scene := trainerClone(t.Public)
	elapsed := max(int64(0), g.simMS-g.onset)
	if g.phase == "READY" {
		elapsed = 0
	}
	encoding := g.phase == "READY" || g.phase == "ENCODE"
	if t.Kind == "pincode" && encoding {
		value := trainerString(t.Secret["digits"])
		i := int(elapsed / 650)
		if i < len(value) && elapsed%650 < 500 {
			scene["digit"] = string(value[i])
		}
		scene["digit_index"] = i
	}
	if t.Kind == "grid-recall" && encoding {
		targets := trainerDecode[[]int](t.Secret["targets"])
		if g.cfg.Mode == "sequential" {
			i := int(elapsed / 650)
			if i < len(targets) && elapsed%650 < 500 {
				scene["blue"] = []int{targets[i]}
			}
		} else {
			scene["blue"] = targets
		}
		scene["red"] = t.Secret["decoys"]
	}
	if t.Kind == "hold-fire" && encoding {
		delete(scene, "color")
		delete(scene, "shape")
	}
	if t.Kind == "sweet-spot" {
		scene["dials"] = g.dials
	}
	if t.Kind == "mystery" {
		attrs := trainerDecode[[]map[string]any](t.Public["attributes"])
		all := mysteryAll(attrs)
		scene["candidate_count"] = len(mysteryConsistent(all, g.move.Guesses, g.move.Counts))
	}
	if t.Kind == "fermi" && g.move.Hints > 0 {
		scene["hint"] = t.Secret["hint"]
	}
	if g.phase == "ANIMATE" {
		scene["shot"] = g.pending["shot"]
	}
	return scene
}
func (g *trainerGame) Snapshot(player string) (json.RawMessage, error) {
	if player != g.ctx.Players[0] {
		return nil, fmt.Errorf("unknown player")
	}
	g.Tick()
	remaining := int64(0)
	if g.started && g.cfg.DurationMS > 0 {
		remaining = max(int64(0), g.cfg.DurationMS-(g.ctx.ElapsedMS()-g.startedAt))
	}
	out := map[string]any{"profile": g.profile, "config": g.cfg, "phase": g.phase, "started": g.started, "finished": g.done, "elapsed_ms": g.simMS, "wall_elapsed_ms": max(int64(0), g.ctx.ElapsedMS()-g.startedAt), "time_remaining_ms": remaining, "level": g.level, "completed": len(g.history), "score": g.score, "bank_cents": g.bank, "comparable": g.comparable, "move_state": g.move, "feedback": g.feedback, "book": g.book, "scene": g.visibleScene(), "renderer_version": "trainer-renderer-v1"}
	if t := g.current(); t != nil {
		out["trial_id"], out["kind"], out["onset_ms"], out["response_onset_ms"] = t.ID, t.Kind, g.onset, g.responseOnset
		out["encode_ms"], out["mask_ms"], out["response_ms"] = t.EncodeMS, t.MaskMS, t.ResponseMS
		if t.Question != nil {
			out["question"] = assessmentQuestionPublic(*t.Question)
			if t.Question.ResponseType == "digits" {
				out["question"].(map[string]any)["response_type"] = "digits"
			}
		}
	}
	return canonicalAssessmentJSON(out)
}

type trainerPayload struct {
	TrialID    string   `json:"trial_id"`
	ElapsedMS  int64    `json:"elapsed_ms"`
	ReactionMS *float64 `json:"reaction_time_ms"`
	Phase      string   `json:"phase"`
	Answer     string   `json:"answer"`
	Yes        *bool    `json:"yes"`
	Side       string   `json:"side"`
	Choice     int      `json:"choice"`
	Cells      []int    `json:"cells"`
	Left       string   `json:"left"`
	Right      string   `json:"right"`
	Operator   string   `json:"operator"`
	From       int      `json:"from"`
	To         int      `json:"to"`
	Values     []string `json:"values"`
	Target     int      `json:"target"`
	Cannon     int      `json:"cannon"`
	X          float64  `json:"x"`
	Y          float64  `json:"y"`
	Bid        string   `json:"bid"`
	Ask        string   `json:"ask"`
	Size       int      `json:"size"`
	Reasoning  string   `json:"reasoning"`
}

func (g *trainerGame) Apply(req engine.ActionRequest, seq int64) (out engine.Output) {
	defer func() {
		g.events = append(g.events, trainerEvent{g.ctx.ElapsedMS(), seq, req, out.Accepted, out.Reason})
	}()
	reject := func(reason string) engine.Output { return engine.Output{Reason: reason} }
	if req.PlayerID != g.ctx.Players[0] {
		return reject(engine.ReasonUnknownPlayer)
	}
	var p trainerPayload
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return reject("INVALID_FORMAT")
		}
	}
	g.Tick()
	if g.done {
		return reject(engine.ReasonMarketClosed)
	}
	if req.Action == "BEGIN" {
		if g.started {
			return reject(engine.ReasonAlreadyActed)
		}
		g.started = true
		g.startedAt = g.ctx.ElapsedMS()
		g.simMS = 0
		g.nextTrial()
		return engine.Output{Accepted: true}
	}
	if !g.started {
		return reject("NOT_STARTED")
	}
	if req.Action == "FOCUS_LOST" || req.Action == "RESUME" {
		g.comparable = false
		g.interruptions = append(g.interruptions, map[string]any{"type": req.Action, "server_ms": g.ctx.ElapsedMS() - g.startedAt, "reported_ms": p.ElapsedMS})
		if req.Action == "RESUME" {
			g.advanceSim(max(g.simMS, g.ctx.ElapsedMS()-g.startedAt))
		}
		return engine.Output{Accepted: true}
	}
	wall := g.ctx.ElapsedMS() - g.startedAt
	if p.ElapsedMS < g.simMS || p.ElapsedMS > wall+2000 {
		return reject("INVALID_CLOCK")
	}
	g.advanceSim(p.ElapsedMS)
	if g.done && req.Action == "ADVANCE_TIME" {
		return engine.Output{Accepted: true, GameOver: true}
	}
	if g.done {
		return reject(engine.ReasonLate)
	}
	if req.Action == "END_SESSION" {
		g.finish("stopped")
		return engine.Output{Accepted: true, GameOver: true}
	}
	if req.Action == "ADVANCE_TIME" {
		return engine.Output{Accepted: true}
	}
	t := g.current()
	if t == nil || p.TrialID != t.ID {
		return reject("STALE_TRIAL")
	}
	if req.Action == "PRESENT" {
		if g.phase != "READY" {
			return reject("ALREADY_PRESENTED")
		}
		g.onset = g.simMS
		g.responseOnset = g.simMS
		t := g.current()
		g.phase = "RESPONSE"
		if t.EncodeMS > 0 {
			g.phase = "ENCODE"
			g.responseOnset = g.onset + t.EncodeMS + t.MaskMS
		}
		return engine.Output{Accepted: true}
	}
	if req.Action == "DISPLAY" {
		if p.Phase != g.phase {
			return reject("STALE_PHASE")
		}
		if g.phase == "RESPONSE" && g.simMS-g.responseOnset > 150 {
			g.comparable = false
			g.metrics["late_presentations"]++
		}
		g.metrics["presentation_events"]++
		return engine.Output{Accepted: true}
	}
	if req.Action == "NEXT" {
		if g.phase != "FEEDBACK" {
			return reject("WRONG_PHASE")
		}
		g.nextTrial()
		return engine.Output{Accepted: true, GameOver: g.done}
	}
	if g.phase != "RESPONSE" {
		return reject("WRONG_PHASE")
	}
	if p.ReactionMS != nil && (!finiteTrainer(*p.ReactionMS) || *p.ReactionMS < 0 || *p.ReactionMS > float64(g.simMS-g.responseOnset)+2000) {
		return reject("INVALID_CLOCK")
	}
	result := map[string]any{"correct": false, "status": "wrong", "points": -1, "elapsed_trial_ms": g.simMS - g.onset}
	if p.ReactionMS != nil {
		result["reaction_time_ms"] = *p.ReactionMS
	}
	switch req.Action {
	case "SKIP":
		if t.Kind == "target-number" && g.simMS-g.responseOnset < 10000 {
			return reject("SKIP_LOCKED")
		}
		if !slices.Contains([]string{"math", "sequence", "probability", "cube", "target-number"}, t.Kind) {
			return reject(engine.ReasonUnknownAction)
		}
		result["status"] = "skipped"
		result["points"] = 0
		if t.Kind == "target-number" {
			result["points"] = -1
		}
		g.completeTrial(result)
	case "HINT":
		if t.Kind != "fermi" {
			return reject(engine.ReasonUnknownAction)
		}
		g.move.Hints++
		return engine.Output{Accepted: true}
	case "SAVE_REASONING":
		if t.Kind != "fermi" || len(p.Reasoning) > 4000 {
			return reject("INVALID_FORMAT")
		}
		g.move.Reasoning = p.Reasoning
		return engine.Output{Accepted: true}
	case "ANSWER":
		if t.Question == nil {
			return reject(engine.ReasonUnknownAction)
		}
		correct := false
		answer := strings.TrimSpace(p.Answer)
		if t.Question.ResponseType == "digits" {
			if len(answer) != g.cfg.Places || strings.Trim(answer, "0123456789") != "" {
				return reject("INVALID_FORMAT")
			}
			correct = answer == t.Question.Answer
		} else if t.Kind == "fermi" {
			value, ok := parseExactNumber(answer)
			if !ok || value.Sign() <= 0 {
				return reject("INVALID_FORMAT")
			}
			v, _ := value.Float64()
			truth, _ := parseExactNumber(t.Question.Answer)
			y, _ := truth.Float64()
			factor := math.Max(v/y, y/v)
			if !finiteTrainer(factor) {
				return reject("INVALID_FORMAT")
			}
			correct = factor <= 10
			result["factor_error"] = factor
		} else {
			r, ok := normalizeAssessmentAnswer(*t.Question, assessmentResponse{Answer: answer})
			if !ok {
				return reject("INVALID_FORMAT")
			}
			correct = assessmentCorrect(*t.Question, r)
		}
		result["response"] = answer
		result["correct"] = correct
		if correct {
			result["status"] = "correct"
			result["points"] = 1
		} else if t.Kind == "probability" || g.profile.Category == "Trading" || t.Kind == "fermi" {
			result["points"] = 0
		} else if t.Kind == "fractions" {
			result["points"] = -2
		}
		g.applyCaseAnswer(correct, result)
		g.completeTrial(result)
	case "RESPOND":
		correct := false
		switch t.Kind {
		case "task-switch":
			if p.Yes == nil {
				return reject("INVALID_FORMAT")
			}
			correct = *p.Yes == t.Secret["yes"]
			result["response"] = *p.Yes
			result["switch"] = t.Secret["switch"]
		case "response-conflict":
			if p.Side != "LEFT" && p.Side != "RIGHT" {
				return reject("INVALID_FORMAT")
			}
			correct = p.Side == t.Secret["side"]
			result["response"] = p.Side
			result["congruent"] = t.Secret["congruent"]
		case "pincode":
			if len(p.Answer) != trainerInt(t.Public["length"]) || strings.Trim(p.Answer, "0123456789") != "" {
				return reject("INVALID_FORMAT")
			}
			correct = p.Answer == t.Secret["answer"]
			result["response"] = p.Answer
		case "code-match":
			if p.Choice < 0 || p.Choice > 3 {
				return reject("INVALID_FORMAT")
			}
			correct = p.Choice == trainerInt(t.Secret["answer"])
			result["choice"] = p.Choice
			if !correct {
				result["points"] = 0
			}
		default:
			return reject(engine.ReasonUnknownAction)
		}
		result["correct"] = correct
		if correct {
			result["status"] = "correct"
			result["points"] = 1
		}
		g.completeTrial(result)
	case "RECALL":
		if t.Kind != "grid-recall" {
			return reject(engine.ReasonUnknownAction)
		}
		seen := map[int]bool{}
		for _, v := range p.Cells {
			if v < 0 || v > 15 || seen[v] {
				return reject("INVALID_FORMAT")
			}
			seen[v] = true
		}
		targets := trainerDecode[[]int](t.Secret["targets"])
		decoys := trainerDecode[[]int](t.Secret["decoys"])
		hits, extras, red := 0, 0, 0
		for _, v := range p.Cells {
			if slices.Contains(targets, v) {
				hits++
			} else {
				extras++
			}
			if slices.Contains(decoys, v) {
				red++
			}
		}
		correct := hits == len(targets) && extras == 0
		result["correct"], result["hits"], result["missed_cells"], result["extras"], result["decoy_clicks"] = correct, hits, len(targets)-hits, extras, red
		result["points"] = 0
		if correct {
			result["points"] = 1
			result["status"] = "correct"
		}
		g.completeTrial(result)
	case "PRESS":
		if t.Kind != "hold-fire" {
			return reject(engine.ReasonUnknownAction)
		}
		target := t.Secret["target"].(bool)
		result["correct"] = target
		result["points"] = 0
		result["status"] = "false_alarm"
		if target {
			result["status"] = "hit"
		}
		g.completeTrial(result)
	case "FIRE":
		if t.Kind != "intercept" || p.Cannon < 0 || p.Cannon > 3 {
			return reject("INVALID_FORMAT")
		}
		crossings := trainerDecode[[]map[string]float64](t.Public["crossings"])
		cross := crossings[p.Cannon]
		delay := float64(g.simMS-g.onset) / 1000
		err := (delay + cross["travel_s"] - cross["ball_s"]) * 1000
		correct := math.Abs(err) <= trainerFloat(t.Public["tolerance_ms"])
		result["correct"], result["arrival_error_ms"] = correct, err
		result["points"] = 0
		result["status"] = "miss"
		if correct {
			result["status"] = "hit"
			result["points"] = 1
		}
		result["shot"] = map[string]any{"cannon": p.Cannon, "fire_ms": g.simMS, "travel_ms": cross["travel_s"] * 1000, "x": cross["x"], "y": cross["y"]}
		g.pending = result
		g.phase = "ANIMATE"
		g.feedbackUntil = g.simMS + int64(cross["travel_s"]*1000)
	case "COMBINE", "UNDO", "RESET", "MOVE", "GUESS", "PUMP", "CASH_IN", "COMMIT_TARGET", "CLICK", "QUOTE":
		reason := g.specialAction(req.Action, p, result)
		if reason != "" {
			return reject(reason)
		}
	default:
		return reject(engine.ReasonUnknownAction)
	}
	return engine.Output{Accepted: true, GameOver: g.done, RoundReward: trainerFloat(result["points"])}
}
func finiteTrainer(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (g *trainerGame) Results() (json.RawMessage, error) {
	g.Tick()
	if !g.done {
		return nil, engine.ErrNotFinished
	}
	correct, wrong := g.metrics["correct"], g.metrics["wrong"]
	accuracy := 0.0
	if correct+wrong > 0 {
		accuracy = correct / (correct + wrong)
	}
	rate := any(nil)
	if g.cfg.DurationMS > 0 && (g.profile.ScoreKind == "net_rate" || g.profile.ScoreKind == "dial_net") {
		rate = g.score / (float64(g.cfg.DurationMS) / 60000)
	}
	result := map[string]any{"finished": true, "runtime_version": trainerVersion, "profile": g.profile, "config": g.cfg, "score_kind": g.profile.ScoreKind, "raw_score": g.score, "net_per_minute": rate, "accuracy": accuracy, "completed": g.metrics["completed"], "correct": correct, "wrong": wrong, "skipped": g.metrics["skipped"], "timeouts": g.metrics["timeouts"], "elapsed_ms": g.simMS, "metrics": g.metrics, "review": g.history, "bank_cents": g.bank, "book": g.book, "comparable": g.comparable, "interruptions": g.interruptions, "timing_origin": "client presentation/input clock; bounded and replayed, not independently authenticated", "rules_origin": "original practice choices where public source is silent"}
	if g.cfg.TrainerID == "hold-fire" {
		t, n := g.metrics["targets"], g.metrics["non_targets"]
		var balanced any
		if t > 0 && n > 0 {
			balanced = .5 * (g.metrics["hit"]/t + g.metrics["correct_rejection"]/n)
		}
		result["balanced_accuracy"] = balanced
		result["raw_score"] = balanced
	}
	if g.cfg.TrainerID == "probability" && g.simMS > 0 {
		result["correct_per_minute"] = correct / (float64(g.simMS) / 60000)
	}
	if g.cfg.TrainerID == "pump" {
		result["raw_score"] = g.bank
		result["adjusted_pumps"] = nil
		if g.metrics["cashed"] > 0 {
			result["adjusted_pumps"] = g.metrics["cashed_pumps"] / g.metrics["cashed"]
		}
	}
	if g.cfg.TrainerID == "mystery" || g.cfg.TrainerID == "skyline" {
		result["net_per_minute"] = nil
	}
	if g.cfg.TrainerID == "sweet-spot" {
		result["correct"] = g.metrics["hits"]
		result["completed"] = g.metrics["hits"] + g.metrics["expiries"]
		result["accuracy"] = 0.0
		if g.metrics["clicks"] > 0 {
			result["accuracy"] = g.metrics["hits"] / g.metrics["clicks"]
		}
		result["mean_hit_angular_error"] = nil
		hits := g.metrics["hits"]
		if hits > 0 {
			result["mean_hit_angular_error"] = g.metrics["angular_error_sum"] / hits
		}
		resolved := hits + g.metrics["expiries"]
		result["hit_rate"] = 0.0
		if resolved > 0 {
			result["hit_rate"] = hits / resolved
		}
	}
	for _, kind := range []string{"switch", "congruent"} {
		groups := map[bool][]float64{true: {}, false: {}}
		for _, row := range g.history {
			if row["correct"] == true {
				if value, ok := row["reaction_time_ms"]; ok {
					if group, ok := row[kind].(bool); ok {
						groups[group] = append(groups[group], trainerFloat(value))
					}
				}
			}
		}
		a, b := trainerMedian(groups[true]), trainerMedian(groups[false])
		result[kind+"_median_true_ms"], result[kind+"_median_false_ms"] = a, b
		if a != nil && b != nil {
			if kind == "congruent" {
				result["conflict_cost_ms"] = *b - *a
			}
			result[kind+"_difference_ms"] = *a - *b
		} else {
			result[kind+"_difference_ms"] = nil
		}
	}
	return canonicalAssessmentJSON(result)
}
func trainerMedian(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	n := len(values)
	v := values[n/2]
	if n%2 == 0 {
		v = (values[n/2-1] + v) / 2
	}
	return &v
}

type trainerTape struct {
	RuntimeVersion  string          `json:"runtime_version"`
	BankVersion     string          `json:"bank_version"`
	RendererVersion string          `json:"renderer_version"`
	PRNG            string          `json:"prng"`
	Seed            uint64          `json:"seed"`
	Player          string          `json:"player"`
	Profile         TrainerProfile  `json:"profile"`
	Config          trainerConfig   `json:"config"`
	Trials          []trainerTrial  `json:"trials"`
	TrialHash       string          `json:"trial_sha256"`
	Events          []trainerEvent  `json:"events"`
	FinalClockMS    int64           `json:"final_clock_ms"`
	Results         json.RawMessage `json:"results"`
}

func (g *trainerGame) Tape() (json.RawMessage, error) {
	results, err := g.Results()
	if err != nil {
		return nil, err
	}
	hash := trainerTrialHash(g.trials)
	return canonicalAssessmentJSON(trainerTape{trainerVersion, trainerBankVersion, "trainer-renderer-v1", "splitmix64-v1", g.ctx.Seed, g.ctx.Players[0], g.profile, g.cfg, g.trials, hash, g.events, g.ctx.ElapsedMS(), results})
}
func trainerTrialHash(trials []trainerTrial) string {
	questions := make([]assessmentQuestion, len(trials))
	for i, t := range trials {
		questions[i] = assessmentQuestion{ID: t.ID, Model: t}
	}
	return assessmentQuestionHash(questions)
}
func ReplayTrainer(raw json.RawMessage) (output json.RawMessage, replayErr error) {
	defer func() {
		if recover() != nil {
			output = nil
			replayErr = fmt.Errorf("malformed trainer replay")
		}
	}()
	var tape trainerTape
	if err := json.Unmarshal(raw, &tape); err != nil {
		return nil, err
	}
	if tape.RuntimeVersion != trainerVersion || tape.BankVersion != trainerBankVersion || tape.RendererVersion != "trainer-renderer-v1" || tape.PRNG != "splitmix64-v1" {
		return nil, fmt.Errorf("unsupported trainer replay version")
	}
	if len(tape.Trials) > 5000 || len(tape.Events) > 100000 || tape.FinalClockMS < 0 || tape.FinalClockMS > 86400000 {
		return nil, fmt.Errorf("invalid trainer replay limits")
	}
	if tape.TrialHash != trainerTrialHash(tape.Trials) {
		return nil, fmt.Errorf("scenario hash mismatch")
	}
	if _, ok := trainerProfile(tape.Config.TrainerID); !ok {
		return nil, fmt.Errorf("unknown trainer")
	}
	if tape.Config.DurationMS < 0 || tape.Config.DurationMS > 3600000 || tape.Config.Rounds < 0 || tape.Config.Rounds > 128 {
		return nil, fmt.Errorf("invalid trainer replay config")
	}
	validated, profile, err := resolveTrainerConfig(engine.MustJSON(tape.Config))
	if err != nil || !sameJSON(validated, tape.Config) || !sameJSON(profile, tape.Profile) {
		return nil, fmt.Errorf("invalid replay preset")
	}
	for i, t := range tape.Trials {
		if t.ID != fmt.Sprintf("trial-%03d", i+1) || t.Index != i || t.Kind != tape.Config.TrainerID || t.Level < 1 || t.Level > 18 || t.EncodeMS < 0 || t.EncodeMS > 60000 || t.MaskMS < 0 || t.MaskMS > 10000 || t.ResponseMS < 0 || t.ResponseMS > 6000000 || t.Public == nil || t.Secret == nil {
			return nil, fmt.Errorf("invalid trial manifest")
		}
		if t.Kind == "mystery" {
			attrs := trainerDecode[[]map[string]any](t.Public["attributes"])
			if len(attrs) < 2 || len(attrs) > 6 {
				return nil, fmt.Errorf("invalid attribute count")
			}
			for _, a := range attrs {
				values := trainerDecode[[]string](a["values"])
				if len(values) < 2 || len(values) > 4 {
					return nil, fmt.Errorf("invalid attribute domain")
				}
			}
		}
	}
	clock := int64(0)
	ctx := &engine.Ctx{Players: []string{tape.Player}, Seed: tape.Seed, Start: time.Unix(0, 0), NowFn: func() time.Time { return time.Unix(0, clock*int64(time.Millisecond)) }}
	g := trainerFromConfig(ctx, tape.Config, tape.Profile, tape.Trials)
	for i, event := range tape.Events {
		if event.AtMS < clock || event.AtMS > 86400000 {
			return nil, fmt.Errorf("invalid event time")
		}
		clock = event.AtMS
		out := g.Apply(event.Request, event.Sequence)
		if out.Accepted != event.Accepted || out.Reason != event.Reason {
			return nil, fmt.Errorf("receipt diverged at event %d: %s", i, out.Reason)
		}
	}
	if tape.FinalClockMS < clock {
		return nil, fmt.Errorf("final clock precedes actions")
	}
	clock = tape.FinalClockMS
	g.Tick()
	if g.failure != "" {
		return nil, fmt.Errorf("%s", g.failure)
	}
	result, err := g.Results()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(result, tape.Results) {
		return nil, fmt.Errorf("trainer replay results differ")
	}
	return result, nil
}

// NextWake schedules only the authoritative session deadline. Item presentation
// advances through timestamped browser actions; untimed puzzles need no alarm.
func (g *trainerGame) NextWake() time.Time {
	if !g.started || g.done || g.cfg.DurationMS <= 0 {
		return time.Time{}
	}
	return g.ctx.Start.Add(time.Duration(g.startedAt+g.cfg.DurationMS) * time.Millisecond)
}
