# Company assessments: reproducible specifications and overlap analysis

Research date: November 17, 2026. Runtime: `assessment-runtime-v1`. Question bank: `original-tasks-v1`. Preset manifest: `assessment-profiles-v1`.

This document specifies the playable `/assessment` implementation and distinguishes it from the public practice products that informed the presets. It incorporates the assessment inventory supplied with the task and checks the linked public catalogue and lobbies. It does not describe access to premium questions, private employer banks, or proctoring systems.

## 1. Evidence, scope, and status labels

The [Tradermath catalogue](https://www.tradermath.org/online-assessments) contains 21 named assessment cards across 11 firms. Expanding the three Maven subjects into classic and new variants gives 24 profiles. Maven's separately linked [Confidence Intervals lobby](https://www.tradermath.org/online-assessments/maven-securities/confidence-interval/lobby) adds profile 25, covering 22 named assessments. Cube folding is a task within the Mako-style mixed battery, not an additional catalogue assessment.

**Published** means a public practice page states a property. It does not establish employer endorsement or the employer's current test. **Proposed / implemented** means this application's declared original choice. **Unknown** means that the public material does not determine the answer. No company pass mark or population percentile is inferred. Full and preview budgets are independent presets: the preview's average seconds per item must not be extrapolated to recover the full timer.

Every preset is runnable with original generated content. Career Match is prominently labelled a content demo: its timing and forward navigation are published, but its question taxonomy is not. The Akuna grid uses an explicitly invented, fully disclosed payoff model and a proposed 45-second item clock. It does not reproduce an undisclosed noisy or misleading signal. Maven interval reasoning is text for human review; the implementation does not record video or claim to grade spoken reasoning.

The canonical metadata is `backend/internal/games/assessment_profiles.json`, embedded in Go and imported by Astro. This single manifest supplies company names, identifiers, counts, timers, controls, evidence notes, families, source links, and overlaps. A shared assessment engine provides lifecycle and scoring. It is registered as the `assessment` game without adding 25 near-identical runtime implementations or changing the nine-game trading catalogue.

## 2. Public catalogue and configuration contract

`GET /api/assessments` returns `{ "runtime_version": "assessment-runtime-v1", "profiles": [...] }`. A profile exposes `spec_id`, stable `id`, company and company ID, title/variant, full and preview counts/budgets, `timer_mode`, default item clock, `navigation`, `response_mode`, ordered `families`, distinct `engines`, related game slugs, score/navigation/content origins, aids, proposed choices, unresolved details, source URL, review date, and manifest version. Repeated entries in `families` are intentional weights.

Create an attempt through the same session API as the trading games:

```json
{
  "game": "assessment",
  "players": ["alice"],
  "seed": 42,
  "config": {
    "profile_id": "maven-numerical-new",
    "mode": "preview"
  }
}
```

The response has a session ID, actual players, seed, and initial per-player snapshots. Exactly one player is allowed per assessment attempt. Independent candidates create independent sessions; they do not share a cursor or answer map. Player names must be nonempty and unique under the common session store's validation. Assessment session state is in memory: refreshing a browser preserves a live attempt while that backend process exists, but restarting the backend does not persist it.

`mode` is `preview` (default), `full`, or `custom`. Preview/full use their profile's own budgets. Nonzero overrides in those modes are rejected rather than silently changing the published shell. Custom supports:

| Key | Meaning and bounds |
|---|---|
| `question_count` | 1–300; omitted/zero uses the full-profile count |
| `duration_ms` | Global clock, 1,000–7,200,000 ms; omitted/zero uses full budget |
| `item_time_ms` | Grid item clock, 1,000–600,000 ms; omitted/zero uses proposed 45,000 ms |
| `navigation` | `forward_only` or `revisable`; grid requires forward-only |
| `response_mode` | `numeric`, `single_choice`, `mixed`, `token`, or `interval`, subject to task constraints |
| `family` | One declared family from the profile; omitted selects its complete weighted mix |
| `wrong_points` | Integer −5 through 0; UI offers 0 or −1; correct remains +1 and blank 0 |

For per-question timing the resolved theoretical duration is count × item clock; this is not a second global deadline. `token` controls are restricted to sequence profiles. Interval profiles require interval controls and cannot be turned into ordinary arithmetic by changing a string. Numeric/mixed modes preserve intrinsic choice tasks such as cube views, verbal logic, and highest-EV grid selection; they do not fabricate a numeric answer for a qualitative choice. Conversely, choice mode wraps scalar numeric/letter tasks in four stable options. Cube tasks keep five views.

Aids are descriptive preparation notes, not enforcement. The application has no embedded calculator or camera. An original generator's difficulty distribution is not established to equal an employer's difficulty merely because the question count and timer match.

## 3. Candidate observation, actions, and privacy

Read `GET /api/sessions/:id/state?player=alice`. The outer response identifies the game/player, available players, and completion status. Its assessment state has this shape:

```json
{
  "phase": "ASSESSMENT",
  "finished": false,
  "profile": {"id":"drw-assessment","navigation":"revisable"},
  "config": {"profile_id":"drw-assessment","mode":"preview"},
  "index": 0,
  "question_number": 1,
  "question_count": 3,
  "time_remaining_ms": 600000,
  "timer_mode": "global",
  "item_deadline_ms": 0,
  "navigation": "revisable",
  "saved_question_ids": [],
  "committed_question_ids": [],
  "saved_response": {},
  "question": {
    "question_id":"q001",
    "family":"advanced_probability",
    "prompt":"A fully specified original mathematical question",
    "response_type":"single_choice",
    "choices":[{"id":"a","text":"1/3"},{"id":"b","text":"2/3"}],
    "assets":null,
    "units":"",
    "item_time_ms":0
  }
}
```

The actual metadata/config objects are complete, not the abbreviated examples above. Questions use stable `q001`, `q002`, … identifiers in tape order. Choice IDs remain attached to their text after shuffling. Public questions are constructed from a whitelist. They omit the answer, explanation, evaluator model, and grading tolerances. Observations never transmit future question content or an opponent's responses. Saved responses only contain this candidate's current draft.

Actions are posted to `POST /api/sessions/:id/action` in the common envelope:

```json
{
  "action_id":"unique-client-command-7",
  "player_id":"alice",
  "action":"SAVE_ANSWER",
  "payload":{"question_id":"q001","response":{"answer":"3/4"}}
}
```

For choices, `answer` is an option ID such as `b`, not its visible label. A numeric response is a string. An interval response is `{ "low":"80", "high":"120", "reasoning":"My assumptions…" }`. Letter answers normalize to uppercase. A successful receipt identifies acceptance, sequence, saved state/current index, and whether the test has finished. It returns no correctness, hidden truth, reward, or solution while the assessment is live.

`SAVE_ANSWER` validates and overwrites the current saved response without advancing. `CLEAR_ANSWER` removes it. `NEXT` moves forward: in forward-only mode it locks the question, including an explicit blank; in revisable mode it simply navigates and leaves all answers editable. `PREVIOUS` and `JUMP_TO {index}` use zero-based destination indices and require revisable navigation. `SUBMIT_TEST` finalizes every currently saved response, including blanks elsewhere. Normal save/clear/navigation commands carry the current question ID so a delayed response cannot be applied to a later item. Final submission does not depend on a particular cursor.

The browser autosaves complete valid drafts after 450 ms of input inactivity. A save is also awaited before navigation or submission. An empty draft is cleared; a malformed or incomplete interval blocks ordinary commit and remains editable. The explicit Skip button clears it and commits a blank. Inputs are not reconstructed on clock polls. Write operations are serialized, and reads begun before a mutation are discarded. A reload restores the latest server-accepted save, not text that never reached the server.

Rejected malformed input returns `INVALID_FORMAT`, does not consume an answer, does not reset a clock, and does not assign a wrong-answer penalty. Other reasons include `STALE_QUESTION`, `LATE`, `NAVIGATION_LOCKED`, `NO_NEXT_QUESTION`, `NO_PREVIOUS_QUESTION`, `UNKNOWN_PLAYER`, and `MARKET_CLOSED`. Accepted action-ID retries return the original session receipt with `DUPLICATE`; they do not save twice, advance twice, or add a second engine event. A valid but mathematically wrong answer is saved normally and graded only after finalization.

Live results and tapes are protected by the backend, not only hidden buttons: both endpoints return HTTP 409 with an explanatory error until the attempt ends. This assessment-specific guarantee is stronger than the legacy trading-game inspection APIs. Replays accept supplied completed tapes; they do not provide access to another live session's answer key.

## 4. Timing, normalization, and scoring

The authoritative clock is elapsed server time relative to attempt creation. Global expiry is inclusive: an action received exactly at `duration_ms` is late. Final elapsed time on automatic expiry is the declared deadline, not the next polling time. Forward-only exhaustion or explicit submission can finish earlier. Revisable tests finish on submission or expiry, not merely by reaching the last item.

An item-timed run starts question 1 at elapsed zero. Its deadline is `item_start + item_time_ms`. Explicit commit starts the next item's clock at that action's receive time; unused time does not carry. Timeout locks the current saved answer or blank and starts the next item at the exact previous deadline. A delayed tick catches up through all already-expired items, avoiding dependence on how often a browser happened to poll. It does not give a fresh full clock after an idle tab returns. There is no pause command in this baseline.

The shared exact parser accepts signed integers, finite decimals, signed integer fractions, and bounded scientific notation. Whitespace around the answer is removed; the decimal separator is a dot. Scientific exponents are limited to −60…60 and the total string to 128 characters. Denominator zero, NaN/infinity, hexadecimal, expressions, and comma-separated numbers are rejected. Thus `0.75`, `3/4`, `6/8`, and `7.5e-1` are the same rational value. Parsing is shared with Math Challenge, while scoring is not.

Every scalar question stores absolute and relative tolerances as rational strings. The bank currently uses both as zero: equality is exact. The evaluator supports `abs(actual−expected) <= max(abs_tol, rel_tol × abs(expected))` for a future explicitly tolerant item. No preset silently accepts `0.67` as `2/3`: Tradermath's reported rounded-decimal acceptance is a documented difference. A loose global tolerance is deliberately avoided because it could accept zero for a tiny nonzero probability. To change a question's rounding policy, change its versioned item data/prompt and replay tape together.

The default accuracy score is `correct + wrong × wrong_points`, with `wrong_points=0`, blank=0. Only the cited Mako Round 2, DRW mock, and Grid public descriptions establish their one-point practice rule; the rest are labelled proposed. Assessment math does not inherit the trading Math Challenge's −1 penalty. Results report correct, wrong, blank, attempted, raw/maximum score, attempted accuracy, completion rate, elapsed time, per-family counts, and question review. Zero attempts yield accuracy 0, not division by zero. Revisions are graded once from the final answer map.

Intervals are separate: finite positive rational bounds must satisfy L≤U, convert to finite numbers no greater than 10^60, and carry at most 4,000 reasoning characters. For 80% coverage, alpha=0.2 and the original loss is `(U−L)+10(L−y) if y<L`, or `(U−L)+10(y−U) if y>U`, otherwise just width. Lower is better. Bounds count as covering when the truth equals either endpoint. The output reports coverage, width, individual loss, covered count, coverage rate, and mean loss over submitted intervals. It deliberately returns `raw_score:null`, `maximum_score:null`, and no attempted-accuracy scalar for this mode. A blank interval has no loss, not a perfect zero loss. Reasoning is stored for human review; no automatic quality grade is claimed. One interval cannot measure long-run calibration.

## 5. Deterministic generation and replay

A seed is convenient, but a frozen tape is the reproducibility artifact. Generation uses `splitmix64-v1` seeded with `seed + 0xA55E55` modulo 2^64. The profile's ordered family list is copied and shuffled once; question i uses schedule[i mod length], unless a custom family overrides it. Repeated families create explicit weights and ensure every declared family appears in a sufficiently long run. Family-specific draws then construct original questions, evaluator models, choices and option order. The exact generated questions are retained, so replay does not rely on a future generator preserving draw order.

A completed JSON tape contains runtime/bank/PRNG versions, seed as a decimal string, player, the complete profile snapshot, resolved configuration, questions including answer keys/explanations/models/tolerances/assets, canonical question SHA-256, engine action events, automatic/explicit transitions, final replay time, and canonical result bytes. Each action event stores relative receive time, sequence, request envelope, acceptance, and rejection reason. Each transition records time, type, and source index. Duplicate accepted retries filtered by the session store are not re-executed engine events.

Canonical JSON sorts nested object keys after decoding with number preservation. This matters because a typed cube scene and the same scene loaded as generic JSON maps must have the same hash and result bytes. Arrays, option order, and numeric spellings stay part of the frozen input. SHA-256 detects accidental question mutation; it is not a signature authenticating an employer result.

The UI downloads a completed tape. To verify it, import it on `/assessment`, post the raw JSON object to `POST /api/assessments/replay`, or run:

```sh
cd backend
go run ./cmd/replay /path/to/assessment-tape.json
```

Replay validates supported versions, manifest limits and question IDs, the question hash, item deadlines, tolerance formats, and monotone event times. It advances an injected clock to every recorded action, replays the command, and compares acceptance/reasons. It then advances to the recorded final time and requires identical transitions and canonical final results. A successful HTTP response is `{ "verified":true, "results":{...} }`; malformed or divergent recordings receive HTTP 400. HTTP imports are limited to 5 MiB. Legacy trading-game tapes have a different schema and are not accepted by this assessment replay endpoint.

## 6. Shared task-engine specifications

### E1 — Arithmetic and numerical reasoning

The scalar input is an expression, relation, or percentage problem with stated units and an exact rational key. Integer/decimal/fraction tasks directly reuse `genMathQuestion`, the existing Math Challenge generator. Additional families cover four-digit cent-valued addition/subtraction, percentages, bounded integer powers, `b*x+a=rhs`, and chained decimal multiplication. Models freeze operands or expression plus generator identity. Four-digit tasks draw cent-valued operands with simple quarter/half decimals. Powers use bases 2–8 and exponents 2–4. Relations construct an integral solution. Chained products store exact rational operands, not floating-point grading keys.

Choice wrapping generates one correct and three distinct normalized alternatives, shuffles once, and assigns IDs a–d. Candidate answers select the resulting stable ID. The wrong alternatives are original simple offsets, not a reconstruction of commercial distractor algorithms. The review preserves the expression, normalized response, key, explanation, and time spent. Static assessment answers never create a trading position or first-click score.

### E2 — Number/letter sequences and anomalies

A question supplies tokens plus a precise task: next term, next letter, or wrong value. The original grammar includes arithmetic/geometric progressions, increasing successive differences, and two interleaved progressions. Starts, steps, and intended grammar are hidden until review. Letters use A–Z with no wraparound and a constant shift. Anomaly tasks explicitly say they are arithmetic progressions with exactly one corrupted interior term and request the wrong VALUE rather than the index; the stored model includes both index and correction. This avoids conflating several superficially similar interfaces.

The baseline does not assert that a finite sequence has a unique continuation among all imaginable mathematical functions. Its intended solution is defined within the declared generator grammar. New Maven schedules weight anomalies and include some letter tasks. Classic Maven wraps next-term answers in choices. Akuna mixes number and letter continuation; Flow and All Options use numeric continuation. Review gives the grammar and completed result, separate from the candidate-facing tokens.

### E3 — Probability, statistics, counting, and expected value

Elementary questions reuse Probability Betting's finite primitive and predicate machinery: one die, two dice, three fair coins, or a standard card rank. The assessment enumerates elementary weights exactly: totals 6, 36, 8, or 13 equally weighted rank classes. It never uses a sampled outcome or bankroll to grade a probability answer. Independence, replacement, and card-rank conventions appear in the prompt.

Additional original templates include urn draws without replacement, Bayes with two possible coins, variance of independent Bernoulli sums, geometric waiting times including success, expected time to two consecutive heads, unordered committee counts, and two-point net-payoff expected values. Answer keys are exact rationals or integer binomial coefficients. The advanced label denotes additional templates, not verified equivalence to a commercial test's difficulty. Derived EV questions in the grid engine enumerate explicit signed payoffs. Static probability scoring rewards the mathematical answer rather than the luck of a realized bet.

### E4 — Mixed reasoning and E5 — cube folding

Mixed reasoning includes syllogistic subset/disjointness deductions, a small original analogy pack, exact eight-character code matching, percentage growth from synthetic table data, and constant quarter-turn arrow patterns. Qualitative items freeze their intended relation and choices. Charts store both underlying numeric rows and units so the answer is not reverse-engineered from image pixels. The current pack is a starting bank, not an exhaustive verbal or cognitive aptitude battery.

Cube tasks store a six-cell net with integer paper coordinates and labels, plus five candidate top/front/right triples. The generator rotates a valid cross-with-tail net in the paper plane and permutes labels. Breadth-first folding assigns each face a 3D outward normal and paper axes U/V. Crossing an edge applies a 90-degree fold. A candidate is valid only if its faces are distinct and `right_normal × top_normal = front_normal`; reflection-only views and opposite faces touching are invalid. The complete cube admits 24 proper rotations; exactly one of the five displayed views is valid. Labels have no directional texture. Textured/oriented faces would require additional versioned texture axes and equivalence tests and are not silently simulated by this plain-label variant.

SVG rendering is versioned as `cube-svg-v1`. The public scene contains the net and candidates, not the folded mapping or correct ID. Accessible text preserves every net coordinate and candidate triple. Review exposes the intended fold explanation. The standalone commercial cube trainer's speed score is not used as the mixed Mako assessment score.

### E6 — Equity, E7 — letter grid, E8 — intervals

Equity questions freeze a synthetic FY2026 statement in millions. One template computes diluted EPS then equity price using a supplied P/E. The second computes enterprise value as EBITDA × EV/EBITDA, converts to equity by subtracting debt and adding cash, and divides by diluted shares. The observation explicitly distinguishes enterprise/equity values, units and share basis. Review includes the reconciliation. No live share price, subjective recommendation, or changing financial statement enters grading.

Grid questions display a shuffled 26-letter board as 13 pairs and a disclosed rational probability for each row. Original row weights are independent integers 1–5, normalized by their sum. A +letter pays +12 on its row and −1 otherwise; a −letter negates that payoff; paired components add even if a letter repeats; the stated cost is subtracted once. Four bets have costs 1/101…4/101. These costs prevent ties because uncosted EVs have denominator at most 65. A question either requests bet A's exact net EV or selects the unique best of four bets. Evaluation enumerates all 13 rows. Review stores per-row payoffs and all four net EVs. There is no secret winner drawn for assessment scoring, no inferred noisy signal, and no claim that these are Akuna's actual payoff rules.

Interval questions reuse a frozen positive-valued subset of the Fermi bank. They disclose quantity, units, desired 80% coverage and deadline, hide the answer, and accept low/high bounds plus reasoning. The same truth can support a Fermi point estimate, an interval, or a market-making quote, but their outputs remain distinct: multiplicative point error, interval loss/calibration, and quote/inventory performance are not interchangeable.

## 7. Individual company profiles

All profiles below inherit the lifecycle, privacy, parsing, scoring, replay, and review contracts above. Budgets describe dated public practice shells, not promised employer formats. Each worked example is original and illustrative; actual seeded attempts generate their own complete tapes.

### A01 — Mako Trading: Aptitude (standard)

**Identifier:** `mako-aptitude`. **Published practice shell:** 37 questions / 15 total minutes. **Preview:** 5 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/mako-trading/lobby).

