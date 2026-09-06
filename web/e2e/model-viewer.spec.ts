import { expect, test } from "@playwright/test";

// Evidence screenshots land here; override with SHOT_DIR to keep them out of the
// repo (e.g. a temp dir) when running against a real deployment.
const SHOT_DIR = process.env.SHOT_DIR ?? "test-results/model-viewer";

// The library must contain an indexed 3D model whose name matches; override with
// MODEL_NAME. The deploy step scans a library holding cube.glb.
const MODEL_NAME = process.env.MODEL_NAME ?? "cube.glb";

test("3D viewer: load → rotate → toggle material, no console errors", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  page.on("pageerror", (e) => errors.push(String(e)));

  await page.goto("/");

  // The library grid shows the indexed model; open its detail modal (double-click).
  const card = page.locator('[class*="card"]', { hasText: MODEL_NAME }).first();
  await expect(card).toBeVisible();
  await page.screenshot({ path: `${SHOT_DIR}/01-grid.png`, fullPage: true });
  await card.dblclick();

  // The detail modal mounts <model-viewer>; wait for the model to finish loading.
  const viewer = page.locator("model-viewer");
  await expect(viewer).toBeVisible();
  await page.waitForFunction(
    () => {
      const mv = document.querySelector("model-viewer") as
        | (Element & { loaded?: boolean })
        | null;
      return !!mv && mv.loaded === true;
    },
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
  const materialBtn = page.getByTestId("toggle-material");
  await materialBtn.click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: `${SHOT_DIR}/04-material-clay.png` });
  await materialBtn.click();
  await page.waitForTimeout(200);

  expect(errors, `unexpected console errors:\n${errors.join("\n")}`).toHaveLength(0);
});
