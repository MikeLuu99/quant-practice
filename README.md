# Quant Practice

A practice hub with nine market-making games, 25 company-assessment profiles and 22
trainers — **Go** backend API + **Astro** frontend, implemented from the market-making
games specification (November 2026). Unpublished site defaults are replaced with the
spec's documented proposals.

## Quick start

```sh
make run     # builds frontend + backend, serves both on http://localhost:8080
make test    # go test ./... + bun test
```

Dev mode: `STATIC_DIR="" ./bin/qserver` + `cd frontend && bun run dev` (proxies `/api` to `:8080`).

## Structure

- `backend/internal/engine` — shared core: SplitMix64 RNG, action envelopes, idempotent session store
- `backend/internal/games` — the 9 game engines, assessment + trainer tasks, question banks
- `backend/internal/api` — HTTP handlers; `cmd/replay` verifies exported session tapes
- `frontend/` — Astro 5 pages: `/games/`, `/assessment/`, `/trainer/`

Every session saves a full event tape (`GET /api/sessions/:id/tape`) that replays
byte-for-byte (verified in tests). Actions are idempotent; rejected actions never change
state. Solo play works out of the box (Cards adds two bots); multiplayer takes
comma-separated players and the acting-player switch.

## Games

| slug | game | notes |
|---|---|---|
| `fruit` | Fruit Market | value = mult × Σapples × Σoranges; first-click accuracy multiplies profit |
| `math` | Math Challenge | exact rational answers (`0.75 = 3/4`); published speed thresholds |
| `cards` | Market of Cards | maker rotation, first responder wins, zero-sum settlement |
| `marble` | The Marble Olympics | hidden Poisson rates, MAIN + side contracts, edge-point bins |
| `etf` | ETF Trading Challenge | atomic ETF/basket cycles, quote reset after each fill |
| `nextcard` | Next Card Betting | higher/lower + inside/outside, pushes return stake, Kelly targets |
| `prob` | Probability Betting | enumerated exact probabilities, scaled joint Kelly |
| `fermi` | Fermi Questions | multiplicative-error table, −2 for missing |
| `makeme` | Make Me a Market | estimate → maker rotation → book questions, A2 estimation bonus |

Scoring publishes raw components (decision/sizing fractions, accuracy, edge points), not
just a final scalar.

## API

- `GET /api/games` — engine catalog
- `POST /api/sessions` — `{game, players, config?, seed?}`; also `game:"assessment"` / `game:"trainer"`
- `GET /api/sessions/:id/state?player=` — merged public + private observation
- `POST /api/sessions/:id/action` — receipt: `accepted`, `reason`, `round_reward`, …
- `GET /api/sessions/:id/results` / `GET /api/sessions/:id/tape` — 409 before completion
- `GET /api/assessments`, `GET /api/trainers`, replay endpoints, `go run ./cmd/replay <tape.json>`

## Docs

- [Assessments + games specification](market_making_games_specification.md) and
  [assessment overlap](tradermath_assessments_specification_and_overlap.md)
- [Trainers specification](tradermath_trainers_specification_and_overlap.md)
- [Cloudflare Workers deployment](docs/cloudflare-workers.md) — Worker gateway, Durable Objects, CI

All three are copied to the site's `/specifications/` downloads at build time.
