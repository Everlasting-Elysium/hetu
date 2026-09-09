import { test, expect, type Page } from "@playwright/test";

// Smoke tests for issue #53: duration (时长) + created/indexed time (创建时间/
// 索引时间) facets, layered on the #75/#101 facet pipeline (facets.spec.ts /
// facets-dimensions.spec.ts). Like that suite this targets a DEDICATED scratch
// server/library seeded with 4 purpose-built media fixtures whose durations are
// deterministic (committed under web/e2e/fixtures/):
//   dur-audio-45s.mp3    45s  (0.75 min -> "< 1分钟" bucket, audio)
//   dur-video-90s.mp4    90s  (1.5  min -> "1–5分钟"  bucket, video)
//   dur-video-330s.mp4  330s  (5.5  min -> "> 5分钟"  bucket, video)
//   dur-image.png        —    (an image: no duration; must fall in NO band)
// Seed a scratch library with exactly these four, scan it with ffmpeg/ffprobe on
// PATH (so audio.duration/video.duration annotations are written), then run with
// BASE_URL pointing at that server. All four are scanned at ~now, so the time
// facet is exercised with bounds relative to the run clock rather than fixed dates.

const API = "/api/dam";

// 时长/创建时间/索引时间 facets now live under a collapsed 高级筛选 disclosure
// (default collapsed when no advanced filter is active), so open it before
// touching those controls; the common 格式/星级/标签 tier is unaffected.
async function openAdvanced(page: Page) {
  await page.getByTestId("advanced-toggle").click();
}

// ---------------------------------------------------------------------------
// Pure API smoke tests — verify backend filtering logic without a browser.
// ---------------------------------------------------------------------------

test("?minDuration=/?maxDuration= narrow on seconds", async ({ request }) => {
  const long = await (await request.get(`${API}/assets?minDuration=60`)).json();
  expect(long.length).toBe(2); // 90s + 330s
  const short = await (await request.get(`${API}/assets?maxDuration=60`)).json();
  expect(short.length).toBe(1); // only the 45s clip is <= 60s
  expect(short[0].name).toBe("dur-audio-45s.mp3");
});

test("duration band isolates a single clip", async ({ request }) => {
  const band = await (await request.get(`${API}/assets?minDuration=60&maxDuration=120`)).json();
  expect(band.length).toBe(1);
  expect(band[0].name).toBe("dur-video-90s.mp4");
});

test("a band spanning the divide returns both an audio and a video clip", async ({ request }) => {
  const res = await (await request.get(`${API}/assets?minDuration=40&maxDuration=100`)).json();
  const kinds = res.map((a: { kind: string }) => a.kind).sort();
  expect(kinds).toEqual(["audio", "video"]); // 45s audio + 90s video
});

test("any duration bound excludes the durationless image", async ({ request }) => {
  const withDur = await (await request.get(`${API}/assets?maxDuration=100000`)).json();
  expect(withDur.length).toBe(3); // the image (no duration annotation) drops out
  expect(withDur.every((a: { name: string }) => a.name !== "dur-image.png")).toBe(true);
});

test("?createdBefore=/?createdAfter= take effect (all scanned ~now)", async ({ request }) => {
  const future = Math.floor(Date.now() / 1000) + 3600;
  const past = Math.floor(Date.now() / 1000) - 86400;
  const before = await (await request.get(`${API}/assets?createdBefore=${future}`)).json();
  expect(before.length).toBe(4); // everything was created before an hour from now
  const after = await (await request.get(`${API}/assets?createdAfter=${future}`)).json();
  expect(after.length).toBe(0); // nothing created in the future
  const indexed = await (await request.get(`${API}/assets?indexedAfter=${past}`)).json();
  expect(indexed.length).toBe(4); // everything was indexed after a day ago
});

test("duration + kind compose (AND): video AND >=60s excludes the audio clip", async ({
  request,
}) => {
  const res = await (await request.get(`${API}/assets?kind=video&minDuration=60`)).json();
  const names = res.map((a: { name: string }) => a.name).sort();
  expect(names).toEqual(["dur-video-330s.mp4", "dur-video-90s.mp4"]);
});

// ---------------------------------------------------------------------------
// UI: main library sidebar — duration + time facets.
// ---------------------------------------------------------------------------

