package games

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"quantgame/backend/internal/engine"
)

const assessmentBankVersion = "original-tasks-v1"

type assessmentChoice struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
type assessmentQuestion struct {
	ID           string             `json:"question_id"`
	Family       string             `json:"family"`
	Prompt       string             `json:"prompt"`
	ResponseType string             `json:"response_type"`
	Choices      []assessmentChoice `json:"choices,omitempty"`
	Assets       any                `json:"assets,omitempty"`
	Units        string             `json:"units,omitempty"`
	Answer       string             `json:"answer"`
	Explanation  string             `json:"explanation"`
	Model        any                `json:"model,omitempty"`
	AbsTolerance string             `json:"absolute_tolerance"`
	RelTolerance string             `json:"relative_tolerance"`
	ItemTimeMS   int64              `json:"item_time_ms"`
}

func assessmentTask(rng *engine.RNG, family string, index int, responseMode string) assessmentQuestion {
	q := assessmentQuestion{Family: family, ResponseType: "numeric", AbsTolerance: "0", RelTolerance: "0"}
	switch family {
	case "sequence", "anomaly", "letter":
		q = sequenceTask(rng, family)
	case "probability", "advanced_probability", "urn", "counting":
		q = probabilityTask(rng, family)
	case "logic", "reasoning", "verbal", "clerical", "chart", "visual":
		q = reasoningTask(rng, family)
	case "cube":
		q = cubeTask(rng)
	case "equity":
		q = equityTask(rng)
	case "grid":
		q = gridTask(rng)
	case "interval":
		// Same frozen fact pack as Fermi; only the response and loss change.
		safe := []int{0, 1, 2, 3, 5, 6, 9, 11, 12, 16, 17, 18, 19, 23}
		fact := fermiBank[safe[rng.Int(len(safe))]]
		q.Prompt, q.Units = fact.Prompt, fact.Units
		q.Answer = strconv.FormatFloat(fact.Answer, 'g', -1, 64)
		q.ResponseType = "interval"
		q.Explanation = fact.Explanation + " Frozen reference: " + fact.Source + "."
		q.Model = map[string]any{"fact_id": fact.ID, "coverage": 0.8, "alpha": 0.2, "bank_version": "fermi-bank-v1"}
	default:
		q = arithmeticTask(rng, family, index)
	}
	q.ID = fmt.Sprintf("q%03d", index+1)
	q.Family = family
	if q.AbsTolerance == "" {
		q.AbsTolerance = "0"
	}
	if q.RelTolerance == "" {
		q.RelTolerance = "0"
	}
	if responseMode == "single_choice" && len(q.Choices) == 0 && q.ResponseType != "interval" {
		makeAssessmentChoices(rng, &q)
	}
	return q
}

func arithmeticTask(rng *engine.RNG, family string, index int) assessmentQuestion {
	q := assessmentQuestion{ResponseType: "numeric"}
	a, b := rng.Range(2, 99), rng.Range(2, 30)
	switch family {
	case "four_digit":
		x, y := rng.Range(1000, 9999)*100+rng.Int(4)*25, rng.Range(1000, 9999)*100+rng.Int(2)*50
		if rng.Int(2) == 0 {
			q.Prompt = fmt.Sprintf("%.2f + %.2f = ?", float64(x)/100, float64(y)/100)
			q.Answer = canon(big.NewRat(int64(x+y), 100))
		} else {
			if x < y {
				x, y = y, x
			}
			q.Prompt = fmt.Sprintf("%.2f − %.2f = ?", float64(x)/100, float64(y)/100)
			q.Answer = canon(big.NewRat(int64(x-y), 100))
		}
		q.Model = map[string]any{"family": "four_digit", "operand_cents": []int{x, y}}
	case "percentage":
		q.Prompt = fmt.Sprintf("What is %d%% of %d?", b, a*10)
		q.Answer = canon(big.NewRat(int64(b*a), 10))
		q.Units = "number, not percent"
		q.Model = map[string]int{"percentage": b, "quantity": a * 10}
	case "power":
		a, b = rng.Range(2, 8), rng.Range(2, 4)
		v := 1
		for range b {
			v *= a
		}
		q.Prompt = fmt.Sprintf("%d raised to the power %d = ?", a, b)
		q.Answer = strconv.Itoa(v)
		q.Model = map[string]int{"base": a, "exponent": b}
	case "relation":
		q.Prompt = fmt.Sprintf("Solve for x: %dx + %d = %d", b, a, b*(a+2)+a)
		q.Answer = strconv.Itoa(a + 2)
		q.Model = map[string]int{"coefficient": b, "offset": a, "rhs": b*(a+2) + a}
	case "chained":
		c := rng.Range(2, 12)
		q.Prompt = fmt.Sprintf("%.1f × %d × %.1f = ?", float64(a)/10, b, float64(c)/10)
		q.Answer = canon(big.NewRat(int64(a*b*c), 100))
		q.Model = map[string]any{"operands": []string{canon(big.NewRat(int64(a), 10)), strconv.Itoa(b), canon(big.NewRat(int64(c), 10))}}
	default:
		mode := family
		if mode == "arithmetic" {
			mode = []string{"integer", "decimal", "fraction"}[rng.Int(3)]
		}
		item := genMathQuestion(rng, index, mode)
		q.Prompt, q.Answer = item.Expression+" = ?", item.AnswerNum
		q.Model = map[string]any{"shared_generator": "math-challenge-v1", "mode": mode, "expression": item.Expression}
	}
	q.Explanation = fmt.Sprintf("Evaluate the displayed expression with exact arithmetic. The result is %s.", q.Answer)
	return q
}

