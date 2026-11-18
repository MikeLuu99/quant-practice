package games

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"quantgame/backend/internal/engine"
)

func trainerMathQuestion(rng *engine.RNG, c trainerConfig, index int) assessmentQuestion {
	family := c.Family
	if family == "mixed" {
		family = []string{"integer", "decimal", "fraction"}[rng.Int(3)]
	}
	op := c.Operations[rng.Int(len(c.Operations))]
	if op == "%" || family == "percentage" {
		return arithmeticTask(rng, "percentage", index)
	}
	// The rational parser/canonicalizer and fraction generator are shared with
	// Math Challenge; explicit operand-size settings use a bounded generator.
	if family == "fraction" {
		for range 100 {
			q := genMathQuestion(rng, index, "fraction")
			fields := strings.Fields(q.Expression)
			operator := strings.ReplaceAll(fields[1], "x", "*")
			if slices.Contains(c.Operations, operator) {
				return assessmentQuestion{Prompt: q.Expression + " = ?", ResponseType: "numeric", Answer: q.AnswerNum, Explanation: "Use exact rational arithmetic: " + q.AnswerNum + ".", Family: "fraction", AbsTolerance: "0", RelTolerance: "0"}
			}
		}
	}
	lo := int(math.Pow10(c.Digits - 1))
	hi := int(math.Pow10(c.Digits)) - 1
	a, b := rng.Range(lo, hi), rng.Range(lo, hi)
	left, right := big.NewRat(int64(a), 1), big.NewRat(int64(b), 1)
	if family == "decimal" {
		left.Quo(left, big.NewRat(10, 1))
		right.Quo(right, big.NewRat(10, 1))
	}
	value := new(big.Rat)
	switch op {
	case "+":
		value.Add(left, right)
	case "-":
		value.Sub(left, right)
	case "*":
		value.Mul(left, right)
	case "/":
		value.Quo(left, right)
	}
	l, r := canon(left), canon(right)
	if family == "decimal" {
		l = left.FloatString(1)
		r = right.FloatString(1)
	}
	return assessmentQuestion{Prompt: fmt.Sprintf("%s %s %s = ?", l, map[string]string{"*": "×", "/": "÷", "+": "+", "-": "−"}[op], r), ResponseType: "numeric", Answer: canon(value), Explanation: fmt.Sprintf("Evaluate using exact arithmetic: %s.", canon(value)), Family: family, AbsTolerance: "0", RelTolerance: "0"}
}
func trainerProbabilityQuestion(rng *engine.RNG, c trainerConfig) assessmentQuestion {
	family := "probability"
	if c.Difficulty == 3 || c.Family == "bayes" || c.Family == "expectation" {
		family = "advanced_probability"
	}
	if c.Difficulty == 2 && rng.Int(2) == 1 {
		family = "advanced_probability"
	}
	if c.Family == "counting" {
		family = "counting"
	}
	for range 200 {
		q := probabilityTask(rng, family)
		q.Family = "probability"
		if c.Family != "bayes" && c.Family != "expectation" {
			return q
		}
		m := q.Model.(map[string]any)
		name := trainerString(m["template"])
		if c.Family == "bayes" && name == "bayes-two-coins" {
			return q
		}
		if c.Family == "expectation" && slices.Contains([]string{"geometric-wait", "two-consecutive-heads", "two-payoff-expectation"}, name) {
			return q
		}
	}
	return probabilityTask(rng, "advanced_probability")
}
func roundedFractionDigits(n, d, places int) string {
	scale := int64(1)
	for range places {
		scale *= 10
	}
	quot, rem := int64(n)*scale/int64(d), int64(n)*scale%int64(d)
	if rem*2 >= int64(d) {
		quot++
	}
	return fmt.Sprintf("%0*d", places, quot)
}
func (g *trainerGame) generateTrial() trainerTrial {
	i := len(g.trials)
	rng := trainerRNG(g.ctx.Seed, g.cfg.TrainerID, i, g.level)
	t := trainerTrial{ID: fmt.Sprintf("trial-%03d", i+1), Index: i, Level: g.level, Kind: g.cfg.TrainerID, Public: map[string]any{}, Secret: map[string]any{}}
	c := g.cfg
	switch c.TrainerID {
	case "math":
		q := trainerMathQuestion(rng, c, i)
		t.Question = &q
	case "sequence":
		family := "sequence"
		if c.Mode == "letters" {
			family = "letter"
		}
		q := sequenceTask(rng, family)
		if c.Mode == "numbers" && c.Difficulty > 1 && rng.Int(5) == 0 {
			a, b := rng.Range(1, 5), rng.Range(1, 7)
			values := []int{a, b}
			for len(values) < 7 {
				n := len(values)
				values = append(values, values[n-1]+values[n-2])
			}
			tokens := []string{}
			for _, v := range values[:6] {
				tokens = append(tokens, strconv.Itoa(v))
			}
			q.Assets = map[string]any{"kind": "sequence", "tokens": tokens, "task": "sequence"}
			q.Answer = strconv.Itoa(values[6])
			q.Model = map[string]any{"grammar": "sum-previous-two", "values": values}
			q.Explanation = "Each term is the sum of the preceding two: " + q.Answer + "."
		}
		t.Question = &q
	case "probability":
		q := trainerProbabilityQuestion(rng, c)
		t.Question = &q
	case "fractions":
		n, d := 0, 0
		digits := ""
		for n == 0 || len(digits) > c.Places {
			d = rng.Range(3, 97)
			n = rng.Range(1, d-1)
			digits = roundedFractionDigits(n, d, c.Places)
		}
		q := assessmentQuestion{Prompt: fmt.Sprintf("Round %d/%d to %d decimal places. Enter only the %d digits after 0.", n, d, c.Places, c.Places), ResponseType: "digits", Answer: digits, Explanation: fmt.Sprintf("%d/%d = %s…; round half up at %d places to 0.%s.", n, d, big.NewRat(int64(n), int64(d)).FloatString(c.Places+6), c.Places, digits)}
		t.Question = &q
		t.Secret["numerator"], t.Secret["denominator"] = n, d
		t.Public["places"] = c.Places
	case "fermi":
		item := fermiBank[rng.Int(len(fermiBank))]
		if c.ItemID != "" {
			for _, q := range fermiBank {
				if q.ID == c.ItemID {
					item = q
					break
				}
			}
		}
		if item.Answer <= 0 {
			item = fermiBank[0]
		}
		q := assessmentQuestion{Prompt: item.Prompt, Units: item.Units, ResponseType: "numeric", Answer: strconv.FormatFloat(item.Answer, 'g', -1, 64), Explanation: item.Explanation + " Reference: " + item.Source}
		t.Question = &q
		t.Public["fact_id"] = item.ID
		t.Secret["hint"] = fmt.Sprintf("Think in units of roughly 10^%d %s. This is a learning hint, not an unassisted answer.", int(math.Floor(math.Log10(item.Answer))), item.Units)
	case "cube":
		q := cubeTask(rng)
		decorateTrainerCube(&q, c.FaceStyle, rng)
		t.Question = &q
	case "etf", "options", "futures", "prediction":
		return g.tradingTrial(t, rng)
	case "task-switch":
		rule := rng.Int(2)
		yes := false
		if c.Mode == "classic" {
			letter := []string{"A", "E", "I", "O", "U", "B", "C", "D", "F", "G"}[rng.Int(10)]
			n := rng.Range(1, 9)
			t.Public["display"] = fmt.Sprintf("%s   %d", letter, n)
			if rule == 0 {
				t.Public["cue"] = "Is the letter a vowel?"
				yes = strings.Contains("AEIOU", letter)
			} else {
				t.Public["cue"] = "Is the number odd?"
				yes = n%2 == 1
			}
		}
		if c.Mode == "realistic" {
			if rule == 0 {
				a := []string{"↑", "→", "↓", "←"}[rng.Int(4)] + []string{"↑", "→", "↓", "←"}[rng.Int(4)]
				b := a
				yes = rng.Int(2) == 0
				if !yes {
					for b == a {
						b = []string{"↑↑", "→→", "↓↓", "←←", "↑←"}[rng.Int(5)]
					}
				}
				t.Public["cue"] = "Are the arrow sets identical?"
				t.Public["display"] = a + "    " + b
			} else {
				a, b := rng.Range(1, 30), rng.Range(1, 30)
				yes = (a+b)%2 == 1
				t.Public["cue"] = "Is this sum odd?"
				t.Public["display"] = fmt.Sprintf("%d + %d", a, b)
			}
		}
		t.Secret["yes"] = yes
		t.Public["mapping"] = "← No / Yes →"
		t.Public["rule"] = rule
		t.Secret["switch"] = i > 0 && g.lastRule != rule
		g.lastRule = rule
	case "response-conflict":
		target := rng.Int(2)
		distractor := rng.Int(2)
		side := []string{"LEFT", "RIGHT"}[target]
		t.Public["mapping"] = "H / Circle → Left   ·   S / Square → Right"
		t.Secret["side"] = side
		t.Secret["congruent"] = target == distractor
		if c.Mode == "flanker" {
			letters := []string{"H", "S"}
			t.Public["display"] = letters[distractor] + letters[distractor] + letters[target] + letters[distractor] + letters[distractor]
			t.Public["cue"] = "Answer the CENTRAL letter"
		} else {
			t.Public["shape"] = []string{"circle", "square"}[target]
			t.Public["location"] = []string{"left", "right"}[distractor]
			t.Public["cue"] = "Answer the shape's mapped side, not its position"
		}
	case "pincode":
		span := g.level
		if c.FixedSpan > 0 {
			span = c.FixedSpan
		}
		var digits strings.Builder
		for range span {
			digits.WriteByte(byte('0' + rng.Int(10)))
		}
		value := digits.String()
		want := []byte(value)
		if c.Mode == "reverse" {
			slices.Reverse(want)
		}
		if c.Mode == "sort" {
			slices.Sort(want)
		}
		t.Secret["digits"], t.Secret["answer"] = value, string(want)
		t.Public["length"], t.Public["mode"] = span, c.Mode
		t.EncodeMS = int64(span*650 - 150)
		t.MaskMS = 300
	case "code-match":
		n := 7 + 3*i/max(1, c.Rounds-1)
		alphabet := "ABCDEFGHJKLMNPQRSTUVWXYZ0123456789"
		var ref strings.Builder
		for range n {
			ref.WriteByte(alphabet[rng.Int(len(alphabet))])
		}
		value := ref.String()
		options := []string{value}
		seen := map[string]bool{value: true}
		for len(options) < 4 {
			v := []byte(value)
			pos := rng.Int(n)
			if rng.Int(2) == 0 && pos+1 < n {
				v[pos], v[pos+1] = v[pos+1], v[pos]
			} else {
				v[pos] = alphabet[rng.Int(len(alphabet))]
			}
			s := string(v)
			if !seen[s] {
				seen[s] = true
				options = append(options, s)
			}
		}
		rng.Shuffle(4, func(i, j int) { options[i], options[j] = options[j], options[i] })
		t.Public["reference"] = value
		t.Public["options"] = options
		t.Secret["answer"] = slices.Index(options, value)
		first, last := 6000, 3500
		if c.Mode == "pressure" {
			first, last = 5000, 2000
		}
		t.ResponseMS = int64(first - (first-last)*i/max(1, c.Rounds-1))
	case "grid-recall":
		cells := make([]int, 16)
		for j := range cells {
			cells[j] = j
		}
		rng.Shuffle(16, func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
		span := min(12, max(1, g.level))
		targets := append([]int(nil), cells[:span]...)
		decoys := []int{}
		if i%2 == 1 {
			decoys = append(decoys, cells[span:min(span+2, 16)]...)
		}
		t.Secret["targets"], t.Secret["decoys"] = targets, decoys
		t.Public["span"] = span
		t.Public["sequential"] = c.Mode == "sequential"
		t.EncodeMS = 1000
		if c.Mode == "sequential" {
			t.EncodeMS = int64(span*650 - 150)
		}
		t.MaskMS = 300
	case "intercept":
		v := 100 + float64(g.level-1)*3
		line := []float64{160, 320, 480, 640}[rng.Int(4)]
		cannons := []map[string]float64{{"x": 0, "y": 500, "slope": 0.5}, {"x": 0, "y": 420, "slope": 0.4}, {"x": 800, "y": 500, "slope": -0.5}, {"x": 800, "y": 420, "slope": -0.4}}
		crossings := []map[string]float64{}
		deadline := 0.0
		for _, cannon := range cannons {
			dx := line - cannon["x"]
			y := cannon["y"] - cannon["slope"]*dx
			travel := math.Hypot(dx, y-cannon["y"]) / 800
			ball := (y + 20) / v
			crossings = append(crossings, map[string]float64{"x": line, "y": y, "travel_s": travel, "ball_s": ball, "ideal_fire_s": ball - travel})
			deadline = math.Max(deadline, ball)
		}
		t.Public["line_x"], t.Public["speed"], t.Public["cannons"], t.Public["crossings"] = line, v, cannons, crossings
		t.Public["occluded"] = c.Mode == "occluded"
		t.Public["tolerance_ms"] = math.Max(35, 140-float64(g.level-1)*10)
		t.ResponseMS = int64(deadline*1000) + 300
	case "hold-fire":
		rule := g.lastRule
		chance := 4
		if c.Mode == "switching" {
			chance = 8
		}
		if i == 0 || rng.Int(10) < chance {
			rule = 1 - rule
		}
		target := g.holdTargets[i%len(g.holdTargets)]
		color := []string{"red", "blue"}[rng.Int(2)]
		shape := []string{"circle", "square"}[rng.Int(2)]
		if rule == 0 {
			color = "blue"
			if target {
				color = "red"
			}
		} else {
			shape = "square"
			if target {
				shape = "circle"
			}
		}
		t.Public["cue"] = []string{"COLOUR · press only for RED", "SHAPE · press only for CIRCLES"}[rule]
		t.Public["color"], t.Public["shape"] = color, shape
		t.Secret["target"] = target
		t.Secret["switch"] = i > 0 && rule != g.lastRule
		g.lastRule = rule
		t.EncodeMS = 300
		t.ResponseMS = max(int64(350), 1200-int64(g.level-1)*50)
	case "target-number":
		sources, target, witness, count := targetNumberPuzzle(rng, i)
		t.Public["sources"], t.Public["target"] = sources, target
		t.Secret["solution"] = witness
		t.Secret["solution_count"] = count
		t.ResponseMS = 45000
		if c.Mode == "timed" {
			t.ResponseMS = 18000
		}
	case "pump":
		t.Secret["threshold"] = g.pumpThresholds[i]
		t.Public["cents_per_pump"] = 10
		if c.Mode == "stakes" {
			t.Public["cents_per_pump"] = 20
		}
		t.Public["mode"] = c.Mode
	case "skyline":
		capacities := []int{5, 5, 5}
		if c.Mode == "capped" {
			capacities = []int{3, 2, 1}
		}
		goal := [][]string{{"A", "B"}, {"C", "D"}, {"E"}}
		distance := 2 + i%6
		start, witness := skylinePuzzle(rng, goal, capacities, distance)
		t.Public["start"], t.Public["goal"], t.Public["capacities"], t.Public["minimum_moves"] = start, goal, capacities, len(witness)
		t.Secret["solution"] = witness
	case "mystery":
		attrs := []map[string]any{{"name": "Colour", "values": []string{"Red", "Blue", "Gold"}}, {"name": "Shape", "values": []string{"Circle", "Square", "Triangle"}}, {"name": "Pattern", "values": []string{"Plain", "Striped", "Dotted"}}, {"name": "Border", "values": []string{"Thin", "Thick", "Dashed"}}, {"name": "Symbol", "values": []string{"Star", "Moon", "Plus", "Diamond"}}, {"name": "Size", "values": []string{"Small", "Medium", "Large", "Huge"}}}
		n := []int{2, 3, 4, 5, 5}[i%5]
		if c.Mode == "deep" {
			n = []int{3, 4, 5, 6, 6}[i%5]
		}
		secret := []string{}
		for _, a := range attrs[:n] {
			v := a["values"].([]string)
			secret = append(secret, v[rng.Int(len(v))])
		}
		t.Public["attributes"] = attrs[:n]
		t.Secret["secret"] = secret
	case "sweet-spot":
		t.Public["field_width"], t.Public["field_height"] = 800, 500
	}
	if t.Question != nil {
		t.Question.ID = t.ID
		t.Question.AbsTolerance = "0"
		t.Question.RelTolerance = "0"
		if c.AnswerFormat == "choice" && t.Question.ResponseType != "digits" && len(t.Question.Choices) == 0 && c.TrainerID != "fermi" {
			makeAssessmentChoices(rng, t.Question)
		}
	}
	return t
}
func decorateTrainerCube(q *assessmentQuestion, style string, rng *engine.RNG) {
	styles := []string{"colors", "shapes", "dice", "numbers", "letters", "icons", "patterns"}
	if style == "mixed" {
		style = styles[rng.Int(len(styles))]
	}
	a := q.Assets.(map[string]any)
	a["face_style"] = style
	a["texture_policy"] = "quarter-turn symmetric textures; labelled face identity is authoritative"
	q.Explanation += " This original renderer uses rotation-symmetric face motifs, so no hidden texture orientation changes the answer."
}
