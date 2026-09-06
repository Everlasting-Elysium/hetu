import { defineConfig } from "@playwright/test";

// E2E tests run against a hetu server started externally (either `bin/hetu
// serve` or docker compose). The base URL defaults to the docker-compose port.
export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:8080",
    headless: true,
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "chromium", use: { browserName: "chromium" } },
  ],
});
