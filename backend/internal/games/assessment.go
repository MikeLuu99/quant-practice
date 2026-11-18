package games

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"quantgame/backend/internal/engine"
)

const assessmentVersion = "assessment-runtime-v1"

type assessmentConfig struct {
	ProfileID     string `json:"profile_id"`
	Mode          string `json:"mode"`
	QuestionCount int    `json:"question_count"`
	DurationMS    int64  `json:"duration_ms"`
	ItemTimeMS    int64  `json:"item_time_ms"`
	Navigation    string `json:"navigation"`
	ResponseMode  string `json:"response_mode"`
	Family        string `json:"family,omitempty"`
	WrongPoints   int    `json:"wrong_points"`
}
type assessmentResponse struct {
	Answer    string `json:"answer,omitempty"`
	Low       string `json:"low,omitempty"`
	High      string `json:"high,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}
type assessmentEvent struct {
	AtMS     int64                `json:"at_ms"`
	Sequence int64                `json:"sequence"`
	Request  engine.ActionRequest `json:"request"`
	Accepted bool                 `json:"accepted"`
	Reason   string               `json:"reason,omitempty"`
}
type assessmentTransition struct {
	AtMS  int64  `json:"at_ms"`
	Type  string `json:"type"`
	Index int    `json:"index"`
}
type assessmentTape struct {
	RuntimeVersion string                 `json:"runtime_version"`
	BankVersion    string                 `json:"bank_version"`
	PRNG           string                 `json:"prng"`
	Seed           string                 `json:"seed"`
	Player         string                 `json:"player"`
	Profile        AssessmentProfile      `json:"profile"`
	Config         assessmentConfig       `json:"config"`
	Questions      []assessmentQuestion   `json:"questions"`
	QuestionHash   string                 `json:"question_sha256"`
	Events         []assessmentEvent      `json:"events"`
	Transitions    []assessmentTransition `json:"transitions"`
	FinalClockMS   int64                  `json:"final_clock_ms"`
	Results        json.RawMessage        `json:"results"`
}
type assessmentGame struct {
	ctx         *engine.Ctx
	cfg         assessmentConfig
	profile     AssessmentProfile
	questions   []assessmentQuestion
	responses   map[string]assessmentResponse
	savedAt     map[string]int64
	committed   map[string]bool
	viewedAt    []int64
	spentMS     []int64
	index       int
	enteredAt   int64
	itemStart   int64
	finishedAt  int64
	done        bool
	events      []assessmentEvent
	transitions []assessmentTransition
}

func resolveAssessmentConfig(raw json.RawMessage) (assessmentConfig, AssessmentProfile, error) {
	var cfg assessmentConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, AssessmentProfile{}, fmt.Errorf("invalid assessment config: %w", err)
	}
	p, ok := assessmentProfile(cfg.ProfileID)
	if !ok {
		return cfg, p, fmt.Errorf("unknown assessment profile %q", cfg.ProfileID)
	}
	if cfg.Mode == "" {
		cfg.Mode = "preview"
	}
	if cfg.Mode != "preview" && cfg.Mode != "full" && cfg.Mode != "custom" {
		return cfg, p, fmt.Errorf("mode must be preview, full or custom")
	}
	if cfg.Mode != "custom" && (cfg.QuestionCount != 0 || cfg.DurationMS != 0 || cfg.ItemTimeMS != 0 || cfg.Navigation != "" || cfg.ResponseMode != "" || cfg.Family != "" || cfg.WrongPoints != 0) {
		return cfg, p, fmt.Errorf("overrides require custom mode")
	}
	count, duration := p.QuestionCount, p.DurationMS
	if cfg.Mode == "preview" {
		count, duration = p.PreviewCount, p.PreviewDurationMS
	}
	if cfg.QuestionCount == 0 {
		cfg.QuestionCount = count
	}
	if cfg.DurationMS == 0 {
		cfg.DurationMS = duration
	}
	if cfg.ItemTimeMS == 0 {
		cfg.ItemTimeMS = p.ItemTimeMS
	}
	if cfg.Navigation == "" {
		cfg.Navigation = p.Navigation
	}
	if cfg.ResponseMode == "" {
		cfg.ResponseMode = p.ResponseMode
	}
	if cfg.QuestionCount < 1 || cfg.QuestionCount > 300 {
		return cfg, p, fmt.Errorf("question_count must be 1..300")
	}
	if cfg.Navigation != "forward_only" && cfg.Navigation != "revisable" {
		return cfg, p, fmt.Errorf("invalid navigation")
	}
	if !slices.Contains([]string{"single_choice", "numeric", "token", "mixed", "interval"}, cfg.ResponseMode) {
		return cfg, p, fmt.Errorf("invalid response_mode")
	}
	if (p.ResponseMode == "interval") != (cfg.ResponseMode == "interval") {
		return cfg, p, fmt.Errorf("interval controls are required only for interval profiles")
	}
	if cfg.ResponseMode == "token" && !slices.Contains(p.Engines, "sequences") {
		return cfg, p, fmt.Errorf("token mode is only available for sequences")
	}
	if cfg.WrongPoints > 0 || cfg.WrongPoints < -5 {
		return cfg, p, fmt.Errorf("wrong_points must be between -5 and 0")
	}
	if cfg.Family != "" && !slices.Contains(p.Families, cfg.Family) {
		return cfg, p, fmt.Errorf("family must be one of the profile's declared families")
	}
	if p.TimerMode == "per_question" {
		if cfg.Navigation != "forward_only" {
			return cfg, p, fmt.Errorf("per-question timing requires forward_only navigation")
		}
		if cfg.ItemTimeMS < 1000 || cfg.ItemTimeMS > 600000 {
			return cfg, p, fmt.Errorf("item_time_ms must be 1000..600000")
		}
		cfg.DurationMS = int64(cfg.QuestionCount) * cfg.ItemTimeMS
	} else if cfg.DurationMS < 1000 || cfg.DurationMS > 7200000 {
		return cfg, p, fmt.Errorf("duration_ms must be 1000..7200000")
	}
	return cfg, p, nil
}
func newAssessment(raw json.RawMessage, ctx *engine.Ctx) (engine.Game, error) {
	if len(ctx.Players) != 1 {
		return nil, fmt.Errorf("assessments are individual attempts: supply one player")
	}
	cfg, p, err := resolveAssessmentConfig(raw)
	if err != nil {
		return nil, err
	}
	rng := engine.NewRNG(ctx.Seed + 0xA55E55)
	questions := make([]assessmentQuestion, cfg.QuestionCount)
	// A shuffled balanced schedule guarantees every declared family appears in
	// a sufficiently long run. Repeated family entries are explicit weights.
	schedule := append([]string(nil), p.Families...)
	rng.Shuffle(len(schedule), func(i, j int) { schedule[i], schedule[j] = schedule[j], schedule[i] })
	for i := range questions {
		family := schedule[i%len(schedule)]
		if cfg.Family != "" {
			family = cfg.Family
		}
		questions[i] = assessmentTask(rng, family, i, cfg.ResponseMode)
		questions[i].ItemTimeMS = cfg.ItemTimeMS
	}
	return assessmentFromQuestions(ctx, cfg, p, questions), nil
}
func assessmentFromQuestions(ctx *engine.Ctx, cfg assessmentConfig, p AssessmentProfile, questions []assessmentQuestion) *assessmentGame {
	g := &assessmentGame{ctx: ctx, cfg: cfg, profile: p, questions: questions, responses: map[string]assessmentResponse{}, savedAt: map[string]int64{}, committed: map[string]bool{}, viewedAt: make([]int64, len(questions)), spentMS: make([]int64, len(questions)), events: []assessmentEvent{}, transitions: []assessmentTransition{}}
	g.viewedAt[0] = 0
	return g
}
func (g *assessmentGame) Meta() engine.GameMeta {
	return engine.GameMeta{Slug: "assessment", Name: g.profile.Company + " · " + g.profile.Title, MinPlayers: 1, MaxPlayers: 1, Access: "free", DurationMS: g.cfg.DurationMS, Description: "Original practice questions in a versioned company format.", Rules: "Save answers before the server deadline. Review and solutions appear only after finalization."}
}
func (g *assessmentGame) Tick() { g.tick() }
func (g *assessmentGame) tick() {
	if g.done {
		return
	}
	now := g.ctx.ElapsedMS()
	if g.profile.TimerMode == "global" {
		if now >= g.cfg.DurationMS {
			g.finish(g.cfg.DurationMS, "deadline")
		}
		return
	}
	for !g.done {
		deadline := g.itemStart + g.questions[g.index].ItemTimeMS
		if now < deadline {
			break
		}
		g.advance(deadline, "item_timeout")
	}
}
func (g *assessmentGame) finish(at int64, reason string) {
	if g.done {
		return
	}
	g.spentMS[g.index] += max(int64(0), at-g.enteredAt)
	g.done = true
	g.finishedAt = at
	g.transitions = append(g.transitions, assessmentTransition{at, reason, g.index})
}
func (g *assessmentGame) advance(at int64, reason string) {
	g.committed[g.questions[g.index].ID] = true
	if g.index+1 == len(g.questions) {
		g.finish(at, reason)
		return
	}
	g.spentMS[g.index] += max(int64(0), at-g.enteredAt)
	g.transitions = append(g.transitions, assessmentTransition{at, reason, g.index})
	g.index++
	g.enteredAt = at
	g.itemStart = at
	g.viewedAt[g.index] = at
}
func (g *assessmentGame) navigate(index int, now int64) {
	g.spentMS[g.index] += max(int64(0), now-g.enteredAt)
	g.index = index
	g.enteredAt = now
	g.viewedAt[g.index] = now
}
func (g *assessmentGame) Snapshot(player string) (json.RawMessage, error) {
	if player != g.ctx.Players[0] {
		return nil, fmt.Errorf("unknown player %q", player)
	}
	g.tick()
	remaining := g.cfg.DurationMS - g.ctx.ElapsedMS()
	itemDeadline := int64(0)
	if g.profile.TimerMode == "per_question" {
		itemDeadline = g.itemStart + g.questions[g.index].ItemTimeMS
		remaining = itemDeadline - g.ctx.ElapsedMS()
	}
	if g.done {
		remaining = 0
	}
	saved := make([]string, 0, len(g.responses))
	locked := make([]string, 0, len(g.committed))
	for _, q := range g.questions {
		if _, ok := g.responses[q.ID]; ok {
			saved = append(saved, q.ID)
		}
		if g.committed[q.ID] {
			locked = append(locked, q.ID)
		}
	}
	snap := map[string]any{"phase": "ASSESSMENT", "finished": g.done, "profile": g.profile, "config": g.cfg, "index": g.index, "question_number": g.index + 1, "question_count": len(g.questions), "time_remaining_ms": max(int64(0), remaining), "timer_mode": g.profile.TimerMode, "item_deadline_ms": itemDeadline, "saved_question_ids": saved, "committed_question_ids": locked, "navigation": g.cfg.Navigation, "saved_response": g.responses[g.questions[g.index].ID]}
	if g.done {
		snap["phase"] = "DONE"
	} else {
		snap["question"] = assessmentQuestionPublic(g.questions[g.index])
	}
	return engine.MustJSON(snap), nil
}
func (g *assessmentGame) Apply(req engine.ActionRequest, seq int64) (out engine.Output) {
	now := g.ctx.ElapsedMS()
	oldIndex := g.index
	g.tick()
	defer func() {
		g.events = append(g.events, assessmentEvent{AtMS: now, Sequence: seq, Request: req, Accepted: out.Accepted, Reason: out.Reason})
	}()
	reject := func(reason string) engine.Output { return engine.Output{Reason: reason} }
	if req.PlayerID != g.ctx.Players[0] {
		return reject(engine.ReasonUnknownPlayer)
	}
	if g.done {
		if now >= g.cfg.DurationMS && g.profile.TimerMode == "global" {
			return reject(engine.ReasonLate)
		}
		return reject(engine.ReasonMarketClosed)
	}
	var payload struct {
		QuestionID string             `json:"question_id"`
		Response   assessmentResponse `json:"response"`
		Index      int                `json:"index"`
	}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			return reject("INVALID_FORMAT")
		}
	}
	current := g.questions[g.index]
	if req.Action != "SUBMIT_TEST" && payload.QuestionID != current.ID {
		if oldIndex != g.index {
			return reject(engine.ReasonLate)
		}
		return reject("STALE_QUESTION")
	}
	switch req.Action {
	case "SAVE_ANSWER":
		response, ok := normalizeAssessmentAnswer(current, payload.Response)
		if !ok {
			return reject("INVALID_FORMAT")
		}
		g.responses[current.ID] = response
		g.savedAt[current.ID] = now
	case "CLEAR_ANSWER":
		delete(g.responses, current.ID)
		delete(g.savedAt, current.ID)
	case "NEXT":
		if g.cfg.Navigation == "forward_only" {
			g.advance(now, "committed")
		} else if g.index+1 < len(g.questions) {
			g.navigate(g.index+1, now)
		} else {
			return reject("NO_NEXT_QUESTION")
		}
	case "PREVIOUS":
		if g.cfg.Navigation != "revisable" {
			return reject("NAVIGATION_LOCKED")
		}
		if g.index == 0 {
			return reject("NO_PREVIOUS_QUESTION")
		}
		g.navigate(g.index-1, now)
	case "JUMP_TO":
		if g.cfg.Navigation != "revisable" {
			return reject("NAVIGATION_LOCKED")
		}
		if payload.Index < 0 || payload.Index >= len(g.questions) {
			return reject(engine.ReasonInvalidValue)
		}
		g.navigate(payload.Index, now)
	case "SUBMIT_TEST":
		g.finish(now, "submitted")
	default:
		return reject(engine.ReasonUnknownAction)
	}
	// No correctness, reward, hidden value or answer key in action receipts.
	return engine.Output{Accepted: true, GameOver: g.done, PrivateChanges: engine.MustJSON(map[string]any{"saved": true, "index": g.index})}
}
func (g *assessmentGame) Finished() bool { g.tick(); return g.done }

func (g *assessmentGame) Results() (json.RawMessage, error) {
	g.tick()
	if !g.done {
		return nil, engine.ErrNotFinished
	}
	correct, wrong, blank, covered, intervals := 0, 0, 0, 0, 0
	intervalSum := 0.0
	review := make([]map[string]any, 0, len(g.questions))
	topics := map[string]map[string]int{}
	for i, q := range g.questions {
		response, present := g.responses[q.ID]
		status := "blank"
		points := 0
		if topics[q.Family] == nil {
			topics[q.Family] = map[string]int{"total": 0, "correct": 0, "wrong": 0, "blank": 0}
		}
		topic := topics[q.Family]
		topic["total"]++
		row := map[string]any{"question_id": q.ID, "question": assessmentQuestionPublic(q), "response": response, "correct_answer": assessmentAnswerText(q), "explanation": q.Explanation, "time_spent_ms": g.spentMS[i], "absolute_tolerance": q.AbsTolerance, "relative_tolerance": q.RelTolerance}
		if !present {
			blank++
			topic["blank"]++
		} else if q.ResponseType == "interval" {
			low, _ := parseExactNumber(response.Low)
			high, _ := parseExactNumber(response.High)
			truth, _ := parseExactNumber(q.Answer)
			l, _ := low.Float64()
			u, _ := high.Float64()
			y, _ := truth.Float64()
			loss := u - l
			isCovered := low.Cmp(truth) <= 0 && high.Cmp(truth) >= 0
			if y < l {
				loss += 10 * (l - y)
			}
			if y > u {
				loss += 10 * (y - u)
			}
			intervals++
			intervalSum += loss
			status = "missed"
			if isCovered {
				covered++
				status = "covered"
			}
			row["covered"], row["interval_width"], row["interval_loss"] = isCovered, u-l, loss
		} else if assessmentCorrect(q, response) {
			correct++
			topic["correct"]++
			status = "correct"
			points = 1
		} else {
			wrong++
			topic["wrong"]++
			status = "wrong"
			points = g.cfg.WrongPoints
		}
		row["status"], row["points"] = status, points
		if at, ok := g.savedAt[q.ID]; ok {
			row["last_saved_ms"] = at
		}
		review = append(review, row)
	}
	attempted := len(g.responses)
	accuracy := 0.0
	if correct+wrong > 0 {
		accuracy = float64(correct) / float64(correct+wrong)
	}
	result := map[string]any{"finished": true, "runtime_version": assessmentVersion, "assessment_id": g.profile.ID, "profile": g.profile, "config": g.cfg, "correct": correct, "wrong": wrong, "blank": blank, "attempted": attempted, "accuracy_attempted": accuracy, "completion_rate": float64(attempted) / float64(len(g.questions)), "elapsed_ms": g.finishedAt, "score_rule_origin": g.profile.ScoreOrigin, "score_kind": "accuracy", "raw_score": correct + wrong*g.cfg.WrongPoints, "maximum_score": len(g.questions), "review": review, "topics": topics}
	if g.cfg.Mode == "custom" {
		result["score_rule_origin"] = "configured original"
	}
	if g.profile.ResponseMode == "interval" {
		coverage, loss := 0.0, 0.0
		if intervals > 0 {
			coverage = float64(covered) / float64(intervals)
			loss = intervalSum / float64(intervals)
		}
		result["score_kind"] = "interval_loss"
		result["raw_score"] = nil
		result["maximum_score"] = nil
		result["accuracy_attempted"] = nil
		result["covered"], result["intervals_answered"], result["coverage_rate"] = covered, intervals, coverage
		result["mean_interval_loss"] = nil
		if intervals > 0 {
			result["mean_interval_loss"] = loss
		}
		result["reasoning_grade"] = "human review only"
	}
	return canonicalAssessmentJSON(result)
}
func assessmentQuestionHash(questions []assessmentQuestion) string {
	raw, _ := canonicalAssessmentJSON(questions)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (g *assessmentGame) Tape() (json.RawMessage, error) {
	g.tick()
	if !g.done {
		return nil, engine.ErrNotFinished
	}
	results, err := g.Results()
	if err != nil {
		return nil, err
	}
	last := g.finishedAt
	if len(g.events) > 0 {
		last = max(last, g.events[len(g.events)-1].AtMS)
	}
	return canonicalAssessmentJSON(assessmentTape{RuntimeVersion: assessmentVersion, BankVersion: assessmentBankVersion, PRNG: "splitmix64-v1", Seed: fmt.Sprint(g.ctx.Seed), Player: g.ctx.Players[0], Profile: g.profile, Config: g.cfg, Questions: g.questions, QuestionHash: assessmentQuestionHash(g.questions), Events: g.events, Transitions: g.transitions, FinalClockMS: last, Results: results})
}

// ReplayAssessment consumes the frozen questions and relative receive times;
// it does not regenerate a possibly different bank from only a random seed.
func ReplayAssessment(raw json.RawMessage) (json.RawMessage, error) {
	var tape assessmentTape
	if err := json.Unmarshal(raw, &tape); err != nil {
		return nil, err
	}
	if tape.RuntimeVersion != assessmentVersion || tape.BankVersion != assessmentBankVersion || tape.PRNG != "splitmix64-v1" {
		return nil, fmt.Errorf("unsupported replay version")
	}
	if len(tape.Questions) < 1 || len(tape.Questions) > 300 || tape.Config.QuestionCount != len(tape.Questions) || tape.Player == "" {
		return nil, fmt.Errorf("invalid replay manifest")
	}
	if tape.QuestionHash != assessmentQuestionHash(tape.Questions) {
		return nil, fmt.Errorf("question tape hash mismatch")
	}
	if tape.Profile.TimerMode != "global" && tape.Profile.TimerMode != "per_question" {
		return nil, fmt.Errorf("invalid replay timer mode")
	}
	if tape.Config.DurationMS < 1000 || tape.Config.DurationMS > 180000000 || len(tape.Events) > 100000 {
		return nil, fmt.Errorf("invalid replay limits")
	}
	ids := map[string]bool{}
	for _, q := range tape.Questions {
		if ids[q.ID] || q.ID == "" {
			return nil, fmt.Errorf("duplicate or missing question ID")
		}
		ids[q.ID] = true
		if !slices.Contains([]string{"numeric", "token", "single_choice", "interval"}, q.ResponseType) {
			return nil, fmt.Errorf("invalid replay response type")
		}
		if tape.Profile.TimerMode == "per_question" && (q.ItemTimeMS < 1000 || q.ItemTimeMS > 600000) {
			return nil, fmt.Errorf("invalid replay item deadline")
		}
		if q.ResponseType == "numeric" || q.ResponseType == "interval" {
			if _, ok := parseExactNumber(q.Answer); !ok {
				return nil, fmt.Errorf("invalid replay answer")
			}
		}
		for _, tolerance := range []string{q.AbsTolerance, q.RelTolerance} {
			v, ok := parseExactNumber(tolerance)
			if !ok || v.Sign() < 0 {
				return nil, fmt.Errorf("invalid replay tolerance")
			}
		}
	}
	clock := int64(0)
	ctx := &engine.Ctx{Players: []string{tape.Player}, Start: time.Unix(0, 0), NowFn: func() time.Time { return time.Unix(0, clock*int64(time.Millisecond)) }}
	g := assessmentFromQuestions(ctx, tape.Config, tape.Profile, tape.Questions)
	for i, event := range tape.Events {
		if event.AtMS < clock || event.AtMS > 180000000 {
			return nil, fmt.Errorf("invalid event time at %d", i)
		}
		clock = event.AtMS
		out := g.Apply(event.Request, event.Sequence)
		if out.Accepted != event.Accepted || out.Reason != event.Reason {
			return nil, fmt.Errorf("receipt diverged at event %d", i)
		}
	}
	if tape.FinalClockMS < clock || tape.FinalClockMS > 180000000 {
		return nil, fmt.Errorf("invalid final replay time")
	}
	clock = tape.FinalClockMS
	g.tick()
	results, err := g.Results()
	if err != nil {
		return nil, fmt.Errorf("replay did not finish: %w", err)
	}
	transitions, _ := json.Marshal(g.transitions)
	recorded, _ := json.Marshal(tape.Transitions)
	if !bytes.Equal(transitions, recorded) || !bytes.Equal(results, tape.Results) {
		return nil, fmt.Errorf("replay result or transitions differ from recorded output")
	}
	return results, nil
}

// Normalize nested typed scenes and decoded maps to the same key order while
// preserving JSON number spellings. SHA256 and replay compare canonical bytes.
func canonicalAssessmentJSON(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}
