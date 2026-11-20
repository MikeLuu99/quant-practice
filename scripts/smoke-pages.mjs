// Verifies a deployed Pages front end: static pages served, and /api/*
// proxied to the API Worker (which reports the deployed revision).
// QUANT_PAGES_URL = the pages.dev URL; QUANT_EXPECTED_REVISION optional.
import assert from 'node:assert/strict';
const base = process.env.QUANT_PAGES_URL;
if (!base) throw Error('Set QUANT_PAGES_URL to the deployed Pages URL.');
const url = new URL(base);
if (!['https:', 'http:'].includes(url.protocol)) throw Error('QUANT_PAGES_URL must be HTTP(S)');

async function get(path) {
  // Fresh Pages deployments briefly return transient 5xx (e.g. 522) while
  // the project's assets sync; retry a few times before failing.
  for (let attempt = 1; ; attempt++) {
    const response = await fetch(new URL(path, url), { signal: AbortSignal.timeout(30000) });
    if (response.ok) return response.text();
    const text = await response.text();
    if (response.status >= 500 && attempt < 6) {
      await new Promise((r) => setTimeout(r, 5000));
      continue;
    }
    throw Error(`${path}: ${response.status} ${text.slice(0, 200)}`);
  }
}

// A freshly created project can take a few seconds to activate its pages.dev
// hostname, so the whole check retries before failing.
let version = null;
for (let attempt = 1; ; attempt++) {
  try {
    version = JSON.parse(await get('/api/version'));
    break;
  } catch (e) {
    if (attempt >= 6) throw e;
    await new Promise((r) => setTimeout(r, 10_000));
  }
}
assert.equal(version.ok, true, '/api/version must report ok through the Pages proxy');
assert.match(version.revision ?? '', /^[0-9a-f]{7,40}|.+$/, 'revision present');
if (process.env.QUANT_EXPECTED_REVISION) {
  let observed = version.revision;
  for (let attempt = 1; observed !== process.env.QUANT_EXPECTED_REVISION && attempt <= 6; attempt++) {
    await new Promise((r) => setTimeout(r, 5000));
    observed = (JSON.parse(await get('/api/version'))).revision;
  }
  assert.equal(observed, process.env.QUANT_EXPECTED_REVISION, 'Stale API revision behind the Pages proxy');
}
for (const path of ['/', '/games/', '/games/fruit/', '/assessment/', '/trainer/', '/favicon.svg']) await get(path);
console.log(`Verified ${base} (pages) against API revision ${version.revision}`);