A mixed battery, including actual cube-net scenes, rather than an arithmetic test with a different company label. Original family schedules cycle through shuffled arithmetic, probability, reasoning and cube slots.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `arithmetic, probability, reasoning, cube`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** A bag contains 3 red and 2 blue balls. One uniform draw is made. Choose the probability of red from 1/5, 2/5, 3/5, 4/5. Expected response: **3/5**. There are 3 favorable balls among 5. Selecting its stable option ID saves the answer; no correctness appears before finalization.

**Reuse and unresolved details.** Math Challenge, Fruit Market, Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A02 — Mako Trading: Round 2 · Codility (standard)

**Identifier:** `mako-codility`. **Published practice shell:** 28 questions / 30 total minutes. **Preview:** 3 questions / 5 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/mako-trading/codility/lobby).

The nickname does not introduce coding. Revisitable single-choice answers, delayed grading, and an unchanged server deadline are the important runtime distinctions. Original probability/statistics and reasoning templates only approximate the topic breadth.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `probability, advanced_probability, advanced_probability, probability, reasoning, arithmetic`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: No calculator; pen and paper described.

**Actions, progression and outputs.** Save, clear, next, previous and jump may revise any question before final submission; navigating does not grade or lock an answer. Navigation origin is **published**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **published practice**; changing custom penalties is labelled configured original.

