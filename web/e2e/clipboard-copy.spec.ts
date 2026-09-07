import { test, expect } from "@playwright/test";

// E2E smoke tests for issue #90: Ctrl/Cmd+C copies selected assets to the
// system clipboard. Image assets write a PNG bitmap + URL text; non-image
// assets write URL text only.
//
// Clipboard API in Playwright/Chromium requires explicit permission grants
// on the browser context. We override context creation to inject them.

const API = "/api/dam";

interface AssetRow {
  id: string;
  kind: string;
  name: string;
}

// Grant clipboard permissions at the context level so navigator.clipboard.write
// succeeds inside the page. Playwright's default Chromium context blocks it.
test.use({
  permissions: ["clipboard-read", "clipboard-write"],
});

test.describe("Clipboard copy smoke (#90)", () => {
  let assets: AssetRow[] = [];
  let hasImage = false;
  let hasNonImage = false;

  test.beforeAll(async ({ request }) => {
    const res = await request.get(`${API}/assets?limit=50`);
    if (!res.ok()) return;
    assets = (await res.json()) as AssetRow[];
    hasImage = assets.some((a) => a.kind === "image");
    hasNonImage = assets.some((a) => a.kind !== "image");
  });

  test.beforeEach(async ({ page }) => {
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

  test("Ctrl+C on selected image → clipboard has image/png, toast confirms", async ({
    page,
  }) => {
    test.skip(!hasImage, "needs at least 1 image asset");

    // Find the index of the first image in the loaded asset list.
    const imgIdx = assets.findIndex((a) => a.kind === "image");
    const grid = page.getByTestId("grid-view");
    const checks = grid.locator('button[title="选择"]');
    await expect(checks.nth(imgIdx)).toBeVisible();

    // Select the image card via its checkbox.
    await checks.nth(imgIdx).click();
    await expect(page.getByText(/已选/)).toBeVisible();

    // Trigger Ctrl+C (Playwright sends Meta+C on macOS automatically when
    // using the keyboard shortcut approach via modifier).
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);

    // Toast should confirm an image was copied.
    await expect(page.getByText(/已复制图片/)).toBeVisible({ timeout: 10_000 });

    // Verify clipboard contains image/png via page.evaluate.
    const hasBlob = await page.evaluate(async () => {
      try {
        const items = await navigator.clipboard.read();
        if (items.length === 0) return false;
        const types = items[0].types;
        return types.includes("image/png");
      } catch {
        return false;
      }
    });
    expect(hasBlob).toBe(true);

    await page.screenshot({ path: "e2e/screenshots/clipboard-image-copy.png" });
  });

  test("Ctrl+C on non-image asset → clipboard has text URL, toast says '已复制链接'", async ({
    page,
  }) => {
    test.skip(!hasNonImage, "needs at least 1 non-image asset");

    const nonImgIdx = assets.findIndex((a) => a.kind !== "image");
    const grid = page.getByTestId("grid-view");
    const checks = grid.locator('button[title="选择"]');
    await expect(checks.nth(nonImgIdx)).toBeVisible();

    await checks.nth(nonImgIdx).click();
    await expect(page.getByText(/已选/)).toBeVisible();

    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);

    await expect(page.getByText(/已复制.*链接/)).toBeVisible({ timeout: 10_000 });

    // Clipboard text should be an absolute URL pointing to the asset's file endpoint.
    const text = await page.evaluate(async () => {
      try {
        return await navigator.clipboard.readText();
      } catch {
        return "";
      }
    });
    expect(text).toContain("/api/dam/assets/");
    expect(text).toContain("/file");
    // Must be absolute (starts with http).
    expect(text).toMatch(/^https?:\/\//);

    await page.screenshot({ path: "e2e/screenshots/clipboard-url-copy.png" });
  });

  test("multi-select → Ctrl+C → clipboard text has multiple URLs", async ({
    page,
  }) => {
    test.skip(assets.length < 2, "needs at least 2 assets");

    const grid = page.getByTestId("grid-view");
    const checks = grid.locator('button[title="选择"]');
    await expect(checks.nth(1)).toBeVisible();

    // Select first two assets.
    await checks.nth(0).click();
    await checks.nth(1).click();
    await expect(page.getByText(/已选.*2.*项/)).toBeVisible();

    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);

    // Wait for the copy toast.
    await expect(
      page.getByText(/已复制/).first(),
    ).toBeVisible({ timeout: 10_000 });

    const text = await page.evaluate(async () => {
      try {
        return await navigator.clipboard.readText();
      } catch {
        return "";
      }
    });
    const lines = text.split("\n").filter(Boolean);
    expect(lines.length).toBe(2);
    for (const line of lines) {
      expect(line).toMatch(/^https?:\/\//);
      expect(line).toContain("/api/dam/assets/");
    }

    await page.screenshot({ path: "e2e/screenshots/clipboard-multi-copy.png" });
  });

  test("Ctrl+C with no selection and no focus → does NOT hijack, no toast", async ({
    page,
  }) => {
    // Just press Ctrl+C on an empty selection — should be a no-op.
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);

    // Short wait — no toast should appear.
    await page.waitForTimeout(500);
    await expect(page.getByText(/已复制/)).toBeHidden();
  });

  test("Ctrl+C in search input does NOT hijack native copy", async ({
    page,
  }) => {
    // Focus the search input, type text, select it, then Ctrl+C should NOT
    // trigger asset copy — the native text copy should work instead.
    const search = page.locator('input[type="search"], input[placeholder*="搜索"]').first();
    if (!(await search.isVisible())) {
      test.skip(true, "no search input visible");
      return;
    }
    await search.fill("test");
    await search.selectText();

    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);

    // No asset-copy toast should appear.
    await page.waitForTimeout(500);
    await expect(page.getByText(/已复制图片|已复制.*链接/)).toBeHidden();
  });

  test("immersive view Ctrl+C copies current asset", async ({ page }) => {
    test.skip(assets.length < 1, "needs at least 1 asset");

    const grid = page.getByTestId("grid-view");
    const checks = grid.locator('button[title="选择"]');
    await expect(checks.first()).toBeVisible();

    // Select first asset then enter immersive view.
    await checks.nth(0).click();
    await expect(page.getByText(/已选/)).toBeVisible();

    // The immersive view is opened via the view switcher; look for the button.
    const immBtn = page.getByTestId("view-immersive");
    if (!(await immBtn.isVisible())) {
      test.skip(true, "no immersive view button");
      return;
    }
    await immBtn.click();
    await expect(page.getByTestId("immersive-overlay")).toBeVisible();

    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);

    // Should see a copy toast.
    await expect(
      page.getByText(/已复制/).first(),
    ).toBeVisible({ timeout: 10_000 });

    await page.screenshot({ path: "e2e/screenshots/clipboard-immersive-copy.png" });

    // Exit immersive.
    await page.keyboard.press("Escape");
  });

  test("no console errors during clipboard operations", async ({ page }) => {
    test.skip(assets.length < 1, "needs at least 1 asset");

    const errors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") errors.push(msg.text());
    });
    page.on("pageerror", (err) => errors.push(err.message));

    const grid = page.getByTestId("grid-view");
    const checks = grid.locator('button[title="选择"]');
    await expect(checks.first()).toBeVisible();
    await checks.nth(0).click();

    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+c`);
    await expect(page.getByText(/已复制/).first()).toBeVisible({ timeout: 10_000 });

    // Filter out expected noise: favicon 404, thumbnail 404.
    const real = errors.filter((e) => !/favicon/i.test(e) && !/404/i.test(e));
    expect(real, `unexpected console errors:\n${real.join("\n")}`).toEqual([]);
  });
});
