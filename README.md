# Quant Practice

A practice hub with nine market-making games, 25 company-assessment profiles and 22 trainers, with a **Go** backend API and an **Astro** frontend,
implemented from the "Market Making Games: game mechanics and reproducible input/output
specifications" research document (November 2026). The spec's *proposed reproduction
contracts* are treated as the engineering contract; unpublished site defaults are replaced
with the documented proposals.

## Quick start

```sh
make run        # builds frontend + backend, serves both on http://localhost:8080
```

Or manually:

```sh
cd frontend && bun install && bun run build
cd backend && go build -o bin/qserver ./cmd/server && ./bin/qserver
```

Development mode (hot reload): start the Go API (`STATIC_DIR="" ./bin/qserver`) and
`cd frontend && bun run dev` — the Astro dev server proxies `/api` to `:8080`.

```sh
make test       # go test ./... + bun test
```

## Architecture

```
backend/                     Go 1.26, standard library only
  cmd/server/                entry point; serves API + built frontend (STATIC_DIR)
  internal/engine/           shared core: SplitMix64 RNG, action envelopes,
                             Game interface, idempotent session store, registry
  internal/games/            the 9 engines + question banks + registry
  internal/api/              HTTP handlers, CORS, background ticking
frontend/                    Astro 5 (vanilla JS islands, no framework)
  src/lib/api.ts             API client + shared board controller (setup, polling, actions)
  src/lib/logic.ts           pure helpers (Kelly, Fermi table, fruit value) — bun-tested
  src/pages/games/*.astro    one page per game
```

Every session saves a **full event tape** (`GET /api/sessions/:id/tape`): PRNG version,
generated questions/decks/quotes/goals, and configuration. Replaying a tape reproduces the
run byte-for-byte (verified in tests). All commands are processed server-side with
idempotent `action_id` receipts; rejected actions never change state; stale
`market_version` clicks fail with `STALE_MARKET`.

## The nine games

| slug | game | solo bots? | notes |
|---|---|---|---|
| `fruit` | Fruit Market | — | value = mult × Σapples × Σoranges; first-click accuracy multiplies profit |
| `math` | Math Challenge | — | exact rational answers (`0.75 = 3/4`); speed thresholds per published table |
| `cards` | Market of Cards | 2 bots | maker rotation, first responder wins the fill, zero-sum settlement (asserted) |
| `marble` | The Marble Olympics | — | hidden Poisson rates, MAIN + side contracts, edge-point bins, windows |
| `etf` | ETF Trading Challenge | — | atomic ETF/basket cycles, quote reset after each fill |
| `nextcard` | Next Card Betting | — | higher/lower + inside/outside, pushes return stake, Kelly targets |
| `prob` | Probability Betting | — | enumerated exact probabilities, fair odds, scaled joint Kelly |
| `fermi` | Fermi Questions | — | multiplicative-error table, −2 for missing, shared ranks |
| `makeme` | Make Me a Market | informed bot | estimate → maker rotation → book questions, A2 estimation bonus |

Scoring follows the spec's **Appendix A** proposals (`skill = 3·D + 7·E` for betting
games; `clip(3−4·log₂ r, −8, 3)` multiplayer / `clip(2−3·log₂ r, −5, 2)` solo estimation
bonuses) and publishes raw components — never just a final scalar.

## API

| endpoint | description |
|---|---|
| `GET /api/games` | catalog of the 9 engines |
| `POST /api/sessions` | `{game, players, config?, seed?}` → session + initial snapshots |
| `GET /api/sessions/:id/state?player=` | merged public + private observation |
| `POST /api/sessions/:id/action` | action envelope → receipt (`accepted`, `reason`, `round_reward`, …) |
| `GET /api/sessions/:id/results` | per-player rows, raw score components, ranks, global points |
| `GET /api/sessions/:id/tape` | full reproducibility tape |

Rejection reasons: `LATE`, `STALE_MARKET`, `INVALID_VALUE`, `INSUFFICIENT_BALANCE`,
`ALREADY_ACTED`, `NO_CAPACITY`, `RATE_LIMIT`, `NOT_YOUR_TURN`, `MARKET_CLOSED`, …

## Tests

**Go** (`backend`): every engine has table + behavioral tests driven by an injectable
fake clock, covering the spec's Appendix B checks — idempotent retries, racing two
trades for one remaining lot, deadline submission, push outcomes, zero-trade
accuracy, card conservation, zero-sum peer-to-peer settlement (violations error out),
tape determinism, and snapshot privacy (opponent cards / hidden values never leak).