**Original worked example.** A fair coin is tossed independently three times. What is the probability of exactly two heads? Expected response: **3/8**. The favorable ordered outcomes are HHT, HTH, THH among eight. Repeatedly changing the selected option changes one saved response, not the earned points.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards, Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A03 — Chicago Trading Company: Cognitive Aptitude (standard)

**Identifier:** `ctc-cognitive`. **Published practice shell:** 50 questions / 15 total minutes. **Preview:** 7 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/chicago-trading-company/lobby).

Arithmetic, verbal analogy, deduction, visual arrows, clerical code matching and chart questions share one global clock. Topic-level results remain separate so arithmetic speed cannot conceal poor visual accuracy.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `arithmetic, verbal, logic, visual, clerical, chart`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** A sales table gives period A=80 units and B=100 units. Find the percentage increase. Expected response: **25**. Compute (100−80)/80 × 100. The synthetic table, units, four choices, and option order belong to the frozen item.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A04-C — Maven Securities: Numerical (classic)

**Identifier:** `maven-numerical-classic`. **Published practice shell:** 50 questions / 5 total minutes. **Preview:** 10 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/mental-math/maven-securities/math-test/realistic-test-mode/lobby).

Classic numerical practice includes integer, fraction, decimal, percentage and power families. The multiple-choice interface is preserved rather than merely slowing down the typed test.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `integer, fraction, decimal, percentage, power`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Evaluate 3/4 + 5/8; choose among four normalized alternatives. Expected response: **11/8**. Convert 3/4 to 6/8, then add. One option is equivalent to 11/8. Distinct display strings that normalize to the same answer must not become separate valid options.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A04-N — Maven Securities: Numerical (new)

