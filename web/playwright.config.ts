import { defineConfig, devices } from "@playwright/test";

// E2E smoke config for the interactive 3D viewer (#51). Point BASE_URL at a
// running hetu instance (default: the docker-compose service on :8080). Run:
//   npx playwright test            (against an already-running server)
// The tests never start a server themselves so they can target a real deploy.
export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  reporter: [["list"]],
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:8080",
    headless: true,
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
