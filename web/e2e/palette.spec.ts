import { expect, test } from "@playwright/test";

// Smoke tests for issue #88: extracted-palette swatches in the detail surfaces
// (double-click modal + single-click inspector) and click-to-filter by color.
// The E2E library is seeded by the deploy step with distinct-color assets:
//   img-red-1.png / img-red-2.png (red)  img-blue.png (blue)  img-green.png (green)
//   vid-red.mp4 (red video, palette via ffmpeg)  cube.glb (red, palette via thumb upload)
// Clicking a red swatch must surface every red asset (image + video + model),
// proving palette extraction spans all three kinds and click-to-filter reuses
// the existing CIEDE2000 color search.

const SHOT = process.env.SHOT_DIR ?? "test-results/palette";

// A grid card by asset name (the meta row renders display_name || name).
const card = (page: import("@playwright/test").Page, name: string) =>
  page.getByTestId("asset-card").filter({ hasText: name });

// Swatches inside the full-screen detail modal (scoped to its overlay so the
// inspector's swatches — a double-click also selects — never match here).
const modalSwatches = (page: import("@playwright/test").Page) =>
  page.locator('[class*="overlay"] [class*="paletteSwatch"]');

// Distance badges appear only on color-search result cards, so their presence
// proves the grid switched into color-filter mode.
const distanceBadges = (page: import("@playwright/test").Page) =>
  page.locator('[class*="distance"]');

async function openDetailAndFilter(
  page: import("@playwright/test").Page,
  name: string,
  shotPrefix: string,
) {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  const c = card(page, name).first();
  // Select first so the one-time inspector/batch-bar mount happens now. A bare
  // double-click races that mount re-render — it lands between the two clicks and
  // the browser drops the dblclick (see AssetCard) — so pre-selecting (a plain
  // click replaces the selection) makes opening the modal deterministic.
  await c.click();
  await expect(
    page.locator('[class*="paletteRow"] [class*="paletteSwatch"]').first(),
  ).toBeVisible({ timeout: 10_000 });

  await c.dblclick();

  // The detail modal opens; assert its overlay container first so a same-named
  // swatch elsewhere can never stand in for a modal that failed to open.
  await expect(page.locator('[class*="overlay"]')).toBeVisible({ timeout: 10_000 });

  // The detail modal shows the extracted palette as a row of swatches.
  const swatches = modalSwatches(page);
  await expect(swatches.first()).toBeVisible({ timeout: 10_000 });
  await page.waitForTimeout(400); // let the pop animation settle for clean evidence
  await page.screenshot({ path: `${SHOT}/${shotPrefix}-detail.png` });

  // Click the first (dominant) swatch → color search; the modal closes so the
  // filtered grid behind it becomes visible.
  await swatches.first().click();
  await expect(page.locator('[class*="overlay"]')).toHaveCount(0, { timeout: 10_000 });

  // The grid is now color-filtered: result cards carry a ΔE distance badge.
  await expect(distanceBadges(page).first()).toBeVisible({ timeout: 10_000 });
  await page.screenshot({ path: `${SHOT}/${shotPrefix}-filtered.png` });

  // Regression (issue #88 "回闪没"): the color filter must PERSIST past the
  // keyword-search debounce (~300ms). A prior bug re-ran SearchBar's keyword
  // debounce on every render and cleared colorHex right after the swatch applied,
  // so the filtered grid flashed then reverted to the full list. Wait past the
  // debounce window and re-assert the filter still holds.
  await page.waitForTimeout(700);
  await expect(distanceBadges(page).first()).toBeVisible();
}

test("image detail: palette swatch → red grid filter", async ({ page }) => {
  await openDetailAndFilter(page, "img-red-1.png", "image");

  // Red matches every red asset; a blue one must be excluded.
  await expect(card(page, "img-red-1.png")).toBeVisible();
  await expect(card(page, "img-red-2.png")).toBeVisible();
  await expect(card(page, "img-blue.png")).toHaveCount(0);
});

test("video detail: palette swatch → red grid filter", async ({ page }) => {
  await openDetailAndFilter(page, "vid-red.mp4", "video");

  // The video's own red palette (extracted from an ffmpeg keyframe) filters to reds.
  await expect(card(page, "vid-red.mp4")).toBeVisible();
  await expect(card(page, "img-blue.png")).toHaveCount(0);
});

test("model detail: palette swatch → red grid filter", async ({ page }) => {
  await openDetailAndFilter(page, "cube.glb", "model");

  // The model's palette came from its uploaded thumbnail (no Blender locally),
  // proving the 3D thumb-upload re-extraction path (issue #88).
  await expect(card(page, "cube.glb")).toBeVisible();
  await expect(card(page, "img-blue.png")).toHaveCount(0);
});

test("inspector: single-click shows palette swatches", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // Single click selects the asset → the right inspector mounts with its own
  // palette row (scoped by the paletteRow container, distinct from the modal).
  await card(page, "img-green.png").first().click();
  const inspectorSwatches = page.locator('[class*="paletteRow"] [class*="paletteSwatch"]');
  await expect(inspectorSwatches.first()).toBeVisible({ timeout: 10_000 });
  await page.screenshot({ path: `${SHOT}/inspector-green.png` });

  // Clicking the green swatch filters the grid to green (excludes red/blue).
  await inspectorSwatches.first().click();
  await expect(distanceBadges(page).first()).toBeVisible({ timeout: 10_000 });
  await expect(card(page, "img-green.png")).toBeVisible();
  await expect(card(page, "img-red-1.png")).toHaveCount(0);
  await page.screenshot({ path: `${SHOT}/inspector-green-filtered.png` });

  // Regression (issue #88 "回闪没"): the filter must persist past the keyword
  // debounce window instead of flashing then reverting to the full list.
  await page.waitForTimeout(700);
  await expect(distanceBadges(page).first()).toBeVisible();
  await expect(card(page, "img-red-1.png")).toHaveCount(0);
});