**Identifier:** `maven-numerical-new`. **Published practice shell:** 50 questions / 5 total minutes. **Preview:** 10 questions / 1 minutes. [Dedicated public reference](https://www.tradermath.org/mental-math/maven-securities/math-test/new-format/lobby).

The new schedule weights four-digit addition/subtraction and includes arithmetic, percentages and relations. This changes both the content recipe and the interface from classic, while retaining the same full count/time.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `four_digit, four_digit, four_digit, integer, percentage, relation`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Tradermath describes no calculator or paper for the reported new employer format.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Evaluate 4286.25 + 713.5. Expected response: **4999.75**. Treat both operands as exact cent-valued rationals. The observation has a typed field rather than answer choices.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. New Maven formats are based on candidate reports; retain both classic and new presets. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A05-C — Maven Securities: Sequences (classic)

**Identifier:** `maven-sequences-classic`. **Published practice shell:** 30 questions / 6 total minutes. **Preview:** 10 questions / 5 minutes. [Dedicated public reference](https://www.tradermath.org/sequences/maven-securities/sequence-test/realistic-test-mode/lobby).

This is continuation recognition, not corruption detection. The task supplies a next-term prompt and choices; it does not ask which term is wrong.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `sequence`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Continue 3, 7, 11, 15, ? using the intended arithmetic grammar. Expected response: **19**. The difference is 4. The classic profile asks for the next term through a stable choice ID.

**Reuse and unresolved details.** No direct equivalent among the original nine games. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A05-N — Maven Securities: Sequences (new)

**Identifier:** `maven-sequences-new`. **Published practice shell:** 20 questions / 4 total minutes. **Preview:** 5 questions / 1 minutes. [Dedicated public reference](https://www.tradermath.org/sequences/maven-securities/sequence-test/new-format/lobby).

This is mainly anomaly detection with some letters. The original implementation names its arithmetic-progression corruption grammar explicitly; index and wrong-value answers are never interchangeable.

**Inputs and hidden state.** Candidate controls are `token` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `anomaly, anomaly, anomaly, anomaly, letter`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** An arithmetic progression contains exactly one wrong value: 3, 7, 11, 16, 19. Enter the WRONG VALUE. Expected response: **16**. The intended progression has difference 4; the fourth value should be 15. An answer of 4 would be wrong because the prompt asks for the value, not its index.

**Reuse and unresolved details.** No direct equivalent among the original nine games. A timed original practice bank using the published count and time budget. New Maven formats are based on candidate reports; retain both classic and new presets. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A06-C — Maven Securities: Probability (classic)

**Identifier:** `maven-probability-classic`. **Published practice shell:** 18 questions / 30 total minutes. **Preview:** 3 questions / 6 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/maven-securities/probability/lobby).

Longer classic probability practice uses choices and an original elementary/advanced mix. Calculator permission is described by the public practice page.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `probability, advanced_probability`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Calculator allowed in the cited practice description.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** A bag contains 3 red and 2 blue balls. Draw two without replacement. Find P(both red). Expected response: **3/10**. Multiply 3/5 by 2/4. The classic profile presents choices; its calculator permission is a source note, not an embedded calculator.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A06-N — Maven Securities: Probability (new)

**Identifier:** `maven-probability-new`. **Published practice shell:** 20 questions / 10 total minutes. **Preview:** 4 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/maven-securities/probability/new-format/lobby).

Faster typed probability practice uses elementary sample spaces and urn calculations. New and classic versions remain distinct records because interface, pace and content distribution differ.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `probability, urn`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Tradermath describes no calculator or paper for the reported new employer format.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** The same two-without-replacement experiment asks for a typed probability. Expected response: **0.3 or 3/10**. Both normalize to 3/10. An approximate answer for a nonterminating rational only passes if that specific item declares a nonzero tolerance; this implementation defaults to exact grading.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. New Maven formats are based on candidate reports; retain both classic and new presets. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A07 — Da Vinci Trading: Numerical Quiz (standard)

**Identifier:** `da-vinci-numerical`. **Published practice shell:** 20 questions / 12 total minutes. **Preview:** 3 questions / 1 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/da-vinci-derivatives/numerical-quiz/lobby).

