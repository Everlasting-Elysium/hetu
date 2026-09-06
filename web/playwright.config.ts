import { defineConfig, devices } from "@playwright/test";

// E2E smoke config. Point BASE_URL (or E2E_BASE_URL) at a running hetu instance
// (default :8080). Tests never start a server themselves so they can target a
// real deploy — the model-viewer (#51) and multi-view (#52) smokes live in ./e2e.
const baseURL = process.env.BASE_URL ?? process.env.E2E_BASE_URL ?? "http://localhost:8080";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  reporter: [["list"]],
  use: {
    baseURL,
    headless: true,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
