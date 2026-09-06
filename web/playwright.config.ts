import { defineConfig, devices } from "@playwright/test";

// The Go binary serves the SPA + API on one origin. Point the smoke test at a
// running `bin/hetu serve` (override with E2E_BASE_URL). No webServer is spawned
// here — the orchestrator boots the backend with seeded test assets.
const baseURL = process.env.E2E_BASE_URL ?? "http://localhost:8080";

export default defineConfig({
  testDir: "./tests",
  outputDir: "./tests/.output",
  reporter: [["list"]],
  timeout: 30_000,
  use: {
    baseURL,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
