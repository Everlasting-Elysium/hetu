import { test, expect } from "@playwright/test";

// Smoke tests for Issue #75 (format/rating/tag facet plumbing) and #108
// (facets consolidated into the global sidebar, context-routed to either the
// full library or the board panel's query). The E2E library is seeded
// deterministically by cmd/seede2e (11 assets: image=4, video=3, audio=2,
// document=1, model=1; img-1/img-2/vid-1 rated 5★; tag "hero" on img-1 + vid-1).

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
// UI: board asset panel + sidebar facets (issue #108).
// BoardAssetPanel no longer mounts its own FilterFacets — folder/tag/format/
// star filtering moved to the global sidebar, which routes to the board-local
// query while `view === "board"` and must never navigate away from the
// canvas. Only the keyword search stays local to the panel. The panel is
// scoped by the aside that contains the search placeholder (the sidebar also
// renders an aside but without a search input); since the panel no longer
// duplicates the format/rating buttons, the sidebar's copies are unambiguous
// even while a board is open — no `.first()` needed here (contrast the
// library-view tests above, which keep `.first()` since #75).
// ---------------------------------------------------------------------------

test("board panel: search narrows assets locally", async ({ page }) => {
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
});

test("board sidebar: format + rating facets narrow the board panel, stay on board", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  const panel = page
    .locator("aside")
    .filter({ has: page.getByPlaceholder("搜索素材…") });

  // Restrict to image kind via the sidebar → 4 draggable source items in the
  // panel, and the board toolbar must stay put (the #108 bug this fixes: the
  // old lq-bound facet setter used to bounce the view back to a browse layout).
  const imgBtn = page.getByRole("button", { name: /图片/ });
  await imgBtn.click();
  await expect(imgBtn).toHaveAttribute("aria-pressed", "true");
  await expect(panel.getByText(/素材 · 4/)).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("返回图板列表")).toBeVisible();
  await page.screenshot({ path: "e2e/screenshots/board-sidebar-format-filter.png" });

  // Clear the format facet, then apply the rating facet alone → 3 assets are
  // rated 5★ in the seed (img-1, img-2, vid-1), spanning two kinds.
  await imgBtn.click();
  await page.getByTitle("5 星").click();
  await expect(page.getByText("5 星以上")).toBeVisible();
  await expect(panel.getByText(/素材 · 3/)).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("返回图板列表")).toBeVisible();
  await page.screenshot({ path: "e2e/screenshots/board-sidebar-rating-filter.png" });

  await page.getByTitle("5 星").click(); // clear for the next test
});

test("board sidebar: 标签 facet narrows the board panel, stays on board", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  const panel = page
    .locator("aside")
    .filter({ has: page.getByPlaceholder("搜索素材…") });

  // Sidebar renders its own CRUD-capable tag list (not FilterFacets' chips),
  // so it carries no aria-pressed — assert via the resulting panel count.
  const heroTag = page.locator("button").filter({ hasText: "hero" }).first();
  await heroTag.click();
  await expect(panel.getByText(/素材 · 2/)).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("返回图板列表")).toBeVisible();
  await page.screenshot({ path: "e2e/screenshots/board-sidebar-tag-filter.png" });

  // Re-click clears it (useBoardPanelQuery.pickTag toggles on re-pick, #108).
  await heroTag.click();
  await expect(panel.getByText(/素材 · 2/)).toBeHidden();
});

test("board sidebar filter → drag into canvas → switch to library scopes the full library", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  // Resolve the newest board's id (created_at DESC, so boards[0] is newest).
  const boards = await (await request.get(`${API}/boards`)).json();
  const boardId: string = boards[0].id;

  const panel = page
    .locator("aside")
    .filter({ has: page.getByPlaceholder("搜索素材…") });

  // Narrow the panel to images via the sidebar → 4 draggable source items.
  const imgBtn = page.getByRole("button", { name: /图片/ });
  await imgBtn.click();
  await expect(panel.getByText(/素材 · 4/)).toBeVisible({ timeout: 5_000 });
  const sources = panel.locator("[draggable='true']");
  await expect(sources).toHaveCount(4);

  // Canvas drag-drop with Konva is not reliably automatable via Playwright;
  // place the filtered asset via the REST API instead — same substitute
  // boards.spec.ts already uses.
  const assets = await (await request.get(`${API}/assets?kind=image&limit=1`)).json();
  const placed = await request.post(`${API}/boards/${boardId}/items`, {
    data: { asset_id: assets[0].id, x: 120, y: 120, w: 200, h: 150, rotation: 0, z: 0 },
  });
  expect(placed.ok()).toBeTruthy();
  const board = await (await request.get(`${API}/boards/${boardId}`)).json();
  expect(board.items?.length).toBe(1);
  expect(board.items[0].asset_id).toBe(assets[0].id);
  await page.screenshot({ path: "e2e/screenshots/board-filtered-drag.png" });

  // "全部素材" clears filters and leaves the board — pre-existing nav
  // semantics this issue does not touch (design decision 4/(a) only reroutes
  // facet *clicks* to the board query; it does not change what the "全部
  // 素材" nav button does). So the facet starts unpressed here…
  await page.getByText("返回图板列表").click();
  await expect(page.getByText("新建图板")).toBeVisible();
  await page.getByText("全部素材", { exact: true }).click();
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await expect(imgBtn).toHaveAttribute("aria-pressed", "false");

  // …and clicking it now scopes the *full library* grid, the same FilterFacets
  // instance App re-points at `lq` once `view !== "board"`.
  await imgBtn.click();
  await expect(imgBtn).toHaveAttribute("aria-pressed", "true");
  await page.waitForTimeout(400);
  await page.screenshot({ path: "e2e/screenshots/board-sidebar-filter-after-library.png" });
});
