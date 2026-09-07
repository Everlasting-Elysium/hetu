import { expect, test } from "@playwright/test";

// Evidence screenshots land here; override with SHOT_DIR to keep them out of the
// repo (e.g. a temp dir) when running against a real deployment.
const SHOT_DIR = process.env.SHOT_DIR ?? "test-results/model-viewer";

// The library must contain an indexed 3D model whose name matches; override with
// MODEL_NAME. The deploy step scans a library holding cube.glb.
const MODEL_NAME = process.env.MODEL_NAME ?? "cube.glb";

// The detail modal (.overlay) and the inspector sidebar both mount a
// <model-viewer>. All selectors MUST be scoped to the modal overlay to avoid
// strict-mode violations from the duplicate element.
const MODAL_VIEWER = '[class*="overlay"] model-viewer';
const MODAL_VIEWER_QS = '[class*="overlay"] model-viewer';

// Collect console errors while ignoring benign 404s from thumbnail requests
// (3D fixtures have no Blender sidecar → /thumb always 404s in E2E).
function wireErrorCollector(page: import("@playwright/test").Page): string[] {
  const errors: string[] = [];
  const pendingThumb404s = { count: 0 };
  page.on("response", (r) => {
    if (r.status() === 404 && /\/thumb\b/.test(r.url())) pendingThumb404s.count++;
  });
  page.on("console", (m) => {
    if (m.type() !== "error") return;
    // Chrome emits a generic "Failed to load resource…404" for every network
    // 404 — match it against pending thumbnail 404s and discard.
    if (pendingThumb404s.count > 0 && /failed to load resource/i.test(m.text())) {
      pendingThumb404s.count--;
      return;
    }
    errors.push(m.text());
  });
  page.on("pageerror", (e) => errors.push(String(e)));
  return errors;
}

test("3D viewer: load → rotate → toggle material, no console errors", async ({ page }) => {
  const errors = wireErrorCollector(page);

  await page.goto("/");

  // The library grid shows the indexed model; open its detail modal (double-click).
  const card = page.locator('[class*="card"]', { hasText: MODEL_NAME }).first();
  await expect(card).toBeVisible();
  await page.screenshot({ path: `${SHOT_DIR}/01-grid.png`, fullPage: true });
  await card.dblclick();

  // The detail modal mounts <model-viewer>; wait for the model to finish loading.
  // Scope to the modal overlay so the sidebar's viewer doesn't cause ambiguity.
  const viewer = page.locator(MODAL_VIEWER);
  await expect(viewer).toBeVisible();
  await page.waitForFunction(
    (sel) => {
      const mv = document.querySelector(sel) as
        | (Element & { loaded?: boolean })
        | null;
      return !!mv && mv.loaded === true;
    },
    MODAL_VIEWER_QS,
    { timeout: 45_000 },
  );
  await page.screenshot({ path: `${SHOT_DIR}/02-loaded.png` });

  // Rotate: drag across the viewer to orbit the camera.
  const box = await viewer.boundingBox();
  if (!box) throw new Error("model-viewer has no bounding box");
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.78, box.y + box.height * 0.38, { steps: 14 });
  await page.mouse.move(box.x + box.width * 0.28, box.y + box.height * 0.62, { steps: 14 });
  await page.mouse.up();
  await page.screenshot({ path: `${SHOT_DIR}/03-rotated.png` });

  // Toggle material (original → clay → original); must not raise console errors.
  const overlay = page.locator('[class*="overlay"]');
  const materialBtn = overlay.getByTestId("toggle-material");
  await materialBtn.click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: `${SHOT_DIR}/04-material-clay.png` });
  await materialBtn.click();
  await page.waitForTimeout(200);

  expect(errors, `unexpected console errors:\n${errors.join("\n")}`).toHaveLength(0);
});

test("3D viewer: auto-rotate toggle enables and disables rotation", async ({ page }) => {
  const errors = wireErrorCollector(page);

  await page.goto("/");

  // Open the 3D model detail.
  const card = page.locator('[class*="card"]', { hasText: MODEL_NAME }).first();
  await expect(card).toBeVisible();
  await card.dblclick();

  // Wait for the modal's model-viewer to finish loading.
  const viewer = page.locator(MODAL_VIEWER);
  await expect(viewer).toBeVisible();
  await page.waitForFunction(
    (sel) => {
      const mv = document.querySelector(sel) as
        | (Element & { loaded?: boolean })
        | null;
      return !!mv && mv.loaded === true;
    },
    MODAL_VIEWER_QS,
    { timeout: 45_000 },
  );

  const overlay = page.locator('[class*="overlay"]');
  const autoRotateBtn = overlay.getByTestId("toggle-auto-rotate");
  await expect(autoRotateBtn).toBeVisible();

  // Before clicking: auto-rotate is off.
  const beforeClick = await page.evaluate((sel) => {
    const mv = document.querySelector(sel) as
      | (Element & { autoRotate?: boolean })
      | null;
    return mv?.autoRotate ?? null;
  }, MODAL_VIEWER_QS);
  expect(beforeClick, "auto-rotate should be off initially").toBe(false);

  // Click → enable auto-rotate.
  await autoRotateBtn.click();
  await page.waitForTimeout(200);

  const afterEnable = await page.evaluate((sel) => {
    const mv = document.querySelector(sel) as
      | (Element & { autoRotate?: boolean })
      | null;
    return mv?.autoRotate ?? null;
  }, MODAL_VIEWER_QS);
  expect(afterEnable, "auto-rotate should be on after first click").toBe(true);
  // Button should have the active class.
  await expect(autoRotateBtn).toHaveAttribute("class", /active/);
  await page.screenshot({ path: `${SHOT_DIR}/05-auto-rotate-on.png` });

  // Click again → disable auto-rotate.
  await autoRotateBtn.click();
  await page.waitForTimeout(200);

  const afterDisable = await page.evaluate((sel) => {
    const mv = document.querySelector(sel) as
      | (Element & { autoRotate?: boolean })
      | null;
    return mv?.autoRotate ?? null;
  }, MODAL_VIEWER_QS);
  expect(afterDisable, "auto-rotate should be off after second click").toBe(false);
  // Button should lose the active class.
  await expect(autoRotateBtn).not.toHaveAttribute("class", /active/);
  await page.screenshot({ path: `${SHOT_DIR}/06-auto-rotate-off.png` });

  expect(errors, `unexpected console errors:\n${errors.join("\n")}`).toHaveLength(0);
});