test("duration preset '< 1分钟' narrows the grid to the 45s clip", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await openAdvanced(page);

  await page.getByRole("button", { name: "< 1分钟" }).click();
  await page.waitForTimeout(400);
  await expect(
    page.locator("[data-testid='grid-view'] >> img, [data-testid='grid-view'] svg"),
  ).toHaveCount(1, { timeout: 5000 });
  await page.screenshot({ path: "e2e/screenshots/dur-facet-under-1min.png" });

  // The 时长 section's 清除 (the only active facet) resets the range input.
  await page.getByRole("button", { name: "清除", exact: true }).first().click();
  await expect(page.getByLabel("最小时长（分钟）")).toHaveValue("");
});

test("duration preset '1–5分钟' isolates the 90s video", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await openAdvanced(page);

  await page.getByRole("button", { name: "1–5分钟" }).click();
  await page.waitForTimeout(400);
  await expect(
    page.locator("[data-testid='grid-view'] >> img, [data-testid='grid-view'] svg"),
  ).toHaveCount(1, { timeout: 5000 });
  await page.screenshot({ path: "e2e/screenshots/dur-facet-1-5min.png" });
});

test("duration min-minutes input narrows after the debounce settles", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await openAdvanced(page);

  // 5 minutes = 300s: only the 330s video qualifies. RangeField commits minutes
  // as seconds; the field debounces ~300ms so wait past it before asserting.
  await page.getByLabel("最小时长（分钟）").fill("5");
  await page.waitForTimeout(500);
  await expect(
    page.locator("[data-testid='grid-view'] >> img, [data-testid='grid-view'] svg"),
  ).toHaveCount(1, { timeout: 5000 });
  await page.screenshot({ path: "e2e/screenshots/dur-facet-min-minutes.png" });

  await page.getByRole("button", { name: "清除", exact: true }).first().click();
  await expect(page.getByLabel("最小时长（分钟）")).toHaveValue("");
});

test("created-time facet: a future start date empties the grid, clearing restores it", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await openAdvanced(page);

  // Everything was indexed ~now, so a start date well in the future excludes all.
  await page.getByLabel("起始创建时间").fill("2099-01-01");
  await page.waitForTimeout(400);
  await expect(
    page.locator("[data-testid='grid-view'] >> img, [data-testid='grid-view'] svg"),
  ).toHaveCount(0, { timeout: 5000 });
  await page.screenshot({ path: "e2e/screenshots/time-facet-future-empty.png" });

  await page.getByRole("button", { name: "清除", exact: true }).first().click();
  await expect(
    page.locator("[data-testid='grid-view'] >> img, [data-testid='grid-view'] svg"),
  ).toHaveCount(4, { timeout: 5000 });
});

test("clearing duration does not clear an independently-set created-time filter", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await openAdvanced(page);

  await page.getByRole("button", { name: "> 5分钟" }).click();
  await page.getByLabel("起始创建时间").fill("2099-01-01");
  await page.waitForTimeout(500);

  // Clear only the 时长 section (its own 清除, first of the two active sections;
  // exact:true excludes the 高级筛选 header clear-all, named 清除高级筛选).
  await page.getByRole("button", { name: "清除", exact: true }).first().click();
  // The created-time filter the user set is untouched by the duration clear.
  await expect(page.getByLabel("起始创建时间")).toHaveValue("2099-01-01");
});

// ---------------------------------------------------------------------------
// UI: board view — the sidebar duration facet narrows the drag-source panel in
// place (issue #108 routes all board-panel filtering through the sidebar).
// ---------------------------------------------------------------------------

test("board view: sidebar duration facet narrows the drag source panel", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  await page.getByText("图板", { exact: true }).click();
  await page.getByText("新建图板").click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 8_000 });

  const panel = page.locator("aside").filter({ has: page.getByPlaceholder("搜索素材…") });
  await openAdvanced(page);

  // Clicking the sidebar facet must keep the user on the board while narrowing
  // the panel (issue #108). "1–5分钟" isolates the single 90s video.
  await page.getByRole("button", { name: "1–5分钟" }).click();
  await page.waitForTimeout(400);
  await expect(page.getByText("返回图板列表")).toBeVisible();
  await expect(panel.getByText(/素材 · 1/)).toBeVisible({ timeout: 5_000 });
  await page.screenshot({ path: "e2e/screenshots/dur-board-1-5min.png" });

  await page.getByRole("button", { name: "清除", exact: true }).first().click();
});
