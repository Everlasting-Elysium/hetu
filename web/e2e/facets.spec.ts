import { test, expect } from "@playwright/test";

// Smoke tests for Issue #75: format (kind) + rating facets in the main library
// sidebar and the board asset panel. The E2E library is seeded deterministically
// by cmd/seede2e (11 assets: image=4, video=3, audio=2, document=1, model=1;
// img-1/img-2/vid-1 rated 5★; tag "hero" on img-1 + vid-1).

const API = "/api/dam";

// ---------------------------------------------------------------------------
// Pure API smoke tests — verify backend filtering logic without a browser.
// ---------------------------------------------------------------------------

test("GET /facets returns correct kind counts", async ({ request }) => {
  const res = await request.get(`${API}/facets`);
  expect(res.ok()).toBeTruthy();
  const { kinds } = await res.json();
  const m: Record<string, number> = Object.fromEntries(
    kinds.map((k: { kind: string; count: number }) => [k.kind, k.count]),
  );
  expect(m.image).toBe(4);
  expect(m.video).toBe(3);
  expect(m.audio).toBe(2);
  expect(m.document).toBe(1);
  expect(m.model).toBe(1);
});

test("?kind= returns only the requested kind", async ({ request }) => {
  const imgs = await (await request.get(`${API}/assets?kind=image&limit=50`)).json();
  expect(imgs.length).toBe(4);
  expect(imgs.every((a: { kind: string }) => a.kind === "image")).toBeTruthy();
});

test("?kind= multi-value composes with ?rating=", async ({ request }) => {
  const combo = await (
    await request.get(`${API}/assets?kind=video&rating=5&limit=50`)
  ).json();
  expect(combo.length).toBe(1);
  expect(combo[0].name).toBe("sunset-timelapse");
});

test("search endpoint accepts ?kind= and returns intersection", async ({ request }) => {
  const res = await (
    await request.get(`${API}/search?q=sunset&kind=image`)
  ).json();
  expect(res.length).toBe(1);
  expect(res[0].name).toBe("sunset-beach");
});

// ---------------------------------------------------------------------------
// UI: main library sidebar format + rating facets.
// ---------------------------------------------------------------------------

test("format facet: clicking a kind button toggles aria-pressed", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // FilterFacets renders one button per kind that has count > 0. In library view
  // only the sidebar mounts this component, so .first() is unambiguous.
  const imgBtn = page.getByRole("button", { name: /图片/ }).first();
  await expect(imgBtn).toHaveAttribute("aria-pressed", "false");

  await imgBtn.click();
  await expect(imgBtn).toHaveAttribute("aria-pressed", "true");
  await page.waitForTimeout(400); // let the filter animate
  await page.screenshot({ path: "e2e/screenshots/facet-format-image.png" });

  // Re-click clears the facet.
  await imgBtn.click();
  await expect(imgBtn).toHaveAttribute("aria-pressed", "false");
});

test("rating facet: 5th star sets label to '5 星以上', re-click clears", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // RatingStars renders star buttons with title="N 星". In library view only
  // the sidebar facet renders RatingStars (the board panel is not mounted).
  await page.getByTitle("5 星").first().click();
  await expect(page.getByText("5 星以上").first()).toBeVisible();
  await page.waitForTimeout(400);
  await page.screenshot({ path: "e2e/screenshots/facet-rating-5.png" });

  // Re-click the same star clears it (value === rating → emit 0).
  await page.getByTitle("5 星").first().click();
  await expect(page.getByText("全部星级").first()).toBeVisible();
});

test("format + rating facets compose simultaneously", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByRole("button", { name: /视频/ }).first().click();
  await page.getByTitle("5 星").first().click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: "e2e/screenshots/facet-video-5star.png" });

  await expect(
    page.getByRole("button", { name: /视频/ }).first(),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("5 星以上").first()).toBeVisible();
});

// ---------------------------------------------------------------------------
// UI: board asset panel — search + facets.
// Board panel is scoped by the aside that contains the search placeholder (the
// sidebar also renders an aside but without the search input).
// ---------------------------------------------------------------------------

test("board panel: search narrows assets; format facet narrows further", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // Create a board via UI → immediately navigates to the board canvas.
  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  const panel = page
    .locator("aside")
    .filter({ has: page.getByPlaceholder("搜索素材…") });

  // Search "sunset" → debounce 300ms + network → 2 matches.
  await panel.getByPlaceholder("搜索素材…").fill("sunset");
  await expect(panel.getByText(/素材 · 2/)).toBeVisible({ timeout: 6_000 });
  await page.screenshot({ path: "e2e/screenshots/board-search.png" });

  // Additionally restrict to image kind → 1 match (sunset-beach).
  await panel.getByRole("button", { name: /图片/ }).click();
  await expect(panel.getByText(/素材 · 1/)).toBeVisible({ timeout: 5_000 });
  await page.screenshot({ path: "e2e/screenshots/board-search-filter.png" });
});

test("board panel: filtered items are draggable; placement verified via API", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // Create a fresh board via UI.
  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  // Resolve the newest board's id (created_at DESC, so boards[0] is newest).
  const boards = await (await request.get(`${API}/boards`)).json();
  const boardId: string = boards[0].id;

  const panel = page
    .locator("aside")
    .filter({ has: page.getByPlaceholder("搜索素材…") });

  // Apply image format filter → 4 draggable source items.
  await panel.getByRole("button", { name: /图片/ }).click();
  await expect(panel.getByText(/素材 · 4/)).toBeVisible({ timeout: 5_000 });
  await expect(panel.locator("[draggable='true']").first()).toBeVisible();

  // Verify draggable count matches the expected kind count.
  const sources = panel.locator("[draggable='true']");
  await expect(sources).toHaveCount(4);

  // Canvas drag-drop with Konva is not reliably automatable via Playwright;
  // use the REST API instead — same approach as boards.spec.ts.
  const assets = await (
    await request.get(`${API}/assets?kind=image&limit=1`)
  ).json();
  const placed = await request.post(`${API}/boards/${boardId}/items`, {
    data: { asset_id: assets[0].id, x: 120, y: 120, w: 200, h: 150, rotation: 0, z: 0 },
  });
  expect(placed.ok()).toBeTruthy();

  // Confirm the board now has 1 item.
  const board = await (await request.get(`${API}/boards/${boardId}`)).json();
  expect(board.items?.length).toBe(1);
  expect(board.items[0].asset_id).toBe(assets[0].id);

  await page.screenshot({ path: "e2e/screenshots/board-filtered-drag.png" });
});
