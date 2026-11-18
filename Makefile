# Quant Practice — build, test, run.
.PHONY: test test-backend test-frontend test-worker-router build build-backend build-frontend run clean worker test-worker

test: test-backend test-frontend test-worker-router

test-backend:
	cd backend && go test ./... -count=1

test-frontend:
	cd frontend && bun test

build: build-frontend build-backend

build-backend:
	cd backend && go build -o bin/qserver ./cmd/server

build-frontend:
	cd frontend && bun install && bun run build

# Builds the Astro bundle, then serves frontend/dist + API from one Go origin.
run: build
	cd backend && ./bin/qserver

clean:
	rm -rf backend/bin frontend/dist frontend/node_modules

# Cloudflare Workers target; requires the root Node dependencies.
worker:
	npm run build:staging

test-worker:
	npm run test:worker
	npm run build:worker
	npm run test:wasm

test-worker-router:
	node --test worker/*.test.mjs