The forward-only policy is published. Original numerical relation, ratio/percentage and arithmetic content fills an otherwise undisclosed bank; it is not asserted to be the private source distribution.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `percentage, relation, arithmetic`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **published**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** A price rises from 80 to 92. What is the percentage increase? Expected response: **15**. The increase is 12, and 12/80 × 100 = 15. Commit locks the item. The next page cannot be used to revise the old choice.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A08 — Da Vinci Trading: Career Match (demo)

**Identifier:** `da-vinci-career`. **Published practice shell:** 18 questions / 12 total minutes. **Preview:** 3 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/da-vinci-derivatives/career-match/lobby).

Only the timed single-choice, no-backtracking shell is established. The application explicitly chooses an original logic/arithmetic/verbal demo pack; a faithful subject-matter reconstruction remains impossible from the public evidence.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `logic, arithmetic, verbal`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **published**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Original demo only: all X are Y, and no Y are Z. Which statement must follow? Expected response: **No X are Z**. This tests the supplied mixed-reasoning demo shell. It does not establish that Career Match itself contains syllogisms or is an ability rather than personality assessment.

**Reuse and unresolved details.** Math Challenge, Fruit Market. Content demo: the public lobby discloses timing and navigation, but not the subject matter. This mixed reasoning bank is an original substitute. Exact question distribution, difficulty, and employer pass thresholds. Actual Career Match task taxonomy and grading. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A09 — Susquehanna (SIG): Quantitative Evaluation (standard)

