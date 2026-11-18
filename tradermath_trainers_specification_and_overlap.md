# Quant Practice trainers: inputs, outputs, reproducibility and overlap

Research and implementation date: November 17, 2026. Catalogue reference: [Tradermath Trainers](https://www.tradermath.org/trainers). Companion specifications: [Market Making Games](market_making_games_specification.md) and [Tradermath assessments](tradermath_assessments_specification_and_overlap.md).

This document covers all 22 catalogue trainers and the modes identified in the supplied research. It also specifies the original implementations on Quant Practice's `/trainer/` page. The home page links to `/games/`, `/assessment/` and `/trainer/`; `/trainers/` redirects to the trainer page.

**Published** means described in the supplied public-page research or a linked public reference. **Original implementation** means a decision made for this repository. **Unknown** means the inspected public evidence does not establish the behavior. The repository contains original questions and simplified market cases. It does not reproduce premium question banks, psychometric norms or undisclosed employer scoring.

The catalogue count is 3 numerical, 4 trading, 1 probability/statistics, 1 estimation and 13 cognitive trainers. Premium modes described in the research are available here as original practice modes. This does not mean their hidden source algorithms have been recovered. The local Fermi library contains 24 original frozen questions; the public source's reported 158-item library is not the local bank.

## 1. Complete inventory

| ID | API trainer_id | Modes | Published/session structure | Quant Practice primary measure |
|---|---|---|---|---|
| T01 | math | practice; typed/choice | 1–10 min; default 2 | Original net/min |
| T02 | sequence | numbers, letters; typed/choice | 1–15 min; default 1; skips | Original net/min |
| T03 | fractions | practice, test | Practice untimed; Test 1/2/5/10 min | Correct − 2×wrong |
| T04 | etf | learn, price, hedge, arbitrage, case | 10 lessons; 10-item drills; 9-screen 45-min case | Objective score; equity separate |
| T05 | options | learn, pricing, parity, greeks, case | Same lesson/drill/case structure | Objective score; equity separate |
| T06 | futures | learn, fair_value, carry, arbitrage, case | Same lesson/drill/case structure | Objective score; equity separate |
| T07 | prediction | learn, probability, locks, quoting, case | Same lesson/drill/case structure | Objective score; equity separate |
| T08 | probability | practice; typed/choice | 1–15 min; default 3 | Correct count and accuracy |
| T09 | fermi | library | Search, hint, scratchpad, estimate, solution | Factor error; original within-10 flag |
| T10 | task-switch | classic, realistic | 1–10 min; default 3 | Net/min, switch RT |
| T11 | response-conflict | flanker, simon | 1–10 min; default 1 | Net/min, congruency RT |
| T12 | pincode | repeat, reverse, sort | 1–10 min; default 1; fixed/adaptive span | Net/min, span trajectory |
| T13 | cube | mixed; seven face-style settings | 1–10 min; default 2; five choices | Net/min |
| T14 | code-match | standard, pressure | 30 separately timed items | Correct/30, errors and omissions |
| T15 | grid-recall | standard, sequential | 20 adaptive rounds | Exact rounds, cell recall |
| T16 | intercept | standard, occluded | 30 balls | Hits/30, arrival error |
| T17 | hold-fire | standard, switching | 60 stimuli | Balanced accuracy |
| T18 | target-number | practice, timed | 10 puzzles; 45/18 sec each | Original solves−skips |
| T19 | pump | manual, stakes | 30/20 balloons; self paced | Banked cents and risk summaries |
| T20 | skyline | open, capped | 10 untimed levels | Perfect solves; excess moves |
| T21 | mystery | standard, deep | 5 untimed rounds | Guess counts and information comparison |
| T22 | sweet-spot | standard; device label | 2 min; 2–6 simultaneous dials | Hits−wrong clicks−expiries |

A mode is a configuration of a shared engine rather than a duplicated game. The metadata lives in `backend/internal/games/trainer_profiles.json`, imported by both the Go API and Astro page. Counts and durations remain visible with each result. Scores across different modes, difficulty settings or devices should not be pooled.

## 2. Shared executable contract

### 2.1 Configuration and creation

Create an individual session using the existing session API:

```json
{
  "game": "trainer",
  "players": ["alice"],
  "seed": 42,
  "config": {
    "trainer_id": "math",
    "mode": "practice",
    "duration_ms": 120000,
    "answer_format": "open",
    "difficulty": 2,
    "family": "mixed",
    "operations": ["+", "-", "*", "/", "%"],
    "digits": 2,
    "places": 4,
    "fixed_span": 0,
    "max_span": 8,
    "face_style": "mixed",
    "pointer_device": "mouse",
    "item_id": ""
  }
}
```

Send that object to `POST /api/sessions`. The response includes `session_id`, players, seed and an initial snapshot. `GET /api/trainers` returns all 22 profiles. `GET /api/trainers/library` returns only the Fermi item IDs, prompts and units, without reference answers.

Defaults are resolved server-side. Unknown trainer/mode combinations are rejected. Difficulty is 1–3; operand digits 1–4. Math/probability family names are mixed, integer, decimal, fraction, percentage, counting, bayes and expectation; the UI offers relevant families for each trainer. Operations are a nonempty selection of `+ - * / %`. Numeric settings belonging to other trainers are retained as resolved metadata but do not change their rules.

Timed trainers enforce the published minute ranges. Fractions Test permits only 60,000, 120,000, 300,000 or 600,000 milliseconds, and forces five decimal places. Trading modes force 10 lessons/drill items, or 9 screens and 2,700,000 ms for a case. Pump forces 30 Manual or 20 Stakes rounds. Sweet Spot forces 120,000 ms. Fermi is one selected question per session. Other finite cognitive trainers default to their listed round counts; the API can accept a finite practice override up to 128, which must stay attached to the result. The page uses the listed defaults.

### 2.2 Runtime phases and clock

The state machine is:

`INSTRUCTIONS → READY → ENCODE → MASK → RESPONSE → FEEDBACK → READY … → DONE`.

Non-memory questions skip ENCODE/MASK. Intercept inserts ANIMATE after a shot, allowing the projectile to travel before feedback. Learn, Fermi, Pump, Skyline, Mystery and untimed fraction practice require an explicit Next after feedback. Timed drills and the finite timed cognitive games can advance after an original 450-ms feedback interval. Starting the next trial requires PRESENT, preventing a response to an item that was never presented.

`BEGIN` starts the session. A monotonic logical elapsed clock represents presentation and input time. Each action supplies `elapsed_ms`, and the runtime accepts only nondecreasing values no more than 2 seconds ahead of server wall time. A whole-session deadline is additionally anchored to server start time, so refreshing cannot restart it. Item actions at or after an item deadline are too late: expiry is processed before the answer.

The browser measures `reaction_time_ms` from its actual response-phase render, rather than substituting network arrival latency. DISPLAY records presentation events; a response phase presented more than 150 ms after its logical onset marks the attempt as unsuitable for timing comparison. RESUME catches the logical clock up to elapsed server time and marks interruption. FOCUS_LOST also marks interruption. Time keeps running. Explicitly ending an unfinished session marks it as stopped practice. Scores remain available, with the interruption flag.

The original web implementation exchanges memory phases with the server at roughly 90-ms intervals, motion at 180 ms and ordinary drills at 400 ms; motion itself renders using requestAnimationFrame and elapsed time. Network delays and frame scheduling can affect actual exposures. Client timing is bounded and replayed, **not independently authenticated**. This is a practice tool, not a controlled psychometric laboratory or a tamper-proof benchmark. A rigorous experiment must preserve actual frame/device conditions and implement a calibrated presentation protocol.

### 2.3 Observation and action schemas

A representative response snapshot is:

```json
{
  "profile": {"id":"response-conflict","score_kind":"net_rate"},
  "config": {"trainer_id":"response-conflict","mode":"flanker"},
  "phase": "RESPONSE",
  "trial_id": "trial-001",
  "kind": "response-conflict",
  "elapsed_ms": 850,
  "onset_ms": 100,
  "response_onset_ms": 100,
  "time_remaining_ms": 59150,
  "level": 2,
  "scene": {
    "cue":"Answer the CENTRAL letter",
    "display":"HHSHH",
    "mapping":"H / Circle → Left · S / Square → Right"
  },
  "move_state": {"moves":0,"pumps":0,"first_move_ms":null},
  "feedback": null,
  "comparable": true
}
```

```json
{
  "action_id": "action-unique-23",
  "player_id": "alice",
  "action": "RESPOND",
  "payload": {
    "trial_id":"trial-001",
    "elapsed_ms":850,
    "reaction_time_ms":750,
    "side":"RIGHT"
  }
}
```

Send actions to `POST /api/sessions/{id}/action`. The session store deduplicates action IDs. The engine validates current trial, phase, input format and deadline. Rejections include INVALID_FORMAT, INVALID_CLOCK, STALE_TRIAL, WRONG_PHASE, SKIP_LOCKED, ILLEGAL_MOVE and ILLEGAL_ARITHMETIC. Rejected moves do not consume tiles, move floors or score again. A rejected answer does not pause or restart time.

Use `GET /api/sessions/{id}/state?player=alice` to read the current observation. Question keys, future trials, memory targets after encoding and unrevealed thresholds stay evaluator-side. During an encoding phase only the currently visible digit/cells are exposed, not the complete future sequence. At response time those encoding values disappear from the snapshot and rendered UI. Completed-item feedback can reveal that item's solution; the next active trial never includes future solutions.

### 2.4 Exact answers and reporting

Numeric answers use the shared exact rational parser: integers, finite decimals, fractions and bounded scientific notation normalize to rational values. Equivalent `0.75`, `3/4` and `6/8` compare equal. Denominator zero, NaN, infinity, arbitrary expressions and ambiguous thousands separators are rejected. Letter tokens are normalized by the shared sequence parser. Most questions have zero tolerance; case questions declare a 1e−9 absolute tolerance to accommodate displayed book arithmetic. Fraction-digit answers are strings, preserving zeros and their fixed length.

`GET /api/sessions/{id}/results` and `/tape` return 409 before completion. Results include resolved settings, score kind, raw components, accuracy, skips, timeouts, elapsed time, adaptation levels, interruption status, per-trial review and book/ledger where relevant. Attempted accuracy is correct/(correct+wrong), zero for an empty denominator. Timeouts/skips are separate. Hold Fire supplies balanced accuracy; Sweet Spot supplies dial hit rate and click accuracy; Pump supplies bank and adjusted pumps. These meanings remain distinct even though a common envelope is used.

### 2.5 Frozen replay

A completed tape contains runtime version `trainer-runtime-v1`, bank version `trainer-scenarios-v1`, renderer version `trainer-renderer-v1`, PRNG `splitmix64-v1`, seed, profile/config, every generated trial including hidden evaluator fields, SHA-256 scenario hash, action requests with accepted/rejected receipts, server-relative timestamps, final clock and canonical results.

Generation uses streams keyed by seed, trainer ID, trial index and adaptive level; separate keyed streams drive session schedules and dials. A seed alone is insufficient if rules or generation order change. Replay consumes the frozen scenarios and recorded actions, validates the manifest and receipts, and requires identical canonical results. The browser exports the JSON tape and accepts it for verification at `POST /api/trainers/replay`. The CLI supports both assessment and trainer tapes:

```sh
cd backend
go run ./cmd/replay path/to/trainer-tape.json
```

A hash detects accidental scenario edits against that tape, not authorship or trusted execution. Completed exports intentionally contain past secrets for review/reproduction; they are never the live candidate payload.

## 3. Numerical, trading, probability and estimation trainers

### T01. Math Trainer

Reference: [Math dashboard](https://www.tradermath.org/tradermath-math-trainer/dashboard).

Published: configurable arithmetic with operation/difficulty/operand settings, integer/decimal/fraction work, open or multiple-choice responses and a 1–10-minute clock, default 2. The exact public score/min formula is unresolved in the supplied research.

Original input: an exact expression AST represented by its prompt, answer key and generator parameters; configuration selects operations, 1–4 operand digits, number family and answer interface. Mixed mode chooses integer, decimal or fraction families; a selected percentage operation uses a percentage question. Integer/decimal operands are bounded by the configured digits; decimals have one place. Fraction items share the original Math Challenge fraction generator and filter to allowed operations, rather than scaling denominators by the operand-digit setting.

Action: ANSWER with a rational string, or a stable choice ID. Exactly one answer grades and advances. The four-choice interface removes equivalent duplicate options. SKIP advances without points and records a skip. Division never uses a zero divisor.

Output: correctness, answer, calculation, raw correct−wrong, net divided by configured minutes, accuracy and trial timing. This net rule is an original practice choice, not a recovered Tradermath penalty. Example: `37×24` → `888` → +1; 42 correct and 6 wrong in 2 minutes gives 36 raw and 18/min. Unknown: source distributions, exact difficulty mapping, rate denominator and original skip constraints.

### T02. Sequence Trainer

Reference: [Sequence dashboard](https://www.tradermath.org/tradermath-sequence-trainer/dashboard).

Published: number/letter sequences, open/choice answers, configurable difficulty and skips; 1–15 minutes, default 1. Arithmetic, geometric and Fibonacci patterns are described. Skips do not score or consume an attempted-question cap.

Original input: visible tokens plus a stored intended-rule model. Number mode reuses arithmetic, geometric, growing-difference and interleaved families, with occasional Fibonacci-style recurrence at difficulty 2/3. Letter mode uses an explicit alphabet progression. The frozen manifest disambiguates the intended rule; it does not assert uniqueness across all possible functions matching finite data.

Actions: ANSWER the next term or SKIP. Numeric equivalence is exact; letter casing normalizes. There is no fixed attempted-item cap in the timed page mode. Skipping keeps the session clock running. Output: completed sequence, explanation, correct/wrong, skip count, level and time. Original score is net/min. Example `2,5,10,17,26,?` has successive odd differences and intended answer37. Unknown: full source grammar, ambiguity review, penalty and original difficulty distributions. Maven anomaly identification is a related, separately configured assessment task, not this next-term prompt.

### T03. Fractions to Decimals

Reference: [Fractions dashboard](https://www.tradermath.org/fractions-to-decimals/dashboard).

Published: untimed Practice with 2–5 decimal places; timed Test with five digits after a fixed `0.`, durations 1/2/5/10 minutes; Test correct +1/wrong −2 and raw net score.

Original input: proper positive fraction `n/d`, with d from3–97 and n from1..d−1, requested places and mode. Practice uses a default 20-item local session. The generator excludes values whose rounded result requires an integer part1. The half-up convention is explicit: compute integer quotient and remainder of n×10^places divided by d; increment if twice the remainder is at least d. Avoid binary floating rounding.

Action: ANSWER exactly `places` digits, without the `0.` prefix. Format errors reject without consuming the item. Practice reveals feedback then waits for NEXT; Test advances after feedback. Output: digit string, rounded expansion and raw net. Practice displays the same original net rule for consistency, although the published penalty is established for Test. Example5/12 at five places accepts `41667`; `41666` is incorrect. 1/8 at two places becomes `13`, not `12`. Unknown: source bank, original tie handling, feedback timing and values near1.

### T04–T07. Shared teaching and case behavior

References: [ETF](https://www.tradermath.org/trainers/etf-strategy/dashboard), [Options](https://www.tradermath.org/trainers/options-strategy/dashboard), [Futures](https://www.tradermath.org/trainers/futures-basis/dashboard), [Prediction](https://www.tradermath.org/trainers/prediction-markets/dashboard).

Published structure: each has ten lessons, three ten-question quick-drill categories and a nine-screen case under one 45-minute deadline. The hidden original case content, simulator code, masteries and execution engines remain unknown.

Original Learn input: a lesson title/body/formula/worked example, an editable calculator and one scored quick check. There are ten distinct concept checks for each domain. Changing calculator inputs does not alter the scored question. Learn uses +1 correct/0 wrong and waits for Next. Completed lessons and results persist for the life of the backend session; the browser also stores a small recent-session result history locally. This is not account-based cross-device course progress.

Original Drill input: ten newly generated problems for the chosen category. Actions answer typed numerics or select options; immediate feedback shows the formula/answer. Each earns +1/0. Numeric task input is static, without hidden order fills. No undisclosed source mastery score is claimed.

Original Case inputs: a declared contract context, one evolving book, nine frozen screens and one persistent 45-minute timer. The case starts with zero cash, zero contracts and zero hedge units. All four cases use a fully paid synthetic claim ledger, without fees, borrowing constraints or exchange margin. This deliberately simplified accounting is explicit in the UI. The knowledge rubric is +1/0 per screen; book equity is separate.

The exact nine-screen protocol implemented here is:

| Screen | Candidate input and action | State transition / objective |
|---|---|---|
| 1 Units | Enter supplied multiplier | Grade unit reading |
| 2 Value | Enter initial fair per-unit value | Grade declared valuation |
| 3 Edge | Enter edge on an offered one-contract atomic purchase | After grading, buy that one contract at the disclosed price regardless of answer |
| 4 Exposure | Enter position×multiplier×supplied delta | Grade exposure |
| 5 Plan | Enter signed hedge units needed to flatten delta | Grade planned hedge, no execution yet |
| 6 Execute | Enter actual signed stock/token trade | Execute that input once at supplied spot, update shares/cash; grade neutralizing size |
| 7 Quote | QUOTE bid/ask/size | Grade quote rubric and apply frozen customer flow |
| 8 Shock | Enter contract-mark P&L for a disclosed mark increase, hedge price fixed | Grade actual current position×multiplier×mark change, then update mark |
| 9 Reconcile | Enter final equity at disclosed terminal marks | Grade cash+marked contracts+marked hedge; finish |

The quote must have finite nonnegative bid<ask and size1–100; prediction ask≤1. Objective: fair strictly inside the quote and width no greater than the displayed limit. A frozen customer buys ONE contract at the ask if ask≤fair+half the displayed width limit; otherwise passes. Size is capacity, not an automatic fill quantity. The book sells one claim on that fill. Quote rules are original, visible and deterministic. There is no invented racing counterparty, partial-fill queue or latency advantage. A wrong but valid hedge input changes actual book equity, rather than being silently replaced by the answer key. Complete ledgers accompany results.

#### T04. ETF Market Making

Published skills: construction, capitalization/basket fair value, shocks, hedge ratios, executable buy/sell/pass arbitrage and two-way quoting.

Original models: NAV=Σ(wᵢSᵢ)/D; ΔNAV=wᵢΔSᵢ/D; component hedge=−ETF units×wᵢ/D. Drills use small positive integer weights, A40/B30 and divisor2 for pricing; explicit short positions for component hedges; basket executable bids110 against an ETF ask100+v, v2–9, for edge calculation. This expresses the correct executable-price direction. Lessons additionally explain capitalization, multipliers, creation/redemption and marked equity.

Editable simulator inputs are A shares, A price, B shares, B price and positive divisor; output is basket NAV. Example2A×40+1B×30→110 at divisor1; +3 A shock→116; short5 ETF requires buying10 A and5 B in a full two-component basket model.

The simplified case uses a one-A-share ETF, divisor1, spot/fair100, multiplier1 and delta1. The screen3 offered ask99 creates +1 contract/−99 cash; hedge−1 at100 adds100 cash. A centered 99.5/100.5 quote fills one sale at100.5, possibly leaving a short hedge. Terminal equity includes that actual hedge. Thus case objective score and equity can differ. Unknown: premium basket tapes, conversion limits, fees, order capacity and source case rubric.

#### T05. Options Market Making

Published skills: intrinsic/time value, approximate ATM straddles, parity/conversions, Greeks, hedging and quoting.

Original assumptions: European exercise, zero rates/dividends where stated, multiplier100, volatility as decimal, T in years. Pricing drill uses ATM straddle≈0.8Sσ√T; parity drill uses C−P=S−K; Greek drill converts contracts×100×delta to hedge shares. The calculator accepts S,σ,T and labels output an approximate straddle. Lessons add intrinsic/payoff, conversions, book Greeks and local delta/gamma/vega/theta P&L, with supplied Greek units.

Example ten long calls at delta.6 and multiplier100 give +600 share delta; sell600 shares. S100,σ.2,T.25 gives heuristic straddle8. S105,K100,C8 gives parity put3 at zero carry.

Case: an expressly supplied straddle-proxy fair8, spot100, multiplier100 and delta.6, with later marks supplied rather than generated by a full option repricer. Ask7 creates one claim costing700; a signed−60-share hedge neutralizes its supplied delta. The quote width limit is1. Outputs include actual shares/cash, residual delta, contract fills and marked equity. Unknown: source option model, Greek unit definitions, exercise/expiry calendar, volatility dynamics, dynamic fills and premium grading. A full options exchange simulation is outside this original case.

#### T06. Futures & Basis

Published skills: fair value, financing/dividends/storage, basis, cash-and-carry/reverse arbitrage, hedge ratios and quoting.

Original inputs: spot, simple annual net carry, T in years and explicitly signed basis F−S. F*=S(1+cT). A lesson can describe continuous carry separately, but the implemented calculator/drill uses simple carry. Carry questions subtract yield or add storage/subtract convenience yield in percentage points. Arbitrage uses a future bid against financed executable spot ask, with omitted frictions explicitly zero.

Example S100,c.04,T.5→102. Selling a future at104 and financing spot to102 gives2 per unit; a stated multiplier50 gives100 per contract. Reverse arbitrage requires available shorting, not just a midpoint spread.

The original case uses a **fully paid synthetic forward claim**, fair102/spot100, multiplier1/delta1. This lets the shared cash ledger value an asset-like claim. It does not reproduce futures variation margin, initial margin or daily settlement. Its quote width limit is2. Outputs include fair/basis reasoning, actual hedge and ledger, objective points and final equity. Unknown: source compounding/calendar conventions, curve generation, financing/shorting limits and exchange mechanics.

#### T07. Prediction Markets

Published skills: Yes/No probabilities, EV, exhaustive-outcome/cross-venue locks, evidence updates, skew/collateral and settlement.

Original inputs: binary payout1, stated p, executable prices, exactly complementary outcomes and zero fees where specified. Fair Yes=p, fair No=1−p; purchase EV=p−ask. Bayes calculator inputs prior p and positive likelihood ratio L, output pL/(1−p+pL). Locks require identical resolution and atomic execution. Lessons explain directional inventory, fully paid long collateral and Brier loss as a different forecast measure.

Example prior.4 with LR3 gives posterior2/3. Buying Yes at.60 yields EV1/15. Complement asks.46/.50 cost.96 for guaranteed payout1, giving.04 only under the stated exhaustive-settlement assumptions.

Case: initial probability/fair.6, multiplier1, supplied delta0, a fixed-price1 reference token, terminal event Yes. The reference token is a bookkeeping example, not a claim that Yes has a tradable zero-risk hedge. Screen3 ask.55 buys one Yes. Quote width limit.04 and ask bounds[0,1] apply. Screen8 mark rises.1; final mark1 settles the supplied event. Outputs separate objective score from realized equity; probability-quality examples are pedagogical, not calibration inferred from a single win. Unknown: premium event week, dispute rules, collateral and execution engine.

### T08. Probability Trainer

Reference: [Probability dashboard](https://www.tradermath.org/tradermath-probability-trainer/dashboard).

Published topics include combinatorics/counting, conditional/Bayes, EV/order statistics and reasoning; open/choice interfaces, adjustable difficulty and a 1–15-minute timer default3. The exact source accuracy/rate composite is unknown.

Original input: a fully stated finite experiment or mathematical model, exact answer and derivation. Foundation items share the original Probability Betting sample-space predicates for dice, coins and card ranks. More advanced templates include urn draws, two-coin Bayes, Bernoulli variance, geometric waits, two-consecutive-head waits, combinations and two-payoff EV. Topic filters choose counting, Bayes or expectation templates. This version does not claim a complete order-statistics generator merely because that topic exists in the source catalogue.

Action ANSWER a rational/choice; SKIP records separately. Correct+1/wrong0 is original; report attempted accuracy and correct/minute separately. No bankroll, stakes or realized random payoff. Example4 red/3 blue, two without replacement→(4/7)(3/6)=2/7. Exact fraction accepted; a rounded decimal requires explicit item tolerance, which these exact items do not silently add. Unknown: source distribution, partial credit, advanced settings and scoring formula.

### T09. Fermi Questions

Reference: [Fermi library](https://www.tradermath.org/fermi-questions).

Published: a searchable library with hints, numeric estimates, scratchpad/solutions and progress; factor-of-ten reasoning. No competitive timer established in the reviewed library.

Original input: choose one of24 local dated/frozen facts or select randomly. Library responses expose only ID/prompt/units. The evaluator retains positive reference value, explanation and reference context. HINT reveals a coarse base-10 magnitude; SAVE_REASONING stores at most4000 characters. ANSWER requires a finite positive number. Feedback reveals the frozen reference and explanation. A local session contains one item; choose another session to continue.

Output: factor error max(x/y,y/x), original within-ten success flag, estimate, reasoning, hint use and descriptive elapsed time. No time penalty or source point table is imported. Example estimate600 liters/day against900 gives factor1.5. A quantity built as2000 customers/day×.3 L/customer has consistent units. Unknown: original full paid content, target uncertainty/ranges, negative-quantity policy and exact progress acceptance. One static fact should not be mistaken for a current live-data query.

## 4. Cognitive trainers

### T10. Task Switch

Reference: [Task Switch](https://www.tradermath.org/task-switch/dashboard).

Published: Classic letter consonant/vowel versus number odd/even; Realistic arrow comparisons/arithmetic parity; active highlighted rule only; Yes/No or Left=No/Right=Yes. +1 correct/−1 wrong; default3 minutes, range1–10.

Original input: Classic displays one of five vowels/five unambiguous consonants and digit1–9. The cue asks vowel or odd. Realistic either compares two two-arrow strings or asks if an integer sum is odd. A keyed random rule schedule is frozen. Response frequency/switch count are measured rather than claimed perfectly balanced. Action RESPOND{yes:boolean}; held-key repeat is ignored in the browser. Output includes Boolean response, rule-switch tag, accuracy and correct-trial RT. Switch cost=median switch RT−median repeat RT; null when either group is empty. Example E/7, letter-vowel cue→Yes; B/7, number-odd cue→Yes. Rule changed while button did not. Unknown: source cue geometry, exact switching ratios and RT scoring.

### T11. Response Conflict

Reference: [Response Conflict](https://www.tradermath.org/response-conflict/dashboard).

Published: Flanker central-letter mapping ignores flankers; Simon shape mapping ignores position; arrows/touch, +1/−1; default1 minute, range1–10.

Original input: Flanker has five letters, H→Left/S→Right, center index2; target and distractor sides are independently sampled. Simon maps circle→Left and square→Right, while location is independent. Action RESPOND{side:LEFT|RIGHT}. Output: net/min, correctness, RT, congruency groups, and conflict cost median incongruent−median congruent. Null measures explicitly indicate no trials in a required group. Example HHSHH→Right. A square appearing left also→Right. Unknown: source shapes/letters, exact ratios, stimulus windows, omissions and feedback schedule. This original mode uses the whole-session clock without an additional item deadline.

### T12. Pincode Game

Reference: [Pincode](https://www.tradermath.org/pincode-game/dashboard).

Published: digits flash individually; Repeat/Reverse/ascending Sort; adaptive starts5, increases/decreases by1 after correct/wrong within4–8, or fixed span; default1 minute, range1–10. Keyboard/numpad, backspace and Enter; +1/−1.

Original input: independent digits0–9 including leading zeros/repetitions. Each digit is visible500 ms then blank150 ms, except no final150-ms gap; mask300 ms. These exact exposures are original. Input is disabled during encoding/mask. Only current digit is in the active observation, then all digits are absent during response. ANSWER is implemented as RESPOND{answer:string} of exact required length. Sort preserves duplicates; reverse preserves zeros. Adaptive span clamps to4..configured max; fixed mode does not adapt. A session expiring with an unsubmitted sequence leaves it ungraded.

Output: shown digits after grading, transformed answer, net/min, current/next level and per-trial history. Example0441→Repeat0441, Reverse1440, Sort0144. Unknown: source digit distribution, exact mask/exposure and incomplete-trial policy.

### T13. Cube Folding

Reference: [Cube Folding](https://www.tradermath.org/cube-folding).

Published: a flat six-face net and five candidate cubes; click/keys1–5, instant feedback, review; mixed colors/shapes/dice/numbers/letters/icons/patterns; default2 minutes, range1–10; (correct−wrong)/minutes.

Original input: net cells with face IDs, symbolic folded face normals, five top/front/right candidates, camera geometry and face-style metadata. The existing assessment cube evaluator enumerates the24 proper rotations, rejects reflections and guarantees one valid candidate. The UI provides a six-face net, isometric cube options and a label/texture key. Face labels are authoritative.

This implementation deliberately uses **rotation-invariant labelled face identities**. Decorative glyphs are not independently graded directional textures; their appearance is a learning aid. Full rotated-dice/icon orientation grading would require oriented texture axes and additional asset validation. The source's mode list does not justify claiming that feature is implemented here.

Action ANSWER{answer:choice_id}. Output correct choice, fold explanation and frozen net/views in review/tape. Example opposite A/C cannot touch. Unknown: source net mixture, oriented texture semantics and feedback timing. Same core geometry can serve Mako assessment practice, with its distinct assessment marking.

### T14. Code Match

Reference: [Code Match](https://www.tradermath.org/code-match/dashboard).

Published:30 rounds,7–10-character reference, four candidates/exactly one match, swaps and changed/lookalike characters; Standard6→3.5 seconds, Pressure5→2. Timeouts separate from wrong answers.

Original input: reference length grows from7 to10 across the round schedule; characters come from a frozen uppercase/digit alphabet. Distractors use adjacent swaps or replacements; duplicates and unchanged swaps are rejected. Deadlines decrease linearly between the stated endpoints, an original interpolation. Font is DM Mono with a monospace fallback; font version/device differences can affect perceptual difficulty.

Action RESPOND{choice:0..3}; numeric keys1–4 map to those indexes. Timeout locks the trial with zero points; a wrong click also earns zero. Original primary score is correct count/30, not net or speed. Output includes selected index, reference/options, correctness and RT. Example A0B5C7D versus AOB5C7D/A0B5C7D/A0BSC7D/A0B5D7C→second. Unknown: exact source mutation distribution, lookalike weighting, decay/mastery formula and feedback timing.

### T15. Grid Recall

Reference: [Grid Recall](https://www.tradermath.org/grid-recall/dashboard).

Published:20 rounds,4×4 grid; briefly shown blue targets, sometimes red decoys, then recall. Two consecutive correct rounds grow span; failure lowers it. Sequential mode presents targets one by one. Red commissions are failures; misses affect recall.

Original input: row-major IDs0–15, target set, disjoint decoy set and current span. Starts3, min1/max12. Standard exposure1000 ms/mask300 ms. Sequential uses500-ms target presentations with150-ms intervening blanks, then300-ms mask. Every second trial has up to2 red decoys, displayed as red with an × so color is not the only cue. Exact sizes/timings/schedule are original.

Action toggle cells in response then RECALL{cells:[...]}. Duplicate/out-of-range IDs reject. Original finalization occurs at Submit, rather than immediately ending on a decoy click. Exact target set with no extras earns1; two exact successes increase span, failure lowers it. Sequential grades a set in any response order; ordered recall is not claimed. Output hits, missing cells, extras, red commissions and level trajectory. Example target{1,6,11}, decoy{4}; response{1,11} gives2 hits/1 omission; any submitted4 causes failure. Unknown: source partial score/commit behavior, bounds, exact exposures and sequential-order policy.

### T16. Intercept

Reference: [Intercept](https://www.tradermath.org/intercept/dashboard).

Published:30 descending balls; four Q/A/P/L cannons with real projectile travel and different intersections; all reach each ball line. Two hits raise speed/tighten tolerance; miss relaxes. Occluded hides final motion; hits/30.

Original geometry:800×520 logical field, ball starts y=−20 on x160/320/480/640, constant downward speed100+3×(level−1) pixels/second. Cannons at(0,500),(0,420),(800,500),(800,420), ray slopes .5,.4,−.5,−.4 using y=cannon_y−slope×dx; projectile speed800 px/s. Level1–18 ensures every first-frame crossing still has a feasible lead. Tolerance=max(35,140−10×(level−1)) ms. Item expires300 ms after the latest crossing. Occluded stops drawing the ball below y240; physics continues unchanged.

Action FIRE{cannon:0..3}; one shot only. Evaluate projectile arrival time against ball crossing time; animate actual travel before feedback. Ideal firing delay=dB/vB−dP/vP. Output signed arrival error in ms, hit/miss, cannon, level and hits/30. Example crossing in1.2 seconds, projectile travel.4→fire after.8 seconds. Two hits increment level; miss decrements, bounded. Unknown: source geometry, accelerations, shot policy, adaptation steps and collision windows.

### T17. Hold Fire

Reference: [Hold Fire](https://www.tradermath.org/hold-fire/dashboard).

Published:60 stimuli, about80% targets; after COLOUR respond to red, after SHAPE respond to circles. Space/tap or withhold. Standard switches about40%, Switching about80%. Two consecutive successes tighten window, errors relax. Balanced accuracy scoring.

Original input:exact48 targets/12 non-targets in a seeded shuffled schedule; switch decisions are sampled with.4/.8 probability, so realized counts vary. Cue300 ms, then stimulus; initial response window depends on level (difficulty defaults2). Deadline=max(350,1200−50×(level−1)) ms, level1–18. Irrelevant shape/color independently varies. Action PRESS once, or no action until expiry. A correct rejection is awarded only when the window closes. Premature phase presses reject; held-space repeats are ignored.

Output hit/miss/false_alarm/correct_rejection, target RT, targets/non-targets, adaptation and balanced accuracy=.5×(hits/targets+correct_rejections/non_targets). Missing either class gives null, not a misleading partial score. Example40/48 hits plus9/12 rejections→79.17%. Press-all and press-none each50%. Unknown: source exact counts, exposure/adaptation steps and anticipatory error semantics. A high target proportion makes ordinary accuracy a poor replacement for balanced accuracy.

### T18. Target Number

Reference: [Target Number](https://www.tradermath.org/target-number/dashboard).

Published:ten solvable four-number puzzles, pairwise +−×÷, integer exact division, negative intermediates allowed; Practice45 seconds/Timed18; skip after10 seconds costs1, timeout0. Later puzzles have fewer solutions. Positive solve reward not established in the supplied research.

Original input:four integers1–9 with unique source IDs, a target5–200 and integer-only solver witness. Enumerate legal pair reductions, retaining expression witnesses. For each newly sampled tile set choose a target from a decreasing solution-count percentile over the ten-puzzle schedule; counts are not promised strictly decreasing across different tile sets. Expression-string counts use ordered subtraction/division and avoid duplicate operand order for commutative operations; this is a declared practical count, not all algebraic equivalence classes.

Actions COMBINE{left,right,operator}; consume two tile IDs/create a result with merged source provenance. Reject same/missing tile, noninteger division and zero divisor without changing tiles. UNDO/RESET restore tiles within the same deadline. Success requires one target-valued tile using all four original sources exactly once; an incorrect final tile locks the round with0. SKIP unlocks at10 seconds and earns−1; timeout0; solve+1 is original.

Output expression, move state, solve/skip/timeout, first-move time and frozen witness/count. Example2,3,4,6:6÷3=2;4+original2=6;result2×result6=12. Both different source2s remain distinguishable by IDs. Unknown: source operands, target distributions, equivalence count and reset semantics.

### T19. Pump

Reference: [Pump](https://www.tradermath.org/pump/dashboard).

Published:Manual30 balloons at10¢/pump; cash-in banks, pop loses round. Thresholds drawn1–128 without replacement. Stakes20 at20¢, commit target up front; pop additionally penalizes half accrued round value. Source FAQ describes64 as a Manual EV optimum.

Original input:secret shuffled threshold permutation. The Kth attempted pump pops, while prior K−1 pumps accrue provisionally. Manual actions PUMP/CASH_IN; Stakes COMMIT_TARGET{target:0..127}. A safe cash-in reveals only K>m, never the exact threshold; live state never includes future thresholds. Stakes pop discards round accrual and subtracts half **safe pre-pop** accrual from the bank; negative bank allowed. This resolves an undisclosed boundary convention explicitly.

Output bank cents, safe pumps, pop/cash counts, bank changes, threshold observation and adjusted mean pumps on cashed balloons (null if none). ExampleK7, cash after5 Manual safe pumps→+50¢. Attempt7 pops. Stakes atK7 has6 safe pumps×20¢=120¢ provisional, discards it and subtracts60¢ bank. For initially uniform K, Manual targetm has expected cents10m(128−m)/128, maximized at64. Sampling without replacement and censored observations can change later posterior-optimal policies;64 is not asserted universal after learning. Unknown: source boundary, penalty rounding/floors and disclosure/behavioral summaries.

### T20. Skyline

Reference: [Skyline](https://www.tradermath.org/skyline/dashboard).

Published:ten untimed three-tower rearrangements; move only top floor; displayed verified optimum2–7; Open unlimited heights, Capped3/2/1. First-move planning recorded without time penalty.

Original input:five uniquely labelled/color-coded floors A–E, bottom-to-top arrays, goal[[A,B],[C,D],[E]], capacities Open[5,5,5] or Capped[3,2,1]. Open capacity5 is effectively unlimited for five total floors. BFS from the goal samples a start at desired distance2+(index mod6); its reversed path is a shortest witness. Five floors leave a spare capacity in Capped; six fully packed floors would prevent any move under the published transfer rule.

Action MOVE{from,to}, indexes0–2, transfers a top floor if capacity allows. Invalid moves preserve state. UNDO is a legal reverse and increments moves; RESET restores initial stacks without erasing accumulated moves. Exact goal ends level. Output legal moves M, optimum M*, excess M−M*, efficiency M*/M and first-move latency. Original raw score counts perfect levels. Example[[R,B],[G],[]]→[[R,G],[B],[]] requires three: B→3,G→1,B→2. Unknown: source floor count/colors, level generation, reset/undo and aggregation.

### T21. Mystery Figure

Reference: [Mystery Figure](https://www.tradermath.org/mystery-figure/dashboard).

Published:five untimed deduction rounds, hidden attributes, count-only feedback and retained history. Standard2–5 attributes, Deep up to6/1296 possibilities. Attempts matter; comparison with information-efficient play. Domain sizes/reference algorithm not disclosed.

Original domains:Colour3,Shape3,Pattern3,Border3,Symbol4,Size4. Standard counts[2,3,4,5,5]; Deep[3,4,5,6,6]. Product at six attributes=1296. A uniformly sampled secret tuple is frozen. Action GUESS{values:[...]} must provide one allowed value per attribute; feedback k counts exact attribute matches without identifying them. Keep only tuples matching every prior feedback count. Repeated/noncandidate guesses are allowed; a noncandidate can be informative.

Output guesses-to-solve, feedback history, remaining candidate count, first-move time and per-last-guess comparison. The reference heuristic minimizes expected remaining candidates Σ_k |C_k|²/|C| across allowed guesses under uniform surviving prior. Its local minimum is not globally optimal expected guesses. Example(red,circle,striped) vs(red,square,plain) returns1. Raw correct-round count is a convenience, while guess efficiency remains the relevant detailed measure. Unknown: source attributes, five-round schedule, guess caps, scoring and optimal-policy baseline.

### T22. Sweet Spot

Reference: [Sweet Spot](https://www.tradermath.org/sweet-spot/dashboard).

Published:two minutes,2–6 independent dials,0.5–1 rotation/s,25–45° arcs,4–9-second lifetimes; hit+1, wrong/background−1, unresolved expiry−1. Angular-error/hit-rate outputs and optional device label.

Original input:800×500 field, six nonoverlapping fixed slots, radius87; initial theta, signed angular speed180–360°/s, target center, width and expiry frozen through keyed dial generation. Concurrency target=min(6,2+floor(elapsed/24000)):2,3,4,5,6 across five24-second intervals. This exact growth schedule and fixed placement are original choices.

Action CLICK{x,y,elapsed_ms}. Convert screen coordinates to logical field coordinates, identify live dial hitbox, compute theta(t)=theta0+omega×(t−spawn)/1000. Signed circular error is normalized to[−180,180); hit if absolute error≤width/2. A hit removes/replenishes the dial. A miss leaves it live; subsequent expiry can cost another point. Background click−1. Process every intervening expiry in time order even across slow frames; expiry wins at exact equality. At global end close remaining dials as expired without respawning.

Output raw hits−wrong_clicks−expiries, hits/(hits+expiries) dial rate, hit/click accuracy, mean absolute angular error among hits (null when none), never-hit expiries and click ledger. Example center90°,width30°,theta0=0,omega180°/s:click at.5 sec→90°,hit; .7 sec→126°,miss. Device label is metadata, not a correction factor. Unknown: source concurrency/spawn schedule, miss/retry and final-boundary policy, hit-rate denominator and latency calibration.

## 5. Overlap and merge decisions

Strong overlap means reusable task logic, not identical source questions or scores.

| Trainer | Existing assessments | Existing games | Shared component and retained distinction |
|---|---|---|---|
| Math | Maven/Akuna/Flow/All Options numerical; mixed tests | Math Challenge; partial Fruit/ETF | Exact parser/generation; distinct timings/distributions/marking |
| Sequences | Maven/Akuna/Flow/All Options | None dedicated | Intended-rule engine; next-term/anomaly/letter are separate modes |
| Fractions | Numerical/probability subskill | Math/Probability | Rational math; new fixed-digit rounding and asymmetric penalty |
| ETF | Numerical/valuation questions | ETF Trading | Basket/hedge math; lessons/cases differ from live arbitrage lifecycle |
| Options | Quantitative/finance subskills | Generic quote/inventory games | Shared book validation; new parity, Greeks and multiplier semantics |
| Futures | Numerical/finance subskills | ETF arbitrage | Shared execution/accounting; new carry/basis conventions |
| Prediction | Probability/EV, Akuna Grid mathematics | Probability Betting; marble side markets | Shared EV; contract settlement, collateral and evidence updates differ |
| Probability | Maven/Mako/SIG/DRW/Citadel/Wincent | Betting/card probability | Same mathematical evaluation; quiz accuracy differs from wagering |
| Fermi | Maven interval | Fermi; Make Me a Market | Same frozen facts; point/interval/bid-ask outputs score differently |
| Task Switch | Broad mixed aptitude relevance only | None | New active-rule/RT scheduler |
| Response Conflict | Broad cognitive relevance only | None | New congruency/side-mapping runtime |
| Pincode | Memory modality additional | None | New encode/mask/transform/recall |
| Cube | Mako spatial | None | Reuse assessment geometry; standalone marking stays separate |
| Code Match | CTC clerical checking | None | String checking reused; shrinking item deadline added |
| Grid Recall | Spatial memory additional | None | New masked target/decoy sets and adaptive span |
| Intercept | Anticipation additional | No mechanical equivalent | New deterministic ball/projectile simulation |
| Hold Fire | Inhibition additional | None | New go/no-go confusion matrix and balanced accuracy |
| Target Number | Arithmetic/planning subskills | Partial Math Challenge | Shared arithmetic; new tile provenance and solver |
| Pump | Risk/learning additional | Partial bankroll concepts | Censored unknown hazard differs from known-probability Kelly bets |
| Skyline | Planning additional | None | New legal-state graph, BFS and excess moves |
| Mystery | General logical deduction | None | New count-feedback/candidate filtering |
| Sweet Spot | Attention/motor timing additional | None | New multi-object angular simulation |

The implementation adds one trainer runtime and shared question/math helpers rather than22 independent API/game infrastructures. It reuses assessment asset rendering and cube math; probability questions use the existing finite sample spaces; fractions use exact rationals; Fermi shares original frozen facts. Dedicated puzzle and motion modules preserve their own state/actions. Existing nine games retain market fills, stakes, settlement and P&L rules. Related company assessment links are preparation shortcuts, not assertions that a firm uses every trainer.

Scores deliberately remain separate: net speed, asymmetric fraction net, accuracy, balanced accuracy, recall/span, timing precision, planning efficiency, information efficiency, simulated bank and trading equity. A single universal total would conceal what the player actually did.

## 6. Reproducibility verification and remaining limits

The automated suite exercises all22 trainers and every declared mode through original correct-action playthroughs, then replays their frozen tapes. Separate checks cover memory response isolation, half-up rounding/carry, tile witness validity, BFS shortest witnesses, cash-in threshold censoring, Stakes penalty accounting, Hold Fire press-all/press-none50%, exact deadline rejection and changed-tape hashes. HTTP checks verify22-profile discovery,24 public Fermi prompts without answers, live review/tape rejection and replay verification. Frontend tests cover catalogue/guide completeness, memory render restrictions, editable-model calculations and escaping. TypeScript checking and static Astro production build cover imports/routes/assets.

These checks establish deterministic implementation behavior under the tested configuration; they do not validate source-site parity, psychological measurement validity or hiring performance. The public evidence still lacks premium case scripts, private question distributions, precise source exposure times, several source scores, adaptive step sizes and boundary policies. Cube oriented textures, full option repricing/exchange-futures margin, account-based course persistence and laboratory-grade presentation calibration remain explicitly outside the original simplified implementation. Preserve versions and published/original/unknown labels when extending them, so prior tapes retain their meaning.
