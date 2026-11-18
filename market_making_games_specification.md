# Market-making games: reproducible input/output specifications

Research date: November 17, 2026. Scope: the nine games in the [Market Making Games catalogue](https://marketmakinggames.com/games), compared with this repository's original Go engines and Astro interface. Related company assessment specifications are in `tradermath_assessments_specification_and_overlap.md`.

## 1. What this document does and does not reproduce

Each profile separates **published** high-level mechanics from the **implemented original contract** in this project. Public descriptions do not disclose private generators, all defaults, fill algorithms, exact stochastic models, or every scoring boundary. A runnable original model with explicit assumptions is reproducible; an assertion that its random seed recreates another site's private behavior would not be justified.

The Go code is normative for the implementation details below. The browser is a view/controller, not the evaluator. Prices, trades, balances, answer locks, deadlines and final scores are processed server-side. Player-visible information must be distinguished from complete evaluator state, including facts that will only be revealed later. The detailed contracts make it possible to write a different client, build fixtures, and compare outputs without relying on screenshots or a particular refresh rate.

Nine public game slugs remain `fruit`, `math`, `cards`, `marble`, `etf`, `nextcard`, `prob`, `fermi`, and `makeme`. Company assessments use a separate `assessment` runtime and `/assessment` UI, sharing mathematical components rather than pretending that static answer grading is the same action space as market making. The metadata label `premium` in legacy game descriptions identifies the reference site's catalogue tier; this local suite does not impose a premium access gate.

## 2. Shared session and command contracts

The catalogue is `GET /api/games`. Create a session with a game slug, ordered distinct player names, optional config object and seed:

```json
{"game":"cards","players":["alice","bob"],"seed":42,"config":{"prep_ms":10000,"quote_timer_ms":20000,"response_timer_ms":12000,"center_count":3,"max_spread":20}}
```

Omitting players at the HTTP layer supplies `p1`. Empty or duplicate names are rejected. Game-specific minimum/maximum human counts are checked by the store after construction. A supplied zero seed is treated as omitted by the HTTP API and replaced with a time-derived seed; use a positive integer for an intentional repeat. For browser input, use a JavaScript-safe positive integer. Reproduction should record the returned seed, not the word “random.”

Creation returns the session ID, game, actual human player list, seed and initial per-player snapshots. The state endpoint `GET /api/sessions/:id/state?player=alice` returns `{session_id,game,players,player,finished,state}`. `state` is the merged public and requesting-player private observation. The player switch on a shared device selects among human participants; it is not a multiplayer networking or authentication system. It intentionally does not let a lone human take over the computer traders.

A command uses this envelope:

```json
{"action_id":"client-unique-17","player_id":"alice","market_version":4,"action":"BUY","payload":{"quantity":1}}
```

`market_version` is a quote cursor or turn token, not a timestamp. Include the observed value when trading. Some legacy engines use zero as “not supplied,” whereas ETF requires equality even at zero; do not assume every game has identical stale-version semantics. The server processes one session under a mutex. Every applied command receives a sequence. Accepted commands with nonempty action IDs are cached: retry returns the original execution/reward and sequence with reason `DUPLICATE`. A retry must not create another fill. Rejected commands are not cached as successful receipts.

A receipt can contain `accepted`, `reason`, `server_sequence`, `execution`, `public_changes`, `private_changes`, `round_reward`, and `game_over`. Omitted zero rewards are zero. A receipt is an action result, while a fresh snapshot is the authoritative current observation. Rejections include unknown player/action, invalid value, stale quote, insufficient balance, already acted, no capacity, rate limit, wrong turn, and closed market. Validation must happen before book/balance mutation. Time-driven transitions can occur when an action, snapshot, results call, or background tick checks the clock; rejected actions do not undo those elapsed-time transitions.

`GET /api/sessions/:id/results` supplies game-specific rows/components. `GET /api/sessions/:id/tape` supplies the engine's scenario tape. The normal game UI only enables these controls after completion, because several legacy results/tapes expose hidden scenario data. Unlike the new assessment runtime, legacy APIs are developer inspection surfaces and do not uniformly enforce post-completion answer privacy. That distinction matters if deploying a competitive service.

Session storage is in memory. A browser reload can restore a session while that server process lives; a backend restart loses it. Session IDs are not durable identities or authorization tokens. The defaults below are the local application defaults, not a promise of the reference site's unpublished room settings.

## 3. Reproduction inputs, numeric rules, and evidence

The common PRNG is `splitmix64-v1`. Its unsigned 64-bit state increments by `0x9E3779B97F4A7C15`; the mixing steps use shifts 30/27/31 and multipliers `0xBF58476D1CE4E5B9` and `0x94D049BB133111EB`. A bounded integer draw uses the generated integer modulo the bound, inclusive range draws add the lower bound, and a float uses the top 53 bits divided by 2^53. These details, the game-specific seed offset, and draw order are part of a generator version, not incidental implementation choices.

Trading prices/ranks/quantities are generally integer points. Cash, skill and some scores use floating-point arithmetic and explicit two-decimal rounding. Math and assessment scalar answer grading use rational arithmetic. An exact numeric string can be a signed integer, finite decimal, integer fraction, or bounded scientific notation. The shared parser rejects commas, expressions, hexadecimal, NaN/infinity and division by zero; maximum input length is 128 and exponent magnitude 60. Letter and special-token parsing are task-specific. Do not replace the book-question tolerance or estimate parser with this exact parser without versioning the resulting behavior.

To reproduce an **interactive legacy run**, preserve the engine source/version, resolved config, ordered player list, returned seed, initial state/tape, all accepted and rejected request envelopes, their server receive times relative to start, and the finalization time. Some phase-driven engines start the next phase at the time their tick observes a transition, so the timing of those checks also matters. A seed alone does not define a human's orders, their racing order, or poll-driven phase timing.

The nine legacy `Tape()` methods have different scenario schemas. They are not the comprehensive action-and-clock recordings used by `assessment-runtime-v1`. Some omit initial conditions or full configuration and depend on reconstructing the same source/seed; Make Me a Market's tape does not contain a complete order ledger. Their existing deterministic unit tests compare scenarios and controlled fake-clock behavior. The new assessment replay CLI intentionally rejects these legacy formats. A future universal replay format should wrap the scenario plus the complete command/transition log rather than pretending all current tape objects already offer that guarantee.

## 4. Fruit Market (`fruit`)

**Published scope.** [Fruit Market](https://marketmakinggames.com/games/fruit-market-game) describes trading a basket-derived value, changing fruit counts and special value events, with first-trade accuracy affecting the final result. The exact generation and pricing process is not public. The following is the local implemented model.

**Configuration.** Defaults are `duration_ms=300000`, `max_count=12`, `min_trade_interval_ms=400`, `trade_quantity=1`, `event_duration_ms=30000`, `spread_bp=400`. Duration must be positive, cap at least 8, and configured quantity at least 1. The playable/verified unit-trade contract uses quantity 1. Although `spread_bp` is accepted in configuration, the current quote function uses its explicit 1%-of-value half-spread formula rather than this field; it is not a working spread control until that behavior is versioned and changed.

**Visible inputs.** Snapshot fields include two effective bags `{apples,oranges}`, current event `{type,multiplier,nullified}`, bid, ask, version, time remaining, completion, and the player's profit/trade/first-click counters. A nullified bag is shown with effective zero counts. The value itself is withheld from the observation so the candidate calculates it. Opponent books are not needed to make an immediate market-taking decision.

**Hidden state and generation.** Four initial counts independently draw 3–8 from seed offset `0xF00D`. The engine constructs four bag/fruit update streams with 15–20 second intervals and additions 0–3; tape entries carry any scheduled reset counts. Two or three spaced DOUBLE/HALF/NULLIFY events can be generated within a sufficiently long run. The quote tape includes offsets uniformly in [−0.04,0.04). Initial conditions and the exact update/reset generation code are essential because independently generated streams are subsequently applied together.

**Calculation and actions.** At an observation, let A and O be total effective apples and oranges across both bags. `V=round(multiplier*A*O)`. Nullification zeroes the selected bag's contribution. Current quotes use `h=max(1,round(0.01*V))`, `mid=round(V+offset*V)`, `bid=mid−h`, `ask=mid+h`. A quote refresh increments the version. BUY earns `V−ask`; SELL earns `bid−V`, settled immediately. Commands are BUY/SELL with configured quantity, plus NOOP. A successful trade starts that player's cooldown. Rejected rate-limited, stale, wrong-quantity or closed-market commands do not change profit or consume a first click.

**Progress and score.** The first accepted trade by a player at a version consumes that version's first-click opportunity. It is correct only when its immediate reward is strictly positive; flat counts as wrong. Accuracy is correct first clicks / attempted first clicks, or 0 when none. Final score is raw profit times unrounded accuracy, then rounded for output. Results expose final effective bags/value, raw profit, first-click numerator/denominator, accuracy, score and global components. Shared rank counts strictly greater final scores. Returned rows carry those computed shared ranks. Local global points also use an accuracy multiplier on outranked opponents, which differs from the reference's described additive bonus.

**Worked input/output.** Bags (4 apples,5 oranges) and (3 apples,6 oranges), multiplier 1, imply V=77. At 74/76, BUY quantity 1 yields execution price 76 and reward +1. A first such trade records 1/1 accuracy and provisional score 1. A second buy at the same version changes raw profit but not the first-click denominator. DOUBLE makes the same counts worth 154. These numbers are original fixtures, not a sampled commercial question.

**Unresolved or divergent details.** Private quote offsets, reset/stream interactions, exact event scheduling, room controls and multiplayer scoring variants are not recovered from the public description. The accepted-but-unused spread setting and incomplete initial-state tape must be addressed before claiming a complete standalone legacy recording or exposing arbitrary quantity/spread knobs.

## 5. Math Challenge (`math`)

**Published scope.** [Math Challenge](https://marketmakinggames.com/games/math-challenge) specifies timed integer/decimal/fraction arithmetic, positive/negative answer points, review and mode-dependent speed awards. The local implementation uses an original generated bank; it is not a downloaded commercial bank.

**Configuration and input.** Backend defaults are 300,000 ms, 200 items and integer mode. The game page selects mixed mode. Accepted modes are integer, decimal, fraction or mixed; count and duration are positive. All players receive one frozen ordered question tape generated with offset `0xCA1C`, but each player owns an independent index, score, correctness counts, attempt log and last-answer time. No player's speed advances another player.

The current observation gives remaining time, zero-based index, total item count, score, correct/incorrect, completion, and current `{question_id,expression,mode}`. It never includes that item's answer. Integer expressions use bounded addition/subtraction/multiplication/division. Decimal expressions retain exact rational operands; fractions retain exact numerators/denominators. Mixed chooses a mode per generated question. The tape stores every expression, mode and canonical solution.

**Actions and validation.** `ANSWER {question_id,answer}` requires the player's current ID. An old/repeated question, malformed rational, unknown player, unknown action or expired session is rejected. A well-formed wrong value is a real attempt: −1, incorrect increment, and advance. A correct equivalent rational earns +1 and advances. There is no backwards edit or skip in this trading-game mode. Receive-time latency is measured from that player's preceding accepted answer, starting at session time zero, so initial thinking time is included.

**Progress and outputs.** Expiry makes the session finish. Exhausting every player's tape also finishes it; a fast individual exhausting their tape does not finish another candidate's work. The per-action receipt reveals correctness and the correct answer immediately, as appropriate to this game. Results expose per-player review and rank; solo also keeps the original top-level scalar shape for API compatibility. Review rows contain question ID, expression, submitted string, correctness and latency.

Net score is correct−incorrect. For positive net score, seconds per point use configured duration/1000/net. Integer speed thresholds award 3,2,1 at ≤3,≤3.5,≤4.5 seconds; other modes at ≤6,≤6.75,≤7.5. Nonpositive net has no finite seconds-per-point output or speed award. Shared session rank is determined by strictly larger net score; the reference's multi-attempt Open Challenge aggregation is not implemented.

**Worked example.** Current q0 asks `1/2 + 1/4`. ANSWER `0.75`, `3/4` or `6/8` is correct and returns reward +1, correctness=true, and canonical answer 3/4. `0.76` is a valid wrong answer and advances with −1. `1/0` is invalid and leaves index unchanged. With 100 net points in 300 seconds in integer mode, configured seconds per point is 3 and speed award 3.

**Overlap and replication limits.** Assessment arithmetic directly shares this generator and exact parser. It adds options, different budgets, family schedules and delayed grading, and defaults to +1/0 rather than +1/−1. The reference's question distribution, choice distractors, adaptive difficulty, Open Challenge history and complete original bank remain unknown. Keep the two scoring/navigation modes explicitly named when sharing components.

## 6. Market of Cards (`cards`)

**Published scope.** [Market of Cards](https://marketmakinggames.com/games/market-of-cards) describes private hands, progressively revealed center cards, rotating makers, and responding to bid/ask quotes. The local engine makes the accounting and fill rules explicit.

**Configuration and generation.** Defaults: 10,000 ms preparation, 20,000 quote timer, 12,000 response timer, three center cards, maximum spread 20. Human limit is 15. Use a complete standard 52-card deck with ranks A=1…K=13 and four suits. Offset `0xCA12` drives one shuffle. Humans are sorted for maker order. A lone human gets two uniquely named computer traders, avoiding collisions with the human's name. Deal two private cards per participant followed by the configured center. The constructor requires available deck capacity.

**Observation and hidden information.** State includes PREP/QUOTE/RESPONSE/DONE, timer, active maker, rotation/turn, participant order, center count/revealed count and revealed cards, quote if present, maximum spread, responder lock, requesting player's hand/book, and public fills. Opponent hands and unrevealed center cards stay hidden in snapshots. The UI renders conventional ranks/suits and patterned backs from a common card renderer; styling cannot reveal a concealed card.

**Actions.** In QUOTE only the active maker can submit integer `QUOTE {bid,ask}` satisfying bid≤ask and the spread cap. During RESPONSE, another participant can BUY at ask, SELL at bid or PASS; trades use quantity 1. The first valid trade wins the available fill and closes that market. Passes lock that responder without themselves being fills. Once capacity has gone, a racing order is rejected without another cash/position mutation. A responder cannot trade with their own quote. The maker's and responder's books are changed together under the session lock.

**Accounting and progression.** A responder purchase at p changes responder `(C,Q)` by `(−p,+1)` and maker by `(+p,−1)`; a sale reverses those signs. Every completed maker rotation reveals another center card. After the configured number of rotations, the local run ends with all center cards visible. Bots make quotes/respond using a model based on their own private cards and public information. Their model is an original counterpart policy, not inferred private source logic.

Settlement value V is the sum of every dealt private and center rank. Profit is C+QV. The sum of participant profits must be exactly zero; results fail defensively if conservation is violated. Ranking is shared by profit. Local global points count 2 per outranked bot and 3 per outranked human. Output includes settlement value, all hands/center, every player's cash/position/profit/rank/bot flag, and ledger.

**Worked example.** Suppose V=65. Alice makes 60/64, Bob buys one at 64. Bob ends that trade at C=−64,Q=1, and Alice C=64,Q=−1. At settlement Bob makes +1 and Alice −1. Carol racing to buy the same unit receives NO_CAPACITY. This is a terminal payoff claim, not immediate proof that Bob knew the hidden total when trading.

**Reproduction gaps.** The exact commercial preparation defaults, bot strategy, maximum-spread policy and tie/race semantics beyond the description are not recoverable. Freeze deck, deal order, maker order, public reveals, quote deadlines and server action order. A screenshot of visible hands does not preserve a complete table or a reproducible race.

## 7. The Marble Olympics (`marble`)

**Published scope.** [The Marble Olympics](https://marketmakinggames.com/games/marble-olympics) describes trading a simulated final using earlier results as evidence, a main product contract and time-limited side bets. The local simulation deliberately exposes a smaller declared model rather than claiming the reference's complete match/pause/window system.

**Configuration.** Defaults are 5,400,000 simulated milliseconds, playback multiplier 90, five history matches, six sides, 8,000 ms side start spacing, 6,000 ms window length and 5,000 ms main quote step. Thus the local 90-game-minute fixture plays in about 60 real seconds, not the reference's advertised real duration. Playback must be positive and simulated duration at least one minute. Separate simulated and real clocks in every event record.

**Evaluator and public input.** Offset `0xB7A1` generates four hidden rates for A/B × orange/red, each 0.3…1.2 per game minute. Independent exponential interarrival times generate goal tapes. Historical results supply public evidence. The observation contains current simulated match minutes, real remaining time, observed team/color totals, history, main quotes, side descriptions/status/open quotes and remaining window time, market version, own per-contract books, edge points and trades. It omits future goals and hidden rates.

A side is a fixed predicate on terminal counts, such as A≥7, B≥8, A-orange≥3 or combined total≥13. Every side pays 100 if true and 0 otherwise. The main contract pays final A total × final B total. Quote construction and its perturbations are frozen in the scenario tape. These original rate/threshold choices need not produce the same difficulty or score distribution as the reference site.

**Actions and evaluation.** `BUY` or `SELL` carries `{contract_id,quantity:1}`. MAIN is open while the game is live; a side requires its exact active window. Stale nonzero version tokens are rejected. Trades update C and Q in that contract's book. Main trades affect terminal profit but do not receive side edge points. A side trade is evaluated using its conditional fair payout at receive time, after any observed goals at that time.

Let E=100p and sd=100√(p(1−p)). Define d=E−ask for buying, or bid−E for selling. Local edge points are sign(d)×min(4,ceil(abs(d)/sd)); exactly zero d gives zero. At zero sd, favorable/unfavorable nonzero edge receives +4/−4. This binning is a disclosed local choice. End profit sums C+Q×contract payout. Multiplayer rank score is 2 per opponent with fewer edge points plus 1 per opponent with lower profit; equal rank score is broken by more edge points. Solo global output gives 1 for positive profit; commercial duration bonuses are not implemented.

**Worked example.** Final totals A=8,B=7 make MAIN pay 56. Buying at 53 yields +3 at terminal settlement. A side with conditional p=0.6 has fair payout 60 and sd≈48.99. Buying its ask 55 gives d=5 and local edge score +1. If its terminal predicate is false, its cash loss is still 55: an ex-ante favorable action need not win its one realized outcome.

**Unknowns and reductions.** The commercial strength prior, scoring-rate model, quote adaptation, detailed side catalogue, concurrent late windows, pauses and scrubber timeline are not recovered. This local model has sequential windows and no pause action. Exact conditional probability routines and any Poisson-tail truncation are source-version inputs; do not substitute Monte Carlo grading without changing the contract. Live probabilities can be shared with static assessments, while event windows and positions cannot.

## 8. ETF Trading Challenge (`etf`)

**Published scope.** [ETF Trading Challenge](https://marketmakinggames.com/games/etf-challenge) explains ETF-versus-basket arbitrage and instant multi-leg execution. The local version fixes one ETF with unit holdings so a fill is mechanically auditable.

**Configuration and input.** Defaults: 300,000 ms, three stocks, fee 0, quote step 5,000 ms, arbitrage probability 0.25 and news probability 0.35. Stock count is 2–4 and quote step at least 500 ms. Offset `0xE17` generates the complete quote tape. Every snapshot contains stock bid/ask pairs, ETF E1 bid/ask, event tag, version and remaining time, plus the candidate's profit components, number of fills and profitable fills. Composition is one share of every listed stock; no ambiguous weights or creation-unit conversion are hidden.

**Actions and atomic calculations.** `BUY_ETF {etf_id:"E1",quantity:1}` buys one ETF at its ask and sells one of each stock at bid. Profit is sum(stock bids)−ETF ask−fee. `SELL_ETF` sells at ETF bid and buys each constituent at ask, earning ETF bid−sum(stock asks)−fee. The entire cycle validates and executes on one snapshot; there is no partially filled basket or exposed residual position. Version equality is mandatory, including version zero.

After a successful fill, the tape cursor advances for everyone when a next snapshot exists. Scheduled quote progression can also advance it. An action based on an old snapshot must fail rather than fill at unseen prices. A tag other than news is recorded as normal for profit attribution. Both negative and positive cycles are permitted because recognizing a bad arbitrage is part of the task. Immediate receipts expose all legs, tag and profit; the observation updates total/news/normal P&L and accuracy.

**Scoring.** Rank by total P&L with shared ranks. Accuracy is strictly positive-profit fills / all fills; a zero-profit execution is not profitable. With no trades, accuracy and global points are zero. Multiplayer global points are `ceil((r_total+r_news+2*r_normal)*accuracy)`, where each r counts strictly lower corresponding P&L among opponents. Local solo global points remain zero. This secondary metric is separate from the actual currency/point profit and must not be substituted for it in a review.

**Worked example.** Two stocks quote 50/51 and 30/31; ETF quotes 77/79, fee 0. Buying the ETF cycle earns 50+30−79=1. Its receipt has BUY E1 at 79 and SELL S1/S2 at 50/30. Selling instead earns 77−51−31=−5. Changing fee to 1 makes the buy flat and therefore not a profitable fill for accuracy. Two candidates racing from one version cannot both assume that version remains executable after the first accepted reset.

**Missing commercial details.** Original price processes, news logic, trend persistence, reset/reduction algorithm, ETF composition policy, liquidity and transaction costs are not public. The local next-tape-snapshot reset is an original deterministic approximation. Synthetic equity assessment items share arithmetic and unit discipline, but introduce frozen accounting statements and valuation formulas rather than this fill-and-reset environment.

## 9. Next Card Betting (`nextcard`)

**Published scope.** [Next Card Betting](https://marketmakinggames.com/games/next-card) describes rank predictions, bankroll sizing, push outcomes and skill-based evaluation. The local model freezes all deck cycles and preallocates reference/draw cards without looking ahead in candidate snapshots.

**Configuration.** Defaults are four suits, aces low, `higher_lower`, bankroll 100, betting clock 20,000 ms, result clock `result_ms=6000`, one cycle, and max_rounds=0 (no explicit cap). Modes include higher/lower, inside/outside, and alternating. Suit count is 1–4. Offset `0xCAB0` shuffles each cycle. Higher/lower consumes a reference and draw; inside/outside consumes two references and a draw. Retain cycle identity and deck order when reconstructing remaining ranks.

**Visible versus hidden.** During BETTING, the candidate sees current references, mode, round count, time, bankroll, own acted lock and past visible hands. The next draw remains hidden. RESULT reveals the draw and that candidate's payoff/feedback; the completed snapshot remains readable and shows the final draw rather than indexing past the tape. Probability evaluation counts the remaining unseen ranks after all already exposed and current reference cards are removed. Do not use all thirteen ranks uniformly if cards have been seen or two references have different multiplicities.

**Actions and outcomes.** `BET {prediction,stake}` uses HIGHER/LOWER or INSIDE/OUTSIDE according to the round. Stake rounds to cents, is finite/positive, and cannot exceed bankroll. One accepted BET or PASS locks the round. Timeout without a decision is an implicit pass. Win adds stake, loss subtracts stake, equality with the relevant reference returns it. With two references, inside/outside refer to the strict interior/exterior and equality pushes. Explicit and implicit passes make no wager and receive outcome MISS with zero delta; a pass is not a losing zero-stake bet.

**Skill calculation.** For a direction with probabilities pW,pL,pP, the favorable even-money fraction is `max(0,(pW−pL)/(pW+pL))`, with zero when denominator is zero. The best positive-edge direction is the target. A decision is correct when betting that direction, or passing if neither side offers an edge. On a positive opportunity with target f*, sizing closeness is `max(0,1−abs(f−f*)/f*)`, otherwise zero for a missed opportunity. f uses stake divided by max(1, pre-bet balance) in the current legacy implementation. D is correct decisions / rounds; E is sizing sum / opportunity rounds; skill=3D+7E.

Outputs expose skill, D, E, final bankroll, shared rank (skill then bankroll), suit bonus and local global points. The suit bonus is suits−1. Bankrupt players get zero global points. The legacy global-point calculation is outranked count plus suit bonus (solo: bonus), rather than the reference's described skill-plus-bonus scalar; skill itself remains explicitly reported.

**Worked example.** With one suit and reference 9, if the unseen ranks contain eight lower and four higher, LOWER has pW=2/3, pL=1/3, f*=1/3. A 90 bankroll suggests a 30 target stake. Winning makes bankroll 120; losing 60; a permitted equal-rank push in a multisuit deck leaves 90. With six unseen ranks on each side of 7 and no other advantage, passing is decision-correct.

**Replication limits.** The exact commercial dealing/reuse convention, ace policy, cycle boundaries, feedback selection, decision-tie handling and global scoring details require explicit contracts. The local config field is `result_ms`, not Probability Betting's `result_timer_ms`. A new static card-probability assessment may reuse rank counting, but it must remove realized bankroll luck and Kelly sizing from ordinary answer accuracy.

## 10. Probability Betting (`prob`)

**Published scope.** [Probability Betting](https://marketmakinggames.com/games/probability-betting) describes quoted dice/coin/card opportunities, joint bet submission and Kelly-based decision/sizing feedback. The local engine fixes finite predicates and a declared scaling approximation.

**Configuration and scenario.** Defaults: eight rounds, bankroll 100, 30,000 ms betting, `result_timer_ms=8000`. Offset `0xF00B` selects a primitive and one or two predicates per round. Primitives include two fair dice summarized by sum (weights 1,2,3,4,5,6,5,4,3,2,1 over 36), one die, three fair coin tosses summarized by heads (1,3,3,1 over 8), and a card rank uniform over 13 equal rank classes. Predicates include sum/rank/face thresholds, equality, parity, face ranks or head counts.

The true event probability is the sum of matching outcome mass. Fair decimal odds are 1/p. Offered decimal odds are a rounded perturbation of fair odds with a lower bound above 1, saved in the tape. The realized primitive outcome uses cumulative categorical sampling from the declared masses (`categorical-cdf-v1`), not uniform selection among distinct aggregate descriptions. All correlated predicates share this single outcome. Both evaluator model and sampled outcome stay hidden while betting; only event text and offered odds are public.

**Actions and validation.** `SUBMIT_BETS {bets:[{event_id,stake},...]}` validates the entire collection atomically: every event exists, IDs are unique, each stake is finite/positive and rounds to cents, and total stakes do not exceed current bankroll. No event is partially reserved if another fails. Empty collection or PASS commits no bets. Each candidate may act once per round. Invalid submissions remain editable. All acted, or the betting deadline, leads to settlement, then a timed result phase.

**Payouts and decisions.** With offered decimal odds D, a win earns stake×(D−1), a loss loses stake. The multiplier includes return of the original stake; do not add the stake twice. Per-unit EV=p(D−1)−(1−p). Kelly target is max(0,EV/(D−1)). When the sum of positive target fractions exceeds 1, targets are scaled proportionally to sum to 1. This is an original approximation, not a full optimizer for correlated joint outcomes.

A decision is correct when staking a favorable event or leaving a nonfavorable event unbet. Sizing closeness compares each stake fraction against its scaled positive target; missed positive opportunities contribute zero, so selective participation cannot inflate E by excluding hard rounds. Final skill is 3D+7E, bankroll breaks skill ties. Round feedback includes p, fair/offered odds, EV, original/scaled Kelly, realized stake/outcome/profit and sizing score. Results expose those skill components. The local legacy global points use outranked counts plus a 0.5/1/1.5 length bonus for 5–6/7–8/9–10 rounds; they do not copy the reference's skill-plus-bonus global scalar.

**Worked example.** P(two-dice sum≥8)=15/36=5/12. At decimal odds 3, EV=(5/12)×2−7/12=1/4 and f*=1/8. With bankroll 100, target stake is 12.5. A win makes 25 profit and balance 125; a loss leaves 87.5. A static assessment asks for 5/12 or 1/4 without drawing a winner or choosing a stake, despite reusing the same primitive/predicate mathematics.

**Unresolved details.** Commercial event distributions, odds perturbations, dependence handling, target optimizer and exact skill/leaderboard formulas remain partly undisclosed. This local sampler version is essential: an older uniform-description sampler had the wrong frequency for dice sums/head counts and must not be confused with the corrected categorical model. Freeze outcomes and odds in a tape rather than regenerating them from a seed after a sampler change.

## 11. Fermi Questions (`fermi`)

**Published scope.** [Fermi Questions](https://marketmakinggames.com/games/fermi-questions) describes positive estimates, logarithmic closeness, a missing-answer penalty and explanatory reveals. Exact full scoring bins and truth-source policy are declared here for the local bank.

**Configuration and inputs.** Defaults: ten rounds, 60,000 ms per question, reveal_ms=8000. Round count must fit the 24-entry frozen bank. Offset `0xFE12` shuffles the bank indices, then selects the required prefix. Entries store stable ID, prompt, units, canonical numeric truth, explanation and source string. Time-varying facts must be treated as frozen reference quantities, not silently updated by a live search during a run. Several bank values are conventional approximations rather than exact natural constants; that context belongs in the prompt/review.

A QUESTION snapshot contains the current ID/prompt/units, round/total, clock, submitted-player names, own points and public leaderboard. It omits all submitted values and the true answer. A REVEAL snapshot includes the frozen question with its answer/explanation/source and rows for every player's estimate, multiplicative error, log10 error and points. Submitted status is public, not submitted content before closure.

**Actions and progression.** `ESTIMATE {question_id,value}` accepts one finite positive numeric value. The first valid answer locks the candidate; malformed, zero, negative and repeated submissions are rejected. When everyone has submitted or time runs out, close the round once, score, and start REVEAL. The local implementation moves on automatically after its reveal timer. This differs from the source page's described manual Next interaction. There is no editing after lock and no numeric answer penalty for merely malformed input.

For estimate x and truth y, symmetric multiplicative error is r=max(x/y,y/x). Local bins award 10 if r≤1.1, 8 if≤1.25, 6 if≤1.5, 4 if≤2, 2 if≤3, otherwise 0. A missing answer is −2. These inclusive boundaries are part of the declared implementation. Sum round points; rank by strictly larger total with shared ranks. Performance bonuses are 3 at total≥80, 2 at≥60, 1 at≥40, else 0. Multiplayer global points add outranked count; solo uses the performance bonus.

**Worked example and output.** With frozen truth 10,000, an estimate of 5,000 has r=2 and earns 4; 20,000 also has r=2 and earns 4. An estimate 11,000 has r=1.1 and earns 10. The log10 error for a factor 2 is approximately 0.3010. A blank is not a zero estimate: it produces null value and −2. Results give every player total/rank/global points and a complete reveal log.

**Overlap and missing details.** Maven intervals reuse selected positive truths and units but ask for bounds and reasoning, grading width/coverage rather than these point-error bins. The reference's exact source bank, answer updates, units, manual/automatic progression defaults, full boundary table and treatment of disputed estimates are not recovered. For reproducibility preserve the text and explanation alongside the number; changing an ambiguous geographic definition without changing the prompt changes the task.

## 12. Make Me a Market (`makeme`)

**Published scope.** [Make Me a Market](https://marketmakinggames.com/games/make-me-a-market) combines factual estimation, two-sided quotes, inventory, informed solo flow and questions about the candidate's book. The local engine simplifies several source interaction phases while fixing all evaluator quantities.

**Configuration.** Defaults: three maker rounds, estimate timer 30,000 ms, quote 20,000 ms, respond 12,000 ms, book-question 45,000 ms, spread_pct=0.15, min/max lots 1/10, reveal_factor=0.5, solo_pause_ms=1500. Offset `0xA6E7` selects one frozen factual MarketQ and a shown book-settlement z in the central portion of its public [0,U] domain. The spread limit is ceil(V×spread_pct), with V hidden. Human maker order is sorted.

**Candidate state.** Everyone sees the factual prompt, units, domain, cap, phase/clock and own quote/book points. Multiplayer ESTIMATE reveals only own lock status. QUOTE/RESPONSE exposes the public quote and requesting player's current cash/position. A response lock prevents a second action. SOLO_WAIT exposes the computer's last action. During BOOK, cash/position are hidden until that player pays the per-question reveal penalty. A revealed book remains in subsequent snapshots for that question and disappears again for the next one.

**Actions and execution.** Multiplayer begins with `LOCK_ESTIMATE {value}` and then rotates makers. `QUOTE {bid,ask,bid_size,ask_size}` requires positive bid, bid≤ask, width within cap and integer sizes within the lot range. Timely multiplayer quotes earn +2, missed turns −1. Other participants BUY at ask, SELL at bid, or PASS once each during RESPONSE. Each trade is one lot and consumes that side's remaining quoted capacity; the maker receives the opposite cash/position change. No participant should trade their own quote.

Solo starts directly in QUOTE. The informed computer sells to a bid above V, otherwise buys an ask below V, otherwise passes. It acts immediately, then the short solo pause leads to the next quote or BOOK. Local solo applies the same spread cap from the start; the source's uncapped first 95% interval and “Are you sure?” interaction are not implemented. The candidate's final-market accuracy bonus is therefore an original scoring reconstruction, not a claimed reproduction of those omitted decisions.

**Book questions and scoring.** There are four in order: signed position Q; worst loss over [0,U], `max(0,−min(C,C+QU))`; break-even −C/Q when Q≠0 and the value is inside the domain, otherwise NONE (or ANY when C=Q=0); and profit at shown z, C+Qz. `ANSWER_BOOK_QUESTION {answer:string}` advances once, correct or wrong. Numeric grading tolerance is max(0.02,1% of absolute truth); NONE/ANY are case-insensitive tokens. `REVEAL_HISTORY` halves the current question's remaining award and reveals the own book.

Each correct question earns 1 solo or 2 multiplayer, multiplied by reveal_factor if revealed. The original local estimation bonus uses symmetric multiplicative error with clipped log2 formulas: multiplayer clip(3−4log2(r),−8,3), solo clip(2−3log2(r),−5,2). Multiplayer total combines quote points, book points and estimate bonus; solo excludes quote points. Trading profit is a book truth, not the overall ranking score. Results expose factual answer/explanation, z, cash, position, point components, rank, local global points, book truths, market review and computer log.

**Worked example.** Buy two lots at 40 and sell one at 55: C=−25,Q=1. With U=100 and z=50, position=1, worst-case loss=25, break-even=25 and profit=25. Revealing history before a solo book answer makes it worth 0.5 instead of 1. A factual estimate V/2 and 2V have the same multiplicative error and estimation bonus.

**Replication gaps.** Source question banks, exact clipping formulas, first-quote interval handling, confirmation challenges, unlimited-size rooms, round-ready review, anonymization of order flow and bot tie behavior are not all public or implemented. The local tape contains selected question/config/domain/answer/z, but not a full action/response chronology. Sharing facts with interval assessments is useful; treating quote width as an 80% calibrated interval without a declared interval score is not.

## 13. Implementation versus source: explicit boundary

These profiles are detailed engineering contracts for original environments. The local defaults intentionally shorten some reference games. Several secondary leaderboard formulas are original proposals or known legacy divergences; use the published-source paragraph and the implemented-score paragraph separately. The suite does not claim that all private-site algorithms have been recovered.

For the nine trading games, a production-quality universal replay extension still needs a common versioned wrapper containing complete initial state, all resolved configuration, event ordering and timestamps, every accepted/rejected action, automatic transitions, evaluator versions, deterministic finalization and canonical results. It should add state/tape privacy enforcement before competitive deployment. Existing engine tests cover rational equivalence, stale snapshots, deadline behavior, deck conservation, zero-sum settlement, races, invalid estimates and deterministic scenario generation; they do not manufacture missing commercial evidence.

The new assessment runtime already implements the stricter completed-attempt tape and replay verifier described in the companion document. Its tests cover every firm preset in full and preview modes, exact-boundary clocks, revisable answer maps, no live answer-key access, proper cube rotations, interval loss, and independently enumerated grid EV. Reuse these contracts where mechanics match, preserve separate scoring where they do not, and version any change that affects what a candidate sees or how an action is evaluated.
