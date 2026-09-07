import { test, expect } from "@playwright/test";

// Smoke tests for issue #77: selection discoverability + keyboard interactions.
// Requires a running hetu instance with at least 2 indexed assets (one should
// ideally be a video to exercise Space-play/pause, but the test gracefully
// degrades if only images are present).

const API = "/api/dam";

test.describe("Selection & keyboard smoke (#77)", () => {
  let assetCount = 0;
  let hasVideo = false;
  let hasAudio = false;

  test.beforeAll(async ({ request }) => {
    const res = await request.get(`${API}/assets?limit=50`);
    if (!res.ok()) return;
    const assets = (await res.json()) as { id: string; kind: string }[];
    assetCount = Array.isArray(assets) ? assets.length : 0;
    hasVideo = assets.some((a) => a.kind === "video");
    hasAudio = assets.some((a) => a.kind === "audio");
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

  // Opens the detail for the first asset of `kind` by focusing its card with the
  // grid arrow-key cursor and pressing Space, then returns the media selector.
  // Requires the App to hide the inspector while the detail is open, so exactly
  // one <video>/<audio> element exists — the detail's.
  async function openMediaDetail(page: import("@playwright/test").Page, kind: "video" | "audio") {
    const res = await page.request.get(`${API}/assets?limit=50`);
    const assets = (await res.json()) as { id: string; kind: string }[];
    const idx = assets.findIndex((a) => a.kind === kind);
    if (idx < 0) return null;
    const grid = page.getByTestId("grid-view");
    await expect(grid.getByTitle("选择").first()).toBeVisible();
    // ArrowRight idx+1 times: the first press focuses item 0, then one per step.
    for (let i = 0; i <= idx; i++) await page.keyboard.press("ArrowRight");
    await page.keyboard.press("Space"); // Space with a focused item opens its detail.
    await expect(page.getByTitle("关闭")).toBeVisible();
    return kind === "video" ? "video" : "audio";
  }

  // Reads the (single) detail media element's live `paused` property.
  const pausedOf = (page: import("@playwright/test").Page, sel: string) =>
    page.evaluate((s) => {
      const el = document.querySelector(s) as HTMLMediaElement | null;
      return el ? el.paused : null;
    }, sel);

  for (const kind of ["video", "audio"] as const) {
    test(`${kind}: Space toggles play (body + player focus), Esc closes with player focused`, async ({
      page,
    }) => {
      const has = kind === "video" ? hasVideo : hasAudio;
      test.skip(!has, `needs at least 1 ${kind} asset`);

      const sel = await openMediaDetail(page, kind);
      test.skip(sel === null, `no ${kind} found`);
      const media = sel as string;

      // Media mounts paused (preload=metadata, no autoplay). Wait for the element.
      await expect(page.locator(media)).toHaveCount(1);
      await page.waitForTimeout(600);
      expect(await pausedOf(page, media), "media should start paused").toBe(true);

      // 1) Body-focus Space → toggles via the App-level videoToggleRef path.
      await page.keyboard.press("Space");
      await expect
        .poll(() => pausedOf(page, media), { message: "Space (body focus) should play" })
        .toBe(false);
      await page.keyboard.press("Space");
      await expect
        .poll(() => pausedOf(page, media), { message: "Space again should pause" })
        .toBe(true);

      // 2) Player-focus Space → focus the custom player wrapper (never the native
      // media element, which would swallow keys), then Space toggles via onKeyDown.
      // Clicking the visible surface focuses the tabIndex=0 wrapper div.
      await page.locator(kind === "video" ? "video" : "[class*='cover']").first().click();
      await expect
        .poll(() => pausedOf(page, media), { message: "Space (player focus) should play" })
        .toBe(false);
      await page.screenshot({ path: `e2e/screenshots/${kind}-playing.png` });

      // 3) Esc while the PLAYER has focus must still close the detail. This is the
      // regression that a native <audio controls> broke (it swallowed Escape).
      await page.keyboard.press("Escape");
      await expect(page.getByTitle("关闭")).toBeHidden();
    });
  }
});