func makeAssessmentChoices(rng *engine.RNG, q *assessmentQuestion) {
	values := []string{q.Answer}
	seen := map[string]bool{q.Answer: true}
	truth, numeric := parseExactNumber(q.Answer)
	for offset := 1; len(values) < 4; offset++ {
		var candidate string
		if numeric {
			v := new(big.Rat).Add(truth, big.NewRat(int64(offset), 1))
			if offset%2 == 0 {
				v.Sub(truth, big.NewRat(int64(offset), 1))
			}
			if q.Family == "probability" && truth.Sign() >= 0 && truth.Cmp(big.NewRat(1, 1)) <= 0 {
				v = big.NewRat(int64(offset), int64(offset+2))
			}
			candidate = canon(v)
		} else if len(q.Answer) == 1 && q.Answer[0] >= 'A' && q.Answer[0] <= 'Z' {
			candidate = string(rune('A' + (int(q.Answer[0]-'A')+offset)%26))
		} else {
			candidate = fmt.Sprintf("Alternative %d", offset)
		}
		if !seen[candidate] {
			seen[candidate] = true
			values = append(values, candidate)
		}
	}
	rng.Shuffle(len(values), func(i, j int) { values[i], values[j] = values[j], values[i] })
	truthText := q.Answer
	q.Choices = nil
	for i, text := range values {
		id := string(rune('a' + i))
		q.Choices = append(q.Choices, assessmentChoice{id, text})
		if text == truthText {
			q.Answer = id
		}
	}
	q.ResponseType = "single_choice"
}

func sequenceTask(rng *engine.RNG, family string) assessmentQuestion {
	q := assessmentQuestion{ResponseType: "numeric"}
	start, step := rng.Range(1, 20), rng.Range(2, 6)
	tokens := make([]string, 5)
	answer := 0
	rule := ""
	if family == "letter" {
		step = rng.Range(1, 4)
		start = rng.Range(0, 25-5*step)
		for i := range tokens {
			tokens[i] = string(rune('A' + start + i*step))
		}
		q.Answer = string(rune('A' + start + 5*step))
		q.ResponseType = "token"
		rule = fmt.Sprintf("Move forward %d positions in A–Z each time, with no wraparound.", step)
	} else if family == "anomaly" {
		values := make([]int, 5)
		for i := range values {
			values[i] = start + i*step
		}
		wrong := rng.Range(1, 3)
		values[wrong]++
		for i, v := range values {
			tokens[i] = strconv.Itoa(v)
		}
		q.Answer = strconv.Itoa(values[wrong])
		rule = fmt.Sprintf("The intended arithmetic progression has step %d. Position %d should contain %d, so the wrong value is %d.", step, wrong+1, start+wrong*step, values[wrong])
		q.Model = map[string]any{"grammar": "one-corrupt-arithmetic", "start": start, "step": step, "wrong_index": wrong, "corrected_value": start + wrong*step}
	} else {
		kind := rng.Int(4)
		for i := range tokens {
			v := start + i*step
			switch kind {
			case 1:
				v = start
				for range i {
					v *= step
				}
			case 2:
				v = start + step*i*(i+1)/2
			case 3:
				v = start + (i/2)*step
				if i%2 == 1 {
					v += 20
				}
			}
			tokens[i] = strconv.Itoa(v)
		}
		switch kind {
		case 0:
			answer = start + 5*step
			rule = fmt.Sprintf("Add %d each time.", step)
		case 1:
			answer = start
			for range 5 {
				answer *= step
			}
			rule = fmt.Sprintf("Multiply by %d each time.", step)
		case 2:
			answer = start + 15*step
			rule = fmt.Sprintf("Successive differences are %d, %d, %d, …", step, 2*step, 3*step)
		case 3:
			answer = start + 20 + 2*step
			rule = fmt.Sprintf("Interleave two arithmetic progressions, both with step %d; the second begins 20 above the first.", step)
		}
		q.Answer = strconv.Itoa(answer)
		q.Model = map[string]any{"grammar": []string{"arithmetic", "geometric", "growing-differences", "interleaved"}[kind], "start": start, "step": step}
	}
	q.Assets = map[string]any{"kind": "sequence", "tokens": tokens, "task": family}
	q.Prompt = "Enter the next term in this sequence."
	if family == "anomaly" {
		q.Prompt = "This arithmetic progression contains exactly one wrong value. Enter the WRONG VALUE, not its position."
	}
	q.Explanation = rule + " Answer: " + q.Answer + "."
	return q
}

