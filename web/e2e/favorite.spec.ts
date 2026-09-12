import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";

// E2E for issue #62 (favorite): the per-card heart toggle persists across a page
// reload, and the sidebar "只看收藏" facet narrows the grid to favorited assets.
// Self-provisioning via the REST API (like boards.spec.ts / palette.spec.ts do
// for canvas placement) so the spec is robust to whatever the deploy seed holds:
// it clears all favorites first, then favorites exactly one known asset.

const API = "/api/dam";

interface ApiAsset {
  id: string;
  name: string;
  display_name: string;
  favorite: boolean;
}

// listAssets fetches the live library (favorites included via the DTO).
async function listAssets(request: APIRequestContext): Promise<ApiAsset[]> {
  const res = await request.get(`${API}/assets?limit=200`);
  expect(res.ok()).toBeTruthy();
  return res.json();
}

// setFavorite favorites/unfavorites one asset through the batch endpoint (the
// same endpoint the per-card toggle uses with a one-id list).
async function setFavorite(request: APIRequestContext, id: string, favorite: boolean) {
  const res = await request.post(`${API}/batch/favorite`, {
    data: { asset_ids: [id], favorite },
  });
  expect(res.ok()).toBeTruthy();
}

// clearAllFavorites resets the shared library so a test starts from a known base.
async function clearAllFavorites(request: APIRequestContext): Promise<ApiAsset[]> {
  const assets = await listAssets(request);
  const favored = assets.filter((a) => a.favorite).map((a) => a.id);
  if (favored.length > 0) {
    const res = await request.post(`${API}/batch/favorite`, {
      data: { asset_ids: favored, favorite: false },
    });
    expect(res.ok()).toBeTruthy();
  }
  return assets;
}

// A grid card located by the name its meta row renders (display_name || name).
const card = (page: Page, name: string) =>
  page.getByTestId("asset-card").filter({ hasText: name });

test("favorite persists across a page reload", async ({ page, request }) => {
  const assets = await clearAllFavorites(request);
  expect(assets.length).toBeGreaterThan(0);
  const target = assets[0]!;
  const label = target.display_name || target.name;

  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  const heart = card(page, label).getByTestId("favorite-toggle").first();
  await expect(heart).toHaveAttribute("aria-pressed", "false");

  // Click the heart → optimistic favorite; the backend PATCH lands right after.
  await heart.click();
  await expect(heart).toHaveAttribute("aria-pressed", "true");

  // The favorite is server-persisted, not just optimistic UI state.
  await expect
    .poll(async () => {
      const after = await listAssets(request);
      return after.find((a) => a.id === target.id)?.favorite ?? false;
    })
    .toBe(true);

  // Reload the page: the heart is still filled from the fetched state.
  await page.reload();
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const heartAfter = card(page, label).getByTestId("favorite-toggle").first();
  await expect(heartAfter).toHaveAttribute("aria-pressed", "true");
  await page.screenshot({ path: "e2e/screenshots/favorite-persist.png" });
});

test("只看收藏 facet narrows the grid to favorited assets", async ({ page, request }) => {
  const assets = await clearAllFavorites(request);
  expect(assets.length).toBeGreaterThan(1); // need at least one unfavorited too
  const target = assets[0]!;
  const label = target.display_name || target.name;
  await setFavorite(request, target.id, true);

  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // Baseline: the grid shows the whole library (more than one card).
  const cards = page.getByTestId("asset-card");
  await expect.poll(async () => cards.count()).toBeGreaterThan(1);

  // Toggle "只看收藏" in the sidebar favorite facet (scoped by its own testid so
  // it never collides with the per-card heart toggles in the grid).
  const facet = page.getByTestId("favorite-facet");
  await facet.getByTestId("favorite-toggle").click();
  await expect(facet.getByText("只看收藏")).toBeVisible();

  // Only the favorited asset remains in the grid.
  await expect.poll(async () => cards.count()).toBe(1);
  await expect(card(page, label)).toBeVisible();
  await page.screenshot({ path: "e2e/screenshots/favorite-facet-filtered.png" });

  // Clearing the facet restores the full library.
  await facet.getByTestId("favorite-toggle").click();
  await expect(facet.getByText("全部素材")).toBeVisible();
  await expect.poll(async () => cards.count()).toBeGreaterThan(1);
});