**Identifier:** `sig-quantitative`. **Published practice shell:** 16 questions / 20 total minutes. **Preview:** 3 questions / 3 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/susquehanna-international-group/sig-quantitative-evaluation/lobby).

Keep this assessment distinct from SIG Problem Solving. Original probability and statistics questions state their stochastic assumptions and return numeric answers in this implementation.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `probability, advanced_probability`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Calculator allowed in the cited practice description.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Independent X~Bernoulli(0.4), Y~Bernoulli(0.7). Find Var(X+Y). Expected response: **0.45**. Independence makes variances add: 0.4×0.6+0.7×0.3=0.45. The typed numeric control is an original implementation choice for this preset.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A10 — Susquehanna (SIG): Problem Solving (standard)

**Identifier:** `sig-problem-solving`. **Published practice shell:** 17 questions / 60 total minutes. **Preview:** 3 questions / 3 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/susquehanna-international-group/problem-solving-assessment/lobby).

This longer profile emphasizes mathematical and logical problem solving. Counting, deduction and advanced probability templates share the shell but use their own controls and exact keys.

**Inputs and hidden state.** Candidate controls are `mixed` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `logic, counting, advanced_probability`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Choose an unordered committee of 2 from 5 distinct people. Expected response: **10**. C(5,2)=5!/(2!3!)=10. The problem explicitly says unordered and without replacement. Logic templates retain choices even in a mixed interface.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A11 — Susquehanna (SIG): Equity Research (standard)

**Identifier:** `sig-equity`. **Published practice shell:** 20 questions / 30 total minutes. **Preview:** 3 questions / 3 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/susquehanna-international-group/equity-research/lobby).

Frozen synthetic statements replace changing financial data. Enterprise value, equity value, diluted share count, metric dates and unit scales are always explicit.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `equity`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** A synthetic statement gives net income 24 million, diluted shares 12 million, and justified P/E 15. Find equity price per share. Expected response: **30**. EPS=24/12=2 and price=2×15=30. Money and shares are both in millions, so the per-share result is in currency units.

**Reuse and unresolved details.** ETF Trading, Make Me a Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A12 — DRW: Online Assessment (practice MCQ)

**Identifier:** `drw-assessment`. **Published practice shell:** 6 questions / 45 total minutes. **Preview:** 3 questions / 10 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/drw/lobby).

The source expressly distinguishes its four-option mock from employer free entry. The published revisable navigation and no-negative-marking rule apply to the mock preset, not automatically to the employer.

**Inputs and hidden state.** Candidate controls are `single_choice` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `advanced_probability, counting, logic`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save, clear, next, previous and jump may revise any question before final submission; navigating does not grade or lock an answer. Navigation origin is **published**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **published practice**; changing custom penalties is labelled configured original.

**Original worked example.** Independent trials succeed with probability 1/6. Count the successful trial. What is the expected number of trials until success? Expected response: **6**. E[T]=1/p=6. This practice preset uses four choices. Its documented employer counterpart is described as numeric entry, so a custom numeric variant must be named separately.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. Practice uses four choices. Tradermath describes numeric entry for the employer format. Use custom numeric mode to train without choices. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A13 — Akuna Capital: Math (standard)

**Identifier:** `akuna-math`. **Published practice shell:** 80 questions / 8 total minutes. **Preview:** 10 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/mental-math/akuna-capital/math-test/realistic-test-mode/lobby).

Arithmetic, fraction and chained-operation templates reuse the exact math generator. Topic distribution and numeric controls remain declared original choices where the public lobby does not settle them.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `integer, fraction, chained`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Compute 84/7 + 3/4. Expected response: **51/4 or 12.75**. Both are exact equivalents. The 80/8 budget is global; it does not impose a hard six-second deadline on every item.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A14 — Akuna Capital: Sequences (standard)

