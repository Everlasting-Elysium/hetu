import { test, expect } from "@playwright/test";

// Browse layouts that render a keyed container we can assert + screenshot.
const VIEWS = [
  { name: "网格", testid: "grid-view", file: "grid" },
  { name: "瀑布流", testid: "waterfall-view", file: "waterfall" },
  { name: "画廊", testid: "gallery-view", file: "gallery" },
] as const;

test("switch through all views + immersive keyboard nav, no console errors", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") errors.push(msg.text());
  });
  page.on("pageerror", (err) => errors.push(err.message));

  // Hermetic start: ignore any persisted layout from a previous run.
  await page.addInitScript(() => {
    try {
      localStorage.clear();
    } catch {
      /* storage unavailable */
    }
  });
  await page.goto("/");

  // App shell is up once the view switcher renders.
  await expect(page.getByTestId("view-grid")).toBeVisible();

  for (const v of VIEWS) {
    await page.getByRole("button", { name: v.name }).click();
    await expect(page.getByTestId(v.testid)).toBeVisible();
    await page.waitForTimeout(600); // let thumbnails/layout settle for evidence
    await page.screenshot({ path: `tests/screenshots/${v.file}.png` });
  }

  // Immersive: enter, navigate with the keyboard, capture, then Escape to exit.
  await page.getByRole("button", { name: "沉浸" }).click();
  const overlay = page.getByTestId("immersive-overlay");
  await expect(overlay).toBeVisible();
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowLeft");
  await page.waitForTimeout(400);
  await page.screenshot({ path: "tests/screenshots/immersive.png" });
  await page.keyboard.press("Escape");
  await expect(overlay).toBeHidden();

  // A missing favicon is not an application error.
  const real = errors.filter((e) => !/favicon/i.test(e));
  expect(real, `unexpected console errors:\n${real.join("\n")}`).toEqual([]);
});

test("browse layout preference persists across reload", async ({ page }) => {
  // Clear once via evaluate (not addInitScript, which would re-run on reload and
  // wipe the very preference we are trying to verify survives a reload).
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
  await page.getByRole("button", { name: "瀑布流" }).click();
  await expect(page.getByTestId("waterfall-view")).toBeVisible();

  await page.reload();
  // The stored layout is restored — no need to re-click.
  await expect(page.getByTestId("waterfall-view")).toBeVisible();
});
