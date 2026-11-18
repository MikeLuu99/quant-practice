package games

import (
	"fmt"
	"strconv"

	"quantgame/backend/internal/engine"
)

type trainerLesson struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Formula string `json:"formula"`
	Example string `json:"example"`
}

var trainerLessons = map[string][]trainerLesson{
	"etf": {
		{"Basket units", "Holdings specify shares per creation unit. A divisor converts basket dollars to one ETF unit.", "NAV = Σ(wᵢ × Sᵢ) / D", "2 A at 40 and 1 B at 30, D=1 → 110."},
		{"Capitalization", "Market capitalization uses outstanding shares, rather than a stock's price alone.", "Capitalization = price × outstanding shares", "20 dollars × 5 million shares = 100 million dollars."},
		{"Fair value", "Add marked component values with consistent currency and units.", "NAV = basket value / divisor", "Basket 450, divisor 5 → 90 per ETF."},
		{"Component shocks", "Only exposed components move basket value in a single-name shock.", "ΔNAV = wᵢ ΔSᵢ / D", "Two A shares rise 3 dollars each → NAV rises 6."},
		{"Hedge direction", "A short ETF carries negative component exposure; buying the basket offsets it.", "Component hedge = −ETF position × wᵢ / D", "Short 5 ETF with two A each → buy 10 A."},
		{"Contract units", "Multiply by the number of ETF units in each contract before hedging.", "Exposure = contracts × units × weight / divisor", "3 contracts of 10 units, weight 2 → 60 shares."},
		{"Executable arbitrage", "Use basket bids when selling components and the ETF ask when buying the fund.", "Buy ETF edge = basket bids / D − ETF ask − fees", "Executable basket 110; ETF ask 108 → 2 gross edge."},
		{"Creation and redemption", "An original frictionless baseline permits an atomic ETF/basket conversion. Real constraints need their own settings.", "ETF ↔ specified basket", "Buying ETF at 108 and redeeming into a 110 basket locks 2 before fees."},
		{"Two-way quoting", "A valid two-way quote has a finite bid below ask and positive size. This practice rubric asks for a tight quote around fair value.", "bid < fair < ask", "Fair 100: 99.5 / 100.5 is centered with width 1."},
		{"Inventory and settlement", "Cash and positions change on fills; marking open positions is distinct from realized cash.", "Equity = cash + ETF units × NAV + component shares × price", "Buy 1 ETF at 99 then mark at 101 → equity +2."}},
	"options": {
		{"Contract payoffs", "This original curriculum uses European options, no dividends or rates, and a 100-share multiplier unless stated.", "Call payoff = max(S−K,0)", "S=110,K=100 → call payoff 10 per share."},
		{"Intrinsic value", "Premium equals intrinsic value plus time value before expiry.", "Time value = premium − intrinsic", "Call premium 14, intrinsic 10 → time value 4."},
		{"ATM straddle heuristic", "The displayed rule is an approximation, not an exact option model. Volatility is a decimal and T is years.", "Straddle ≈ 0.8 S σ √T", "S=100,σ=.20,T=.25 → approximately 8."},
		{"Put-call parity", "At zero rates and dividends, equal-strike European options obey the stated parity.", "C − P = S − K", "S=105,K=100,C=8 → P=3."},
		{"Conversions", "Compare executable legs with the parity identity. This drill supplies frictionless prices explicitly.", "Parity residual = C − P − (S−K)", "C=8,P=4,S−K=3 → residual +1."},
		{"Delta shares", "A contract delta becomes underlying-share exposure after applying the multiplier.", "Share delta = contracts × multiplier × delta", "10 calls ×100×.6 = +600 shares."},
		{"Delta hedging", "Add existing stock before computing the neutralizing trade. The fill changes actual stock exposure.", "Hedge = −option delta shares − existing shares", "+600 option delta and −100 stock → sell another 500 shares."},
		{"Book Greeks", "Supplied Greeks have explicit units. This curriculum measures vega per one volatility percentage point.", "Book Greek = Σ contracts × multiplier × per-share Greek", "2 contracts with delta .5, multiplier100 → delta100."},
		{"Local P&L", "A Greek approximation is local and can differ from full repricing. Theta is per day here.", "ΔV ≈ Δ ΔS + ½ Γ ΔS² + vega Δvol_points + theta Δdays", "Delta .5, spot move2, other terms0 → about1 per share."},
		{"Quote and settle", "Original case fills are declared customer orders with fixed reservation values. Equity includes option and stock marks.", "Equity = cash + contracts ×100× option mark + shares×spot", "Buy option at7 then mark8 → +100 per contract before hedge."}},
	"futures": {
		{"Spot and maturity", "Time is in years. The default model is simple carry; continuous carry is a separate convention.", "T = days / declared year basis", "Half a year is T=.5."},
		{"Fair futures value", "Simple annual net carry is supplied as a decimal in this original preset.", "F* = S(1+cT)", "S=100,c=.04,T=.5 → F*=102."},
		{"Basis sign", "This curriculum fixes basis as futures minus spot.", "Basis = F−S", "F=103,S=100 → basis3."},
		{"Financing and dividends", "The simplified carry input nets financing against yield. Do not subtract dividends twice.", "c = financing − yield", "5% financing and2% yield →3% net carry."},
		{"Commodity carry", "Storage raises fair value; convenience yield lowers it in a stated model.", "c = r + storage − convenience yield", "4%+2%−1% →5% net carry."},
		{"Cash and carry", "Sell a rich future at its bid and buy the spot at its ask, financing it to maturity.", "Lock = future bid − financed spot ask − fees", "Future104, financed spot102 →2 per unit."},
		{"Reverse arbitrage", "A cheap future may support the reverse trade only when shorting and financing are feasible.", "Buy future + short spot", "A midpoint discrepancy alone does not prove executable profit."},
		{"Contract multiplier", "A one-point move is scaled by the monetary multiplier and contract count.", "P&L = contracts × multiplier × ΔF", "2 contracts ×50 ×3 points =300."},
		{"Hedge and quote", "Match economic units before trading a basket or ETF against futures exposure.", "Hedge units = −contracts × units per contract", "Short3 contracts representing50 units → buy150 units."},
		{"Settlement", "Our case uses a fully paid synthetic forward claim to share cash accounting with the other cases; margin is outside this preset.", "Equity = cash + claim units × mark + hedge units × spot", "A separate exchange-futures model must use variation margin instead."}},
	"prediction": {
		{"Binary contracts", "Each Yes contract pays exactly1 if its fully specified event occurs, otherwise0.", "Fair Yes = p", "Probability .65 → fair Yes .65."},
		{"Complement prices", "Yes and No must refer to exactly complementary settlement events.", "Fair No = 1−p", "p=.65 → fair No .35."},
		{"Expected profit", "Use executable ask for a purchase, with fees supplied separately.", "Buy Yes EV = p − ask − fees", "p=.6,ask=.54,fees0 → EV .06."},
		{"Evidence updates", "Likelihood ratios multiply odds, not probabilities.", "Posterior odds = prior odds × likelihood ratio", "Prior .4 and LR3 → odds2 → p=2/3."},
		{"Exhaustive locks", "A complete mutually exclusive outcome set pays1 in total. Settlement and atomic fills must match.", "Lock = 1 − Σ outcome asks − fees", ".46+.50=.96 → lock .04."},
		{"Cross-venue locks", "Equal labels alone do not establish identical resolution. This original exercise explicitly assumes identical contracts.", "Lock = executable sale bid − purchase ask", "Buy .48 on A, sell .53 on B → .05 before fees."},
		{"Inventory skew", "Inventory changes risk and can justify a shifted quote, while the mathematical event probability remains separate.", "Inventory exposure = Yes units − No units", "Long100 Yes can be reduced by selling Yes or buying a true complement."},
		{"Collateral", "The original case uses fully paid long contracts and enough cash for declared fills; real venues can require different collateral.", "Maximum long loss = units × purchase price", "Buy20 Yes at.4 → maximum purchase loss8."},
		{"Probability quality", "A realized win is not proof of a good forecast. Report calibration separately over many events.", "Brier loss = (p−outcome)²", "p=.7,outcome1 → loss .09."},
		{"Settlement", "The terminal event tape is frozen. Payoffs, cash and final inventory are reported separately from knowledge accuracy.", "Terminal equity = cash + contracts × outcome", "Buy10 at.6 then event resolves Yes → profit4."}},
}

