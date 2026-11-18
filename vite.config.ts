import {defineConfig} from 'vite';
import {cloudflare} from '@cloudflare/vite-plugin';

// Astro has already generated these files. Vite packages them with the Go Wasm
// module and durable class into one versioned Cloudflare Build Output.
export default defineConfig({
 publicDir:'frontend/dist',
 plugins:[cloudflare()],
});