**Identifier:** `akuna-sequences`. **Published practice shell:** 30 questions / 16 total minutes. **Preview:** 10 questions / 5 minutes. [Dedicated public reference](https://www.tradermath.org/sequences/akuna-capital/sequence-test/realistic-test-mode/lobby).

The mixed numeric/letter sequence preset uses an exact token parser and a declared A–Z convention. Its dedicated 16-minute lobby budget is retained instead of substituting a generic historical guide timing.

**Inputs and hidden state.** Candidate controls are `token` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `sequence, sequence, sequence, letter`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Continue the letters B, E, H, K, ?. Expected response: **N**. Shift three alphabet positions with no wraparound. Lowercase n normalizes to N. The actual profile also includes numeric continuation items.

**Reuse and unresolved details.** No direct equivalent among the original nine games. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A15 — Akuna Capital: VidCruiter Grid (original payoff model)

**Identifier:** `akuna-grid`. **Published practice shell:** 18 questions / 45 seconds per item (proposed; source does not publish individual deadlines). **Preview:** 3 items on the same original item clocks. [Dedicated public reference](https://www.tradermath.org/akuna-vidcruiter).

The most underdetermined commercial format receives the strongest implementation label. Only the 13×2 board, signed single/paired bet theme, 18 separately timed items and one-point practice scoring are attributed to the public description.

**Inputs and hidden state.** Candidate controls are `mixed` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `grid`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `per_question` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **published practice**; changing custom penalties is labelled configured original.

**Original worked example.** Under the original model, use uniform row probabilities and put A and B on different rows. Evaluate the uncosted pair +A +B. Expected response: **0**. The pair pays 11 in either matching row and −2 in each of the remaining eleven: (2×11−11×2)/13=0. If its cost is 1/101, net EV is −1/101. Generated items disclose possibly nonuniform row probabilities.

**Reuse and unresolved details.** Probability Betting. Original model: signed letter payoffs, additive paired bets, disclosed row weights and costs. The source does not publish these rules or item timers. Exact question distribution, difficulty, and employer pass thresholds. Commercial payoff, pairing, noisy signals, controls and per-item deadlines. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A16 — Citadel Securities: Trading Assessment (standard)

**Identifier:** `citadel-trading`. **Published practice shell:** 10 questions / 30 total minutes. **Preview:** 3 questions / 5 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/citadel-securities/probability-test/lobby).

The trading title does not make this a trading simulator. It is a static probability/statistics/expected-value answer task with calculator permission described by the source.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `probability, advanced_probability`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Calculator allowed in the cited practice description.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** A payoff is +10 with probability 0.3 and −2 otherwise. Find its expected value. Expected response: **1.6**. E[X]=0.3×10+0.7×(−2)=1.6. This is a static mathematical answer, not an actual purchase, a stake decision, or an equity price.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A17 — Wincent: Quant Assessment (standard)

**Identifier:** `wincent-quant`. **Published practice shell:** 12 questions / 100 total minutes. **Preview:** 3 questions / 25 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/wincent/quant-assessment/lobby).

Open-ended numeric or fractional answers fit the exact parser. Long-form waiting-time and counting examples are original practice templates, not copies of the twelve private brainteasers.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `advanced_probability, counting`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Calculator, pen and paper allowed in the cited description.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Toss a fair coin until two consecutive heads. Starting with no trailing head, find expected tosses. Expected response: **6**. Let E be the initial wait and H the wait after one head: E=1+(E+H)/2 and H=1+E/2. Solving gives 6. The machine grades the numeric answer; an essay derivation is not required.

**Reuse and unresolved details.** Probability Betting, Next Card Betting, Market of Cards. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A18 — Flow Traders: Math (standard)

