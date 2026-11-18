# Quant Practice on Cloudflare Workers

The Go engines now have a separate Workers entry point, built with
[syumai/workers-go](https://github.com/syumai/workers-go), pinned to
`v0.36.1-0.20261101075724-e63084a9f753` (`e63084a9f753`). The ordinary local Go
server remains available. No container or Docker daemon is required for this target.

## Architecture

- Astro generates the existing home, games, assessments and trainer pages.
- `worker/index.mjs` serves those static files and routes `/api/*`.
- Public catalogues and replay verification use Go HTTP handlers through Wasm.
- Each session gets a random UUID and a distinct SQLite-backed `GameSession`
  Durable Object. Requests never depend on an in-memory map in a stateless Worker.
- The durable room appends each request, its server timestamp and a rules version
  to SQLite **before** executing the engine. A crash after the write is recovered
  by replaying that same request, preserving accepted action IDs and original clocks.
- Warm objects keep the Go engine in memory. On eviction/restart the journal
  rebuilds it. HTTP reads are logged too, because reads can advance bots/deadlines.
- Untimed trainers require no idle alarm. Timed trainers schedule their server
  deadline. Legacy games and assessments use approximately one-second alarms plus
  updates before incoming requests. Alarms are not precise display timers; action
  deadlines are still validated against the recorded server clock.
- Native server ticks at its existing rate; this adapter does not start a permanent
  ticking goroutine in the Workers runtime.

All nine games, 25 assessment profiles and 22 trainers use the same existing Go
engines. The frontend API URLs and request/action formats are preserved. Session
IDs in the Worker are opaque UUIDs; native development retains `s-1` style IDs.
Random server seeds are restricted to JavaScript's exact integer range for safe
JSON exports. Browser reaction times remain practice measurements, not authenticated
psychometric measurements.

## Git and Codex skills

The current agent workspace prevented `.git` writes and writes into the global
Codex skills folder. Run this once from an ordinary terminal:

```sh
cd /Users/user/Documents/quant-game
./scripts/setup-local.sh
```

It initializes the repository on `main` and installs the six requested
[web-quality skills](https://github.com/addyosmani/web-quality-skills) from the
reviewable copies in `codex-skills/`. Existing installed skills are preserved.
The skills become available on the next Codex turn. Their MIT license and source
attribution are included. No Git remote, initial commit, push or deployment is
performed by that script. Review `git status`, then make the initial commit and
connect your chosen GitHub repository. `.gitignore` excludes dependencies, generated
build output, local variables/secrets, and native binaries.

## Toolchain and local checks

Use Go 1.26, Bun 1.3.14 and a Node version supported by the pinned Cloudflare CLI:
Node ^22.23.0, ^24.22.0 or >=26.12.0. Cloudflare tooling runs on Node, not Bun.
The host used to implement this target had Node26.7 and could not install the
Cloudflare package graph through the restricted package manager, so the actual
Cloudflare bundle/dev-server and staging deployment remain to be run in that
supported environment. Do not treat the Node Wasm smoke test as workerd certification.

```sh
npm install
cd frontend
bun install --frozen-lockfile
cd ..
npm run test:worker
npm run build:worker
npm run test:wasm
npm run build:staging
npm run dev:worker
```

The first `npm install` creates a root `package-lock.json`; commit it, then use
`npm ci` in CI for a completely locked dependency graph. Direct Cloudflare/Vite
build-tool versions are pinned already. The existing frontend has its Bun lockfile.

- `npm run build:worker` generates the workers-go JS adapter and compiles
  `backend/cmd/worker` to `worker/build/app.wasm`.
- `npm run test:worker` exercises the real gateway routing with test namespace stubs.
- `npm run test:wasm` executes the actual generated Go/Wasm runtime in Node VM
  modules, using Node SQLite and an alarm adapter. It checks session creation,
  an answer, object reconstruction, duplicate receipts and completed tape access.
  Its measured latency is local Node evidence, not a production Workers benchmark.
- `npm run build:staging` builds Astro, Go Wasm and Cloudflare Build Output using
  the Cloudflare Vite plugin. Output belongs in `.cloudflare/output/v1/`.
- `npm run dev:worker` uses the generated Wasm and current built Astro files in a
  local Workers environment. Re-run the staging build when static pages change.
- Native checks remain `cd backend && go test ./... -count=1` and the frontend's
  `bun test`, `bun run typecheck`, `bun run build`.

The implementation smoke run produced an 8.11-MiB uncompressed Wasm binary.
Validate compressed upload size, Worker startup limits, memory and CPU under
`cf`/workerd and staging before making this the production target.

## Environments and deployment

`cloudflare.config.ts` uses the official **cf** CLI configuration; there is no
Wrangler configuration in this project. The Vite plugin packages the existing
Astro `frontend/dist` as static assets alongside the Worker and Wasm module.

| Mode | Worker name | Durable sessions |
|---|---|---|
| staging | quant-practice-staging | Staging namespace |
| production | quant-practice | Separate production namespace |

Both modes declare `GameSession` with SQLite storage. Cloudflare's typed
configuration manages class creation/migrations; do not rename the class or change
its storage engine without a migration plan. Preserve the session rules version
when changing only UI. Engine/generator changes require a compatible restore path
or an explicit new journal version, not silent reinterpretation of older logs.

For a local deploy after reviewing and testing the build:

```sh
npm run build:staging
npm run deploy:staging
```

Supply `CLOUDFLARE_ACCOUNT_ID` and `CLOUDFLARE_API_TOKEN` through your terminal's
secret/environment configuration. No secrets are committed. The `--prebuilt`
deploy commands use the already-built mode and do not rebuild a different revision.
To verify the deployed version:

```sh
QUANT_BASE_URL=https://YOUR-STAGING-WORKER.workers.dev node scripts/smoke-deployment.mjs
```

This check reads the build revision, creates a real trainer and assessment,
verifies session reads, and checks the home and practice routes. It creates real
practice records in the target namespace. Set `QUANT_EXPECTED_REVISION` to the
expected commit SHA for deployment gating.

## GitHub Actions

`.github/workflows/ci.yml` checks Go engines/durable restoration, gateway routing,
frontend tests/types, the real Wasm smoke and the Cloudflare bundle. It uploads
that exact mode's Cloudflare Build Output as an artifact. It receives no Cloudflare
deployment credentials.

`.github/workflows/deploy.yml` runs CI on pushes to `main`, then can deploy the
verified staging artifact. Automatic deployment is disabled until the repository
variable `CLOUDFLARE_DEPLOY_ENABLED` is set to `true`. Manual workflow dispatch can
select staging or production. For each GitHub environment, configure:

- Secrets: `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN`.
- Variable: `QUANT_BASE_URL`, the corresponding deployed public URL.

After your first successful local install, commit the root npm lockfile. The
workflows automatically use `npm ci` when that lockfile is present. Production and
staging have distinct names/storage. Branch PR builds do not receive deployment
secrets. No remote GitHub/Cloudflare resources or credentials were created by the
implementation task.

## Validation and known limits

Native tests verify reconstruction for all eleven engine slugs (nine games plus
assessment/trainer), trainer expiry after reconstruction, duplicate action receipts,
and journal identity/version/clock validation. A timed session retains its original
start time on restore. A stored request that the client retries after a crash is
replayed before the retry, allowing the original receipt to be returned.

This version is deliberately event-sourced. Rebuilding a long session costs time
proportional to its logged requests. Benchmark long assessments and rapid cognitive
sessions on workerd/staging; add versioned compact checkpoints before high-volume
production if recovery CPU or memory warrants them. Journals currently have no
automatic retention policy. The experimental workers-go Durable Objects API is
pinned; upgrades require running native and Wasm restoration tests again. Journal
state remains in the object's private SQLite database and is never returned through
live candidate snapshots.
