import { test, expect } from "@playwright/test";

// Smoke tests for issue #101: shape (aspect-ratio) + pixel-dimension + file-size
// facets, layered on the #75 facet pipeline (facets.spec.ts). This suite targets
// a DEDICATED scratch server/library (see the PR description for the exact
// setup) seeded with 5 purpose-built fixtures whose width/height/size are
// deterministic:
//   dim-landscape.png  1600x800   5,694B   (ratio 2.0  -> landscape)
//   dim-portrait.png   800x1600   7,294B   (ratio 0.5  -> portrait)
//   dim-square.png     1000x1000  5,220B   (ratio 1.0  -> square)
//   size-small.png     200x200    595B     (ratio 1.0  -> square, tiny file)
//   size-large.png     1400x1400  5,885,411B ≈5.6MB (ratio 1.0 -> square, big file)
// Run against BASE_URL pointing at that scratch server (not the shared :8080
// dev server, to avoid interaction with other suites' fixtures/state).

const API = "/api/dam";

// ---------------------------------------------------------------------------
// Pure API smoke tests — verify backend filtering logic without a browser.
// ---------------------------------------------------------------------------

test("?shape= returns only the requested aspect-ratio bucket", async ({ request }) => {
  const landscape = await (await request.get(`${API}/assets?shape=landscape`)).json();
  expect(landscape.length).toBe(1);
  expect(landscape[0].name).toBe("dim-landscape.png");

  // Three assets share ratio 1.0 (square): dim-square, size-small, size-large.
  const square = await (await request.get(`${API}/assets?shape=square`)).json();
  expect(square.length).toBe(3);
});

test("?minWidth=/?maxHeight= narrow on pixel dimensions", async ({ request }) => {
  const wide = await (await request.get(`${API}/assets?minWidth=1000`)).json();
  expect(wide.length).toBe(3); // dim-landscape(1600) + dim-square(1000) + size-large(1400)
  const short = await (await request.get(`${API}/assets?maxHeight=250`)).json();
  expect(short.length).toBe(1); // only size-small (height 200) is that short
  expect(short[0].name).toBe("size-small.png");
});

test("?minSize=/?maxSize= narrow on file bytes, independent of dimensions", async ({
  request,
}) => {
  const big = await (await request.get(`${API}/assets?minSize=1048576`)).json();
  expect(big.length).toBe(1);
  expect(big[0].name).toBe("size-large.png");
  const tiny = await (await request.get(`${API}/assets?maxSize=1000`)).json();
  expect(tiny.length).toBe(1);
  expect(tiny[0].name).toBe("size-small.png");
});

test("shape + size compose (AND): square AND <=1MB excludes the big square file", async ({
  request,
}) => {
  const res = await (
    await request.get(`${API}/assets?shape=square&maxSize=1048576`)
  ).json();
  const names = res.map((a: { name: string }) => a.name).sort();
  expect(names).toEqual(["dim-square.png", "size-small.png"]);
});

// ---------------------------------------------------------------------------
// UI: main library sidebar — shape / dimension / file-size facets.
// ---------------------------------------------------------------------------

test("shape facet: toggling 横向 narrows the grid and sets aria-pressed", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  const landscapeBtn = page.getByRole("button", { name: /横向/ });
  await expect(landscapeBtn).toHaveAttribute("aria-pressed", "false");

  await landscapeBtn.click();
  await expect(landscapeBtn).toHaveAttribute("aria-pressed", "true");
  await page.waitForTimeout(400);
  await expect(page.locator("[data-testid='grid-view'] >> img, [data-testid='grid-view'] svg")).toHaveCount(1, {
    timeout: 5000,
  });
  await page.screenshot({ path: "e2e/screenshots/dim-facet-shape-landscape.png" });

  // Re-click clears it.
  await landscapeBtn.click();
  await expect(landscapeBtn).toHaveAttribute("aria-pressed", "false");
});

test("shape facet: multi-select OR (横向+纵向) widens back out", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByRole("button", { name: /横向/ }).click();
  await page.getByRole("button", { name: /纵向/ }).click();
  await page.waitForTimeout(400);
  await expect(page.getByRole("button", { name: /横向/ })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: /纵向/ })).toHaveAttribute("aria-pressed", "true");
  await page.screenshot({ path: "e2e/screenshots/dim-facet-shape-multi.png" });

  // 清除 clears every selected shape at once.
  await page.getByRole("button", { name: "清除" }).first().click();
  await expect(page.getByRole("button", { name: /横向/ })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByRole("button", { name: /纵向/ })).toHaveAttribute("aria-pressed", "false");
});