**Identifier:** `flow-math`. **Published practice shell:** 60 questions / 6 total minutes. **Preview:** 10 questions / 3 minutes. [Dedicated public reference](https://www.tradermath.org/mental-math/flow-traders/math-test/realistic-test-mode/lobby).

The family recipe includes integers, decimals, fractions, percentages and chained multiplication. The full six-minute budget is independent of the slower ten-question preview.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `integer, decimal, fraction, percentage, chained`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Compute 1.2 × 25 × 0.4. Expected response: **12**. The exact rational expression is (6/5)×25×(2/5)=12. A different family may ask 17.5% of 240=42; both share the arithmetic engine but retain separate family labels.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A19 — Flow Traders: Sequences (standard)

**Identifier:** `flow-sequences`. **Published practice shell:** 26 questions / 25 total minutes. **Preview:** 10 questions / 5 minutes. [Dedicated public reference](https://www.tradermath.org/sequences/flow-traders/sequence-test/realistic-test-mode/lobby).

A numeric continuation preset with its own manifest, despite matching All Options count/time. No identity of underlying questions is inferred.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `sequence`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Continue 1, 2, 4, 7, 11, ?. Expected response: **16**. Successive differences are 1,2,3,4, so the next intended difference is 5. The stored grammar explains the answer without claiming all conceivable continuations are impossible.

**Reuse and unresolved details.** No direct equivalent among the original nine games. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A20 — All Options: Math (standard)

**Identifier:** `all-options-math`. **Published practice shell:** 80 questions / 8 total minutes. **Preview:** 10 questions / 3 minutes. [Dedicated public reference](https://www.tradermath.org/mental-math/all-options/math-test/realistic-test-mode/lobby).

The arithmetic runtime and 80/8 timer can be shared with Akuna, but the declared family list contains integers, decimals and fractions. The two company labels select distinct versioned manifests.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `integer, decimal, fraction`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Compute 3.6 / 0.12. Expected response: **30**. Convert to rationals: (18/5)/(3/25)=30. The profile shares an 80/8 shell with Akuna, but not a claim of identical banks or grading.

**Reuse and unresolved details.** Math Challenge, Fruit Market. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A21 — All Options: Sequences (standard)

**Identifier:** `all-options-sequences`. **Published practice shell:** 26 questions / 25 total minutes. **Preview:** 10 questions / 5 minutes. [Dedicated public reference](https://www.tradermath.org/sequences/all-options/sequence-test/realistic-test-mode/lobby).

Runtime, numeric parser and generator grammar are shared with Flow. The company preset and frozen question tape remain distinct reproducibility inputs.

**Inputs and hidden state.** Candidate controls are `numeric` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `sequence`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return +1 correct, 0 wrong, 0 blank by default, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Continue 2, 6, 18, 54, ?. Expected response: **162**. Multiply by 3 each time under the geometric grammar. A numeric input of 162.0 is equivalent to 162.

**Reuse and unresolved details.** No direct equivalent among the original nine games. A timed original practice bank using the published count and time budget. Exact question distribution, difficulty, and employer pass thresholds. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

### A22 — Maven Securities: Confidence Intervals (associated assessment)

**Identifier:** `maven-interval`. **Published practice shell:** 1 questions / 2 total minutes. **Preview:** 1 questions / 2 minutes. [Dedicated public reference](https://www.tradermath.org/online-assessments/maven-securities/confidence-interval/lobby).

This associated assessment extends Fermi point estimates into uncertainty intervals. It has a single final commit, a two-minute clock, stated 80% coverage, and reasoning retained for human rather than automatic evaluation.

**Inputs and hidden state.** Candidate controls are `interval` (intrinsic visual/qualitative choice tasks keep their controls). The implemented family schedule is `interval`; repeated names express original weights. The candidate sees the prompt, units, relevant scene/table and choices, current saved response, progress and remaining time. Keys, solutions, generation models and future questions stay evaluator-only. Aids: Not established by the inspected public lobby; original practice uses no integrated calculator.

**Actions, progression and outputs.** Save or clear the current draft, then commit with Next or explicitly skip. A committed question cannot be revisited. Navigation origin is **proposed**. Timing uses `global` as defined in section 4. Final results return mean interval loss, coverage, width and reasoning for human review; no ordinary raw accuracy score, plus completion, elapsed time, question-level review and versioned replay data. Score origin is **proposed**; changing custom penalties is labelled configured original.

**Original worked example.** Freeze the hidden truth at 100. Compare intervals [80,120] and [105,115] at 80% target coverage. Expected response: **Losses 40 and 60**. The first covers and has width 40. The second has width 10 but misses below by 5, so loss=10+10×5=60. The smaller interval is worse under the declared original rule.

**Reuse and unresolved details.** Fermi Questions, Make Me a Market. One 80% estimation interval with typed reasoning. Original interval loss rewards both width and coverage; it is not a disclosed commercial score. Exact question distribution, difficulty, and employer pass thresholds. Original interval grading and media handling. The generated bank, distractors, exact family proportions and difficulty are original and must remain frozen/versioned to reproduce a run.

## 8. Overlap and merge decisions

| Assessment family | Existing game relationship | Implemented reuse | Deliberately separate behavior |
|---|---|---|---|
| Mental arithmetic | Strong with Math Challenge; calculation skills with Fruit | `genMathQuestion`, rational canonicalization/parser, shared session infrastructure | Question counts, MCQ wrapping, company pacing, revisions, +1/0 assessment default versus +1/−1 game |
| Probability/statistics | Mathematical overlap with Probability Betting, Next Card, Cards | Finite primitives, predicates, exact count evaluation; optional visual cards | Static answer grading has no bankroll, random payout, inventory, or Kelly reward |
| Intervals | Strong estimation overlap with Fermi; related to Make Me a Market | Same frozen positive fact pack and units; shared creation/replay infrastructure | Interval coverage/width/loss and reasoning, rather than point-error bins or quote-book score |
| Signed grids | Shared EV foundation with Probability Betting | Exact expected payout computation and rational keys | A disclosed 13-row signed-payoff scene; no stake or sampled winner |
| Equity analysis | Partial calculation/unit overlap with ETF and Make Me a Market | Numeric grading and frozen scenario approach; related-game links | Synthetic statements and enterprise/equity reconciliation are distinct from ETF multi-leg fills |
| Sequences | No dedicated original-game equivalent | Generic controls, timer and review | New grammar, anomaly semantics, letter normalization |
| Cube/visual/verbal/clerical | No dedicated original-game equivalent | Common shell, manifests, deterministic assets | New symbolic scene and reasoning generators |

There are eight reusable assessment engine families, not 25 independent games. Flow and All Options sequence presets share their runtime and inspected 26/25 full shell, but do not claim shared source content. Akuna and All Options math share 80/8 pacing while keeping different family schedules. Maven classic/new pairs are explicit format variants rather than a single toggle that changes only a heading. SIG's three assessments remain separately selectable because their content and budgets differ.

The original trading engines remain environment modules. They need capacity/racing fills, positions, multi-leg execution, or changing public information. Adding a company label must not remove these mechanics, and adding an assessment mode must not silently replace a profit objective with answer accuracy. `/assessment` links overlapping games, and the existing related game pages link directly to relevant company presets. Shared facts and evaluators reduce duplicate logic while independent score outputs preserve the task's meaning.

## 9. Validation and remaining work

The regression suite creates full and preview runs for all 25 profiles, submits valid answers, finalizes, and reproduces canonical results from exported tapes. Additional checks cover server-side live-answer privacy, action-ID retry filtering, restoring a saved answer, changing revisable answers without repeated grading, forward-navigation locks, exact-boundary expiry, idle per-item catch-up, no unused-time carry, blank scoring, rational equivalents, malformed inputs, explicit tolerances, interval loss examples, and damaged tape hashes. Cube generation is tested across seeds for 24 proper rotations and exactly one valid candidate. Grid EV is independently recomputed from every row rather than asserting the generator's own key.

Frontend checks cover manifest counts, variant differences, overlap links, restoring escaped drafts, stable choice IDs, anomaly versus continuation displays, accessible cube geometry, and visible grid costs/probabilities. The production build includes `/assessment/index.html` and downloadable specification files. Browser interaction and screenshots depend on an available connected browser; passing pure/HTTP tests does not prove the absence of every visual issue.

Unrecovered commercial details remain substantial: private generators and difficulty distributions, undisclosed aids/controls/scoring/navigation, Career Match subject matter, Grid payoff/pairing/noise conventions and actual clocks, interval media/score behavior, and employer format assignment or pass thresholds. The application does not sell original substitutes as recovered private behavior. Future banks should increment the bank version, preserve old tapes, and add generator-specific independent mathematics/asset tests before changing a preset. Standalone 1–10 minute cube speed drills, camera reasoning capture, and durable attempt storage are possible extensions, not currently implemented features.