func probabilityTask(rng *engine.RNG, family string) assessmentQuestion {
	q := assessmentQuestion{ResponseType: "numeric"}
	a, b := rng.Range(2, 8), rng.Range(2, 7)
	kind := rng.Int(7)
	parameters := map[string]any{"a": a, "b": b}
	if family == "urn" {
		kind = 0
	}
	if family == "counting" {
		kind = 5
	}
	if family == "probability" {
		primitive := buildPrimitive(rng)
		pred := makePredicate(rng, primitive, "event")
		denom := map[string]int{"dice2": 36, "die1": 6, "coin3": 8, "card": 13}[primitive.Type]
		numer := 0
		for _, o := range primitive.Outcomes {
			if matchesPredicate(primitive.Type, o.Desc, pred.Kind, pred.Arg) {
				numer += int(math.Round(o.P * float64(denom)))
			}
		}
		intro := map[string]string{"dice2": "Roll two independent fair six-sided dice.", "die1": "Roll one fair six-sided die.", "coin3": "Toss a fair coin three independent times.", "card": "Draw one card uniformly from a standard 52-card deck; ace=1, J=11, Q=12, K=13."}[primitive.Type]
		q.Prompt = intro + " Compute P(" + pred.Text + ")."
		q.Answer = canon(big.NewRat(int64(numer), int64(denom)))
		q.Explanation = fmt.Sprintf("Enumerate the equally likely elementary outcomes. There are %d favorable out of %d (card ranks each have equal multiplicity): probability %s.", numer, denom, q.Answer)
		q.Model = map[string]any{"shared_generator": "probability-betting-v1", "primitive": primitive.Type, "predicate": pred.Kind, "argument": pred.Arg, "favorable": numer, "total": denom}
		return q
	}
	switch kind {
	case 0:
		q.Prompt = fmt.Sprintf("A bag has %d red and %d blue balls. Draw two without replacement. What is P(both red)?", a, b)
		q.Answer = canon(big.NewRat(int64(a*(a-1)), int64((a+b)*(a+b-1))))
		q.Explanation = fmt.Sprintf("Multiply the conditional probabilities: %d/%d × %d/%d = %s.", a, a+b, a-1, a+b-1, q.Answer)
	case 1:
		// Two equally likely coins with rational head probabilities a/10,b/10.
		q.Prompt = fmt.Sprintf("Select coin A or B with probability 1/2 each, then toss once. P(heads|A)=%d/10 and P(heads|B)=%d/10. Given heads, what is P(A)?", a, b)
		q.Answer = canon(big.NewRat(int64(a), int64(a+b)))
		q.Explanation = "Bayes: (a/10 × 1/2) / ((a+b)/10 × 1/2) = " + q.Answer + "."
	case 2:
		q.Prompt = fmt.Sprintf("Independent X~Bernoulli(%d/10) and Y~Bernoulli(%d/10). Find Var(X+Y).", a, b)
		q.Answer = canon(big.NewRat(int64(a*(10-a)+b*(10-b)), 100))
		q.Explanation = "Independence makes variances add. p(1−p)+q(1−q) = " + q.Answer + "."
	case 3:
		q.Prompt = fmt.Sprintf("Independent trials succeed with probability %d/%d. Count the successful trial too. What is the expected number of trials until the first success?", a, a+b)
		q.Answer = canon(big.NewRat(int64(a+b), int64(a)))
		q.Explanation = "For a geometric waiting time including the first success, E[T]=1/p = " + q.Answer + "."
	case 4:
		q.Prompt = fmt.Sprintf("Independent tosses have P(heads)=%d/10. What is the expected number of tosses to see two consecutive heads, starting with no trailing head?", a)
		q.Answer = canon(big.NewRat(int64(100+10*a), int64(a*a)))
		q.Explanation = "Let E be the wait with no trailing head and H with one. E=1+(1−p)E+pH; H=1+(1−p)E. Hence E=(1+p)/p² = " + q.Answer + "."
	case 6:
		q.Prompt = fmt.Sprintf("A net payoff is +%d with probability %d/10, otherwise −%d. Find its expected net value.", b*4, a, b)
		q.Answer = canon(big.NewRat(int64(b*(5*a-10)), 10))
		q.Explanation = fmt.Sprintf("Multiply each net payoff by its probability and add: (%d/10)×%d + (1−%d/10)×(−%d) = %s.", a, b*4, a, b, q.Answer)

	case 5:
		n, k := rng.Range(5, 12), rng.Range(2, 4)
		value := new(big.Int).Binomial(int64(n), int64(k))
		parameters["n"], parameters["k"] = n, k
		q.Prompt = fmt.Sprintf("Choose an unordered committee of %d distinct people from %d distinct candidates, without replacement. How many committees exist?", k, n)
		q.Answer = value.String()
		q.Explanation = fmt.Sprintf("Use the binomial coefficient C(%d,%d)=%s. Order does not matter.", n, k, q.Answer)
	}
	q.Model = map[string]any{"template": []string{"urn-without-replacement", "bayes-two-coins", "bernoulli-variance", "geometric-wait", "two-consecutive-heads", "binomial-coefficient", "two-payoff-expectation"}[kind], "parameters": parameters, "exact_answer": q.Answer}
	return q
}

