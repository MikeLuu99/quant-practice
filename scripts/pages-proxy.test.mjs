// Tests the generated Pages _worker.js proxy: /api/* and /healthz go to the
// baked API origin with method/headers/body preserved; everything else is
// served from static assets. Run after scripts/build-pages.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const workerPath = fileURLToPath(new URL('../.cloudflare/pages/staging/_worker.js', import.meta.url));
// The baked origin comes from the generator; tests assert against it.
const origin = (await readFile(workerPath, 'utf8')).match(/const API_ORIGIN = "([^"]+)"/)?.[1]
  ?? (() => { throw Error('generated _worker.js has no API_ORIGIN'); })();

async function loadWorker() {
  const src = await readFile(workerPath, 'utf8');
  // The bundle is ESM; evaluate it as a function body that returns the default export.
  const factory = new Function(src.replace(/export default/, 'return'));
  return factory();
}

function mockFetch(handler) {
  const calls = [];
  const original = globalThis.fetch;
  globalThis.fetch = async (input) => {
    const req = input instanceof Request ? input : new Request(input);
    calls.push(req);
    return handler(req);
  };
  return { calls, restore: () => { globalThis.fetch = original; } };
}

function fakeAssets(files = { '/index.html': '<html>home</html>' }) {
  return { fetch: async (request) => {
    const path = new URL(request.url).pathname;
    const body = files[path];
    return body === undefined ? new Response('not found', { status: 404 }) : new Response(body, { headers: { 'Content-Type': 'text/html' } });
  } };
}

test('worker was generated with the expected origin', async () => {
  const src = await readFile(workerPath, 'utf8');
  assert.ok(src.includes(origin), 'API origin must be baked in');
});

test('api paths proxy to the API origin preserving method, headers and body', async () => {
  const worker = await loadWorker();
  const { calls, restore } = mockFetch(() => new Response('{"ok":true,"revision":"abc"}', { headers: { 'X-Quant-Revision': 'abc' } }));
  try {
    const request = new Request(`https://pages.test/api/sessions`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"game":"fruit"}' });
    const response = await worker.fetch(request, { ASSETS: fakeAssets() });
    assert.equal(calls.length, 1);
    assert.equal(calls[0].url, `${origin}/api/sessions`);
    assert.equal(calls[0].method, 'POST');
    assert.equal(await calls[0].text(), '{"game":"fruit"}');
    assert.equal(response.status, 200);
    assert.equal(response.headers.get('Cache-Control'), 'no-store');
    assert.equal(response.headers.get('X-Quant-Revision'), 'abc');
  } finally { restore(); }
});

test('healthz and version are proxied; unknown static paths hit assets', async () => {
  const worker = await loadWorker();
  const { calls, restore } = mockFetch((req) => new Response(JSON.stringify({ ok: true, revision: 'x' })));
  try {
    const assets = fakeAssets({ '/': '<html>home</html>' });
    const health = await worker.fetch(new Request('https://pages.test/healthz'), { ASSETS: assets });
    assert.equal(calls[0].url, `${origin}/healthz`);
    assert.equal((await health.json()).ok, true);

    calls.length = 0;
    const home = await worker.fetch(new Request('https://pages.test/'), { ASSETS: assets });
    assert.equal(calls.length, 0, 'static requests must not be proxied');
    assert.equal(await home.text(), '<html>home</html>');
  } finally { restore(); }
});

test('proxy preserves upstream failure status codes', async () => {
  const worker = await loadWorker();
  const { restore } = mockFetch(() => new Response('{"error":"nope"}', { status: 404 }));
  try {
    const response = await worker.fetch(new Request('https://pages.test/api/games'), { ASSETS: fakeAssets() });
    assert.equal(response.status, 404);
  } finally { restore(); }
});
