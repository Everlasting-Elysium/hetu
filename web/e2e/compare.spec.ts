import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";

// E2E for issue #127 (image comparison UI): selecting exactly two library assets
// exposes a batch-bar "对比" button that opens the comparison page; running the
// comparison renders per-dimension score cards, a switchable overlay viewer, and
// an AI-critique panel that shows a guidance box when no VLM is configured (the
// default test-env state). Each test imports its own throwaway PNGs (canvas
// toBlob, random color for a unique content hash so /import never dedup-skips)
// and trashes them afterward, so it never disturbs other specs' fixtures.

const API = "/api/dam";

let imported: string[] = [];

// importFixture uploads a uniquely-colored 48×48 PNG through POST /import and
// returns its id + name, tracking the id for afterEach cleanup.
async function importFixture(page: Page): Promise<{ id: string; name: string }> {
  const name = `compare-fixture-${Date.now()}-${Math.floor(Math.random() * 1e6)}.png`;
  const color = `#${Math.floor(Math.random() * 0xffffff)
    .toString(16)
    .padStart(6, "0")}`;
  const id = await page.evaluate(
    async ({ name, color }) => {
      const c = document.createElement("canvas");
      c.width = 48;
      c.height = 48;
      const ctx = c.getContext("2d");
      if (!ctx) return "";
      ctx.fillStyle = color;
      ctx.fillRect(0, 0, 48, 48);
      const blob = await new Promise<Blob | null>((r) => c.toBlob(r, "image/png"));
      if (!blob) return "";
      const form = new FormData();
      form.append("file", blob, name);
      const res = await fetch("/api/dam/import", { method: "POST", body: form });
      const data = (await res.json()) as { asset?: { id: string } };
      return data.asset?.id ?? "";
    },
    { name, color },
  );
  expect(id, "import fixture asset").toBeTruthy();
  imported.push(id);
  return { id, name };
}

const card = (page: Page, name: string) =>
  page.getByTestId("asset-card").filter({ hasText: name });

// selectCard toggles a card's selection via its checkbox overlay (the same entry
// point the batch bar reacts to), located by the card's rendered name.
async function selectCard(page: Page, name: string) {
  await card(page, name).locator('button[title="选择"]').first().click();
}

test.beforeEach(async ({ page }) => {
  imported = [];
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
});

test.afterEach(async ({ request }: { request: APIRequestContext }) => {
  if (imported.length > 0) {
    await request.post(`${API}/batch/trash`, { data: { asset_ids: imported } });
  }
});

test("对比 button appears only when exactly two assets are selected", async ({ page }) => {
  const a = await importFixture(page);
  const b = await importFixture(page);
  const c = await importFixture(page);
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  const compareBtn = page.getByTestId("compare-button");

  // One selected: the batch bar shows, but comparison is binary — no 对比 button.
  await selectCard(page, a.name);
  await expect(page.getByText(/已选/)).toBeVisible();
  await expect(compareBtn).toHaveCount(0);

  // Two selected: the 对比 button appears.
  await selectCard(page, b.name);
  await expect(compareBtn).toBeVisible();

  // Three selected: it disappears again (regression guard).
  await selectCard(page, c.name);
  await expect(compareBtn).toHaveCount(0);
});

test("compares two assets: dimension cards, overlay modes, AI critique", async ({ page }) => {
  const a = await importFixture(page);
  const b = await importFixture(page);
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await selectCard(page, a.name);
  await selectCard(page, b.name);

  const compareBtn = page.getByTestId("compare-button");
  await expect(compareBtn).toBeVisible();
  await compareBtn.click();

  // The compare page opens with both slots naming the picked assets.
  const pageEl = page.getByTestId("compare-page");
  await expect(pageEl).toBeVisible();
  await expect(page.getByTestId("compare-slot-reference-name")).toContainText(".png");
  await expect(page.getByTestId("compare-slot-target-name")).toContainText(".png");
  await expect(pageEl).toContainText(a.name);
  await expect(pageEl).toContainText(b.name);

  // Run the comparison.
  await page.getByTestId("compare-run").click();

  // Per-dimension result: the overall total and the color card's ΔE metric.
  await expect(page.getByTestId("compare-overall")).toBeVisible();
  await expect(page.getByTestId("compare-overall")).toContainText(/\d/);
  await expect(page.getByTestId("compare-card-color")).toContainText("主色 ΔE");

  // Overlay viewer: switch side-by-side → onion, then the opacity slider is
  // present and operable (its value changes when dragged).
  await expect(page.getByTestId("compare-overlay")).toBeVisible();
  await expect(page.getByTestId("compare-overlay-mode")).toBeVisible();
  await page.getByTestId("compare-mode-onion").click();
  const slider = page.getByTestId("compare-onion-opacity");
  await expect(slider).toBeVisible();
  await slider.evaluate((el) => {
    const input = el as HTMLInputElement;
    input.value = "20";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await expect(slider).toHaveValue("20");

  // AI critique: no HETU_AI_VLM_MODEL in the test env → the guidance box, never
  // a blank or an error.
  const critique = page.getByTestId("compare-critique-unavailable");
  await expect(critique).toBeVisible();
  await expect(critique).toContainText("HETU_AI_VLM_MODEL");
  await page.screenshot({ path: "e2e/screenshots/compare-result.png" });
});
