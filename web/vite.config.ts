import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

// `vite build` empties dist/ (emptyOutDir defaults to true) — including the
// committed `web/dist/.gitkeep` placeholder that `//go:embed all:web/dist`
// depends on for a clean clone. Restoring it here (rather than only in the
// Makefile) keeps every build path honest: a bare `npm run build` and a
// `make build` both leave the placeholder behind, so no `git add -A` can
// silently re-open the clean-clone build break.
function restoreDistPlaceholder(): Plugin {
  return {
    name: 'auctionlab:restore-dist-placeholder',
    closeBundle() {
      writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '')
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [restoreDistPlaceholder(), react()],
  server: {
    // Fixed port + strictPort: the probe and the docs assume :5173, so a busy
    // port must fail fast instead of silently moving the URL.
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: false },
      '/ws': { target: 'ws://localhost:8080', ws: true },
      // Do NOT add rewriteWsOrigin: it opens CSRF, and gorilla's default
      // same-origin CheckOrigin on the Go side is a real defence here.
    },
  },
})
