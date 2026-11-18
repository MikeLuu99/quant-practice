import { defineConfig } from 'astro/config';

// The Go API runs on :8080; the dev server proxies /api to it so the
// frontend works same-origin during development. The production build is
// served behind the same reverse proxy or alongside the Go server.
export default defineConfig({
  vite: {
    server: {
      proxy: {
        '/api': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
      },
    },
  },
});