func trainerNumberQuestion(prompt string, answer float64, explanation string) *assessmentQuestion {
	return &assessmentQuestion{Prompt: prompt, ResponseType: "numeric", Answer: strconv.FormatFloat(answer, 'g', 12, 64), AbsTolerance: "0", RelTolerance: "0", Explanation: explanation, Family: "trading"}
}
func (g *trainerGame) tradingTrial(t trainerTrial, rng *engine.RNG) trainerTrial {
	id, mode := g.cfg.TrainerID, g.cfg.Mode
	i := t.Index
	v := float64(rng.Range(2, 9))
	n := float64(rng.Range(2, 8))
	q := trainerNumberQuestion("", 0, "")
	if mode == "case" {
		return g.tradingCase(t)
	}
	lessonIndex := i % 10
	lesson := trainerLessons[id][lessonIndex]
	t.Public["lesson"] = lesson
	if mode == "learn" {
		mode = map[string]string{"etf": "price", "options": "pricing", "futures": "fair_value", "prediction": "probability"}[id]
		t.Public["lesson_index"] = lessonIndex + 1
		t.Public["simulator"] = map[string]any{"domain": id, "editable": true, "model": "declared frictionless baseline"}
	}
	switch id {
	case "etf":
		switch mode {
		case "price":
			q = trainerNumberQuestion(fmt.Sprintf("An ETF holds %.0f A at 40 and %.0f B at 30, divisor 2. What is NAV?", v, n), (40*v+30*n)/2, "Sum share holdings × price, then divide by2.")
		case "hedge":
			q = trainerNumberQuestion(fmt.Sprintf("You are short %.0f ETF units, each holding %.0f A shares (divisor1). How many A shares must you buy?", n, v), n*v, "A short ETF requires a long component hedge.")
		case "arbitrage":
			q = trainerNumberQuestion(fmt.Sprintf("Executable basket bids total110. ETF ask is %.0f, conversion divisor1, no fees, atomic conversion. Gross edge from buying one ETF?", 100+v), 10-v, "Sell the redeemed basket at executable bids, subtract the ETF ask.")
		}
	case "options":
		switch mode {
		case "pricing":
			q = trainerNumberQuestion(fmt.Sprintf("Use the stated ATM-straddle heuristic 0.8 S σ √T: S=100, σ=%.2f, T=.25. Approximate straddle premium per share?", v/20), 2*v, "0.8 ×100 ×σ ×.5. This is a heuristic.")
		case "parity":
			q = trainerNumberQuestion(fmt.Sprintf("European options, zero rates/dividends: S=105,K=100,C=%.0f. What put premium satisfies C−P=S−K?", 5+v), v, "P=C−(S−K).")
		case "greeks":
			q = trainerNumberQuestion(fmt.Sprintf("Long %.0f calls, multiplier100, delta .6, no existing stock. Enter signed stock hedge (buy positive).", n), -60*n, "Hedge shares = −contracts×100×.6.")
		}
	case "futures":
		switch mode {
		case "fair_value":
			q = trainerNumberQuestion(fmt.Sprintf("Spot100, simple net annual carry %.0f%%, T=.5 years. Fair future?", v), 100+v/2, "F*=S(1+cT), using simple carry.")
		case "carry":
			q = trainerNumberQuestion(fmt.Sprintf("Annual financing %.0f%%, dividend yield2%%. Net simple carry, in percentage points?", v+2), v, "Subtract dividend yield from financing.")
		case "arbitrage":
			q = trainerNumberQuestion(fmt.Sprintf("Future bid %.0f, financed executable spot ask at maturity102, fees0. Cash-and-carry edge per unit?", 102+v), v, "Sell future bid and buy/finance spot ask.")
		}
	case "prediction":
		switch mode {
		case "probability":
			q = trainerNumberQuestion(fmt.Sprintf("Binary Yes pays1, model probability %.2f, purchase ask .20, fees0. Expected profit per Yes?", v/10), v/10-.2, "EV=p−ask. Enter a decimal or exact fraction.")
		case "locks":
			q = trainerNumberQuestion(fmt.Sprintf("Yes and No are exact complements, atomic asks .40 and %.2f, fees0. Guaranteed profit from buying one of each?", .5-v/100), .1+v/100, "A complement pair pays1; subtract both asks.")
		case "quoting":
			q = trainerNumberQuestion(fmt.Sprintf("Fair Yes probability %.2f. What is the fair No price?", v/10), 1-v/10, "Complement fair prices sum to1.")
		}
	}
	// Learn quick checks follow each lesson's concept using fixed, inspectable examples.
	if g.cfg.Mode == "learn" {
		q = trainerLessonCheck(id, lessonIndex)
		t.Public["lesson"] = lesson
	}
	q.ID = t.ID
	if g.cfg.AnswerFormat == "choice" {
		makeAssessmentChoices(rng, q)
	}
	t.Question = q
	return t
}
func trainerLessonCheck(id string, i int) *assessmentQuestion {
	prompts := map[string][]string{
		"etf":        {"2 A at40 +1 B at30, divisor1: NAV?", "Price20 ×5 million shares: market cap in millions?", "Basket450, divisor5: NAV?", "Weight2, stock shock+3, divisor1: NAV change?", "Short5 ETF, weight2 A, divisor1: A shares to buy?", "3 contracts ×10 units ×weight2: exposure shares?", "Basket bids110, ETF ask108, no fees: buy-side edge?", "Convert one108 ETF into executable110 basket: profit?", "Bid99.5, ask100.5: spread width?", "Buy1 ETF at99, final mark101, no fees: profit?"},
		"options":    {"S110,K100: call payoff per share?", "Premium14, intrinsic10: time value?", "0.8×100×.2×sqrt(.25): approximate straddle?", "S105,K100,C8, zero rates/dividends: put price?", "C8,P4,S−K3: parity residual?", "10 calls, delta.6, multiplier100: share delta?", "Option delta+600, existing stock−100: signed additional stock hedge?", "2 contracts, delta.5, multiplier100: share delta?", "Delta.5 ×spot move2, other Greek terms0: approximate per-share P&L?", "1 option contract bought7 and marked8, multiplier100: profit?"},
		"futures":    {"Half a year, in years?", "S100,c.04,T.5 simple carry: fair future?", "F103,S100, basis=F−S: basis?", "5% financing minus2% yield: net carry percentage points?", "4% financing +2% storage −1% convenience: percentage points?", "Future bid104, financed spot102: edge?", "Buy future99, effective short-spot proceeds102: reverse edge?", "2 contracts ×50 multiplier ×3-point move: P&L?", "Short3 contracts representing50 units: hedge units to buy?", "Synthetic claim bought102, final mark105: per-unit profit?"},
		"prediction": {"Yes probability.65: fair Yes?", "Yes probability.65: fair No?", "p.6, ask.54, fees0: EV?", "Prior.4, likelihood ratio3: posterior probability?", "Complement asks.46+.50: guaranteed lock?", "Identical contracts: buy.48, sell.53, fees0: lock?", "Long100 Yes, buy30 No: net directional units?", "20 Yes purchased.4: maximum purchase loss?", "p.7,outcome1: Brier loss?", "10 Yes bought.6, event resolves1: profit?"}}
	answers := map[string][]string{"etf": {"110", "100", "90", "6", "10", "60", "2", "2", "1", "2"}, "options": {"10", "4", "8", "3", "1", "600", "-500", "100", "1", "100"}, "futures": {"1/2", "102", "3", "3", "5", "2", "3", "300", "150", "3"}, "prediction": {"13/20", "7/20", "3/50", "2/3", "1/25", "1/20", "70", "8", "9/100", "4"}}
	return &assessmentQuestion{Prompt: prompts[id][i], ResponseType: "numeric", Answer: answers[id][i], Explanation: trainerLessons[id][i].Formula + ". " + trainerLessons[id][i].Example, AbsTolerance: "0", RelTolerance: "0", Family: "trading"}
}
func (g *trainerGame) tradingCase(t trainerTrial) trainerTrial {
	id, i := g.cfg.TrainerID, t.Index
	t.Public["case_screen"] = i + 1
	t.Public["case_screens"] = 9
	t.Public["case_rules"] = "Original nine-screen case. +1 per objective, 0 otherwise; P&L separate. Fully paid synthetic claims, no fees, atomic fills, no borrowing limit. Quotes: bid<ask, size1..100; fair must be strictly inside and width≤declared limit. A frozen customer may fill at its reservation."
	fair, spot, mult, delta := 100.0, 100.0, 1.0, 1.0
	context := "ETF: one A share per unit, divisor1, A=100."
	if id == "options" {
		fair, spot, mult, delta = 8, 100, 100, .6
		context = "European ATM straddle proxy: S100, σ.2,T.25, supplied fair premium8 from heuristic. Multiplier100, supplied delta.6. This proxy's later marks are supplied, not a full option repricer."
	}
	if id == "futures" {
		fair, spot = 102, 100
		context = "Fully paid synthetic forward claim: spot100, simple net carry.04, T.5, fair102. This is not an exchange margin simulation."
	}
	if id == "prediction" {
		fair, spot, delta = .6, 1, 0
		context = "Yes contract pays1/0; model probability.6. Reference hedge token has fixed price1 and zero contract delta. Frozen terminal event resolves Yes."
	}
	t.Public["context"] = context
	t.Secret["fair"], t.Secret["spot"], t.Secret["multiplier"], t.Secret["delta"] = fair, spot, mult, delta
	label := []string{"Define units", "Value the contract", "Read the executable edge", "Translate exposure", "Plan the hedge", "Execute a stock/token hedge", "Quote to the customer", "Mark the shock", "Reconcile terminal equity"}[i]
	t.Public["screen_title"] = label
	q := trainerNumberQuestion("", 0, "")
	switch i {
	case 0:
		q = trainerNumberQuestion("What is the supplied contract multiplier?", mult, "Use the declared contract units.")
	case 1:
		q = trainerNumberQuestion("What is the initial fair premium/claim value per unit?", fair, "Apply the declared valuation model.")
	case 2:
		discount := 1.0
		if id == "prediction" {
			discount = .05
		}
		t.Public["ask"] = fair - discount
		q = trainerNumberQuestion(fmt.Sprintf("An atomic purchase of ONE contract is available at %.4g per unit, no fees. What is gross edge per contract at current fair?", fair-discount), discount*mult, "(fair−ask)×multiplier. The offered trade executes after this screen independent of your answer.")
		t.Secret["purchase_price"] = fair - discount
	case 3:
		q = trainerNumberQuestion("After the purchase, what is the contract's supplied share delta (position × multiplier × delta)?", mult*delta, "Multiply position1, contract multiplier, and supplied delta.")
	case 4:
		q = trainerNumberQuestion("Enter the signed additional hedge units needed to neutralize that delta (buy positive).", -mult*delta, "Hedge=−contract delta−existing hedge units. This screen plans; next screen executes.")
	case 5:
		q = trainerNumberQuestion("Execute a signed stock/token hedge now. Enter units to trade; your actual input changes cash and exposure at supplied spot.", -mult*delta, "Your submitted hedge executes once at the stated spot.")
		t.Secret["execute_hedge"] = true
	case 6:
		q = nil
		limit := 2.0
		if id == "options" {
			limit = 1
		}
		if id == "prediction" {
			limit = .04
		}
		t.Public["quote_prompt"] = fmt.Sprintf("Enter a two-way quote around fair, width ≤ %.4g, size1..100. A frozen customer buys 1 contract if ask ≤ fair + half that limit; otherwise it passes. Quote objective earns1 if strictly straddling fair with allowed width.", limit)
		t.Public["max_spread"] = limit
		t.Secret["customer_reservation"] = fair + limit/2
	case 7:
		shock := 2.0
		if id == "prediction" {
			shock = .1
		}
		t.Public["mark_shock"] = shock
		q = trainerNumberQuestion(fmt.Sprintf("The contract mark increases by %.4g per unit, holding supplied stock/token price constant. What is contract mark P&L for your current position?", shock), g.book.Position*mult*shock, "Position×multiplier×mark change. Hedge price is unchanged for this explicitly supplied scenario.")
		t.Secret["new_mark"] = fair + shock
	case 8:
		terminal := fair + 2
		if id == "prediction" {
			terminal = 1
		}
		equity := g.book.Cash + g.book.Position*mult*terminal + g.book.Shares*spot
		t.Public["terminal_mark"] = terminal
		q = trainerNumberQuestion(fmt.Sprintf("Terminal mark is %.4g per unit, hedge/token price %.4g. Using your actual book, enter final equity (cash + position×multiplier×mark + hedge units×hedge price).", terminal, spot), equity, "Reconcile all cash flows and terminal positions. This marked equity is P&L because the case started at zero equity.")
		t.Secret["new_mark"] = terminal
	}
	if q != nil {
		q.ID = t.ID
		q.AbsTolerance = "0.000000001"
		q.RelTolerance = "0"
		t.Question = q
	}
	return t
}
func (g *trainerGame) applyCaseAnswer(correct bool, result map[string]any) {
	t := g.current()
	if g.cfg.Mode != "case" || g.profile.Category != "Trading" {
		return
	}
	fair := trainerFloat(t.Secret["fair"])
	g.book.Delta = trainerFloat(t.Secret["delta"])
	g.book.Multiplier = trainerFloat(t.Secret["multiplier"])
	g.book.Spot = trainerFloat(t.Secret["spot"])
	if price, ok := t.Secret["purchase_price"]; ok {
		p := trainerFloat(price)
		g.book.Position++
		g.book.Cash -= p * g.book.Multiplier
		g.book.Ledger = append(g.book.Ledger, map[string]any{"type": "atomic_purchase", "units": 1, "price": p, "cash": g.book.Cash})
	}
	if t.Secret["execute_hedge"] == true {
		answer, ok := parseExactNumber(trainerString(result["response"]))
		if ok {
			v, _ := answer.Float64()
			g.book.Shares += v
			g.book.Cash -= v * g.book.Spot
			g.book.Ledger = append(g.book.Ledger, map[string]any{"type": "hedge", "units": v, "price": g.book.Spot, "cash": g.book.Cash})
		}
	}
	if mark, ok := t.Secret["new_mark"]; ok {
		fair = trainerFloat(mark)
	}
	g.book.Fair = fair
	result["equity"] = g.book.Cash + g.book.Position*g.book.Multiplier*fair + g.book.Shares*g.book.Spot
	result["residual_share_delta"] = g.book.Position*g.book.Multiplier*g.book.Delta + g.book.Shares
}
func (g *trainerGame) applyTrainerQuote(p trainerPayload, result map[string]any) string {
	t := g.current()
	if g.cfg.Mode != "case" || t.Index != 6 {
		return "UNKNOWN_ACTION"
	}
	bid, ok := parseExactNumber(p.Bid)
	if !ok {
		return "INVALID_QUOTE"
	}
	ask, ok := parseExactNumber(p.Ask)
	if !ok || bid.Sign() < 0 || bid.Cmp(ask) >= 0 || p.Size < 1 || p.Size > 100 {
		return "INVALID_QUOTE"
	}
	b, _ := bid.Float64()
	a, _ := ask.Float64()
	if !finiteTrainer(a) || !finiteTrainer(b) || a > 1e9 {
		return "INVALID_QUOTE"
	}
	if g.cfg.TrainerID == "prediction" && a > 1 {
		return "INVALID_QUOTE"
	}
	fair := trainerFloat(t.Secret["fair"])
	width := trainerFloat(t.Public["max_spread"])
	correct := b < fair && a > fair && a-b <= width+1e-12
	result["correct"], result["points"], result["status"] = correct, 0, "wrong"
	if correct {
		result["points"], result["status"] = 1, "correct"
	}
	result["quote"] = map[string]any{"bid": b, "ask": a, "size": p.Size}
	if a <= trainerFloat(t.Secret["customer_reservation"]) {
		g.book.Position--
		g.book.Cash += a * g.book.Multiplier
		g.book.Ledger = append(g.book.Ledger, map[string]any{"type": "customer_buy", "units": 1, "price": a, "cash": g.book.Cash})
		result["fill"] = "customer buys1 at your ask"
	} else {
		result["fill"] = "pass"
	}
	g.book.Fair = fair
	g.completeTrial(result)
	return ""
}
