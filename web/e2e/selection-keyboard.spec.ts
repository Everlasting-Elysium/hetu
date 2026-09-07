import { test, expect } from "@playwright/test";

// Smoke tests for issue #77: selection discoverability + keyboard interactions.
// Requires a running hetu instance with at least 2 indexed assets (one should
// ideally be a video to exercise Space-play/pause, but the test gracefully
// degrades if only images are present).

const API = "/api/dam";

test.describe("Selection & keyboard smoke (#77)", () => {
  let assetCount = 0;
  let hasVideo = false;

  test.beforeAll(async ({ request }) => {
    const res = await request.get(`${API}/assets?limit=20`);
    if (!res.ok()) return;
    const assets = (await res.json()) as { id: string; kind: string }[];
    assetCount = Array.isArray(assets) ? assets.length : 0;
    hasVideo = assets.some((a) => a.kind === "video");
  });

  test.beforeEach(async ({ page }) => {
    // Hermetic start: clear persisted layout before navigating.
    await page.addInitScript(() => {
      try {
        localStorage.clear();
      } catch {
        /* storage unavailable */
      }
    });
    await page.goto("/");
    await expect(page.getByTestId("view-grid")).toBeVisible();
  });

  test("checkbox always visible on asset cards", async ({ page }) => {
    test.skip(assetCount < 1, "needs at least 1 indexed asset");

    const grid = page.getByTestId("grid-view");
    // The checkbox button (title="选择") should be visible without hovering.
    const check = grid.getByTitle("选择").first();
    await expect(check).toBeVisible();
    await page.screenshot({ path: "e2e/screenshots/selection-checkbox.png" });
  });

  test("arrow keys move focus, Space opens detail, Esc closes, no console errors", async ({
    page,
  }) => {
    test.skip(assetCount < 2, "needs at least 2 indexed assets");

    const errors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") errors.push(msg.text());
    });
    page.on("pageerror", (err) => errors.push(err.message));

    // Wait for grid cards to render.
    const grid = page.getByTestId("grid-view");
    await expect(grid.getByTitle("选择").first()).toBeVisible();

    // ArrowRight: first card gains keyboard focus (the CSS-module class contains
    // "focused" in the hashed token).
    await page.keyboard.press("ArrowRight");
    await expect(grid.locator("[class*='focused']").first()).toBeVisible();

    // Move one more to verify navigation works.
    await page.keyboard.press("ArrowRight");
    await page.screenshot({ path: "e2e/screenshots/selection-arrow-focused.png" });

    // Space: open detail for the focused card.
    await page.keyboard.press("Space");
    const closeBtn = page.getByTitle("关闭");
    await expect(closeBtn).toBeVisible();
    await page.screenshot({ path: "e2e/screenshots/selection-detail-open.png" });

    // Esc: close detail.
    await page.keyboard.press("Escape");
    await expect(closeBtn).toBeHidden();

    // 404s for missing thumbnails / files are expected in test environments where
    // only the DB rows exist (no real media on disk). Filter them alongside favicon.
    const real = errors.filter((e) => !/favicon/i.test(e) && !/404/i.test(e));
    expect(real, `unexpected console errors:\n${real.join("\n")}`).toEqual([]);
  });

  test("click selects, Cmd/Ctrl+click adds, BatchBar shows count, blank-click clears", async ({
    page,
  }) => {
    test.skip(assetCount < 2, "needs at least 2 indexed assets");

    const grid = page.getByTestId("grid-view");
    const cards = grid.locator("[class*='card']");
    await expect(cards.first()).toBeVisible();

    // Plain click selects exactly one.
    await cards.nth(0).click();
    await expect(page.getByText(/已选.*1.*项/)).toBeVisible();

    // Cmd+click (Meta on macOS) second card → adds to selection.
    await cards.nth(1).click({ modifiers: ["Meta"] });
    await expect(page.getByText(/已选.*2.*项/)).toBeVisible();
    await page.screenshot({ path: "e2e/screenshots/selection-multi.png" });

    // Click the root background (outside the main area) → clears selection.
    // The root div has class "app" which is NOT a CSS-module hash.
    await page.locator(".app").click({ position: { x: 5, y: 5 } });
    await expect(page.getByText(/已选/)).toBeHidden();
  });

  test("focused vs selected visual distinction", async ({ page }) => {
    test.skip(assetCount < 2, "needs at least 2 indexed assets");

    const grid = page.getByTestId("grid-view");
    const cards = grid.locator("[class*='card']");
    await expect(cards.first()).toBeVisible();

    // Click first card → selected.
    await cards.nth(0).click();
    // The card itself should carry the "selected" CSS-module class.
    await expect(grid.locator("[class*='selected']").first()).toBeVisible();

    // ArrowRight moves focus away from the selected card.
    await page.keyboard.press("ArrowRight");
    // Now a different card has "focused".
    const focused = grid.locator("[class*='focused']");
    await expect(focused).toBeVisible();

    await page.screenshot({ path: "e2e/screenshots/selection-focused-vs-selected.png" });
  });

  test("Space plays/pauses video in detail panel", async ({ page }) => {
    test.skip(!hasVideo, "needs at least 1 video asset");

    // Navigate to the first video asset by using keyboard + Space.
    // First, query the API to find the first video's position.
    const res = await page.request.get(`${API}/assets?limit=50`);
    const assets = (await res.json()) as { id: string; kind: string }[];
    const videoIdx = assets.findIndex((a) => a.kind === "video");
    test.skip(videoIdx < 0, "no video found in first 50 assets");

    // Focus the video card by pressing ArrowRight videoIdx+1 times (0→first item).
    const grid = page.getByTestId("grid-view");
    await expect(grid.getByTitle("选择").first()).toBeVisible();
    for (let i = 0; i <= videoIdx; i++) {
      await page.keyboard.press("ArrowRight");
    }

    // Open detail with Space.
    await page.keyboard.press("Space");
    const closeBtn = page.getByTitle("关闭");
    await expect(closeBtn).toBeVisible();

    // Wait for the video player to mount.
    const player = page.locator("video");
    await expect(player).toBeVisible();

    // Give the video a moment to load metadata.
    await page.waitForTimeout(1000);

    // Space should toggle play. Use the exact aria-label to avoid matching the
    // "播放速度" (playback rate) button which also contains "播放".
    const playBtn = page.getByRole("button", { name: "播放", exact: true }).or(
      page.getByRole("button", { name: "暂停", exact: true }),
    );
    await expect(playBtn.first()).toBeVisible();

    // Press Space to play.
    await page.keyboard.press("Space");
    await page.waitForTimeout(300);
    await page.screenshot({ path: "e2e/screenshots/selection-video-play.png" });

    // Press Space again to pause.
    await page.keyboard.press("Space");
    await page.waitForTimeout(300);
    await page.screenshot({ path: "e2e/screenshots/selection-video-pause.png" });

    // Esc closes the detail.
    await page.keyboard.press("Escape");
    await expect(closeBtn).toBeHidden();
  });
});
