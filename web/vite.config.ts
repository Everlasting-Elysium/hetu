import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Build output goes to dist/ (Vite default), which web/embed.go embeds via
// `go:embed all:dist`. The dev server proxies API + health to the Go backend
// so the SPA can run against `bin/hetu serve` on :8080.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "/healthz": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