func reasoningTask(rng *engine.RNG, family string) assessmentQuestion {
	if family == "reasoning" {
		family = []string{"logic", "verbal", "clerical", "chart", "visual"}[rng.Int(5)]
	}
	q := assessmentQuestion{ResponseType: "single_choice"}
	texts := []string{}
	correct := 0
	switch family {
	case "verbal":
		pairs := [][4]string{{"Bird : nest", "Bee : hive", "Bee : honey", "Bee : flower"}, {"Thermometer : temperature", "Clock : time", "Clock : wall", "Clock : hand"}, {"Page : book", "Leaf : tree", "Leaf : green", "Leaf : autumn"}, {"Painter : brush", "Writer : pen", "Writer : reader", "Writer : paper"}}
		pair := pairs[rng.Int(len(pairs))]
		q.Prompt = "Choose the analogy that preserves this relationship: " + pair[0]
		texts = []string{pair[1], pair[2], pair[3], "None of these"}
		q.Explanation = "The matching relation is " + pair[1] + "; compare object/function or part/whole relationships in the prompt."
	case "clerical":
		alphabet := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		var code strings.Builder
		for range 8 {
			code.WriteByte(alphabet[rng.Int(len(alphabet))])
		}
		s := code.String()
		q.Prompt = "Which code is EXACTLY identical to " + s + "?"
		texts = []string{s}
		for i := 0; i < 3; i++ {
			v := []byte(s)
			pos := i * 2
			v[pos] = alphabet[(strings.IndexByte(alphabet, v[pos])+1)%len(alphabet)]
			texts = append(texts, string(v))
		}
		q.Explanation = "Compare each character in order. The identical code is " + s + "."
	case "chart":
		before, after := rng.Range(4, 16)*10, rng.Range(17, 30)*10
		q.Prompt = "The table shows sales in units. What is the percentage increase from period A to B? Enter the percentage as a number (e.g. 25 for 25%)."
		q.ResponseType = "numeric"
		q.Units = "percent"
		q.Answer = canon(big.NewRat(int64((after-before)*100), int64(before)))
		q.Explanation = fmt.Sprintf("(B−A)/A × 100 = (%d−%d)/%d × 100 = %s%%.", after, before, before, q.Answer)
		q.Assets = map[string]any{"kind": "table", "headers": []string{"Period", "Sales (units)"}, "rows": [][]string{{"A", strconv.Itoa(before)}, {"B", strconv.Itoa(after)}}}
		return q
	case "visual":
		start := rng.Int(4)
		step := rng.Range(1, 3)
		shapes := make([]int, 4)
		for i := range shapes {
			shapes[i] = (start + i*step) % 4
		}
		want := (start + 4*step) % 4
		q.Prompt = "Each arrow turns by a constant number of quarter-turns clockwise. Choose the next orientation."
		q.Assets = map[string]any{"kind": "arrows", "orientations": shapes}
		texts = []string{"↑", "→", "↓", "←"}
		correct = want
		q.Explanation = fmt.Sprintf("Advance %d quarter-turn(s) clockwise each time. The next arrow is %s.", step, texts[want])
	default:
		sets := [][3]string{{"cedars", "trees", "stones"}, {"traders", "analysts", "robots"}, {"A-items", "B-items", "C-items"}}
		v := sets[rng.Int(len(sets))]
		q.Prompt = fmt.Sprintf("All %s are %s. No %s are %s. Which statement must be true?", v[0], v[1], v[1], v[2])
		texts = []string{fmt.Sprintf("No %s are %s.", v[0], v[2]), fmt.Sprintf("All %s are %s.", v[2], v[0]), fmt.Sprintf("Some %s are %s.", v[0], v[2]), fmt.Sprintf("Every %s is a %s.", v[1], v[0])}
		q.Explanation = "The first group is a subset of the second; the second is disjoint from the third. Therefore the first and third are disjoint."
	}
	order := []int{0, 1, 2, 3}
	rng.Shuffle(4, func(i, j int) { order[i], order[j] = order[j], order[i] })
	for i, j := range order {
		id := string(rune('a' + i))
		q.Choices = append(q.Choices, assessmentChoice{id, texts[j]})
		if j == correct {
			q.Answer = id
		}
	}
	q.Model = map[string]any{"template": family, "correct_text": texts[correct]}
	return q
}

