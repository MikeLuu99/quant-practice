package games

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"quantgame/backend/internal/engine"
)

type cubeVector [3]int

func cubeNeg(v cubeVector) cubeVector { return cubeVector{-v[0], -v[1], -v[2]} }
func cubeCross(a, b cubeVector) cubeVector {
	return cubeVector{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

type cubeFace struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Label string `json:"label"`
}
type cubeFrame struct{ U, V, N cubeVector }
type cubeCandidate struct {
	ID    string `json:"id"`
	Right string `json:"right"`
	Top   string `json:"top"`
	Front string `json:"front"`
}

// BFS folds the versioned 2D net. U points right on the paper, V down.
func foldCube(net []cubeFace) map[string]cubeFrame {
	frames := map[[2]int]cubeFrame{{net[0].X, net[0].Y}: {U: cubeVector{1, 0, 0}, V: cubeVector{0, 1, 0}, N: cubeVector{0, 0, 1}}}
	cells := map[[2]int]string{}
	for _, f := range net {
		cells[[2]int{f.X, f.Y}] = f.Label
	}
	queue := [][2]int{{net[0].X, net[0].Y}}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		a := frames[pos]
		next := []cubeFrame{{U: cubeNeg(a.N), V: a.V, N: a.U}, {U: a.N, V: a.V, N: cubeNeg(a.U)}, {U: a.U, V: cubeNeg(a.N), N: a.V}, {U: a.U, V: a.N, N: cubeNeg(a.V)}}
		for i, d := range dirs {
			p := [2]int{pos[0] + d[0], pos[1] + d[1]}
			if _, ok := cells[p]; !ok {
				continue
			}
			if _, ok := frames[p]; ok {
				continue
			}
			frames[p] = next[i]
			queue = append(queue, p)
		}
	}
	out := map[string]cubeFrame{}
	for p, label := range cells {
		out[label] = frames[p]
	}
	return out
}
func validCubeCandidate(frames map[string]cubeFrame, c cubeCandidate) bool {
	r, rok := frames[c.Right]
	t, tok := frames[c.Top]
	f, fok := frames[c.Front]
	return rok && tok && fok && c.Right != c.Top && c.Right != c.Front && c.Top != c.Front && cubeCross(r.N, t.N) == f.N
}
func cubeTask(rng *engine.RNG) assessmentQuestion {
	coords := [][2]int{{1, 0}, {0, 1}, {1, 1}, {2, 1}, {1, 2}, {1, 3}}
	labels := []string{"1", "2", "3", "4", "5", "6"}
	rng.Shuffle(6, func(i, j int) { labels[i], labels[j] = labels[j], labels[i] })
	turns := rng.Int(4)
	net := []cubeFace{}
	minX, minY := 0, 0
	for i, p := range coords {
		for range turns {
			p = [2]int{-p[1], p[0]}
		}
		if p[0] < minX {
			minX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		net = append(net, cubeFace{p[0], p[1], labels[i]})
	}
	for i := range net {
		net[i].X -= minX
		net[i].Y -= minY
	}
	frames := foldCube(net)
	valid := []cubeCandidate{}
	invalid := []cubeCandidate{}
	for _, r := range labels {
		for _, t := range labels {
			for _, f := range labels {
				if r == t || r == f || t == f {
					continue
				}
				c := cubeCandidate{Right: r, Top: t, Front: f}
				if validCubeCandidate(frames, c) {
					valid = append(valid, c)
				} else {
					invalid = append(invalid, c)
				}
			}
		}
	}
	rng.Shuffle(len(invalid), func(i, j int) { invalid[i], invalid[j] = invalid[j], invalid[i] })
	candidates := append([]cubeCandidate{valid[rng.Int(len(valid))]}, invalid[:4]...)
	rng.Shuffle(5, func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	q := assessmentQuestion{Prompt: "Fold this net into a cube. Which view is possible using rotations only? Reflections are not allowed. Each label is centered and has no directional texture.", ResponseType: "single_choice"}
	for i := range candidates {
		id := string(rune('a' + i))
		candidates[i].ID = id
		c := candidates[i]
		q.Choices = append(q.Choices, assessmentChoice{id, fmt.Sprintf("View %s · top %s / front %s / right %s", strings.ToUpper(id), c.Top, c.Front, c.Right)})
		if validCubeCandidate(frames, c) {
			q.Answer = id
		}
	}
	q.Assets = map[string]any{"kind": "cube", "net": net, "candidates": candidates, "asset_version": "cube-svg-v1"}
	q.Model = map[string]any{"folded_frames": frames, "proper_rotation_count": len(valid), "net_version": "cross-tail-v1"}
	q.Explanation = "Fold adjacent faces through 90°. A possible view must have three distinct adjacent faces with the correct handedness: right-normal × top-normal = front-normal. This net admits 24 proper rotations; only view " + strings.ToUpper(q.Answer) + " matches."
	return q
}

type gridComponent struct {
	Letter string `json:"letter"`
	Sign   int    `json:"sign"`
}
type gridBet struct {
	ID         string          `json:"id"`
	Components []gridComponent `json:"components"`
	Cost       string          `json:"cost"`
}

func gridPayoff(rows [][2]string, bet gridBet, row int) int {
	total := 0
	for _, c := range bet.Components {
		pay := -1
		if rows[row][0] == c.Letter || rows[row][1] == c.Letter {
			pay = 12
		}
		total += c.Sign * pay
	}
	return total
}
func gridEV(rows [][2]string, weights []int, bet gridBet) *big.Rat {
	sum, numer := 0, 0
	for i, w := range weights {
		sum += w
		numer += w * gridPayoff(rows, bet, i)
	}
	cost, _ := parseExactNumber(bet.Cost)
	return new(big.Rat).Sub(big.NewRat(int64(numer), int64(sum)), cost)
}
func gridTask(rng *engine.RNG) assessmentQuestion {
	letters := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rng.Shuffle(26, func(i, j int) { letters[i], letters[j] = letters[j], letters[i] })
	rows := make([][2]string, 13)
	weights := make([]int, 13)
	total := 0
	for i := range rows {
		rows[i] = [2]string{string(letters[i*2]), string(letters[i*2+1])}
		weights[i] = rng.Range(1, 5)
		total += weights[i]
	}
	probabilities := make([]string, 13)
	for i, w := range weights {
		probabilities[i] = canon(big.NewRat(int64(w), int64(total)))
	}
	bets := make([]gridBet, 4)
	evs := make([]string, 4)
	payoffs := make([][]int, 4)
	// Distinct costs with denominator 101 prevent EV ties: uncosted EV has
	// denominator sum(weights)<=65, coprime to the prime 101.
	for i := range bets {
		sign := 1
		if rng.Int(2) == 0 {
			sign = -1
		}
		components := []gridComponent{{string(letters[rng.Int(26)]), sign}}
		if rng.Int(2) == 0 {
			components = append(components, gridComponent{string(letters[rng.Int(26)]), 1})
		}
		bets[i] = gridBet{ID: string(rune('a' + i)), Components: components, Cost: canon(big.NewRat(int64(i+1), 101))}
		evs[i] = canon(gridEV(rows, weights, bets[i]))
		for r := range rows {
			payoffs[i] = append(payoffs[i], gridPayoff(rows, bets[i], r))
		}
	}
	q := assessmentQuestion{ResponseType: "numeric", Units: "expected net payout"}
	if rng.Int(2) == 0 {
		q.Prompt = "Original payoff model. Compute the expected NET payout of bet A using the disclosed row probabilities, after subtracting its cost."
		q.Answer = evs[0]
	} else {
		q.Prompt = "Original payoff model. Select the bet with the highest expected NET payout after its cost. Each row's probability is disclosed."
		q.ResponseType = "single_choice"
		best := 0
		for i := range bets {
			if gridEV(rows, weights, bets[i]).Cmp(gridEV(rows, weights, bets[best])) > 0 {
				best = i
			}
			q.Choices = append(q.Choices, assessmentChoice{bets[i].ID, "Bet " + strings.ToUpper(bets[i].ID)})
		}
		q.Answer = bets[best].ID
	}
	q.Assets = map[string]any{"kind": "grid", "rows": rows, "probabilities": probabilities, "bets": bets, "rules": "A +letter pays +12 if that letter appears in the winning row, otherwise −1. A −letter negates that payoff. Paired components add, even if a letter repeats. Subtract the cost once. There is no hidden signal or lying grid in this original model."}
	q.Model = map[string]any{"weights": weights, "payoff_table_by_bet": payoffs, "expected_net_payouts": evs, "model_version": "signed-additive-disclosed-weights-v1"}
	parts := []string{}
	for i, ev := range evs {
		parts = append(parts, "Bet "+strings.ToUpper(bets[i].ID)+": "+ev)
	}
	q.Explanation = "For each bet, enumerate all 13 rows, multiply each signed payoff by that row's probability, sum and subtract the cost. " + strings.Join(parts, "; ") + "."
	return q
}

func assessmentQuestionPublic(q assessmentQuestion) map[string]any {
	return map[string]any{"question_id": q.ID, "family": q.Family, "prompt": q.Prompt, "response_type": q.ResponseType, "choices": q.Choices, "assets": q.Assets, "units": q.Units, "item_time_ms": q.ItemTimeMS}
}

func assessmentAnswerText(q assessmentQuestion) string {
	if q.ResponseType == "single_choice" {
		for _, c := range q.Choices {
			if c.ID == q.Answer {
				return c.Text
			}
		}
	}
	return q.Answer
}

func normalizeAssessmentAnswer(q assessmentQuestion, response assessmentResponse) (assessmentResponse, bool) {
	if q.ResponseType == "interval" {
		low, lok := parseExactNumber(response.Low)
		high, hok := parseExactNumber(response.High)
		if !lok || !hok || low.Sign() <= 0 || high.Cmp(low) < 0 || len(response.Reasoning) > 4000 {
			return assessmentResponse{}, false
		}
		lf, _ := low.Float64()
		hf, _ := high.Float64()
		if !finiteAssessmentBound(lf) || !finiteAssessmentBound(hf) {
			return assessmentResponse{}, false
		}
		return assessmentResponse{Low: canon(low), High: canon(high), Reasoning: strings.TrimSpace(response.Reasoning)}, true
	}
	value := strings.TrimSpace(response.Answer)
	if q.ResponseType == "single_choice" {
		for _, c := range q.Choices {
			if c.ID == value {
				return assessmentResponse{Answer: value}, true
			}
		}
		return assessmentResponse{}, false
	}
	if q.ResponseType == "token" {
		value = strings.ToUpper(value)
		if len(value) == 1 && value[0] >= 'A' && value[0] <= 'Z' {
			return assessmentResponse{Answer: value}, true
		}
		return assessmentResponse{}, false
	}
	number, ok := parseExactNumber(value)
	if !ok {
		return assessmentResponse{}, false
	}
	return assessmentResponse{Answer: canon(number)}, true
}
func finiteAssessmentBound(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n > 0 && n <= 1e60
}

func assessmentCorrect(q assessmentQuestion, r assessmentResponse) bool {
	if q.ResponseType == "single_choice" || q.ResponseType == "token" {
		return r.Answer == q.Answer
	}
	actual, ok := parseExactNumber(r.Answer)
	expected, eok := parseExactNumber(q.Answer)
	if !ok || !eok {
		return false
	}
	difference := new(big.Rat).Sub(actual, expected)
	difference.Abs(difference)
	absolute, _ := parseExactNumber(q.AbsTolerance)
	relative, _ := parseExactNumber(q.RelTolerance)
	allowed := new(big.Rat).Mul(relative, new(big.Rat).Abs(expected))
	if absolute.Cmp(allowed) > 0 {
		allowed.Set(absolute)
	}
	return difference.Cmp(allowed) <= 0
}
