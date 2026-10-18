// Local design QA only: serves src/ with hot reload and proxies /api to the running stack on :8090.
// Sign in on http://localhost:8090 first; the session cookie is shared across localhost ports.
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));

export default defineConfig({
  root,
  plugins: [svelte(), tailwindcss()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://localhost:8090', changeOrigin: true, headers: { origin: 'http://localhost:8090' } }
    }
  }
});