**Bun** (`frontend`): pure helper tests — clock formatting, stake parsing, fruit
valuation, both Kelly formulas (matching the spec's worked examples), Fermi scoring
table.

Run everything: `make test`.

## Interface

The responsive game suite follows the [Cobalt Grid design system](https://github.com/zarazhangrui/frontend-slides/blob/main/bold-template-pack/templates/cobalt-grid/design.md): cream graph paper, cobalt ink, editorial serif headlines, and fine rules. Card games use standard rank/suit faces, pip layouts, and concealed backs. The [Market Making Games catalogue](https://marketmakinggames.com/games) informed the table and phase layouts.

Start with one player to play solo (Cards adds two computer traders). For a shared-device game, enter unique comma-separated names and use the player selector. Session links preserve the current player and can be reloaded while the backend remains running. A new backend process starts with an empty in-memory session store.

Live polling updates the clock without replacing unchanged game controls. Drafts survive updates within a decision, and clear when the question, round, or player changes. Results and event tapes become available in the interface after the session ends.

## Company assessments

Open **`/assessment/`** for 25 format profiles across 11 firms, including classic/new Maven Numerical, Sequences and Probability, plus its related Confidence Intervals test. Preview, full and custom sessions share a server-timed runtime with autosaved drafts, forward/revisable navigation, delayed grading, explanations, and replay export/import. The original nine games remain available, with links to overlapping assessment modes.

Arithmetic shares Math Challenge's generator and exact rational parser. Probability shares the finite primitive/predicate evaluator with Probability Betting; interval estimates share Fermi facts. Sequences/anomalies, mixed reasoning, cube folding, equity statements and signed letter grids add original generators. Career Match is labelled a content demo and Grid payoff/timer choices are explicitly original; these presets are independent practice content, not employer replicas or endorsements.

Specifications are saved as [market_making_games_specification.md](market_making_games_specification.md) and [tradermath_assessments_specification_and_overlap.md](tradermath_assessments_specification_and_overlap.md). They document inputs, observations, private state, actions, deadlines, formulas, worked examples, source gaps, and implementation differences. `bun run build` copies them into the site's `/specifications/` downloads.

New endpoints: `GET /api/assessments` and `POST /api/assessments/replay`. Start through `POST /api/sessions` with `game:"assessment"` and `config:{profile_id,mode}`. Assessment results/tapes return **409 before completion**, including direct API requests.

Verify an exported assessment tape with the page's import control or:

```sh
cd backend
go run ./cmd/replay /path/to/tape.json
```

The verifier uses frozen questions and relative action times, validates receipts and transitions, and compares canonical results. Legacy trading tapes are scenario dumps with a different schema and are not accepted by this replay command. Assessment attempts remain in memory until backend restart.

Run `make test` for backend/frontend tests, and `cd frontend && bun run typecheck` for client TypeScript checks. Assessment tests exercise all 25 profiles in preview and full modes and verify privacy, deadlines, revisions, exact numeric parsing, interval loss, cube rotations, grid EV, and deterministic replay.


## Home and focused trainers

The site is named **Quant Practice**, with its existing grid mark as `/favicon.svg`.
The new home page `/` leads to `/games/`, `/assessment/` and `/trainer/`.
The nine-game catalogue now lives at `/games/`; existing individual game URLs stay valid.

The trainer page supports all 22 catalogue entries and their declared modes, using original
practice content. Numerical/probability/sequence/cube/estimation components reuse the games
and assessment engines. Cognitive tasks add phase-aware memory, reaction, timing and puzzle
state; the four trading trainers each have ten lessons with editable models, three ten-item
drills and an original nine-screen 45-minute case. Scores keep their own task meanings.

- `GET /api/trainers`: 22 profiles and modes.
- `GET /api/trainers/library`: 24 original Fermi prompts, without reference answers.
- `POST /api/sessions`: `game:"trainer"`, `config:{trainer_id,mode,...}`.
- `POST /api/trainers/replay`: verify a completed frozen scenario/action tape.

Trainer results/tapes are unavailable before completion. The existing replay CLI now handles
both assessment and trainer exports. Reaction times use local presentation/input timestamps,
are bounded/replayed, and are not independently authenticated. Refreshing or hiding the tab
marks an attempt as interrupted; the server session deadline continues. Sessions and course
state are held in memory until backend restart; recent result summaries are also saved in the
browser. There is no account-based course synchronization.

See [tradermath_trainers_specification_and_overlap.md](tradermath_trainers_specification_and_overlap.md)
for all 22 detailed input/output contracts, generation rules, mode settings, calculations,
worked examples, case accounting, overlap decisions and source gaps. Build copies this document
into `/specifications/` alongside the games and assessment specifications.

## Go on Cloudflare Workers

The new `backend/cmd/worker` entry point uses a pinned `syumai/workers-go` revision.
A Worker gateway serves the Astro build and routes each session to a separate
SQLite-backed Durable Object, with persisted request replay for restarts. The
existing local Go server remains supported.

See [the Workers setup guide](docs/cloudflare-workers.md) for builds, local Wasm
checks, staging/production configuration, GitHub Actions and deployment smoke tests.
The six requested web-quality skills are staged under `codex-skills/`; install them
and initialize Git from a normal terminal using `./scripts/setup-local.sh`.
The coding sandbox blocked Git metadata/global skill writes; these setup operations
were not completed automatically. No remote or Cloudflare deployment was created.