test("dimension facet: typing a min width narrows after the debounce settles", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByLabel("最小宽（px）").fill("1000");
  // RangeField debounces the commit ~300ms; wait past it before asserting.
  await page.waitForTimeout(500);
  await page.screenshot({ path: "e2e/screenshots/dim-facet-width.png" });

  // Clearing dimensions resets the input back to empty (0 = unbounded). Only
  // the 尺寸 section has an active filter here, so its 清除 is the sole match.
  await page.getByRole("button", { name: "清除" }).first().click();
  await expect(page.getByLabel("最小宽（px）")).toHaveValue("");
});

test("file-size facet: 1–10MB preset isolates the one big fixture", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByRole("button", { name: "1–10MB" }).click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: "e2e/screenshots/dim-facet-size-preset.png" });

  await page.getByRole("button", { name: "清除" }).last().click();
});

test("shape + file-size facets compose simultaneously (combination)", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByRole("button", { name: /方形/ }).click();
  await page.getByRole("button", { name: "≤ 1MB" }).click();
  await page.waitForTimeout(400);
  await expect(page.getByRole("button", { name: /方形/ })).toHaveAttribute("aria-pressed", "true");
  await page.screenshot({ path: "e2e/screenshots/dim-facet-combo.png" });
});

test("clearing shape does not clear an independently-set dimension filter", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByRole("button", { name: /方形/ }).click();
  await page.getByLabel("最小宽（px）").fill("500");
  await page.waitForTimeout(500);

  // Clear only the 形状 section (its own 清除 button, not 尺寸's).
  await page.getByRole("button", { name: "清除" }).first().click();
  await expect(page.getByRole("button", { name: /方形/ })).toHaveAttribute("aria-pressed", "false");
  // The width filter the user typed is untouched by the shape clear.
  await expect(page.getByLabel("最小宽（px）")).toHaveValue("500");
});

// ---------------------------------------------------------------------------
// UI: board view — shape/dimension/size facets now live in the GLOBAL SIDEBAR
// (issue #108 routed all board-panel filtering there, in place, without
// leaving the canvas), narrowing BoardAssetPanel's drag-source list, which
// keeps only its own search box. So these tests click the sidebar while
// `view === "board"` and assert on the panel's own aside (scoped by its
// search placeholder, which is unambiguous since the panel itself renders no
// facet controls anymore).
// ---------------------------------------------------------------------------

test("board view: sidebar shape + dimension facets narrow the drag source panel", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  const panel = page.locator("aside").filter({ has: page.getByPlaceholder("搜索素材…") });

  // The sidebar facet lives outside the panel's aside; clicking it must keep
  // the user on the board (issue #108's whole point) while narrowing the panel.
  await page.getByRole("button", { name: /横向/ }).click();
  await page.waitForTimeout(400);
  await expect(page.getByText("返回图板列表")).toBeVisible();
  await expect(panel.getByText(/素材 · 1/)).toBeVisible({ timeout: 5_000 });
  await page.screenshot({ path: "e2e/screenshots/dim-board-shape.png" });

  await page.getByRole("button", { name: /横向/ }).click(); // clear shape
  await page.getByLabel("最小宽（px）").fill("1000");
  await page.waitForTimeout(500);
  await expect(page.getByText("返回图板列表")).toBeVisible();
  await expect(panel.getByText(/素材 · 3/)).toBeVisible({ timeout: 5_000 });
  await page.screenshot({ path: "e2e/screenshots/dim-board-dimensions.png" });

  // Leaving the board and clearing brings the sidebar back to acting on the
  // full library (regression guard for the #108 routing switch).
  await page.getByRole("button", { name: "清除" }).first().click();
});

test("board view: sidebar file-size facet narrows a draggable, placeable item", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  const boards = await (await request.get(`${API}/boards`)).json();
  const boardId: string = boards[0].id;

  const panel = page.locator("aside").filter({ has: page.getByPlaceholder("搜索素材…") });
  await page.getByRole("button", { name: "1–10MB" }).click();
  await page.waitForTimeout(400);
  await expect(panel.getByText(/素材 · 1/)).toBeVisible({ timeout: 5_000 });
  await expect(panel.locator("[draggable='true']").first()).toBeVisible();

  const assets = await (await request.get(`${API}/assets?minSize=1048576`)).json();
  const placed = await request.post(`${API}/boards/${boardId}/items`, {
    data: { asset_id: assets[0].id, x: 100, y: 100, w: 200, h: 150, rotation: 0, z: 0 },
  });
  expect(placed.ok()).toBeTruthy();
  const board = await (await request.get(`${API}/boards/${boardId}`)).json();
  expect(board.items?.length).toBe(1);
  expect(board.items[0].asset_id).toBe(assets[0].id);

  await page.screenshot({ path: "e2e/screenshots/dim-board-filtered-drag.png" });
});