func equityTask(rng *engine.RNG) assessmentQuestion {
	q := assessmentQuestion{ResponseType: "numeric", Units: "currency per diluted share"}
	shares, eps, multiple := rng.Range(5, 25), rng.Range(2, 8), rng.Range(8, 25)
	rows := [][]string{}
	kind := rng.Int(2)
	if kind == 0 {
		q.Prompt = "Synthetic FY2026 statement: use diluted EPS and the supplied P/E multiple to calculate the implied EQUITY price per share. All money and share counts are in millions."
		rows = [][]string{{"Net income", strconv.Itoa(shares * eps)}, {"Diluted shares", strconv.Itoa(shares)}, {"P/E multiple", strconv.Itoa(multiple)}}
		q.Answer = strconv.Itoa(eps * multiple)
		q.Explanation = fmt.Sprintf("EPS = %d / %d = %d; price = EPS × P/E = %d.", shares*eps, shares, eps, eps*multiple)
	} else {
		ebitda, debt, cash := rng.Range(20, 80), rng.Range(10, 50), rng.Range(5, 20)
		ev := ebitda * multiple
		q.Prompt = "Synthetic FY2026 statement: value the enterprise at the supplied EV/EBITDA multiple, then convert to EQUITY price per diluted share. All money and share counts are in millions."
		rows = [][]string{{"EBITDA", strconv.Itoa(ebitda)}, {"EV/EBITDA", strconv.Itoa(multiple)}, {"Debt", strconv.Itoa(debt)}, {"Cash", strconv.Itoa(cash)}, {"Diluted shares", strconv.Itoa(shares)}}
		q.Answer = canon(big.NewRat(int64(ev-debt+cash), int64(shares)))
		q.Explanation = fmt.Sprintf("EV = %d × %d = %d. Equity = EV − debt + cash = %d. Divide by %d diluted shares to get %s.", ebitda, multiple, ev, ev-debt+cash, shares, q.Answer)
	}
	q.Assets = map[string]any{"kind": "table", "headers": []string{"Frozen metric", "Value"}, "rows": rows}
	q.Model = map[string]any{"template": []string{"pe-valuation", "enterprise-to-equity"}[kind], "statement": "synthetic-fy2026", "units": "millions"}
	return q
}
